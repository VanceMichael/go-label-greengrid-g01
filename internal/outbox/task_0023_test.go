package outbox_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func TestGreenGridTask0023(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/task.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "outbox-cancel")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.Store.DB().Exec(`INSERT INTO outbox_events(id,tenant_id,kind,aggregate_id,payload,status,attempts,next_attempt_at,created_at) VALUES('event',?,?,?,?, 'pending',0,?,?)`, tenantID, "test", "aggregate", "{}", now, now); err != nil {
		t.Fatal(err)
	}
	event, err := s.Outbox.Claim(ctx, "worker", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := s.Outbox.Finish(cancelled, "worker", event.ID, errors.New("remote result unknown")); err != nil {
		t.Fatal(err)
	}
	var status, owner string
	if err := s.Store.DB().QueryRow(`SELECT status,COALESCE(lease_owner,'') FROM outbox_events WHERE id='event'`).Scan(&status, &owner); err != nil {
		t.Fatal(err)
	}
	if status != "retry" || owner != "" {
		t.Fatalf("outbox status=%s owner=%s", status, owner)
	}
}
