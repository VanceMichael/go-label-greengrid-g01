package reservation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/VanceMichael/greengrid/internal/reservation"
)

// TestActiveAtRespectsRequestCancellation guards the active-window read's
// context propagation: a request cancelled during the read must release the
// (single-connection) store immediately instead of letting the query run to
// completion on a detached context.
func TestActiveAtRespectsRequestCancellation(t *testing.T) {
	s, tenant, u, c := reservationFixture(t)
	r := approvedReservation(t, s, tenant, u, c)

	q := reservation.NewQuery(s.Store)
	at := r.StartsAt.Add(30 * time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := q.ActiveAt(ctx, tenant, c.ID, at); err == nil {
		t.Fatalf("ActiveAt: expected cancellation error, got nil (context not propagated into read)")
	} else if !errors.Is(err, context.Canceled) {
		t.Fatalf("ActiveAt: expected context.Canceled, got %v", err)
	}

	// The cancelled read must not hold the connection: a follow-up read on a
	// fresh context completes and returns the active reservation.
	rows, err := q.ActiveAt(context.Background(), tenant, c.ID, at)
	if err != nil {
		t.Fatalf("follow-up ActiveAt: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != r.ID {
		t.Fatalf("follow-up rows=%+v", rows)
	}
}
