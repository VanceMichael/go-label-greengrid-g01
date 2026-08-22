package config

import (
	"fmt"
	"strconv"
	"time"
)

type Config struct {
	Address         string
	DatabasePath    string
	ShutdownTimeout time.Duration
	SessionTTL      time.Duration
	WorkerInterval  time.Duration
}

type Lookup func(string) string

func Load(get Lookup) (Config, error) {
	cfg := Config{
		Address:         value(get, "GREENGRID_ADDR", ":8080"),
		DatabasePath:    value(get, "GREENGRID_DB", "./greengrid.db"),
		ShutdownTimeout: 10 * time.Second,
		SessionTTL:      8 * time.Hour,
		WorkerInterval:  100 * time.Millisecond,
	}
	for key, target := range map[string]*time.Duration{
		"GREENGRID_SHUTDOWN_TIMEOUT": &cfg.ShutdownTimeout,
		"GREENGRID_SESSION_TTL":      &cfg.SessionTTL,
		"GREENGRID_WORKER_INTERVAL":  &cfg.WorkerInterval,
	} {
		if raw := get(key); raw != "" {
			d, err := time.ParseDuration(raw)
			if err != nil || d <= 0 {
				return Config{}, fmt.Errorf("%s: invalid duration %q", key, raw)
			}
			*target = d
		}
	}
	if cfg.Address == "" || cfg.DatabasePath == "" {
		return Config{}, fmt.Errorf("address and database path are required")
	}
	return cfg, nil
}

func value(get Lookup, key, fallback string) string {
	if v := get(key); v != "" {
		return v
	}
	return fallback
}

func ParseBool(raw string, fallback bool) bool {
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseBool(raw)
	return err == nil && v
}
