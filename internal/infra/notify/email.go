package notify

import (
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

type EmailConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	UseTLS   bool
	Timeout  time.Duration
}

type Email struct {
	cfg EmailConfig
}

func NewEmailFromEnv() (*Email, error) {
	port := getenvInt("SMTP_PORT", 587)
	cfg := EmailConfig{
		Host:     strings.TrimSpace(os.Getenv("SMTP_HOST")),
		Port:     port,
		Username: strings.TrimSpace(os.Getenv("SMTP_USERNAME")),
		Password: os.Getenv("SMTP_PASSWORD"),
		From:     strings.TrimSpace(os.Getenv("SMTP_FROM")),
		UseTLS:   getenvBool("SMTP_USE_TLS", port == 465),
		Timeout:  20 * time.Second,
	}
	return NewEmail(cfg)
}

func NewEmail(cfg EmailConfig) (*Email, error) {
	cfg.Host = strings.TrimSpace(cfg.Host)
	cfg.From = strings.TrimSpace(cfg.From)
	if cfg.Host == "" {
		return nil, fmt.Errorf("notify: SMTP_HOST not set")
	}
	if cfg.Port <= 0 {
		return nil, fmt.Errorf("notify: invalid SMTP_PORT")
	}
	if cfg.From == "" {
		return nil, fmt.Errorf("notify: SMTP_FROM not set")
	}
	if _, err := mail.ParseAddress(cfg.From); err != nil {
		return nil, fmt.Errorf("notify: invalid SMTP_FROM")
	}
	if (cfg.Username == "") != (cfg.Password == "") {
		return nil, fmt.Errorf("notify: SMTP username and password must be set together")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 20 * time.Second
	}
	return &Email{cfg: cfg}, nil
}

func EmailConfiguredFromEnv() bool {
	return strings.TrimSpace(os.Getenv("SMTP_HOST")) != "" &&
		strings.TrimSpace(os.Getenv("SMTP_FROM")) != ""
}

func (e *Email) Send(ctx context.Context, recipients []string, subject, body string) error {
	if e == nil {
		return fmt.Errorf("notify: email sender not configured")
	}
	to, err := parseAddresses(recipients)
	if err != nil {
		return err
	}
	if len(to) == 0 {
		return fmt.Errorf("notify: no email recipients")
	}
	from, err := mail.ParseAddress(e.cfg.From)
	if err != nil {
		return fmt.Errorf("notify: invalid SMTP_FROM")
	}

	client, err := e.smtpClient(ctx)
	if err != nil {
		return err
	}
	defer client.Close()

	if e.cfg.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", e.cfg.Username, e.cfg.Password, e.cfg.Host)); err != nil {
			return fmt.Errorf("notify: smtp auth: %w", err)
		}
	}
	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("notify: smtp mail from: %w", err)
	}
	for _, recipient := range to {
		if err := client.Rcpt(recipient.Address); err != nil {
			return fmt.Errorf("notify: smtp recipient: %w", err)
		}
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("notify: smtp data: %w", err)
	}
	if _, err := writer.Write(composePlainEmail(from, to, subject, body)); err != nil {
		_ = writer.Close()
		return fmt.Errorf("notify: smtp write: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("notify: smtp close data: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("notify: smtp quit: %w", err)
	}
	return nil
}

func (e *Email) smtpClient(ctx context.Context) (*smtp.Client, error) {
	addr := net.JoinHostPort(e.cfg.Host, strconv.Itoa(e.cfg.Port))
	dialer := &net.Dialer{Timeout: e.cfg.Timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("notify: smtp connect: %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(e.cfg.Timeout))

	if e.cfg.UseTLS {
		tlsConn := tls.Client(conn, &tls.Config{ServerName: e.cfg.Host})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("notify: smtp tls: %w", err)
		}
		client, err := smtp.NewClient(tlsConn, e.cfg.Host)
		if err != nil {
			_ = tlsConn.Close()
			return nil, fmt.Errorf("notify: smtp client: %w", err)
		}
		return client, nil
	}

	client, err := smtp.NewClient(conn, e.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("notify: smtp client: %w", err)
	}
	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{ServerName: e.cfg.Host}); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("notify: smtp starttls: %w", err)
		}
	}
	return client, nil
}

func composePlainEmail(from *mail.Address, to []*mail.Address, subject, body string) []byte {
	toHeaders := make([]string, 0, len(to))
	for _, recipient := range to {
		toHeaders = append(toHeaders, sanitizeHeader(recipient.String()))
	}
	headers := []string{
		"From: " + sanitizeHeader(from.String()),
		"To: " + strings.Join(toHeaders, ", "),
		"Subject: " + mime.QEncoding.Encode("UTF-8", sanitizeHeader(subject)),
		"Date: " + time.Now().Format(time.RFC1123Z),
		"Content-Type: text/plain; charset=UTF-8",
		"MIME-Version: 1.0",
	}
	return []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + body)
}

func parseAddresses(values []string) ([]*mail.Address, error) {
	var out []*mail.Address
	for _, value := range values {
		for _, part := range splitEmailRecipients(value) {
			addr, err := mail.ParseAddress(part)
			if err != nil {
				return nil, fmt.Errorf("notify: invalid email recipient")
			}
			out = append(out, addr)
		}
	}
	return out, nil
}

func splitEmailRecipients(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func sanitizeHeader(value string) string {
	value = strings.ReplaceAll(value, "\r", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}

func getenvInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func getenvBool(key string, fallback bool) bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}
