package middleware_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/VanceMichael/greengrid/internal/middleware"
)

func TestGreenGridTask0028(t *testing.T) {
	handler := middleware.Recovery(slog.Default(), http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("handler failure") }))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "panic-request-28")
	res := httptest.NewRecorder()
	middleware.RequestID(handler).ServeHTTP(res, req)
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", res.Code)
	}
	if res.Header().Get("X-Request-ID") != "panic-request-28" {
		t.Fatalf("response request id=%q", res.Header().Get("X-Request-ID"))
	}
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	errorBody, _ := body["error"].(map[string]any)
	if errorBody["request_id"] != "panic-request-28" {
		t.Fatalf("body=%v", body)
	}
}
