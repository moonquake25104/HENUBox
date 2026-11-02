package notify

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeSMTP struct {
	ln        net.Listener
	failFirst bool

	mu       sync.Mutex
	messages []string
	conns    int
}

func startFakeSMTP(t *testing.T, failFirst bool) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &fakeSMTP{ln: ln, failFirst: failFirst}
	go srv.serve()
	t.Cleanup(func() {
		_ = ln.Close()
	})
	return srv
}

func (s *fakeSMTP) port() int {
	return s.ln.Addr().(*net.TCPAddr).Port
}

func (s *fakeSMTP) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *fakeSMTP) handle(conn net.Conn) {
	defer conn.Close()

	s.mu.Lock()
	s.conns++
	connNumber := s.conns
	s.mu.Unlock()

	if s.failFirst && connNumber == 1 {
		_, _ = io.WriteString(conn, "421 Service not available\r\n")
		return
	}

	_, _ = io.WriteString(conn, "220 fake ESMTP ready\r\n")
	reader := bufio.NewReader(conn)

	var data strings.Builder
	inData := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		if inData {
			if line == ".\r\n" || line == ".\n" {
				inData = false
				s.mu.Lock()
				s.messages = append(s.messages, data.String())
				s.mu.Unlock()
				data.Reset()
				_, _ = io.WriteString(conn, "250 OK queued\r\n")
				continue
			}
			if strings.HasPrefix(line, "..") {
				line = line[1:]
			}
			data.WriteString(line)
			continue
		}

		command := strings.ToUpper(strings.TrimRight(line, "\r\n"))
		switch {
		case strings.HasPrefix(command, "EHLO"):
			_, _ = io.WriteString(conn, "250-fake\r\n250 OK\r\n")
		case strings.HasPrefix(command, "HELO"):
			_, _ = io.WriteString(conn, "250 OK\r\n")
		case strings.HasPrefix(command, "MAIL FROM"):
			_, _ = io.WriteString(conn, "250 OK\r\n")
		case strings.HasPrefix(command, "RCPT TO"):
			_, _ = io.WriteString(conn, "250 OK\r\n")
		case strings.HasPrefix(command, "DATA"):
			_, _ = io.WriteString(conn, "354 End data with <CR><LF>.<CR><LF>\r\n")
			inData = true
		case strings.HasPrefix(command, "QUIT"):
			_, _ = io.WriteString(conn, "221 Bye\r\n")
			return
		default:
			_, _ = io.WriteString(conn, "250 OK\r\n")
		}
	}
}

func (s *fakeSMTP) connCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.conns
}

func (s *fakeSMTP) waitMessages(t *testing.T, want int) []string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		s.mu.Lock()
		got := append([]string(nil), s.messages...)
		s.mu.Unlock()
		if len(got) >= want {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d message(s), got %d", want, len(got))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func newTestMailer(port int, cfg SMTPConfig) *Mailer {
	cfg.Host = "127.0.0.1"
	cfg.Port = port
	cfg.TLS = "none"
	return NewMailer(cfg, testLogger())
}

func TestMailerEnabledAndRecipient(t *testing.T) {
	disabled := NewMailer(SMTPConfig{}, testLogger())
	if disabled.Enabled() {
		t.Error("empty host should be disabled")
	}

	fallback := NewMailer(SMTPConfig{From: "Kairos <no-reply@kairos.local>"}, testLogger())
	if got, want := fallback.Recipient(), "no-reply@kairos.local"; got != want {
		t.Errorf("Recipient() = %q, want %q", got, want)
	}

	explicit := NewMailer(SMTPConfig{From: "Kairos <no-reply@kairos.local>", To: "student@example.com"}, testLogger())
	if got, want := explicit.Recipient(), "student@example.com"; got != want {
		t.Errorf("Recipient() = %q, want %q", got, want)
	}
}

func TestMailerSendDisabledIsNoop(t *testing.T) {
	mailer := NewMailer(SMTPConfig{}, testLogger())
	if err := mailer.Send(context.Background(), Message{Subject: "x", Body: "y"}); err != nil {
		t.Fatalf("disabled Send returned error: %v", err)
	}
}

func TestMailerSendRoundtrip(t *testing.T) {
	srv := startFakeSMTP(t, false)
	mailer := newTestMailer(srv.port(), SMTPConfig{
		From: "Kairos <no-reply@kairos.local>",
		To:   "student@example.com",
	})

	subject, body := DailyTaskEmail("背单词")
	if err := mailer.Send(context.Background(), Message{Subject: subject, Body: body}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	message := srv.waitMessages(t, 1)[0]
	for _, want := range []string{
		"From: Kairos <no-reply@kairos.local>",
		"To: student@example.com",
		"Subject: 每日任务提醒",
		"Content-Type: text/plain; charset=UTF-8",
		"「背单词」—— 别忘了今天完成",
	} {
		if !strings.Contains(message, want) {
			t.Errorf("captured message missing %q:\n%s", want, message)
		}
	}
}

func TestMailerSendUsesMessageRecipient(t *testing.T) {
	srv := startFakeSMTP(t, false)
	mailer := newTestMailer(srv.port(), SMTPConfig{
		From: "no-reply@kairos.local",
		To:   "default@example.com",
	})

	if err := mailer.Send(context.Background(), Message{
		To:      "override@example.com",
		Subject: "任务提醒",
		Body:    "「交报告」—— 到时间了，别忘了完成",
	}); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if message := srv.waitMessages(t, 1)[0]; !strings.Contains(message, "To: override@example.com") {
		t.Errorf("message recipient not overridden:\n%s", message)
	}
}

func TestMailerRetriesOnce(t *testing.T) {
	srv := startFakeSMTP(t, true)
	mailer := newTestMailer(srv.port(), SMTPConfig{
		From: "Kairos <no-reply@kairos.local>",
		To:   "student@example.com",
	})

	subject, body := ExamEmail("高等数学", 60)
	if err := mailer.Send(context.Background(), Message{Subject: subject, Body: body}); err != nil {
		t.Fatalf("Send after retry: %v", err)
	}

	if got := srv.connCount(); got != 2 {
		t.Fatalf("connection attempts = %d, want 2", got)
	}
	if message := srv.waitMessages(t, 1)[0]; !strings.Contains(message, "考试「高等数学」将在 1小时 后开始") {
		t.Errorf("retried message body missing:\n%s", message)
	}
}

func TestMailerBodyUsesCRLF(t *testing.T) {
	mailer := newTestMailer(0, SMTPConfig{From: "no-reply@kairos.local"})
	message := mailer.buildMessage("no-reply@kairos.local", "to@example.com", Message{
		Subject: "s",
		Body:    "line1\nline2",
	})
	if strings.Contains(message, "line1\nline2") {
		t.Error("body newlines were not converted to CRLF")
	}
	if !strings.Contains(message, "line1\r\nline2") {
		t.Errorf("expected CRLF body:\n%q", message)
	}
}
