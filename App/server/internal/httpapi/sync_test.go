package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"kairos/server/internal/httpapi/dto"
	"kairos/server/internal/store"
	"kairos/server/internal/store/migrate"
)

func newSyncTestRouter(t *testing.T) (http.Handler, string) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testDatabaseURL())
	if err != nil {
		t.Fatalf("connect to test database: %v", err)
	}
	schema := "sync_httpapi_test_" + randomHex()
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
		Pool:         conn,
		DataDir:      t.TempDir(),
	})
	return h, loginToken(t, h)
}

func TestSyncConfigReadWrite(t *testing.T) {
	h, token := newSyncTestRouter(t)

	rec := doJSON(t, h, http.MethodGet, "/api/sync/config", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("get config status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var cfg dto.SyncConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if cfg.PasswordConfigured {
		t.Fatal("password_configured should be false initially")
	}
	if cfg.DeviceID == nil || cfg.DatasetID == nil {
		t.Fatal("device_id/dataset_id should be backfilled")
	}

	rec = doJSON(t, h, http.MethodPatch, "/api/sync/config", dto.UpdateSyncConfigRequest{
		ServerURL: "https://webdav.example.com",
		Username:  "user",
		AutoSync:  true,
		Password:  strPtr("secret"),
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch config status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var updated dto.SyncConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated config: %v", err)
	}
	if updated.ServerURL != "https://webdav.example.com" || !updated.AutoSync || !updated.PasswordConfigured {
		t.Fatalf("updated config = %+v", updated)
	}

	// Password omitted keeps the stored one.
	rec = doJSON(t, h, http.MethodPatch, "/api/sync/config", dto.UpdateSyncConfigRequest{
		ServerURL: "https://webdav.example.com",
		Username:  "user",
		AutoSync:  false,
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch config status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated config: %v", err)
	}
	if !updated.PasswordConfigured {
		t.Fatal("password should be preserved when omitted")
	}

	// Empty password clears it.
	rec = doJSON(t, h, http.MethodPatch, "/api/sync/config", dto.UpdateSyncConfigRequest{
		ServerURL: "https://webdav.example.com",
		Username:  "user",
		AutoSync:  false,
		Password:  strPtr(""),
	}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch config status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated config: %v", err)
	}
	if updated.PasswordConfigured {
		t.Fatal("password should be cleared by empty string")
	}
}

func TestSyncTestConnection(t *testing.T) {
	h, token := newSyncTestRouter(t)

	// No server_url configured → 400.
	rec := doJSON(t, h, http.MethodPost, "/api/sync/test", nil, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("test status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}

	// Configure a reachable stub → true.
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer stub.Close()
	doJSON(t, h, http.MethodPatch, "/api/sync/config", dto.UpdateSyncConfigRequest{
		ServerURL: stub.URL,
		Username:  "user",
		Password:  strPtr("pass"),
	}, token)

	rec = doJSON(t, h, http.MethodPost, "/api/sync/test", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("test status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "true\n" {
		t.Fatalf("test body = %q, want true", rec.Body.String())
	}
}

func TestSyncNowEndpoint(t *testing.T) {
	h, token := newSyncTestRouter(t)

	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.WriteHeader(http.StatusNotFound)
		case http.MethodPut:
			w.Header().Set("ETag", `"snap-1"`)
			w.WriteHeader(http.StatusCreated)
		case http.MethodHead:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer stub.Close()
	doJSON(t, h, http.MethodPatch, "/api/sync/config", dto.UpdateSyncConfigRequest{
		ServerURL: stub.URL,
		Username:  "user",
		Password:  strPtr("pass"),
	}, token)

	rec := doJSON(t, h, http.MethodPost, "/api/sync/now", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("sync now status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var result dto.SyncResult
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode sync result: %v", err)
	}
	if !result.Uploaded || result.Downloaded {
		t.Fatalf("result = %+v, want uploaded=true downloaded=false", result)
	}

	// last_sync_at should now be persisted.
	rec = doJSON(t, h, http.MethodGet, "/api/sync/config", nil, token)
	var cfg dto.SyncConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil {
		t.Fatalf("decode config: %v", err)
	}
	if cfg.LastSyncAt == nil {
		t.Fatal("last_sync_at should be set after sync now")
	}
	if cfg.RemoteEtag == nil || *cfg.RemoteEtag != `"snap-1"` {
		t.Fatalf("remote_etag = %v, want \"snap-1\"", cfg.RemoteEtag)
	}
}

func TestSyncRecoveryKeyEndpoints(t *testing.T) {
	h, token := newSyncTestRouter(t)

	rec := doJSON(t, h, http.MethodGet, "/api/sync/ai-recovery-key", nil, token)
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
	rec = doJSON(t, h, http.MethodPost, "/api/sync/ai-recovery-key", dto.SetRecoveryKeyRequest{RecoveryKey: key}, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("set recovery key status = %d, body=%s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, h, http.MethodGet, "/api/sync/ai-recovery-key", nil, token)
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode recovery key: %v", err)
	}
	if resp.RecoveryKey == nil || *resp.RecoveryKey != key {
		t.Fatalf("recovery key = %v, want %q", resp.RecoveryKey, key)
	}

	rec = doJSON(t, h, http.MethodPost, "/api/sync/ai-recovery-key", dto.SetRecoveryKeyRequest{RecoveryKey: "bad"}, token)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid key status = %d, want 400", rec.Code)
	}
}

func TestSyncEndpointsRequireAuth(t *testing.T) {
	h, _ := newSyncTestRouter(t)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/sync/config"},
		{http.MethodPatch, "/api/sync/config"},
		{http.MethodPost, "/api/sync/test"},
		{http.MethodPost, "/api/sync/now"},
		{http.MethodGet, "/api/sync/ai-recovery-key"},
		{http.MethodPost, "/api/sync/ai-recovery-key"},
	} {
		rec := doJSON(t, h, tc.method, tc.path, nil, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status = %d, want 401", tc.method, tc.path, rec.Code)
		}
	}
}
