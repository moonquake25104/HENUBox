package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"kairos/server/internal/store"
)

func TestManualCourseCoexistence(t *testing.T) {
	h, q := academicTestHandler(t)
	token := loginToken(t, h)
	ctx := context.Background()
	mustSettings(t, h, token, 0)

	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d", rec.Code)
	}
	preview := decodePreview(t, rec.Body.Bytes())
	if code, body := postJSONRaw(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("coexist-seed"), "expected_aggregate_revision": 1,
	}); code != http.StatusOK {
		t.Fatalf("commit = %d %s", code, string(body))
	}

	manualSame := map[string]any{
		"name": "手工课同", "day_of_week": 1, "start_time": "08:00", "end_time": "09:30",
		"week_pattern": "1-16", "semester": "2026S2", "semester_start_date": "2026-08-31",
	}
	if rec := doJSON(t, h, http.MethodPost, "/api/courses", manualSame, token); rec.Code != http.StatusCreated {
		t.Fatalf("manual same = %d body=%s", rec.Code, rec.Body.String())
	}
	manualOther := map[string]any{
		"name": "手工课别的学期", "day_of_week": 2, "start_time": "08:00", "end_time": "09:30",
		"week_pattern": "1-16", "semester": "2026S3", "semester_start_date": "2027-02-22",
	}
	if rec := doJSON(t, h, http.MethodPost, "/api/courses", manualOther, token); rec.Code != http.StatusCreated {
		t.Fatalf("manual other = %d body=%s", rec.Code, rec.Body.String())
	}

	same, _ := q.ListCourses(ctx, store.TextOf("2026S2"))
	other, _ := q.ListCourses(ctx, store.TextOf("2026S3"))
	if len(same) != 3 || len(other) != 1 {
		t.Fatalf("same=%d other=%d", len(same), len(other))
	}

	// A no-op reimport must not touch manual courses or the other semester.
	rec = multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("reupload = %d", rec.Code)
	}
	again := decodePreview(t, rec.Body.Bytes())
	if code, body := postJSONRaw(h, token, "/api/academic/henu/previews/"+again.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("coexist-noop"), "expected_aggregate_revision": 3,
	}); code != http.StatusOK {
		t.Fatalf("reimport = %d %s", code, string(body))
	}
	same, _ = q.ListCourses(ctx, store.TextOf("2026S2"))
	other, _ = q.ListCourses(ctx, store.TextOf("2026S3"))
	if len(same) != 3 || len(other) != 1 {
		t.Fatalf("after noop same=%d other=%d", len(same), len(other))
	}
}

