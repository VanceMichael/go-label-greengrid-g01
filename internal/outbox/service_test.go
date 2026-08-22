package outbox_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func outboxServices(t *testing.T) testsupport.Services {
	s, err := testsupport.Open(t.TempDir() + "/outbox.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Store.Close() })
	return s
}
func insertOutbox(t *testing.T, s testsupport.Services) string {
	t.Helper()
	ctx := context.Background()
	tenant, _ := s.Identity.CreateTenant(ctx, "tenant")
	id := "event-1"
	_, err := s.Store.DB().Exec(`INSERT INTO outbox_events(id,tenant_id,kind,aggregate_id,payload,status,attempts,next_attempt_at,created_at) VALUES(?,?,?,?,?,'pending',0,?,?)`, id, tenant, "test", "aggregate", "{}", time.Now().UTC().Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestOutboxClaimAndSendFinish(t *testing.T) {
	s := outboxServices(t)
	ctx := context.Background()
	id := insertOutbox(t, s)
	event, err := s.Outbox.Claim(ctx, "worker", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if event.ID != id || event.Status != "sending" {
		t.Fatalf("event=%+v", event)
	}
	if err := s.Outbox.Finish(ctx, "worker", id, nil); err != nil {
		t.Fatal(err)
	}
	count, _ := s.Outbox.Count(ctx, "sent")
	if count != 1 {
		t.Fatalf("sent=%d", count)
	}
	if _, err := s.Outbox.Claim(ctx, "worker", time.Now().UTC()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("claimed sent=%v", err)
	}
}

func TestOutboxRetriesThenFailsAtLimit(t *testing.T) {
	s := outboxServices(t)
	ctx := context.Background()
	id := insertOutbox(t, s)
	for i := 1; i <= 3; i++ {
		_, err := s.Outbox.Claim(ctx, "worker", time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Outbox.Finish(ctx, "worker", id, fmt.Errorf("remote %d", i)); err != nil {
			t.Fatal(err)
		}
		if i < 3 {
			time.Sleep(time.Duration(i+1) * 1100 * time.Millisecond)
		}
	}
	failed, _ := s.Outbox.Count(ctx, "failed")
	if failed != 1 {
		t.Fatalf("failed=%d", failed)
	}
}

func TestOutboxWrongOwnerAndExpiredRecovery(t *testing.T) {
	s := outboxServices(t)
	ctx := context.Background()
	id := insertOutbox(t, s)
	_, _ = s.Outbox.Claim(ctx, "owner-a", time.Now().UTC())
	if err := s.Outbox.Finish(ctx, "owner-b", id, nil); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("wrong owner=%v", err)
	}
	n, err := s.Outbox.RecoverExpired(ctx, time.Now().UTC().Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("recovered=%d", n)
	}
	event, err := s.Outbox.Claim(ctx, "owner-b", time.Now().UTC().Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if event.ID != id {
		t.Fatal(event)
	}
}
