package config_test

import (
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/config"
)

func TestConfigDefaultsAndOverrides(t *testing.T) {
	values := map[string]string{"GREENGRID_ADDR": "127.0.0.1:9090", "GREENGRID_DB": "/tmp/green.db", "GREENGRID_SESSION_TTL": "2h", "GREENGRID_WORKER_INTERVAL": "20ms"}
	cfg, err := config.Load(func(key string) string { return values[key] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Address != "127.0.0.1:9090" || cfg.DatabasePath != "/tmp/green.db" || cfg.SessionTTL != 2*time.Hour || cfg.WorkerInterval != 20*time.Millisecond {
		t.Fatalf("cfg=%+v", cfg)
	}
}
func TestConfigRejectsBadDurations(t *testing.T) {
	for _, raw := range []string{"0s", "not-duration"} {
		_, err := config.Load(func(key string) string {
			if key == "GREENGRID_SESSION_TTL" {
				return raw
			}
			return ""
		})
		if err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	cfg, err := config.Load(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Address == "" {
		t.Fatal("default address missing")
	}
}
