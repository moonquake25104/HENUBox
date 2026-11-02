// Package push implements an opt-in device test channel, separate from course reminders.
package push

import (
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/jackc/pgx/v5"
	"kairos/server/internal/store"
)

type Keys struct {
	Public  string `json:"public"`
	Private string `json:"private"`
}
type Service struct {
	Q      *store.Queries
	Keys   Keys
	Client webpush.HTTPClient
	Log    *slog.Logger
	mu     sync.Mutex
}

func LoadKeys(dir string) (Keys, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return Keys{}, err
	}
	name := filepath.Join(dir, "push-vapid.json")
	b, err := os.ReadFile(name)
	if os.IsNotExist(err) {
		private, public, e := webpush.GenerateVAPIDKeys()
		if e != nil {
			return Keys{}, e
		}
		k := Keys{public, private}
		b, _ = json.Marshal(k)
		f, e := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return Keys{}, e
		}
		_, e = f.Write(b)
		closeErr := f.Close()
		if e != nil {
			return Keys{}, e
		}
		if closeErr != nil {
			return Keys{}, closeErr
		}
	} else if err != nil {
		return Keys{}, err
	}
	var k Keys
	if err := json.Unmarshal(b, &k); err != nil {
		return k, err
	}
	raw, err := base64.RawURLEncoding.DecodeString(k.Private)
	if err != nil {
		return k, err
	}
	private, err := ecdh.P256().NewPrivateKey(raw)
	if err != nil {
		return k, err
	}
	if base64.RawURLEncoding.EncodeToString(private.PublicKey().Bytes()) != k.Public {
		return k, errors.New("VAPID key pair does not match")
	}
	return k, nil
}
func ValidEndpoint(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil || len(endpoint) > 4096 || u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.Opaque != "" || u.Path == "" || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "fcm.googleapis.com" || host == "updates.push.services.mozilla.com" || strings.HasSuffix(host, ".notify.windows.com")
}
func ValidSubscription(s webpush.Subscription) bool {
	if !ValidEndpoint(s.Endpoint) {
		return false
	}
	key, err := base64.RawURLEncoding.DecodeString(s.Keys.P256dh)
	if err != nil {
		return false
	}
	if _, err := ecdh.P256().NewPublicKey(key); err != nil {
		return false
	}
	auth, err := base64.RawURLEncoding.DecodeString(s.Keys.Auth)
	return err == nil && len(auth) == 16
}
func New(q *store.Queries, k Keys, log *slog.Logger) *Service {
	transport := &http.Transport{Proxy: nil, TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, err
			}
			for _, ip := range ips {
				if !ip.IP.IsGlobalUnicast() || ip.IP.IsPrivate() || ip.IP.IsLoopback() || ip.IP.IsLinkLocalUnicast() {
					return nil, errors.New("non-public push destination")
				}
			}
			if len(ips) == 0 {
				return nil, errors.New("no push destination")
			}
			return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ips[0].IP.String(), port))
		}}
	return &Service{Q: q, Keys: k, Log: log, Client: &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("push redirects disabled") }}}
}
func (s *Service) Queue(ctx context.Context, user string, id int64, delay int) (store.PushTest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Q.QueuePushTest(ctx, user, id, delay)
}
func (s *Service) Run(ctx context.Context) {
	if err := s.Q.RecoverPushTests(ctx); err != nil {
		s.Log.Error("push recovery failed")
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}
func (s *Service) tick(ctx context.Context) {
	job, user, err := s.Q.ClaimPushTest(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return
	}
	if err != nil {
		s.Log.Error("claim push test failed")
		return
	}
	status, detail := "failed", "发送失败"
	sub, err := s.Q.GetPushSubscription(ctx, user, job.SubscriptionID)
	switch {
	case time.Since(job.DueAt) > 2*time.Minute:
		status, detail = "expired", "测试已过期，请重新安排"
	case err != nil:
		detail = "设备记录不可用"
	case !sub.Active:
		status, detail = "cancelled", "设备已停用"
	default:
		target := webpush.Subscription{Endpoint: sub.Endpoint, Keys: webpush.Keys{P256dh: sub.P256dh, Auth: sub.Auth}}
		if ValidSubscription(target) {
			payload, _ := json.Marshal(map[string]any{"title": "HENUBox · 手机测试", "body": "这是一条测试提醒，收到后请回到设置页确认。", "tag": "xique-test-" + strconv.FormatInt(job.ID, 10), "url": "/#/settings", "test_id": job.ID})
			sendCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			resp, e := webpush.SendNotificationWithContext(sendCtx, payload, &target, &webpush.Options{HTTPClient: s.Client, Subscriber: "https://xique-assistant.local", VAPIDPublicKey: s.Keys.Public, VAPIDPrivateKey: s.Keys.Private, TTL: 120, Urgency: webpush.UrgencyHigh})
			cancel()
			if e != nil {
				status, detail = "unknown", "连接失败或超时，送达结果不确定；请检查手机"
			} else {
				resp.Body.Close()
				switch {
				case resp.StatusCode >= 200 && resp.StatusCode < 300:
					status, detail = "accepted", "推送服务已接受，等待你确认手机收到"
				case resp.StatusCode == 404 || resp.StatusCode == 410:
					detail = "订阅已失效，请停用后重新启用"
					_ = s.Q.DisablePushSubscription(ctx, user, sub.ID)
				default:
					detail = fmt.Sprintf("推送服务返回 HTTP %d；本次不自动重发", resp.StatusCode)
				}
			}
		} else {
			detail = "设备订阅不受支持，请重新启用"
		}
	}
	// Never log endpoint, keys, payload or raw transport errors.
	finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := s.Q.FinishPushTest(finishCtx, job.ID, status, detail); err != nil {
		s.Log.Error("save push outcome failed", "test_id", job.ID)
	}
}
