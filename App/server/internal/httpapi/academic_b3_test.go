package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"kairos/server/internal/store"
)

type commitReceipt struct {
	CommitID  string `json:"commit_id"`
	PreviewID string `json:"preview_id"`
	Status    string `json:"status"`
	NoChanges bool   `json:"no_changes"`
	Counts    struct {
		CoursesCreated       int `json:"courses_created"`
		CoursesUpdated       int `json:"courses_updated"`
		SessionsCreated      int `json:"sessions_created"`
		SessionsUpdated      int `json:"sessions_updated"`
		SessionsRetired      int `json:"sessions_retired"`
		LegacyCoursesCreated int `json:"legacy_courses_created"`
		LegacyCoursesUpdated int `json:"legacy_courses_updated"`
	} `json:"counts"`
}

func TestRetireMissingSessionSoftDeletesProjection(t *testing.T) {
	h, q := academicTestHandler(t)
	token := loginToken(t, h)
	ctx := context.Background()
	mustSettings(t, h, token, 0)

	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d", rec.Code)
	}
	preview := decodePreview(t, rec.Body.Bytes())
	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key":             testKey("retire-seed"),
		"expected_aggregate_revision": 1,
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("seed commit = %d body=%s", rec.Code, rec.Body.String())
	}

	tt := InvokeTimetable(t, h, token)
	if len(tt) != 2 {
		t.Fatalf("sessions = %d", len(tt))
	}
	// Drop course 乙 from the export: its arrangement becomes missing.
	onlyA := strings.Replace(academicFixture, "课程门数：2", "课程门数：1", 1)
	start := strings.Index(onlyA, "<tr><td>D-2")
	end := start + strings.Index(onlyA[start:], "</tr>") + len("</tr>")
	onlyA = onlyA[:start] + onlyA[end:]
	rec = multipartPreview(t, h, token, "2026S2", []byte(onlyA))
	if rec.Code != http.StatusCreated {
		t.Fatalf("second preview = %d body=%s", rec.Code, rec.Body.String())
	}
	second := decodePreview(t, rec.Body.Bytes())
	if second.Status != "NEEDS_REVIEW" || len(second.Diff.Missing) != 1 {
		t.Fatalf("second = %s missing=%+v", second.Status, second.Diff.Missing)
	}
	missingID := second.Diff.Missing[0].SessionID

	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/previews/"+second.PreviewID+"/rebase", map[string]any{
		"expected_aggregate_revision": 2,
		"resolutions": []map[string]any{{
			"kind": "RETIRE_SESSION", "session_id": missingID,
		}},
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("retire rebase = %d body=%s", rec.Code, rec.Body.String())
	}
	retirePreview := decodePreview(t, rec.Body.Bytes())
	if retirePreview.Status != "READY" {
		t.Fatalf("retire preview status = %s", retirePreview.Status)
	}
	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/previews/"+retirePreview.PreviewID+"/commit", map[string]any{
		"idempotency_key":             testKey("retire-1"),
		"expected_aggregate_revision": 2,
		"acknowledged_warning_codes":  []string{},
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("retire commit = %d body=%s", rec.Code, rec.Body.String())
	}
	receipt := decodeReceipt(t, rec.Body.Bytes())
	if receipt.Counts.SessionsRetired != 1 {
		t.Fatalf("receipt = %+v", receipt.Counts)
	}

	legacy, err := q.ListCourses(ctx, store.TextOf("2026S2"))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(legacy) != 1 {
		t.Fatalf("legacy courses = %d, want 1 after retire", len(legacy))
	}
	if legacy[0].Name != "合成课程甲" {
		t.Fatalf("wrong course kept: %s", legacy[0].Name)
	}
}

