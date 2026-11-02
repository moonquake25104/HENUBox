package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWarningAckRejectsExtraCodes(t *testing.T) {
	h, _ := academicTestHandler(t)
	token := loginToken(t, h)
	mustSettings(t, h, token, 0)

	noLocation := stringsReplaceOnce(academicFixture, " 示例楼306(120)", "")
	rec := multipartPreview(t, h, token, "2026S2", []byte(noLocation))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d", rec.Code)
	}
	preview := decodePreview(t, rec.Body.Bytes())
	code, body := postJSONRaw(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("extra-ack"), "expected_aggregate_revision": 1,
		"acknowledged_warning_codes": []string{"MISSING_LOCATION", "TIMES_REQUIRED"},
	})
	if code != http.StatusBadRequest {
		t.Fatalf("extra ack = %d %s", code, string(body))
	}
}

func TestCommitAliasBoundToOriginalRequest(t *testing.T) {
	h, _ := academicTestHandler(t)
	token := loginToken(t, h)
	mustSettings(t, h, token, 0)

	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d", rec.Code)
	}
	preview := decodePreview(t, rec.Body.Bytes())
	code, body := postJSONRaw(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("alias-key-1"), "expected_aggregate_revision": 1,
	})
	if code != http.StatusOK {
		t.Fatalf("commit = %d %s", code, string(body))
	}
	first := decodeReceipt(t, body)

	// New key replaying the same committed preview registers an alias.
	code, body = postJSONRaw(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("alias-key-2"), "expected_aggregate_revision": 1,
	})
	if code != http.StatusOK || decodeReceipt(t, body).CommitID != first.CommitID {
		t.Fatalf("alias replay = %d %s", code, string(body))
	}

	// The alias key cannot be reused for another preview/request.
	rec = multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("second preview = %d", rec.Code)
	}
	second := decodePreview(t, rec.Body.Bytes())
	code, body = postJSONRaw(h, token, "/api/academic/henu/previews/"+second.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("alias-key-2"), "expected_aggregate_revision": 2,
	})
	if code != http.StatusConflict {
		t.Fatalf("alias reuse = %d %s", code, string(body))
	}
}

