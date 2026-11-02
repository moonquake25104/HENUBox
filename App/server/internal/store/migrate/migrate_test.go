package migrate

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

func testConn(t *testing.T) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), "postgres://kairos:kairos_dev@localhost:5432/kairos_dev")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { conn.Close(context.Background()) })
	return conn
}

func TestUpCreatesAllTablesAndSeeds(t *testing.T) {
	conn := testConn(t)
	ctx := context.Background()

	if _, err := conn.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS migrate_test"); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := conn.Exec(ctx, "DROP SCHEMA IF EXISTS migrate_test CASCADE"); err != nil {
		t.Fatalf("drop schema: %v", err)
	}
	if _, err := conn.Exec(ctx, "CREATE SCHEMA migrate_test"); err != nil {
		t.Fatalf("recreate schema: %v", err)
	}
	if _, err := conn.Exec(ctx, "SET search_path TO migrate_test"); err != nil {
		t.Fatalf("set search_path: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), "DROP SCHEMA IF EXISTS migrate_test CASCADE")
	})

	if err := Up(ctx, conn); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	var tableCount int
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM information_schema.tables
		WHERE table_schema = 'migrate_test' AND table_type = 'BASE TABLE'
	`).Scan(&tableCount); err != nil {
		t.Fatalf("count tables: %v", err)
	}
	if tableCount != 14 {
		t.Fatalf("expected 14 tables (13 + schema_migrations), got %d", tableCount)
	}

	var migrationCount int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&migrationCount); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if migrationCount != 7 {
		t.Fatalf("expected 7 migration records, got %d", migrationCount)
	}

	var profiles int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM pomodoro_profiles WHERE is_builtin").Scan(&profiles); err != nil {
		t.Fatalf("count builtin profiles: %v", err)
	}
	if profiles != 3 {
		t.Fatalf("expected 3 builtin profiles, got %d", profiles)
	}

	var singletons int
	if err := conn.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM pomodoro_config)
		     + (SELECT count(*) FROM sync_config)
		     + (SELECT count(*) FROM notification_config)
		     + (SELECT count(*) FROM ai_config)
	`).Scan(&singletons); err != nil {
		t.Fatalf("count singletons: %v", err)
	}
	if singletons != 4 {
		t.Fatalf("expected 4 singleton rows, got %d", singletons)
	}
}

func TestUpIsIdempotent(t *testing.T) {
	conn := testConn(t)
	ctx := context.Background()

	if _, err := conn.Exec(ctx, "DROP SCHEMA IF EXISTS migrate_idem CASCADE"); err != nil {
		t.Fatalf("drop schema: %v", err)
	}
	if _, err := conn.Exec(ctx, "CREATE SCHEMA migrate_idem"); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := conn.Exec(ctx, "SET search_path TO migrate_idem"); err != nil {
		t.Fatalf("set search_path: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), "DROP SCHEMA IF EXISTS migrate_idem CASCADE")
	})

	if err := Up(ctx, conn); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	if err := Up(ctx, conn); err != nil {
		t.Fatalf("second apply should be a no-op: %v", err)
	}

	var migrationCount int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&migrationCount); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if migrationCount != 7 {
		t.Fatalf("expected 7 migration records after second run, got %d", migrationCount)
	}

	var profiles int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM pomodoro_profiles WHERE is_builtin").Scan(&profiles); err != nil {
		t.Fatalf("count builtin profiles: %v", err)
	}
	if profiles != 3 {
		t.Fatalf("builtin profiles must not duplicate on re-run, got %d", profiles)
	}
}
