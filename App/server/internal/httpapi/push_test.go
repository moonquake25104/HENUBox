package httpapi

import (
	"net/http"
	"strings"
	"testing"

	"kairos/server/internal/domain/ntfy"
	"kairos/server/internal/domain/push"
)

func TestNativeChannelConfigurationPrivacy(t *testing.T) {
	c := &ntfy.Client{Config: ntfy.Config{Owner: "kairos", ServerURL: "https://192.168.1.2:9443", Topic: "xique-test", ReaderUser: "phone", ReaderPassword: "reader-private", SenderPassword: "sender-private"}}
	h, token := newCoreTestRouterConfigured(t, func(opts *Options) { opts.NativePush = c })
	rec := doJSON(t, h, http.MethodGet, "/api/ntfy/config", nil, "")
	if rec.Code != 401 {
		t.Fatal("native config unauthenticated")
	}
	rec = doJSON(t, h, http.MethodGet, "/api/ntfy/config", nil, token)
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "private") {
		t.Fatal("native config leaked secret")
	}
	rec = doJSON(t, h, http.MethodPost, "/api/ntfy/setup", nil, token)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "reader-private") || strings.Contains(rec.Body.String(), "sender-private") {
		t.Fatal("reader setup exposed wrong credential")
	}
	rec = doJSON(t, h, http.MethodPost, "/api/ntfy/test", map[string]any{"delay_seconds": -1}, token)
	if rec.Code != 400 {
		t.Fatal("native invalid delay accepted")
	}
	c.Config.Owner = "another-user"
	rec = doJSON(t, h, http.MethodPost, "/api/ntfy/setup", nil, token)
	if rec.Code != 403 {
		t.Fatal("another user accessed native receiver password")
	}
}

func TestPushProtectedRoutes(t *testing.T) {
	h, token := newCoreTestRouterConfigured(t, func(opts *Options) {
		keys, err := push.LoadKeys(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		opts.Push = push.New(opts.Store, keys, opts.Log)
	})
	rec := doJSON(t, h, http.MethodGet, "/api/push/config", nil, "")
	if rec.Code != 401 {
		t.Fatal("unauthenticated push config accessible")
	}
	rec = doJSON(t, h, http.MethodGet, "/api/push/config", nil, token)
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "private") {
		t.Fatal("push config response invalid")
	}
	rec = doJSON(t, h, http.MethodPost, "/api/push/subscriptions", map[string]any{"endpoint": "http://127.0.0.1/internal"}, token)
	if rec.Code != 400 {
		t.Fatal("unsafe subscription accepted")
	}
	rec = doJSON(t, h, http.MethodPost, "/api/push/test", map[string]any{"subscription_id": 1, "delay_seconds": -1}, token)
	if rec.Code != 400 {
		t.Fatal("invalid delay accepted")
	}
	rec = doJSON(t, h, http.MethodPost, "/api/push/test", map[string]any{"subscription_id": 1, "delay_seconds": 0}, token)
	if rec.Code != 404 {
		t.Fatal("unknown device accepted")
	}
	rec = doJSON(t, h, http.MethodGet, "/api/push/tests", nil, token)
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatal("initial history invalid")
	}
}
