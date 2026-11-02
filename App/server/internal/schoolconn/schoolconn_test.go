package schoolconn

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func randRead(b []byte) (int, error) { return rand.Read(b) }

func base64DecodeStd(s string) (string, error) {
	unescaped, err := url.QueryUnescape(s)
	if err != nil {
		unescaped = s
	}
	raw, err := base64.StdEncoding.DecodeString(unescaped)
	if err != nil {
		raw, err = base64.StdEncoding.DecodeString(s)
		if err != nil {
			return "", err
		}
	}
	return string(raw), nil
}

func TestSealOpenRoundtrip(t *testing.T) {
	key := make([]byte, 32)
	if _, err := osRandRead(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	aad := SessionAAD("alice", "henu", 3)
	sealed, err := Seal(key, []byte(`{"cookies":[],"user_code":"u1"}`), aad)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	plain, err := Open(key, sealed, aad)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !bytes.Contains(plain, []byte("u1")) {
		t.Fatalf("roundtrip content mismatch: %q", plain)
	}
}

func osRandRead(b []byte) (int, error) {
	return rand.Read(b)
}

// Wrong key, tampered ciphertext, wrong AAD and a foreign row's blob must all
// collapse into ErrSessionUnreadable: callers cannot distinguish them and must
// never treat a failure as a clean logout.
func TestOpenFailuresAreUniform(t *testing.T) {
	key := make([]byte, 32)
	if _, err := randRead(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	otherKey := make([]byte, 32)
	otherKey[0] = 1
	sealed, err := Seal(key, []byte("secret"), SessionAAD("alice", "henu", 1))
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	cases := map[string][]byte{
		"wrong key": func() []byte { s, _ := Seal(otherKey, []byte("x"), SessionAAD("alice", "henu", 1)); return s }(),
		"tampered":  append(sealed[:len(sealed)-1], sealed[len(sealed)-1]^0xff),
		"truncated": sealed[:len(sealed)-4],
		"empty":     {},
	}
	for name, ct := range cases {
		if _, err := Open(key, ct, SessionAAD("alice", "henu", 1)); !errors.Is(err, ErrSessionUnreadable) {
			t.Fatalf("%s: err = %v, want ErrSessionUnreadable", name, err)
		}
	}
	if _, err := Open(key, sealed, SessionAAD("mallory", "henu", 1)); !errors.Is(err, ErrSessionUnreadable) {
		t.Fatalf("wrong aad: err = %v", err)
	}
	if _, err := Open(key, sealed, SessionAAD("alice", "henu", 2)); !errors.Is(err, ErrSessionUnreadable) {
		t.Fatalf("wrong generation: err = %v", err)
	}
}

func TestLoadSessionKeyLifecycle(t *testing.T) {
	dir := t.TempDir()
	keyFile := filepath.Join(dir, "session.key")

	// create=false must not create anything.
	if _, err := LoadSessionKey(keyFile, false); !os.IsNotExist(err) {
		t.Fatalf("missing key err = %v, want not-exist", err)
	}
	key, err := LoadSessionKey(keyFile, true)
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("key length = %d", len(key))
	}
	again, err := LoadSessionKey(keyFile, false)
	if err != nil || string(again) != string(key) {
		t.Fatalf("reload mismatch: %v", err)
	}
	if err := os.WriteFile(keyFile, make([]byte, 10), 0o600); err != nil {
		t.Fatalf("write short key: %v", err)
	}
	if _, err := LoadSessionKey(keyFile, true); err == nil {
		t.Fatal("corrupt key file must error, not be silently replaced")
	}
}

func TestBuildTimetableParams(t *testing.T) {
	got := BuildTimetableParams(&Term{Year: "2026", Term: "0", Label: "第一学期", UserCode: "u1"})
	decoded, err := base64DecodeStd(got)
	if err != nil {
		t.Fatalf("params not decodable: %v", err)
	}
	if !strings.Contains(decoded, "xn=2026") || !strings.Contains(decoded, "xq=0") || !strings.Contains(decoded, "xh=u1") {
		t.Fatalf("decoded params = %q", decoded)
	}
}

func TestClientClassification(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		switch {
		case strings.HasSuffix(r.URL.Path, "showYearTerm.action"):
			w.Header().Set("Content-Type", "text/plain;charset=UTF-8")
			_, _ = w.Write([]byte(`{"xn":"2026","xqM":"0","xqName":"第一学期","userCode":"u1"}`))
		case strings.HasSuffix(r.URL.Path, "data10319.jsp"):
			if r.URL.Query().Get("params") == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "text/html;charset=GBK")
			_, _ = w.Write([]byte("<html>timetable</html>"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	client, err := NewClient(srv.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	term, err := client.FetchTerm(context.Background())
	if err != nil {
		t.Fatalf("fetch term: %v", err)
	}
	if term.Year != "2026" || term.Term != "0" || term.Label != "第一学期" || term.UserCode != "u1" {
		t.Fatalf("term = %+v", term)
	}
	raw, err := client.FetchTimetable(context.Background(), term)
	if err != nil || !bytes.Contains(raw, []byte("timetable")) {
		t.Fatalf("fetch timetable: %v", err)
	}
}

func TestClientErrorClassification(t *testing.T) {
	mode := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch mode {
		case "login":
			w.Header().Set("Content-Type", "text/html;charset=UTF-8")
			_, _ = w.Write([]byte(`<html><form action="/cas/logon.action"><input name="randnumber"></form></html>`))
		case "boom":
			http.Error(w, "internal", http.StatusInternalServerError)
		case "forbidden":
			w.WriteHeader(http.StatusForbidden)
		case "term-bad-json":
			_, _ = w.Write([]byte("not json at all"))
		case "term-missing-fields":
			_, _ = w.Write([]byte(`{"xn":"","xqM":"0"}`))
		case "huge":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(make([]byte, maxResponseBytes+1024))
		default:
			_, _ = w.Write([]byte(`{"xn":"2026","xqM":"0","xqName":"第一学期","userCode":"u1"}`))
		}
	}))
	defer srv.Close()

	client, _ := NewClient(srv.URL)
	fetch := func() error {
		_, err := client.FetchTerm(context.Background())
		return err
	}
	mode = "login"
	if err := fetch(); !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("login page err = %v", err)
	}
	mode = "boom"
	if err := fetch(); !errors.Is(err, ErrUpstreamUnavailable) {
		t.Fatalf("5xx err = %v", err)
	}
	mode = "forbidden"
	if err := fetch(); !errors.Is(err, ErrUpstreamSafe) {
		t.Fatalf("403 err = %v", err)
	}
	mode = "term-bad-json"
	if err := fetch(); !errors.Is(err, ErrUpstreamSafe) {
		t.Fatalf("bad json err = %v", err)
	}
	mode = "term-missing-fields"
	if err := fetch(); !errors.Is(err, ErrUpstreamSafe) {
		t.Fatalf("missing fields err = %v", err)
	}
	mode = "huge"
	if err := fetch(); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("oversized err = %v", err)
	}
	mode = ""
	if err := fetch(); err != nil {
		t.Fatalf("happy path err = %v", err)
	}
}

func TestClientBlocksCrossHostRedirect(t *testing.T) {
	inner := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"xn":"2026","xqM":"0","xqName":"第一学期","userCode":"u1"}`))
	}))
	defer inner.Close()
	outer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, inner.URL+"/jw/common/showYearTerm.action", http.StatusFound)
	}))
	defer outer.Close()

	client, _ := NewClient(outer.URL)
	if _, err := client.FetchTerm(context.Background()); !errors.Is(err, ErrUpstreamUnavailable) {
		t.Fatalf("cross-host redirect err = %v, want unavailable-classified", err)
	}
}

func TestClientRedirectLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/jw/common/showYearTerm.action?n=next", http.StatusFound)
	}))
	defer srv.Close()
	client, _ := NewClient(srv.URL)
	if _, err := client.FetchTerm(context.Background()); err == nil {
		t.Fatal("redirect loop must terminate with an error")
	}
}
