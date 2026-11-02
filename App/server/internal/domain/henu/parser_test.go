package henu

import (
	"errors"
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
<html><head><meta http-equiv="Content-Type" content="text/html; charset=GBK">
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

func TestParseSyntheticTimetable(t *testing.T) {
	html := sampleHTML(
		row("DEMO-001", "示例班", "[DEMO-001]合成课程甲", "3.0", "[T-01]示例教师甲", "1-18周 三[11-13] 示例楼甲306(120)"),
		row("DEMO-002", "示例班", "[DEMO-002]合成课程乙", "2.0", "[T-02]示例教师乙", "4-14周(双) 四[11-13]"),
		row("DEMO-003", "示例班", "无代码合成课程丙", "2.5", "[T-03]示例教师丙、[T-04]示例教师丁",
			"1-18周 五[6-8] 示例楼乙309(78)；11-16周 五[11-13]"),
	)

	res, err := ParseText(html)
	if err != nil {
		t.Fatalf("ParseText: %v", err)
	}
	if res.SourceSemesterLabel != "2026-2027学年第一学期" {
		t.Errorf("semester label = %q", res.SourceSemesterLabel)
	}
	if res.DeclaredCourseCount != 3 {
		t.Errorf("declared count = %d, want 3", res.DeclaredCourseCount)
	}
	if res.Stats.LogicalCourses != 3 || res.Stats.SourceRows != 3 {
		t.Errorf("stats = %+v", res.Stats)
	}
	if res.Stats.Sessions != 4 {
		t.Errorf("sessions = %d, want 4 (two rows with one arrangement, one row with two)", res.Stats.Sessions)
	}
	if len(res.Issues) != 2 {
		t.Fatalf("expected two MISSING_LOCATION warnings, got %+v", res.Issues)
	}
	for _, issue := range res.Issues {
		if issue.Code != CodeMissingLocation || issue.Severity != SeverityWarning {
			t.Errorf("unexpected issue %+v", issue)
		}
	}

	first := res.Courses[0]
	if first.Name != "合成课程甲" || first.UpstreamCourseCode != "DEMO-001" {
		t.Errorf("course = %+v", first)
	}
	if first.Credits == nil || *first.Credits != "3.0" {
		t.Errorf("credits = %v", first.Credits)
	}
	if len(first.Teachers) != 1 || first.Teachers[0] != "示例教师甲" {
		t.Errorf("teachers = %v", first.Teachers)
	}

	second := res.Courses[1]
	if len(second.Sessions) != 1 || second.Sessions[0].Weeks[0] != 4 || len(second.Sessions[0].Weeks) != 6 {
		t.Errorf("even-week session = %+v", second.Sessions)
	}

	third := res.Courses[2]
	if len(third.Teachers) != 2 {
		t.Errorf("teachers = %v", third.Teachers)
	}
	if len(third.Sessions) != 2 {
		t.Fatalf("sessions = %+v", third.Sessions)
	}
	if third.Sessions[0].Location != "示例楼乙309" || third.Sessions[1].Location != "" {
		t.Errorf("locations = %q / %q", third.Sessions[0].Location, third.Sessions[1].Location)
	}
}

func TestParseSplitsNonContiguousSections(t *testing.T) {
	html := sampleHTML(
		row("D-1", "示例班", "[D-1]合成课程", "2.0", "[T]示例教师", "1-18周 二[1,2,4] 示例楼101(60)"),
	)
	res, err := ParseText(html)
	if err != nil {
		t.Fatalf("ParseText: %v", err)
	}
	sessions := res.Courses[0].Sessions
	if len(sessions) != 2 {
		t.Fatalf("sessions = %+v, want 2", sessions)
	}
	if len(sessions[0].Sections) != 2 || len(sessions[1].Sections) != 1 || sessions[1].Sections[0] != 4 {
		t.Errorf("sections = %v / %v", sessions[0].Sections, sessions[1].Sections)
	}
}

func TestParseKeepsUnknownFragmentAsBlocking(t *testing.T) {
	html := sampleHTML(
		row("D-2", "示例班", "[D-2]合成课程", "2.0", "[T]示例教师", "1-18周 三[11-13] 示例楼101；无法识别的片段"),
	)
	res, err := ParseText(html)
	if err != nil {
		t.Fatalf("ParseText: %v", err)
	}
	if !HasBlocking(res.Issues) {
		t.Fatalf("expected a blocking issue, got %+v", res.Issues)
	}
	if res.Stats.UnparsedFragments != 1 {
		t.Errorf("unparsed fragments = %d, want 1", res.Stats.UnparsedFragments)
	}
	if res.Stats.Sessions != 1 {
		t.Errorf("sessions = %d, want the valid arrangement only", res.Stats.Sessions)
	}
}

func TestParseGBKDocument(t *testing.T) {
	html := sampleHTML(
		row("D-3", "示例班", "[D-3]合成课程", "1.0", "[T]示例教师", "1-2周 一[1-2] 示例楼202(40)"),
	)
	encoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewEncoder(), []byte(html))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	res, err := Parse(encoded)
	if err != nil {
		t.Fatalf("Parse(GBK): %v", err)
	}
	if len(res.Courses) != 1 || res.Courses[0].Name != "合成课程" {
		t.Fatalf("courses = %+v", res.Courses)
	}
}

func TestParseUTF8WithoutDeclaration(t *testing.T) {
	html := strings.Replace(sampleHTML(
		row("D-4", "示例班", "[D-4]合成课程", "1.0", "[T]示例教师", "1-2周 一[1-2]"),
	), `charset=GBK`, `charset=utf-8`, 1)
	if _, err := Parse([]byte(html)); err != nil {
		t.Fatalf("Parse(UTF-8): %v", err)
	}
}

func TestDecodeRejectsBinaryAndBadEncoding(t *testing.T) {
	ole := append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, []byte("rest")...)
	if _, err := Decode(ole); !errors.Is(err, ErrUnsupportedFileType) {
		t.Errorf("OLE: err = %v, want ErrUnsupportedFileType", err)
	}
	zip := append([]byte{0x50, 0x4B, 0x03, 0x04}, []byte("rest")...)
	if _, err := Decode(zip); !errors.Is(err, ErrUnsupportedFileType) {
		t.Errorf("ZIP: err = %v, want ErrUnsupportedFileType", err)
	}

	// Declares GBK but contains an invalid lead byte sequence.
	bad := append([]byte(`<html><meta charset="gbk"><table><tr><td>`),
		0x81, 0x20, 0x80, 0x80)
	bad = append(bad, []byte(`</td></tr></table></html>`)...)
	if _, err := Decode(bad); err == nil {
		t.Errorf("expected an encoding error")
	}
}

func TestParseRejectsLoginAndUnknownDocuments(t *testing.T) {
	login := `<html><body><div>统一身份认证</div><div>用户登录</div>
<p>请输入密码和验证码</p></body></html>`
	if _, err := ParseText(login); !errors.Is(err, ErrLoginPage) {
		t.Errorf("login page: err = %v", err)
	}

	unknown := `<html><body><table><tr><td>通知公告</td></tr></table></body></html>`
	if _, err := ParseText(unknown); !errors.Is(err, ErrUnrecognizedDoc) {
		t.Errorf("unknown: err = %v", err)
	}
}

func TestParseEmptyTimetable(t *testing.T) {
	html := sampleHTML()
	_, err := ParseText(html)
	if !errors.Is(err, ErrEmptyTimetable) {
		t.Fatalf("err = %v, want ErrEmptyTimetable", err)
	}
}
