package artifact

import (
	"context"
	"database/sql"
	"errors"
	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/pagination"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
	"time"
)

type VersionQuery struct{ store *sqlite.Store }

func NewVersionQuery(store *sqlite.Store) *VersionQuery { return &VersionQuery{store: store} }
func (q *VersionQuery) List(ctx context.Context, tenantID, artifactID string, page pagination.Page) (pagination.Result[domain.ArtifactVersion], error) {
	if err := page.Validate(); err != nil {
		return pagination.Result[domain.ArtifactVersion]{}, err
	}
	var total int
	if err := q.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM artifact_versions v JOIN artifacts a ON a.id=v.artifact_id WHERE a.tenant_id=? AND a.id=?`, tenantID, artifactID).Scan(&total); err != nil {
		return pagination.Result[domain.ArtifactVersion]{}, err
	}
	rows, err := q.store.DB().QueryContext(ctx, `SELECT v.id,v.artifact_id,v.digest,v.status,v.size_bytes,v.version,v.created_at FROM artifact_versions v JOIN artifacts a ON a.id=v.artifact_id WHERE a.tenant_id=? AND a.id=? ORDER BY v.created_at DESC,v.id DESC LIMIT ? OFFSET ?`, tenantID, artifactID, page.Limit, page.Offset)
	if err != nil {
		return pagination.Result[domain.ArtifactVersion]{}, err
	}
	defer rows.Close()
	var out []domain.ArtifactVersion
	for rows.Next() {
		var v domain.ArtifactVersion
		var created string
		if err := rows.Scan(&v.ID, &v.ArtifactID, &v.Digest, &v.Status, &v.SizeBytes, &v.Version, &created); err != nil {
			return pagination.Result[domain.ArtifactVersion]{}, err
		}
		v.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return pagination.Result[domain.ArtifactVersion]{}, err
		}
		out = append(out, v)
	}
	return pagination.Result[domain.ArtifactVersion]{Items: out, Meta: pagination.Meta{Total: total, Limit: page.Limit, Offset: page.Offset}}, rows.Err()
}
func (q *VersionQuery) Active(ctx context.Context, tenantID, artifactID string) (domain.ArtifactVersion, error) {
	var v domain.ArtifactVersion
	var created string
	err := q.store.DB().QueryRowContext(ctx, `SELECT v.id,v.artifact_id,v.digest,v.status,v.size_bytes,v.version,v.created_at FROM artifact_versions v JOIN artifacts a ON a.active_version_id=v.id WHERE a.id=? AND a.tenant_id=?`, artifactID, tenantID).Scan(&v.ID, &v.ArtifactID, &v.Digest, &v.Status, &v.SizeBytes, &v.Version, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ArtifactVersion{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.ArtifactVersion{}, err
	}
	v.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	return v, err
}
func (q *VersionQuery) JobReferences(ctx context.Context, tenantID, versionID string) (int, error) {
	var count int
	err := q.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE tenant_id=? AND artifact_version_id=? AND status IN ('queued','claimed','running')`, tenantID, versionID).Scan(&count)
	return count, err
}
