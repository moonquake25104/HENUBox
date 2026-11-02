package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"kairos/server/internal/store/migrate"
)

func testDatabaseURL() string {
	if v := os.Getenv("TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://kairos:kairos_dev@localhost:5432/kairos_dev"
}

// newTestStore connects to the dev database, creates a dedicated schema,
// applies migrations into it and returns a store bound to that schema.
// Every test therefore starts from a clean, isolated schema.
func newTestStore(t *testing.T) (*Queries, *pgx.Conn) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testDatabaseURL())
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}

	schema := newSchemaName()
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema %s: %v", schema, err)
	}
	if _, err := conn.Exec(ctx, "SET search_path TO "+schema); err != nil {
		t.Fatalf("set search_path to %s: %v", schema, err)
	}
	if err := migrate.Up(ctx, conn); err != nil {
		t.Fatalf("apply migrations to schema %s: %v", schema, err)
	}

	t.Cleanup(func() {
		defer conn.Close(context.Background())
		if _, err := conn.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE"); err != nil {
			t.Errorf("drop schema %s: %v", schema, err)
		}
	})
	return New(conn), conn
}

func newSchemaName() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return "store_test_" + hex.EncodeToString(b[:])
}

func dateOf(s string) pgtype.Date {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return pgtype.Date{Time: t, Valid: true}
}

func clockOf(s string) pgtype.Time {
	t, err := time.Parse("15:04", s)
	if err != nil {
		panic(err)
	}
	return pgtype.Time{Microseconds: int64((t.Hour()*3600 + t.Minute()*60)) * 1e6, Valid: true}
}

func tsOf(s string) pgtype.Timestamptz {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func pgtype_TimestamptzInvalid() pgtype.Timestamptz {
	return pgtype.Timestamptz{}
}

func textOf(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: true}
}

func int8Of(v int64) pgtype.Int8 {
	return pgtype.Int8{Int64: v, Valid: true}
}

func pgtype_Int8Invalid() pgtype.Int8 {
	return pgtype.Int8{}
}

func int4Of(v int32) pgtype.Int4 {
	return pgtype.Int4{Int32: v, Valid: true}
}

func pgtype_Int4Invalid() pgtype.Int4 {
	return pgtype.Int4{}
}

func jsonInts(t *testing.T, b []byte) []int {
	t.Helper()
	var out []int
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("decode jsonb %q: %v", b, err)
	}
	return out
}
