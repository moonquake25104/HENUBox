package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
	"kairos/server/internal/httpapi"
)

// Real TLS, reverse proxy and API authentication, with synthetic credentials.
// No production database, school session, firewall or OS trust store is used.
func TestAndroidTLSProxyCORSAndBearer(t *testing.T) {
	dir := t.TempDir()
	if err := prepare(dir, net.ParseIP("127.0.0.1")); err != nil {
		t.Fatal(err)
	}
	caBytes, err := os.ReadFile(filepath.Join(dir, "xique-mobile-ca.crt"))
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caBytes) {
		t.Fatal("CA")
	}
	cert, err := tls.LoadX509KeyPair(filepath.Join(dir, "server.crt"), filepath.Join(dir, "server.key"))
	if err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("synthetic-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	api := httptest.NewServer(httpapi.New(httpapi.Options{
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Username: "android-qa", PasswordHash: string(hash),
		JWTSecret: []byte("synthetic-android-qa-secret-only"), JWTTTL: time.Hour,
		CORSAllowedOrigins: []string{"https://localhost"},
	}))
	defer api.Close()
	target, _ := url.Parse(api.URL)
	server := httptest.NewUnstartedServer(httputil.NewSingleHostReverseProxy(target))
	server.Config.ErrorLog = nil
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
	server.StartTLS()
	defer server.Close()
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	request := func(method, path, origin, body, token string) *http.Response {
		t.Helper()
		req, _ := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		req.Header.Set("Origin", origin)
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if method == "OPTIONS" {
			req.Header.Set("Access-Control-Request-Method", "POST")
			req.Header.Set("Access-Control-Request-Headers", "authorization, content-type")
		}
		response, err := client.Do(req)
		if err != nil {
			t.Fatal("TLS/API request failed", err)
		}
		t.Cleanup(func() { response.Body.Close() })
		return response
	}
	preflight := request("OPTIONS", "/api/auth/login", "https://localhost", "", "")
	if preflight.StatusCode != 204 || preflight.Header.Get("Access-Control-Allow-Origin") != "https://localhost" {
		t.Fatal("preflight")
	}
	denied := request("GET", "/api/auth/me", "https://localhost", "", "")
	if denied.StatusCode != 401 || denied.Header.Get("Access-Control-Allow-Origin") != "https://localhost" {
		t.Fatal("auth bypass")
	}
	login := request("POST", "/api/auth/login", "https://localhost", `{"username":"android-qa","password":"synthetic-password"}`, "")
	var result struct {
		Token string `json:"token"`
	}
	if login.StatusCode != 200 || json.NewDecoder(login.Body).Decode(&result) != nil || result.Token == "" {
		t.Fatal("login")
	}
	me := request("GET", "/api/auth/me", "https://localhost", "", result.Token)
	if me.StatusCode != 200 || me.Header.Get("Access-Control-Allow-Credentials") != "" {
		t.Fatal("Bearer")
	}
	evil := request("OPTIONS", "/api/auth/me", "https://localhost.evil", "", "")
	if evil.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("origin reflection")
	}

	// The same chain is rejected with an unrelated trust store or hostname.
	leafBytes, _ := os.ReadFile(filepath.Join(dir, "server.crt"))
	block, _ := pem.Decode(leafBytes)
	leaf, _ := x509.ParseCertificate(block.Bytes)
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: x509.NewCertPool(), DNSName: "127.0.0.1"}); err == nil {
		t.Fatal("untrusted issuer accepted")
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: "192.0.2.1"}); err == nil {
		t.Fatal("wrong SAN accepted")
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: "127.0.0.1", CurrentTime: leaf.NotAfter.Add(time.Minute)}); err == nil {
		t.Fatal("expired leaf accepted")
	}
}
