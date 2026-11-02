package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"kairos/server/internal/store"
)

func postJSONRaw(h http.Handler, token, path string, body any) (int, []byte) {
	body = withCommitAck(path, body)
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

// TestConcurrentCommitSamePreview fires two different idempotency keys at the
// same preview at once: exactly one commit may be applied, the other must get
// the same receipt (or a clean 409), never a second set of legacy courses.
func TestConcurrentCommitSamePreview(t *testing.T) {
	h, q, conn, _, _ := academicTestEnv(t)
	token := loginToken(t, h)
	ctx := context.Background()
	mustSettings(t, h, token, 0)

	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d", rec.Code)
	}
	preview := decodePreview(t, rec.Body.Bytes())

	type result struct {
		code int
		body []byte
	}
	results := make([]result, 2)
	var wg sync.WaitGroup
	for i, key := range []string{testKey("concurrent-a"), testKey("concurrent-b")} {
		wg.Add(1)
		go func(i int, key string) {
			defer wg.Done()
			code, body := postJSONRaw(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
				"idempotency_key":             key,
				"expected_aggregate_revision": 1,
			})
			results[i] = result{code, body}
		}(i, key)
	}
	wg.Wait()

	var commitIDs []string
	for _, r := range results {
		switch r.code {
		case http.StatusOK:
			var receipt commitReceipt
			_ = json.Unmarshal(r.body, &receipt)
			commitIDs = append(commitIDs, receipt.CommitID)
			if receipt.Counts.SessionsCreated != 2 {
				t.Fatalf("unexpected receipt counts: %+v", receipt.Counts)
			}
		case http.StatusConflict:
			// Acceptable: the loser saw an in-flight/committed preview.
		default:
			t.Fatalf("unexpected status %d body=%s", r.code, string(r.body))
		}
	}
	if len(commitIDs) == 0 {
		t.Fatal("neither commit succeeded")
	}
	for _, id := range commitIDs[1:] {
		if id != commitIDs[0] {
			t.Fatalf("two different commits applied: %v", commitIDs)
		}
	}

	legacy, err := q.ListCourses(ctx, store.TextOf("2026S2"))
	if err != nil {
		t.Fatalf("list legacy: %v", err)
	}
	if len(legacy) != 2 {
		t.Fatalf("legacy courses = %d, want 2 (duplicate commit?)", len(legacy))
	}
	var batches int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM academic_import_batches").Scan(&batches); err != nil {
		t.Fatalf("count batches: %v", err)
	}
	if batches != 1 {
		t.Fatalf("batches = %d, want 1", batches)
	}
}

