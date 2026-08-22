package cluster

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/pagination"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
)

type NodeSummary struct {
	ID, Name, Status         string
	CapacityGPU, ReservedGPU int
	Version                  int64
}
type ServiceQuery struct{ store *sqlite.Store }

func NewQuery(store *sqlite.Store) *ServiceQuery { return &ServiceQuery{store: store} }
func (q *ServiceQuery) Nodes(ctx context.Context, tenantID, clusterID string, page pagination.Page) (pagination.Result[NodeSummary], error) {
	if err := page.Validate(); err != nil {
		return pagination.Result[NodeSummary]{}, err
	}
	var total int
	if err := q.store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM nodes n JOIN clusters c ON c.id=n.cluster_id WHERE c.tenant_id=? AND c.id=?`, tenantID, clusterID).Scan(&total); err != nil {
		return pagination.Result[NodeSummary]{}, err
	}
	rows, err := q.store.DB().QueryContext(ctx, `SELECT n.id,n.name,n.status,n.gpu_capacity,n.gpu_reserved,n.version FROM nodes n JOIN clusters c ON c.id=n.cluster_id WHERE c.tenant_id=? AND c.id=? ORDER BY n.name LIMIT ? OFFSET ?`, tenantID, clusterID, page.Limit, page.Offset)
	if err != nil {
		return pagination.Result[NodeSummary]{}, err
	}
	defer rows.Close()
	var items []NodeSummary
	for rows.Next() {
		var n NodeSummary
		if err := rows.Scan(&n.ID, &n.Name, &n.Status, &n.CapacityGPU, &n.ReservedGPU, &n.Version); err != nil {
			return pagination.Result[NodeSummary]{}, err
		}
		items = append(items, n)
	}
	return pagination.Result[NodeSummary]{Items: items, Meta: pagination.Meta{Total: total, Limit: page.Limit, Offset: page.Offset}}, rows.Err()
}
func (q *ServiceQuery) FindHealthy(ctx context.Context, tenantID, clusterID string) ([]NodeSummary, error) {
	rows, err := q.store.DB().QueryContext(ctx, `SELECT n.id,n.name,n.status,n.gpu_capacity,n.gpu_reserved,n.version FROM nodes n JOIN clusters c ON c.id=n.cluster_id WHERE c.tenant_id=? AND c.id=? AND n.status='ready' AND n.gpu_capacity>n.gpu_reserved ORDER BY n.gpu_reserved,n.name`, tenantID, clusterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []NodeSummary
	for rows.Next() {
		var n NodeSummary
		if err := rows.Scan(&n.ID, &n.Name, &n.Status, &n.CapacityGPU, &n.ReservedGPU, &n.Version); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
func (q *ServiceQuery) GetNode(ctx context.Context, tenantID, nodeID string) (NodeSummary, error) {
	var n NodeSummary
	err := q.store.DB().QueryRowContext(ctx, `SELECT n.id,n.name,n.status,n.gpu_capacity,n.gpu_reserved,n.version FROM nodes n JOIN clusters c ON c.id=n.cluster_id WHERE n.id=? AND c.tenant_id=?`, nodeID, tenantID).Scan(&n.ID, &n.Name, &n.Status, &n.CapacityGPU, &n.ReservedGPU, &n.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return NodeSummary{}, domain.ErrNotFound
	}
	if err != nil {
		return NodeSummary{}, fmt.Errorf("node query: %w", err)
	}
	return n, nil
}
