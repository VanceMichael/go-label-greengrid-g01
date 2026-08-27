package artifact

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
	"github.com/google/uuid"
)

type Service struct{ store *sqlite.Store }

func NewService(store *sqlite.Store) *Service { return &Service{store: store} }

func (s *Service) Create(ctx context.Context, tenantID, name, digest string, size int64, actorID, requestID string) (domain.Artifact, domain.ArtifactVersion, error) {
	if tenantID == "" || strings.TrimSpace(name) == "" || len(digest) < 8 || size < 0 {
		return domain.Artifact{}, domain.ArtifactVersion{}, fmt.Errorf("%w: artifact fields", domain.ErrInvalid)
	}
	a := domain.Artifact{ID: uuid.NewString(), TenantID: tenantID, Name: name, Status: domain.ArtifactUploaded, Version: 1}
	v := domain.ArtifactVersion{ID: uuid.NewString(), ArtifactID: a.ID, Digest: digest, Status: domain.ArtifactUploaded, SizeBytes: size, Version: 1, CreatedAt: time.Now().UTC()}
	err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `INSERT INTO artifacts(id,tenant_id,name,active_version_id,status,version,created_at) VALUES(?,?,?,?,?,?,?)`, a.ID, tenantID, name, nil, a.Status, 1, now); err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				return domain.ErrAlreadyExists
			}
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO artifact_versions(id,artifact_id,digest,status,size_bytes,version,created_at) VALUES(?,?,?,?,?,?,?)`, v.ID, a.ID, digest, v.Status, size, 1, now); err != nil {
			return err
		}
		return audit(tx, tenantID, actorID, "artifact", a.ID, "upload", requestID, "uploaded")
	})
	return a, v, err
}

func (s *Service) Scan(ctx context.Context, tenantID, versionID string, passed bool, actorID, requestID string) error {
	return s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var status string
		var artifactID string
		if err := tx.QueryRowContext(ctx, `SELECT v.status,v.artifact_id FROM artifact_versions v JOIN artifacts a ON a.id=v.artifact_id WHERE v.id=? AND a.tenant_id=?`, versionID, tenantID).Scan(&status, &artifactID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if status != string(domain.ArtifactUploaded) {
			return domain.ErrState
		}
		next := domain.ArtifactScanned
		if !passed {
			next = domain.ArtifactRetired
		}
		if _, err := tx.ExecContext(ctx, `UPDATE artifact_versions SET status=?,version=version+1 WHERE id=? AND status='uploaded'`, next, versionID); err != nil {
			return err
		}
		return audit(tx, tenantID, actorID, "artifact_version", versionID, "scan", requestID, string(next))
	})
}

func (s *Service) Promote(ctx context.Context, tenantID, actorID, artifactID, versionID string, expectedArtifactVersion int64, requestID string) error {
	return s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var current, status, old sql.NullString
		var version int64
		if err := tx.QueryRowContext(ctx, `SELECT a.active_version_id,a.status,v.status,a.version FROM artifacts a JOIN artifact_versions v ON v.id=? WHERE a.id=? AND a.tenant_id=?`, versionID, artifactID, tenantID).Scan(&current, &status, &old, &version); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if old.String != string(domain.ArtifactScanned) {
			return domain.ErrState
		}
		if version != expectedArtifactVersion {
			return domain.ErrConflict
		}
		result, err := tx.ExecContext(ctx, `UPDATE artifacts SET active_version_id=?,status='promoted',version=version+1 WHERE id=? AND tenant_id=? AND version=? AND (active_version_id IS NULL OR active_version_id<>?)`, versionID, artifactID, tenantID, expectedArtifactVersion, versionID)
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return domain.ErrConflict
		}
		if _, err := tx.ExecContext(ctx, `UPDATE artifact_versions SET status='promoted',version=version+1 WHERE id=? AND status='scanned'`, versionID); err != nil {
			return err
		}
		return audit(tx, tenantID, actorID, "artifact", artifactID, "promote", requestID, "promoted")
	})
}

func (s *Service) Retire(ctx context.Context, tenantID, actorID, versionID, requestID string) error {
	return s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var status, artifactID string
		if err := tx.QueryRowContext(ctx, `SELECT v.status,v.artifact_id FROM artifact_versions v JOIN artifacts a ON a.id=v.artifact_id WHERE v.id=? AND a.tenant_id=?`, versionID, tenantID).Scan(&status, &artifactID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if status == string(domain.ArtifactPromoted) {
			var active string
			if err := tx.QueryRowContext(ctx, `SELECT active_version_id FROM artifacts WHERE id=?`, artifactID).Scan(&active); err != nil {
				return err
			}
			if active == versionID {
				return domain.ErrConflict
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE artifact_versions SET status='retired',version=version+1 WHERE id=? AND status IN ('uploaded','scanned','promoted')`, versionID); err != nil {
			return err
		}
		return audit(tx, tenantID, actorID, "artifact_version", versionID, "retire", requestID, "retired")
	})
}

func audit(tx *sql.Tx, tenantID, actorID, typ, id, action, requestID, details string) error {
	_, err := tx.Exec(`INSERT INTO audit_events(id,tenant_id,actor_id,aggregate_type,aggregate_id,action,result,request_id,details,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, uuid.NewString(), tenantID, actorID, typ, id, action, "success", requestID, details, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
