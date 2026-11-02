package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"kairos/server/internal/domain/calendar"
	"kairos/server/internal/store"
)

// requestTimeout caps a single provider generation (Rust REQUEST_TIMEOUT).
const requestTimeout = 15 * time.Second

// Provider is the OpenAI-compatible DeepSeek client. It supports the
// non-streaming path (7:00 scheduler / ?sync=true) and the SSE streaming path
// (manual generate).
type Provider struct {
	client  *http.Client
	baseURL string
	model   string
	apiKey  string
}

// ResolveProvider decrypts the stored API key and builds a provider. It returns
// (nil, nil) when AI is disabled or no key is configured, so the caller can
// downgrade to the rule engine without treating it as an error.
func ResolveProvider(config store.AiConfig, key [32]byte, client *http.Client) (*Provider, error) {
	if !config.Enabled || config.ApiKeyEncrypted == "" {
		return nil, nil
	}
	apiKey, err := DecryptAPIKey(config.ApiKeyEncrypted, key)
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}
	return &Provider{
		client:  client,
		baseURL: config.BaseUrl,
		model:   config.Model,
		apiKey:  apiKey,
	}, nil
}

// Generate performs a non-streaming generation and returns the full result.
func (p *Provider) Generate(ctx context.Context, briefing *calendar.TodayBriefingResponse) (AiBriefResult, error) {
	text, err := p.generateNonStreaming(ctx, briefing)
	if err != nil {
		return AiBriefResult{}, err
	}
	return AiBriefResult{Summary: text, Source: SourceAI, Model: p.model}, nil
}

// GenerateStreaming performs an SSE generation, invoking onDelta per chunk, and
// returns the accumulated text. When the stream yields no content it falls back
// to a non-streaming request (some third-party relays are unreliable in SSE).
func (p *Provider) GenerateStreaming(ctx context.Context, briefing *calendar.TodayBriefingResponse, onDelta func(string)) (string, error) {
	body, err := p.requestBody(briefing, true)
	if err != nil {
		return "", err
	}
	resp, err := p.doRequest(ctx, body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	text, err := collectSSEStream(resp.Body, onDelta)
	if err != nil {
		return "", err
	}
	if text != "" {
		return text, nil
	}
	return p.generateNonStreaming(ctx, briefing)
}

func (p *Provider) generateNonStreaming(ctx context.Context, briefing *calendar.TodayBriefingResponse) (string, error) {
	body, err := p.requestBody(briefing, false)
	if err != nil {
		return "", err
	}
	resp, err := p.doRequest(ctx, body)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var parsed chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", &ParseError{Msg: fmt.Sprintf("解析 AI 非流式响应失败: %v", err)}
	}
	if len(parsed.Choices) == 0 {
		return "", &ParseError{Msg: "AI 响应 choices 为空"}
	}
	return parsed.Choices[0].Message.Content, nil
}

func (p *Provider) requestBody(briefing *calendar.TodayBriefingResponse, stream bool) (chatRequest, error) {
	userMessage, err := BuildUserMessage(briefing)
	if err != nil {
		return chatRequest{}, &ParseError{Msg: err.Error()}
	}
	return chatRequest{
		Model: p.model,
		Messages: []chatMessage{
			{Role: "system", Content: SystemPrompt},
			{Role: "user", Content: userMessage},
		},
		Temperature: Temperature,
		MaxTokens:   MaxTokens,
		Stream:      stream,
	}, nil
}

func (p *Provider) doRequest(ctx context.Context, body chatRequest) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, &ParseError{Msg: err.Error()}
	}
	url := chatCompletionsURL(p.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, &NetworkError{Err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, mapTransportError(err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, &HTTPError{Status: resp.StatusCode, Body: string(bodyBytes)}
	}
	return resp, nil
}

func mapTransportError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ErrTimeout
	}
	return &NetworkError{Err: err}
}

func chatCompletionsURL(baseURL string) string {
	return strings.TrimRight(baseURL, "/") + "/v1/chat/completions"
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float32       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
	Stream      bool          `json:"stream"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

type chatChoice struct {
	Message chatResponseMessage `json:"message"`
}

type chatResponseMessage struct {
	Content string `json:"content"`
}

type chatChunk struct {
	Choices []chatChunkChoice `json:"choices"`
}

type chatChunkChoice struct {
	Delta chatChunkDelta `json:"delta"`
}

type chatChunkDelta struct {
	Content *string `json:"content"`
}

// collectSSEStream accumulates delta.content from an SSE event stream, invoking
// onDelta per parsed chunk, and returns the full text. It parses line by line
// (LF/CRLF) so it does not depend on blank-line event separators. When no delta
// is parsed and the body is a single JSON object, it falls back to the
// non-streaming DTO shape.
func collectSSEStream(r io.Reader, onDelta func(string)) (string, error) {
	reader := bufio.NewReader(r)
	var full strings.Builder
	lastLine := ""

	process := func(line string) {
		lastLine = line
		if delta, ok := handleSSELine(line); ok {
			full.WriteString(delta)
			if onDelta != nil {
				onDelta(delta)
			}
		}
	}

	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			process(strings.TrimRight(line, "\r\n"))
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return full.String(), &NetworkError{Err: err}
		}
	}

	if full.Len() == 0 && strings.HasPrefix(strings.TrimSpace(lastLine), "{") {
		var parsed chatResponse
		if err := json.Unmarshal([]byte(strings.TrimSpace(lastLine)), &parsed); err == nil && len(parsed.Choices) > 0 {
			content := parsed.Choices[0].Message.Content
			if content != "" {
				full.WriteString(content)
				if onDelta != nil {
					onDelta(content)
				}
			}
		}
	}
	return full.String(), nil
}

// handleSSELine parses one SSE line. It returns the extracted delta and true
// only for a `data:` line with a usable content delta.
func handleSSELine(line string) (string, bool) {
	data, ok := strings.CutPrefix(strings.TrimLeft(line, " \t"), "data:")
	if !ok {
		return "", false
	}
	data = strings.TrimSpace(data)
	if data == "[DONE]" {
		return "", false
	}
	return parseSSEDelta(data)
}

// parseSSEDelta extracts choices[0].delta.content from a single SSE data JSON.
func parseSSEDelta(data string) (string, bool) {
	var chunk chatChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return "", false
	}
	if len(chunk.Choices) == 0 || chunk.Choices[0].Delta.Content == nil {
		return "", false
	}
	return *chunk.Choices[0].Delta.Content, true
}
