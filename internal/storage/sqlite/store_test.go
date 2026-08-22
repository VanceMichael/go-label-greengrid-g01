package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/VanceMichael/greengrid/internal/domain"
	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
)

func TestMigrationIsIdempotentAndEnforcesRelations(t *testing.T) {
	path := t.TempDir() + "/store.db"
	s, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var version int
	if err := s.DB().QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 1 {
		t.Fatalf("version=%d", version)
	}
	if _, err := s.DB().Exec(`INSERT INTO users(id,tenant_id,email,display_name,role,active,created_at,password_hash) VALUES('u','missing','u@x','U','scheduler',1,'now','hash')`); err == nil {
		t.Fatal("foreign key disabled")
	}
	_ = s.Close()
}

func TestWithTxRollsBackAllWrites(t *testing.T) {
	s, err := sqlite.Open(t.TempDir() + "/rollback.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	err = s.WithTx(context.Background(), func(tx *sql.Tx) error {
		if _, err := tx.Exec(`INSERT INTO tenants(id,name,status,created_at) VALUES('t','tenant','active','now')`); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO users(id,tenant_id,email,display_name,role,active,created_at,password_hash) VALUES('u','t','u@x','U','scheduler',1,'now','hash')`); err != nil {
			return err
		}
		return errors.New("forced failure")
	})
	if err == nil {
		t.Fatal("transaction succeeded")
	}
	var count int
	_ = s.DB().QueryRow(`SELECT COUNT(*) FROM tenants`).Scan(&count)
	if count != 0 {
		t.Fatalf("rollback count=%d", count)
	}
}

func TestDatabaseReopensWithPersistentState(t *testing.T) {
	path := t.TempDir() + "/restart.db"
	s, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err = s.DB().Exec(`INSERT INTO tenants(id,name,status,created_at) VALUES('t','tenant','active','now')`)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	var name string
	if err := reopened.DB().QueryRow(`SELECT name FROM tenants WHERE id='t'`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "tenant" {
		t.Fatal(name)
	}
}

func TestDatabasePingAndClose(t *testing.T) {
	s, err := sqlite.Open(t.TempDir() + "/ping.db")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Ping(context.Background()); err == nil {
		t.Fatal("ping closed store succeeded")
	}
	_ = domain.ErrNotFound
}
