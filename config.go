package main

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// Config holds all runtime settings.
//
// Load order: defaults → YAML file → environment variables.
// Env vars always win.
//
//	DYNSERVER_HOST         string   bind host for the mgmt API (default: all interfaces)
//	DYNSERVER_PORT         int      mgmt API port              (default: 8080)
//	DYNSERVER_STATE_FILE   string   persistence path           (default: state.json)
//	DYNSERVER_SHUTDOWN_TTL int      graceful shutdown seconds  (default: 10)
type Config struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	StateFile   string `yaml:"state_file"`
	ShutdownTTL int    `yaml:"shutdown_ttl"`
}

func defaultConfig() Config {
	return Config{
		Host:        "",
		Port:        8080,
		StateFile:   "state.json",
		ShutdownTTL: 10,
	}
}

func loadConfig(path string) (Config, error) {
	cfg := defaultConfig()

	if path != "" {
		f, err := os.Open(path)
		if err != nil {
			return cfg, fmt.Errorf("open config: %w", err)
		}
		defer f.Close()
		if err := yaml.NewDecoder(f).Decode(&cfg); err != nil {
			return cfg, fmt.Errorf("decode config: %w", err)
		}
	}

	if v := os.Getenv("DYNSERVER_HOST"); v != "" {
		cfg.Host = v
	}
	if v := os.Getenv("DYNSERVER_PORT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return cfg, fmt.Errorf("DYNSERVER_PORT must be an integer: %w", err)
		}
		cfg.Port = n
	}
	if v := os.Getenv("DYNSERVER_STATE_FILE"); v != "" {
		cfg.StateFile = v
	}
	if v := os.Getenv("DYNSERVER_SHUTDOWN_TTL"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return cfg, fmt.Errorf("DYNSERVER_SHUTDOWN_TTL must be an integer: %w", err)
		}
		cfg.ShutdownTTL = n
	}

	return cfg, nil
}
