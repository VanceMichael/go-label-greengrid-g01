package cluster

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
)

type CapacitySnapshot struct {
	ClusterID   string
	CapacityGPU int
	ReservedGPU int
	FreeGPU     int
	Nodes       int
	ReadyNodes  int
	Version     int64
	CapturedAt  time.Time
}

func Snapshot(ctx context.Context, store *sqlite.Store, tenantID, clusterID string) (CapacitySnapshot, error) {
	var snapshot CapacitySnapshot
	err := store.RunReadOnly(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, `SELECT id,capacity_gpu,reserved_gpu,version FROM clusters WHERE id=? AND tenant_id=?`, clusterID, tenantID).Scan(&snapshot.ClusterID, &snapshot.CapacityGPU, &snapshot.ReservedGPU, &snapshot.Version); err != nil {
			if err == sql.ErrNoRows {
				return domain.ErrNotFound
			}
			return fmt.Errorf("read capacity: %w", err)
		}
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(CASE WHEN status='ready' THEN 1 ELSE 0 END),0) FROM nodes WHERE cluster_id=?`, clusterID).Scan(&snapshot.Nodes, &snapshot.ReadyNodes); err != nil {
			return fmt.Errorf("read node capacity: %w", err)
		}
		return nil
	})
	if err != nil {
		return CapacitySnapshot{}, err
	}
	snapshot.FreeGPU = snapshot.CapacityGPU - snapshot.ReservedGPU
	snapshot.CapturedAt = time.Now().UTC()
	return snapshot, nil
}

func (s CapacitySnapshot) Valid() error {
	if s.CapacityGPU < 0 || s.ReservedGPU < 0 || s.FreeGPU < 0 {
		return fmt.Errorf("%w: negative capacity", domain.ErrConflict)
	}
	if s.ReservedGPU > s.CapacityGPU {
		return fmt.Errorf("%w: overcommitted cluster", domain.ErrConflict)
	}
	if s.ReadyNodes > s.Nodes {
		return fmt.Errorf("%w: ready node count", domain.ErrConflict)
	}
	return nil
}

func (s CapacitySnapshot) CanReserve(gpu int) bool {
	return gpu > 0 && gpu <= s.FreeGPU && s.StatusReady()
}

func (s CapacitySnapshot) StatusReady() bool {
	return s.ReadyNodes > 0 && s.FreeGPU > 0
}

func (s CapacitySnapshot) Utilization() float64 {
	if s.CapacityGPU == 0 {
		return 0
	}
	return float64(s.ReservedGPU) / float64(s.CapacityGPU)
}

func (s CapacitySnapshot) WithReservation(gpu int) (CapacitySnapshot, error) {
	if !s.CanReserve(gpu) {
		return CapacitySnapshot{}, domain.ErrCapacity
	}
	s.ReservedGPU += gpu
	s.FreeGPU -= gpu
	s.Version++
	return s, nil
}

func (s CapacitySnapshot) WithRelease(gpu int) (CapacitySnapshot, error) {
	if gpu <= 0 || gpu > s.ReservedGPU {
		return CapacitySnapshot{}, domain.ErrConflict
	}
	s.ReservedGPU -= gpu
	s.FreeGPU += gpu
	s.Version++
	return s, nil
}
