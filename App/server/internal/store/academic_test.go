package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"kairos/server/internal/store/migrate"
)

func TestAcademicSettingsAndVersionsLifecycle(t *testing.T) {
	q, _ := newTestStore(t)
	ctx := context.Background()
	const owner, semester = "owner-a", "2026S2"

	v, err := q.GetAcademicVersions(ctx, owner, semester)
	if err != nil {
		t.Fatalf("GetAcademicVersions: %v", err)
	}
	if v != (AcademicVersions{}) {
		t.Fatalf("expected zero versions, got %+v", v)
	}

	row := AcademicSettingsRow{
		Semester:            semester,
		Revision:            1,
		TimeZone:            "Asia/Shanghai",
		WeekAnchorSource:    "USER",
		WeekAnchorConfirmed: true,
		WeekStartDay:        1,
		PeriodCount:         13,
		Periods:             []byte(`[{"index":1,"start_time":null,"end_time":null,"configured":false}]`),
	}
	anchor := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	if err := q.InsertAcademicSettings(ctx, owner, row, &anchor); err != nil {
		t.Fatalf("InsertAcademicSettings: %v", err)
	}

	got, err := q.GetAcademicSettings(ctx, owner, semester)
	if err != nil {
		t.Fatalf("GetAcademicSettings: %v", err)
	}
	if got.Revision != 1 || got.WeekAnchorDate == nil || !got.WeekAnchorConfirmed {
		t.Fatalf("unexpected row: %+v", got)
	}

	updated := row
	updated.Revision = 2
	updated.WeekAnchorConfirmed = false
	ok, err := q.UpdateAcademicSettings(ctx, owner, updated, 1, nil)
	if err != nil || !ok {
		t.Fatalf("first update ok=%v err=%v", ok, err)
	}
	ok, err = q.UpdateAcademicSettings(ctx, owner, updated, 1, nil)
	if err != nil {
		t.Fatalf("second update err=%v", err)
	}
	if ok {
		t.Fatal("stale revision must not update")
	}

	v, err = q.BumpAcademicVersions(ctx, owner, semester, 1, 0, 1)
	if err != nil {
		t.Fatalf("BumpAcademicVersions: %v", err)
	}
	if v.SettingsRevision != 1 || v.AggregateRevision != 1 || v.TimetableRevision != 0 {
		t.Fatalf("versions = %+v", v)
	}
}

func TestAcademicPreviewIsolationAndExpiry(t *testing.T) {
	q, _ := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	makePreview := func(owner, id string, expires time.Time) AcademicPreviewRow {
		return AcademicPreviewRow{
			ID: id, Owner: owner, Semester: "2026S2", Source: "henu", Acquisition: "FILE_IMPORT",
			ParserVersion: "henu-html-v1", Status: "NEEDS_CONFIGURATION",
			Payload: []byte(`{"schema_version":1}`), Summary: []byte(`{}`), Issues: []byte(`[]`),
			FileSHA256: "abc", FileBytes: 10,
			BaseSettingsRevision: 0, BaseTimetableRevision: 0, BaseAggregateRevision: 0,
			ReceivedAt: now, CreatedAt: now, ExpiresAt: expires,
		}
	}

	live := makePreview("owner-a", "11111111-1111-4111-8111-111111111111", now.Add(time.Hour))
	if err := q.InsertAcademicPreview(ctx, live); err != nil {
		t.Fatalf("insert preview: %v", err)
	}

	// Another owner must not see it.
	if _, err := q.GetAcademicPreview(ctx, "owner-b", live.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cross-owner read err = %v, want ErrNoRows", err)
	}
	if _, err := q.GetAcademicPreview(ctx, "owner-a", live.ID); err != nil {
		t.Fatalf("owner read: %v", err)
	}

	expired := makePreview("owner-a", "22222222-2222-4222-8222-222222222222", now.Add(-time.Minute))
	expired.CreatedAt = now.Add(-2 * time.Hour)
	expired.ReceivedAt = now.Add(-2 * time.Hour)
	expired.RawPath = strPtrStore("/tmp/example.bin")
	if err := q.InsertAcademicPreview(ctx, expired); err != nil {
		t.Fatalf("insert expired preview: %v", err)
	}

	list, err := q.ListExpiredPreviews(ctx, now, 10)
	if err != nil {
		t.Fatalf("ListExpiredPreviews: %v", err)
	}
	if len(list) != 1 || list[0].ID != expired.ID || list[0].RawPath == nil {
		t.Fatalf("expired list = %+v", list)
	}
	if err := q.InsertTombstone(ctx, expired.ID, "owner-a", "2026S2", now); err != nil {
		t.Fatalf("InsertTombstone: %v", err)
	}
	if err := q.EnqueuePreviewFile(ctx, expired.ID, "owner-a", "/tmp/example.bin"); err != nil {
		t.Fatalf("EnqueuePreviewFile: %v", err)
	}
	if err := q.DeleteAcademicPreview(ctx, expired.ID); err != nil {
		t.Fatalf("DeleteAcademicPreview: %v", err)
	}
	if ok, err := q.GetTombstone(ctx, "owner-a", expired.ID); err != nil || !ok {
		t.Fatalf("tombstone ok=%v err=%v", ok, err)
	}
	if ok, _ := q.GetTombstone(ctx, "owner-b", expired.ID); ok {
		t.Fatal("tombstone must be owner-scoped")
	}
	if _, err := q.GetAcademicPreview(ctx, "owner-a", expired.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expired preview still readable: %v", err)
	}

	tasks, err := q.ListDuePreviewFiles(ctx, now.Add(time.Second), 10)
	if err != nil || len(tasks) != 1 {
		t.Fatalf("file tasks = %+v err=%v", tasks, err)
	}
	if err := q.FailPreviewFile(ctx, expired.ID, "locked", now.Add(time.Minute)); err != nil {
		t.Fatalf("FailPreviewFile: %v", err)
	}
	if tasks, _ := q.ListDuePreviewFiles(ctx, now, 10); len(tasks) != 0 {
		t.Fatalf("failed task should be deferred: %+v", tasks)
	}
	if err := q.CompletePreviewFile(ctx, expired.ID); err != nil {
		t.Fatalf("CompletePreviewFile: %v", err)
	}

	n, err := q.CountCommittedBatches(ctx, "owner-a", "2026S2")
	if err != nil || n != 0 {
		t.Fatalf("committed batches = %d err=%v", n, err)
	}
}

