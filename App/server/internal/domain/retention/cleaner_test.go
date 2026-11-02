package retention

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"kairos/server/internal/store"
)

type fakeStore struct {
	tombstoned []string
	enqueued   map[string]string
	failed     map[string]string
	completed  []string
	deleted    []string
	purgeCalls int
	purged     int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{enqueued: map[string]string{}, failed: map[string]string{}}
}

func (f *fakeStore) ListExpiredPreviews(context.Context, time.Time, int) ([]store.ExpiredPreview, error) {
	return nil, nil
}
func (f *fakeStore) InsertTombstone(_ context.Context, id, _, _ string, _ time.Time) error {
	f.tombstoned = append(f.tombstoned, id)
	return nil
}
func (f *fakeStore) DeleteAcademicPreview(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}
func (f *fakeStore) EnqueuePreviewFile(_ context.Context, id, _, path string) error {
	f.enqueued[id] = path
	return nil
}
func (f *fakeStore) ListDuePreviewFiles(context.Context, time.Time, int) ([]store.PreviewFileTask, error) {
	return nil, nil
}
func (f *fakeStore) CompletePreviewFile(_ context.Context, id string) error {
	f.completed = append(f.completed, id)
	return nil
}
func (f *fakeStore) FailPreviewFile(_ context.Context, id, reason string, _ time.Time) error {
	f.failed[id] = reason
	return nil
}
func (f *fakeStore) DeleteExpiredTombstones(_ context.Context, _ time.Time, _ int) (int64, error) {
	f.purgeCalls++
	return f.purged, nil
}

func (f *fakeStore) ClearCommittedPreviewDetails(_ context.Context, _ time.Time, _ int) (int64, error) {
	return 0, nil
}

// TestSweepPurgesTombstonesButKeepsFileTasks pins the ordering guarantee: the
// retention sweep drops old tombstones and leaves the file queue alone.
func TestSweepPurgesTombstonesButKeepsFileTasks(t *testing.T) {
	q := newFakeStore()
	q.purged = 2
	c := newCleaner(t, t.TempDir(), q)

	c.Sweep(context.Background())

	if q.purgeCalls != 1 {
		t.Fatalf("purge calls = %d, want 1", q.purgeCalls)
	}
	if len(q.completed) != 0 || len(q.deleted) != 0 {
		t.Fatalf("purge must not touch file tasks: %+v", q)
	}
}

func newCleaner(t *testing.T, root string, q Store) *Cleaner {
	t.Helper()
	return &Cleaner{Q: q, Root: root, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestReclaimNowDeletesFileInsideRoot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "preview.bin")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	q := newFakeStore()
	c := newCleaner(t, root, q)

	err := c.ReclaimNow(context.Background(), store.ExpiredPreview{
		ID: "p1", Owner: "o", Semester: "s", RawPath: &path,
	})
	if err != nil {
		t.Fatalf("ReclaimNow: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file still exists: %v", err)
	}
	if len(q.completed) != 1 || len(q.deleted) != 1 || len(q.tombstoned) != 1 {
		t.Fatalf("store calls: %+v", q)
	}
}

func TestReclaimNowRefusesFileOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "user-original.xls")
	if err := os.WriteFile(outside, []byte("x"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	q := newFakeStore()
	c := newCleaner(t, root, q)

	if err := c.ReclaimNow(context.Background(), store.ExpiredPreview{
		ID: "p2", Owner: "o", Semester: "s", RawPath: &outside,
	}); err != nil {
		t.Fatalf("ReclaimNow: %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file must not be deleted: %v", err)
	}
	if _, ok := q.failed["p2"]; !ok {
		t.Fatal("expected a deferred failure record")
	}
}

func TestReclaimNowToleratesMissingFile(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "already-gone.bin")
	q := newFakeStore()
	c := newCleaner(t, root, q)

	if err := c.ReclaimNow(context.Background(), store.ExpiredPreview{
		ID: "p3", Owner: "o", Semester: "s", RawPath: &missing,
	}); err != nil {
		t.Fatalf("ReclaimNow: %v", err)
	}
	if len(q.completed) != 1 {
		t.Fatalf("completed = %v", q.completed)
	}
}