func TestOverrideNoopDoesNotBump(t *testing.T) {
	h, q := academicTestHandler(t)
	token := loginToken(t, h)
	mustSettings(t, h, token, 0)

	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d", rec.Code)
	}
	preview := decodePreview(t, rec.Body.Bytes())
	code, body := postJSONRaw(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("override-noop-seed"), "expected_aggregate_revision": 1,
	})
	if code != http.StatusOK {
		t.Fatalf("commit = %d %s", code, string(body))
	}
	sessionID := sessionIDByCourseName(t, q, "合成课程甲")

	if rec := doJSON(t, h, http.MethodPatch, "/api/academic/sessions/"+sessionID+"/overrides", map[string]any{
		"expected_aggregate_revision": 2, "set": map[string]any{"location": "个人教室"},
	}, token); rec.Code != http.StatusOK {
		t.Fatalf("first override = %d", rec.Code)
	} else {
		var body struct {
			OverriddenFields []string `json:"overridden_fields"`
			LegacyCourseIDs  []int64  `json:"legacy_course_ids"`
			Revision         int64    `json:"revision"`
			Aggregate        int64    `json:"aggregate_revision"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode override: %v", err)
		}
		if len(body.OverriddenFields) != 1 || len(body.LegacyCourseIDs) == 0 || body.Revision == 0 || body.Aggregate == 0 {
			t.Fatalf("override contract fields: %+v", body)
		}
	}
	versions, _ := q.GetAcademicVersions(contextBG(), "kairos", "2026S2")

	rec = doJSON(t, h, http.MethodPatch, "/api/academic/sessions/"+sessionID+"/overrides", map[string]any{
		"expected_aggregate_revision": versions.AggregateRevision,
		"set":                         map[string]any{"location": "个人教室"},
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("second override = %d body=%s", rec.Code, rec.Body.String())
	}
	after, _ := q.GetAcademicVersions(contextBG(), "kairos", "2026S2")
	if after != versions {
		t.Fatalf("no-op override bumped versions: %+v -> %+v", versions, after)
	}
}

func TestOverrideRejectsSetAndResetSameField(t *testing.T) {
	h, q := academicTestHandler(t)
	token := loginToken(t, h)
	mustSettings(t, h, token, 0)

	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d", rec.Code)
	}
	preview := decodePreview(t, rec.Body.Bytes())
	if code, body := postJSONRaw(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("override-conflict"), "expected_aggregate_revision": 1,
	}); code != http.StatusOK {
		t.Fatalf("commit = %d %s", code, string(body))
	}
	sessionID := sessionIDByCourseName(t, q, "合成课程甲")
	rec = doJSON(t, h, http.MethodPatch, "/api/academic/sessions/"+sessionID+"/overrides", map[string]any{
		"expected_aggregate_revision": 2,
		"set":                         map[string]any{"location": "个人教室"},
		"reset_fields":                []string{"location"},
	}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("set+reset = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, http.MethodPatch, "/api/academic/sessions/"+sessionID+"/overrides", map[string]any{
		"expected_aggregate_revision": 2,
		"set":                         map[string]any{"color": "red"},
	}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad color = %d body=%s", rec.Code, rec.Body.String())
	}
}

func contextBG() context.Context { return context.Background() }

func stringsReplaceOnce(s, old, new string) string {
	return strings.Replace(s, old, new, 1)
}

// TestRetiredSessionNeedsExplicitLink: a retired arrangement reappearing in the
// export must be ambiguous, then reactivate only after LINK_SESSION.
func TestRetiredSessionNeedsExplicitLink(t *testing.T) {
	h, q := academicTestHandler(t)
	token := loginToken(t, h)
	mustSettings(t, h, token, 0)

	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d", rec.Code)
	}
	preview := decodePreview(t, rec.Body.Bytes())
	if code, body := postJSONRaw(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("retire-seed-2"), "expected_aggregate_revision": 1,
	}); code != http.StatusOK {
		t.Fatalf("seed commit = %d %s", code, string(body))
	}
	sessionID := sessionIDByCourseName(t, q, "合成课程乙")

	// Retire it by submitting a file without course 乙.
	onlyA := academicFixture
	start := strings.Index(onlyA, "<tr><td>D-2")
	end := start + strings.Index(onlyA[start:], "</tr>") + len("</tr>")
	onlyA = onlyA[:start] + onlyA[end:]
	rec = multipartPreview(t, h, token, "2026S2", []byte(onlyA))
	if rec.Code != http.StatusCreated {
		t.Fatalf("retire preview = %d", rec.Code)
	}
	retirePreview := decodePreview(t, rec.Body.Bytes())
	code, body := postJSONRaw(h, token, "/api/academic/henu/previews/"+retirePreview.PreviewID+"/rebase", map[string]any{
		"expected_aggregate_revision": 2,
		"resolutions": []map[string]any{{
			"kind": "RETIRE_SESSION", "session_id": sessionID,
		}},
	})
	if code != http.StatusCreated {
		t.Fatalf("retire rebase = %d %s", code, string(body))
	}
	retired := decodePreview(t, body)
	if retired.Status != "READY" {
		t.Fatalf("retire preview status = %s", retired.Status)
	}
	code, body = postJSONRaw(h, token, "/api/academic/henu/previews/"+retired.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("retire-commit-2"), "expected_aggregate_revision": 2,
		"acknowledged_warning_codes": []string{},
	})
	if code != http.StatusOK {
		t.Fatalf("retire commit = %d %s", code, string(body))
	}

	// Re-upload full file: the retired arrangement must be ambiguous.
	rec = multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("reupload = %d", rec.Code)
	}
	again := decodePreview(t, rec.Body.Bytes())
	if again.Status != "NEEDS_REVIEW" || len(again.Diff.Ambiguous) != 1 {
		t.Fatalf("retired reappear = %s %+v", again.Status, again.Diff)
	}
	var psID string
	for _, course := range again.Courses {
		for _, s := range course.Sessions {
			for _, cand := range again.Diff.Ambiguous[0].CandidateSessionIDs {
				if cand == sessionID {
					psID = s.PreviewSessionID
				}
			}
		}
	}
	if psID == "" {
		t.Fatal("ambiguous preview session not found")
	}
	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/previews/"+again.PreviewID+"/rebase", map[string]any{
		"expected_aggregate_revision": 3,
		"resolutions": []map[string]any{{
			"kind": "LINK_SESSION", "preview_session_id": psID, "session_id": sessionID,
		}},
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("rebase = %d %s", rec.Code, rec.Body.String())
	}
	linked := decodePreview(t, rec.Body.Bytes())
	if linked.Status != "READY" {
		t.Fatalf("linked status = %s", linked.Status)
	}
}

// TestCommittedPreviewDetailsPurgedAfterRetention: batch receipt survives while
// the stored timetable payload is dropped after the window.
func TestCommittedPreviewDetailsPurgedAfterRetention(t *testing.T) {
	h, q, conn, _, _ := academicTestEnv(t)
	token := loginToken(t, h)
	ctx := context.Background()
	mustSettings(t, h, token, 0)

	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d", rec.Code)
	}
	preview := decodePreview(t, rec.Body.Bytes())
	code, body := postJSONRaw(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("purge-details"), "expected_aggregate_revision": 1,
	})
	if code != http.StatusOK {
		t.Fatalf("commit = %d %s", code, string(body))
	}
	receipt := decodeReceipt(t, body)
	if _, err := conn.Exec(ctx, "UPDATE academic_previews SET created_at = now() - interval '2 days', expires_at = now() - interval '1 day' WHERE id = $1::uuid", preview.PreviewID); err != nil {
		t.Fatalf("expire: %v", err)
	}
	if n, err := q.ClearCommittedPreviewDetails(ctx, time.Now(), 10); err != nil || n < 1 {
		t.Fatalf("purge details n=%d err=%v", n, err)
	}
	rec = doJSON(t, h, http.MethodGet, "/api/academic/henu/previews/"+preview.PreviewID, nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("get purged preview = %d", rec.Code)
	}
	var minimal struct {
		PreviewID string  `json:"preview_id"`
		Status    string  `json:"status"`
		CommitID  *string `json:"commit_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &minimal); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if minimal.PreviewID != preview.PreviewID || minimal.Status != "COMMITTED" || minimal.CommitID == nil || *minimal.CommitID != receipt.CommitID {
		t.Fatalf("minimal preview = %+v", minimal)
	}
	if strings.Contains(rec.Body.String(), "courses") {
		t.Fatalf("purged preview still exposes timetable payload")
	}
}