func TestLinkCourseResolution(t *testing.T) {
	h, q := academicTestHandler(t)
	token := loginToken(t, h)
	mustSettings(t, h, token, 0)
	seedB2Committed(t, q, 0)

	// Renamed course + changed code: metadata key no longer matches.
	renamed := strings.Replace(b2Fixture("三", "1-3周"), "[D-1]合成来源课", "[D-9]合成来源课（新）", 1)
	rec := multipartPreview(t, h, token, "2026S2", []byte(renamed))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d body=%s", rec.Code, rec.Body.String())
	}
	preview := decodePreview(t, rec.Body.Bytes())
	if preview.Status != "NEEDS_REVIEW" || len(preview.Diff.Added) != 1 || len(preview.Diff.Missing) != 1 {
		t.Fatalf("unexpected diff: %s %+v", preview.Status, preview.Diff)
	}
	courseKey := preview.Courses[0].PreviewCourseID
	if courseKey == "" {
		t.Fatal("preview course id missing")
	}

	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/previews/"+preview.PreviewID+"/rebase", map[string]any{
		"expected_aggregate_revision": 1,
		"resolutions": []map[string]any{{
			"kind":               "LINK_COURSE",
			"preview_course_id":  courseKey,
			"academic_course_id": b2CourseUUID,
		}},
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("rebase = %d body=%s", rec.Code, rec.Body.String())
	}
	rebased := decodePreview(t, rec.Body.Bytes())
	if rebased.Status != "READY" || len(rebased.Diff.Unchanged) != 1 || len(rebased.Diff.Added) != 0 {
		t.Fatalf("linked diff = %s %+v", rebased.Status, rebased.Diff)
	}
	if rebased.Courses[0].AcademicCourseID == nil || *rebased.Courses[0].AcademicCourseID != b2CourseUUID {
		t.Fatalf("course not linked: %+v", rebased.Courses[0])
	}
}

func decodeReceipt(t *testing.T, body []byte) commitReceipt {
	t.Helper()
	var r commitReceipt
	if err := json.Unmarshal(body, &r); err != nil {
		t.Fatalf("decode receipt: %v", err)
	}
	return r
}

