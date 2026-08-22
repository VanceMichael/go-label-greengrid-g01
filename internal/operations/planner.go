package operations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
	"sort"
	"time"
)

type CandidateNode struct {
	ID      string
	FreeGPU int
	Status  string
	Version int64
}
type Plan struct {
	TenantID, ClusterID string
	GPU                 int
	StartsAt, EndsAt    time.Time
	Nodes               []CandidateNode
	Accepted            bool
	Reason              string
}
type Planner struct{ store *sqlite.Store }

func NewPlanner(store *sqlite.Store) *Planner { return &Planner{store: store} }
func (p *Planner) Preview(ctx context.Context, tenantID, clusterID string, gpu int, start, end time.Time) (Plan, error) {
	if gpu <= 0 || !end.After(start) {
		return Plan{}, fmt.Errorf("%w: plan input", domain.ErrInvalid)
	}
	var owner string
	if err := p.store.DB().QueryRowContext(ctx, `SELECT tenant_id FROM clusters WHERE id=?`, clusterID).Scan(&owner); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Plan{}, domain.ErrNotFound
		}
		return Plan{}, err
	}
	if owner != tenantID {
		return Plan{}, domain.ErrForbidden
	}
	rows, err := p.store.DB().QueryContext(ctx, `SELECT n.id,n.gpu_capacity-n.gpu_reserved,n.status,n.version FROM nodes n WHERE n.cluster_id=? AND n.status='ready' ORDER BY (n.gpu_capacity-n.gpu_reserved) DESC,n.id`, clusterID)
	if err != nil {
		return Plan{}, err
	}
	defer rows.Close()
	plan := Plan{TenantID: tenantID, ClusterID: clusterID, GPU: gpu, StartsAt: start.UTC(), EndsAt: end.UTC(), Reason: "insufficient capacity"}
	remaining := gpu
	for rows.Next() {
		var n CandidateNode
		if err := rows.Scan(&n.ID, &n.FreeGPU, &n.Status, &n.Version); err != nil {
			return Plan{}, err
		}
		if n.FreeGPU > 0 {
			plan.Nodes = append(plan.Nodes, n)
			remaining -= n.FreeGPU
			if remaining <= 0 {
				plan.Accepted = true
				plan.Reason = "capacity available"
				break
			}
		}
	}
	return plan, rows.Err()
}
func (p *Planner) ReserveNodes(ctx context.Context, tenantID, clusterID string, gpu int, start, end time.Time, actorID, requestID string) (Plan, error) {
	plan, err := p.Preview(ctx, tenantID, clusterID, gpu, start, end)
	if err != nil || !plan.Accepted {
		return plan, err
	}
	err = p.store.WithTx(ctx, func(tx *sql.Tx) error {
		remaining := gpu
		for _, node := range plan.Nodes {
			take := node.FreeGPU
			if take > remaining {
				take = remaining
			}
			result, err := tx.ExecContext(ctx, `UPDATE nodes SET gpu_reserved=gpu_reserved+?,version=version+1 WHERE id=? AND cluster_id=? AND status='ready' AND gpu_capacity-gpu_reserved>=? AND version=?`, take, node.ID, clusterID, take, node.Version)
			if err != nil {
				return err
			}
			n, _ := result.RowsAffected()
			if n != 1 {
				return domain.ErrConflict
			}
			remaining -= take
			if remaining == 0 {
				break
			}
		}
		if remaining != 0 {
			return domain.ErrCapacity
		}
		_, err := tx.Exec(`INSERT INTO audit_events(id,tenant_id,actor_id,aggregate_type,aggregate_id,action,result,request_id,details,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, fmt.Sprintf("plan-%d", time.Now().UnixNano()), tenantID, actorID, "cluster", clusterID, "node_reserve", "success", requestID, fmt.Sprintf("reserved %d gpu", gpu), time.Now().UTC().Format(time.RFC3339Nano))
		return err
	})
	return plan, err
}
func SortCandidates(nodes []CandidateNode) {
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].FreeGPU != nodes[j].FreeGPU {
			return nodes[i].FreeGPU > nodes[j].FreeGPU
		}
		return nodes[i].ID < nodes[j].ID
	})
}
func Available(nodes []CandidateNode) int {
	total := 0
	for _, n := range nodes {
		if n.Status == "ready" && n.FreeGPU > 0 {
			total += n.FreeGPU
		}
	}
	return total
}
