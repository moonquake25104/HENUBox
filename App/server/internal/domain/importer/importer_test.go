package importer

import "testing"

func TestParseCourseImportText(t *testing.T) {
	text := "课程号\t课程\n序号\t课程名称\t任课教师\t学 分\t选课属性\t考核方式\t考试\n性质\t是否\n缓考\t上课时间、地点\t教材\t教学记录\t过程性成绩\n2043056\t3\t自动控制原理\t李红信\n3\t必修\t未确定\t正常考试\t非缓考\n1-17周全周\t星期三\t上午34节\t秦岭堂A114\n1-17周单周\t星期二\t晚9-10节\t天山堂A312\n \t查看\t查看"

	courses, err := ParseCourseImportText(text, "2026S1", "2026-02-24")
	if err != nil {
		t.Fatalf("parse course import: %v", err)
	}
	if len(courses) != 2 {
		t.Fatalf("expected 2 courses, got %d", len(courses))
	}
	if courses[0].Name != "自动控制原理" {
		t.Fatalf("course[0].name = %q, want 自动控制原理", courses[0].Name)
	}
	if courses[0].WeekPattern != "1-17周全周" {
		t.Fatalf("course[0].week_pattern = %q, want 1-17周全周", courses[0].WeekPattern)
	}
	if courses[0].StartTime != "10:30" || courses[0].EndTime != "12:10" {
		t.Fatalf("course[0] time = %s-%s, want 10:30-12:10", courses[0].StartTime, courses[0].EndTime)
	}
	if courses[0].SemesterStartDate != "2026-02-24" {
		t.Fatalf("course[0].semester_start_date = %q, want 2026-02-24", courses[0].SemesterStartDate)
	}
	if courses[1].DayOfWeek != 2 {
		t.Fatalf("course[1].day_of_week = %d, want 2", courses[1].DayOfWeek)
	}
	if courses[1].StartTime != "19:00" || courses[1].EndTime != "20:40" {
		t.Fatalf("course[1] time = %s-%s, want 19:00-20:40", courses[1].StartTime, courses[1].EndTime)
	}
}

func TestParseTimeRangeUsesSchoolLessonTimes(t *testing.T) {
	singles := []struct {
		raw, start, end string
	}{
		{"上午1节", "08:30", "09:15"},
		{"上午2节", "09:25", "10:10"},
		{"上午3节", "10:30", "11:15"},
		{"上午4节", "11:25", "12:10"},
		{"下午5节", "14:30", "15:15"},
		{"下午6节", "15:25", "16:10"},
		{"下午7节", "16:30", "17:15"},
		{"下午8节", "17:25", "18:10"},
		{"晚9节", "19:00", "19:45"},
		{"晚10节", "19:55", "20:40"},
		{"晚11节", "20:50", "21:35"},
	}
	for _, tc := range singles {
		start, end, ok := ParseTimeRange(tc.raw)
		if !ok || start != tc.start || end != tc.end {
			t.Errorf("ParseTimeRange(%q) = (%q, %q, %v), want (%q, %q, true)", tc.raw, start, end, ok, tc.start, tc.end)
		}
	}

	ranges := []struct {
		raw, start, end string
	}{
		{"上午12节", "08:30", "10:10"},
		{"上午34节", "10:30", "12:10"},
		{"下午56节", "14:30", "16:10"},
		{"下午78节", "16:30", "18:10"},
		{"下午5-7节", "14:30", "17:15"},
		{"晚9-11节", "19:00", "21:35"},
	}
	for _, tc := range ranges {
		start, end, ok := ParseTimeRange(tc.raw)
		if !ok || start != tc.start || end != tc.end {
			t.Errorf("ParseTimeRange(%q) = (%q, %q, %v), want (%q, %q, true)", tc.raw, start, end, ok, tc.start, tc.end)
		}
	}
}

func TestParseExamImportText(t *testing.T) {
	text := "课程号\t课程名称\t考试时间\t考试地点\t考试性质\n2043056\t自动控制原理\t2026-07-06 16:00--18:00\t天山堂A409\t正常考试"
	exams, err := ParseExamImportText(text, "2026S1")
	if err != nil {
		t.Fatalf("parse exam import: %v", err)
	}
	if len(exams) != 1 {
		t.Fatalf("expected 1 exam, got %d", len(exams))
	}
	if exams[0].CourseName != "自动控制原理" {
		t.Fatalf("exam.course_name = %q, want 自动控制原理", exams[0].CourseName)
	}
	if exams[0].Location != "天山堂A409" {
		t.Fatalf("exam.location = %q, want 天山堂A409", exams[0].Location)
	}
	if exams[0].Notes != "正常考试" {
		t.Fatalf("exam.notes = %q, want 正常考试", exams[0].Notes)
	}
	if exams[0].Semester != "2026S1" {
		t.Fatalf("exam.semester = %q, want 2026S1", exams[0].Semester)
	}
	if exams[0].ExamDatetime != "2026-07-06T08:00:00Z" {
		t.Fatalf("exam.exam_datetime = %q, want 2026-07-06T08:00:00Z", exams[0].ExamDatetime)
	}
	if exams[0].ExamEndDatetime != "2026-07-06T10:00:00Z" {
		t.Fatalf("exam.exam_end_datetime = %q, want 2026-07-06T10:00:00Z", exams[0].ExamEndDatetime)
	}
}

func TestParseExamImportTextWithSingleDashSeparator(t *testing.T) {
	text := "2043056\t自动控制原理\t2026-07-06 16:00-18:00\t天山堂A409\t正常考试"
	exams, err := ParseExamImportText(text, "2026S1")
	if err != nil {
		t.Fatalf("parse exam import: %v", err)
	}
	if exams[0].ExamDatetime != "2026-07-06T08:00:00Z" {
		t.Fatalf("exam.exam_datetime = %q, want 2026-07-06T08:00:00Z", exams[0].ExamDatetime)
	}
	if exams[0].ExamEndDatetime != "2026-07-06T10:00:00Z" {
		t.Fatalf("exam.exam_end_datetime = %q, want 2026-07-06T10:00:00Z", exams[0].ExamEndDatetime)
	}
}

func TestParseCourseImportTextEmptyReturnsError(t *testing.T) {
	if _, err := ParseCourseImportText("无关内容", "2026S1", "2026-02-24"); err == nil {
		t.Fatal("expected error for unparseable course text")
	}
}

func TestParseExamImportTextEmptyReturnsError(t *testing.T) {
	if _, err := ParseExamImportText("无关内容", "2026S1"); err == nil {
		t.Fatal("expected error for unparseable exam text")
	}
}

func TestInferColorStableAcrossBatches(t *testing.T) {
	a := InferColor("2043056:自动控制原理")
	b := InferColor("2043056:自动控制原理")
	if a != b {
		t.Fatalf("infer_color must be stable, got %q vs %q", a, b)
	}
	if a == "" || a[0] != '#' {
		t.Fatalf("infer_color should return a hex color, got %q", a)
	}
}