func InvokeTimetable(t *testing.T, h http.Handler, token string) []string {
	t.Helper()
	rec := doJSON(t, h, http.MethodGet, "/api/academic/timetable?semester=2026S2", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("timetable = %d", rec.Code)
	}
	var tt struct {
		Courses []struct {
			Sessions []struct {
				SessionID string `json:"session_id"`
			} `json:"sessions"`
		} `json:"courses"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tt); err != nil {
		t.Fatalf("decode timetable: %v", err)
	}
	var ids []string
	for _, course := range tt.Courses {
		for _, s := range course.Sessions {
			ids = append(ids, s.SessionID)
		}
	}
	return ids
}

func TestSettingsChangeReprojectsCommittedCourses(t *testing.T) {
	h, q := academicTestHandler(t)
	token := loginToken(t, h)
	ctx := context.Background()
	mustSettings(t, h, token, 0)

	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d", rec.Code)
	}
	preview := decodePreview(t, rec.Body.Bytes())
	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key":             testKey("settings-reproject-1"),
		"expected_aggregate_revision": 1,
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("commit = %d body=%s", rec.Code, rec.Body.String())
	}
	before, err := q.ListCourses(ctx, store.TextOf("2026S2"))
	if err != nil || len(before) != 2 {
		t.Fatalf("legacy before = %d err=%v", len(before), err)
	}

	// Change period 11 start time; projections must follow in the same commit.
	body := configuredSettingsBody(1)
	periods := body["periods"].([]map[string]any)
	periods[10]["start_time"] = "16:25"
	if rec := doJSON(t, h, http.MethodPut, "/api/academic/semester-settings", body, token); rec.Code != http.StatusOK {
		t.Fatalf("settings PUT = %d body=%s", rec.Code, rec.Body.String())
	}

	after, err := q.ListCourses(ctx, store.TextOf("2026S2"))
	if err != nil || len(after) != len(before) {
		t.Fatalf("legacy after = %d err=%v", len(after), err)
	}
	if after[0].ID != before[0].ID {
		t.Fatalf("legacy id changed: %d -> %d", before[0].ID, after[0].ID)
	}
	updated := false
	for _, c := range after {
		if store.ClockString(c.StartTime) == "16:25" {
			updated = true
		}
	}
	if !updated {
		t.Fatalf("projection not rebuilt: %+v", after)
	}
}

func TestCommitLifecycleAndOverrides(t *testing.T) {
	h, q := academicTestHandler(t)
	token := loginToken(t, h)
	ctx := context.Background()
	mustSettings(t, h, token, 0)

	// Upload and get a READY preview (configured periods + confirmed anchor).
	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d body=%s", rec.Code, rec.Body.String())
	}
	preview := decodePreview(t, rec.Body.Bytes())
	if preview.Status != "READY" {
		t.Fatalf("preview status = %s", preview.Status)
	}

	commitBody := map[string]any{
		"idempotency_key":             testKey("commit-key-1"),
		"expected_aggregate_revision": 1,
		"acknowledged_warning_codes":  []string{},
	}
	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", commitBody, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("commit = %d body=%s", rec.Code, rec.Body.String())
	}
	var receipt commitReceipt
	_ = json.Unmarshal(rec.Body.Bytes(), &receipt)
	if receipt.Status != "COMMITTED" || receipt.Counts.CoursesCreated != 2 || receipt.Counts.SessionsCreated != 2 {
		t.Fatalf("receipt = %+v", receipt)
	}
	if receipt.Counts.LegacyCoursesCreated != 2 {
		t.Fatalf("legacy courses = %+v", receipt.Counts)
	}

	// Idempotent replay returns the same commit.
	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", commitBody, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("replay = %d body=%s", rec.Code, rec.Body.String())
	}
	var replay commitReceipt
	_ = json.Unmarshal(rec.Body.Bytes(), &replay)
	if replay.CommitID != receipt.CommitID {
		t.Fatalf("replay commit id changed: %s vs %s", replay.CommitID, receipt.CommitID)
	}

	// Same key with a different request is a conflict.
	conflictBody := map[string]any{
		"idempotency_key":             testKey("commit-key-1"),
		"expected_aggregate_revision": 2,
	}
	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", conflictBody, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("conflict = %d body=%s", rec.Code, rec.Body.String())
	}

	// Commit receipt is readable.
	rec = doJSON(t, h, http.MethodGet, "/api/academic/commits/"+receipt.CommitID, nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("get commit = %d", rec.Code)
	}
	var fetched commitReceipt
	_ = json.Unmarshal(rec.Body.Bytes(), &fetched)
	if fetched.Counts.SessionsCreated != 2 {
		t.Fatalf("fetched receipt = %+v", fetched)
	}

	// Legacy courses were projected.
	legacy, err := q.ListCourses(ctx, store.TextOf("2026S2"))
	if err != nil {
		t.Fatalf("list legacy: %v", err)
	}
	if len(legacy) != 2 {
		t.Fatalf("legacy courses = %d, want 2", len(legacy))
	}

	// Committed preview reports COMMITTED and cannot be re-committed.
	rec = doJSON(t, h, http.MethodGet, "/api/academic/henu/previews/"+preview.PreviewID, nil, token)
	var committed previewResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &committed)
	if committed.Status != "COMMITTED" || committed.CanCommit {
		t.Fatalf("committed preview = %s can_commit=%v", committed.Status, committed.CanCommit)
	}

	// Re-uploading the same file must produce an idempotent diff.
	rec = multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("re-upload = %d body=%s", rec.Code, rec.Body.String())
	}
	again := decodePreview(t, rec.Body.Bytes())
	if len(again.Diff.Unchanged) != 2 || len(again.Diff.Added) != 0 || len(again.Diff.Missing) != 0 {
		t.Fatalf("re-upload diff = %+v", again.Diff)
	}

	// Timetable read exposes sessions with effective values.
	rec = doJSON(t, h, http.MethodGet, "/api/academic/timetable?semester=2026S2", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("timetable = %d body=%s", rec.Code, rec.Body.String())
	}
	var tt struct {
		AggregateRevision int64 `json:"aggregate_revision"`
		Courses           []struct {
			Sessions []struct {
				SessionID string `json:"session_id"`
				Effective struct {
					Location *string `json:"location"`
				} `json:"effective_values"`
			} `json:"sessions"`
		} `json:"courses"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tt); err != nil {
		t.Fatalf("decode timetable: %v", err)
	}
	var sessionID string
	for _, course := range tt.Courses {
		if len(course.Sessions) > 0 {
			sessionID = course.Sessions[0].SessionID
		}
	}
	if sessionID == "" {
		t.Fatal("timetable has no sessions")
	}

	// Override location, then reset it back to the school value.
	rec = doJSON(t, h, http.MethodPatch, "/api/academic/sessions/"+sessionID+"/overrides", map[string]any{
		"expected_aggregate_revision": 2,
		"set":                         map[string]any{"location": "个人教室"},
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("override = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, http.MethodPatch, "/api/academic/sessions/"+sessionID+"/overrides", map[string]any{
		"expected_aggregate_revision": 3,
		"reset_fields":                []string{"location"},
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset override = %d body=%s", rec.Code, rec.Body.String())
	}
	var after struct {
		Effective struct {
			Location *string `json:"location"`
		} `json:"effective_values"`
		OverrideFields []string `json:"overridden_fields"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &after)
	if len(after.OverrideFields) != 0 {
		t.Fatalf("override fields = %v", after.OverrideFields)
	}
}

func TestCreateCourseResolutionForcesNewIdentity(t *testing.T) {
	h, q := academicTestHandler(t)
	token := loginToken(t, h)
	mustSettings(t, h, token, 0)
	seedB2Committed(t, q, 0)

	rec := multipartPreview(t, h, token, "2026S2", []byte(b2Fixture("三", "1-3周")))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d body=%s", rec.Code, rec.Body.String())
	}
	preview := decodePreview(t, rec.Body.Bytes())
	if preview.Status != "READY" || len(preview.Diff.Unchanged) != 1 {
		t.Fatalf("baseline = %s %+v", preview.Status, preview.Diff)
	}

	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/previews/"+preview.PreviewID+"/rebase", map[string]any{
		"expected_aggregate_revision": 1,
		"resolutions": []map[string]any{{
			"kind":              "CREATE_COURSE",
			"preview_course_id": preview.Courses[0].PreviewCourseID,
		}, {
			"kind": "RETIRE_SESSION", "session_id": b2SessionUUID,
		}},
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("rebase = %d body=%s", rec.Code, rec.Body.String())
	}
	forced := decodePreview(t, rec.Body.Bytes())
	if forced.Status != "READY" || len(forced.Diff.Added) != 1 || len(forced.Diff.Missing) != 1 {
		t.Fatalf("forced diff = %s %+v", forced.Status, forced.Diff)
	}
	if forced.Courses[0].AcademicCourseID != nil {
		t.Fatalf("CREATE_COURSE still linked: %v", *forced.Courses[0].AcademicCourseID)
	}

	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/previews/"+forced.PreviewID+"/commit", map[string]any{
		"idempotency_key":             testKey("create-course-1"),
		"expected_aggregate_revision": 1,
		"acknowledged_warning_codes":  []string{},
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("commit = %d body=%s", rec.Code, rec.Body.String())
	}
	receipt := decodeReceipt(t, rec.Body.Bytes())
	if receipt.Counts.CoursesCreated != 1 || receipt.Counts.LegacyCoursesCreated != 1 || receipt.Counts.SessionsRetired != 1 {
		t.Fatalf("receipt = %+v", receipt.Counts)
	}
}

func TestWarningAcknowledgementRequired(t *testing.T) {
	h, _ := academicTestHandler(t)
	token := loginToken(t, h)
	mustSettings(t, h, token, 0)

	noLocation := strings.Replace(academicFixture, " 示例楼306(120)", "", 1)
	rec := multipartPreview(t, h, token, "2026S2", []byte(noLocation))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d body=%s", rec.Code, rec.Body.String())
	}
	preview := decodePreview(t, rec.Body.Bytes())
	if preview.Status != "READY" {
		t.Fatalf("status = %s", preview.Status)
	}

	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key":             testKey("warn-1"),
		"expected_aggregate_revision": 1,
		"acknowledged_warning_codes":  []string{},
	}, token)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "MISSING_LOCATION") {
		t.Fatalf("unacknowledged = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key":             testKey("warn-2"),
		"expected_aggregate_revision": 1,
		"acknowledged_warning_codes":  []string{"MISSING_LOCATION"},
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("acknowledged = %d body=%s", rec.Code, rec.Body.String())
	}
}
