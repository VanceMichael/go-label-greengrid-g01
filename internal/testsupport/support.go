package testsupport

import (
	"context"
	"time"

	"github.com/VanceMichael/greengrid/internal/artifact"
	"github.com/VanceMichael/greengrid/internal/carbon"
	"github.com/VanceMichael/greengrid/internal/cluster"
	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/identity"
	"github.com/VanceMichael/greengrid/internal/job"
	"github.com/VanceMichael/greengrid/internal/outbox"
	"github.com/VanceMichael/greengrid/internal/reservation"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
	"github.com/VanceMichael/greengrid/internal/telemetry"
)

type Services struct {
	Store       *sqlite.Store
	Identity    *identity.Service
	Cluster     *cluster.Service
	Reservation *reservation.Service
	Job         *job.Service
	Telemetry   *telemetry.Service
	Carbon      *carbon.Service
	Artifact    *artifact.Service
	Outbox      *outbox.Service
}

func Open(path string) (Services, error) {
	store, err := sqlite.Open(path)
	if err != nil {
		return Services{}, err
	}
	if err := store.Migrate(context.Background()); err != nil {
		_ = store.Close()
		return Services{}, err
	}
	clusters := cluster.NewService(store)
	return Services{Store: store, Identity: identity.NewService(store, 8*time.Hour), Cluster: clusters, Reservation: reservation.NewService(store, clusters), Job: job.NewService(store), Telemetry: telemetry.NewService(store), Carbon: carbon.NewService(store), Artifact: artifact.NewService(store), Outbox: outbox.NewService(store, 3)}, nil
}
func CreateTenant(ctx context.Context, s Services, name string) (string, error) {
	return s.Identity.CreateTenant(ctx, name)
}
func CreateUser(ctx context.Context, s Services, tenant, email string, role domain.Role) (domain.User, error) {
	return s.Identity.CreateUser(ctx, tenant, email, email, "secret-pass", role)
}
