package identity

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type Service struct {
	store      *sqlite.Store
	sessionTTL time.Duration
}

func NewService(store *sqlite.Store, sessionTTL time.Duration) *Service {
	return &Service{store: store, sessionTTL: sessionTTL}
}

func (s *Service) CreateTenant(ctx context.Context, name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("%w: tenant name", domain.ErrInvalid)
	}
	id := uuid.NewString()
	_, err := s.store.DB().ExecContext(ctx, `INSERT INTO tenants(id,name,status,created_at) VALUES(?,?,?,?)`, id, name, "active", time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return "", domain.ErrAlreadyExists
		}
		return "", fmt.Errorf("create tenant: %w", err)
	}
	return id, nil
}

func (s *Service) CreateUser(ctx context.Context, tenantID, email, displayName, password string, role domain.Role) (domain.User, error) {
	if tenantID == "" || email == "" || password == "" || !role.Valid() {
		return domain.User{}, fmt.Errorf("%w: user fields", domain.ErrInvalid)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return domain.User{}, fmt.Errorf("hash password: %w", err)
	}
	id := uuid.NewString()
	now := time.Now().UTC()
	_, err = s.store.DB().ExecContext(ctx, `INSERT INTO users(id,tenant_id,email,display_name,role,active,created_at,password_hash) VALUES(?,?,?,?,?,?,?,?)`, id, tenantID, strings.ToLower(strings.TrimSpace(email)), displayName, role, 1, now.Format(time.RFC3339Nano), string(hash))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return domain.User{}, domain.ErrAlreadyExists
		}
		return domain.User{}, fmt.Errorf("create user: %w", err)
	}
	return domain.User{ID: id, TenantID: tenantID, Email: email, DisplayName: displayName, Role: role, Active: true, CreatedAt: now}, nil
}

func (s *Service) Authenticate(ctx context.Context, email, password string) (domain.Session, domain.User, error) {
	var user domain.User
	var hash string
	var created string
	err := s.store.DB().QueryRowContext(ctx, `SELECT id,tenant_id,email,display_name,role,active,created_at,password_hash FROM users WHERE email=?`, strings.ToLower(strings.TrimSpace(email))).Scan(&user.ID, &user.TenantID, &user.Email, &user.DisplayName, &user.Role, &user.Active, &created, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Session{}, domain.User{}, domain.ErrForbidden
	}
	if err != nil {
		return domain.Session{}, domain.User{}, fmt.Errorf("read user: %w", err)
	}
	if !user.Active || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return domain.Session{}, domain.User{}, domain.ErrForbidden
	}
	createdAt, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return domain.Session{}, domain.User{}, fmt.Errorf("parse user time: %w", err)
	}
	token := uuid.NewString() + uuid.NewString()
	tokenHash := hashToken(token)
	session := domain.Session{ID: uuid.NewString(), UserID: user.ID, TokenHash: tokenHash, ExpiresAt: time.Now().UTC().Add(s.sessionTTL)}
	_, err = s.store.DB().ExecContext(ctx, `INSERT INTO sessions(id,user_id,token_hash,expires_at,revoked,created_at) VALUES(?,?,?,?,?,?)`, session.ID, user.ID, tokenHash, session.ExpiresAt.Format(time.RFC3339Nano), 0, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return domain.Session{}, domain.User{}, fmt.Errorf("create session: %w", err)
	}
	user.CreatedAt = createdAt
	// TokenHash is intentionally returned as an opaque bearer token to the HTTP layer;
	// the stored value remains one-way hashed.
	session.TokenHash = token
	return session, user, nil
}

func (s *Service) AuthenticateToken(ctx context.Context, token string) (domain.User, domain.Session, error) {
	if token == "" {
		return domain.User{}, domain.Session{}, domain.ErrForbidden
	}
	var user domain.User
	var session domain.Session
	var expires, created string
	var revoked, active int
	var sessionCreated string
	err := s.store.DB().QueryRowContext(ctx, `SELECT u.id,u.tenant_id,u.email,u.display_name,u.role,u.active,u.created_at,s.id,s.expires_at,s.revoked,s.created_at FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=?`, hashToken(token)).Scan(&user.ID, &user.TenantID, &user.Email, &user.DisplayName, &user.Role, &active, &created, &session.ID, &expires, &revoked, &sessionCreated)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.User{}, domain.Session{}, domain.ErrForbidden
	}
	if err != nil {
		return domain.User{}, domain.Session{}, fmt.Errorf("read session: %w", err)
	}
	if revoked != 0 || active == 0 {
		return domain.User{}, domain.Session{}, domain.ErrForbidden
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, expires)
	if err != nil || !time.Now().UTC().Before(expiresAt) {
		return domain.User{}, domain.Session{}, domain.ErrForbidden
	}
	user.Active = true
	user.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	session.UserID = user.ID
	session.ExpiresAt = expiresAt
	session.Revoked = false
	return user, session, nil
}

func (s *Service) RevokeSession(ctx context.Context, sessionID, userID string) error {
	result, err := s.store.DB().ExecContext(ctx, `UPDATE sessions SET revoked=1 WHERE id=? AND user_id=? AND revoked=0`, sessionID, userID)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Service) DeactivateUser(ctx context.Context, actorID, userID, requestID string) error {
	return s.store.WithTx(ctx, func(tx *sql.Tx) error {
		var tenantID string
		if err := tx.QueryRowContext(ctx, `SELECT tenant_id FROM users WHERE id=? AND active=1`, userID).Scan(&tenantID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return domain.ErrNotFound
			}
			return fmt.Errorf("read active user: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE users SET active=0 WHERE id=? AND active=1`, userID); err != nil {
			return fmt.Errorf("deactivate user: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `UPDATE sessions SET revoked=1 WHERE user_id=? AND revoked=0`, userID); err != nil {
			return fmt.Errorf("revoke sessions: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO audit_events(id,tenant_id,actor_id,aggregate_type,aggregate_id,action,result,request_id,details,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, uuid.NewString(), tenantID, actorID, "user", userID, "deactivate", "success", requestID, "user and sessions deactivated", time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("audit deactivation: %w", err)
		}
		return nil
	})
}

func hashToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	encoded := hex.EncodeToString(digest[:])
	return encoded
}
