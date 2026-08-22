package cluster

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
	"github.com/google/uuid"
)

type Service struct{ store *sqlite.Store }

func NewService(store *sqlite.Store) *Service { return &Service{store: store} }

func (s *Service) CreateCluster(ctx context.Context, tenantID, name, region string, capacity int) (domain.Cluster, error) {
	if tenantID == "" || strings.TrimSpace(name) == "" || capacity <= 0 {
		return domain.Cluster{}, fmt.Errorf("%w: cluster fields", domain.ErrInvalid)
	}
	now := time.Now().UTC()
	cluster := domain.Cluster{ID: uuid.NewString(), TenantID: tenantID, Name: name, Region: region, CapacityGPU: capacity, Status: "online", Version: 1}
	_, err := s.store.DB().ExecContext(ctx, `INSERT INTO clusters(id,tenant_id,name,region,capacity_gpu,reserved_gpu,version,status,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, cluster.ID, tenantID, name, region, capacity, 0, 1, cluster.Status, now.Format(time.RFC3339Nano))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return domain.Cluster{}, domain.ErrAlreadyExists
		}
		return domain.Cluster{}, fmt.Errorf("create cluster: %w", err)
	}
	return cluster, nil
}

func (s *Service) AddNode(ctx context.Context, actorID, tenantID, clusterID, name string, capacity int, requestID string) (domain.Node, error) {
	if capacity <= 0 || name == "" {
		return domain.Node{}, fmt.Errorf("%w: node fields", domain.ErrInvalid)
	}
	node := domain.Node{ID: uuid.NewString(), ClusterID: clusterID, Name: name, GPUCapacity: capacity, Status: "ready", Version: 1}
	err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var owner string
		if err := tx.QueryRowContext(ctx, `SELECT tenant_id FROM clusters WHERE id=? AND status='online'`, clusterID).Scan(&owner); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return fmt.Errorf("read cluster: %w", err)
		}
		if owner != tenantID {
			return domain.ErrForbidden
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO nodes(id,cluster_id,name,gpu_capacity,gpu_reserved,status,version,created_at) VALUES(?,?,?,?,?,?,?,?)`, node.ID, clusterID, name, capacity, 0, node.Status, 1, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				return domain.ErrAlreadyExists
			}
			return fmt.Errorf("insert node: %w", err)
		}
		if err := audit(tx, tenantID, actorID, "node", node.ID, "register", requestID, "ready"); err != nil {
			return fmt.Errorf("audit node: %w", err)
		}
		return nil
	})
	if err != nil {
		return domain.Node{}, err
	}
	return node, nil
}

func (s *Service) GetCluster(ctx context.Context, tenantID, id string) (domain.Cluster, error) {
	var c domain.Cluster
	err := s.store.DB().QueryRowContext(ctx, `SELECT id,tenant_id,name,region,capacity_gpu,reserved_gpu,version,status FROM clusters WHERE id=? AND tenant_id=?`, id, tenantID).Scan(&c.ID, &c.TenantID, &c.Name, &c.Region, &c.CapacityGPU, &c.ReservedGPU, &c.Version, &c.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Cluster{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Cluster{}, fmt.Errorf("get cluster: %w", err)
	}
	return c, nil
}

func (s *Service) GetNode(ctx context.Context, tenantID, nodeID string) (domain.Node, error) {
	var n domain.Node
	err := s.store.DB().QueryRowContext(ctx, `SELECT n.id,n.cluster_id,n.name,n.gpu_capacity,n.gpu_reserved,n.status,n.version FROM nodes n JOIN clusters c ON c.id=n.cluster_id WHERE n.id=? AND c.tenant_id=?`, nodeID, tenantID).Scan(&n.ID, &n.ClusterID, &n.Name, &n.GPUCapacity, &n.GPUReserved, &n.Status, &n.Version)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Node{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.Node{}, fmt.Errorf("get node: %w", err)
	}
	return n, nil
}

func (s *Service) ReserveCapacity(tx *sql.Tx, clusterID string, gpu int) error {
	if gpu <= 0 {
		return fmt.Errorf("%w: gpu count", domain.ErrInvalid)
	}
	result, err := tx.Exec(`UPDATE clusters SET reserved_gpu=reserved_gpu+?,version=version+1 WHERE id=? AND status='online' AND capacity_gpu-reserved_gpu>=?`, gpu, clusterID, gpu)
	if err != nil {
		return fmt.Errorf("reserve cluster capacity: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return domain.ErrCapacity
	}
	return nil
}

func (s *Service) ReleaseCapacity(tx *sql.Tx, clusterID string, gpu int) error {
	result, err := tx.Exec(`UPDATE clusters SET reserved_gpu=reserved_gpu-?,version=version+1 WHERE id=? AND reserved_gpu>=?`, gpu, clusterID, gpu)
	if err != nil {
		return fmt.Errorf("release cluster capacity: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows != 1 {
		return domain.ErrConflict
	}
	return nil
}

func audit(tx *sql.Tx, tenantID, actorID, aggregateType, aggregateID, action, requestID, details string) error {
	_, err := tx.Exec(`INSERT INTO audit_events(id,tenant_id,actor_id,aggregate_type,aggregate_id,action,result,request_id,details,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, uuid.NewString(), tenantID, actorID, aggregateType, aggregateID, action, "success", requestID, details, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
