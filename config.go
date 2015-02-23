package main

import (
	"code.google.com/p/gcfg"
	"log"
	"strings"
)

var build_version string

type Config struct {
	Mysql struct {
		MysqlUser       string
		MysqlPassword   string
		MysqlIPProto    string
		MysqlServerAddr string
		MysqlServerPort string
		MysqlDatabase   string
	}

	Network struct {
		BindAddress string
		BindPort    string
	}

	Templates struct {
		Status string
	}

	Mail struct {
		Host   string
		Target string
	}
}

func (kc Config) GetSqlURI() string {
	mysql_auth_strings := []string{kc.Mysql.MysqlUser,
		":",
		kc.Mysql.MysqlPassword,
		"@",
		kc.Mysql.MysqlIPProto,
		"(",
		kc.Mysql.MysqlServerAddr,
		":",
		kc.Mysql.MysqlServerPort,
		")/",
		kc.Mysql.MysqlDatabase,
		"?parseTime=true",
	}
	return strings.Join(mysql_auth_strings, "")
}

func LoadConfiguration(config_path string) *Config {
	kc := new(Config)
	err := gcfg.ReadFileInto(kc, config_path)
	if err != nil {
		log.Fatal("Failed to parse gcfg data: ", err)
	}
	return kc
}
