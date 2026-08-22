package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/VanceMichael/greengrid/internal/artifact"
	"github.com/VanceMichael/greengrid/internal/carbon"
	"github.com/VanceMichael/greengrid/internal/cluster"
	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/identity"
	"github.com/VanceMichael/greengrid/internal/job"
	"github.com/VanceMichael/greengrid/internal/middleware"
	"github.com/VanceMichael/greengrid/internal/reservation"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
	"github.com/VanceMichael/greengrid/internal/telemetry"
)

type Dependencies struct {
	Store       *sqlite.Store
	Identity    *identity.Service
	Cluster     *cluster.Service
	Reservation *reservation.Service
	Job         *job.Service
	Telemetry   *telemetry.Service
	Carbon      *carbon.Service
	Artifact    *artifact.Service
}
type Server struct {
	deps Dependencies
	mux  *http.ServeMux
}

func New(deps Dependencies) *Server {
	s := &Server{deps: deps, mux: http.NewServeMux()}
	s.routes()
	return s
}
func (s *Server) Handler() http.Handler { return middleware.RequestID(s.mux) }

func (s *Server) routes() {
	s.mux.HandleFunc("GET /livez", s.livez)
	s.mux.HandleFunc("GET /readyz", s.readyz)
	s.mux.HandleFunc("POST /v1/tenants", s.createTenant)
	s.mux.HandleFunc("POST /v1/users", s.createUser)
	s.mux.HandleFunc("POST /v1/sessions", s.login)
	s.mux.HandleFunc("DELETE /v1/sessions", s.revoke)
	s.mux.HandleFunc("POST /v1/clusters", s.createCluster)
	s.mux.HandleFunc("POST /v1/clusters/{id}/nodes", s.addNode)
	s.mux.HandleFunc("POST /v1/reservations", s.requestReservation)
	s.mux.HandleFunc("POST /v1/reservations/{id}/approve", s.approveReservation)
	s.mux.HandleFunc("POST /v1/reservations/{id}/activate", s.activateReservation)
	s.mux.HandleFunc("POST /v1/reservations/{id}/cancel", s.cancelReservation)
	s.mux.HandleFunc("POST /v1/reservations/{id}/release", s.releaseReservation)
	s.mux.HandleFunc("POST /v1/jobs", s.submitJob)
	s.mux.HandleFunc("GET /v1/jobs/{id}", s.getJob)
	s.mux.HandleFunc("POST /v1/jobs/{id}/cancel", s.cancelJob)
	s.mux.HandleFunc("POST /v1/telemetry", s.recordTelemetry)
	s.mux.HandleFunc("POST /v1/carbon/reports", s.generateCarbon)
	s.mux.HandleFunc("POST /v1/carbon/reports/{id}/approve", s.approveCarbon)
	s.mux.HandleFunc("POST /v1/artifacts", s.createArtifact)
	s.mux.HandleFunc("POST /v1/artifact-versions/{id}/scan", s.scanArtifact)
	s.mux.HandleFunc("POST /v1/artifacts/{id}/promote", s.promoteArtifact)
	s.mux.HandleFunc("POST /v1/artifact-versions/{id}/retire", s.retireArtifact)
}

func (s *Server) livez(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "alive"})
}
func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	if err := s.deps.Store.Ping(r.Context()); err != nil {
		fail(w, r, domain.ErrUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

type tenantRequest struct {
	Name string `json:"name"`
}

func (s *Server) createTenant(w http.ResponseWriter, r *http.Request) {
	var in tenantRequest
	if !decode(w, r, &in) {
		return
	}
	id, err := s.deps.Identity.CreateTenant(r.Context(), in.Name)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

type userRequest struct {
	TenantID, Email, DisplayName, Password string
	Role                                   domain.Role `json:"role"`
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TenantID    string      `json:"tenant_id"`
		Email       string      `json:"email"`
		DisplayName string      `json:"display_name"`
		Password    string      `json:"password"`
		Role        domain.Role `json:"role"`
	}
	if !decode(w, r, &in) {
		return
	}
	u, err := s.deps.Identity.CreateUser(r.Context(), in.TenantID, in.Email, in.DisplayName, in.Password, in.Role)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, u)
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Password string }
	if !decode(w, r, &in) {
		return
	}
	session, user, err := s.deps.Identity.Authenticate(r.Context(), in.Email, in.Password)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": session.TokenHash, "expires_at": session.ExpiresAt, "user": user})
}
func (s *Server) revoke(w http.ResponseWriter, r *http.Request) {
	user, session, err := s.requireUser(r)
	if err != nil {
		fail(w, r, err)
		return
	}
	if err := s.deps.Identity.RevokeSession(r.Context(), session.ID, user.ID); err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) requireUser(r *http.Request) (domain.User, domain.Session, error) {
	raw := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if raw == "" {
		return domain.User{}, domain.Session{}, domain.ErrForbidden
	}
	u, sess, err := s.deps.Identity.AuthenticateToken(r.Context(), raw)
	return u, sess, err
}
func (s *Server) requireRole(r *http.Request, roles ...domain.Role) (domain.User, error) {
	u, _, err := s.requireUser(r)
	if err != nil {
		return domain.User{}, err
	}
	for _, role := range roles {
		if u.Role == role {
			return u, nil
		}
	}
	return domain.User{}, domain.ErrForbidden
}

