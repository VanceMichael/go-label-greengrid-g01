package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
)

func TestGreenGridTask0029(t *testing.T) {
	s, err := sqlite.Open(t.TempDir() + "/migration.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.Migrate(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled migration error=%v", err)
	}
	var tables int
	if err := s.DB().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='tenants'`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if tables != 0 {
		t.Fatalf("schema persisted after cancelled migration")
	}
}
