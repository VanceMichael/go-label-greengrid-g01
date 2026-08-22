package sqlite

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

func ExecAudit(tx *sql.Tx, tenantID, actorID, aggregateType, aggregateID, action, result, requestID, details string) error {
	_, err := tx.Exec(`INSERT INTO audit_events(id,tenant_id,actor_id,aggregate_type,aggregate_id,action,result,request_id,details,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, uuid.NewString(), tenantID, actorID, aggregateType, aggregateID, action, result, requestID, details, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
