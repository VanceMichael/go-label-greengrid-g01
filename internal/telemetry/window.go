package telemetry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
	"time"
)

type WindowSummary struct {
	NodeID                    string
	From, To                  time.Time
	Samples                   int
	EnergyKWh, RenewableShare float64
	Complete                  bool
}
type WindowQuery struct{ store *sqlite.Store }

func NewWindowQuery(store *sqlite.Store) *WindowQuery { return &WindowQuery{store: store} }
func (q *WindowQuery) Summarize(ctx context.Context, tenantID, nodeID string, from, to time.Time, expected int) (WindowSummary, error) {
	if !to.After(from) || expected < 1 {
		return WindowSummary{}, fmt.Errorf("%w: telemetry window", domain.ErrInvalid)
	}
	var owner string
	if err := q.store.DB().QueryRowContext(ctx, `SELECT c.tenant_id FROM nodes n JOIN clusters c ON c.id=n.cluster_id WHERE n.id=?`, nodeID).Scan(&owner); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WindowSummary{}, domain.ErrNotFound
		}
		return WindowSummary{}, err
	}
	if owner != tenantID {
		return WindowSummary{}, domain.ErrForbidden
	}
	var watts, share float64
	var count int
	if err := q.store.DB().QueryRowContext(ctx, `SELECT COALESCE(SUM(power_watts),0),COALESCE(AVG(renewable_share),0),COUNT(*) FROM telemetry_readings WHERE tenant_id=? AND node_id=? AND measured_at>=? AND measured_at<?`, tenantID, nodeID, from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano)).Scan(&watts, &share, &count); err != nil {
		return WindowSummary{}, err
	}
	return WindowSummary{NodeID: nodeID, From: from.UTC(), To: to.UTC(), Samples: count, EnergyKWh: watts / 1000, RenewableShare: share, Complete: count >= expected}, nil
}
func (q *WindowQuery) LatestSequence(ctx context.Context, tenantID, nodeID string) (int64, error) {
	var sequence sql.NullInt64
	err := q.store.DB().QueryRowContext(ctx, `SELECT MAX(sequence) FROM telemetry_readings WHERE tenant_id=? AND node_id=?`, tenantID, nodeID).Scan(&sequence)
	if err != nil {
		return 0, err
	}
	if !sequence.Valid {
		return 0, domain.ErrNotFound
	}
	return sequence.Int64, nil
}
func (q *WindowQuery) MissingSequences(ctx context.Context, tenantID, nodeID string, from, to time.Time) ([]int64, error) {
	rows, err := q.store.DB().QueryContext(ctx, `SELECT sequence FROM telemetry_readings WHERE tenant_id=? AND node_id=? AND measured_at>=? AND measured_at<? ORDER BY sequence`, tenantID, nodeID, from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var seq []int64
	for rows.Next() {
		var n int64
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		seq = append(seq, n)
	}
	return seq, rows.Err()
}