func (s *Server) createCluster(w http.ResponseWriter, r *http.Request) {
	u, err := s.requireRole(r, domain.RolePlatformAdmin, domain.RoleClusterOps)
	if err != nil {
		fail(w, r, err)
		return
	}
	var in struct {
		Name     string `json:"name"`
		Region   string `json:"region"`
		Capacity int    `json:"capacity_gpu"`
	}
	if !decode(w, r, &in) {
		return
	}
	c, err := s.deps.Cluster.CreateCluster(r.Context(), u.TenantID, in.Name, in.Region, in.Capacity)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}
func (s *Server) addNode(w http.ResponseWriter, r *http.Request) {
	u, err := s.requireRole(r, domain.RolePlatformAdmin, domain.RoleClusterOps)
	if err != nil {
		fail(w, r, err)
		return
	}
	var in struct {
		Name     string `json:"name"`
		Capacity int    `json:"gpu_capacity"`
	}
	if !decode(w, r, &in) {
		return
	}
	n, err := s.deps.Cluster.AddNode(r.Context(), u.ID, u.TenantID, r.PathValue("id"), in.Name, in.Capacity, middleware.RequestIDFrom(r.Context()))
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

func (s *Server) requestReservation(w http.ResponseWriter, r *http.Request) {
	u, err := s.requireRole(r, domain.RoleTenantAdmin, domain.RoleScheduler)
	if err != nil {
		fail(w, r, err)
		return
	}
	var in struct {
		ClusterID string    `json:"cluster_id"`
		GPU       int       `json:"gpu_count"`
		StartsAt  time.Time `json:"starts_at"`
		EndsAt    time.Time `json:"ends_at"`
	}
	if !decode(w, r, &in) {
		return
	}
	v, err := s.deps.Reservation.Request(r.Context(), u.TenantID, u.ID, in.ClusterID, in.GPU, in.StartsAt, in.EndsAt, middleware.RequestIDFrom(r.Context()))
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}
func (s *Server) approveReservation(w http.ResponseWriter, r *http.Request) {
	u, err := s.requireRole(r, domain.RoleTenantAdmin, domain.RolePlatformAdmin)
	if err != nil {
		fail(w, r, err)
		return
	}
	var in struct {
		Version int64 `json:"version"`
	}
	if !decode(w, r, &in) {
		return
	}
	err = s.deps.Reservation.Approve(r.Context(), u.TenantID, u.ID, r.PathValue("id"), in.Version, middleware.RequestIDFrom(r.Context()))
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
func (s *Server) activateReservation(w http.ResponseWriter, r *http.Request) {
	u, err := s.requireRole(r, domain.RoleTenantAdmin, domain.RoleScheduler)
	if err != nil {
		fail(w, r, err)
		return
	}
	var in struct {
		Version int64 `json:"version"`
	}
	if !decode(w, r, &in) {
		return
	}
	err = s.deps.Reservation.Activate(r.Context(), u.TenantID, r.PathValue("id"), in.Version, middleware.RequestIDFrom(r.Context()))
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
func (s *Server) cancelReservation(w http.ResponseWriter, r *http.Request) {
	u, err := s.requireRole(r, domain.RoleTenantAdmin, domain.RoleScheduler)
	if err != nil {
		fail(w, r, err)
		return
	}
	var in struct {
		Version int64 `json:"version"`
	}
	if !decode(w, r, &in) {
		return
	}
	err = s.deps.Reservation.Cancel(r.Context(), u.TenantID, u.ID, r.PathValue("id"), in.Version, middleware.RequestIDFrom(r.Context()))
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
func (s *Server) releaseReservation(w http.ResponseWriter, r *http.Request) {
	u, err := s.requireRole(r, domain.RoleTenantAdmin, domain.RoleScheduler)
	if err != nil {
		fail(w, r, err)
		return
	}
	var in struct {
		Version int64 `json:"version"`
	}
	if !decode(w, r, &in) {
		return
	}
	err = s.deps.Reservation.Release(r.Context(), u.TenantID, u.ID, r.PathValue("id"), in.Version, middleware.RequestIDFrom(r.Context()))
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) submitJob(w http.ResponseWriter, r *http.Request) {
	u, err := s.requireRole(r, domain.RoleTenantAdmin, domain.RoleScheduler)
	if err != nil {
		fail(w, r, err)
		return
	}
	var in struct {
		ReservationID     string `json:"reservation_id"`
		ArtifactVersionID string `json:"artifact_version_id"`
		Name              string `json:"name"`
		GPU               int    `json:"gpu_count"`
	}
	if !decode(w, r, &in) {
		return
	}
	j, err := s.deps.Job.Submit(r.Context(), u.TenantID, in.ReservationID, in.ArtifactVersionID, in.Name, in.GPU, u.ID, middleware.RequestIDFrom(r.Context()))
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, j)
}
func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	u, _, err := s.requireUser(r)
	if err != nil {
		fail(w, r, err)
		return
	}
	j, err := s.deps.Job.Get(r.Context(), u.TenantID, r.PathValue("id"))
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, j)
}
func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request) {
	u, err := s.requireRole(r, domain.RoleTenantAdmin, domain.RoleScheduler)
	if err != nil {
		fail(w, r, err)
		return
	}
	var in struct {
		Version int64 `json:"version"`
	}
	if !decode(w, r, &in) {
		return
	}
	err = s.deps.Job.Cancel(r.Context(), u.TenantID, u.ID, r.PathValue("id"), in.Version, middleware.RequestIDFrom(r.Context()))
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) recordTelemetry(w http.ResponseWriter, r *http.Request) {
	u, err := s.requireRole(r, domain.RoleClusterOps, domain.RolePlatformAdmin)
	if err != nil {
		fail(w, r, err)
		return
	}
	var in struct {
		NodeID         string    `json:"node_id"`
		Sequence       int64     `json:"sequence"`
		MeasuredAt     time.Time `json:"measured_at"`
		PowerWatts     float64   `json:"power_watts"`
		RenewableShare float64   `json:"renewable_share"`
	}
	if !decode(w, r, &in) {
		return
	}
	v, err := s.deps.Telemetry.Record(r.Context(), u.TenantID, in.NodeID, in.Sequence, in.MeasuredAt, in.PowerWatts, in.RenewableShare, middleware.RequestIDFrom(r.Context()))
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}
func (s *Server) generateCarbon(w http.ResponseWriter, r *http.Request) {
	u, err := s.requireRole(r, domain.RoleClusterOps, domain.RoleAuditor, domain.RolePlatformAdmin)
	if err != nil {
		fail(w, r, err)
		return
	}
	var in struct {
		ClusterID string    `json:"cluster_id"`
		From      time.Time `json:"from"`
		To        time.Time `json:"to"`
	}
	if !decode(w, r, &in) {
		return
	}
	v, err := s.deps.Carbon.Generate(r.Context(), u.TenantID, in.ClusterID, in.From, in.To, u.ID, middleware.RequestIDFrom(r.Context()))
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}
func (s *Server) approveCarbon(w http.ResponseWriter, r *http.Request) {
	u, err := s.requireRole(r, domain.RoleAuditor, domain.RolePlatformAdmin)
	if err != nil {
		fail(w, r, err)
		return
	}
	var in struct {
		Version int64 `json:"version"`
	}
	if !decode(w, r, &in) {
		return
	}
	err = s.deps.Carbon.Approve(r.Context(), u.TenantID, u.ID, r.PathValue("id"), in.Version, middleware.RequestIDFrom(r.Context()))
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func (s *Server) createArtifact(w http.ResponseWriter, r *http.Request) {
	u, err := s.requireRole(r, domain.RoleTenantAdmin, domain.RoleScheduler)
	if err != nil {
		fail(w, r, err)
		return
	}
	var in struct {
		Name   string `json:"name"`
		Digest string `json:"digest"`
		Size   int64  `json:"size_bytes"`
	}
	if !decode(w, r, &in) {
		return
	}
	a, v, err := s.deps.Artifact.Create(r.Context(), u.TenantID, in.Name, in.Digest, in.Size, u.ID, middleware.RequestIDFrom(r.Context()))
	if err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"artifact": a, "version": v})
}
func (s *Server) scanArtifact(w http.ResponseWriter, r *http.Request) {
	u, err := s.requireRole(r, domain.RoleTenantAdmin, domain.RoleClusterOps)
	if err != nil {
		fail(w, r, err)
		return
	}
	var in struct {
		Passed bool `json:"passed"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.deps.Artifact.Scan(r.Context(), u.TenantID, r.PathValue("id"), in.Passed, u.ID, middleware.RequestIDFrom(r.Context())); err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
func (s *Server) promoteArtifact(w http.ResponseWriter, r *http.Request) {
	u, err := s.requireRole(r, domain.RoleTenantAdmin, domain.RolePlatformAdmin)
	if err != nil {
		fail(w, r, err)
		return
	}
	var in struct {
		VersionID string `json:"version_id"`
		Version   int64  `json:"version"`
	}
	if !decode(w, r, &in) {
		return
	}
	if err := s.deps.Artifact.Promote(r.Context(), u.TenantID, u.ID, r.PathValue("id"), in.VersionID, in.Version, middleware.RequestIDFrom(r.Context())); err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}
func (s *Server) retireArtifact(w http.ResponseWriter, r *http.Request) {
	u, err := s.requireRole(r, domain.RoleTenantAdmin, domain.RolePlatformAdmin)
	if err != nil {
		fail(w, r, err)
		return
	}
	if err := s.deps.Artifact.Retire(r.Context(), u.TenantID, u.ID, r.PathValue("id"), middleware.RequestIDFrom(r.Context())); err != nil {
		fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusNoContent, nil)
}

func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		fail(w, r, errors.Join(domain.ErrInvalid, err))
		return false
	}
	return true
}
