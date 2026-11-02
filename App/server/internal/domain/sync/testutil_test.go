package sync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"kairos/server/internal/store"
	"kairos/server/internal/store/migrate"
)

func testDatabaseURL() string {
	if v := os.Getenv("TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://kairos:kairos_dev@localhost:5432/kairos_dev"
}

// newTestStore connects to the dev database, creates a dedicated schema,
// applies migrations into it and returns a store bound to that schema plus the
// underlying connection (used for transactions).
func newTestStore(t *testing.T) (*store.Queries, *pgx.Conn) {
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
	return store.New(conn), conn
}

func newSchemaName() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return "sync_test_" + hex.EncodeToString(b[:])
}

// importAll runs ImportAll inside a transaction, mirroring the Rust
// import_all(&mut conn, &data) test entry point.
func importAll(t *testing.T, conn *pgx.Conn, q *store.Queries, data *SyncData) (SyncStats, error) {
	t.Helper()
	ctx := context.Background()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	stats, err := ImportAll(ctx, q.WithTx(tx), data)
	if err != nil {
		return stats, err
	}
	if err := tx.Commit(ctx); err != nil {
		return stats, err
	}
	return stats, nil
}

func exportAll(t *testing.T, q *store.Queries) *SyncData {
	t.Helper()
	data, err := ExportAll(context.Background(), q)
	if err != nil {
		t.Fatalf("export all: %v", err)
	}
	return data
}

func sampleSyncData() *SyncData {
	return &SyncData{
		SchemaVersion: 2,
		DatasetID:     "dataset-test",
		DeviceID:      "device-test",
		ExportedAt:    "2024-06-01T10:00:00Z",
	}
}

func sampleTask(id int64, syncID, updatedAt string) Task {
	return Task{
		ID:          id,
		SyncID:      syncID,
		Title:       "Test Task",
		Description: "Desc",
		Status:      "todo",
		Priority:    "high",
		Tags:        "[]",
		CreatedAt:   "2024-01-01T00:00:00Z",
		UpdatedAt:   updatedAt,
	}
}

func sampleCourse(id int64, syncID, updatedAt string) Course {
	return Course{
		ID:                id,
		SyncID:            syncID,
		Name:              "Math 101",
		DayOfWeek:         1,
		StartTime:         "08:00",
		EndTime:           "09:30",
		WeekPattern:       "1-16",
		SemesterStartDate: "2026-02-24",
		Location:          "Room 101",
		Teacher:           "Prof. Smith",
		Color:             "#3B82F6",
		Semester:          "2024S1",
		CreatedAt:         "2024-01-01T00:00:00Z",
		UpdatedAt:         updatedAt,
	}
}

func sampleExam(id int64, syncID, updatedAt string) Exam {
	return Exam{
		ID:              id,
		SyncID:          syncID,
		CourseName:      "Math 101",
		ExamDatetime:    "2024-06-15T10:00:00Z",
		ExamEndDatetime: "2024-06-15T12:00:00Z",
		Location:        "Hall A",
		Notes:           "",
		CourseID:        int64Ptr(1),
		Semester:        "2024S1",
		CreatedAt:       "2024-01-01T00:00:00Z",
		UpdatedAt:       updatedAt,
	}
}

func int64Ptr(v int64) *int64 { return &v }

func strPtr(s string) *string { return &s }
