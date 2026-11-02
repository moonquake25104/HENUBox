package henu

import "testing"

func TestUnscheduledCoursesDoNotBlockScheduledArrangements(t *testing.T) {
	for _, missing := range []string{"1-18周", "", "1-17周(单)", "2-18周（双）", "1-4,6-8周"} {
		r, err := ParseText(sampleHTML(
			row("D1", "示例班", "[D1]合成正常课", "2", "合成教师", "1-18周 一[1-2] 合成楼101"),
			row("D2", "示例班", "[D2]合成未排课", "2", "合成教师", missing)))
		if err != nil {
			t.Fatal(err)
		}
		if r.Stats.LogicalCourses != 2 || r.Stats.SourceRows != 2 || r.Stats.Sessions != 1 || r.Stats.UnparsedFragments != 0 {
			t.Fatalf("wrong counters: %+v", r.Stats)
		}
		if len(r.Courses[1].Sessions) != 0 || len(r.Issues) != 1 || r.Issues[0].Code != CodeUnscheduledCourse || r.Issues[0].Severity != SeverityWarning || !r.Issues[0].HasCourse || r.Issues[0].CourseIndex != 1 || HasBlocking(r.Issues) {
			t.Fatal("unscheduled course was not a scoped, nonblocking warning")
		}
	}
}

func TestMalformedTimesAreNotSilentlySkipped(t *testing.T) {
	for _, broken := range []string{"1-18周 一1-2节", "1-18周 一[未知]", "1-18周 08:30-09:15", "1-18周 教室101", "18-1周", "1-99周", "1-18周 一[1-2] 合成楼101；1-18周", "待确认的时间片段"} {
		r, err := ParseText(sampleHTML(row("D3", "示例班", "[D3]合成异常课", "2", "合成教师", broken)))
		if err != nil {
			t.Fatal(err)
		}
		if !HasBlocking(r.Issues) || r.Stats.UnparsedFragments == 0 {
			t.Fatal("malformed schedule must remain blocking")
		}
		for _, issue := range r.Issues {
			if issue.Code == CodeUnscheduledCourse {
				t.Fatal("malformed schedule must not be skipped")
			}
		}
	}
}

func TestScheduledCourseWithoutLocationIsRetained(t *testing.T) {
	r, err := ParseText(sampleHTML(row("D4", "示例班", "[D4]合成体育课", "1", "合成教师", "1-18周 二[7-8]")))
	if err != nil {
		t.Fatal(err)
	}
	if r.Stats.Sessions != 1 || len(r.Issues) != 1 || r.Issues[0].Code != CodeMissingLocation {
		t.Fatal("missing location alone cannot exclude a course")
	}
}
