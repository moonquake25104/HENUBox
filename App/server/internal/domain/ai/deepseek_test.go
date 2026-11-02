package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestChatCompletionRequestSerialization(t *testing.T) {
	body := chatRequest{
		Model: "deepseek-v4-flash",
		Messages: []chatMessage{
			{Role: "system", Content: SystemPrompt},
			{Role: "user", Content: "data"},
		},
		Temperature: Temperature,
		MaxTokens:   MaxTokens,
		Stream:      false,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if decoded["model"] != "deepseek-v4-flash" {
		t.Fatalf("model = %v", decoded["model"])
	}
	if temp, _ := decoded["temperature"].(float64); temp != 0.7 {
		t.Fatalf("temperature = %v, want 0.7", decoded["temperature"])
	}
	if decoded["max_tokens"] != float64(400) {
		t.Fatalf("max_tokens = %v, want 400", decoded["max_tokens"])
	}
	if decoded["stream"] != false {
		t.Fatalf("stream = %v, want false", decoded["stream"])
	}
	messages, _ := decoded["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("messages len = %d, want 2", len(messages))
	}
	first, _ := messages[0].(map[string]any)
	if first["role"] != "system" {
		t.Fatalf("messages[0].role = %v, want system", first["role"])
	}
}

func TestURLAppendsV1Once(t *testing.T) {
	cases := map[string]string{
		"https://api.deepseek.com":  "https://api.deepseek.com/v1/chat/completions",
		"https://api.deepseek.com/": "https://api.deepseek.com/v1/chat/completions",
	}
	for base, want := range cases {
		if got := chatCompletionsURL(base); got != want {
			t.Fatalf("chatCompletionsURL(%q) = %q, want %q", base, got, want)
		}
	}
}

func TestParseSSEDeltaExtractsContent(t *testing.T) {
	got, ok := parseSSEDelta(`{"choices":[{"delta":{"content":"你好"}}]}`)
	if !ok || got != "你好" {
		t.Fatalf("parseSSEDelta = (%q, %v), want (你好, true)", got, ok)
	}
}

func TestParseSSEDeltaNoneForNoContent(t *testing.T) {
	if _, ok := parseSSEDelta(`{"choices":[{"delta":{"role":"assistant"}}]}`); ok {
		t.Fatal("无 content 增量应返回 false")
	}
	if _, ok := parseSSEDelta("not json"); ok {
		t.Fatal("非法 JSON 应返回 false")
	}
}

func TestHandleSSELineExtractsDelta(t *testing.T) {
	delta, ok := handleSSELine(`data: {"choices":[{"delta":{"content":"你好"}}]}`)
	if !ok || delta != "你好" {
		t.Fatalf("handleSSELine = (%q, %v)", delta, ok)
	}
	for _, line := range []string{"", ":keep-alive", "data: [DONE]", `data: {"choices":[{"delta":{"role":"assistant"}}]}`} {
		if _, ok := handleSSELine(line); ok {
			t.Fatalf("line %q 不应产生 delta", line)
		}
	}
}

func TestHandleSSELineMultipleDeltasAccumulate(t *testing.T) {
	var full strings.Builder
	for _, line := range []string{
		`data: {"choices":[{"delta":{"content":"今天"}}]}`,
		`data: {"choices":[{"delta":{"content":"很棒"}}]}`,
	} {
		if delta, ok := handleSSELine(line); ok {
			full.WriteString(delta)
		}
	}
	if full.String() != "今天很棒" {
		t.Fatalf("accumulated = %q, want 今天很棒", full.String())
	}
}

func TestCollectSSEStreamAssemblesDeltas(t *testing.T) {
	body := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"## "}}]}`,
		"",
		`data: {"choices":[{"delta":{"content":"今日重点"}}]}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	var deltas []string
	full, err := collectSSEStream(strings.NewReader(body), func(d string) { deltas = append(deltas, d) })
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if full != "## 今日重点" {
		t.Fatalf("full = %q", full)
	}
	if len(deltas) != 2 || deltas[0] != "## " || deltas[1] != "今日重点" {
		t.Fatalf("deltas = %#v", deltas)
	}
}

func TestCollectSSEStreamFallsBackToJSON(t *testing.T) {
	body := `{"choices":[{"message":{"content":"完整文本"}}]}`
	var got string
	full, err := collectSSEStream(strings.NewReader(body), func(d string) { got += d })
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if full != "完整文本" || got != "完整文本" {
		t.Fatalf("full=%q got=%q", full, got)
	}
}

func newTestProvider(server *httptest.Server) *Provider {
	return &Provider{
		client:  server.Client(),
		baseURL: server.URL,
		model:   "deepseek-v4-flash",
		apiKey:  "sk-test",
	}
}

func TestProviderNonStreaming(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"完整摘要"}}]}`)
	}))
	defer server.Close()

	p := newTestProvider(server)
	res, err := p.Generate(context.Background(), sampleBriefing())
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if res.Summary != "完整摘要" {
		t.Fatalf("summary = %q", res.Summary)
	}
	if res.Source != SourceAI || res.Model != "deepseek-v4-flash" {
		t.Fatalf("result = %+v", res)
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("authorization = %q", gotAuth)
	}
}

func TestProviderStreamingEmitsDeltas(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, d := range []string{"## ", "今日重点"} {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", d)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	p := newTestProvider(server)
	var deltas []string
	full, err := p.GenerateStreaming(context.Background(), sampleBriefing(), func(d string) { deltas = append(deltas, d) })
	if err != nil {
		t.Fatalf("streaming: %v", err)
	}
	if full != "## 今日重点" {
		t.Fatalf("full = %q", full)
	}
	if len(deltas) != 2 {
		t.Fatalf("deltas = %#v", deltas)
	}
}

func TestProviderStreamingFallsBackToNonStreaming(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), `"stream":true`) {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"非流式回退"}}]}`)
	}))
	defer server.Close()

	p := newTestProvider(server)
	full, err := p.GenerateStreaming(context.Background(), sampleBriefing(), nil)
	if err != nil {
		t.Fatalf("streaming fallback: %v", err)
	}
	if full != "非流式回退" {
		t.Fatalf("full = %q", full)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want 2", calls.Load())
	}
}

func TestProviderHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, "invalid key")
	}))
	defer server.Close()

	p := newTestProvider(server)
	_, err := p.Generate(context.Background(), sampleBriefing())
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("err = %v, want *HTTPError", err)
	}
	if httpErr.Status != 401 {
		t.Fatalf("status = %d, want 401", httpErr.Status)
	}
	if got := UserMessage(err); got != "API key 无效，请在 AI 设置中重新配置" {
		t.Fatalf("user message = %q", got)
	}
}

func TestProviderTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = io.WriteString(w, `{"choices":[]}`)
	}))
	defer server.Close()

	p := newTestProvider(server)
	p.client = &http.Client{Timeout: 20 * time.Millisecond}
	_, err := p.Generate(context.Background(), sampleBriefing())
	if err != ErrTimeout {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
}
