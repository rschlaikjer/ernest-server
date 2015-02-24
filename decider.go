package main

import (
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"log"
	"math/rand"
	"time"
)

const SETTING_IDLE_TEMP = "idle_temp"
const SETTING_ACTIVE_TEMP = "min_temp"
const SETTING_OVERRIDE = "override"
const SETTING_FURNACE_ON = "furnace_on"
const SETTING_PRIMARY_NODE = "primary_node"

type Decider struct {
	db          *sql.DB
	dhcp_tailer *DhcpStatus
}

func NewDecider(c *Config, d *DhcpStatus) *Decider {
	t := new(Decider)

	db, err := sql.Open("mysql", c.GetSqlURI())
	if err != nil {
		log.Println(err)
	}
	t.db = db

	t.dhcp_tailer = d

	return t
}

func (d *Decider) getFloatSetting(name string) (float64, error) {
	row := d.db.QueryRow("SELECT `value` FROM `settings` WHERE `key` = ?", name)
	var fv float64
	err := row.Scan(&fv)
	return fv, err
}

func (d *Decider) setFloatSetting(name string, value float64) error {
	_, err := d.db.Exec(
		"INSERT INTO settings (`key`, `value`) VALUES (?, ?) ON DUPLICATE KEY UPDATE `value` = ?",
		name, value, value,
	)
	return err
}

func (d *Decider) getBoolSetting(name string) (bool, error) {
	row := d.db.QueryRow("SELECT `value` FROM `settings` WHERE `key` = ?", name)
	var bv bool
	err := row.Scan(&bv)
	return bv, err
}

func (d *Decider) setBoolSetting(name string, value bool) error {
	_, err := d.db.Exec(
		"INSERT INTO settings (`key`, `value`) VALUES (?, ?) ON DUPLICATE KEY UPDATE `value` = ?",
		name, value, value,
	)
	return err
}

func (d *Decider) getIntSetting(name string) (int64, error) {
	row := d.db.QueryRow("SELECT `value` FROM `settings` WHERE `key` = ?", name)
	var bv int64
	err := row.Scan(&bv)
	return bv, err
}

func (d *Decider) setIntSetting(name string, value int64) error {
	_, err := d.db.Exec(
		"INSERT INTO settings (`key`, `value`) VALUES (?, ?) ON DUPLICATE KEY UPDATE `value` = ?",
		name, value, value,
	)
	return err
}

func (d *Decider) getIdleTemp() float64 {
	// Grab the temperature to keep the house at when unoccupied
	temp, err := d.getFloatSetting(SETTING_IDLE_TEMP)
	if err != nil {
		log.Println(err)
		return 12.50
	}
	return temp
}

func (d *Decider) getActiveTemp() float64 {
	// Get the temperature to keep the house at when occupied
	temp, err := d.getFloatSetting(SETTING_ACTIVE_TEMP)
	if err != nil {
		log.Println(err)
		return 16
	}
	return temp
}

func (d *Decider) getOverride() bool {
	// Return whether the furnace override is on
	override, err := d.getIntSetting(SETTING_OVERRIDE)
	if err != nil {
		log.Println(err)
		return false
	}
	override_started := time.Unix(override, 0)
	override_until := override_started.Add(time.Minute * 20)
	if override_until.Before(time.Now()) {
		return false
	} else {
		return true
	}
}

func (d *Decider) anybodyHome() bool {
	last_seen := d.dhcp_tailer.LastPersonActive()
	if last_seen == nil {
		return false
	}
	return last_seen.isHome()
}

func (d *Decider) getLastFurnaceState() bool {
	// Return the state the furnace was in last time.
	// True = on, false = off
	state, err := d.getBoolSetting(SETTING_FURNACE_ON)
	if err != nil {
		return false
	}
	return state
}

func (d *Decider) getLastTemperature() float64 {
	row := d.db.QueryRow(`
		SELECT readings.temp FROM readings, settings
		WHERE readings.node_id = settings.value
		AND settings.key = "primary_node"
		ORDER BY timestamp DESC LIMIT 1
	`)
	var temp float64
	row.Scan(&temp)
	return temp
}

type NodePlotOpts struct {
	Name    string
	Graph_r uint8
	Graph_g uint8
	Graph_b uint8
}

func (d *Decider) getNodePlotOpts(node_id int64) *NodePlotOpts {
	opts := new(NodePlotOpts)

	row := d.db.QueryRow("SELECT name,graph_r,graph_g,graph_b from node_names WHERE node_id = ?", node_id)
	err := row.Scan(
		&opts.Name,
		&opts.Graph_r,
		&opts.Graph_g,
		&opts.Graph_b,
	)

	if err != nil {
		opts.Name = fmt.Sprintf("Node %d", node_id)
		opts.Graph_r = uint8(rand.Int31() % 255)
		opts.Graph_g = uint8(rand.Int31() % 255)
		opts.Graph_b = uint8(rand.Int31() % 255)
	}

	return opts
}

type ReadingHistory map[int64][]*ReadingData

type ReadingData struct {
	Time      time.Time
	Staleness time.Duration
	Node      int64
	Name      string
	Temp      sql.NullFloat64
	Pressure  sql.NullFloat64
	Humidity  sql.NullFloat64
}

