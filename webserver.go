package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
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
}

func NewWebServer(c *Config, dhcp *DhcpStatus, decider *Decider) *WebServer {
	t := new(WebServer)
	t.decider = decider
	t.dhcp_tailer = dhcp
	t.config = c
	t.server_started = time.Now().Round(time.Second)
	t.servlets = make(map[string]func(http.ResponseWriter, *http.Request))
	t.servlets["/nest.php"] = t.ControlPage
	return t
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
	FurnaceState   string
	CurrentTempC   string
	CurrentTempF   string
	MinActiveTempC string
	MinActiveTempF string
	MinIdleTempC   string
	MinIdleTempF   string
	OverrideState  string
	HouseOccupied  string
	People         []*Housemate
	History        []*HistData
	Farenheit      bool
	PeopleHistory  []*PeopleHistData
	ShowGraph      bool
	Override       bool
	Uptime         time.Duration
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
		template_data.History = t.decider.getHistory()
		template_data.PeopleHistory = t.decider.getPeopleHistory()
	} else {
		template_data.ShowGraph = false
	}

	if r.Form.Get("unit") == "f" {
		template_data.Farenheit = true
		for _, v := range template_data.History {
			v.Temp = v.Temp*1.8 + 32.0
		}
	} else {
		template_data.Farenheit = false
	}

	template_data.Override = t.decider.getOverride()

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
	r.ParseForm()
	current_temp_s := r.Form.Get("temp")
	current_temp, err := strconv.ParseFloat(current_temp_s, 64)
	if err != nil {
		log.Println(err)
		fmt.Fprintf(w, "burn-n")
		return
	}

	current_pressure_s := r.Form.Get("pressure")
	current_pressure, err := strconv.ParseFloat(current_pressure_s, 64)
	if err != nil {
		log.Println(err)
		fmt.Fprintf(w, "burn-n")
		return
	}

	furnace_on := t.decider.ShouldFurnace(current_temp)

	t.decider.LogStats(current_temp, current_pressure, furnace_on)

	if furnace_on {
		fmt.Fprintf(w, "burn-y")
	} else {
		fmt.Fprintf(w, "burn-n")
	}
}