// postJSONStrict sends the body unchanged (no test-side ack defaulting).
func postJSONStrict(h http.Handler, token, path string, body any) (int, []byte) {
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

// TestCommitStrictContract rejects missing fields, unknown fields and non-UUID
// keys, and accepts an upper-case UUID by normalising to lower case.
func TestCommitStrictContract(t *testing.T) {
	h, _ := academicTestHandler(t)
	token := loginToken(t, h)
	mustSettings(t, h, token, 0)

	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d", rec.Code)
	}
	preview := decodePreview(t, rec.Body.Bytes())
	path := "/api/academic/henu/previews/" + preview.PreviewID + "/commit"
	key := strings.ToUpper(testKey("strict-key"))

	cases := []struct {
		name string
		body map[string]any
	}{
		{"missing-ack", map[string]any{"idempotency_key": key, "expected_aggregate_revision": 1}},
		{"removed-resolutions", map[string]any{"idempotency_key": key, "expected_aggregate_revision": 1, "acknowledged_warning_codes": []string{}, "resolutions": []any{}}},
		{"unknown-field", map[string]any{"idempotency_key": key, "expected_aggregate_revision": 1, "acknowledged_warning_codes": []string{}, "foo": "bar"}},
		{"bad-uuid", map[string]any{"idempotency_key": "not-a-uuid", "expected_aggregate_revision": 1, "acknowledged_warning_codes": []string{}}},
	}
	for _, tc := range cases {
		code, body := postJSONStrict(h, token, path, tc.body)
		if code != http.StatusBadRequest {
			t.Fatalf("%s = %d %s", tc.name, code, string(body))
		}
	}

	code, body := postJSONStrict(h, token, path, map[string]any{
		"idempotency_key": key, "expected_aggregate_revision": 1, "acknowledged_warning_codes": []string{},
	})
	if code != http.StatusOK {
		t.Fatalf("upper-case key commit = %d %s", code, string(body))
	}
	first := decodeReceipt(t, body)
	// The same UUID in lower case is the same key and replays the receipt.
	code, body = postJSONStrict(h, token, path, map[string]any{
		"idempotency_key": strings.ToLower(key), "expected_aggregate_revision": 1, "acknowledged_warning_codes": []string{},
	})
	if code != http.StatusOK || decodeReceipt(t, body).CommitID != first.CommitID {
		t.Fatalf("lower-case replay = %d %s", code, string(body))
	}
}

