package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/VanceMichael/greengrid/internal/middleware"
)

func TestRequestIDPreservesOrCreatesID(t *testing.T) {
	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if middleware.RequestIDFrom(r.Context()) == "" {
			t.Error("missing context id")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "known")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Header().Get("X-Request-ID") != "known" {
		t.Fatal(res.Header())
	}
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/", nil))
	if res.Header().Get("X-Request-ID") == "" {
		t.Fatal("generated id missing")
	}
}

func TestRequestIDDoesNotTrustEmptyHeader(t *testing.T) {
	handler := middleware.RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", " ")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Header().Get("X-Request-ID") == " " || res.Header().Get("X-Request-ID") == "" {
		t.Fatal("bad generated id")
	}
}
