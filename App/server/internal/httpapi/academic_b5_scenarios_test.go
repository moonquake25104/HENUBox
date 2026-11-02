package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"

	"kairos/server/internal/store"
)

func configuredPeriods() []map[string]any {
	return configuredSettingsBody(0)["periods"].([]map[string]any)
}

func unconfiguredPeriods() []map[string]any {
	return settingsBody(0, false)["periods"].([]map[string]any)
}

func mustTestPasswordHash(t *testing.T) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	return string(hash)
}

func mixedWeeksFixture() string {
	return `<!DOCTYPE html><html><head><meta charset="utf-8"></head><body>
<div>2026-2027学年第一学期&ensp;课程门数：1&ensp;</div>
<table><thead><tr><th>上课班级代码</th><th>上课班级名称</th><th>课程</th><th>总学时</th><th>学分</th>
<th>修读性质</th><th>任课教师</th><th>选课状态</th><th>是否跨学科</th><th>教材</th><th>上课时间地点</th><th>备注</th></tr></thead>
<tbody><tr><td>D-7</td><td>示例班</td><td>[D-7]合成混合周课</td><td>54</td><td>3.0</td><td>初修</td><td>示例教师</td>
<td>选中</td><td>否</td><td>否</td><td>1-16周（单） 三[1-2] 示例楼101(60)；2-16周（双） 五[3-4] 示例楼102(60)</td><td></td></tr>
</tbody></table></body></html>`
}

// TestTwoArrangementsAndMixedWeeks: one logical course, two arrangements with
// odd/even weeks must produce two legacy rows with the right patterns.
func TestTwoArrangementsAndMixedWeeks(t *testing.T) {
	h, q := academicTestHandler(t)
	token := loginToken(t, h)
	ctx := context.Background()
	mustSettings(t, h, token, 0)

	rec := multipartPreview(t, h, token, "2026S2", []byte(mixedWeeksFixture()))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d body=%s", rec.Code, rec.Body.String())
	}
	preview := decodePreview(t, rec.Body.Bytes())
	if preview.Summary.LogicalCourses != 1 || preview.Summary.Sessions != 2 {
		t.Fatalf("summary = %+v", preview.Summary)
	}
	code, body := postJSONRaw(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("mixed-weeks"), "expected_aggregate_revision": 1,
	})
	if code != http.StatusOK {
		t.Fatalf("commit = %d body=%s", code, string(body))
	}
	legacy, err := q.ListCourses(ctx, store.TextOf("2026S2"))
	if err != nil || len(legacy) != 2 {
		t.Fatalf("legacy = %d err=%v", len(legacy), err)
	}
	patterns := map[string]bool{}
	for _, c := range legacy {
		patterns[c.WeekPattern] = true
	}
	if len(patterns) != 2 {
		t.Fatalf("patterns not distinct: %v", patterns)
	}
	for _, c := range legacy {
		if c.DayOfWeek != 3 {
			continue
		}
		for w := 1; w <= 16; w++ {
			if weekHas(c.WeekPattern, w) != (w%2 == 1) {
				t.Fatalf("odd-week mismatch at week %d", w)
			}
		}
	}
}

func weekHas(pattern string, week int) bool {
	needle := itoa(int64(week)) + "周"
	for _, part := range splitComma(pattern) {
		if part == needle {
			return true
		}
	}
	return false
}

