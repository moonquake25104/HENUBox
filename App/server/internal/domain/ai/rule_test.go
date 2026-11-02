package ai

import (
	"strings"
	"testing"

	"kairos/server/internal/domain/calendar"
)

func sampleBriefing() *calendar.TodayBriefingResponse {
	return &calendar.TodayBriefingResponse{
		Date:         "2026-07-31",
		WeekdayLabel: "周五",
		Courses: calendar.TodayCourses{
			TodayCount: 2,
			NextCourse: &calendar.NextCourse{
				Title:     "操作系统",
				StartTime: "10:10",
				EndTime:   "11:50",
				Location:  "天山堂A205",
			},
		},
		Tasks: calendar.TodayTasks{
			OverdueCount:  1,
			DueTodayCount: 2,
			Spotlight: []calendar.TaskSpotlight{
				{ID: 1, Title: "交操作系统实验报告", Priority: "high", DueDate: strPtr("2026-07-31")},
			},
		},
		Exam: &calendar.UpcomingExam{
			ID:           1,
			CourseName:   "高数期末",
			ExamDatetime: "2026-08-05T08:00:00+08:00",
			DaysUntil:    5,
			Location:     "天山堂B103",
		},
		Pomodoro: calendar.PomodoroBriefing{
			IsRunning:         false,
			Phase:             "work",
			RemainingSeconds:  0,
			CompletedSessions: 2,
		},
		Phase: calendar.PhaseBriefing{
			PhaseType:                "teaching",
			TermLabel:                strPtr("2026S1"),
			CurrentWeek:              int64Ptr(17),
			CoursesVisible:           true,
			ExamNotificationsEnabled: true,
			PomodoroProfile:          "default",
		},
	}
}

func strPtr(s string) *string { return &s }

func int64Ptr(v int64) *int64 { return &v }

func TestRuleBriefingHasAllSectionsAndFooter(t *testing.T) {
	md := RenderRuleBriefing(sampleBriefing())
	for _, title := range SectionTitles {
		if !strings.Contains(md, title) {
			t.Fatalf("缺节标题 %s", title)
		}
	}
	if !strings.HasSuffix(strings.TrimRight(md, " \t\r\n"), RuleFooter) {
		t.Fatalf("规则摘要应以 %q 结尾，实际结尾: %q", RuleFooter, md[max(0, len(md)-40):])
	}
	if !strings.Contains(md, "操作系统") {
		t.Fatal("应包含下一节课程名")
	}
	if !strings.Contains(md, "高数期末") {
		t.Fatal("应包含考试课程名")
	}
	if !strings.Contains(md, "1 项逾期") {
		t.Fatal("应包含逾期计数")
	}
}

func TestRuleBriefingEmptyState(t *testing.T) {
	b := sampleBriefing()
	b.Courses.TodayCount = 0
	b.Courses.NextCourse = nil
	b.Tasks.OverdueCount = 0
	b.Tasks.DueTodayCount = 0
	b.Tasks.Spotlight = nil
	b.Exam = nil

	md := RenderRuleBriefing(b)
	if !strings.Contains(md, "今天没有课程") {
		t.Fatal("无课应写今天没有课程")
	}
	if !strings.Contains(md, "暂无") {
		t.Fatal("空状态应包含暂无")
	}
}

func TestRuleServiceSourceIsRule(t *testing.T) {
	result := RuleBasedService{}.Generate(sampleBriefing())
	if result.Source != SourceRule {
		t.Fatalf("source = %q, want %q", result.Source, SourceRule)
	}
	if result.Model != "" {
		t.Fatalf("model = %q, want empty", result.Model)
	}
	if result.Summary == "" {
		t.Fatal("规则摘要不应为空")
	}
}

func TestRuleBriefingCoursesHidden(t *testing.T) {
	b := sampleBriefing()
	b.Phase.CoursesVisible = false
	md := RenderRuleBriefing(b)
	if !strings.Contains(md, "暂无课程安排") {
		t.Fatal("非教学周应写暂无课程安排")
	}
}
