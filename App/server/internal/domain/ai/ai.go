// Package ai ports the Rust AI morning-brief domain (src-tauri/src/ai/) into the
// Go web backend: an OpenAI-compatible DeepSeek provider (non-streaming + SSE
// streaming), a deterministic rule-based fallback, prompt/output validation,
// AES-256-GCM API-key encryption with a local key file, and the daily
// generation orchestration (cache + in-flight guard + fallback).
//
// The package is deliberately split into pure functions (prompt, rule, SSE
// parsing) and a small orchestration type so it can be unit-tested without a
// network or database, mirroring the Rust #[cfg(test)] scenarios.
package ai

import (
	"errors"
	"fmt"
)

// BriefSource identifies where a generated brief came from. The literal values
// match the ai_morning_brief.source CHECK constraint and the frontend
// src/types/ai.ts BriefSource union.
type BriefSource string

const (
	// SourceAI is a DeepSeek-generated brief.
	SourceAI BriefSource = "ai"
	// SourceRule is a locally generated fallback brief.
	SourceRule BriefSource = "rule"
)

// AiBriefResult is the shared result shape of the AI provider and the rule
// engine (Rust ai::AiBriefResult). Model is only set for SourceAI.
type AiBriefResult struct {
	Summary     string
	Source      BriefSource
	GeneratedAt string
	Model       string
}

// ErrTimeout is returned when the provider request exceeds the request timeout.
var ErrTimeout = errors.New("AI 请求超时，请稍后重试")

// ErrGenerating is returned when a generation for the same date is already
// in-flight and no cached brief is available yet (Rust "正在生成今日摘要，请稍候").
var ErrGenerating = errors.New("正在生成今日摘要，请稍候")

// HTTPError is returned for a non-2xx provider response.
type HTTPError struct {
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("AI 服务返回错误 %d: %s", e.Status, e.Body)
}

// NetworkError wraps a transport-level failure.
type NetworkError struct {
	Err error
}

func (e *NetworkError) Error() string {
	return "网络错误: " + e.Err.Error()
}

func (e *NetworkError) Unwrap() error { return e.Err }

// ParseError is returned when a provider response cannot be decoded or carries
// no usable content.
type ParseError struct {
	Msg string
}

func (e *ParseError) Error() string { return "AI 响应解析失败: " + e.Msg }

// UserMessage maps an AI error to the user-facing Chinese message used by the
// Rust AiError Display impl. Callers normally fall back to the rule engine, so
// this is only surfaced for diagnostics.
func UserMessage(err error) string {
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.Status {
		case 401:
			return "API key 无效，请在 AI 设置中重新配置"
		case 429:
			return "请求过于频繁，请稍后重试"
		}
	}
	if errors.Is(err, ErrTimeout) {
		return ErrTimeout.Error()
	}
	return err.Error()
}
