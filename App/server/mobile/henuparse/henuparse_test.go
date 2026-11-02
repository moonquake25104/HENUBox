package henuparse

import (
	"encoding/json"
	"strings"
	"testing"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

const testHeaders = `<tr>
<th>上课班级代码</th><th>上课班级名称</th><th>课程</th><th>总学时</th><th>学分</th>
<th>修读性质</th><th>任课教师</th><th>选课状态</th><th>是否跨学科</th><th>教材</th>
<th>上课时间地点</th><th>备注</th></tr>`

func sampleHTML(rows ...string) string {
	return `<!DOCTYPE HTML PUBLIC "-//W3C//DTD HTML 4.01 Transitional//EN">
<html><head><meta http-equiv="Content-Type" content="text/html; charset=utf-8">
<title></title></head><body>
<div>2026-2027学年第一学期&ensp;课程门数：` + itoa(len(rows)) + `&ensp;</div>
<table><thead>` + testHeaders + `</thead><tbody>` + strings.Join(rows, "\n") + `</tbody></table>
</body></html>`
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func row(classCode, className, course, credits, teacher, timePlace string) string {
	return `<tr><td>` + classCode + `</td><td>` + className + `</td><td>` + course +
		`</td><td>54</td><td>` + credits + `</td><td>初修</td><td>` + teacher +
		`</td><td>选中</td><td>否</td><td>否</td><td>` + timePlace + `</td><td></td></tr>`
}

func mustParse(t *testing.T, html string) map[string]any {
	t.Helper()
	out, err := Parse([]byte(html))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var dto map[string]any
	if err := json.Unmarshal([]byte(out), &dto); err != nil {
		t.Fatalf("decode DTO: %v\n%s", err, out)
	}
	return dto
}

func TestParseSyntheticDtoFields(t *testing.T) {
	dto := mustParse(t, sampleHTML(
		row("DEMO-001", "示例班", "[DEMO-001]合成课程甲", "3.0", "[T-01]示例教师甲", "1-18周 三[11-13] 示例楼甲306(120)"),
		row("DEMO-002", "示例班", "[DEMO-002]合成课程乙", "2.0", "[T-02]示例教师乙", "4-14周(双) 四[11-13]"),
		row("DEMO-003", "示例班", "无代码合成课程丙", "2.5", "[T-03]示例教师丙、[T-04]示例教师丁",
			"1-18周 五[6-8] 示例楼乙309(78)；11-16周 五[11-13]"),
	))

	if dto["schema_version"].(float64) != 1 {
		t.Fatalf("schema_version = %v", dto["schema_version"])
	}
	if dto["parser_version"] != "henu-html-v2" {
		t.Fatalf("parser_version = %v", dto["parser_version"])
	}
	if dto["semester_label"] != "2026-2027学年第一学期" {
		t.Fatalf("semester_label = %v", dto["semester_label"])
	}
	if dto["declared_course_count"].(float64) != 3 {
		t.Fatalf("declared_course_count = %v", dto["declared_course_count"])
	}
	stats := dto["stats"].(map[string]any)
	if stats["logical_courses"].(float64) != 3 || stats["sessions"].(float64) != 4 || stats["source_rows"].(float64) != 3 {
		t.Fatalf("stats = %v", stats)
	}
	if stats["unparsed_fragments"].(float64) != 0 {
		t.Fatalf("unparsed = %v", stats)
	}

	courses := dto["courses"].([]any)
	if len(courses) != 3 {
		t.Fatalf("courses = %d", len(courses))
	}
	first := courses[0].(map[string]any)
	if first["course_index"].(float64) != 0 {
		t.Fatalf("course_index not 0-based: %v", first["course_index"])
	}
	if first["name"] != "合成课程甲" {
		t.Fatalf("name = %v", first["name"])
	}
	if first["upstream_course_code"] != "DEMO-001" {
		t.Fatalf("upstream_course_code = %v", first["upstream_course_code"])
	}
	if first["credits"] != "3.0" {
		t.Fatalf("credits = %v", first["credits"])
	}
	teachers := first["teachers"].([]any)
	if len(teachers) != 1 || teachers[0] != "示例教师甲" {
		t.Fatalf("teachers = %v", teachers)
	}
	sessions := first["sessions"].([]any)
	s0 := sessions[0].(map[string]any)
	if s0["session_index"].(float64) != 0 || s0["weekday"].(float64) != 3 {
		t.Fatalf("session0 = %v", s0)
	}
	sections := s0["sections"].([]any)
	if len(sections) != 3 || sections[0].(float64) != 11 || sections[2].(float64) != 13 {
		t.Fatalf("sections = %v", sections)
	}
	if s0["location"] != "示例楼甲306" {
		t.Fatalf("location = %v (parser strips the capacity suffix)", s0["location"])
	}

	// Course without an upstream code must serialize null, not "".
	third := courses[2].(map[string]any)
	if v, ok := third["upstream_course_code"]; !ok || v != nil {
		t.Fatalf("missing code must be null, got %v (%v)", third["upstream_course_code"], ok)
	}
	// Two arrangements in one course, in order.
	if len(third["sessions"].([]any)) != 2 {
		t.Fatalf("course3 sessions = %d", len(third["sessions"].([]any)))
	}
	thirdTeachers := third["teachers"].([]any)
	if len(thirdTeachers) != 2 {
		t.Fatalf("course3 teachers = %v", thirdTeachers)
	}

	// Even-week expansion for course 2 and its missing-location warning.
	second := courses[1].(map[string]any)
	weeks := second["sessions"].([]any)[0].(map[string]any)["weeks"].([]any)
	if len(weeks) != 6 || weeks[0].(float64) != 4 || weeks[5].(float64) != 14 {
		t.Fatalf("even weeks = %v", weeks)
	}
	issues := dto["issues"].([]any)
	if len(issues) != 2 {
		t.Fatalf("issues = %v", issues)
	}
	issue := issues[0].(map[string]any)
	if issue["code"] != "MISSING_LOCATION" || issue["severity"] != "WARNING" {
		t.Fatalf("issue = %v", issue)
	}
	if issue["course_index"].(float64) != 1 {
		t.Fatalf("issue course_index = %v", issue["course_index"])
	}
	if issue["source_row"].(float64) < 1 {
		t.Fatalf("issue source_row = %v", issue["source_row"])
	}
}

func TestParseSplitsNonContiguousSectionsInDto(t *testing.T) {
	dto := mustParse(t, sampleHTML(
		row("DEMO-010", "示例班", "[DEMO-010]分段课", "1.0", "[T-10]教师", "1-18周 一[1,2,4] 示例楼"),
	))
	sessions := dto["courses"].([]any)[0].(map[string]any)["sessions"].([]any)
	if len(sessions) != 2 {
		t.Fatalf("sessions = %d, want 2", len(sessions))
	}
	s0 := sessions[0].(map[string]any)["sections"].([]any)
	s1 := sessions[1].(map[string]any)["sections"].([]any)
	if len(s0) != 2 || len(s1) != 1 || s1[0].(float64) != 4 {
		t.Fatalf("sections split: %v / %v", s0, s1)
	}
	if sessions[1].(map[string]any)["session_index"].(float64) != 1 {
		t.Fatal("session_index must follow split order")
	}
}

func TestParseGbkBytes(t *testing.T) {
	html := strings.Replace(sampleHTML(row("DEMO-021", "示例班", "[DEMO-021]编码课", "2.0", "[T-21]教师", "1-8周 二[3-4]")),
		`charset=utf-8`, `charset=GBK`, 1)
	encoder := simplifiedchinese.GBK.NewEncoder()
	gbk, _, err := transform.Bytes(encoder, []byte(html))
	if err != nil {
		t.Fatalf("encode GBK: %v", err)
	}
	out, err := Parse(gbk)
	if err != nil {
		t.Fatalf("Parse GBK: %v", err)
	}
	if !strings.Contains(out, `"name":"编码课"`) {
		t.Fatalf("GBK name lost: %s", out)
	}
}

func TestParseErrorCodesAreSafe(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want error
	}{
		{"bad gbk", []byte{0xcf, 0xb5, 0xff, 0xfe, 0x80, 0x81}, ErrInvalidEncoding},
		{"login page", []byte(`<html><body><div>河南大学统一身份认证</div><p>请输入密码和验证码</p></body></html>`), ErrLoginPage},
		{"empty table", []byte(sampleHTML()), ErrEmptyTimetable},
		{"binary", []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}, ErrUnsupportedFileType},
		{"garbage", []byte("<html><body>neither a timetable nor a login page</body></html>"), ErrUnrecognized},
	}
	for _, tc := range cases {
		out, err := Parse(tc.in)
		if !strings.Contains(err.Error(), tc.want.Error()) {
			t.Fatalf("%s: err = %v, want %v", tc.name, err, tc.want)
		}
		if out != "" {
			t.Fatalf("%s: output on error = %q", tc.name, out)
		}
	}
}

