package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/middleware"
)

func statusFor(err error) int {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrCapacity), errors.Is(err, domain.ErrLeaseHeld):
		return http.StatusConflict
	case errors.Is(err, domain.ErrInvalid), errors.Is(err, domain.ErrState):
		return http.StatusBadRequest
	case errors.Is(err, domain.ErrUnavailable):
		return http.StatusServiceUnavailable
	case errors.Is(err, context.Canceled):
		return 499
	default:
		return http.StatusInternalServerError
	}
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = jsonEncode(w, value)
}
func jsonEncode(w http.ResponseWriter, value any) error { return json.NewEncoder(w).Encode(value) }
func fail(w http.ResponseWriter, r *http.Request, err error) {
	middlewareErr(w, r, statusFor(err), err)
}
func middlewareErr(w http.ResponseWriter, r *http.Request, status int, err error) {
	code := "internal_error"
	if status == http.StatusBadRequest {
		code = "invalid_request"
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		code = "forbidden"
	}
	if status == http.StatusConflict {
		code = "conflict"
	}
	if status == http.StatusNotFound {
		code = "not_found"
	}
	if status == http.StatusServiceUnavailable {
		code = "dependency_unavailable"
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": err.Error(), "request_id": middleware.RequestIDFrom(r.Context())}})
}
