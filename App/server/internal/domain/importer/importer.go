// Package importer ports the Rust text importers (src-tauri/src/importers.rs):
// parsing the school-system TSV clipboard format into course/exam candidates
// and the dedup rules used when persisting them.
package importer

import (
	"fmt"
	"strings"
	"time"
)

// ImportTextResult mirrors the Rust ImportTextResult: parsed/imported/skipped
// counts plus a Chinese summary message for direct display.
type ImportTextResult struct {
	Parsed   int    `json:"parsed"`
	Imported int    `json:"imported"`
	Skipped  int    `json:"skipped"`
	Message  string `json:"message"`
}

// FromCounts builds an ImportTextResult with the standard summary message.
func FromCounts(parsed, imported, skipped int) ImportTextResult {
	return ImportTextResult{
		Parsed:   parsed,
		Imported: imported,
		Skipped:  skipped,
		Message:  fmt.Sprintf("已解析 %d 条，导入 %d 条，跳过 %d 条重复记录。", parsed, imported, skipped),
	}
}

// CourseCandidate is a parsed course row ready for insertion.
type CourseCandidate struct {
	Name              string
	DayOfWeek         int64
	StartTime         string
	EndTime           string
	WeekPattern       string
	SemesterStartDate string
	Location          string
	Teacher           string
	Color             string
	Semester          string
}

// ExamCandidate is a parsed exam row ready for insertion.
type ExamCandidate struct {
	CourseName      string
	ExamDatetime    string
	ExamEndDatetime string
	Location        string
	Notes           string
	CourseID        *int64
	Semester        string
}

// importColors is the course color pool; the same course maps to the same
// color across import batches via inferColor.
var importColors = []string{
	"#3B82F6", "#10B981", "#F59E0B", "#EC4899", "#06B6D4", "#EF4444", "#7C8CC0", "#8B5CF6",
}

// InferColor picks a color from the pool by hashing the seed (course code +
// name), matching the Rust infer_color semantics.
func InferColor(seed string) string {
	var hash uint32
	for _, ch := range seed {
		hash = hash*31 + uint32(ch)
	}
	return importColors[int(hash)%len(importColors)]
}

func normalizeLine(raw string) string {
	return strings.TrimSpace(strings.ReplaceAll(raw, "\r", ""))
}

func normalizedLines(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if l := normalizeLine(line); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func isCourseHeaderLine(line string) bool {
	return strings.Contains(line, "课程号") ||
		strings.Contains(line, "课程名称") ||
		strings.Contains(line, "上课时间、地点") ||
		strings.Contains(line, "考试性质") ||
		strings.Contains(line, "缓考")
}

func isExamHeaderLine(line string) bool {
	return strings.Contains(line, "课程号") || strings.Contains(line, "考试时间") || strings.Contains(line, "考试地点")
}

func isCourseStartLine(line string) bool {
	cells := splitCells(line)
	if len(cells) < 4 {
		return false
	}
	first := []rune(cells[0])
	if len(first) == 0 || first[0] < '0' || first[0] > '9' {
		return false
	}
	if !allDigits(cells[1]) {
		return false
	}
	return cells[2] != ""
}

func isCourseMetadataLine(line string) bool {
	cells := splitCells(line)
	if len(cells) < 5 {
		return false
	}
	_, err := parseFloat(cells[0])
	return err == nil
}

func isTimeLocationLine(line string) bool {
	cells := splitCells(line)
	if len(cells) < 4 {
		return false
	}
	return strings.Contains(cells[0], "周") && matchesDayLabel(cells[1]) != 0
}

func isTerminatorLine(line string) bool {
	for _, cell := range splitCells(line) {
		if cell == "查看" {
			return true
		}
	}
	return false
}

func splitCells(line string) []string {
	parts := strings.Split(line, "\t")
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = strings.TrimSpace(p)
	}
	return out
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}

// matchesDayLabel maps Chinese weekday labels to ISO weekday numbers (1-7).
func matchesDayLabel(label string) int64 {
	switch label {
	case "星期一":
		return 1
	case "星期二":
		return 2
	case "星期三":
		return 3
	case "星期四":
		return 4
	case "星期五":
		return 5
	case "星期六":
		return 6
	case "星期日", "星期天":
		return 7
	}
	return 0
}

func normalizeTeacher(lines []string) string {
	var parts []string
	for _, line := range lines {
		if l := normalizeLine(line); l != "" {
			parts = append(parts, l)
		}
	}
	return strings.Join(parts, " / ")
}

