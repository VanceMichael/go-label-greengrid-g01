package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

type contextKey string

const requestIDKey contextKey = "request_id"
const userKey contextKey = "user"

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if id == "" {
			id = uuid.NewString()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}
func RequestIDFrom(ctx context.Context) string { v, _ := ctx.Value(requestIDKey).(string); return v }
func WithUser(ctx context.Context, user any) context.Context {
	return context.WithValue(ctx, userKey, user)
}
func UserFrom(ctx context.Context) any { return ctx.Value(userKey) }
