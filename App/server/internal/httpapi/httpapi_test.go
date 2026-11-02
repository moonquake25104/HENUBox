package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"kairos/server/internal/httpapi/dto"
)

const testSecret = "0123456789abcdef0123456789abcdef" // 32 bytes

func testHandler(t *testing.T) http.Handler {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(Options{
		Log:          log,
		Username:     "kairos",
		PasswordHash: string(hash),
		JWTSecret:    []byte(testSecret),
		JWTTTL:       time.Hour,
	})
}

// withCommitAck fills the now-mandatory empty warning array for tests that do
// not exercise the strict commit contract. Production never defaults it.
func withCommitAck(path string, body any) any {
	if !strings.HasSuffix(path, "/commit") {
		return body
	}
	m, ok := body.(map[string]any)
	if !ok {
		return body
	}
	if _, exists := m["acknowledged_warning_codes"]; !exists {
		m["acknowledged_warning_codes"] = []string{}
	}
	return m
}

func doJSON(t *testing.T, h http.Handler, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	body = withCommitAck(path, body)
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealthzUnauthenticated(t *testing.T) {
	h := testHandler(t)
	rec := doJSON(t, h, http.MethodGet, "/healthz", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", rec.Code)
	}
}

func TestLoginSuccess(t *testing.T) {
	h := testHandler(t)
	rec := doJSON(t, h, http.MethodPost, "/api/auth/login",
		dto.LoginRequest{Username: "kairos", Password: "correct-horse"}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp dto.LoginResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("login response missing token")
	}
	if resp.User.Username != "kairos" {
		t.Fatalf("login user = %q, want kairos", resp.User.Username)
	}
}

func TestLoginWrongPassword(t *testing.T) {
	h := testHandler(t)
	rec := doJSON(t, h, http.MethodPost, "/api/auth/login",
		dto.LoginRequest{Username: "kairos", Password: "wrong"}, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("login status = %d, want 401", rec.Code)
	}
}

func TestLoginWrongUsername(t *testing.T) {
	h := testHandler(t)
	rec := doJSON(t, h, http.MethodPost, "/api/auth/login",
		dto.LoginRequest{Username: "nobody", Password: "correct-horse"}, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("login status = %d, want 401", rec.Code)
	}
}

func TestLoginUsernameCaseInsensitive(t *testing.T) {
	h := testHandler(t)
	rec := doJSON(t, h, http.MethodPost, "/api/auth/login",
		dto.LoginRequest{Username: "KAIROS", Password: "correct-horse"}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

func TestLoginUsernameTrimmed(t *testing.T) {
	h := testHandler(t)
	rec := doJSON(t, h, http.MethodPost, "/api/auth/login",
		dto.LoginRequest{Username: "  kairos  ", Password: "correct-horse"}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

func TestMeWithValidToken(t *testing.T) {
	h := testHandler(t)
	login := doJSON(t, h, http.MethodPost, "/api/auth/login",
		dto.LoginRequest{Username: "kairos", Password: "correct-horse"}, "")
	var resp dto.LoginResponse
	if err := json.Unmarshal(login.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode login: %v", err)
	}

	rec := doJSON(t, h, http.MethodGet, "/api/auth/me", nil, resp.Token)
	if rec.Code != http.StatusOK {
		t.Fatalf("me status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var me dto.MeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if me.Username != "kairos" {
		t.Fatalf("me username = %q, want kairos", me.Username)
	}
}

func TestMeMissingToken(t *testing.T) {
	h := testHandler(t)
	rec := doJSON(t, h, http.MethodGet, "/api/auth/me", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("me status = %d, want 401", rec.Code)
	}
}

func TestMeForgedToken(t *testing.T) {
	h := testHandler(t)
	rec := doJSON(t, h, http.MethodGet, "/api/auth/me", nil, "not.a.real.token")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("me status = %d, want 401", rec.Code)
	}
}

func TestLogoutRequiresAuth(t *testing.T) {
	h := testHandler(t)
	rec := doJSON(t, h, http.MethodPost, "/api/auth/logout", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("logout status = %d, want 401", rec.Code)
	}
}