// TestCommitRollbackOnProjectionFailure injects a trigger that fails the second
// legacy course insert and verifies the whole commit (source rows, projection,
// versions, batch, idempotency record) is rolled back as one unit.
func TestCommitRollbackOnProjectionFailure(t *testing.T) {
	h, q, conn, _, _ := academicTestEnv(t)
	token := loginToken(t, h)
	ctx := context.Background()
	mustSettings(t, h, token, 0)

	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d", rec.Code)
	}
	preview := decodePreview(t, rec.Body.Bytes())

	if _, err := conn.Exec(ctx, `CREATE OR REPLACE FUNCTION fail_course() RETURNS trigger LANGUAGE plpgsql AS 'BEGIN IF NEW.name = ''合成课程乙'' THEN RAISE EXCEPTION ''injected projection failure''; END IF; RETURN NEW; END'`); err != nil {
		t.Fatalf("create trigger function: %v", err)
	}
	if _, err := conn.Exec(ctx, `CREATE TRIGGER fail_course_trg BEFORE INSERT ON courses FOR EACH ROW EXECUTE FUNCTION fail_course()`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	body := map[string]any{"idempotency_key": testKey("rollback-1"), "expected_aggregate_revision": 1}
	code, respBody := postJSONRaw(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", body)
	if code != http.StatusInternalServerError {
		t.Fatalf("commit = %d body=%s", code, string(respBody))
	}

	for table, want := range map[string]int{"courses": 0, "academic_courses": 0, "academic_sessions": 0, "academic_import_batches": 0, "academic_commit_requests": 0} {
		var n int
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if n != want {
			t.Fatalf("%s = %d, want %d after rollback", table, n, want)
		}
	}
	var committed *string
	if err := conn.QueryRow(ctx, "SELECT committed_at::text FROM academic_previews WHERE id = $1::uuid", preview.PreviewID).Scan(&committed); err != nil {
		t.Fatalf("preview committed_at: %v", err)
	}
	if committed != nil {
		t.Fatal("preview marked committed despite rollback")
	}
	versions, err := q.GetAcademicVersions(ctx, "kairos", "2026S2")
	if err != nil {
		t.Fatalf("versions: %v", err)
	}
	if versions.AggregateRevision != 1 {
		t.Fatalf("aggregate = %d, want 1 (no bump on rollback)", versions.AggregateRevision)
	}

	// After removing the trigger the same key must still be usable.
	if _, err := conn.Exec(ctx, "DROP TRIGGER fail_course_trg ON courses"); err != nil {
		t.Fatalf("drop trigger: %v", err)
	}
	code, respBody = postJSONRaw(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", body)
	if code != http.StatusOK {
		t.Fatalf("retry commit = %d body=%s", code, string(respBody))
	}
}

// TestCommittedRawFileRemoved pins the retention rule: committing must not keep
// the uploaded file around past its usefulness.
func TestCommittedRawFileRemoved(t *testing.T) {
	h, _, conn, _, _ := academicTestEnv(t)
	token := loginToken(t, h)
	ctx := context.Background()
	mustSettings(t, h, token, 0)

	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d", rec.Code)
	}
	preview := decodePreview(t, rec.Body.Bytes())

	var rawPath *string
	if err := conn.QueryRow(ctx, "SELECT raw_path FROM academic_previews WHERE id = $1::uuid", preview.PreviewID).Scan(&rawPath); err != nil {
		t.Fatalf("raw path: %v", err)
	}
	if rawPath == nil {
		t.Fatal("preview has no raw file before commit")
	}

	code, respBody := postJSONRaw(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("raw-1"),
		"expected_aggregate_revision": 1,
	})
	if code != http.StatusOK {
		t.Fatalf("commit = %d body=%s", code, string(respBody))
	}

	var after *string
	if err := conn.QueryRow(ctx, "SELECT raw_path FROM academic_previews WHERE id = $1::uuid", preview.PreviewID).Scan(&after); err != nil {
		t.Fatalf("raw path after: %v", err)
	}
	if after != nil {
		t.Fatalf("raw_path still set: %s", *after)
	}
	if _, err := os.Stat(*rawPath); err == nil {
		t.Fatalf("raw file still exists: %s", *rawPath)
	}

	// The committed preview stays readable.
	rec = doJSON(t, h, http.MethodGet, "/api/academic/henu/previews/"+preview.PreviewID, nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("get committed preview = %d", rec.Code)
	}
}

// TestCalendarWeekUsesCommitProjection checks the old calendar reader sees the
// committed timetable with the configured period times.
func TestCalendarWeekUsesCommitProjection(t *testing.T) {
	h, _, _, _, _ := academicTestEnv(t)
	token := loginToken(t, h)
	mustSettings(t, h, token, 0)

	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d", rec.Code)
	}
	preview := decodePreview(t, rec.Body.Bytes())
	code, body := postJSONRaw(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("calendar-1"),
		"expected_aggregate_revision": 1,
	})
	if code != http.StatusOK {
		t.Fatalf("commit = %d body=%s", code, string(body))
	}

	rec = doJSON(t, h, http.MethodGet, "/api/calendar/week?semester=2026S2&week_index=1", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("calendar week = %d body=%s", rec.Code, rec.Body.String())
	}
	payload := rec.Body.String()
	if !strings.Contains(payload, "合成课程甲") {
		t.Fatalf("calendar missing committed course: %s", payload)
	}
	if !strings.Contains(payload, "16:20") {
		t.Fatalf("calendar missing configured start time: %s", payload)
	}
	if strings.Contains(payload, "合成课程乙") {
		t.Fatalf("calendar shows course outside week 1: %s", payload)
	}
}
