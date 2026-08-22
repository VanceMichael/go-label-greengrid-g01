package httpapi

import (
	"fmt"
	"github.com/VanceMichael/greengrid/internal/domain"
	"net/http"
	"strings"
	"time"
)

func requireMethod(r *http.Request, method string) error {
	if r.Method != method {
		return fmt.Errorf("%w: method %s", domain.ErrInvalid, r.Method)
	}
	return nil
}
func requireNonEmpty(values ...string) error {
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%w: empty field", domain.ErrInvalid)
		}
	}
	return nil
}
func requirePositive(values ...int) error {
	for _, value := range values {
		if value <= 0 {
			return fmt.Errorf("%w: positive value", domain.ErrInvalid)
		}
	}
	return nil
}
func requireWindow(start, end time.Time) error {
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return fmt.Errorf("%w: time window", domain.ErrInvalid)
	}
	return nil
}
func isJSON(r *http.Request) bool {
	return strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json")
}
