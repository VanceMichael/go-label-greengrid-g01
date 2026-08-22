package domain

import (
	"fmt"
	"strings"
	"time"
)

type TenantID string
type ClusterID string
type JobID string
type RequestID string

func (id TenantID) Valid() bool  { return validIdentifier(string(id)) }
func (id ClusterID) Valid() bool { return validIdentifier(string(id)) }
func (id JobID) Valid() bool     { return validIdentifier(string(id)) }
func (id RequestID) Valid() bool { return validIdentifier(string(id)) }

func validIdentifier(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 3 || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !(r == '-' || r == '_' || r == '.' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func ValidateGPUCount(value int) error {
	if value < 1 || value > 4096 {
		return fmt.Errorf("%w: gpu count %d", ErrInvalid, value)
	}
	return nil
}

func ValidateWindow(start, end time.Time) error {
	if start.IsZero() || end.IsZero() || !end.After(start) {
		return fmt.Errorf("%w: time window", ErrInvalid)
	}
	if end.Sub(start) > 30*24*time.Hour {
		return fmt.Errorf("%w: window exceeds 30 days", ErrInvalid)
	}
	return nil
}

func NormalizeEmail(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || !strings.Contains(value, "@") || strings.ContainsAny(value, " \t\n") {
		return "", fmt.Errorf("%w: email", ErrInvalid)
	}
	return value, nil
}

func NormalizeName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 200 {
		return "", fmt.Errorf("%w: name", ErrInvalid)
	}
	return value, nil
}
