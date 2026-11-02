package ai

import (
	"fmt"
	"strings"

	"kairos/server/internal/domain/calendar"
)

// RuleBasedService renders the deterministic fallback brief. It never touches
// the network or the database and always succeeds.
type RuleBasedService struct{}

// Generate ports Rust RuleBasedService::generate_daily_brief.
func (RuleBasedService) Generate(b *calendar.TodayBriefingResponse) AiBriefResult {
	return AiBriefResult{
		Summary: RenderRuleBriefing(b),
		Source:  SourceRule,
	}
}

// RenderRuleBriefing builds the same five-section markdown shape as the AI
// output, ending with the rule footer.
func RenderRuleBriefing(b *calendar.TodayBriefingResponse) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# %s %s 晨间摘要\n\n", b.Date, b.WeekdayLabel)

	out.WriteString("## 今日重点\n")
	focus := []string{}
	if b.Tasks.OverdueCount > 0 {
		focus = append(focus, fmt.Sprintf("有 %d 项任务逾期，建议优先处理。", b.Tasks.OverdueCount))
	} else if b.Tasks.DueTodayCount > 0 {
		focus = append(focus, fmt.Sprintf("今天有 %d 项任务到期。", b.Tasks.DueTodayCount))
	}
	if b.Exam != nil {
		focus = append(focus, fmt.Sprintf("%s 还有 %d 天，建议提前复习。", b.Exam.CourseName, b.Exam.DaysUntil))
	}
	if b.Phase.TermLabel != nil && b.Phase.CurrentWeek != nil {
		focus = append(focus, fmt.Sprintf("当前%s第 %d 周。", *b.Phase.TermLabel, *b.Phase.CurrentWeek))
	}
	if len(focus) == 0 {
		focus = append(focus, "今天暂无紧急事项，按计划推进即可。")
	}
	out.WriteString(strings.Join(focus, " "))
	out.WriteString("\n\n")

	out.WriteString("## 课程\n")
	switch {
	case !b.Phase.CoursesVisible:
		out.WriteString("暂无课程安排\n\n")
	case b.Courses.TodayCount == 0:
		out.WriteString("今天没有课程\n\n")
	default:
		fmt.Fprintf(&out, "今日共 %d 节。", b.Courses.TodayCount)
		if b.Courses.CurrentCourse != nil {
			cur := b.Courses.CurrentCourse
			fmt.Fprintf(&out, " 当前：%s %s（%s）。", cur.Title, cur.StartTime, cur.Location)
		} else if b.Courses.NextCourse != nil {
			next := b.Courses.NextCourse
			fmt.Fprintf(&out, " 下一节：%s %s @ %s。", next.Title, next.StartTime, next.Location)
		}
		out.WriteString("\n\n")
	}

	out.WriteString("## 待办\n")
	if b.Tasks.OverdueCount == 0 && b.Tasks.DueTodayCount == 0 {
		out.WriteString("暂无\n\n")
	} else {
		fmt.Fprintf(&out, "%d 项逾期、%d 项今日到期。", b.Tasks.OverdueCount, b.Tasks.DueTodayCount)
		if len(b.Tasks.Spotlight) > 0 {
			fmt.Fprintf(&out, " 最要紧：%s。", b.Tasks.Spotlight[0].Title)
		}
		out.WriteString("\n\n")
	}

	out.WriteString("## 考试\n")
	if b.Exam != nil {
		fmt.Fprintf(&out, "%s 还有 %d 天（%s，%s）。", b.Exam.CourseName, b.Exam.DaysUntil, b.Exam.ExamDatetime, b.Exam.Location)
	} else {
		out.WriteString("暂无\n\n")
	}

	out.WriteString("## 专注\n")
	if b.Pomodoro.IsRunning {
		fmt.Fprintf(&out, "当前%s进行中，已完成 %d 个番茄钟。", b.Pomodoro.Phase, b.Pomodoro.CompletedSessions)
	} else {
		fmt.Fprintf(&out, "今日已完成 %d 个番茄钟，可开始下一轮专注。", b.Pomodoro.CompletedSessions)
	}
	out.WriteString("\n\n")

	out.WriteString("---\n")
	out.WriteString(RuleFooter)
	return out.String()
}
