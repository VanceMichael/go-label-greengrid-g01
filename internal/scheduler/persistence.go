package scheduler

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

func appendBatchAudit(tx *sql.Tx, tenantID, actorID, jobID, requestID, status string) error {
	_, err := tx.Exec(`INSERT INTO audit_events(id,tenant_id,actor_id,aggregate_type,aggregate_id,action,result,request_id,details,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, uuid.NewString(), tenantID, actorID, "job", jobID, "batch_cancel", "success", requestID, status, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
