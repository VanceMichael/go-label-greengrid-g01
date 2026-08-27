package identity_test

import (
	"context"
	"errors"
	"testing"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/testsupport"
)

func TestGreenGridTask0002(t *testing.T) {
	s, err := testsupport.Open(t.TempDir() + "/task.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Store.Close()
	ctx := context.Background()
	tenantID, _ := s.Identity.CreateTenant(ctx, "cancelled-auth")
	_, _ = s.Identity.CreateUser(ctx, tenantID, "user@example.com", "User", "secret", domain.RoleScheduler)
	session, _, err := s.Identity.Authenticate(ctx, "user@example.com", "secret")
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, _, err := s.Identity.AuthenticateToken(cancelled, session.TokenHash); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled authentication error=%v", err)
	}
}
