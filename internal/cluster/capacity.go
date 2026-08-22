package cluster

import (
	"database/sql"
	"fmt"

	"github.com/VanceMichael/greengrid/internal/domain"
)

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
