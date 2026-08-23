package outbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/outbox"
	"github.com/VanceMichael/greengrid/internal/pagination"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func TestGreenGridTask0022(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/task.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "outbox-list")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.Store.DB().Exec(`INSERT INTO outbox_events(id,tenant_id,kind,aggregate_id,payload,status,attempts,lease_until,next_attempt_at,created_at) VALUES('sent',?,?,?,?, 'sent',0,'',?,?),('failed',?,?,?,?, 'failed',3,'',?,?)`, tenantID, "test", "a", "{}", now, now, tenantID, "test", "b", "{}", now, now); err != nil {
		t.Fatal(err)
	}
	page, err := outbox.NewQuery(s.Store).List(ctx, tenantID, "failed", pagination.Page{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Meta.Total != 1 || len(page.Items) != 1 || page.Items[0].Status != "failed" {
		t.Fatalf("outbox page=%+v", page)
	}
	_ = domain.ErrNotFound
}
