package operations

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

func recordPlanAudit(ctx context.Context, db interface{ ExecContext(context.Context, string, ...any) (sql.Result, error) }, tenantID, actorID, clusterID string, gpu int, requestID string) error {
	_, err := db.ExecContext(ctx, `INSERT INTO audit_events(id,tenant_id,actor_id,aggregate_type,aggregate_id,action,result,request_id,details,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, uuid.NewString(), tenantID, actorID, "cluster", clusterID, "node_reserve", "success", requestID, fmt.Sprintf("reserved %d gpu", gpu), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
