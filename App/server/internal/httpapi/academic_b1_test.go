package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"kairos/server/internal/httpapi/dto"
	"kairos/server/internal/store"
)

func configuredSettingsBody(revision int64) map[string]any {
	periods := make([]map[string]any, 0, 13)
	for i := 1; i <= 13; i++ {
		startMin := 8*60 + (i-1)*50
		endMin := startMin + 45
		periods = append(periods, map[string]any{
			"index":      i,
			"start_time": fmt.Sprintf("%02d:%02d", startMin/60, startMin%60),
			"end_time":   fmt.Sprintf("%02d:%02d", endMin/60, endMin%60),
			"configured": true,
		})
	}
	return map[string]any{
		"semester":          "2026S2",
		"expected_revision": revision,
		"semester_label":    "2026-2027学年第一学期",
		"week_anchor":       map[string]any{"start_date": "2026-08-31", "source": "USER", "confirmed": true},
		"total_weeks":       18,
		"period_count":      13,
		"periods":           periods,
	}
}

func mustSettings(t *testing.T, h http.Handler, token string, revision int64) {
	t.Helper()
	if rec := doJSON(t, h, http.MethodPut, "/api/academic/semester-settings", configuredSettingsBody(revision), token); rec.Code != http.StatusOK {
		t.Fatalf("settings PUT = %d body=%s", rec.Code, rec.Body.String())
	}
}

func createLegacyCourse(t *testing.T, h http.Handler, token string) dto.Course {
	t.Helper()
	body := map[string]any{
		"name": "合成手工课", "day_of_week": 3, "start_time": "18:10", "end_time": "20:40",
		"week_pattern": "1周,2周,3周", "semester_start_date": "2026-08-31",
		"location": "示例楼101", "teacher": "示例教师", "semester": "2026S2",
	}
	rec := doJSON(t, h, http.MethodPost, "/api/courses", body, token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create course = %d body=%s", rec.Code, rec.Body.String())
	}
	var c dto.Course
	if err := json.Unmarshal(rec.Body.Bytes(), &c); err != nil {
		t.Fatalf("decode course: %v", err)
	}
	return c
}

func TestLegacyWritesBumpVersions(t *testing.T) {
	h, q := academicTestHandler(t)
	token := loginToken(t, h)
	ctx := context.Background()

	v0, err := q.GetAcademicVersions(ctx, "kairos", "2026S2")
	if err != nil {
		t.Fatalf("versions: %v", err)
	}
	c := createLegacyCourse(t, h, token)
	v1, _ := q.GetAcademicVersions(ctx, "kairos", "2026S2")
	if v1.TimetableRevision != v0.TimetableRevision+1 || v1.AggregateRevision != v0.AggregateRevision+1 {
		t.Fatalf("create did not bump: before=%+v after=%+v", v0, v1)
	}

	// No-op PATCH must not bump.
	rec := doJSON(t, h, http.MethodPatch, fmt.Sprintf("/api/courses/%d", c.ID), map[string]any{"location": "示例楼101"}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("noop patch = %d body=%s", rec.Code, rec.Body.String())
	}
	v2, _ := q.GetAcademicVersions(ctx, "kairos", "2026S2")
	if v2.AggregateRevision != v1.AggregateRevision {
		t.Fatalf("no-op patch bumped aggregate: %+v -> %+v", v1, v2)
	}

	rec = doJSON(t, h, http.MethodPatch, fmt.Sprintf("/api/courses/%d", c.ID), map[string]any{"location": "示例楼202"}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch = %d body=%s", rec.Code, rec.Body.String())
	}
	v3, _ := q.GetAcademicVersions(ctx, "kairos", "2026S2")
	if v3.AggregateRevision != v2.AggregateRevision+1 {
		t.Fatalf("patch did not bump: %+v", v3)
	}

	// import-text: a real batch bumps once; re-importing the same text is a no-op.
	importText := "课程号\t课程\n序号\t课程名称\t任课教师\t学 分\t选课属性\t考核方式\t考试\n性质\t是否\n缓考\t上课时间、地点\t教材\t教学记录\t过程性成绩\n2043056\t3\t自动控制原理\t李红信\n3\t必修\t未确定\t正常考试\t非缓考\n1-17周全周\t星期三\t上午34节\t秦岭堂A114\n1-17周单周\t星期二\t晚9-10节\t天山堂A312\n \t查看\t查看"
	importBody := map[string]any{"text": importText, "semester": "2026S3", "semester_start_date": "2026-02-24"}
	rec = doJSON(t, h, http.MethodPost, "/api/courses/import-text", importBody, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("import-text = %d body=%s", rec.Code, rec.Body.String())
	}
	v4, _ := q.GetAcademicVersions(ctx, "kairos", "2026S3")
	if v4.AggregateRevision != 1 {
		t.Fatalf("first import aggregate = %+v, want 1", v4)
	}
	rec = doJSON(t, h, http.MethodPost, "/api/courses/import-text", importBody, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("second import-text = %d body=%s", rec.Code, rec.Body.String())
	}
	v4b, _ := q.GetAcademicVersions(ctx, "kairos", "2026S3")
	if v4b.AggregateRevision != v4.AggregateRevision {
		t.Fatalf("duplicate import bumped: %+v -> %+v", v4, v4b)
	}

	rec = doJSON(t, h, http.MethodDelete, fmt.Sprintf("/api/courses/%d", c.ID), nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete = %d body=%s", rec.Code, rec.Body.String())
	}
	v5, _ := q.GetAcademicVersions(ctx, "kairos", "2026S2")
	if v5.AggregateRevision != v3.AggregateRevision+1 {
		t.Fatalf("delete did not bump: %+v (before %+v)", v5, v3)
	}
}