type PeopleHistData struct {
	Time  int64
	Count int64
}

func (d *Decider) getRecentReadings() []*ReadingData {
	rows, err := d.db.Query(`
	SELECT a.timestamp, a.node_id, a.temp, a.pressure, a.humidity
	FROM readings a INNER JOIN (SELECT node_id, max(id) AS maxid
	FROM readings group by node_id) AS b
	ON a.id = b.maxid
	WHERE a.timestamp > DATE_SUB(CURRENT_TIMESTAMP, INTERVAL 5 MINUTE)
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	r := make([]*ReadingData, 0)

	for rows.Next() {
		reading := new(ReadingData)
		if err := rows.Scan(
			&reading.Time,
			&reading.Node,
			&reading.Temp,
			&reading.Pressure,
			&reading.Humidity,
		); err != nil {
			continue
		}
		node_info := d.getNodePlotOpts(reading.Node)
		reading.Name = node_info.Name
		reading.Staleness = time.Now().Round(time.Second).Sub(
			reading.Time.Round(time.Second),
		) - (time.Hour * 5)
		r = append(r, reading)
	}
	return r
}

func (d *Decider) getReadingHistory() ReadingHistory {
	// Get all the node IDs that have reported data in the past week
	node_id_rows, err := d.db.Query(`SELECT  node_id
		FROM  readings
		WHERE  timestamp > DATE_SUB( CURRENT_TIMESTAMP( ) , INTERVAL 1 WEEK )
		GROUP BY  node_id `)
	if err != nil {
		log.Println(err)
		return nil
	}
	defer node_id_rows.Close()
	history := make(ReadingHistory)
	for node_id_rows.Next() {
		var node_id int64
		if err := node_id_rows.Scan(&node_id); err != nil {
			log.Println(err)
			continue
		}
		history[node_id] = d.getReadingHistoryForNode(node_id)
	}

	return history
}

func (d *Decider) getReadingHistoryForNode(node_id int64) []*ReadingData {
	rows, err := d.db.Query(`
		SELECT timestamp, temp, pressure, humidity FROM nest.readings WHERE
		timestamp > DATE_SUB(CURRENT_TIMESTAMP(), INTERVAL 1 WEEK)
		AND id % 5 = 0
		AND node_id = ?
		ORDER BY timestamp ASC
	`, node_id)
	if err != nil {
		log.Println(err)
		return nil
	}
	defer rows.Close()

	history := make([]*ReadingData, 0)
	for rows.Next() {
		h := new(ReadingData)
		if err := rows.Scan(
			&h.Time,
			&h.Temp,
			&h.Pressure,
			&h.Humidity,
		); err != nil {
			continue
		}
		h.Node = node_id
		history = append(history, h)
	}
	return history
}

func (d *Decider) getPeopleHistory() []*PeopleHistData {
	rows, err := d.db.Query(`SELECT  timestamp , SUM( is_home ) AS  'count'
		FROM  people_history
		WHERE timestamp > DATE_SUB(CURRENT_TIMESTAMP(), INTERVAL 1 WEEK)
		GROUP BY  timestamp`)
	if err != nil {
		log.Println(err)
		return nil
	}
	defer rows.Close()

	history := make([]*PeopleHistData, 0)
	for rows.Next() {
		h := new(PeopleHistData)
		var timestamp time.Time
		if err := rows.Scan(
			&timestamp,
			&h.Count,
		); err != nil {
			continue
		}
		h.Time = timestamp.Unix()
		history = append(history, h)
	}
	return history

}

func (d *Decider) LogReading(node_id int64, current_temp, current_pressure, current_humidity sql.NullFloat64) {
	_, err := d.db.Exec(`INSERT INTO  nest.readings
		(id, timestamp, node_id, temp, pressure, humidity)
		VALUES
		(NULL, CURRENT_TIMESTAMP, ?, ?, ?, ?)`,
		node_id, current_temp, current_pressure, current_humidity)
	if err != nil {
		log.Println(err)
	}

}

func (d *Decider) LogPeople() {
	for _, housemate := range d.dhcp_tailer.housemates {
		_, err := d.db.Exec(`INSERT INTO  nest.people_history
				(timestamp, person, is_home)
				VALUES
				(CURRENT_TIMESTAMP, ?, ?)`,
			housemate.Id, housemate.isHome(),
		)
		if err != nil {
			log.Println(err)
		}
	}
}

func (d *Decider) ShouldFurnace(current_temp float64) bool {
	// If the temp is lower than the idle temp, always turn up the heat
	if current_temp < d.getIdleTemp() {
		return true
	}

	// If people are home and the temp is below the active temp, turn on the heat
	if d.anybodyHome() && current_temp < d.getActiveTemp() {
		return true
	}

	// If the override is on, turn the furnace on no matter what
	if d.getOverride() {
		return true
	}

	// Sticky furnace on - don't toggle too frequently
	furnace_already_on := d.getLastFurnaceState()
	if furnace_already_on {
		if d.anybodyHome() {
			if current_temp < d.getActiveTemp()*1.05 {
				return true
			}
		} else {
			if current_temp < d.getIdleTemp()*1.05 {
				return true
			}
		}
	}

	return false
}
