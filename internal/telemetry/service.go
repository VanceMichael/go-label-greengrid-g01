package telemetry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
	"github.com/google/uuid"
)

type Service struct{ store *sqlite.Store }

func NewService(store *sqlite.Store) *Service { return &Service{store: store} }

func (s *Service) Record(ctx context.Context, tenantID, nodeID string, sequence int64, measuredAt time.Time, watts, renewable float64, requestID string) (domain.TelemetryReading, error) {
	if sequence < 0 || watts < 0 || renewable < 0 || renewable > 1 {
		return domain.TelemetryReading{}, fmt.Errorf("%w: telemetry values", domain.ErrInvalid)
	}
	r := domain.TelemetryReading{ID: uuid.NewString(), TenantID: tenantID, NodeID: nodeID, Sequence: sequence, MeasuredAt: measuredAt.UTC(), PowerWatts: watts, RenewableShare: renewable}
	err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var owner string
		if err := tx.QueryRowContext(ctx, `SELECT c.tenant_id FROM nodes n JOIN clusters c ON c.id=n.cluster_id WHERE n.id=?`, nodeID).Scan(&owner); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if owner != tenantID {
			return domain.ErrForbidden
		}
		result, err := tx.ExecContext(ctx, `INSERT INTO telemetry_readings(id,tenant_id,node_id,sequence,measured_at,power_watts,renewable_share) VALUES(?,?,?,?,?,?,?)`, r.ID, tenantID, nodeID, sequence, r.MeasuredAt.Format(time.RFC3339Nano), watts, renewable)
		if err != nil {
			if isConstraint(err) {
				return domain.ErrAlreadyExists
			}
			return err
		}
		_ = result
		return nil
	})
	return r, err
}

func (s *Service) Readings(ctx context.Context, tenantID, nodeID string, from, to time.Time) ([]domain.TelemetryReading, error) {
	rows, err := s.store.DB().QueryContext(ctx, `SELECT id,tenant_id,node_id,sequence,measured_at,power_watts,renewable_share FROM telemetry_readings WHERE tenant_id=? AND node_id=? AND measured_at>=? AND measured_at<? ORDER BY measured_at,sequence`, tenantID, nodeID, from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.TelemetryReading
	for rows.Next() {
		var r domain.TelemetryReading
		var measured string
		if err := rows.Scan(&r.ID, &r.TenantID, &r.NodeID, &r.Sequence, &measured, &r.PowerWatts, &r.RenewableShare); err != nil {
			return nil, err
		}
		r.MeasuredAt, err = time.Parse(time.RFC3339Nano, measured)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func isConstraint(err error) bool {
	return err != nil && (stringsContains(err.Error(), "UNIQUE") || stringsContains(err.Error(), "constraint"))
}
func stringsContains(value, part string) bool {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