func TestDtoNeverHasNullSlices(t *testing.T) {
	out, err := Parse([]byte(sampleHTML(
		row("DEMO-031", "示例班", "[DEMO-031]无教师无教室", "1.0", "", "1-6周 三[1-2]"),
	)))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for _, key := range []string{`"teachers":[]`, `"sections":[`, `"weeks":[`, `"courses":[`, `"issues":[`} {
		if !strings.Contains(out, key) {
			t.Fatalf("output missing %s: %s", key, out)
		}
	}
	if strings.Contains(out, "null") {
		// nulls are only legal for optional scalar fields; with this fixture
		// every optional scalar is empty→null, so a bare scan is fine as long
		// as no ARRAY is null. Verify no "[]":null and no "null," inside arrays.
		if strings.Contains(out, `"teachers":null`) || strings.Contains(out, `"sessions":null`) ||
			strings.Contains(out, `"sections":null`) || strings.Contains(out, `"weeks":null`) ||
			strings.Contains(out, `"courses":null`) || strings.Contains(out, `"issues":null`) {
			t.Fatalf("null slice in DTO: %s", out)
		}
	}
}

func TestUnscheduledCourseDTOHasEmptySessionsAndScopedWarning(t *testing.T) {
	dto := mustParse(t, sampleHTML(
		row("D1", "示例班", "[D1]合成正常课", "2", "教师", "1-18周 一[1-2] 合成楼101"),
		row("D2", "示例班", "[D2]合成网课", "2", "教师", "1-18周")))
	stats := dto["stats"].(map[string]any)
	if stats["unparsed_fragments"].(float64) != 0 || stats["sessions"].(float64) != 1 {
		t.Fatal("unexpected counters")
	}
	courses := dto["courses"].([]any)
	if len(courses[1].(map[string]any)["sessions"].([]any)) != 0 {
		t.Fatal("unscheduled course has sessions")
	}
	issues := dto["issues"].([]any)
	issue := issues[0].(map[string]any)
	if len(issues) != 1 || issue["code"] != "UNSCHEDULED_COURSE" || issue["severity"] != "WARNING" || issue["course_index"].(float64) != 1 {
		t.Fatal("warning not scoped to skipped row")
	}
}
