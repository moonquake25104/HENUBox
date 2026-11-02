// Package ntfy is a server-configured, opt-in native notification test channel.
package ntfy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Owner          string `json:"owner"`
	ServerURL      string `json:"server_url"`
	WebURL         string `json:"web_url"`
	Topic          string `json:"topic"`
	PublisherURL   string `json:"publisher_url"`
	SenderUser     string `json:"sender_user"`
	SenderPassword string `json:"sender_password"`
	ReaderUser     string `json:"reader_user"`
	ReaderPassword string `json:"reader_password"`
}

func (c Config) Validate() error {
	public, err := url.Parse(c.ServerURL)
	if err != nil || public.Scheme != "https" || public.Host == "" || public.User != nil || public.RawQuery != "" || public.Fragment != "" || public.Path != "" {
		return errors.New("invalid ntfy public URL")
	}
	internal, err := url.Parse(c.PublisherURL)
	if err != nil || internal.Scheme != "http" || internal.Hostname() != "127.0.0.1" || internal.Port() == "" || internal.User != nil || internal.Path != "" || internal.RawQuery != "" || internal.Fragment != "" {
		return errors.New("ntfy publisher must be a configured loopback HTTP endpoint")
	}
	if c.Owner == "" || c.SenderUser == "" || c.ReaderUser == "" || c.SenderUser == c.ReaderUser || len(c.SenderPassword) < 16 || len(c.ReaderPassword) < 16 || !regexp.MustCompile(`^xique-[a-f0-9]{16}$`).MatchString(c.Topic) {
		return errors.New("invalid private ntfy credentials or topic")
	}
	return nil
}

type Client struct {
	Config Config
	HTTP   *http.Client
	mu     sync.Mutex
	last   time.Time
	day    string
	count  int
}

func Load(path string) (*Client, error) {
	if path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err = json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	if err = c.Validate(); err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Client{Config: c, HTTP: &http.Client{Transport: transport, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("ntfy redirects disabled") }}}, nil
}

type Receipt struct {
	ID      string `json:"id"`
	Time    int64  `json:"time"`
	Delayed bool   `json:"delayed"`
	Status  string `json:"status"`
}

var ErrRate = errors.New("请等待 30 秒后再试；每次 API 运行期间每日最多 20 条测试")

func (c *Client) Send(ctx context.Context, delay int) (Receipt, error) {
	if delay != 0 && delay != 60 && delay != 900 {
		return Receipt{}, errors.New("测试延时只能为 0、60 或 900 秒")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if now.Sub(c.last) < 30*time.Second {
		return Receipt{}, ErrRate
	}
	day := now.UTC().Add(8 * time.Hour).Format("2006-01-02")
	if day != c.day {
		c.day = day
		c.count = 0
	}
	if c.count >= 20 {
		return Receipt{}, ErrRate
	}
	c.last = now
	c.count++
	payload := map[string]any{"topic": c.Config.Topic, "message": "这是一条HENUBox测试提醒。收到后请在聊天中告诉我。", "title": "HENUBox · 通知测试", "priority": 4, "click": strings.TrimRight(c.Config.WebURL, "/") + "/#/settings"}
	if delay > 0 {
		payload["delay"] = fmt.Sprintf("%ds", delay)
	}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Config.PublisherURL+"/", bytes.NewReader(data))
	if err != nil {
		return Receipt{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Firebase", "no")
	req.SetBasicAuth(c.Config.SenderUser, c.Config.SenderPassword)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return Receipt{}, errors.New("本地通知服务连接失败或超时，结果不确定；请检查服务状态及手机")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return Receipt{}, fmt.Errorf("本地通知服务返回 HTTP %d", res.StatusCode)
	}
	var receipt Receipt
	if err = json.NewDecoder(io.LimitReader(res.Body, 8192)).Decode(&receipt); err != nil || receipt.ID == "" {
		return Receipt{}, errors.New("通知服务返回无法确认的结果，请检查手机后再决定是否重试")
	}
	receipt.Status = "accepted"
	receipt.Delayed = delay > 0
	return receipt, nil
}
