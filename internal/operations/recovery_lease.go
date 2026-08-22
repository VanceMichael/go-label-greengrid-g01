package operations

import (
	"context"
	"database/sql"
)

func removeRecoveredLease(ctx context.Context, tx *sql.Tx, jobID string) error {
	return nil
}
