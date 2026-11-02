package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"kairos/server/internal/domain/academic"
	"kairos/server/internal/store"
)

func seedLinkedLegacy(t *testing.T, q *store.Queries) int64 {
	t.Helper()
	ctx := context.Background()
	seedB2Committed(t, q, 0)
	course, err := q.CreateCourse(ctx, store.CreateCourseParams{
		Name: "合成来源课", DayOfWeek: 3,
		StartTime: store.ClockOf("18:10"), EndTime: store.ClockOf("20:40"),
		WeekPattern: "1周,2周,3周", SemesterStartDate: store.DateOf("2026-08-31"),
		Location: "示例楼101", Teacher: "示例教师", Color: "#7C8CC0", Semester: "2026S2",
	})
	if err != nil {
		t.Fatalf("legacy course: %v", err)
	}
	if err := q.UpsertLegacyLink(ctx, b2SessionUUID, course.ID); err != nil {
		t.Fatalf("link: %v", err)
	}
	if err := q.UpsertSessionOverride(ctx, b2SessionUUID, []string{"location"}, nil, nil, strPtrH("个人教室"), nil, nil); err != nil {
		t.Fatalf("override: %v", err)
	}
	return course.ID
}

// TestLinkPreservesOverrideAndResetsToNewSchoolValue: a school location change
// without a stable key must go through LINK; the personal override survives,
// the source value updates, and reset follows the new school value.
func TestLinkPreservesOverrideAndResetsToNewSchoolValue(t *testing.T) {
	h, q := academicTestHandler(t)
	token := loginToken(t, h)
	ctx := context.Background()
	mustSettings(t, h, token, 0)
	legacyID := seedLinkedLegacy(t, q)

	// Renamed course + changed location: must not match silently.
	changed := strings.ReplaceAll(b2Fixture("三", "1-3周"), "示例楼101(60)", "示例楼999(60)")
	changed = strings.ReplaceAll(changed, "[D-1]合成来源课", "[D-9]合成来源课（新）")
	rec := multipartPreview(t, h, token, "2026S2", []byte(changed))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d body=%s", rec.Code, rec.Body.String())
	}
	preview := decodePreview(t, rec.Body.Bytes())
	if len(preview.Diff.Added) != 1 || len(preview.Diff.Missing) != 1 {
		t.Fatalf("diff = %+v", preview.Diff)
	}
	psID := preview.Courses[0].Sessions[0].PreviewSessionID

	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/previews/"+preview.PreviewID+"/rebase", map[string]any{
		"expected_aggregate_revision": 1,
		"resolutions": []map[string]any{
			{"kind": "LINK_COURSE", "preview_course_id": preview.Courses[0].PreviewCourseID, "academic_course_id": b2CourseUUID},
			{"kind": "LINK_SESSION", "preview_session_id": psID, "session_id": b2SessionUUID},
		},
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("rebase = %d body=%s", rec.Code, rec.Body.String())
	}
	rebased := decodePreview(t, rec.Body.Bytes())
	if rebased.Status != "READY" || len(rebased.Diff.Changed) != 1 {
		t.Fatalf("rebased = %s %+v", rebased.Status, rebased.Diff)
	}

	rec = doJSON(t, h, http.MethodPost, "/api/academic/henu/previews/"+rebased.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("link-override-1"),
		"expected_aggregate_revision": 1,
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("commit = %d body=%s", rec.Code, rec.Body.String())
	}

	row, err := q.GetSourceSessionByID(ctx, b2SessionUUID)
	if err != nil {
		t.Fatalf("session: %v", err)
	}
	if row.LegacyCourseID == nil || *row.LegacyCourseID != legacyID {
		t.Fatalf("legacy id changed: %v want %d", row.LegacyCourseID, legacyID)
	}
	values, err := academic.SourceValuesFromJSON(row.SourceValues)
	if err != nil {
		t.Fatalf("source values: %v", err)
	}
	if values.Location == nil || *values.Location != "示例楼999" {
		t.Fatalf("source location not updated")
	}

	courses, err := q.ListCourses(ctx, store.TextOf("2026S2"))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(courses) != 1 || courses[0].Location != "个人教室" {
		t.Fatalf("override not preserved: %+v", courses)
	}

	// Reset returns to the *new* school value, not the old one.
	rec = doJSON(t, h, http.MethodPatch, "/api/academic/sessions/"+b2SessionUUID+"/overrides", map[string]any{
		"expected_aggregate_revision": 2,
		"reset_fields":                []string{"location"},
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("reset = %d body=%s", rec.Code, rec.Body.String())
	}
	courses, _ = q.ListCourses(ctx, store.TextOf("2026S2"))
	if len(courses) != 1 || courses[0].Location != "示例楼999" {
		t.Fatalf("reset location = %+v", courses)
	}
}