func strPtrStore(s string) *string { return &s }

// TestTombstoneRetentionAndFileQueueIndependence pins two guarantees:
// tombstones disappear after purge_after, and a pending file deletion is not
// removed with them.
func TestTombstoneRetentionAndFileQueueIndependence(t *testing.T) {
	q, _ := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	liveID := "33333333-3333-4333-8333-333333333333"
	oldID := "44444444-4444-4444-8444-444444444444"
	if err := q.InsertTombstoneUntil(ctx, liveID, "owner-a", "2026S2", now.Add(-time.Hour), now.Add(24*time.Hour)); err != nil {
		t.Fatalf("insert live tombstone: %v", err)
	}
	if err := q.InsertTombstoneUntil(ctx, oldID, "owner-a", "2026S2", now.Add(-48*time.Hour), now.Add(-time.Minute)); err != nil {
		t.Fatalf("insert old tombstone: %v", err)
	}
	if err := q.EnqueuePreviewFile(ctx, oldID, "owner-a", "/tmp/old.bin"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	n, err := q.DeleteExpiredTombstones(ctx, now, 10)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n != 1 {
		t.Fatalf("purged = %d, want 1", n)
	}
	if ok, _ := q.GetTombstone(ctx, "owner-a", liveID); !ok {
		t.Fatal("live tombstone must survive")
	}
	if ok, _ := q.GetTombstone(ctx, "owner-a", oldID); ok {
		t.Fatal("old tombstone must be purged")
	}

	tasks, err := q.ListDuePreviewFiles(ctx, now.Add(time.Second), 10)
	if err != nil {
		t.Fatalf("list files: %v", err)
	}
	if len(tasks) != 1 || tasks[0].PreviewID != oldID {
		t.Fatalf("file queue must survive tombstone purge: %+v", tasks)
	}
}

// TestAcademicVersionLockSerialisesWrites proves the snapshot lock used by
// preview creation blocks a concurrent settings write until it commits.
func TestAcademicVersionLockSerialisesWrites(t *testing.T) {
	ctx := context.Background()
	conn1, err := pgx.Connect(ctx, testDatabaseURL())
	if err != nil {
		t.Skipf("test database unavailable: %v", err)
	}
	schema := newSchemaName()
	if _, err := conn1.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := conn1.Exec(ctx, "SET search_path TO "+schema); err != nil {
		t.Fatalf("search_path: %v", err)
	}
	if err := migrate.Up(ctx, conn1); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		_, _ = conn1.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
		_ = conn1.Close(context.Background())
	})

	conn2, err := pgx.Connect(ctx, testDatabaseURL())
	if err != nil {
		t.Fatalf("connect second: %v", err)
	}
	defer func() { _ = conn2.Close(context.Background()) }()
	if _, err := conn2.Exec(ctx, "SET search_path TO "+schema); err != nil {
		t.Fatalf("second search_path: %v", err)
	}

	q1, q2 := New(conn1), New(conn2)
	tx, err := conn1.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := q1.WithTx(tx).LockAcademicVersions(ctx, "owner", "2026S2"); err != nil {
		t.Fatalf("lock: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := q2.BumpAcademicVersions(ctx, "owner", "2026S2", 1, 0, 1)
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("concurrent bump should block while the version row is locked (err=%v)", err)
	case <-time.After(300 * time.Millisecond):
	}

	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("bump after unlock: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("bump did not resume after unlock")
	}
}
