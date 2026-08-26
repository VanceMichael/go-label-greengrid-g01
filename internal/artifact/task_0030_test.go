package artifact_test

import (
	"context"
	"testing"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func TestGreenGridTask0030(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/task.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "artifact-promotion")
	ops, _ := s.Identity.CreateUser(ctx, tenantID, "ops@example.com", "Ops", "secret", domain.RoleClusterOps)
	artifact, version, err := s.Artifact.Create(ctx, tenantID, "model", "digest-0030", 128, ops.ID, "create")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Artifact.Scan(ctx, tenantID, version.ID, true, ops.ID, "scan"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Store.DB().Exec(`CREATE TRIGGER fail_artifact_promotion BEFORE UPDATE ON artifact_versions WHEN NEW.status='promoted' BEGIN SELECT RAISE(ABORT,'promotion unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.Artifact.Promote(ctx, tenantID, ops.ID, artifact.ID, version.ID, 1, "promote"); err == nil {
		t.Fatal("promotion unexpectedly succeeded")
	}
	var active, artifactStatus, versionStatus string
	if err := s.Store.DB().QueryRow(`SELECT COALESCE(active_version_id,''),status FROM artifacts WHERE id=?`, artifact.ID).Scan(&active, &artifactStatus); err != nil {
		t.Fatal(err)
	}
	if err := s.Store.DB().QueryRow(`SELECT status FROM artifact_versions WHERE id=?`, version.ID).Scan(&versionStatus); err != nil {
		t.Fatal(err)
	}
	if active != "" || artifactStatus != "uploaded" || versionStatus != "scanned" {
		t.Fatalf("partial promotion active=%q artifact_status=%s version_status=%s", active, artifactStatus, versionStatus)
	}
}