func TestOverrideLengthContract(t *testing.T) {
	h, q := academicTestHandler(t)
	token := loginToken(t, h)
	ctx := context.Background()
	mustSettings(t, h, token, 0)

	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d", rec.Code)
	}
	preview := decodePreview(t, rec.Body.Bytes())
	code, body := postJSONStrict(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("length-seed"), "expected_aggregate_revision": 1, "acknowledged_warning_codes": []string{},
	})
	if code != http.StatusOK {
		t.Fatalf("commit = %d %s", code, string(body))
	}
	sessionID := sessionIDByCourseName(t, q, "合成课程甲")
	path := "/api/academic/sessions/" + sessionID + "/overrides"

	patch := func(field, value string) int {
		v, _ := q.GetAcademicVersions(ctx, "kairos", "2026S2")
		rec := doJSON(t, h, http.MethodPatch, path, map[string]any{
			"expected_aggregate_revision": v.AggregateRevision,
			"set":                         map[string]any{field: value},
		}, token)
		return rec.Code
	}

	name256 := strings.Repeat("课", 256)
	name257 := strings.Repeat("课", 257)
	if code := patch("name", name256); code != http.StatusOK {
		t.Fatalf("256-rune name = %d", code)
	}
	if code := patch("name", name257); code != http.StatusBadRequest {
		t.Fatalf("257-rune name = %d, want 400", code)
	}
	teacher512 := strings.Repeat("师", 512)
	teacher513 := strings.Repeat("师", 513)
	if code := patch("teacher", teacher512); code != http.StatusOK {
		t.Fatalf("512-rune teacher = %d", code)
	}
	if code := patch("teacher", teacher513); code != http.StatusBadRequest {
		t.Fatalf("513-rune teacher = %d, want 400", code)
	}
}

// TestCommitAliasFailureRecovery: alias persistence failure must not be reported
// as success; after the fault is removed the same key succeeds.
func TestCommitAliasFailureRecovery(t *testing.T) {
	h, _, conn, _, _ := academicTestEnv(t)
	token := loginToken(t, h)
	ctx := context.Background()
	mustSettings(t, h, token, 0)

	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d", rec.Code)
	}
	preview := decodePreview(t, rec.Body.Bytes())
	code, body := postJSONStrict(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("alias-fail-seed"), "expected_aggregate_revision": 1, "acknowledged_warning_codes": []string{},
	})
	if code != http.StatusOK {
		t.Fatalf("seed commit = %d %s", code, string(body))
	}
	original := decodeReceipt(t, body)

	if _, err := conn.Exec(ctx, `CREATE OR REPLACE FUNCTION fail_alias() RETURNS trigger LANGUAGE plpgsql AS 'BEGIN RAISE EXCEPTION ''injected alias failure''; END'`); err != nil {
		t.Fatalf("function: %v", err)
	}
	if _, err := conn.Exec(ctx, `CREATE TRIGGER fail_alias_trg BEFORE INSERT ON academic_commit_requests FOR EACH ROW EXECUTE FUNCTION fail_alias()`); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	aliasKey := testKey("alias-fail-new")
	code, body = postJSONStrict(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key": aliasKey, "expected_aggregate_revision": 1, "acknowledged_warning_codes": []string{},
	})
	if code == http.StatusOK {
		t.Fatalf("alias failure reported success: %s", string(body))
	}
	var aliasRows int
	if err := conn.QueryRow(ctx, "SELECT count(*) FROM academic_commit_requests WHERE idempotency_key = $1", aliasKey).Scan(&aliasRows); err != nil {
		t.Fatalf("count alias: %v", err)
	}
	if aliasRows != 0 {
		t.Fatalf("alias rows = %d, want 0", aliasRows)
	}

	if _, err := conn.Exec(ctx, "DROP TRIGGER fail_alias_trg ON academic_commit_requests"); err != nil {
		t.Fatalf("drop trigger: %v", err)
	}
	code, body = postJSONStrict(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key": aliasKey, "expected_aggregate_revision": 1, "acknowledged_warning_codes": []string{},
	})
	if code != http.StatusOK || decodeReceipt(t, body).CommitID != original.CommitID {
		t.Fatalf("alias retry = %d %s", code, string(body))
	}
}

