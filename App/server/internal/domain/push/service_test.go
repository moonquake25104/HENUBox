package push

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/jackc/pgx/v5"
	"kairos/server/internal/store"
	"kairos/server/internal/store/migrate"
)

func testSubscription(t *testing.T) webpush.Subscription {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return webpush.Subscription{Endpoint: "https://fcm.googleapis.com/fcm/send/test-device", Keys: webpush.Keys{P256dh: base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), Auth: base64.RawURLEncoding.EncodeToString(make([]byte, 16))}}
}
func TestSubscriptionValidation(t *testing.T) {
	valid := testSubscription(t)
	if !ValidSubscription(valid) {
		t.Fatal("valid P-256 subscription rejected")
	}
	for _, endpoint := range []string{"http://fcm.googleapis.com/a", "https://localhost/a", "https://127.0.0.1/a", "https://fcm.googleapis.com.evil.test/a", "https://evilnotify.windows.com/a", "https://user@fcm.googleapis.com/a", "https://fcm.googleapis.com:8443/a", "https://fcm.googleapis.com/a#fragment"} {
		sub := valid
		sub.Endpoint = endpoint
		if ValidSubscription(sub) {
			t.Errorf("unsafe endpoint accepted: %s", endpoint)
		}
	}
	sub := valid
	sub.Keys.Auth = "bad"
	if ValidSubscription(sub) {
		t.Fatal("invalid auth accepted")
	}
	sub = valid
	sub.Keys.P256dh = base64.RawURLEncoding.EncodeToString(make([]byte, 65))
	if ValidSubscription(sub) {
		t.Fatal("invalid curve point accepted")
	}
	for _, endpoint := range []string{"https://updates.push.services.mozilla.com/wpush/v2/abc", "https://wns2-db5p.notify.windows.com/w/?token=abc"} {
		if !ValidEndpoint(endpoint) {
			t.Fatal("supported endpoint rejected")
		}
	}
}
func TestKeyPersistenceAndCorruption(t *testing.T) {
	dir := t.TempDir()
	first, err := LoadKeys(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadKeys(dir)
	if err != nil || first != second {
		t.Fatal("key changed on restart")
	}
	// Corruption must fail closed, preserving existing device subscriptions.
	bytes, _ := json.Marshal(Keys{Public: first.Public, Private: "invalid"})
	if err := os.WriteFile(dir+"/push-vapid.json", bytes, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKeys(dir); err == nil {
		t.Fatal("corrupt key silently regenerated")
	}
}

type fakeClient struct {
	status int
	calls  int
	t      *testing.T
}

func (f *fakeClient) Do(r *http.Request) (*http.Response, error) {
	f.calls++
	if r.Header.Get("Content-Encoding") != "aes128gcm" || r.Header.Get("Authorization") == "" {
		f.t.Error("missing encrypted Web Push or VAPID headers")
	}
	b, _ := io.ReadAll(r.Body)
	if strings.Contains(string(b), "喜鹊助手") {
		f.t.Error("plaintext push payload")
	}
	return &http.Response{StatusCode: f.status, Body: io.NopCloser(strings.NewReader(""))}, nil
}
func TestPersistentWorkerOutcomes(t *testing.T) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	key, _ := ecdh.P256().GenerateKey(rand.Reader)
	schema := "push_test_" + strings.ToLower(base64.RawURLEncoding.EncodeToString(key.Bytes())[:12])
	schema = strings.ReplaceAll(strings.ReplaceAll(schema, "-", "a"), "_", "b")
	if _, err = conn.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		conn.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		conn.Close(context.Background())
	})
	if _, err = conn.Exec(ctx, "SET search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	if err = migrate.Up(ctx, conn); err != nil {
		t.Fatal(err)
	}
	q := store.New(conn)
	sub := testSubscription(t)
	saved, err := q.SavePushSubscription(ctx, "owner", sub.Endpoint, sub.Keys.P256dh, sub.Keys.Auth)
	if err != nil {
		t.Fatal(err)
	}
	again, err := q.SavePushSubscription(ctx, "owner", sub.Endpoint, sub.Keys.P256dh, sub.Keys.Auth)
	if err != nil || saved.ID != again.ID {
		t.Fatal("subscription not idempotent")
	}
	if _, err = q.GetPushSubscription(ctx, "other", saved.ID); err != pgx.ErrNoRows {
		t.Fatal("cross-user device read allowed")
	}
	if _, err = q.SavePushSubscription(ctx, "other", sub.Endpoint, sub.Keys.P256dh, sub.Keys.Auth); err != pgx.ErrNoRows {
		t.Fatal("cross-user device hijack allowed")
	}
	k, err := LoadKeys(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeClient{status: 201, t: t}
	s := New(q, k, slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.Client = f
	job, err := s.Queue(ctx, "owner", saved.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Queue(ctx, "owner", saved.ID, 0); err != pgx.ErrNoRows {
		t.Fatal("rate limit failed")
	}
	s.tick(ctx)
	s.tick(ctx)
	jobs, err := q.ListPushTests(ctx, "owner")
	if err != nil || len(jobs) != 1 || jobs[0].Status != "accepted" || f.calls != 1 {
		t.Fatalf("accepted job duplicated or missing: %+v %v", jobs, err)
	}
	if err = q.ConfirmPushTest(ctx, "other", job.ID); err != nil {
		t.Fatal(err)
	}
	jobs, _ = q.ListPushTests(ctx, "owner")
	if jobs[0].Confirmed {
		t.Fatal("cross-user confirmation allowed")
	}
	if err = q.ConfirmPushTest(ctx, "owner", job.ID); err != nil {
		t.Fatal(err)
	}
	jobs, _ = q.ListPushTests(ctx, "owner")
	if !jobs[0].Confirmed {
		t.Fatal("user confirmation not saved")
	}
	// A durable delayed test remains queued until due, across service construction.
	conn.Exec(ctx, "UPDATE push_tests SET created_at=now()-interval '1 minute'")
	delayed, err := s.Queue(ctx, "owner", saved.ID, 900)
	if err != nil {
		t.Fatal(err)
	}
	s2 := New(q, k, s.Log)
	s2.Client = f
	s2.tick(ctx)
	if f.calls != 1 {
		t.Fatal("delayed job sent early")
	}
	conn.Exec(ctx, "UPDATE push_tests SET due_at=now()-interval '3 minutes' WHERE id=$1", delayed.ID)
	s2.tick(ctx)
	jobs, _ = q.ListPushTests(ctx, "owner")
	if jobs[0].Status != "expired" || f.calls != 1 {
		t.Fatal("expired test was sent")
	}
	// Endpoint expiry disables the device and cancels queued tests.
	conn.Exec(ctx, "UPDATE push_tests SET created_at=now()-interval '1 minute'")
	_, err = s.Queue(ctx, "owner", saved.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.status = 410
	s.tick(ctx)
	device, _ := q.GetPushSubscription(ctx, "owner", saved.ID)
	if device.Active {
		t.Fatal("expired subscription remained active")
	}
	conn.Exec(ctx, "UPDATE push_tests SET status='sending' WHERE id=$1", delayed.ID)
	if err = q.RecoverPushTests(ctx); err != nil {
		t.Fatal(err)
	}
	jobs, _ = q.ListPushTests(ctx, "owner")
	for _, job := range jobs {
		if job.ID == delayed.ID && job.Status != "unknown" {
			t.Fatal("uncertain interrupted send retried")
		}
	}
	// Deactivation cancels durable pending work.
	q.SavePushSubscription(ctx, "owner", sub.Endpoint, sub.Keys.P256dh, sub.Keys.Auth)
	conn.Exec(ctx, "UPDATE push_tests SET created_at=now()-interval '1 minute'")
	queued, err := s.Queue(ctx, "owner", saved.ID, 900)
	if err != nil {
		t.Fatal(err)
	}
	if err = q.DisablePushSubscription(ctx, "owner", saved.ID); err != nil {
		t.Fatal(err)
	}
	jobs, _ = q.ListPushTests(ctx, "owner")
	if jobs[0].ID != queued.ID || jobs[0].Status != "cancelled" {
		t.Fatal("logout did not cancel pending test")
	}
}
