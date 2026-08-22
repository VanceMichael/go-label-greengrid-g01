package app_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/app"
	"github.com/VanceMichael/greengrid/internal/config"
)

func TestNewRuntimeMigratesDatabaseAndClosesResources(t *testing.T) {
	cfg := config.Config{Address: "127.0.0.1:0", DatabasePath: t.TempDir() + "/runtime.db", ShutdownTimeout: time.Second, SessionTTL: time.Hour, WorkerInterval: time.Millisecond}
	r, err := app.New(cfg, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
}
func TestNewRuntimeRejectsUnusableDatabasePath(t *testing.T) {
	cfg := config.Config{Address: "127.0.0.1:0", DatabasePath: t.TempDir() + "/missing/dir/db", ShutdownTimeout: time.Second, SessionTTL: time.Hour, WorkerInterval: time.Millisecond}
	if _, err := app.New(cfg, slog.Default()); err == nil {
		t.Fatal("invalid database path accepted")
	}
	_ = context.Background
}
