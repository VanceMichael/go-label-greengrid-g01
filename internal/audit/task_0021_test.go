package audit_test

import (
	"context"
	"testing"

	"github.com/VanceMichael/greengrid/internal/audit"
	"github.com/VanceMichael/greengrid/internal/pagination"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func TestGreenGridTask0021(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/task.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "audit-list")
	auditService := audit.NewService(s.Store)
	if err := auditService.Append(ctx, tenantID, "actor", "job", "job-a", "finish", "success", "r1", "a"); err != nil {
		t.Fatal(err)
	}
	if err := auditService.Append(ctx, tenantID, "actor", "reservation", "reservation-a", "approve", "success", "r2", "b"); err != nil {
		t.Fatal(err)
	}
	page, err := auditService.List(ctx, tenantID, "job", "", pagination.Page{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Meta.Total != 1 || len(page.Items) != 1 || page.Items[0].AggregateType != "job" {
		t.Fatalf("audit page=%+v", page)
	}
}
