package operations

import (
	"context"
	"database/sql"
)

// removeRecoveredLease deletes the job lease for a job whose expired lease
// just returned it to the queue. Recovery must converge ownership with the
// new queued status; otherwise the dead worker's lease row survives and a
// restarted worker can be blocked at Start/Finish despite owning the job.
func removeRecoveredLease(ctx context.Context, tx *sql.Tx, jobID string) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM leases WHERE resource_type='job' AND resource_id=?`, jobID)
	return err
}
