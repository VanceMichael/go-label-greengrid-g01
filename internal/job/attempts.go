package job

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

func persistAttempt(ctx context.Context, tx *sql.Tx, jobID string, attemptNo int, workerID, status, message string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := tx.ExecContext(ctx, `INSERT INTO job_attempts(id,job_id,attempt_no,worker_id,status,error_message,started_at,finished_at) VALUES(?,?,?,?,?,?,?,?)`, uuid.NewString(), jobID, attemptNo, workerID, status, nullable(message), now, now)
	return err
}