// TestEveryLegacyWriteInvalidatesPreview runs each old write entry in its own
// semester and checks the previously created preview cannot commit.
func TestEveryLegacyWriteInvalidatesPreview(t *testing.T) {
	type writeCase struct {
		name     string
		semester string
		run      func(t *testing.T, h http.Handler, token, semester string, courseID int64)
	}
	cases := []writeCase{
		{"create", "2026S1", func(t *testing.T, h http.Handler, token, sem string, _ int64) {
			if rec := doJSON(t, h, http.MethodPost, "/api/courses", map[string]any{
				"name": "手工新增", "day_of_week": 1, "start_time": "08:00", "end_time": "09:30",
				"week_pattern": "1-16", "semester": sem, "semester_start_date": "2026-02-23",
			}, token); rec.Code != http.StatusCreated {
				t.Fatalf("create = %d", rec.Code)
			}
		}},
		{"patch", "2026S1", func(t *testing.T, h http.Handler, token, sem string, id int64) {
			if rec := doJSON(t, h, http.MethodPatch, "/api/courses/"+itoa(id), map[string]any{
				"color": "#111111",
			}, token); rec.Code != http.StatusOK {
				t.Fatalf("patch = %d", rec.Code)
			}
		}},
		{"delete", "2026S1", func(t *testing.T, h http.Handler, token, sem string, id int64) {
			if rec := doJSON(t, h, http.MethodDelete, "/api/courses/"+itoa(id), nil, token); rec.Code != http.StatusOK {
				t.Fatalf("delete = %d", rec.Code)
			}
		}},
		{"reset-dates", "2026S1", func(t *testing.T, h http.Handler, token, sem string, _ int64) {
			if rec := doJSON(t, h, http.MethodPost, "/api/courses/reset-semester-dates", map[string]any{
				"semester": sem, "semester_start_date": "2026-03-02",
			}, token); rec.Code != http.StatusOK {
				t.Fatalf("reset = %d body=%s", rec.Code, rec.Body.String())
			}
		}},
		{"move-semester", "2026S1", func(t *testing.T, h http.Handler, token, sem string, id int64) {
			if rec := doJSON(t, h, http.MethodPatch, "/api/courses/"+itoa(id), map[string]any{
				"semester": "2026S3",
			}, token); rec.Code != http.StatusOK {
				t.Fatalf("move = %d body=%s", rec.Code, rec.Body.String())
			}
		}},
		{"import-text", "2026S1", func(t *testing.T, h http.Handler, token, sem string, _ int64) {
			text := "课程号\t课程\n序号\t课程名称\t任课教师\t学 分\t选课属性\t考核方式\t考试\n性质\t是否\n缓考\t上课时间、地点\t教材\t教学记录\t过程性成绩\n2043056\t3\t自动控制原理\t李红信\n3\t必修\t未确定\t正常考试\t非缓考\n1-17周全周\t星期三\t上午34节\t秦岭堂A114\n \t查看\t查看"
			if rec := doJSON(t, h, http.MethodPost, "/api/courses/import-text", map[string]any{
				"text": text, "semester": sem, "semester_start_date": "2026-02-23",
			}, token); rec.Code != http.StatusOK {
				t.Fatalf("import = %d body=%s", rec.Code, rec.Body.String())
			}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := academicTestHandler(t)
			token := loginToken(t, h)
			rec := doJSON(t, h, http.MethodPut, "/api/academic/semester-settings", map[string]any{
				"semester": tc.semester, "expected_revision": 0, "semester_label": "2025-2026学年第二学期",
				"week_anchor": map[string]any{"start_date": "2026-02-23", "source": "USER", "confirmed": true},
				"total_weeks": 18, "period_count": 13, "periods": configuredPeriods(),
			}, token)
			if rec.Code != http.StatusOK {
				t.Fatalf("settings = %d %s", rec.Code, rec.Body.String())
			}
			// Each case needs a course to patch/delete/move (harmless for others).
			created := doJSON(t, h, http.MethodPost, "/api/courses", map[string]any{
				"name": "种子课", "day_of_week": 2, "start_time": "08:00", "end_time": "09:30",
				"week_pattern": "1-16", "semester": tc.semester, "semester_start_date": "2026-02-23",
			}, token)
			var course struct {
				ID int64 `json:"id"`
			}
			_ = json.Unmarshal(created.Body.Bytes(), &course)

			rec = multipartPreview(t, h, token, tc.semester, []byte(academicFixture))
			if rec.Code != http.StatusCreated {
				t.Fatalf("preview = %d body=%s", rec.Code, rec.Body.String())
			}
			preview := decodePreview(t, rec.Body.Bytes())

			tc.run(t, h, token, tc.semester, course.ID)

			rec = doJSON(t, h, http.MethodGet, "/api/academic/henu/previews/"+preview.PreviewID, nil, token)
			var stale struct {
				CanCommit  bool `json:"can_commit"`
				Validation struct {
					State string `json:"state"`
				} `json:"validation"`
			}
			_ = json.Unmarshal(rec.Body.Bytes(), &stale)
			if stale.CanCommit || stale.Validation.State != "STALE" {
				t.Fatalf("preview not stale after %s: %+v", tc.name, stale)
			}
		})
	}
}

