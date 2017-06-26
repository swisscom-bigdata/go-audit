package main

import (
	"io/ioutil"
	"log/syslog"

	yaml "gopkg.in/yaml.v2"
)

// Config defines configuration of go-audit.
type Config struct {
	SockerBuffer struct {
		Receive int `yaml:"receive"`
	} `yaml:"socker_buffer"`

	Events struct {
		Min int `yaml:"min"`
		Max int `yaml:"max"`
	} `yaml:"events"`

	MessageTracking struct {
		Enabled       bool `yaml:"enabled"`
		LogOutOfOrder bool `yaml:"log_out_of_order"`
		MaxOutOfOrder int  `yaml:"max_out_of_order"`
	} `yaml:"message_tracking"`

	Output struct {
		Stdout struct {
			Enabled  bool `yaml:"enabled"`
			Attempts int  `yaml:"attempts"`
		} `yaml:"stdout"`

		Syslog struct {
			Enabled  bool   `yaml:"enabled"`
			Attempts int    `yaml:"attempts"`
			Network  string `yaml:"network"`
			Address  string `yaml:"address"`
			Priority int    `yaml:"priority"`
			Tag      string `yaml:"tag"`
		} `yaml:"syslog"`

		File struct {
			Enabled  bool   `yaml:"enabled"`
			Attempts int    `yaml:"attempts"`
			Path     string `yaml:"path"`
			Mode     int    `yaml:"mode"`
			User     string `yaml:"user"`
			Group    string `yaml:"group"`
		} `yaml:"file"`

		Kafka KafkaConfig `yaml:"kafka"`
	} `yaml:"output"`

	Log struct {
		Level string `yaml:"level"`
	} `yaml:"log"`

	Rules []string `yaml:"rules"`

	Filters []Filter `yaml:"filters"`
}

// Filter specifies syscalls to ignore.
type Filter struct {
	Syscall     int    `yaml:"syscall"`
	MessageType int    `yaml:"message_type"`
	Regex       string `yaml:"regex"`
}

func loadConfig(filename string) (*Config, error) {
	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	config := defaultConfig()
	if err := yaml.Unmarshal(buf, config); err != nil {
		return nil, err
	}
	return config, nil
}

func defaultConfig() *Config {
	config := new(Config)
	config.Events.Min = 1300
	config.Events.Max = 1399
	config.MessageTracking.Enabled = true
	config.MessageTracking.LogOutOfOrder = false
	config.MessageTracking.MaxOutOfOrder = 500
	config.Output.Syslog.Enabled = false
	config.Output.Syslog.Attempts = 3
	config.Output.Syslog.Priority = int(syslog.LOG_LOCAL0 | syslog.LOG_WARNING)
	config.Output.Syslog.Tag = "go-audit"
	config.Log.Level = "warn"
	return config
}
