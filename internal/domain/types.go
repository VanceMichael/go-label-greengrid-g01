package domain

import "time"

type Role string

const (
	RolePlatformAdmin Role = "platform_admin"
	RoleClusterOps    Role = "cluster_ops"
	RoleTenantAdmin   Role = "tenant_admin"
	RoleScheduler     Role = "scheduler"
	RoleAuditor       Role = "auditor"
)

func (r Role) Valid() bool {
	switch r {
	case RolePlatformAdmin, RoleClusterOps, RoleTenantAdmin, RoleScheduler, RoleAuditor:
		return true
	default:
		return false
	}
}

type ReservationStatus string

const (
	ReservationDraft     ReservationStatus = "draft"
	ReservationRequested ReservationStatus = "requested"
	ReservationApproved  ReservationStatus = "approved"
	ReservationActive    ReservationStatus = "active"
	ReservationReleased  ReservationStatus = "released"
	ReservationCancelled ReservationStatus = "cancelled"
)

type JobStatus string

const (
	JobQueued    JobStatus = "queued"
	JobClaimed   JobStatus = "claimed"
	JobRunning   JobStatus = "running"
	JobSucceeded JobStatus = "succeeded"
	JobFailed    JobStatus = "failed"
	JobCancelled JobStatus = "cancelled"
)

type ArtifactStatus string

const (
	ArtifactUploaded ArtifactStatus = "uploaded"
	ArtifactScanned  ArtifactStatus = "scanned"
	ArtifactPromoted ArtifactStatus = "promoted"
	ArtifactRetired  ArtifactStatus = "retired"
)

type User struct {
	ID, TenantID, Email, DisplayName string
	Role                             Role
	Active                           bool
	CreatedAt                        time.Time
}
type Session struct {
	ID, UserID, TokenHash string
	ExpiresAt             time.Time
	Revoked               bool
}
type Cluster struct {
	ID, TenantID, Name, Region string
	CapacityGPU, ReservedGPU   int
	Version                    int64
	Status                     string
}
type Node struct {
	ID, ClusterID, Name      string
	GPUCapacity, GPUReserved int
	Status                   string
	Version                  int64
}
type Reservation struct {
	ID, TenantID, ClusterID, RequestedBy string
	GPUCount                             int
	StartsAt, EndsAt                     time.Time
	Status                               ReservationStatus
	Version                              int64
	CreatedAt                            time.Time
}
type Job struct {
	ID, TenantID, ReservationID, ArtifactVersionID string
	Name                                           string
	GPUCount                                       int
	Status                                         JobStatus
	Attempts                                       int
	Version                                        int64
	CreatedAt                                      time.Time
	StartedAt, FinishedAt                          *time.Time
}
type TelemetryReading struct {
	ID, TenantID, NodeID       string
	Sequence                   int64
	MeasuredAt                 time.Time
	PowerWatts, RenewableShare float64
}
type CarbonReport struct {
	ID, TenantID, ClusterID                string
	WindowStart, WindowEnd                 time.Time
	EnergyKWh, CarbonGrams, RenewableShare float64
	Status                                 string
	Version                                int64
}
type Artifact struct {
	ID, TenantID, Name, ActiveVersionID string
	Status                              ArtifactStatus
	Version                             int64
}
type ArtifactVersion struct {
	ID, ArtifactID, Digest string
	Status                 ArtifactStatus
	SizeBytes              int64
	CreatedAt              time.Time
	Version                int64
}
type OutboxEvent struct {
	ID, TenantID, Kind, AggregateID, Payload string
	Status                                   string
	Attempts                                 int
	LeaseOwner                               string
	LeaseUntil                               *time.Time
	NextAttemptAt                            time.Time
}
