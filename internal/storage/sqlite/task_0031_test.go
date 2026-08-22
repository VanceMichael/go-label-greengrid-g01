package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/VanceMichael/greengrid/internal/storage/sqlite"
)

func TestGreenGridTask0031(t *testing.T) {
	store, err := sqlite.Open(t.TempDir() + "/cancelled-migration.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = store.Migrate(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled migration error = %v", err)
	}

	var tableName string
	queryErr := store.DB().QueryRowContext(
		context.Background(),
		"SELECT name FROM sqlite_master WHERE type='table' AND name='schema_migrations'",
	).Scan(&tableName)
	if !errors.Is(queryErr, sql.ErrNoRows) {
		t.Fatalf("cancelled migration persisted schema table %q (err=%v)", tableName, queryErr)
	}
}
