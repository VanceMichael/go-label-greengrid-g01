package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/VanceMichael/greengrid/internal/artifact"
	"github.com/VanceMichael/greengrid/internal/carbon"
	"github.com/VanceMichael/greengrid/internal/cluster"
	"github.com/VanceMichael/greengrid/internal/config"
	"github.com/VanceMichael/greengrid/internal/eventbus"
	"github.com/VanceMichael/greengrid/internal/httpapi"
	"github.com/VanceMichael/greengrid/internal/identity"
	"github.com/VanceMichael/greengrid/internal/job"
	"github.com/VanceMichael/greengrid/internal/middleware"
	"github.com/VanceMichael/greengrid/internal/outbox"
	"github.com/VanceMichael/greengrid/internal/reservation"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
	"github.com/VanceMichael/greengrid/internal/telemetry"
	"github.com/VanceMichael/greengrid/internal/worker"
)

type Runtime struct {
	cfg     config.Config
	store   *sqlite.Store
	server  *http.Server
	workers *worker.Runner
	logger  *slog.Logger
	bus     *eventbus.Bus
}

func New(cfg config.Config, logger *slog.Logger) (*Runtime, error) {
	store, err := sqlite.Open(cfg.DatabasePath)
	if err != nil {
		return nil, err
	}
	migrateCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := store.Migrate(migrateCtx); err != nil {
		_ = store.Close()
		return nil, err
	}
	ids := identity.NewService(store, cfg.SessionTTL)
	clusters := cluster.NewService(store)
	reservations := reservation.NewService(store, clusters)
	jobs := job.NewService(store)
	telemetrySvc := telemetry.NewService(store)
	carbonSvc := carbon.NewService(store)
	artifactSvc := artifact.NewService(store)
	events := outbox.NewService(store, 5)
	bus := eventbus.New()
	handler := httpapi.New(httpapi.Dependencies{Store: store, Identity: ids, Cluster: clusters, Reservation: reservations, Job: jobs, Telemetry: telemetrySvc, Carbon: carbonSvc, Artifact: artifactSvc}).Handler()
	server := &http.Server{Addr: cfg.Address, Handler: middleware.Recovery(logger, handler), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	sender := outbox.NewLoggerSender(logger)
	runner := worker.New(jobs, events, sender, cfg.WorkerInterval, "greengrid-worker", logger)
	return &Runtime{cfg: cfg, store: store, server: server, workers: runner, logger: logger, bus: bus}, nil
}

func (r *Runtime) Run(parent context.Context) error {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	defer stop()
	r.workers.Start(ctx)
	errCh := make(chan error, 1)
	go func() {
		r.logger.Info("http server listening", "addr", r.cfg.Address)
		if err := r.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	select {
	case err := <-errCh:
		r.workers.Stop(context.Background())
		_ = r.store.Close()
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), r.cfg.ShutdownTimeout)
		defer cancel()
		_ = r.server.Shutdown(shutdownCtx)
		if err := r.workers.Stop(shutdownCtx); err != nil {
			r.logger.Warn("worker drain timeout", "error", err)
		}
		return r.store.Close()
	}
}

func (r *Runtime) Close() error {
	if r.server != nil {
		_ = r.server.Close()
	}
	if r.workers != nil {
		_ = r.workers.Stop(context.Background())
	}
	if r.bus != nil {
		r.bus.Close()
	}
	if r.store != nil {
		return r.store.Close()
	}
	return nil
}
