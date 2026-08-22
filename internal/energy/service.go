package energy

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
	"math"
	"time"
)

type Service struct{ store *sqlite.Store }
type Efficiency struct {
	ClusterID                              string
	From, To                               time.Time
	EnergyKWh, CarbonGrams, RenewableShare float64
	Samples                                int
}

func NewService(store *sqlite.Store) *Service { return &Service{store: store} }
func (s *Service) Calculate(ctx context.Context, tenantID, clusterID string, from, to time.Time) (Efficiency, error) {
	if !to.After(from) {
		return Efficiency{}, fmt.Errorf("%w: energy window", domain.ErrInvalid)
	}
	var owner string
	if err := s.store.DB().QueryRowContext(ctx, `SELECT tenant_id FROM clusters WHERE id=?`, clusterID).Scan(&owner); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Efficiency{}, domain.ErrNotFound
		}
		return Efficiency{}, err
	}
	if owner != tenantID {
		return Efficiency{}, domain.ErrForbidden
	}
	var watts, share float64
	var samples int
	if err := s.store.DB().QueryRowContext(ctx, `SELECT COALESCE(SUM(t.power_watts),0),COALESCE(AVG(t.renewable_share),0),COUNT(*) FROM telemetry_readings t JOIN nodes n ON n.id=t.node_id WHERE n.cluster_id=? AND t.measured_at>=? AND t.measured_at<?`, clusterID, from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano)).Scan(&watts, &share, &samples); err != nil {
		return Efficiency{}, err
	}
	if samples == 0 {
		return Efficiency{}, domain.ErrNotFound
	}
	return Efficiency{ClusterID: clusterID, From: from.UTC(), To: to.UTC(), EnergyKWh: watts / 1000, CarbonGrams: watts / 1000 * (1 - share) * 420, RenewableShare: share, Samples: samples}, nil
}
func (e Efficiency) CarbonIntensity() float64 {
	if e.EnergyKWh == 0 {
		return 0
	}
	return e.CarbonGrams / e.EnergyKWh
}
func (e Efficiency) RenewablePercent() float64 { return math.Round(e.RenewableShare*10000) / 100 }
