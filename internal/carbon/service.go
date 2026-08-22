package carbon

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

func (s *Service) Generate(ctx context.Context, tenantID, clusterID string, from, to time.Time, actorID, requestID string) (domain.CarbonReport, error) {
	if !to.After(from) {
		return domain.CarbonReport{}, fmt.Errorf("%w: report window", domain.ErrInvalid)
	}
	var report domain.CarbonReport
	err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var owner string
		if err := tx.QueryRowContext(ctx, `SELECT tenant_id FROM clusters WHERE id=?`, clusterID).Scan(&owner); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if owner != tenantID {
			return domain.ErrForbidden
		}
		var existing domain.CarbonReport
		var existingStart, existingEnd, existingCreated string
		existingErr := tx.QueryRowContext(ctx, `SELECT id,tenant_id,cluster_id,window_start,window_end,energy_kwh,carbon_grams,renewable_share,status,version,created_at FROM carbon_reports WHERE tenant_id=? AND cluster_id=? AND window_start=? AND window_end=?`, tenantID, clusterID, from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano)).Scan(&existing.ID, &existing.TenantID, &existing.ClusterID, &existingStart, &existingEnd, &existing.EnergyKWh, &existing.CarbonGrams, &existing.RenewableShare, &existing.Status, &existing.Version, &existingCreated)
		if existingErr == nil {
			var parseErr error
			existing.WindowStart, parseErr = time.Parse(time.RFC3339Nano, existingStart)
			if parseErr != nil {
				return parseErr
			}
			existing.WindowEnd, parseErr = time.Parse(time.RFC3339Nano, existingEnd)
			if parseErr != nil {
				return parseErr
			}
			_ = existingCreated
			report = existing
			return nil
		}
		if !errors.Is(existingErr, sql.ErrNoRows) {
			return existingErr
		}
		var watts, renewable float64
		var count int
		if err := tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(t.power_watts),0),COALESCE(AVG(t.renewable_share),0),COUNT(*) FROM telemetry_readings t JOIN nodes n ON n.id=t.node_id WHERE n.cluster_id=? AND t.measured_at>=? AND t.measured_at<?`, clusterID, from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano)).Scan(&watts, &renewable, &count); err != nil {
			return err
		}
		if count == 0 {
			return fmt.Errorf("%w: no telemetry", domain.ErrConflict)
		}
		now := time.Now().UTC()
		report = domain.CarbonReport{ID: uuid.NewString(), TenantID: tenantID, ClusterID: clusterID, WindowStart: from.UTC(), WindowEnd: to.UTC(), EnergyKWh: watts / 1000, CarbonGrams: (watts / 1000) * (1 - renewable) * 420, RenewableShare: renewable, Status: "draft", Version: 1}
		if _, err := tx.ExecContext(ctx, `INSERT INTO carbon_reports(id,tenant_id,cluster_id,window_start,window_end,energy_kwh,carbon_grams,renewable_share,status,version,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, report.ID, tenantID, clusterID, report.WindowStart.Format(time.RFC3339Nano), report.WindowEnd.Format(time.RFC3339Nano), report.EnergyKWh, report.CarbonGrams, report.RenewableShare, report.Status, 1, now.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		return audit(tx, tenantID, actorID, "carbon_report", report.ID, "generate", requestID, "draft")
	})
	return report, err
}

func (s *Service) Approve(ctx context.Context, tenantID, actorID, reportID string, expected int64, requestID string) error {
	return s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var status string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM carbon_reports WHERE id=? AND tenant_id=?`, reportID, tenantID).Scan(&status); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return err
		}
		if status != "draft" {
			return domain.ErrState
		}
		result, err := tx.ExecContext(ctx, `UPDATE carbon_reports SET status='approved',version=version+1 WHERE id=? AND tenant_id=? AND version=? AND status='draft'`, reportID, tenantID, expected)
		if err != nil {
			return err
		}
		n, _ := result.RowsAffected()
		if n != 1 {
			return domain.ErrConflict
		}
		if err := audit(tx, tenantID, actorID, "carbon_report", reportID, "approve", requestID, "approved"); err != nil {
			return err
		}
		now := time.Now().UTC().Format(time.RFC3339Nano)
		_, err = tx.ExecContext(ctx, `INSERT INTO outbox_events(id,tenant_id,kind,aggregate_id,payload,status,attempts,next_attempt_at,created_at) VALUES(?,?,?,?,?,'pending',0,?,?)`, uuid.NewString(), tenantID, "carbon_report.approved", reportID, `{"status":"approved"}`, now, now)
		return err
	})
}

func audit(tx *sql.Tx, tenantID, actorID, typ, id, action, requestID, details string) error {
	_, err := tx.Exec(`INSERT INTO audit_events(id,tenant_id,actor_id,aggregate_type,aggregate_id,action,result,request_id,details,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, uuid.NewString(), tenantID, actorID, typ, id, action, "success", requestID, details, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
