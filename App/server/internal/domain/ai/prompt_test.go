package ai

import (
	"strings"
	"testing"

	"kairos/server/internal/domain/calendar"
)

func TestBuildUserMessageTrimsIDsAndUsesSnakeCase(t *testing.T) {
	msg, err := BuildUserMessage(sampleBriefing())
	if err != nil {
		t.Fatalf("build user message: %v", err)
	}
	if !strings.HasPrefix(msg, "以下是今日数据，请按系统提示生成摘要：\n") {
		t.Fatalf("unexpected prefix: %q", msg[:min(40, len(msg))])
	}
	for _, want := range []string{`"overdue_count":1`, `"next_course"`, `"days_until":5`, `"completed_sessions":2`, `"courses_visible":true`} {
		if !strings.Contains(msg, want) {
			t.Fatalf("user message missing %s: %s", want, msg)
		}
	}
	if strings.Contains(msg, `"id"`) {
		t.Fatalf("user message should not carry id fields: %s", msg)
	}
}

func TestBuildUserMessageRejectsOversizedContext(t *testing.T) {
	b := sampleBriefing()
	b.Tasks.Spotlight = nil
	for i := range 200 {
		b.Tasks.Spotlight = append(b.Tasks.Spotlight, calendar.TaskSpotlight{
			ID:       int64(i),
			Title:    strings.Repeat("超长任务标题", 4),
			Priority: "high",
			DueDate:  strPtr("2026-07-31"),
		})
	}
	if _, err := BuildUserMessage(b); err == nil {
		t.Fatal("超过上下文上限时应返回错误以触发规则降级")
	}
}

func TestValidateAIOutput(t *testing.T) {
	valid := "# 2026-07-31 周五 晨间摘要\n\n" +
		"## 今日重点\nx\n## 课程\nx\n## 待办\nx\n## 考试\nx\n## 专注\nx\n\n---\n" + AIFooter
	if !ValidateAIOutput(valid) {
		t.Fatal("结构完整且以 AI footer 结尾应通过校验")
	}
	if ValidateAIOutput(strings.Replace(valid, "## 考试", "## 备考", 1)) {
		t.Fatal("缺少节标题应校验失败")
	}
	if ValidateAIOutput(strings.TrimSuffix(valid, AIFooter) + RuleFooter) {
		t.Fatal("footer 非 AI 结尾应校验失败")
	}
	if !ValidateAIOutput(valid + "\n\n  ") {
		t.Fatal("尾部空白应被忽略")
	}
}
