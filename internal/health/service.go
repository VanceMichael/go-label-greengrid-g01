package health

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
)

type Status struct {
	Live      bool
	Ready     bool
	CheckedAt time.Time
	Detail    string
}
type Service struct {
	store *sqlite.Store
	live  atomic.Bool
}

func NewService(store *sqlite.Store) *Service {
	h := &Service{store: store}
	h.live.Store(true)
	return h
}
func (s *Service) Live() Status {
	return Status{Live: s.live.Load(), Ready: false, CheckedAt: time.Now().UTC(), Detail: "process"}
}
func (s *Service) Ready(ctx context.Context) Status {
	err := s.store.Ping(ctx)
	return Status{Live: s.live.Load(), Ready: err == nil, CheckedAt: time.Now().UTC(), Detail: detail(err)}
}
func (s *Service) Stop() { s.live.Store(false) }
func detail(err error) string {
	if err == nil {
		return "database reachable"
	}
	return err.Error()
}