// TestHiddenCourseNotResurrectedByReimport: a personal hidden flag survives a
// no-op re-import and does not bring the course back into the list.
func TestHiddenCourseNotResurrectedByReimport(t *testing.T) {
	h, q := academicTestHandler(t)
	token := loginToken(t, h)
	ctx := context.Background()
	mustSettings(t, h, token, 0)

	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d", rec.Code)
	}
	preview := decodePreview(t, rec.Body.Bytes())
	code, body := postJSONRaw(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("hidden-seed"), "expected_aggregate_revision": 1,
	})
	if code != http.StatusOK {
		t.Fatalf("seed commit = %d %s", code, string(body))
	}

	courses, _ := q.ListCourses(ctx, store.TextOf("2026S2"))
	if len(courses) != 2 {
		t.Fatalf("courses = %d", len(courses))
	}
	if rec := doJSON(t, h, http.MethodDelete, "/api/courses/"+itoa(courses[0].ID), nil, token); rec.Code != http.StatusOK {
		t.Fatalf("hide = %d", rec.Code)
	}
	hiddenName := courses[0].Name
	if afterHide, _ := q.ListCourses(ctx, store.TextOf("2026S2")); len(afterHide) != 1 {
		t.Fatalf("hide did not remove from list: %d", len(afterHide))
	}

	rec = multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("reupload = %d", rec.Code)
	}
	again := decodePreview(t, rec.Body.Bytes())
	if again.Status != "READY" || len(again.Diff.Unchanged) != 2 {
		t.Fatalf("reupload diff = %s %+v", again.Status, again.Diff)
	}
	code, body = postJSONRaw(h, token, "/api/academic/henu/previews/"+again.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("hidden-reimport"), "expected_aggregate_revision": 3,
	})
	if code != http.StatusOK {
		t.Fatalf("reimport commit = %d %s", code, string(body))
	}
	receipt := decodeReceipt(t, body)
	if !receipt.NoChanges {
		t.Fatalf("expected no_changes receipt: %+v", receipt.Counts)
	}

	remaining, _ := q.ListCourses(ctx, store.TextOf("2026S2"))
	if len(remaining) != 1 {
		t.Fatalf("hidden course resurrected: %d visible", len(remaining))
	}
	row, _ := q.GetSourceSessionByID(ctx, sessionIDByCourseName(t, q, hiddenName))
	if row.Hidden == nil || !*row.Hidden {
		t.Fatal("hidden override lost")
	}
}

// TestLegacyWriteInvalidatesPreview: any old write entry bumps the aggregate,
// so an older preview can no longer commit.
func TestLegacyWriteInvalidatesPreview(t *testing.T) {
	h, _ := academicTestHandler(t)
	token := loginToken(t, h)
	mustSettings(t, h, token, 0)

	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d", rec.Code)
	}
	preview := decodePreview(t, rec.Body.Bytes())

	rec = doJSON(t, h, http.MethodPost, "/api/courses", map[string]any{
		"name": "手工课", "day_of_week": 5, "start_time": "08:00", "end_time": "09:30",
		"week_pattern": "1-16", "semester": "2026S2", "semester_start_date": "2026-08-31",
	}, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("manual course = %d body=%s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, h, http.MethodGet, "/api/academic/henu/previews/"+preview.PreviewID, nil, token)
	var stale struct {
		CanCommit  bool `json:"can_commit"`
		Validation struct {
			State string `json:"state"`
		} `json:"validation"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &stale)
	if stale.CanCommit || stale.Validation.State != "STALE" {
		t.Fatalf("preview not stale: %+v", stale)
	}

	code, body := postJSONRaw(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("stale-1"), "expected_aggregate_revision": 1,
	})
	if code != http.StatusConflict {
		t.Fatalf("stale commit = %d %s", code, string(body))
	}
}

func sessionIDByCourseName(t *testing.T, q *store.Queries, name string) string {
	t.Helper()
	rows, err := q.ListAcademicState(context.Background(), "kairos", "2026S2")
	if err != nil {
		t.Fatalf("list state: %v", err)
	}
	for _, row := range rows {
		if row.CourseName == name {
			return row.SessionID
		}
	}
	t.Fatalf("course %q has no session", name)
	return ""
}

// TestPreviewOwnerIsolation: another owner's preview is invisible and cannot be
// committed.
func TestPreviewOwnerIsolation(t *testing.T) {
	h, q := academicTestHandler(t)
	token := loginToken(t, h)
	ctx := context.Background()
	id := "77777777-7777-4777-8777-777777777777"
	row := store.AcademicPreviewRow{
		ID: id, Owner: "someone-else", Semester: "2026S2", Source: "henu", Acquisition: "FILE_IMPORT",
		ParserVersion: "henu-html-v1", Status: "READY", CanCommit: true,
		Payload: []byte(`{"schema_version":1}`), Summary: []byte(`{}`), Issues: []byte(`[]`),
		FileSHA256: "x", FileBytes: 1,
		ReceivedAt: time.Now(), CreatedAt: time.Now(), ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	if err := q.InsertAcademicPreview(ctx, row); err != nil {
		t.Fatalf("insert other owner: %v", err)
	}
	rec := doJSON(t, h, http.MethodGet, "/api/academic/henu/previews/"+id, nil, token)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-owner GET = %d, want 404", rec.Code)
	}
	code, _ := postJSONRaw(h, token, "/api/academic/henu/previews/"+id+"/commit", map[string]any{
		"idempotency_key": testKey("cross-owner"), "expected_aggregate_revision": 0,
	})
	if code != http.StatusNotFound {
		t.Fatalf("cross-owner commit = %d, want 404", code)
	}
}