// TestAliasConcurrentSameRequestReplay: two simultaneous replays with the same
// alias key must both return the original receipt.
func TestAliasConcurrentSameRequestReplay(t *testing.T) {
	h, _, _, _, _ := academicTestEnv(t)
	token := loginToken(t, h)
	mustSettings(t, h, token, 0)

	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d", rec.Code)
	}
	preview := decodePreview(t, rec.Body.Bytes())
	path := "/api/academic/henu/previews/" + preview.PreviewID + "/commit"
	code, body := postJSONStrict(h, token, path, map[string]any{
		"idempotency_key": testKey("alias-race-seed"), "expected_aggregate_revision": 1, "acknowledged_warning_codes": []string{},
	})
	if code != http.StatusOK {
		t.Fatalf("seed commit = %d %s", code, string(body))
	}
	original := decodeReceipt(t, body)
	key := testKey("alias-race-new")
	results := make([]int, 2)
	bodies := make([][]byte, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], bodies[i] = postJSONStrict(h, token, path, map[string]any{
				"idempotency_key": key, "expected_aggregate_revision": 1, "acknowledged_warning_codes": []string{},
			})
		}(i)
	}
	wg.Wait()
	for i := 0; i < 2; i++ {
		if results[i] != http.StatusOK || decodeReceipt(t, bodies[i]).CommitID != original.CommitID {
			t.Fatalf("replay %d = %d %s", i, results[i], string(bodies[i]))
		}
	}
}

// TestAliasConcurrentDifferentRequests: one key raced by a replay and a fresh
// commit must resolve to one success and one explicit conflict.
func TestAliasConcurrentDifferentRequests(t *testing.T) {
	h, _, _, _, _ := academicTestEnv(t)
	token := loginToken(t, h)
	mustSettings(t, h, token, 0)

	first := decodePreview(t, multipartPreview(t, h, token, "2026S2", []byte(academicFixture)).Body.Bytes())
	second := decodePreview(t, multipartPreview(t, h, token, "2026S2", []byte(academicFixture)).Body.Bytes())
	code, body := postJSONStrict(h, token, "/api/academic/henu/previews/"+first.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("alias-diff-seed"), "expected_aggregate_revision": 1, "acknowledged_warning_codes": []string{},
	})
	if code != http.StatusOK {
		t.Fatalf("seed commit = %d %s", code, string(body))
	}

	key := testKey("alias-diff-race")
	results := make([]int, 2)
	bodies := make([][]byte, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		results[0], bodies[0] = postJSONStrict(h, token, "/api/academic/henu/previews/"+first.PreviewID+"/commit", map[string]any{
			"idempotency_key": key, "expected_aggregate_revision": 1, "acknowledged_warning_codes": []string{},
		})
	}()
	go func() {
		defer wg.Done()
		results[1], bodies[1] = postJSONStrict(h, token, "/api/academic/henu/previews/"+second.PreviewID+"/commit", map[string]any{
			"idempotency_key": key, "expected_aggregate_revision": 2, "acknowledged_warning_codes": []string{},
		})
	}()
	wg.Wait()
	successes, conflicts := 0, 0
	for _, r := range results {
		switch r {
		case http.StatusOK:
			successes++
		case http.StatusConflict:
			conflicts++
		default:
			t.Fatalf("unexpected status: %v", results)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("results = %v (bodies %s / %s)", results, string(bodies[0]), string(bodies[1]))
	}
}

// TestCommitRawDeleteFailureQueuesRetry: if the direct delete fails, a queued
// task remains for the cleaner.
func TestCommitRawDeleteFailureQueuesRetry(t *testing.T) {
	h, q, conn, _, _ := academicTestEnv(t)
	token := loginToken(t, h)
	ctx := context.Background()
	mustSettings(t, h, token, 0)

	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d", rec.Code)
	}
	preview := decodePreview(t, rec.Body.Bytes())
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "keep.bin"), []byte("x"), 0o600); err != nil {
		t.Fatalf("seed dir: %v", err)
	}
	if _, err := conn.Exec(ctx, "UPDATE academic_previews SET raw_path = $2 WHERE id = $1::uuid", preview.PreviewID, dir); err != nil {
		t.Fatalf("update raw path: %v", err)
	}
	code, body := postJSONRaw(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("raw-fail"), "expected_aggregate_revision": 1,
	})
	if code != http.StatusOK {
		t.Fatalf("commit = %d %s", code, string(body))
	}
	tasks, err := q.ListDuePreviewFiles(ctx, time.Now().Add(time.Hour), 10)
	if err != nil {
		t.Fatalf("tasks: %v", err)
	}
	found := false
	for _, task := range tasks {
		if task.PreviewID == preview.PreviewID && task.Path == dir {
			found = true
		}
	}
	if !found {
		t.Fatalf("delete failure not queued: %+v", tasks)
	}
}
