package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrNotFound reports a 404 from the remote: the caller treats it as "no
// remote data" and skips the download.
var ErrNotFound = errors.New("no remote data found")

// ErrConflict reports HTTP 412 Precondition Failed: the remote changed since
// the last download, so the caller should re-download and retry once.
var ErrConflict = errors.New("remote changed during upload")

const clientTimeout = 10 * time.Second

// Client is the WebDAV transport: Basic Auth, ETag conditional uploads and a
// 10s timeout, mirroring src-tauri/src/sync/webdav.rs.
type Client struct {
	serverURL string
	username  string
	password  string
	http      *http.Client
}

// NewClient builds a WebDAV client for the given credentials.
func NewClient(serverURL, username, password string) *Client {
	return &Client{
		serverURL: serverURL,
		username:  username,
		password:  password,
		http:      &http.Client{Timeout: clientTimeout},
	}
}

// DownloadedSyncData is the result of downloading kairos-sync.json. Etag is
// used for the next conditional upload; nil means the server sent no ETag.
type DownloadedSyncData struct {
	Data *SyncData
	Etag *string
}

// DownloadedAiSettings is the result of downloading kairos-ai-settings.enc.
type DownloadedAiSettings struct {
	Blob string
	Etag *string
}

func (c *Client) syncFileURL() string {
	return c.fileURL("kairos-sync.json")
}

func (c *Client) fileURL(name string) string {
	base := strings.TrimRight(c.serverURL, "/")
	return base + "/" + name
}

// Upload PUTs the snapshot. When remoteEtag is non-empty an If-Match header is
// added for a conditional upload. Returns the new server ETag (nil when the
// server sent none). ErrConflict is returned on HTTP 412.
func (c *Client) Upload(ctx context.Context, data *SyncData, remoteEtag *string) (*string, error) {
	body, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("同步数据序列化失败：%w", err)
	}
	return c.putJSON(ctx, "kairos-sync.json", string(body), remoteEtag)
}

// Download GETs and parses kairos-sync.json. ErrNotFound on 404.
func (c *Client) Download(ctx context.Context) (*DownloadedSyncData, error) {
	etag, body, err := c.getFile(ctx, "kairos-sync.json")
	if err != nil {
		return nil, err
	}
	var data SyncData
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		return nil, fmt.Errorf("同步数据解析失败：%w", err)
	}
	return &DownloadedSyncData{Data: &data, Etag: etag}, nil
}

// TestConnection HEADs the sync file; success or 404 means the server is
// reachable and authenticated.
func (c *Client) TestConnection(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, c.syncFileURL(), nil)
	if err != nil {
		return false, err
	}
	req.SetBasicAuth(c.username, c.password)
	resp, err := c.http.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound, nil
}

// UploadAiSettings PUTs the encrypted AI settings blob. ErrConflict on 412.
func (c *Client) UploadAiSettings(ctx context.Context, blob string, remoteEtag *string) (*string, error) {
	return c.putJSON(ctx, "kairos-ai-settings.enc", blob, remoteEtag)
}

// DownloadAiSettings GETs the encrypted AI settings blob. ErrNotFound on 404.
func (c *Client) DownloadAiSettings(ctx context.Context) (*DownloadedAiSettings, error) {
	etag, body, err := c.getFile(ctx, "kairos-ai-settings.enc")
	if err != nil {
		return nil, err
	}
	return &DownloadedAiSettings{Blob: body, Etag: etag}, nil
}

func (c *Client) putJSON(ctx context.Context, fileName, body string, remoteEtag *string) (*string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.fileURL(fileName), strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("同步数据序列化失败：%w", err)
	}
	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")
	if remoteEtag != nil && strings.TrimSpace(*remoteEtag) != "" {
		req.Header.Set("If-Match", *remoteEtag)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusNoContent:
		return responseEtag(resp.Header), nil
	case resp.StatusCode == http.StatusPreconditionFailed:
		return nil, ErrConflict
	default:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("上传失败：HTTP %d — %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
}

func (c *Client) getFile(ctx context.Context, fileName string) (*string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.fileURL(fileName), nil)
	if err != nil {
		return nil, "", err
	}
	req.SetBasicAuth(c.username, c.password)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, "", fmt.Errorf("读取响应内容失败：%w", err)
		}
		return responseEtag(resp.Header), string(body), nil
	case resp.StatusCode == http.StatusNotFound:
		return nil, "", ErrNotFound
	default:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, "", fmt.Errorf("下载失败：HTTP %d — %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
}

func responseEtag(header http.Header) *string {
	v := strings.TrimSpace(header.Get("ETag"))
	if v == "" {
		return nil
	}
	return &v
}