// TestImportTextMidFailureRollsBack: a failing course insert aborts the whole
// batch and leaves no rows or version bump.
func TestImportTextMidFailureRollsBack(t *testing.T) {
	h, q, conn, _, _ := academicTestEnv(t)
	token := loginToken(t, h)
	ctx := context.Background()

	if _, err := conn.Exec(ctx, `CREATE OR REPLACE FUNCTION fail_import() RETURNS trigger LANGUAGE plpgsql AS 'BEGIN RAISE EXCEPTION ''injected import failure''; END'`); err != nil {
		t.Fatalf("trigger func: %v", err)
	}
	if _, err := conn.Exec(ctx, `CREATE TRIGGER fail_import_trg BEFORE INSERT ON courses FOR EACH ROW EXECUTE FUNCTION fail_import()`); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	before, _ := q.GetAcademicVersions(ctx, "kairos", "2026S1")

	text := "课程号\t课程\n序号\t课程名称\t任课教师\t学 分\t选课属性\t考核方式\t考试\n性质\t是否\n缓考\t上课时间、地点\t教材\t教学记录\t过程性成绩\n2043056\t3\t自动控制原理\t李红信\n3\t必修\t未确定\t正常考试\t非缓考\n1-17周全周\t星期三\t上午34节\t秦岭堂A114\n \t查看\t查看"
	rec := doJSON(t, h, http.MethodPost, "/api/courses/import-text", map[string]any{
		"text": text, "semester": "2026S1", "semester_start_date": "2026-02-23",
	}, token)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("import = %d body=%s", rec.Code, rec.Body.String())
	}
	courses, _ := q.ListCourses(ctx, store.TextOf("2026S1"))
	if len(courses) != 0 {
		t.Fatalf("courses after rollback = %d", len(courses))
	}
	after, _ := q.GetAcademicVersions(ctx, "kairos", "2026S1")
	if after.AggregateRevision != before.AggregateRevision {
		t.Fatalf("aggregate bumped: %d -> %d", before.AggregateRevision, after.AggregateRevision)
	}
}

// TestBriefingSeesProjectedCourse proves the old briefing reader picks up the
// committed projection with the same weekday/time. The term label must equal the
// semester key so HasCoursesForSemester sees the projected courses.
func TestBriefingSeesProjectedCourse(t *testing.T) {
	h, q := academicTestHandler(t)
	token := loginToken(t, h)
	ctx := context.Background()
	mustSettings(t, h, token, 0)
	if _, err := q.UpsertSemesterContext(ctx, store.UpsertSemesterContextParams{
		Source: "manual", TermLabel: "2026S2",
		StartDate:  store.DateOf("2026-08-31"),
		TotalWeeks: pgtype.Int4{Int32: 18, Valid: true},
	}); err != nil {
		t.Fatalf("semester context: %v", err)
	}

	weekday := int(time.Now().Weekday())
	if weekday == 0 {
		weekday = 7
	}
	labels := map[int]string{1: "一", 2: "二", 3: "三", 4: "四", 5: "五", 6: "六", 7: "日"}
	fixture := strings.Replace(academicFixture, "三[11-13]", labels[weekday]+"[11-13]", 1)
	rec := multipartPreview(t, h, token, "2026S2", []byte(fixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d body=%s", rec.Code, rec.Body.String())
	}
	preview := decodePreview(t, rec.Body.Bytes())
	if code, body := postJSONRaw(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("briefing"), "expected_aggregate_revision": 1,
	}); code != http.StatusOK {
		t.Fatalf("commit = %d %s", code, string(body))
	}
	if rec := doJSON(t, h, http.MethodPost, "/api/term-phases", map[string]any{
		"term_label": "2026S2", "phase_type": "teaching",
		"start_week": 1, "end_week": 18,
		"affects_courses": true, "affects_exam_notifications": true,
		"pomodoro_profile": "normal", "notification_rules_json": "{}", "sort_order": 0,
	}, token); rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("term phase = %d body=%s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, http.MethodGet, "/api/briefing/today", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("briefing = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "合成课程甲") {
		t.Fatalf("briefing missing projected course: %s", rec.Body.String())
	}
}

// TestVersionReadFailureIs503: when the version table is unavailable the commit
// endpoint reports 503, not 500.
func TestVersionReadFailureIs503(t *testing.T) {
	h, _, conn, _, _ := academicTestEnv(t)
	token := loginToken(t, h)
	ctx := context.Background()
	mustSettings(t, h, token, 0)

	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d", rec.Code)
	}
	preview := decodePreview(t, rec.Body.Bytes())

	if _, err := conn.Exec(ctx, "ALTER TABLE academic_versions RENAME TO academic_versions_hidden"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	code, body := postJSONRaw(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("no-versions"), "expected_aggregate_revision": 1,
	})
	if code != http.StatusServiceUnavailable {
		t.Fatalf("commit = %d %s", code, string(body))
	}
	if _, err := conn.Exec(ctx, "ALTER TABLE academic_versions_hidden RENAME TO academic_versions"); err != nil {
		t.Fatalf("restore: %v", err)
	}
}
