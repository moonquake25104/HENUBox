package ntfy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testConfig() Config {
	return Config{Owner: "local", ServerURL: "https://192.168.1.2:9443", WebURL: "https://192.168.1.2:8443", Topic: "xique-0123456789abcdef", PublisherURL: "http://127.0.0.1:18081", SenderUser: "sender", SenderPassword: "private-sender-password", ReaderUser: "phone", ReaderPassword: "private-reader-password"}
}
func TestPrivateConfigValidation(t *testing.T) {
	c := testConfig()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"https://example.com", "http://example.com:80", "http://127.0.0.1:80/private", "http://user:pass@127.0.0.1:80", "http://127.0.0.1:80?secret=1"} {
		bad := c
		bad.PublisherURL = u
		if bad.Validate() == nil {
			t.Errorf("unsafe publisher accepted: %s", u)
		}
	}
	bad := c
	bad.Topic = "*"
	if bad.Validate() == nil {
		t.Fatal("wildcard topic accepted")
	}
	if client, err := Load(""); client != nil || err != nil {
		t.Fatal("optional channel failed")
	}
	dir := t.TempDir()
	name := filepath.Join(dir, "ntfy.json")
	data, _ := json.Marshal(c)
	if err := os.WriteFile(name, data, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(name)
	if err != nil || loaded.Config.ReaderUser != "phone" {
		t.Fatal("private configuration load failed")
	}
}
func TestNativePublisherAuthDelayAndRate(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		user, pass, ok := r.BasicAuth()
		if !ok || user != "sender" || pass != "private-sender-password" {
			t.Error("wrong publishing identity")
		}
		if r.Header.Get("Firebase") != "no" {
			t.Error("Firebase not explicitly disabled")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		if payload["topic"] != "xique-0123456789abcdef" || payload["delay"] != "900s" {
			t.Error("topic or durable server delay missing")
		}
		json.NewEncoder(w).Encode(map[string]any{"id": "test-message", "time": time.Now().Add(15 * time.Minute).Unix()})
	}))
	defer server.Close()
	c := &Client{Config: testConfig(), HTTP: server.Client()}
	c.Config.PublisherURL = server.URL
	if _, err := c.Send(context.Background(), -1); err == nil || calls != 0 {
		t.Fatal("invalid delay caused a send")
	}
	receipt, err := c.Send(context.Background(), 900)
	if err != nil || receipt.Status != "accepted" || !receipt.Delayed || receipt.ID != "test-message" {
		t.Fatalf("bad service receipt: %+v %v", receipt, err)
	}
	if _, err = c.Send(context.Background(), 0); err != ErrRate || calls != 1 {
		t.Fatal("rate limiter did not prevent duplicate click")
	}
	c.last = time.Time{}
	c.day = time.Now().UTC().Add(8 * time.Hour).Format("2006-01-02")
	c.count = 20
	if _, err = c.Send(context.Background(), 0); err != ErrRate || calls != 1 {
		t.Fatal("daily test cap failed")
	}
}
func TestPublisherRefusalIsNotDelivery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusForbidden) }))
	defer server.Close()
	c := &Client{Config: testConfig(), HTTP: server.Client()}
	c.Config.PublisherURL = server.URL
	receipt, err := c.Send(context.Background(), 0)
	if err == nil || receipt.Status == "accepted" {
		t.Fatal("refused message reported accepted")
	}
}
