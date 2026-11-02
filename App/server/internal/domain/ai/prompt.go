package ai

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"kairos/server/internal/domain/calendar"
)

// ModelDefault is the default DeepSeek model name (configurable in settings).
const ModelDefault = "deepseek-v4-flash"

// MaxTokens is the hard per-generation token ceiling (cost guard).
const MaxTokens = 400

// Temperature is the generation temperature.
const Temperature = 0.7

// MaxContextChars is the user-message JSON character ceiling. Exceeding it
// downgrades to the rule engine rather than truncating the JSON mid-way.
const MaxContextChars = 4000

// AIFooter is the mandatory last line for AI output (anti-hallucination marker).
const AIFooter = "*AI生成，请核实*"

// RuleFooter is the last line for the rule-engine output.
const RuleFooter = "*本地生成*"

// SectionTitles are the fixed five section headings the output validator checks.
var SectionTitles = []string{"## 今日重点", "## 课程", "## 待办", "## 考试", "## 专注"}

// SystemPrompt is the compile-time-fixed Chinese system prompt. It is never
// sourced from user input so the token budget stays stable.
const SystemPrompt = `你是「Kairos」学生的中文晨间学习助手。每天早晨根据系统提供的 JSON 数据，生成一份简洁自然的 Markdown 晨间摘要，帮学生快速规划当天。

## 硬性规则
1. 只基于给定 JSON 总结，绝不编造或外推任何信息。JSON 里没有的一律不写。
2. 课程名称、起止时间、地点；考试课程、考试时间、剩余天数、地点；任务标题；所有数字计数（课程节数、逾期数、今日到期数、已完成番茄数、当前周次），必须逐字引用，不得改写、不得计算、不得补全。
3. 考试一律用 JSON 的 days_until 表述（如"还有 5 天"），不要自行换算日期或时区；如需日期可原样引用 exam_datetime。
4. 空节必须明确写"暂无"。无考试→"暂无"；无课程安排→"暂无课程安排"；无待办→"暂无"。
5. 语气简洁口语化，适合学生早晨快读。每节 1-3 个要点，全文不超过 350 字。
6. 标题使用 JSON 里的 date 与 weekday_label 原样拼出，格式：# {date} {weekday_label} 晨间摘要。
7. 输出必须使用下方固定结构，节标题一字不差；最后一行必须是：*AI生成，请核实*

## 允许与禁止
- 允许：基于已有数据给出学习优先级、番茄钟建议，用"建议…"口吻，不得写成事实陈述。
- 禁止：引用 JSON 之外字段（如教师姓名、教室未给出时编造地点）；猜测或补全考试时间/地点；杜撰任务；虚构课程。

## 字段含义（助你理解，不要照抄）
- courses.current_course/next_course 是正在上/下一节未开始的课；两者为 null 分别表示无进行中课程/无后续课程。
- tasks.priority 取值 high|medium|low；spotlight 已按 逾期>高优先级>早截止 排序。
- pomodoro.phase 取值 work|short_break|long_break。
- phase.courses_visible 为 false 时表示非教学周，课程节必须写"暂无课程安排"。

## 输出结构（必须完全一致）
# {date} {weekday_label} 晨间摘要

## 今日重点
（1-2 句综合建议，可含优先级引导）

## 课程
（今日共 N 节；进行中/下一节课程；无课写"暂无课程安排"）

## 待办
（逾期/今日到期；列出最要紧的 1-3 项）

## 考试
（最近考试信息；无则写"暂无"）

## 专注
（番茄钟状态与建议）

---
*AI生成，请核实*
`

