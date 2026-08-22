package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/httpapi"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

type httpFixture struct {
	services              testsupport.Services
	server                *httptest.Server
	tenant                string
	admin, scheduler, ops domain.User
}

func newHTTPFixture(t *testing.T) httpFixture {
	t.Helper()
	s, err := testsupport.Open(t.TempDir() + "/http.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Store.Close() })
	ctx := context.Background()
	tenant, _ := s.Identity.CreateTenant(ctx, "http-tenant")
	admin, _ := s.Identity.CreateUser(ctx, tenant, "admin@example.com", "Admin", "secret", domain.RoleTenantAdmin)
	scheduler, _ := s.Identity.CreateUser(ctx, tenant, "scheduler@example.com", "Scheduler", "secret", domain.RoleScheduler)
	ops, _ := s.Identity.CreateUser(ctx, tenant, "ops@example.com", "Ops", "secret", domain.RoleClusterOps)
	h := httpapi.New(httpapi.Dependencies{Store: s.Store, Identity: s.Identity, Cluster: s.Cluster, Reservation: s.Reservation, Job: s.Job, Telemetry: s.Telemetry, Carbon: s.Carbon, Artifact: s.Artifact}).Handler()
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return httpFixture{services: s, server: ts, tenant: tenant, admin: admin, scheduler: scheduler, ops: ops}
}

func requestJSON(t *testing.T, base, method, path, token string, input any) (int, http.Header, map[string]any) {
	t.Helper()
	body := bytes.NewReader(nil)
	if input != nil {
		raw, _ := json.Marshal(input)
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, base+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("X-Request-ID", "request-http-1")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, res.Header, out
}
func loginHTTP(t *testing.T, f httpFixture, email string) string {
	status, _, body := requestJSON(t, f.server.URL, "POST", "/v1/sessions", "", map[string]string{"email": email, "password": "secret"})
	if status != http.StatusOK {
		t.Fatalf("login status=%d body=%v", status, body)
	}
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatalf("missing token: %v", body)
	}
	return token
}

func TestHealthAndRequestID(t *testing.T) {
	f := newHTTPFixture(t)
	status, h, body := requestJSON(t, f.server.URL, "GET", "/livez", "", nil)
	if status != http.StatusOK || body["status"] != "alive" {
		t.Fatalf("livez status=%d body=%v", status, body)
	}
	if h.Get("X-Request-ID") != "request-http-1" {
		t.Fatal(h)
	}
	status, _, body = requestJSON(t, f.server.URL, "GET", "/readyz", "", nil)
	if status != http.StatusOK || body["status"] != "ready" {
		t.Fatalf("readyz status=%d body=%v", status, body)
	}
}

func TestLoginRolesAndRevocation(t *testing.T) {
	f := newHTTPFixture(t)
	token := loginHTTP(t, f, "scheduler@example.com")
	status, _, body := requestJSON(t, f.server.URL, "GET", "/v1/jobs/missing", token, nil)
	if status != http.StatusNotFound || body["error"] == nil {
		t.Fatalf("missing job status=%d body=%v", status, body)
	}
	status, _, _ = requestJSON(t, f.server.URL, "DELETE", "/v1/sessions", token, nil)
	if status != http.StatusNoContent {
		t.Fatalf("revoke status=%d", status)
	}
	status, _, _ = requestJSON(t, f.server.URL, "GET", "/v1/jobs/missing", token, nil)
	if status != http.StatusForbidden {
		t.Fatalf("revoked status=%d", status)
	}
}

func TestHTTPReservationAndJobFlow(t *testing.T) {
	f := newHTTPFixture(t)
	ctx := context.Background()
	token := loginHTTP(t, f, "scheduler@example.com")
	cluster, err := f.services.Cluster.CreateCluster(ctx, f.tenant, "cluster", "region", 8)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	status, _, body := requestJSON(t, f.server.URL, "POST", "/v1/reservations", token, map[string]any{"cluster_id": cluster.ID, "gpu_count": 2, "starts_at": now, "ends_at": now.Add(time.Hour)})
	if status != http.StatusCreated {
		t.Fatalf("request status=%d body=%v", status, body)
	}
	reservationID, _ := body["ID"].(string)
	if reservationID == "" {
		reservationID, _ = body["id"].(string)
	}
	if reservationID == "" {
		t.Fatalf("reservation body=%v", body)
	}
	status, _, body = requestJSON(t, f.server.URL, "POST", "/v1/reservations/"+reservationID+"/approve", loginHTTP(t, f, "admin@example.com"), map[string]int64{"version": 1})
	if status != http.StatusNoContent {
		t.Fatalf("approve status=%d body=%v", status, body)
	}
	status, _, body = requestJSON(t, f.server.URL, "POST", "/v1/reservations/"+reservationID+"/activate", token, map[string]int64{"version": 2})
	if status != http.StatusNoContent {
		t.Fatalf("activate status=%d body=%v", status, body)
	}
	status, _, body = requestJSON(t, f.server.URL, "POST", "/v1/jobs", token, map[string]any{"reservation_id": reservationID, "name": "train", "gpu_count": 1})
	if status != http.StatusCreated {
		t.Fatalf("job status=%d body=%v", status, body)
	}
}

func TestHTTPRejectsUnknownFieldsAndUnauthorized(t *testing.T) {
	f := newHTTPFixture(t)
	status, _, body := requestJSON(t, f.server.URL, "POST", "/v1/sessions", "", map[string]any{"email": "scheduler@example.com", "password": "secret", "private_answer": "leak"})
	if status != http.StatusBadRequest || body["error"] == nil {
		t.Fatalf("unknown field status=%d body=%v", status, body)
	}
	status, _, body = requestJSON(t, f.server.URL, "POST", "/v1/clusters", "", map[string]any{"name": "x", "region": "r", "capacity_gpu": 1})
	if status != http.StatusForbidden || body["error"] == nil {
		t.Fatalf("unauthorized status=%d body=%v", status, body)
	}
}
