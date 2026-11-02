CREATE TABLE push_subscriptions (
    id bigserial PRIMARY KEY,
    username text NOT NULL,
    endpoint text NOT NULL UNIQUE,
    p256dh text NOT NULL,
    auth text NOT NULL,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE push_tests (
    id bigserial PRIMARY KEY,
    subscription_id bigint NOT NULL REFERENCES push_subscriptions(id),
    username text NOT NULL,
    due_at timestamptz NOT NULL,
    status text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued','sending','accepted','failed','expired','unknown','cancelled')),
    detail text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    confirmed_at timestamptz
);
CREATE INDEX push_tests_due ON push_tests(due_at) WHERE status = 'queued';