type promptNextCourse struct {
	Title     string `json:"title"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Location  string `json:"location"`
}

type promptCourses struct {
	TodayCount    int64             `json:"today_count"`
	CurrentCourse *promptNextCourse `json:"current_course"`
	NextCourse    *promptNextCourse `json:"next_course"`
}

type promptSpotlight struct {
	Title    string  `json:"title"`
	Priority string  `json:"priority"`
	DueDate  *string `json:"due_date"`
}

type promptTasks struct {
	OverdueCount  int64             `json:"overdue_count"`
	DueTodayCount int64             `json:"due_today_count"`
	Spotlight     []promptSpotlight `json:"spotlight"`
}

type promptExam struct {
	CourseName   string `json:"course_name"`
	ExamDatetime string `json:"exam_datetime"`
	DaysUntil    int64  `json:"days_until"`
	Location     string `json:"location"`
}

type promptPomodoro struct {
	IsRunning         bool   `json:"is_running"`
	Phase             string `json:"phase"`
	RemainingSeconds  int64  `json:"remaining_seconds"`
	CompletedSessions int64  `json:"completed_sessions"`
}

type promptPhase struct {
	PhaseType                string  `json:"phase_type"`
	TermLabel                *string `json:"term_label"`
	CurrentWeek              *int64  `json:"current_week"`
	CoursesVisible           bool    `json:"courses_visible"`
	ExamNotificationsEnabled bool    `json:"exam_notifications_enabled"`
	PomodoroProfile          string  `json:"pomodoro_profile"`
}

type promptBriefing struct {
	Date         string         `json:"date"`
	WeekdayLabel string         `json:"weekday_label"`
	Courses      promptCourses  `json:"courses"`
	Tasks        promptTasks    `json:"tasks"`
	Exam         *promptExam    `json:"exam"`
	Pomodoro     promptPomodoro `json:"pomodoro"`
	Phase        promptPhase    `json:"phase"`
}

// BuildUserMessage serializes the briefing into the trimmed JSON user message.
// It returns an error when the payload exceeds MaxContextChars; the caller must
// then downgrade to the rule engine (the JSON is never truncated).
func BuildUserMessage(b *calendar.TodayBriefingResponse) (string, error) {
	trimmed := promptBriefing{
		Date:         b.Date,
		WeekdayLabel: b.WeekdayLabel,
		Courses: promptCourses{
			TodayCount:    b.Courses.TodayCount,
			CurrentCourse: trimmedNextCourse(b.Courses.CurrentCourse),
			NextCourse:    trimmedNextCourse(b.Courses.NextCourse),
		},
		Tasks: promptTasks{
			OverdueCount:  b.Tasks.OverdueCount,
			DueTodayCount: b.Tasks.DueTodayCount,
			Spotlight:     trimmedSpotlight(b.Tasks.Spotlight),
		},
		Exam:     trimmedExam(b.Exam),
		Pomodoro: promptPomodoro(b.Pomodoro),
		Phase: promptPhase{
			PhaseType:                b.Phase.PhaseType,
			TermLabel:                b.Phase.TermLabel,
			CurrentWeek:              b.Phase.CurrentWeek,
			CoursesVisible:           b.Phase.CoursesVisible,
			ExamNotificationsEnabled: b.Phase.ExamNotificationsEnabled,
			PomodoroProfile:          b.Phase.PomodoroProfile,
		},
	}

	raw, err := json.Marshal(trimmed)
	if err != nil {
		return "", fmt.Errorf("序列化简报失败: %w", err)
	}
	if utf8.RuneCount(raw) > MaxContextChars {
		return "", fmt.Errorf("今日数据超过 %d 字符上限，降级本地规则生成", MaxContextChars)
	}
	return "以下是今日数据，请按系统提示生成摘要：\n" + string(raw), nil
}

// ValidateAIOutput reports whether the markdown contains all five section
// titles and ends with the AI footer. A failure means the caller downgrades to
// the rule engine without a second API call.
func ValidateAIOutput(markdown string) bool {
	for _, title := range SectionTitles {
		if !strings.Contains(markdown, title) {
			return false
		}
	}
	return strings.HasSuffix(strings.TrimRight(markdown, " \t\r\n\v\f"), AIFooter)
}

func trimmedNextCourse(c *calendar.NextCourse) *promptNextCourse {
	if c == nil {
		return nil
	}
	return &promptNextCourse{
		Title:     c.Title,
		StartTime: c.StartTime,
		EndTime:   c.EndTime,
		Location:  c.Location,
	}
}

func trimmedSpotlight(items []calendar.TaskSpotlight) []promptSpotlight {
	out := make([]promptSpotlight, 0, len(items))
	for _, s := range items {
		out = append(out, promptSpotlight{Title: s.Title, Priority: s.Priority, DueDate: s.DueDate})
	}
	return out
}

func trimmedExam(e *calendar.UpcomingExam) *promptExam {
	if e == nil {
		return nil
	}
	return &promptExam{
		CourseName:   e.CourseName,
		ExamDatetime: e.ExamDatetime,
		DaysUntil:    e.DaysUntil,
		Location:     e.Location,
	}
}
