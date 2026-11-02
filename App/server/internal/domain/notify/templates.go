package notify

import "fmt"

// OffsetDescription renders a reminder offset in minutes as a Chinese phrase:
// whole days for exact multiples of 1440, whole hours for exact multiples of 60
// (when at least an hour), otherwise plain minutes.
func OffsetDescription(minutes int) string {
	if minutes >= 1440 && minutes%1440 == 0 {
		return fmt.Sprintf("%d天", minutes/1440)
	}
	if minutes >= 60 && minutes%60 == 0 {
		return fmt.Sprintf("%d小时", minutes/60)
	}
	return fmt.Sprintf("%d分钟", minutes)
}

// PhaseNameCN maps a pomodoro phase wire value to its Chinese label, echoing
// unknown phases unchanged.
func PhaseNameCN(phase string) string {
	switch phase {
	case "work":
		return "专注"
	case "short_break":
		return "短休息"
	case "long_break":
		return "长休息"
	default:
		return phase
	}
}

// ExamEmail builds the exam countdown subject (the course name) and body.
func ExamEmail(courseName string, offsetMinutes int) (string, string) {
	body := fmt.Sprintf("考试「%s」将在 %s 后开始", courseName, OffsetDescription(offsetMinutes))
	return courseName, body
}

// CourseEmail builds the pre-class reminder subject (the course name) and body.
func CourseEmail(courseName, location string, offsetMinutes int) (string, string) {
	body := fmt.Sprintf("课程「%s」将在 %s 后开始", courseName, OffsetDescription(offsetMinutes))
	if location != "" {
		body += fmt.Sprintf("（地点：%s）", location)
	}
	return courseName, body
}

// DailyTaskEmail builds the recurring daily-task reminder subject and body.
func DailyTaskEmail(title string) (string, string) {
	return "每日任务提醒", fmt.Sprintf("「%s」—— 别忘了今天完成", title)
}

// OneShotTaskEmail builds the one-shot task reminder subject and body.
func OneShotTaskEmail(title string) (string, string) {
	return "任务提醒", fmt.Sprintf("「%s」—— 到时间了，别忘了完成", title)
}

// PomodoroEmail builds the phase-finished subject and body.
func PomodoroEmail(phase string) (string, string) {
	return "番茄钟", fmt.Sprintf("番茄钟「%s」阶段已结束", PhaseNameCN(phase))
}

// AIBriefEmail builds the morning-brief subject; the full markdown is the body.
func AIBriefEmail(markdown string) (string, string) {
	return "今日 AI 摘要已生成", markdown
}

// briefPreview returns at most the first 60 runes of markdown for log lines.
func briefPreview(markdown string) string {
	runes := []rune(markdown)
	if len(runes) <= 60 {
		return markdown
	}
	return string(runes[:60])
}
