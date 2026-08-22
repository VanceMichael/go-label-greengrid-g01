package domain

import (
	"fmt"
	"strings"
)

func ValidateTenantName(value string) error {
	value = strings.TrimSpace(value)
	if len(value) < 2 || len(value) > 120 {
		return fmt.Errorf("%w: tenant name length", ErrInvalid)
	}
	if strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%w: tenant name characters", ErrInvalid)
	}
	return nil
}

func ValidateDigest(value string) error {
	value = strings.TrimSpace(value)
	if len(value) < 16 || !strings.Contains(value, "-") {
		return fmt.Errorf("%w: artifact digest", ErrInvalid)
	}
	return nil
}

func ValidateRenewableShare(value float64) error {
	if value < 0 || value > 1 {
		return fmt.Errorf("%w: renewable share", ErrInvalid)
	}
	return nil
}

func ValidatePower(value float64) error {
	if value < 0 || value > 10_000_000 {
		return fmt.Errorf("%w: power reading", ErrInvalid)
	}
	return nil
}

func ValidateAttempt(attempt, max int) error {
	if attempt < 0 || max < 1 || attempt > max {
		return fmt.Errorf("%w: attempt boundary", ErrInvalid)
	}
	return nil
}

func NormalizeStatus(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func IsTerminalStatus(value string) bool {
	switch NormalizeStatus(value) {
	case "succeeded", "failed", "cancelled", "released", "retired", "sent":
		return true
	default:
		return false
	}
}
