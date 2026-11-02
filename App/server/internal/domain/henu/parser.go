package henu

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// ParserVersion identifies the normalisation rules; bump it whenever the
// output shape of Parse changes.
const ParserVersion = "henu-html-v2"

var (
	ErrLoginPage         = errors.New("document looks like a login page")
	ErrUnrecognizedDoc   = errors.New("document is not a recognized timetable export")
	ErrEmptyTimetable    = errors.New("timetable has no course rows")
	courseCodeRe         = regexp.MustCompile(`^\s*\[([^\]]{1,64})\]\s*(.*)$`)
	semesterLabelRe      = regexp.MustCompile(`20\d{2}\s*[-—]\s*20\d{2}\s*学年\s*第?\s*[一二三四1-4]\s*学期`)
	teacherSplitRe       = regexp.MustCompile(`[、,，;；]`)
	courseCountSummaryRe = regexp.MustCompile(`课程门数[:：]\s*(\d+)`)
)

// Session is one arrangement rule: a weekday, a consecutive section run and a
// set of weeks. Non-consecutive sections are split by the parser.
type Session struct {
	Weekday  int
	Sections []int
	Weeks    []int
	Location string
	Teacher  string
}

// Course is one logical course from one source row. One row may yield several
// sessions (multiple arrangements in the time/place cell).
type Course struct {
	Name               string
	UpstreamCourseCode string
	TeachingClassCode  string
	TeachingClassName  string
	Credits            *string
	Teachers           []string
	Sessions           []Session
	Row                int
}

// Stats separates logical courses, arrangements and source rows; the caller
// must never equate them with the "13 courses" summary of the export.
type Stats struct {
	LogicalCourses    int `json:"logical_courses"`
	Sessions          int `json:"sessions"`
	SourceRows        int `json:"source_rows"`
	UnparsedFragments int `json:"unparsed_fragments"`
}

// Result is the parser output, independent of HTTP and database concerns.
type Result struct {
	Courses             []Course
	Issues              []Issue
	SourceSemesterLabel string
	DeclaredCourseCount int // from the "课程门数：N" summary, 0 when absent
	Stats               Stats
	ParserVersion       string
}

// Parse decodes and parses an exported HENU timetable (HTML, usually with a
// .xls extension). It never executes scripts and never fetches external
// resources: the HTML is only tokenized.
func Parse(raw []byte) (*Result, error) {
	text, err := Decode(raw)
	if err != nil {
		return nil, err
	}
	return ParseText(text)
}

// ParseText is Parse for already-decoded text (useful in tests).
func ParseText(text string) (*Result, error) {
	doc, err := parseHTMLDocument(text)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnrecognizedDoc, err)
	}

	plain := textOfDocument(doc)
	if looksLikeLogin(plain) {
		return nil, ErrLoginPage
	}

	tbl, headerRow, columns := findTimetable(doc)
	if tbl == nil {
		return nil, ErrUnrecognizedDoc
	}

	res := &Result{
		Courses:             []Course{},
		Issues:              []Issue{},
		SourceSemesterLabel: semesterLabelRe.FindString(plain),
		ParserVersion:       ParserVersion,
		Stats:               Stats{},
	}
	if m := courseCountSummaryRe.FindStringSubmatch(plain); m != nil {
		fmt.Sscanf(m[1], "%d", &res.DeclaredCourseCount)
	}

	for rowIdx := headerRow + 1; rowIdx < len(tbl.Rows); rowIdx++ {
		row := tbl.Rows[rowIdx]
		courseCell := cellAt(row, columns.course)
		if strings.TrimSpace(courseCell.Text) == "" {
			continue
		}
		res.Stats.SourceRows++

		course := Course{Row: rowIdx + 1}
		name := courseCell.Text
		if m := courseCodeRe.FindStringSubmatch(name); m != nil {
			course.UpstreamCourseCode = strings.TrimSpace(m[1])
			name = m[2]
		}
		course.Name = strings.TrimSpace(name)
		if course.Name == "" {
			res.Issues = append(res.Issues, Issue{
				Code: CodeMissingCourseName, Severity: SeverityBlocking,
				Message: "课程名称缺失", Row: rowIdx + 1,
			})
			continue
		}

		course.Credits = parseCredits(cellAt(row, columns.credits).Text)
		course.Teachers = parseTeachers(cellAt(row, columns.teacher).Text)
		course.TeachingClassCode = strings.TrimSpace(cellAt(row, columns.classCode).Text)
		course.TeachingClassName = strings.TrimSpace(cellAt(row, columns.className).Text)

		timeText := cellAt(row, columns.time).Text
		if isUnscheduledCell(timeText) {
			res.Issues = append(res.Issues, Issue{
				Code: CodeUnscheduledCourse, Severity: SeverityWarning,
				Message: "该课程未安排上课时间，将跳过课表导入", Row: rowIdx + 1,
				CourseIndex: len(res.Courses), HasCourse: true,
			})
			res.Courses = append(res.Courses, course)
			continue
		}

		courseIdx := len(res.Courses)
		for _, fragment := range SplitScheduleFragments(timeText) {
			frag, issue := ParseScheduleFragment(fragment)
			if issue != nil {
				issue.Row = rowIdx + 1
				issue.CourseIndex = courseIdx
				issue.HasCourse = true
				res.Issues = append(res.Issues, *issue)
				res.Stats.UnparsedFragments++
				continue
			}
			if len(frag.Sections) == 0 {
				continue
			}
			if strings.TrimSpace(frag.Location) == "" {
				res.Issues = append(res.Issues, Issue{
					Code: CodeMissingLocation, Severity: SeverityWarning,
					Message: "该安排没有教室", Row: rowIdx + 1,
					CourseIndex: courseIdx, HasCourse: true,
				})
			}
			for _, run := range ConsecutiveRuns(frag.Sections) {
				course.Sessions = append(course.Sessions, Session{
					Weekday:  frag.Weekday,
					Sections: run,
					Weeks:    frag.Weeks,
					Location: frag.Location,
					Teacher:  strings.Join(course.Teachers, "、"),
				})
			}
		}
		res.Stats.Sessions += len(course.Sessions)
		res.Courses = append(res.Courses, course)
	}

	res.Stats.LogicalCourses = len(res.Courses)
	if res.Stats.SourceRows == 0 {
		return res, ErrEmptyTimetable
	}
	return res, nil
}

