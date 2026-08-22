# GreenGrid

GreenGrid is a production-oriented control plane for the renewable-powered compute operations described by the China Cloud Valley theme. It coordinates tenants, accelerator clusters, time-bounded capacity reservations, training jobs, energy telemetry, carbon-efficiency reports, model artifacts, audit events, and durable outbox delivery.

## Run

```bash
cp .env.example .env
go run ./cmd/server
```

The service exposes `/livez` for process liveness and `/readyz` for database readiness. The default database is a real SQLite file configured by `GREENGRID_DB`.

## Checks

```bash
make test
make race
make vet
make build
```

The test suite uses temporary real SQLite databases and deterministic barriers for concurrency paths. It covers authentication and roles, reservation and job state machines, transactional persistence, telemetry aggregation, artifact promotion, worker lease recovery, HTTP contracts, and graceful shutdown.

## Main business paths

- Operators register clusters and nodes; tenant schedulers request and approve capacity reservations.
- Approved reservations admit training jobs. Workers claim jobs with durable leases, execute retryable attempts, and release capacity on terminal completion.
- Nodes publish ordered power readings. The telemetry service aggregates renewable share and job energy into carbon-efficiency reports.
- Model versions move through upload, scan, promotion, and retirement only when active jobs and production references permit it.
- Audited domain events are delivered through an outbox after durable state changes.
