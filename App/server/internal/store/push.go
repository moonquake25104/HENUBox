package store

import (
	"context"
	"time"
)

type PushSubscription struct {
	ID       int64  `json:"id"`
	Username string `json:"-"`
	Endpoint string `json:"-"`
	P256dh   string `json:"-"`
	Auth     string `json:"-"`
	Active   bool   `json:"active"`
}
type PushTest struct {
	ID             int64     `json:"id"`
	SubscriptionID int64     `json:"subscription_id"`
	DueAt          time.Time `json:"due_at"`
	Status         string    `json:"status"`
	Detail         string    `json:"detail"`
	Confirmed      bool      `json:"confirmed"`
}

func (q *Queries) SavePushSubscription(ctx context.Context, user, endpoint, key, auth string) (PushSubscription, error) {
	var s PushSubscription
	err := q.db.QueryRow(ctx, `INSERT INTO push_subscriptions(username,endpoint,p256dh,auth) VALUES($1,$2,$3,$4)
      ON CONFLICT(endpoint) DO UPDATE SET p256dh=EXCLUDED.p256dh,auth=EXCLUDED.auth,active=true,updated_at=now()
      WHERE push_subscriptions.username=EXCLUDED.username RETURNING id,username,endpoint,p256dh,auth,active`, user, endpoint, key, auth).Scan(&s.ID, &s.Username, &s.Endpoint, &s.P256dh, &s.Auth, &s.Active)
	return s, err
}
func (q *Queries) GetPushSubscription(ctx context.Context, user string, id int64) (PushSubscription, error) {
	var s PushSubscription
	err := q.db.QueryRow(ctx, `SELECT id,username,endpoint,p256dh,auth,active FROM push_subscriptions WHERE id=$1 AND username=$2`, id, user).Scan(&s.ID, &s.Username, &s.Endpoint, &s.P256dh, &s.Auth, &s.Active)
	return s, err
}
func (q *Queries) DisablePushSubscription(ctx context.Context, user string, id int64) error {
	_, err := q.db.Exec(ctx, `UPDATE push_subscriptions SET active=false,updated_at=now() WHERE id=$1 AND username=$2`, id, user)
	if err != nil {
		return err
	}
	_, err = q.db.Exec(ctx, `UPDATE push_tests SET status='cancelled',detail='设备已停用',finished_at=now() WHERE subscription_id=$1 AND username=$2 AND status='queued'`, id, user)
	return err
}
func (q *Queries) QueuePushTest(ctx context.Context, user string, id int64, delay int) (PushTest, error) {
	var t PushTest
	err := q.db.QueryRow(ctx, `INSERT INTO push_tests(subscription_id,username,due_at)
      SELECT $1,$2,now()+$3*interval '1 second' WHERE EXISTS(SELECT 1 FROM push_subscriptions WHERE id=$1 AND username=$2 AND active)
      AND NOT EXISTS(SELECT 1 FROM push_tests WHERE username=$2 AND created_at>now()-interval '30 seconds')
      AND (SELECT count(*) FROM push_tests WHERE username=$2 AND created_at>now()-interval '1 day')<20
      AND (SELECT count(*) FROM push_tests WHERE username=$2 AND status IN ('queued','sending'))<3
      RETURNING id,subscription_id,due_at,status,detail`, id, user, delay).Scan(&t.ID, &t.SubscriptionID, &t.DueAt, &t.Status, &t.Detail)
	return t, err
}
func (q *Queries) ListPushTests(ctx context.Context, user string) ([]PushTest, error) {
	rows, err := q.db.Query(ctx, `SELECT id,subscription_id,due_at,status,detail,confirmed_at IS NOT NULL FROM push_tests WHERE username=$1 ORDER BY id DESC LIMIT 10`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []PushTest{}
	for rows.Next() {
		var t PushTest
		if err := rows.Scan(&t.ID, &t.SubscriptionID, &t.DueAt, &t.Status, &t.Detail, &t.Confirmed); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}
func (q *Queries) ConfirmPushTest(ctx context.Context, user string, id int64) error {
	_, err := q.db.Exec(ctx, `UPDATE push_tests SET confirmed_at=now() WHERE id=$1 AND username=$2 AND status IN ('accepted','unknown')`, id, user)
	return err
}
func (q *Queries) RecoverPushTests(ctx context.Context) error {
	_, err := q.db.Exec(ctx, `UPDATE push_tests SET status='unknown',detail='服务重启，发送结果不确定；请检查手机',finished_at=now() WHERE status='sending'`)
	return err
}
func (q *Queries) ClaimPushTest(ctx context.Context) (PushTest, string, error) {
	var t PushTest
	var user string
	err := q.db.QueryRow(ctx, `UPDATE push_tests SET status='sending' WHERE id=(SELECT id FROM push_tests WHERE status='queued' AND due_at<=now() ORDER BY due_at FOR UPDATE SKIP LOCKED LIMIT 1)
      RETURNING id,subscription_id,due_at,status,detail,username`).Scan(&t.ID, &t.SubscriptionID, &t.DueAt, &t.Status, &t.Detail, &user)
	return t, user, err
}
func (q *Queries) FinishPushTest(ctx context.Context, id int64, status, detail string) error {
	_, err := q.db.Exec(ctx, `UPDATE push_tests SET status=$2,detail=$3,finished_at=now() WHERE id=$1 AND status='sending'`, id, status, detail)
	return err
}
