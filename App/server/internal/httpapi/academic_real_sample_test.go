package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"kairos/server/internal/domain/calendar"
	"kairos/server/internal/domain/henu"
	"kairos/server/internal/store"
)

// TestRealSamplePerSessionProjection is the per-row check from design §9.1:
// every previewed session must land in the legacy table with the same weekday,
// start/end time and week membership, not just the right totals. It skips when
// the user's private sample is not present (CI) and never prints names.
func TestRealSamplePerSessionProjection(t *testing.T) {
	dir := os.Getenv("HENU_SAMPLE_DIR")
	if dir == "" {
		t.Skip("Set HENU_SAMPLE_DIR to an explicitly authorized local sample directory")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "raw", "河南大学学生个人课表.xls"))
	if err != nil {
		t.Skip("real sample not available")
	}
	profileRaw, err := os.ReadFile(filepath.Join(dir, "normalized", "periods-user-2026-10-23.json"))
	if err != nil {
		t.Skip("periods profile not available")
	}
	var profile struct {
		Periods []struct {
			Index      int    `json:"index"`
			StartTime  string `json:"start_time"`
			EndTime    string `json:"end_time"`
			Configured bool   `json:"configured"`
		} `json:"periods"`
	}
	if err := json.Unmarshal(profileRaw, &profile); err != nil {
		t.Fatalf("profile: %v", err)
	}
	periods := make([]map[string]any, 0, len(profile.Periods))
	starts := map[int][2]string{}
	for _, p := range profile.Periods {
		periods = append(periods, map[string]any{
			"index": p.Index, "start_time": p.StartTime, "end_time": p.EndTime, "configured": p.Configured,
		})
		starts[p.Index] = [2]string{p.StartTime, p.EndTime}
	}

	h, q := academicTestHandler(t)
	token := loginToken(t, h)
	body := map[string]any{
		"semester": "2026S2", "expected_revision": 0, "semester_label": "2026-2027学年第一学期",
		"week_anchor": map[string]any{"start_date": "2026-08-31", "source": "USER", "confirmed": true},
		"total_weeks": 18, "period_count": 13, "periods": periods,
	}
	if rec := doJSON(t, h, http.MethodPut, "/api/academic/semester-settings", body, token); rec.Code != http.StatusOK {
		t.Fatalf("settings = %d body=%s", rec.Code, rec.Body.String())
	}

	rec := multipartPreview(t, h, token, "2026S2", raw)
	if rec.Code != http.StatusCreated {
		t.Fatalf("preview = %d body=%s", rec.Code, rec.Body.String())
	}
	preview := decodePreview(t, rec.Body.Bytes())
	if preview.Summary.LogicalCourses != 13 || preview.Summary.Sessions != 18 {
		t.Fatalf("summary courses=%d sessions=%d", preview.Summary.LogicalCourses, preview.Summary.Sessions)
	}
	var ack []string
	seen := map[string]bool{}
	for _, issue := range preview.Issues {
		if issue.Severity == "WARNING" && !seen[issue.Code] {
			seen[issue.Code] = true
			ack = append(ack, issue.Code)
		}
	}

	code, respBody := postJSONRaw(h, token, "/api/academic/henu/previews/"+preview.PreviewID+"/commit", map[string]any{
		"idempotency_key": testKey("real-row-check"), "expected_aggregate_revision": 1,
		"acknowledged_warning_codes": ack,
	})
	if code != http.StatusOK {
		t.Fatalf("commit = %d body=%s", code, string(respBody))
	}

	legacy, err := q.ListCourses(context.Background(), store.TextOf("2026S2"))
	if err != nil {
		t.Fatalf("list legacy: %v", err)
	}
	if len(legacy) != 18 {
		t.Fatalf("legacy rows = %d", len(legacy))
	}

	parsed, err := henu.Parse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	type key struct {
		name  string
		day   int
		start string
		end   string
	}
	expected := map[key]int{}
	expectedWeeks := map[key]map[int]bool{}
	for _, course := range parsed.Courses {
		for _, session := range course.Sessions {
			if len(session.Sections) == 0 {
				continue
			}
			first, last := session.Sections[0], session.Sections[len(session.Sections)-1]
			times, ok := starts[first]
			if !ok {
				t.Fatalf("period %d missing from profile", first)
			}
			_, ok = starts[last]
			if !ok {
				t.Fatalf("period %d missing from profile", last)
			}
			k := key{course.Name, session.Weekday, times[0], starts[last][1]}
			expected[k]++
			if expectedWeeks[k] == nil {
				expectedWeeks[k] = map[int]bool{}
			}
			for _, w := range session.Weeks {
				expectedWeeks[k][w] = true
			}
		}
	}

	actual := map[key]int{}
	weekMismatches := 0
	for _, row := range legacy {
		k := key{row.Name, int(row.DayOfWeek), store.ClockString(row.StartTime), store.ClockString(row.EndTime)}
		actual[k]++
		want := expectedWeeks[k]
		for w := 1; w <= 18; w++ {
			if calendar.MatchesWeekPattern(row.WeekPattern, int64(w)) != want[w] {
				weekMismatches++
				break
			}
		}
	}
	if len(actual) != len(expected) {
		t.Fatalf("distinct projection keys: got %d want %d", len(actual), len(expected))
	}
	for k, want := range expected {
		if actual[k] != want {
			t.Fatalf("projection count mismatch for weekday %d", k.day)
		}
	}
	if weekMismatches != 0 {
		t.Fatalf("week membership mismatches: %d", weekMismatches)
	}
}
