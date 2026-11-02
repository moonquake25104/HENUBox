package notify

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
)

const (
	defaultSMTPPort    = 587
	defaultFromHeader  = "Kairos <no-reply@kairos.local>"
	defaultFromAddress = "no-reply@kairos.local"
	defaultSubject     = "Kairos 提醒"
	sendAttempts       = 2
)

// SMTPConfig holds the SMTP connection settings. TLS selects the transport:
// "starttls" (default), "implicit" (465, TLS from the first byte) or "none".
type SMTPConfig struct {
	Host string
	Port int
	User string
	Pass string
	From string
	To   string
	TLS  string
}

// Message is one outgoing plain-text email. To overrides the mailer recipient.
type Message struct {
	To      string
	Subject string
	Body    string
}

// Mailer sends email over SMTP. It is safe for concurrent use.
type Mailer struct {
	cfg SMTPConfig
	log *slog.Logger
}

// NewMailer builds a mailer. A nil logger falls back to slog.Default.
func NewMailer(cfg SMTPConfig, log *slog.Logger) *Mailer {
	if log == nil {
		log = slog.Default()
	}
	return &Mailer{cfg: cfg, log: log}
}

// Enabled reports whether an SMTP host is configured.
func (m *Mailer) Enabled() bool {
	return m != nil && strings.TrimSpace(m.cfg.Host) != ""
}

// Recipient returns the default destination: the configured To, else the
// address embedded in From, else the built-in no-reply address.
func (m *Mailer) Recipient() string {
	if m == nil {
		return ""
	}
	if strings.TrimSpace(m.cfg.To) != "" {
		return strings.TrimSpace(m.cfg.To)
	}
	if strings.TrimSpace(m.cfg.From) != "" {
		return addressOnly(m.cfg.From)
	}
	return defaultFromAddress
}

// Send delivers msg synchronously, retrying exactly once after a failure. When
// SMTP is not configured it logs a warning and reports success so callers do
// not treat the degenerate deployment as an error.
func (m *Mailer) Send(ctx context.Context, msg Message) error {
	if !m.Enabled() {
		m.logger().Warn("SMTP 未配置，跳过邮件发送", "subject", msg.Subject)
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var lastErr error
	for attempt := 1; attempt <= sendAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if lastErr = m.sendOnce(ctx, msg); lastErr == nil {
			m.logger().Info("邮件发送成功", "subject", msg.Subject, "recipient", m.Recipient())
			return nil
		}
		m.logger().Error("邮件发送失败", "attempt", attempt, "subject", msg.Subject, "error", lastErr)
	}
	return lastErr
}

func (m *Mailer) sendOnce(ctx context.Context, msg Message) error {
	client, err := m.connect(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	from := strings.TrimSpace(m.cfg.From)
	if from == "" {
		from = defaultFromHeader
	}

	if m.cfg.User != "" {
		if err := client.Auth(smtp.PlainAuth("", m.cfg.User, m.cfg.Pass, m.cfg.Host)); err != nil {
			return fmt.Errorf("SMTP 认证失败: %w", err)
		}
	}
	if err := client.Mail(addressOnly(from)); err != nil {
		return fmt.Errorf("MAIL FROM: %w", err)
	}

	to := strings.TrimSpace(msg.To)
	if to == "" {
		to = m.Recipient()
	}
	if to == "" {
		return errors.New("缺少收件人地址")
	}
	if err := client.Rcpt(addressOnly(to)); err != nil {
		return fmt.Errorf("RCPT TO: %w", err)
	}

	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("DATA: %w", err)
	}
	if _, err := writer.Write([]byte(m.buildMessage(from, to, msg))); err != nil {
		_ = writer.Close()
		return fmt.Errorf("写入邮件正文: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("提交邮件: %w", err)
	}
	return client.Quit()
}

func (m *Mailer) connect(ctx context.Context) (*smtp.Client, error) {
	addr := net.JoinHostPort(m.cfg.Host, strconv.Itoa(m.port()))
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("连接 SMTP 服务器失败: %w", err)
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}

	host := m.cfg.Host
	if m.tlsMode() == "implicit" {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("TLS 握手失败: %w", err)
		}
		conn = tlsConn
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("SMTP 握手失败: %w", err)
	}
	if m.tlsMode() == "starttls" {
		if supported, _ := client.Extension("STARTTLS"); supported {
			if err := client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
				_ = client.Close()
				return nil, fmt.Errorf("STARTTLS 失败: %w", err)
			}
		}
	}
	return client, nil
}

func (m *Mailer) buildMessage(from, to string, msg Message) string {
	subject := strings.TrimSpace(msg.Subject)
	if subject == "" {
		subject = defaultSubject
	}

	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", sanitizeHeader(from))
	fmt.Fprintf(&b, "To: %s\r\n", sanitizeHeader(to))
	fmt.Fprintf(&b, "Subject: %s\r\n", sanitizeHeader(subject))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(toCRLF(msg.Body))
	return b.String()
}

func (m *Mailer) tlsMode() string {
	switch strings.ToLower(strings.TrimSpace(m.cfg.TLS)) {
	case "implicit":
		return "implicit"
	case "none":
		return "none"
	default:
		return "starttls"
	}
}

func (m *Mailer) port() int {
	if m.cfg.Port > 0 && m.cfg.Port <= 65535 {
		return m.cfg.Port
	}
	return defaultSMTPPort
}

func (m *Mailer) logger() *slog.Logger {
	if m == nil || m.log == nil {
		return slog.Default()
	}
	return m.log
}

func addressOnly(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if addr, err := mail.ParseAddress(trimmed); err == nil {
		return addr.Address
	}
	return trimmed
}

func sanitizeHeader(value string) string {
	value = strings.ReplaceAll(value, "\r", "")
	value = strings.ReplaceAll(value, "\n", "")
	return strings.TrimSpace(value)
}

func toCRLF(body string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	body = strings.ReplaceAll(body, "\r", "\n")
	return strings.ReplaceAll(body, "\n", "\r\n")
}