// A whole empty/time-place cell or a valid weeks-only cell has no timetable
// placement. Do not classify broken weekdays, brackets, clocks or unknown
// fragments as online courses: those remain blocking parsing errors.
var weeksOnlyCellRe = regexp.MustCompile(`^[0-9第周单双()（）,，、\-–—~～\s]+$`)

func isUnscheduledCell(raw string) bool {
	text := normalizeSpace(raw)
	if text == "" {
		return true
	}
	if !strings.Contains(text, "周") || !weeksOnlyCellRe.MatchString(text) {
		return false
	}
	weeks, issue := parseWeeks(text)
	return issue == nil && len(weeks) > 0
}

func looksLikeLogin(text string) bool {
	markers := []string{"统一身份认证", "用户登录", "请输入密码", "验证码", "登录失败", "会话已过期"}
	hits := 0
	for _, m := range markers {
		if strings.Contains(text, m) {
			hits++
		}
	}
	// A timetable page can legitimately contain the word 登录 in a footer, so
	// require several markers and the absence of timetable headers.
	return hits >= 2 && !strings.Contains(text, "上课时间地点")
}

type columnMap struct {
	course    int
	teacher   int
	time      int
	credits   int
	classCode int
	className int
}

// findTimetable picks the table that has a recognizable header row and the
// most data rows below it. Column positions come from the header text, never
// from fixed indexes.
func findTimetable(doc *html.Node) (*table, int, columnMap) {
	tables := collectTables(doc)
	var (
		best       *table
		bestHeader = -1
		bestCols   columnMap
		bestRows   = -1
	)
	for _, t := range tables {
		for i, row := range t.Rows {
			cols, ok := headerColumns(row)
			if !ok {
				continue
			}
			if n := len(t.Rows) - i - 1; n > bestRows {
				best, bestHeader, bestCols, bestRows = t, i, cols, n
			}
			break
		}
	}
	if best == nil {
		return nil, 0, columnMap{}
	}
	return best, bestHeader, bestCols
}

func headerColumns(row []tableCell) (columnMap, bool) {
	cols := columnMap{course: -1, teacher: -1, time: -1, credits: -1, classCode: -1, className: -1}
	hasCourse, hasTime := false, false
	for i, c := range row {
		switch normalizeHeader(c.Text) {
		case "课程":
			cols.course, hasCourse = i, true
		case "任课教师":
			cols.teacher = i
		case "上课时间地点":
			cols.time, hasTime = i, true
		case "学分":
			cols.credits = i
		case "上课班级代码":
			cols.classCode = i
		case "上课班级名称":
			cols.className = i
		}
	}
	return cols, hasCourse && hasTime
}

func normalizeHeader(s string) string {
	replacer := strings.NewReplacer(" ", "", "\u00a0", "", "\u2002", "", "\u2003", "", "\t", "")
	return replacer.Replace(strings.TrimSpace(s))
}

func cellAt(row []tableCell, idx int) tableCell {
	if idx < 0 || idx >= len(row) {
		return tableCell{}
	}
	return row[idx]
}

func parseCredits(raw string) *string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	for _, r := range trimmed {
		if (r < '0' || r > '9') && r != '.' {
			return nil
		}
	}
	return &trimmed
}

func parseTeachers(raw string) []string {
	var out []string
	for _, part := range teacherSplitRe.Split(raw, -1) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if m := courseCodeRe.FindStringSubmatch(part); m != nil {
			part = strings.TrimSpace(m[2])
		}
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