func splitComma(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ',' {
			out = append(out, cur)
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// TestCommittedReceiptSurvivesExpiry: the receipt stays replayable after the
// preview itself has expired.
func TestCommittedReceiptSurvivesExpiry(t *testing.T) {
	h, _, conn, _, _ := academicTestEnv(t)
	token := loginToken(t, h)
	ctx := context.Background()
	mustSettings(t, h, token, 0)

	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d", rec.Code)
	}
	preview := decodePreview(t, rec.Body.Bytes())
	commitBody := map[string]any{
		"idempotency_key": testKey("survive-expiry"), "expected_aggregate_revision": 1,
	}
	code, body := postJSONRaw(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", commitBody)
	if code != http.StatusOK {
		t.Fatalf("commit = %d %s", code, string(body))
	}
	original := decodeReceipt(t, body)

	if _, err := conn.Exec(ctx, "UPDATE academic_previews SET created_at = now() - interval '2 days', expires_at = now() - interval '1 day' WHERE id = $1::uuid", preview.PreviewID); err != nil {
		t.Fatalf("expire: %v", err)
	}
	code, body = postJSONRaw(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", commitBody)
	if code != http.StatusOK || decodeReceipt(t, body).CommitID != original.CommitID {
		t.Fatalf("replay after expiry = %d %s", code, string(body))
	}
}

// TestSettingsReprojectionKeepsOverridesAndRejectsBadConfig: changing periods
// keeps personal overrides, while a config that cannot express an arrangement
// rolls back entirely.
func TestSettingsReprojectionKeepsOverridesAndRejectsBadConfig(t *testing.T) {
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
		"idempotency_key": testKey("settings-keep"), "expected_aggregate_revision": 1,
	})
	if code != http.StatusOK {
		t.Fatalf("commit = %d %s", code, string(body))
	}
	sessionID := sessionIDByCourseName(t, q, "合成课程甲")
	if rec := doJSON(t, h, http.MethodPatch, "/api/academic/sessions/"+sessionID+"/overrides", map[string]any{
		"expected_aggregate_revision": 2, "set": map[string]any{"location": "个人教室"},
	}, token); rec.Code != http.StatusOK {
		t.Fatalf("override = %d", rec.Code)
	}

	settingsReq := map[string]any{
		"semester": "2026S2", "expected_revision": 1, "semester_label": "2026-2027学年第一学期",
		"week_anchor": map[string]any{"start_date": "2026-08-31", "source": "USER", "confirmed": true},
		"total_weeks": 18, "period_count": 13, "periods": configuredPeriods(),
	}
	periods := settingsReq["periods"].([]map[string]any)
	periods[10]["start_time"] = "16:25"
	if rec := doJSON(t, h, http.MethodPut, "/api/academic/semester-settings", settingsReq, token); rec.Code != http.StatusOK {
		t.Fatalf("settings = %d %s", rec.Code, rec.Body.String())
	}
	courses, _ := q.ListCourses(ctx, store.TextOf("2026S2"))
	if len(courses) != 2 || courses[0].Location != "个人教室" {
		t.Fatalf("override lost after reprojection: %+v", courses)
	}
	if store.ClockString(courses[0].StartTime) != "16:25" {
		t.Fatalf("time not reprojected: %s", store.ClockString(courses[0].StartTime))
	}

	// Bad config: drop period 11; the transaction must roll back and keep rev 2.
	periods[10]["configured"] = false
	periods[10]["start_time"] = nil
	periods[10]["end_time"] = nil
	rec = doJSON(t, h, http.MethodPut, "/api/academic/semester-settings", settingsReq, token)
	if rec.Code != http.StatusConflict {
		t.Fatalf("bad settings = %d body=%s", rec.Code, rec.Body.String())
	}
	settingsNow := doJSON(t, h, http.MethodGet, "/api/academic/semester-settings?semester=2026S2", nil, token)
	var current struct {
		Revision int64 `json:"revision"`
	}
	_ = json.Unmarshal(settingsNow.Body.Bytes(), &current)
	if current.Revision != 2 {
		t.Fatalf("settings revision advanced on rollback: %d", current.Revision)
	}
	courses, _ = q.ListCourses(ctx, store.TextOf("2026S2"))
	if store.ClockString(courses[0].StartTime) != "16:25" {
		t.Fatalf("legacy time changed on rollback")
	}
}

// TestBlockingPreviewCannotCommit: BLOCKING (missing times) cannot be waived by
// acknowledging warnings.
func TestBlockingPreviewCannotCommit(t *testing.T) {
	h, _ := academicTestHandler(t)
	token := loginToken(t, h)

	body := map[string]any{
		"semester": "2026S2", "expected_revision": 0, "semester_label": "2026-2027学年第一学期",
		"week_anchor": map[string]any{"start_date": "2026-08-31", "source": "USER", "confirmed": true},
		"total_weeks": 18, "period_count": 13, "periods": unconfiguredPeriods(),
	}
	if rec := doJSON(t, h, http.MethodPut, "/api/academic/semester-settings", body, token); rec.Code != http.StatusOK {
		t.Fatalf("settings = %d %s", rec.Code, rec.Body.String())
	}
	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d", rec.Code)
	}
	preview := decodePreview(t, rec.Body.Bytes())
	if preview.Status != "NEEDS_CONFIGURATION" {
		t.Fatalf("status = %s", preview.Status)
	}
	code, respBody := postJSONRaw(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("blocking"), "expected_aggregate_revision": 1,
	})
	if code != http.StatusConflict {
		t.Fatalf("blocking commit = %d %s", code, string(respBody))
	}
}

// TestRestartPersistence: a fresh pool + router over the same schema sees the
// committed timetable (simulates a process restart).
func TestRestartPersistence(t *testing.T) {
	h, _, _, _, schema := academicTestEnv(t)
	token := loginToken(t, h)
	ctx := context.Background()
	mustSettings(t, h, token, 0)

	rec := multipartPreview(t, h, token, "2026S2", []byte(academicFixture))
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d", rec.Code)
	}
	preview := decodePreview(t, rec.Body.Bytes())
	code, body := postJSONRaw(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("restart"), "expected_aggregate_revision": 1,
	})
	if code != http.StatusOK {
		t.Fatalf("commit = %d %s", code, string(body))
	}

	cfg, err := pgxpool.ParseConfig(testDatabaseURL())
	if err != nil {
		t.Fatalf("pool config: %v", err)
	}
	cfg.MaxConns = 2
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET search_path TO "+schema)
		return err
	}
	pool2, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pool2: %v", err)
	}
	defer pool2.Close()
	h2 := New(Options{
		Log:          slog.New(slog.NewTextHandler(io.Discard, nil)),
		Username:     "kairos",
		PasswordHash: mustTestPasswordHash(t),
		JWTSecret:    []byte(testSecret),
		JWTTTL:       time.Hour,
		Store:        store.New(pool2),
		Pool:         pool2,
		DataDir:      t.TempDir(),
	})
	token2 := loginToken(t, h2)
	rec = doJSON(t, h2, http.MethodGet, "/api/courses?semester=2026S2", nil, token2)
	if rec.Code != http.StatusOK {
		t.Fatalf("restart list = %d", rec.Code)
	}
	var courses []struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &courses)
	if len(courses) != 2 {
		t.Fatalf("after restart courses = %d", len(courses))
	}
}
