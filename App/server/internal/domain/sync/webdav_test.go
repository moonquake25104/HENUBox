package sync

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSyncFileURLStripsTrailingSlash(t *testing.T) {
	c := NewClient("https://webdav.example.com/", "", "")
	if got := c.syncFileURL(); got != "https://webdav.example.com/kairos-sync.json" {
		t.Fatalf("url = %q", got)
	}
}

func TestSyncFileURLNoTrailingSlash(t *testing.T) {
	c := NewClient("https://webdav.example.com/dav", "", "")
	if got := c.syncFileURL(); got != "https://webdav.example.com/dav/kairos-sync.json" {
		t.Fatalf("url = %q", got)
	}
}

func TestUploadSendsBasicAuthAndIfMatch(t *testing.T) {
	var gotAuth, gotIfMatch string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotIfMatch = r.Header.Get("If-Match")
		w.Header().Set("ETag", `"new-etag"`)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "user", "pass")
	etag := `"old-etag"`
	got, err := c.Upload(context.Background(), &SyncData{SchemaVersion: 2}, &etag)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if got == nil || *got != `"new-etag"` {
		t.Fatalf("etag = %v, want \"new-etag\"", got)
	}
	if !strings.HasPrefix(gotAuth, "Basic ") {
		t.Fatalf("missing Basic auth header: %q", gotAuth)
	}
	if gotIfMatch != `"old-etag"` {
		t.Fatalf("If-Match = %q, want \"old-etag\"", gotIfMatch)
	}
}

func TestUploadWithoutEtagOmitsIfMatch(t *testing.T) {
	var gotIfMatch string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIfMatch = r.Header.Get("If-Match")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "user", "pass")
	if _, err := c.Upload(context.Background(), &SyncData{SchemaVersion: 2}, nil); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if gotIfMatch != "" {
		t.Fatalf("If-Match should be empty, got %q", gotIfMatch)
	}
}

func TestUploadConflictOn412(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPreconditionFailed)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "user", "pass")
	etag := `"stale"`
	_, err := c.Upload(context.Background(), &SyncData{SchemaVersion: 2}, &etag)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}

func TestDownloadParsesSnapshotAndEtag(t *testing.T) {
	data := &SyncData{SchemaVersion: 2, DatasetID: "ds", DeviceID: "dev", ExportedAt: "2024-06-01T10:00:00Z"}
	body, _ := json.Marshal(data)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"abc123"`)
		w.Write(body)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "user", "pass")
	got, err := c.Download(context.Background())
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if got.Etag == nil || *got.Etag != `"abc123"` {
		t.Fatalf("etag = %v", got.Etag)
	}
	if got.Data.SchemaVersion != 2 || got.Data.DatasetID != "ds" {
		t.Fatalf("data = %+v", got.Data)
	}
}

func TestDownloadNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "user", "pass")
	_, err := c.Download(context.Background())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestTestConnectionSuccessAnd404(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNotFound} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		}))
		c := NewClient(srv.URL, "user", "pass")
		ok, err := c.TestConnection(context.Background())
		srv.Close()
		if err != nil {
			t.Fatalf("test connection (status %d): %v", status, err)
		}
		if !ok {
			t.Fatalf("test connection (status %d) = false, want true", status)
		}
	}
}

func TestTestConnectionFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "user", "pass")
	ok, err := c.TestConnection(context.Background())
	if err != nil {
		t.Fatalf("test connection: %v", err)
	}
	if ok {
		t.Fatal("test connection = true, want false on 401")
	}
}

func TestAiSettingsUploadDownloadRoundtrip(t *testing.T) {
	var stored string
	var storedEtag string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			buf := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(buf)
			stored = string(buf)
			storedEtag = r.Header.Get("If-Match")
			w.Header().Set("ETag", `"ai-1"`)
			w.WriteHeader(http.StatusCreated)
		case http.MethodGet:
			if stored == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("ETag", `"ai-1"`)
			w.Write([]byte(stored))
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "user", "pass")
	etag, err := c.UploadAiSettings(context.Background(), `{"format_version":1}`, nil)
	if err != nil {
		t.Fatalf("upload ai settings: %v", err)
	}
	if etag == nil || *etag != `"ai-1"` {
		t.Fatalf("etag = %v", etag)
	}
	if storedEtag != "" {
		t.Fatalf("first upload should have no If-Match, got %q", storedEtag)
	}

	got, err := c.DownloadAiSettings(context.Background())
	if err != nil {
		t.Fatalf("download ai settings: %v", err)
	}
	if got.Blob != `{"format_version":1}` {
		t.Fatalf("blob = %q", got.Blob)
	}
	if got.Etag == nil || *got.Etag != `"ai-1"` {
		t.Fatalf("etag = %v", got.Etag)
	}
}