// ParseTimeRange parses a lesson-slot text such as "上午34节", "下午5-7节",
// "晚9-11节" or "中午第1节" into (HH:mm, HH:mm) using the hard-coded school
// bell schedule.
func ParseTimeRange(slotText string) (string, string, bool) {
	normalized := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, slotText)

	if normalized == "中午第1节" {
		return "12:10", "12:55", true
	}

	var period, rest string
	switch {
	case strings.HasPrefix(normalized, "上午"):
		period, rest = "上午", strings.TrimPrefix(normalized, "上午")
	case strings.HasPrefix(normalized, "下午"):
		period, rest = "下午", strings.TrimPrefix(normalized, "下午")
	case strings.HasPrefix(normalized, "晚"):
		period, rest = "晚", strings.TrimPrefix(normalized, "晚")
	default:
		return "", "", false
	}

	digits := strings.TrimSuffix(rest, "节")
	digits = strings.TrimPrefix(digits, "第")
	startIndex, endIndex, ok := parseLessonIndexRange(digits)
	if !ok {
		return "", "", false
	}
	if !lessonMatchesPeriod(period, startIndex) || !lessonMatchesPeriod(period, endIndex) {
		return "", "", false
	}
	startMinutes, _, ok := lessonIndexToMinutes(startIndex)
	if !ok {
		return "", "", false
	}
	_, endMinutes, ok := lessonIndexToMinutes(endIndex)
	if !ok {
		return "", "", false
	}
	return minutesToTime(startMinutes), minutesToTime(endMinutes), true
}

func parseLessonIndexRange(digits string) (int64, int64, bool) {
	if start, end, found := strings.Cut(digits, "-"); found {
		startIndex, err1 := parseInt(start)
		endIndex, err2 := parseInt(end)
		if err1 != nil || err2 != nil || startIndex > endIndex {
			return 0, 0, false
		}
		return startIndex, endIndex, true
	}

	if single, err := parseInt(digits); err == nil {
		if _, _, ok := lessonIndexToMinutes(single); ok {
			return single, single, true
		}
	}

	runes := []rune(digits)
	if len(runes) != 2 {
		return 0, 0, false
	}
	startIndex := int64(runes[0] - '0')
	endIndex := int64(runes[1] - '0')
	if startIndex > endIndex {
		return 0, 0, false
	}
	return startIndex, endIndex, true
}

func parseInt(s string) (int64, error) {
	var v int64
	_, err := fmt.Sscanf(s, "%d", &v)
	return v, err
}

func lessonMatchesPeriod(period string, index int64) bool {
	switch period {
	case "上午":
		return index >= 1 && index <= 4
	case "下午":
		return index >= 5 && index <= 8
	case "晚":
		return index >= 9 && index <= 11
	}
	return false
}

// lessonIndexToMinutes maps lesson index to (startMinutes, endMinutes) using
// the hard-coded school bell schedule (1-4 morning, 5-8 afternoon, 9-11 evening).
func lessonIndexToMinutes(index int64) (int64, int64, bool) {
	switch index {
	case 1:
		return 8*60 + 30, 9*60 + 15, true
	case 2:
		return 9*60 + 25, 10*60 + 10, true
	case 3:
		return 10*60 + 30, 11*60 + 15, true
	case 4:
		return 11*60 + 25, 12*60 + 10, true
	case 5:
		return 14*60 + 30, 15*60 + 15, true
	case 6:
		return 15*60 + 25, 16*60 + 10, true
	case 7:
		return 16*60 + 30, 17*60 + 15, true
	case 8:
		return 17*60 + 25, 18*60 + 10, true
	case 9:
		return 19 * 60, 19*60 + 45, true
	case 10:
		return 19*60 + 55, 20*60 + 40, true
	case 11:
		return 20*60 + 50, 21*60 + 35, true
	}
	return 0, 0, false
}

func minutesToTime(totalMinutes int64) string {
	return fmt.Sprintf("%02d:%02d", totalMinutes/60, totalMinutes%60)
}

