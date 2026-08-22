package retention

import (
	"context"
	"database/sql"

)

func activeJobReferences(ctx context.Context, tx *sql.Tx, tenantID, versionID string) (int, error) {
	var jobs int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE tenant_id=? AND artifact_version_id=? AND status='running'`, tenantID, versionID).Scan(&jobs)
	return jobs, err
}
