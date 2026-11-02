package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"kairos/server/internal/httpapi/dto"
	"kairos/server/internal/store"
	"kairos/server/internal/store/migrate"
)

func newAITestRouter(t *testing.T) (http.Handler, string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testDatabaseURL())
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	schema := "ai_httpapi_test_" + randomHex()
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema %s: %v", schema, err)
	}
	if _, err := conn.Exec(ctx, "SET search_path TO "+schema); err != nil {
		t.Fatalf("set search_path to %s: %v", schema, err)
	}
	if err := migrate.Up(ctx, conn); err != nil {
		t.Fatalf("apply migrations to schema %s: %v", schema, err)
	}
	t.Cleanup(func() {
		defer conn.Close(context.Background())
		if _, err := conn.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE"); err != nil {
			t.Errorf("drop schema %s: %v", schema, err)
		}
	})

	hash, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := New(Options{
		Log:          log,
		Username:     "kairos",
		PasswordHash: string(hash),
		JWTSecret:    []byte(testSecret),
		JWTTTL:       time.Hour,
		Store:        store.New(conn),
		DataDir:      t.TempDir(),
	})
	return h, loginToken(t, h)
}

func TestAIConfigReadWrite(t *testing.T) {
	h, token := newAITestRouter(t)

	rec := doJSON(t, h, http.MethodGet, "/api/ai/config", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("get config status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var cfg dto.AiConfig
	decodeBody(t, rec, &cfg)
	if cfg.Enabled || cfg.APIKeyConfigured {
		t.Fatalf("initial config should be disabled without a key: %+v", cfg)
	}
	if cfg.BaseURL != "https://api.deepseek.com" || cfg.Model != "deepseek-v4-flash" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}

	enabled := true
	syncEnabled := true
	baseURL := "https://example.com/api"
	model := "deepseek-v4-flash"
	apiKey := "sk-test-key"
	rec = doJSON(t, h, http.MethodPatch, "/api/ai/config", dto.UpdateAiConfigRequest{
		Enabled:     &enabled,
		BaseURL:     &baseURL,
		Model:       &model,
		APIKey:      &apiKey,
		SyncEnabled: &syncEnabled,
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch config status = %d, body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &cfg)
	if !cfg.Enabled || !cfg.APIKeyConfigured || !cfg.SyncEnabled {
		t.Fatalf("updated config = %+v", cfg)
	}
	if cfg.BaseURL != baseURL {
		t.Fatalf("base_url = %q, want %q", cfg.BaseURL, baseURL)
	}

	// Omitted api_key keeps the stored key.
	newModel := "deepseek-reasoner"
	rec = doJSON(t, h, http.MethodPatch, "/api/ai/config", dto.UpdateAiConfigRequest{Model: &newModel}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch config status = %d, body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &cfg)
	if !cfg.APIKeyConfigured {
		t.Fatal("api_key should be preserved when omitted")
	}
	if cfg.Model != newModel {
		t.Fatalf("model = %q, want %q", cfg.Model, newModel)
	}

	// Empty api_key clears it.
	empty := ""
	rec = doJSON(t, h, http.MethodPatch, "/api/ai/config", dto.UpdateAiConfigRequest{APIKey: &empty}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch config status = %d, body=%s", rec.Code, rec.Body.String())
	}
	decodeBody(t, rec, &cfg)
	if cfg.APIKeyConfigured {
		t.Fatal("api_key should be cleared by empty string")
	}
}

func TestAIMorningBriefRuleAndCache(t *testing.T) {
	h, token := newAITestRouter(t)
	const date = "2026-07-31"

	rec := doJSON(t, h, http.MethodGet, "/api/ai/morning-brief?date="+date, nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("get brief status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if strings.TrimSpace(rec.Body.String()) != "null" {
		t.Fatalf("missing brief should be null, got %s", rec.Body.String())
	}

	rec = doJSON(t, h, http.MethodPost, "/api/ai/morning-brief/generate?sync=true&date="+date, nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("generate status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var brief dto.AiMorningBrief
	decodeBody(t, rec, &brief)
	if brief.Source != "rule" {
		t.Fatalf("source = %q, want rule (AI disabled)", brief.Source)
	}
	if !strings.Contains(brief.Markdown, "## 今日重点") || !strings.HasSuffix(strings.TrimRight(brief.Markdown, " \t\r\n"), "*本地生成*") {
		t.Fatalf("rule brief shape invalid: %s", brief.Markdown)
	}

	rec = doJSON(t, h, http.MethodGet, "/api/ai/morning-brief?date="+date, nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("get cached brief status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var cached dto.AiMorningBrief
	decodeBody(t, rec, &cached)
	if cached.ID != brief.ID || cached.GeneratedAt != brief.GeneratedAt {
		t.Fatalf("expected cache hit: got %+v want %+v", cached, brief)
	}

	rec = doJSON(t, h, http.MethodPost, "/api/ai/morning-brief/generate?sync=true&date=not-a-date", nil, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid date status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestAIMorningBriefSSE(t *testing.T) {
	h, token := newAITestRouter(t)

	rec := doJSON(t, h, http.MethodPost, "/api/ai/morning-brief/generate?date=2026-08-01", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("sse status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content-type = %q, want text/event-stream", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data: {"delta":"`) {
		t.Fatalf("sse body missing delta event: %s", body)
	}
	if !strings.HasSuffix(strings.TrimSpace(body), "data: [DONE]") {
		t.Fatalf("sse body should terminate with [DONE]: %s", body)
	}
	if !strings.Contains(body, "*本地生成*") {
		t.Fatalf("rule fallback should stream the full markdown: %s", body)
	}
}

func TestAISyncRecoveryKeyEndpoints(t *testing.T) {
	h, token := newAITestRouter(t)

	rec := doJSON(t, h, http.MethodGet, "/api/ai/sync-recovery-key", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("get recovery key status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var resp dto.RecoveryKeyResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode recovery key: %v", err)
	}
	if resp.RecoveryKey != nil {
		t.Fatalf("recovery key should be null initially, got %v", *resp.RecoveryKey)
	}

	key := "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"
	rec = doJSON(t, h, http.MethodPost, "/api/ai/sync-recovery-key", dto.SetRecoveryKeyRequest{RecoveryKey: key}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("set recovery key status = %d, body=%s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, http.MethodGet, "/api/ai/sync-recovery-key", nil, token)
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode recovery key: %v", err)
	}
	if resp.RecoveryKey == nil || *resp.RecoveryKey != key {
		t.Fatalf("recovery key = %v, want %q", resp.RecoveryKey, key)
	}

	rec = doJSON(t, h, http.MethodPost, "/api/ai/sync-recovery-key", dto.SetRecoveryKeyRequest{RecoveryKey: "bad"}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid recovery key status = %d, want 400", rec.Code)
	}
}

func TestAIEndpointsRequireAuth(t *testing.T) {
	h, _ := newAITestRouter(t)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/ai/config"},
		{http.MethodPatch, "/api/ai/config"},
		{http.MethodGet, "/api/ai/morning-brief"},
		{http.MethodPost, "/api/ai/morning-brief/generate"},
		{http.MethodGet, "/api/ai/sync-recovery-key"},
		{http.MethodPost, "/api/ai/sync-recovery-key"},
	} {
		rec := doJSON(t, h, tc.method, tc.path, nil, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status = %d, want 401", tc.method, tc.path, rec.Code)
		}
	}
}