// ParseCourseImportText parses the school-system TSV course table text into
// course candidates. It returns an error when nothing importable is found.
func ParseCourseImportText(text, semester, semesterStartDate string) ([]CourseCandidate, error) {
	lines := normalizedLines(text)

	var courses []CourseCandidate
	index := 0
	for index < len(lines) {
		line := lines[index]
		if isCourseHeaderLine(line) || isTerminatorLine(line) {
			index++
			continue
		}
		if !isCourseStartLine(line) {
			index++
			continue
		}

		startCells := splitCells(line)
		courseCode := startCells[0]
		courseName := startCells[2]
		teacherLines := []string{startCells[3]}
		index++

		for index < len(lines) && !isCourseMetadataLine(lines[index]) {
			current := lines[index]
			if isTimeLocationLine(current) || isCourseStartLine(current) || isTerminatorLine(current) {
				break
			}
			teacherLines = append(teacherLines, current)
			index++
		}

		if index < len(lines) && isCourseMetadataLine(lines[index]) {
			index++
		}

		teacher := normalizeTeacher(teacherLines)
		for index < len(lines) && isTimeLocationLine(lines[index]) {
			cells := splitCells(lines[index])
			dayOfWeek := matchesDayLabel(cells[1])
			if dayOfWeek == 0 {
				return nil, fmt.Errorf("无法解析星期字段: %s", cells[1])
			}
			startTime, endTime, ok := ParseTimeRange(cells[2])
			if !ok {
				return nil, fmt.Errorf("无法解析节次字段: %s", cells[2])
			}
			location := ""
			if len(cells) > 3 {
				location = cells[3]
			}
			courses = append(courses, CourseCandidate{
				Name:              courseName,
				DayOfWeek:         dayOfWeek,
				StartTime:         startTime,
				EndTime:           endTime,
				WeekPattern:       cells[0],
				SemesterStartDate: semesterStartDate,
				Location:          location,
				Teacher:           teacher,
				Color:             InferColor(courseCode + ":" + courseName),
				Semester:          semester,
			})
			index++
		}

		for index < len(lines) && isTerminatorLine(lines[index]) {
			index++
		}
	}

	if len(courses) == 0 {
		return nil, fmt.Errorf("未识别到可导入的课程，请确认复制内容来自教务系统课表表格。")
	}
	return courses, nil
}

// ParseExamImportText parses the school-system TSV exam table text into exam
// candidates. It returns an error when nothing importable is found.
func ParseExamImportText(text, semester string) ([]ExamCandidate, error) {
	lines := normalizedLines(text)

	var exams []ExamCandidate
	for _, line := range lines {
		if isExamHeaderLine(line) {
			continue
		}
		cells := splitCells(line)
		if len(cells) < 4 || cells[1] == "" || cells[2] == "" {
			continue
		}
		examDatetime, examEndDatetime, err := parseExamTimeRange(cells[2])
		if err != nil {
			return nil, err
		}
		location := ""
		if len(cells) > 3 {
			location = cells[3]
		}
		notes := ""
		if len(cells) > 4 {
			notes = cells[4]
		}
		exams = append(exams, ExamCandidate{
			CourseName:      cells[1],
			ExamDatetime:    examDatetime,
			ExamEndDatetime: examEndDatetime,
			Location:        location,
			Notes:           notes,
			CourseID:        nil,
			Semester:        semester,
		})
	}

	if len(exams) == 0 {
		return nil, fmt.Errorf("未识别到可导入的考试，请确认复制内容来自教务系统考试表格。")
	}
	return exams, nil
}

// parseExamTimeRange parses "YYYY-MM-DD HH:MM--YYYY-MM-DD HH:MM" (or with a
// single dash separator) into UTC RFC3339 start/end strings, interpreting the
// naive times as +08:00.
func parseExamTimeRange(raw string) (string, string, error) {
	normalized := strings.TrimSpace(raw)
	if len(normalized) < 16 {
		return "", "", fmt.Errorf("无法解析考试时间字段: %s", raw)
	}
	startRaw := normalized[:16]
	endRaw := strings.TrimLeft(normalized[16:], " \t-—–－")
	endRaw = strings.TrimSpace(endRaw)
	if endRaw == "" {
		return "", "", fmt.Errorf("无法解析考试时间字段: %s", raw)
	}

	startNaive, err := time.ParseInLocation("2006-01-02 15:04", strings.TrimSpace(startRaw), chinaTZ())
	if err != nil {
		return "", "", fmt.Errorf("无法解析考试开始时间: %s", raw)
	}

	var endNaive time.Time
	if strings.Contains(endRaw, " ") {
		endNaive, err = time.ParseInLocation("2006-01-02 15:04", endRaw, chinaTZ())
		if err != nil {
			return "", "", fmt.Errorf("无法解析考试结束时间: %s", raw)
		}
	} else {
		date := startNaive.Format("2006-01-02")
		endNaive, err = time.ParseInLocation("2006-01-02 15:04", date+" "+endRaw, chinaTZ())
		if err != nil {
			return "", "", fmt.Errorf("无法解析考试结束时间: %s", raw)
		}
	}
	if endNaive.Before(startNaive) {
		return "", "", fmt.Errorf("考试结束时间早于开始时间: %s", raw)
	}

	return startNaive.UTC().Format("2006-01-02T15:04:05Z"), endNaive.UTC().Format("2006-01-02T15:04:05Z"), nil
}

func chinaTZ() *time.Location {
	return time.FixedZone("CST", 8*3600)
}
