package schoolconn

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

type loginProtocolWriter struct{ bytes.Buffer }

func (w *loginProtocolWriter) Close() error { return nil }

func TestReadHelperCookieExpiryWire(t *testing.T) {
	writer := &loginProtocolWriter{}
	input := `{"event":"cookies","cookies":[{"name":"session","value":"synthetic","expires":null},{"name":"persistent","value":"synthetic","expires":"2026-10-26T15:14:56.250Z"}]}` + "\n"
	out, err := readHelperEvents(context.Background(), strings.NewReader(input), writer)
	if err != nil || out.Failed || len(out.Cookies) != 2 {
		t.Fatal("valid helper frame rejected")
	}
	if out.Cookies[0].Expires != nil {
		t.Fatal("session cookie gained an expiry")
	}
	expected := time.Date(2026, 10, 26, 15, 14, 56, 250000000, time.UTC)
	if out.Cookies[1].Expires == nil || !out.Cookies[1].Expires.Equal(expected) {
		t.Fatal("persistent expiry was not preserved")
	}
	if writer.String() != "ack\n" {
		t.Fatal("valid cookie frame did not receive ack")
	}
}

func TestReadHelperMalformedCookieFrameFailsPromptly(t *testing.T) {
	writer := &loginProtocolWriter{}
	// Exact old mismatch: Playwright numeric seconds against Go *time.Time.
	input := `{"event":"cookies","cookies":[{"name":"session","value":"synthetic","expires":-1}]}` + "\n"
	out, err := readHelperEvents(context.Background(), strings.NewReader(input), writer)
	if err != nil || !out.Failed {
		t.Fatal("malformed cookie frame was silently ignored")
	}
	if writer.String() != "cancel\n" {
		t.Fatal("malformed frame did not cancel helper")
	}
	if strings.Contains(out.Message, "synthetic") {
		t.Fatal("protocol error exposed cookie data")
	}
}
