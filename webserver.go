package main

import (
	"database/sql"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type WebServer struct {
	decider        *Decider
	config         *Config
	dhcp_tailer    *DhcpStatus
	server_started time.Time
	servlets       map[string]func(http.ResponseWriter, *http.Request)
	last_update    time.Time
}

func NewWebServer(c *Config, dhcp *DhcpStatus, decider *Decider) *WebServer {
	t := new(WebServer)
	t.decider = decider
	t.dhcp_tailer = dhcp
	t.config = c
	t.server_started = time.Now().Round(time.Second)
	t.servlets = make(map[string]func(http.ResponseWriter, *http.Request))
	t.servlets["/control"] = t.ControlPage
	t.servlets["/graph"] = http.FileServer(http.Dir("/var/www/nest")).ServeHTTP
	t.last_update = time.Now()
	go t.disconnectWatchdog()
	return t
}

// Watchdog that sends us an email if the base station fails to update within
// a given amount of time
func (t *WebServer) disconnectWatchdog() {
	for {
		time.Sleep(1 * time.Minute)
		if time.Now().Sub(t.last_update) > time.Minute*5 {
			// Send a warning email
			// Connect to the remote SMTP server.
			conn, err := smtp.Dial(t.config.Mail.Host)
			if err != nil {
				log.Println(err)
				continue
			}

			// Set the sender and recipient first
			if err := conn.Mail("ernest"); err != nil {
				log.Println(err)
				continue
			}
			if err := conn.Rcpt(t.config.Mail.Target); err != nil {
				log.Println(err)
				continue
			}

			// Send the email body.
			wc, err := conn.Data()
			if err != nil {
				log.Println(err)
				continue
			}
			_, err = fmt.Fprintf(wc, `Subject: No data warning for Ernest
Just a heads up, but there have been no communications from the Ernest base station
within the past 5 minutes.`)
			if err != nil {
				log.Println(err)
				continue
			}
			err = wc.Close()
			if err != nil {
				log.Println(err)
				continue
			}

			// Send the QUIT command and close the connection.
			err = conn.Quit()
			if err != nil {
				log.Println(err)
				continue
			}
		}
	}
}

func (t *WebServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for path, servlet := range t.servlets {
		if strings.HasPrefix(r.RequestURI, path) {
			servlet(w, r)
			return
		}
	}
	t.StatusPage(w, r)
}

type StatusInfo struct {
	FurnaceState       string
	CurrentTempC       string
	CurrentTempF       string
	MinActiveTempC     string
	MinActiveTempF     string
	MinIdleTempC       string
	MinIdleTempF       string
	OverrideState      string
	HouseOccupied      string
	People             []*Housemate
	History            []*ReadingData
	ReadingHistoryText string
	Farenheit          bool
	PeopleHistory      []*PeopleHistData
	ShowGraph          bool
	Override           bool
	Uptime             time.Duration
	RecentReadings     []*ReadingData
}

func (t *WebServer) GetStatusInfo(r *http.Request) *StatusInfo {
	template_data := new(StatusInfo)

	template_data.Uptime = time.Now().Round(time.Second).Sub(t.server_started)

	// Furnace State
	if t.decider.getLastFurnaceState() {
		template_data.FurnaceState = "On"
	} else {
		template_data.FurnaceState = "Off"
	}

	// Current temps
	cur_temp_c := t.decider.getLastTemperature()
	cur_temp_f := (cur_temp_c * 9.0 / 5.0) + 32.0
	template_data.CurrentTempC = strconv.FormatFloat(cur_temp_c, 'f', 2, 64)
	template_data.CurrentTempF = strconv.FormatFloat(cur_temp_f, 'f', 2, 64)

	// Min temps
	template_data.MinActiveTempC = strconv.FormatFloat(t.decider.getActiveTemp(), 'f', 2, 64)
	template_data.MinActiveTempF = strconv.FormatFloat((t.decider.getActiveTemp()*9.0/5.0)+32.0, 'f', 2, 64)
	template_data.MinIdleTempC = strconv.FormatFloat(t.decider.getIdleTemp(), 'f', 2, 64)
	template_data.MinIdleTempF = strconv.FormatFloat((t.decider.getIdleTemp()*9.0/5.0)+32.0, 'f', 2, 64)

	// Override state
	if t.decider.getOverride() {
		template_data.OverrideState = "On"
	} else {
		template_data.OverrideState = "Off"
	}

	// People home?
	if t.decider.anybodyHome() {
		template_data.HouseOccupied = "Yes"
	} else {
		template_data.HouseOccupied = "No"
	}

	template_data.People = t.dhcp_tailer.housemates
	for _, person := range template_data.People {
		person.SeenDuration = time.Now().Round(time.Second).Sub(person.Last_seen)
		if person.SeenDuration < time.Minute*10 {
			person.IsHome = "Yes"
		} else {
			person.IsHome = "No"
		}
	}

	if r.Form.Get("graph") == "on" {
		template_data.ShowGraph = true
		template_data.History = t.decider.getReadingHistoryForNode(255)
		template_data.PeopleHistory = t.decider.getPeopleHistory()
		if r.Form.Get("unit") == "f" {
			template_data.Farenheit = true
			for _, v := range template_data.History {
				if v.Temp.Valid {
					v.Temp.Float64 = v.Temp.Float64*1.8 + 32.0
				}
			}
		} else {
			template_data.Farenheit = false
		}
		err := generateTempPlot(
			t.decider,
			template_data.Farenheit,
			"/var/www/nest/graph_temp.png",
		)
		if err != nil {
			log.Println(err)
		}
		err = generatePressurePlot(
			t.decider,
			"/var/www/nest/graph_pressure.png",
		)
		if err != nil {
			log.Println(err)
		}
		err = generateHumidityPlot(
			t.decider,
			"/var/www/nest/graph_humidity.png",
		)
		if err != nil {
			log.Println(err)
		}
	} else {
		template_data.ShowGraph = false
	}

	template_data.Override = t.decider.getOverride()

	template_data.RecentReadings = t.decider.getRecentReadings()

	return template_data
}

func (t *WebServer) StatusPage(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	if r.Form.Get("override") == "on" {
		t.decider.setIntSetting(SETTING_OVERRIDE, time.Now().Unix())
		http.Redirect(w, r, "/", 301)
	} else if r.Form.Get(SETTING_OVERRIDE) == "off" {
		t.decider.setIntSetting("override", 0)
		http.Redirect(w, r, "/", 301)
	}

	template, err := template.ParseFiles(t.config.Templates.Status)
	if err != nil {
		log.Println(err)
		http.Error(w, "Template error", 500)
		return
	}

	template_data := t.GetStatusInfo(r)

	err = template.Execute(w, template_data)
	if err != nil {
		log.Println(err)
		http.Error(w, "Template error", 500)
		return
	}

}

func (t *WebServer) ControlPage(w http.ResponseWriter, r *http.Request) {
	t.last_update = time.Now()
	r.ParseForm()
	var current_temp sql.NullFloat64
	var current_pressure sql.NullFloat64
	var current_humidity sql.NullFloat64
	current_temp.Valid = true
	current_pressure.Valid = true
	current_humidity.Valid = true

	var err error

	node_id_s := r.Form.Get("node_id")
	node_id, err := strconv.ParseInt(node_id_s, 10, 64)
	if err != nil {
		log.Println(err)
		fmt.Fprintf(w, "burn-n")
		return
	}

	// Read the weather data from the form into nullable floats
	// If there was an error parsing the value, assume it was because it wasn't
	// present, and just mark the nullable as not valid.
	current_temp_s := r.Form.Get("temp")
	current_temp.Float64, err = strconv.ParseFloat(current_temp_s, 64)
	if err != nil {
		current_temp.Valid = false
	}
	current_pressure_s := r.Form.Get("pressure")
	current_pressure.Float64, err = strconv.ParseFloat(current_pressure_s, 64)
	if err != nil {
		current_pressure.Valid = false
	}
	current_humidity_s := r.Form.Get("humidity")
	current_humidity.Float64, err = strconv.ParseFloat(current_humidity_s, 64)
	if err != nil {
		current_humidity.Valid = false
	}

	// If none of the readings made sense, there's no point saving any of them.
	if !current_temp.Valid && !current_pressure.Valid && !current_humidity.Valid {
		log.Println("Got useless info from node", node_id)
		fmt.Fprintf(w, "burn-n")
		return
	}

	// Grab the primary node (the node we use to control the heater)
	primary_node, err := t.decider.getIntSetting(SETTING_PRIMARY_NODE)
	if err != nil {
		log.Println(err)
		fmt.Fprintf(w, "burn-i")
		return
	}

	// Log the data
	t.decider.LogReading(node_id, current_temp, current_pressure, current_humidity)

	// If this reading was from the primary, update the heater. Otherwise,
	// no change.
	if node_id == primary_node && current_temp.Valid {
		furnace_on := t.decider.ShouldFurnace(current_temp.Float64)
		err = t.decider.setBoolSetting(SETTING_FURNACE_ON, furnace_on)
		if err != nil {
			log.Println(err)
		}
		if furnace_on {
			fmt.Fprintf(w, "burn-y")
		} else {
			fmt.Fprintf(w, "burn-n")
		}
	} else {
		fmt.Fprintf(w, "burn-i")
	}
}
