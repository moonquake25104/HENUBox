package ai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kairos/server/internal/domain/calendar"
	"kairos/server/internal/store"
)

func aiMarkdown() string {
	return "# 2026-07-31 周五 晨间摘要\n\n" +
		"## 今日重点\n今天有 2 项任务到期。\n\n" +
		"## 课程\n今日共 2 节。\n\n" +
		"## 待办\n1 项逾期、2 项今日到期。\n\n" +
		"## 考试\n高数期末还有 5 天。\n\n" +
		"## 专注\n今日已完成 2 个番茄钟。\n\n" +
		"---\n" + AIFooter
}

func enableAI(t *testing.T, q *store.Queries, dir, baseURL string) {
	t.Helper()
	key, err := EnsureKeyFile(dir)
	if err != nil {
		t.Fatalf("ensure key: %v", err)
	}
	enc, err := EncryptAPIKey("sk-test", key)
	if err != nil {
		t.Fatalf("encrypt key: %v", err)
	}
	if err := q.UpdateAiConfig(context.Background(), store.UpdateAiConfigParams{
		Enabled:         true,
		BaseUrl:         baseURL,
		Model:           "deepseek-v4-flash",
		ApiKeyEncrypted: enc,
		SyncEnabled:     false,
	}); err != nil {
		t.Fatalf("update ai config: %v", err)
	}
}

func TestGenerateBriefRuleFallbackAndCache(t *testing.T) {
	q, _ := newTestStore(t)
	g := NewGenerator(q, t.TempDir())
	now := time.Date(2026, 7, 31, 8, 0, 0, 0, calendar.ChinaTZ)
	g.Now = func() time.Time { return now }
	ctx := context.Background()

	first, err := g.GenerateBrief(ctx, "", false, nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if first.Source != string(SourceRule) {
		t.Fatalf("source = %q, want rule", first.Source)
	}
	if !strings.HasSuffix(strings.TrimRight(first.Markdown, " \t\r\n"), RuleFooter) {
		t.Fatalf("rule markdown should end with rule footer")
	}

	cached, err := g.GenerateBrief(ctx, "", false, nil)
	if err != nil {
		t.Fatalf("cached generate: %v", err)
	}
	if cached.ID != first.ID || !cached.GeneratedAt.Time.Equal(first.GeneratedAt.Time) {
		t.Fatalf("second non-force call should return the cache")
	}

	now = now.Add(time.Hour)
	forced, err := g.GenerateBrief(ctx, "", true, nil)
	if err != nil {
		t.Fatalf("forced generate: %v", err)
	}
	if forced.GeneratedAt.Time.Equal(first.GeneratedAt.Time) {
		t.Fatalf("force=true should regenerate and refresh generated_at")
	}
	if forced.ID != first.ID {
		t.Fatalf("upsert should reuse the same date row")
	}
}

func TestGenerateBriefInvalidDate(t *testing.T) {
	q, _ := newTestStore(t)
	g := NewGenerator(q, t.TempDir())
	if _, err := g.GenerateBrief(context.Background(), "2026-13-99", false, nil); !errors.Is(err, ErrInvalidDate) {
		t.Fatalf("err = %v, want ErrInvalidDate", err)
	}
}

func TestGenerateBriefInFlightGuard(t *testing.T) {
	q, _ := newTestStore(t)
	dir := t.TempDir()

	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			close(started)
		}
		<-release
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}]}`, aiMarkdown())
	}))
	defer server.Close()

	enableAI(t, q, dir, server.URL)
	g := NewGenerator(q, dir)
	g.Client = server.Client()
	g.Now = func() time.Time { return time.Date(2026, 7, 31, 8, 0, 0, 0, calendar.ChinaTZ) }
	ctx := context.Background()

	type result struct {
		brief *store.AiMorningBrief
		err   error
	}
	first := make(chan result, 1)
	second := make(chan result, 1)

	go func() {
		b, err := g.GenerateBrief(ctx, "", true, nil)
		first <- result{b, err}
	}()
	<-started
	go func() {
		b, err := g.GenerateBrief(ctx, "", true, nil)
		second <- result{b, err}
	}()

	r2 := <-second
	if !errors.Is(r2.err, ErrGenerating) {
		t.Fatalf("concurrent generate err = %v, want ErrGenerating", r2.err)
	}
	close(release)
	r1 := <-first
	if r1.err != nil {
		t.Fatalf("first generate: %v", r1.err)
	}
	if r1.brief.Source != string(SourceAI) {
		t.Fatalf("source = %q, want ai", r1.brief.Source)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("provider calls = %d, want 1", got)
	}
}

func TestGenerateBriefAIStreaming(t *testing.T) {
	q, _ := newTestStore(t)
	dir := t.TempDir()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		parts := []string{"# 2026-07-31 周五 晨间摘要\n\n", "## 今日重点\n今天有 2 项任务到期。\n\n", strings.TrimPrefix(aiMarkdown(), "# 2026-07-31 周五 晨间摘要\n\n## 今日重点\n今天有 2 项任务到期。\n\n")}
		for _, p := range parts {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", p)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	enableAI(t, q, dir, server.URL)
	g := NewGenerator(q, dir)
	g.Client = server.Client()
	g.Now = func() time.Time { return time.Date(2026, 7, 31, 8, 0, 0, 0, calendar.ChinaTZ) }

	var streamed strings.Builder
	brief, err := g.GenerateBrief(context.Background(), "", true, func(d string) { streamed.WriteString(d) })
	if err != nil {
		t.Fatalf("generate streaming: %v", err)
	}
	if brief.Source != string(SourceAI) {
		t.Fatalf("source = %q, want ai", brief.Source)
	}
	if brief.Model != "deepseek-v4-flash" {
		t.Fatalf("model = %q", brief.Model)
	}
	if streamed.String() != aiMarkdown() {
		t.Fatalf("streamed text mismatch:\n%s", streamed.String())
	}
}

func TestGenerateBriefAIFallbackOnBadStructure(t *testing.T) {
	q, _ := newTestStore(t)
	dir := t.TempDir()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"结构不完整的摘要"}}]}`)
	}))
	defer server.Close()

	enableAI(t, q, dir, server.URL)
	g := NewGenerator(q, dir)
	g.Client = server.Client()
	g.Now = func() time.Time { return time.Date(2026, 7, 31, 8, 0, 0, 0, calendar.ChinaTZ) }

	brief, err := g.GenerateBrief(context.Background(), "", true, nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if brief.Source != string(SourceRule) {
		t.Fatalf("source = %q, want rule fallback", brief.Source)
	}
	if !strings.HasSuffix(strings.TrimRight(brief.Markdown, " \t\r\n"), RuleFooter) {
		t.Fatal("fallback should render the rule brief")
	}
}

func TestGenerateBriefRuleWhenDisabled(t *testing.T) {
	q, _ := newTestStore(t)
	dir := t.TempDir()
	if err := q.UpdateAiConfig(context.Background(), store.UpdateAiConfigParams{
		Enabled: false, BaseUrl: "https://api.deepseek.com", Model: "deepseek-v4-flash",
	}); err != nil {
		t.Fatalf("update config: %v", err)
	}
	g := NewGenerator(q, dir)
	g.Now = func() time.Time { return time.Date(2026, 7, 31, 8, 0, 0, 0, calendar.ChinaTZ) }

	brief, err := g.GenerateBrief(context.Background(), "", true, nil)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if brief.Source != string(SourceRule) || brief.Model != "" {
		t.Fatalf("disabled AI should use rule with empty model, got %+v", brief)
	}
}
