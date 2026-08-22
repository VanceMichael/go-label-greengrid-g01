package operations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
	"time"
)

type Finding struct{ Code, Severity, AggregateID, Message string }
type Report struct {
	TenantID  string
	CheckedAt time.Time
	Findings  []Finding
	Healthy   bool
}
type Reconciler struct{ store *sqlite.Store }

func NewReconciler(store *sqlite.Store) *Reconciler { return &Reconciler{store: store} }
func (r *Reconciler) CheckTenant(ctx context.Context, tenantID string) (Report, error) {
	if tenantID == "" {
		return Report{}, fmt.Errorf("%w: tenant", domain.ErrInvalid)
	}
	exists, err := r.store.HasTenant(ctx, tenantID)
	if err != nil {
		return Report{}, err
	}
	if !exists {
		return Report{}, domain.ErrNotFound
	}
	report := Report{TenantID: tenantID, CheckedAt: time.Now().UTC(), Healthy: true}
	checks := []func(context.Context, string) ([]Finding, error){r.checkReservationCapacity, r.checkJobLeases, r.checkArtifactReferences, r.checkOutboxLeases}
	for _, check := range checks {
		findings, err := check(ctx, tenantID)
		if err != nil {
			return Report{}, err
		}
		if len(findings) > 0 {
			report.Healthy = false
			report.Findings = append(report.Findings, findings...)
		}
	}
	return report, nil
}
func (r *Reconciler) checkReservationCapacity(ctx context.Context, tenantID string) ([]Finding, error) {
	rows, err := r.store.DB().QueryContext(ctx, `SELECT c.id,c.capacity_gpu,c.reserved_gpu,COALESCE(SUM(CASE WHEN rs.status IN ('approved','active') THEN rs.gpu_count ELSE 0 END),0) FROM clusters c LEFT JOIN reservations rs ON rs.cluster_id=c.id WHERE c.tenant_id=? GROUP BY c.id,c.capacity_gpu,c.reserved_gpu`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Finding
	for rows.Next() {
		var id string
		var capacity, reserved, expected int
		if err := rows.Scan(&id, &capacity, &reserved, &expected); err != nil {
			return nil, err
		}
		if reserved != expected {
			out = append(out, Finding{Code: "cluster_capacity_drift", Severity: "critical", AggregateID: id, Message: fmt.Sprintf("cluster reserved=%d but reservations=%d", reserved, expected)})
		}
		if reserved > capacity {
			out = append(out, Finding{Code: "cluster_overcommitted", Severity: "critical", AggregateID: id, Message: "reserved capacity exceeds physical capacity"})
		}
	}
	return out, rows.Err()
}
func (r *Reconciler) checkJobLeases(ctx context.Context, tenantID string) ([]Finding, error) {
	rows, err := r.store.DB().QueryContext(ctx, `SELECT j.id,j.status,COUNT(l.resource_id) FROM jobs j LEFT JOIN leases l ON l.resource_type='job' AND l.resource_id=j.id WHERE j.tenant_id=? GROUP BY j.id,j.status`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Finding
	for rows.Next() {
		var id, status string
		var leases int
		if err := rows.Scan(&id, &status, &leases); err != nil {
			return nil, err
		}
		if (status == "queued" || status == "succeeded" || status == "failed" || status == "cancelled") && leases > 0 {
			out = append(out, Finding{Code: "terminal_job_lease", Severity: "high", AggregateID: id, Message: "terminal job still owns a lease"})
		}
		if (status == "claimed" || status == "running") && leases != 1 {
			out = append(out, Finding{Code: "running_job_lease", Severity: "high", AggregateID: id, Message: "running job must own exactly one lease"})
		}
	}
	return out, rows.Err()
}
func (r *Reconciler) checkArtifactReferences(ctx context.Context, tenantID string) ([]Finding, error) {
	rows, err := r.store.DB().QueryContext(ctx, `SELECT a.id,a.active_version_id,v.status FROM artifacts a LEFT JOIN artifact_versions v ON v.id=a.active_version_id WHERE a.tenant_id=?`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Finding
	for rows.Next() {
		var id, active string
		var status sql.NullString
		if err := rows.Scan(&id, &active, &status); err != nil {
			return nil, err
		}
		if active != "" && !status.Valid {
			out = append(out, Finding{Code: "missing_active_version", Severity: "critical", AggregateID: id, Message: "artifact points to missing version"})
		}
		if active != "" && status.String != "promoted" {
			out = append(out, Finding{Code: "active_version_not_promoted", Severity: "high", AggregateID: id, Message: "active artifact version is not promoted"})
		}
	}
	return out, rows.Err()
}
func (r *Reconciler) checkOutboxLeases(ctx context.Context, tenantID string) ([]Finding, error) {
	rows, err := r.store.DB().QueryContext(ctx, `SELECT id,status,COALESCE(lease_owner,''),COALESCE(lease_until,'') FROM outbox_events WHERE tenant_id=?`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Finding
	now := time.Now().UTC()
	for rows.Next() {
		var id, status, owner, until string
		if err := rows.Scan(&id, &status, &owner, &until); err != nil {
			return nil, err
		}
		if (status == "pending" || status == "retry" || status == "sent" || status == "failed") && owner != "" {
			out = append(out, Finding{Code: "unexpected_outbox_lease", Severity: "medium", AggregateID: id, Message: "non-sending outbox event owns a lease"})
		}
		if status == "sending" && until != "" {
			expiry, parseErr := time.Parse(time.RFC3339Nano, until)
			if parseErr != nil {
				return nil, parseErr
			}
			if !expiry.After(now) {
				out = append(out, Finding{Code: "expired_outbox_lease", Severity: "high", AggregateID: id, Message: "sending event lease expired"})
			}
		}
	}
	return out, rows.Err()
}
func (r *Reconciler) AssertHealthy(ctx context.Context, tenantID string) error {
	report, err := r.CheckTenant(ctx, tenantID)
	if err != nil {
		return err
	}
	if !report.Healthy {
		return fmt.Errorf("%w: %d reconciliation findings", domain.ErrConflict, len(report.Findings))
	}
	return nil
}
func (r *Reconciler) RepairExpiredLeases(ctx context.Context, tenantID string, now time.Time) (int, error) {
	if _, err := r.CheckTenant(ctx, tenantID); err != nil {
		return 0, err
	}
	result, err := r.store.DB().ExecContext(ctx, `DELETE FROM leases WHERE expires_at<=? AND resource_id IN (SELECT j.id FROM jobs j WHERE j.tenant_id=?)`, now.UTC().Format(time.RFC3339Nano), tenantID)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}
func (r *Reconciler) VerifyTenantExists(ctx context.Context, tenantID string) error {
	ok, err := r.store.HasTenant(ctx, tenantID)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrNotFound
	}
	return nil
}

var _ = errors.Is
