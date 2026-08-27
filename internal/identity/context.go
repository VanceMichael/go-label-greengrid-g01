package identity

// This file intentionally kept as an anchor for the identity package's
// context helpers. AuthenticateToken previously routed its session lookup
// through a wrapper that stripped cancellation (context.WithoutCancel) so
// that upstream-request cancellation never reached the SQLite driver. The
// query now uses the caller's context verbatim; see service.go.
