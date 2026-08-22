package artifact_test

import (
	"context"
	"errors"
	"testing"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func artifactServices(t *testing.T) testsupport.Services {
	s, err := testsupport.Open(t.TempDir() + "/artifact.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Store.Close() })
	return s
}
func artifactFixture(t *testing.T) (testsupport.Services, string, domain.User) {
	s := artifactServices(t)
	ctx := context.Background()
	tenant, _ := s.Identity.CreateTenant(ctx, "tenant")
	u, _ := s.Identity.CreateUser(ctx, tenant, "admin@example.com", "Admin", "secret", domain.RoleTenantAdmin)
	return s, tenant, u
}

func TestArtifactUploadScanPromoteAndRetire(t *testing.T) {
	s, tenant, u := artifactFixture(t)
	ctx := context.Background()
	a, v, err := s.Artifact.Create(ctx, tenant, "vision-model", "sha256-12345678", 100, u.ID, "upload")
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != domain.ArtifactUploaded || v.Status != domain.ArtifactUploaded {
		t.Fatalf("a=%+v v=%+v", a, v)
	}
	if err := s.Artifact.Scan(ctx, tenant, v.ID, true, u.ID, "scan"); err != nil {
		t.Fatal(err)
	}
	if err := s.Artifact.Promote(ctx, tenant, u.ID, a.ID, v.ID, 1, "promote"); err != nil {
		t.Fatal(err)
	}
	var active, status string
	_ = s.Store.DB().QueryRow(`SELECT active_version_id,status FROM artifacts WHERE id=?`, a.ID).Scan(&active, &status)
	if active != v.ID || status != "promoted" {
		t.Fatalf("active=%s status=%s", active, status)
	}
	if err := s.Artifact.Retire(ctx, tenant, u.ID, v.ID, "retire"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("active retire err=%v", err)
	}
}

func TestArtifactFailedScanRetiresVersion(t *testing.T) {
	s, tenant, u := artifactFixture(t)
	ctx := context.Background()
	a, v, err := s.Artifact.Create(ctx, tenant, "model", "sha256-87654321", 20, u.ID, "upload")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Artifact.Scan(ctx, tenant, v.ID, false, u.ID, "scan"); err != nil {
		t.Fatal(err)
	}
	if err := s.Artifact.Promote(ctx, tenant, u.ID, a.ID, v.ID, 1, "promote"); !errors.Is(err, domain.ErrState) {
		t.Fatalf("promote failed scan err=%v", err)
	}
	if err := s.Artifact.Retire(ctx, tenant, u.ID, v.ID, "retire"); err != nil {
		t.Fatal(err)
	}
}

func TestArtifactDuplicateAndVersionCompetition(t *testing.T) {
	s, tenant, u := artifactFixture(t)
	ctx := context.Background()
	a, v, err := s.Artifact.Create(ctx, tenant, "model", "sha256-12345678", 20, u.ID, "upload")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Artifact.Create(ctx, tenant, "model", "sha256-87654321", 20, u.ID, "duplicate-name"); !errors.Is(err, domain.ErrAlreadyExists) {
		t.Fatalf("duplicate name=%v", err)
	}
	if err := s.Artifact.Scan(ctx, tenant, v.ID, true, u.ID, "scan"); err != nil {
		t.Fatal(err)
	}
	if err := s.Artifact.Promote(ctx, tenant, u.ID, a.ID, v.ID, 8, "wrong-version"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("version conflict=%v", err)
	}
}

func TestArtifactInputValidation(t *testing.T) {
	s, tenant, u := artifactFixture(t)
	ctx := context.Background()
	if _, _, err := s.Artifact.Create(ctx, tenant, "", "digest", 1, u.ID, "r"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("empty name=%v", err)
	}
	if _, _, err := s.Artifact.Create(ctx, tenant, "model", "short", 1, u.ID, "r"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("short digest=%v", err)
	}
	if _, _, err := s.Artifact.Create(ctx, tenant, "model", "sha256-12345678", -1, u.ID, "r"); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("negative size=%v", err)
	}
}
