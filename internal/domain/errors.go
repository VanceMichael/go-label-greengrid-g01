package domain

import "errors"

var (
	ErrNotFound      = errors.New("greengrid: not found")
	ErrConflict      = errors.New("greengrid: conflict")
	ErrForbidden     = errors.New("greengrid: forbidden")
	ErrInvalid       = errors.New("greengrid: invalid")
	ErrUnavailable   = errors.New("greengrid: dependency unavailable")
	ErrState         = errors.New("greengrid: invalid state transition")
	ErrCapacity      = errors.New("greengrid: insufficient capacity")
	ErrLeaseHeld     = errors.New("greengrid: lease held")
	ErrAlreadyExists = errors.New("greengrid: already exists")
	ErrCancelled     = errors.New("greengrid: cancelled")
)