// seedSourceSession links an existing legacy course to an academic session, as
// a B3 commit will do.
func seedSourceSession(t *testing.T, q *store.Queries, courseID int64) {
	t.Helper()
	ctx := context.Background()
	const owner, semester = "kairos", "2026S2"
	courseUUID := "0b6f6f0e-0000-4000-8000-000000000001"
	entryUUID := "0b6f6f0e-0000-4000-8000-000000000002"
	revUUID := "0b6f6f0e-0000-4000-8000-000000000003"
	sessionUUID := "0b6f6f0e-0000-4000-8000-000000000004"

	if err := q.InsertAcademicCourse(ctx, courseUUID, owner, semester, "henu", "合成来源课", strPtrH("D-1"), nil, nil, nil); err != nil {
		t.Fatalf("insert academic course: %v", err)
	}
	if err := q.InsertSourceEntry(ctx, entryUUID, owner, semester, "henu", courseUUID, strPtrH("D-1")); err != nil {
		t.Fatalf("insert entry: %v", err)
	}
	values := map[string]any{
		"name": "合成来源课", "teacher": "示例教师", "location": "示例楼101", "color": nil,
		"hidden": false, "weekday": 3, "sections": []int{11, 12, 13},
		"weeks": []int{1, 2, 3}, "date": nil, "start_time": "18:10", "end_time": "20:40",
	}
	raw, _ := json.Marshal(values)
	if err := q.InsertSourceRevision(ctx, revUUID, entryUUID, nil, "sha-test", raw); err != nil {
		t.Fatalf("insert revision: %v", err)
	}
	if err := q.InsertAcademicSession(ctx, sessionUUID, courseUUID, entryUUID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if err := q.UpsertLegacyLink(ctx, sessionUUID, courseID); err != nil {
		t.Fatalf("link: %v", err)
	}
}

func strPtrH(s string) *string { return &s }

func TestSourceCoursePatchAndDelete(t *testing.T) {
	h, q := academicTestHandler(t)
	token := loginToken(t, h)
	mustSettings(t, h, token, 0)
	c := createLegacyCourse(t, h, token)
	seedSourceSession(t, q, c.ID)
	ctx := context.Background()

	// Personal location override.
	rec := doJSON(t, h, http.MethodPatch, fmt.Sprintf("/api/courses/%d", c.ID), map[string]any{"location": "个人教室"}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("override patch = %d body=%s", rec.Code, rec.Body.String())
	}
	var updated dto.Course
	_ = json.Unmarshal(rec.Body.Bytes(), &updated)
	if updated.Location != "个人教室" {
		t.Fatalf("location = %q", updated.Location)
	}
	src, err := q.GetSourceSessionByCourseID(ctx, c.ID)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}
	if !hasFieldH(src.SetFields, "location") || src.Location == nil || *src.Location != "个人教室" {
		t.Fatalf("override not stored: %+v", src)
	}

	// Structural edit must be rejected and change nothing.
	rec = doJSON(t, h, http.MethodPatch, fmt.Sprintf("/api/courses/%d", c.ID), map[string]any{"day_of_week": 4}, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("structural patch = %d body=%s", rec.Code, rec.Body.String())
	}

	// DELETE hides instead of removing the source.
	rec = doJSON(t, h, http.MethodDelete, fmt.Sprintf("/api/courses/%d", c.ID), nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete = %d body=%s", rec.Code, rec.Body.String())
	}
	src, _ = q.GetSourceSessionByCourseID(ctx, c.ID)
	if src.Hidden == nil || !*src.Hidden {
		t.Fatalf("hidden override missing: %+v", src)
	}

	// Whole-semester date reset must refuse while source links exist.
	rec = doJSON(t, h, http.MethodPost, "/api/courses/reset-semester-dates", map[string]any{"date": "2026-09-07"}, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("reset dates = %d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["code"] != "SOURCE_STRUCTURE_EDIT_UNSUPPORTED" {
		t.Fatalf("reset code = %v", body["code"])
	}
}

func hasFieldH(fields []string, f string) bool {
	for _, x := range fields {
		if x == f {
			return true
		}
	}
	return false
}
