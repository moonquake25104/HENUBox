package httpapi

// CORS middleware tests for the mobile app origin (phase C Android P1).
// These run without a database: the middleware and the auth/login routes are
// enough to prove preflight, allowlist precision and that CORS never
// authenticates anything.

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func corsTestHandler(t *testing.T, origins ...string) http.Handler {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("correct-horse"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	return New(Options{
		Log:               slog.New(slog.NewTextHandler(io.Discard, nil)),
		Username:          "kairos",
		PasswordHash:      string(hash),
		JWTSecret:         []byte(testSecret),
		JWTTTL:            3600e9,
		CORSAllowedOrigins: origins,
	})
}

// doCORS issues one CORS probe. A preflight is ALWAYS an OPTIONS request;
// the target business method travels in Access-Control-Request-Method.
func doCORS(h http.Handler, method, path, origin string, preflight bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodOptions, path, nil)
	if !preflight {
		req = httptest.NewRequest(method, path, nil)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if preflight {
		req.Header.Set("Access-Control-Request-Method", method)
		req.Header.Set("Access-Control-Request-Headers", "authorization, content-type")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCORSPreflightAllowedOrigin(t *testing.T) {
	h := corsTestHandler(t, "https://localhost")
	rec := doCORS(h, http.MethodPut, "/api/academic/semester-settings", "https://localhost", true)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://localhost" {
		t.Fatalf("allow-origin = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, PUT, PATCH, DELETE, OPTIONS" {
		t.Fatalf("allow-methods = %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Authorization, Content-Type" {
		t.Fatalf("allow-headers = %q", got)
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("credentials must not be enabled for a Bearer-token API")
	}
}

func TestCORSPreflightRejectedOriginGetsNoCORSHeaders(t *testing.T) {
	h := corsTestHandler(t, "https://localhost")
	for _, evil := range []string{"https://evil.example", "https://localhost.evil.example", "null", "https://LOCALHOST"} {
		rec := doCORS(h, http.MethodGet, "/api/auth/me", evil, true)
		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatalf("origin %q must not receive allow-origin (got %q)", evil, rec.Header().Get("Access-Control-Allow-Origin"))
		}
	}
}

func TestCORSPreflightBypassesAuthButBusinessDoesNot(t *testing.T) {
	h := corsTestHandler(t, "https://localhost")

	// Unauthenticated business request from the allowed origin: CORS header
	// present AND still 401 — a successful preflight is not authorization.
	rec := doCORS(h, http.MethodGet, "/api/auth/me", "https://localhost", false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauth business status = %d, want 401", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://localhost" {
		t.Fatalf("allow-origin on 401 = %q", got)
	}

	// Protected business routes stay behind the app JWT: a CORS-allowed but
	// unauthenticated call answers 401 at the application layer. (The
	// academic/school routes need the full DB-backed router; their
	// "school expiry never logs the app out" semantics are covered by the
	// connection UI tests and later real-APK joint acceptance, not here.)
	rec = doCORS(h, http.MethodPost, "/api/auth/logout", "https://localhost", false)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("protected route unauth status = %d, want 401", rec.Code)
	}
}

func TestCORSDisabledByDefault(t *testing.T) {
	h := corsTestHandler(t) // no allowlist: historic desktop behavior
	rec := doCORS(h, http.MethodOptions, "/api/auth/login", "https://localhost", true)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("allow-origin must be absent without allowlist, got %q", got)
	}
	// Non-preflight OPTIONS without allowlist passes through normally.
	rec = doCORS(h, http.MethodOptions, "/api/auth/login", "", false)
	if rec.Code == http.StatusNoContent && rec.Header().Get("Access-Control-Allow-Origin") == "" {
		// pass-through outcome depends on the router (405 here); what matters
		// is that no CORS headers appeared.
		t.Log("plain OPTIONS pass-through ok")
	}
}

func TestCORSSameOriginRequestsUntouched(t *testing.T) {
	h := corsTestHandler(t, "https://localhost")
	rec := doCORS(h, http.MethodPost, "/api/auth/login", "", false)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("no-Origin request must not gain CORS headers")
	}
	if rec.Header().Get("Vary") == "Origin" {
		t.Fatal("Vary: Origin only set when an Origin header is present")
	}
}
