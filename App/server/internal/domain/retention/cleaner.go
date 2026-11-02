// Package retention reclaims expired previews and their raw uploads.
//
// The queue lives in PostgreSQL so a failed file delete survives a restart.
// Only files under the configured private preview root may ever be removed;
// user-provided raw materials are never touched.
package retention

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kairos/server/internal/store"
)

// Store is the persistence surface the cleaner needs.
type Store interface {
	ListExpiredPreviews(ctx context.Context, now time.Time, limit int) ([]store.ExpiredPreview, error)
	InsertTombstone(ctx context.Context, id, owner, semester string, expiredAt time.Time) error
	DeleteAcademicPreview(ctx context.Context, id string) error
	EnqueuePreviewFile(ctx context.Context, previewID, owner, path string) error
	ListDuePreviewFiles(ctx context.Context, now time.Time, limit int) ([]store.PreviewFileTask, error)
	CompletePreviewFile(ctx context.Context, previewID string) error
	FailPreviewFile(ctx context.Context, previewID, reason string, next time.Time) error
	DeleteExpiredTombstones(ctx context.Context, now time.Time, limit int) (int64, error)
	ClearCommittedPreviewDetails(ctx context.Context, now time.Time, limit int) (int64, error)
}

// Cleaner sweeps expired previews on a timer and at startup.
type Cleaner struct {
	Q    Store
	Root string
	Log  *slog.Logger
	Now  func() time.Time
}

func (c *Cleaner) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Run performs an immediate sweep, then repeats until the context is done.
func (c *Cleaner) Run(ctx context.Context, interval time.Duration) {
	c.Sweep(ctx)
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.Sweep(ctx)
		}
	}
}

// Sweep is one pass: tombstone and delete expired previews, then retry due
// file deletions. Errors are logged per item and never abort the whole pass.
func (c *Cleaner) Sweep(ctx context.Context) {
	now := c.now()

	expired, err := c.Q.ListExpiredPreviews(ctx, now, 100)
	if err != nil {
		c.Log.Error("retention: list expired previews", "error", err)
		return
	}
	for _, p := range expired {
		if err := c.ReclaimNow(ctx, p); err != nil {
			c.Log.Error("retention: reclaim preview", "preview_id", p.ID, "error", err)
		}
	}

	tasks, err := c.Q.ListDuePreviewFiles(ctx, now, 100)
	if err != nil {
		c.Log.Error("retention: list file tasks", "error", err)
		return
	}
	for _, t := range tasks {
		if err := c.remove(t.Path); err != nil {
			next := now.Add(backoff(t.Attempts))
			if ferr := c.Q.FailPreviewFile(ctx, t.PreviewID, err.Error(), next); ferr != nil {
				c.Log.Error("retention: record failure", "preview_id", t.PreviewID, "error", ferr)
			}
			c.Log.Warn("retention: file delete failed", "preview_id", t.PreviewID, "attempts", t.Attempts+1)
			continue
		}
		if err := c.Q.CompletePreviewFile(ctx, t.PreviewID); err != nil {
			c.Log.Error("retention: complete task", "preview_id", t.PreviewID, "error", err)
		}
	}

	// Committed previews keep their receipt but not the uploaded timetable
	// details after the retention window.
	if _, err := c.Q.ClearCommittedPreviewDetails(ctx, now, 100); err != nil {
		c.Log.Error("retention: clear committed preview details", "error", err)
	}

	// Finally drop tombstones past their retention window. File tasks are a
	// separate table and are deliberately left untouched.
	for {
		n, err := c.Q.DeleteExpiredTombstones(ctx, now, 200)
		if err != nil {
			c.Log.Error("retention: purge tombstones", "error", err)
			return
		}
		if n < 200 {
			return
		}
	}
}

// ReclaimNow tombstones one expired preview, deletes its payload and tries to
// delete the raw upload immediately. A failed file delete stays queued.
func (c *Cleaner) ReclaimNow(ctx context.Context, p store.ExpiredPreview) error {
	now := c.now()
	if err := c.Q.InsertTombstone(ctx, p.ID, p.Owner, p.Semester, now); err != nil {
		return err
	}
	hasFile := p.RawPath != nil && *p.RawPath != ""
	if hasFile {
		if err := c.Q.EnqueuePreviewFile(ctx, p.ID, p.Owner, *p.RawPath); err != nil {
			return err
		}
	}
	if err := c.Q.DeleteAcademicPreview(ctx, p.ID); err != nil {
		return err
	}
	if !hasFile {
		return nil
	}
	if err := c.remove(*p.RawPath); err != nil {
		// Keep the task for the next sweep; record the reason for operators.
		if ferr := c.Q.FailPreviewFile(ctx, p.ID, err.Error(), now.Add(backoff(0))); ferr != nil {
			c.Log.Error("retention: record failure", "preview_id", p.ID, "error", ferr)
		}
		return nil
	}
	return c.Q.CompletePreviewFile(ctx, p.ID)
}

// remove deletes a file only when it resolves inside Root.
func (c *Cleaner) remove(path string) error {
	if c.Root == "" {
		return errors.New("preview root is not configured")
	}
	root, err := filepath.Abs(c.Root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}
	target, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	if target != root && !strings.HasPrefix(target, root+string(os.PathSeparator)) {
		return fmt.Errorf("path outside preview root")
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func backoff(attempts int32) time.Duration {
	d := time.Minute
	for i := int32(0); i < attempts && d < time.Hour; i++ {
		d *= 2
	}
	if d > time.Hour {
		d = time.Hour
	}
	return d
}
