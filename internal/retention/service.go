package retention

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
	"github.com/google/uuid"
	"time"
)

type Service struct{ store *sqlite.Store }

func NewService(store *sqlite.Store) *Service { return &Service{store: store} }
func (s *Service) RetireVersion(ctx context.Context, tenantID, actorID, versionID, requestID string) error {
	return s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var status, artifactID string
		if err := tx.QueryRowContext(ctx, `SELECT v.status,v.artifact_id FROM artifact_versions v JOIN artifacts a ON a.id=v.artifact_id WHERE v.id=? AND a.tenant_id=?`, versionID, tenantID).Scan(&status, &artifactID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if status == string(domain.ArtifactRetired) {
			return nil
		}
		var active string
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(active_version_id,'') FROM artifacts WHERE id=?`, artifactID).Scan(&active); err != nil {
			return err
		}
		if active == versionID {
			return fmt.Errorf("%w: active artifact version", domain.ErrConflict)
		}
		var jobs int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE tenant_id=? AND artifact_version_id=? AND status IN ('queued','claimed','running')`, tenantID, versionID).Scan(&jobs); err != nil {
			return err
		}
		if jobs > 0 {
			return fmt.Errorf("%w: version referenced by jobs", domain.ErrConflict)
		}
		result, err := tx.ExecContext(ctx, `UPDATE artifact_versions SET status='retired',version=version+1 WHERE id=? AND status<>?`, versionID, domain.ArtifactRetired)
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return domain.ErrConflict
		}
		_, err = tx.Exec(`INSERT INTO audit_events(id,tenant_id,actor_id,aggregate_type,aggregate_id,action,result,request_id,details,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, uuid.NewString(), tenantID, actorID, "artifact_version", versionID, "retention_retire", "success", requestID, "retired after reference check", time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
}
func (s *Service) ActiveReferences(ctx context.Context, tenantID, versionID string) (int, error) {
	var count int
	err := s.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE tenant_id=? AND artifact_version_id=? AND status IN ('queued','claimed','running')`, tenantID, versionID).Scan(&count)
	return count, err
}
func (s *Service) Eligible(ctx context.Context, tenantID, versionID string) (bool, error) {
	count, err := s.ActiveReferences(ctx, tenantID, versionID)
	if err != nil {
		return false, err
	}
	var active string
	err = s.store.DB().QueryRowContext(ctx, `SELECT COALESCE(a.active_version_id,'') FROM artifact_versions v JOIN artifacts a ON a.id=v.artifact_id WHERE v.id=? AND a.tenant_id=?`, versionID, tenantID).Scan(&active)
	if errors.Is(err, sql.ErrNoRows) {
		return false, domain.ErrNotFound
	}
	return count == 0 && active != versionID, err
}
