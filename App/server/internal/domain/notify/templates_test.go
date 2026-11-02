package notify

import "testing"

func TestOffsetDescription(t *testing.T) {
	cases := []struct {
		minutes int
		want    string
	}{
		{1440, "1天"},
		{2880, "2天"},
		{60, "1小时"},
		{120, "2小时"},
		{30, "30分钟"},
		{5, "5分钟"},
	}
	for _, tc := range cases {
		if got := OffsetDescription(tc.minutes); got != tc.want {
			t.Errorf("OffsetDescription(%d) = %q, want %q", tc.minutes, got, tc.want)
		}
	}
}

func TestPhaseNameCN(t *testing.T) {
	cases := map[string]string{
		"work":          "专注",
		"short_break":   "短休息",
		"long_break":    "长休息",
		"unknown_phase": "unknown_phase",
	}
	for phase, want := range cases {
		if got := PhaseNameCN(phase); got != want {
			t.Errorf("PhaseNameCN(%q) = %q, want %q", phase, got, want)
		}
	}
}

func TestExamEmail(t *testing.T) {
	subject, body := ExamEmail("高等数学", 1440)
	if subject != "高等数学" {
		t.Errorf("subject = %q, want 高等数学", subject)
	}
	if want := "考试「高等数学」将在 1天 后开始"; body != want {
		t.Errorf("body = %q, want %q", body, want)
	}
}

func TestCourseEmail(t *testing.T) {
	subject, body := CourseEmail("线性代数", "秦岭堂A114", 10)
	if subject != "线性代数" {
		t.Errorf("subject = %q, want 线性代数", subject)
	}
	if want := "课程「线性代数」将在 10分钟 后开始（地点：秦岭堂A114）"; body != want {
		t.Errorf("body = %q, want %q", body, want)
	}

	_, bodyNoLocation := CourseEmail("线性代数", "", 10)
	if want := "课程「线性代数」将在 10分钟 后开始"; bodyNoLocation != want {
		t.Errorf("body without location = %q, want %q", bodyNoLocation, want)
	}
}

func TestDailyTaskEmail(t *testing.T) {
	subject, body := DailyTaskEmail("背单词")
	if subject != "每日任务提醒" {
		t.Errorf("subject = %q", subject)
	}
	if want := "「背单词」—— 别忘了今天完成"; body != want {
		t.Errorf("body = %q, want %q", body, want)
	}
}

func TestOneShotTaskEmail(t *testing.T) {
	subject, body := OneShotTaskEmail("交报告")
	if subject != "任务提醒" {
		t.Errorf("subject = %q", subject)
	}
	if want := "「交报告」—— 到时间了，别忘了完成"; body != want {
		t.Errorf("body = %q, want %q", body, want)
	}
}

func TestPomodoroEmail(t *testing.T) {
	for phase, name := range map[string]string{
		"work":        "专注",
		"short_break": "短休息",
		"long_break":  "长休息",
	} {
		subject, body := PomodoroEmail(phase)
		if subject != "番茄钟" {
			t.Errorf("subject = %q", subject)
		}
		if want := "番茄钟「" + name + "」阶段已结束"; body != want {
			t.Errorf("body(%s) = %q, want %q", phase, body, want)
		}
	}
}

func TestAIBriefEmail(t *testing.T) {
	markdown := "## 今日概览\n- 高等数学 08:00\n- 交报告 14:00"
	subject, body := AIBriefEmail(markdown)
	if subject != "今日 AI 摘要已生成" {
		t.Errorf("subject = %q", subject)
	}
	if body != markdown {
		t.Errorf("body = %q, want full markdown %q", body, markdown)
	}
}

func TestBriefPreview(t *testing.T) {
	if got := briefPreview("短文本"); got != "短文本" {
		t.Errorf("preview = %q", got)
	}
	long := ""
	for range 100 {
		long += "字"
	}
	if got := briefPreview(long); len([]rune(got)) != 60 {
		t.Errorf("preview rune length = %d, want 60", len([]rune(got)))
	}
}
