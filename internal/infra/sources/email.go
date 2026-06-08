package sources

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-imap"
	"github.com/emersion/go-imap/client"
	messagemail "github.com/emersion/go-message/mail"
	"golang.org/x/net/html"
)

type EmailInboxConfig struct {
	Account        string
	Password       string
	Host           string
	Port           int
	Mailbox        string
	MaxMessages    int
	Since          time.Time
	UnreadOnly     bool
	ConnectTimeout time.Duration
}

type EmailMessage struct {
	ExternalID string
	MessageID  string
	UID        uint32
	Subject    string
	From       string
	To         string
	ReceivedAt time.Time
	Flags      []string
	Body       string
	Snippet    string
	URL        string
}

func FetchEmailInbox(ctx context.Context, cfg EmailInboxConfig) ([]EmailMessage, error) {
	cfg = normalizeEmailInboxConfig(cfg)
	if strings.TrimSpace(cfg.Account) == "" || cfg.Password == "" {
		return nil, fmt.Errorf("email inbox: account and authorization code are required")
	}
	if strings.TrimSpace(cfg.Host) == "" {
		return nil, fmt.Errorf("email inbox: imap host is required")
	}

	c, err := dialIMAP(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer c.Logout()

	if err := c.Login(cfg.Account, cfg.Password); err != nil {
		return nil, fmt.Errorf("email inbox: login failed: %w", err)
	}
	mbox, err := c.Select(cfg.Mailbox, true)
	if err != nil {
		return nil, fmt.Errorf("email inbox: select mailbox: %w", err)
	}
	if mbox.Messages == 0 {
		return []EmailMessage{}, nil
	}

	from := uint32(1)
	if mbox.Messages > uint32(cfg.MaxMessages) {
		from = mbox.Messages - uint32(cfg.MaxMessages) + 1
	}
	seqset := new(imap.SeqSet)
	seqset.AddRange(from, mbox.Messages)
	section := &imap.BodySectionName{Peek: true}
	items := []imap.FetchItem{imap.FetchEnvelope, imap.FetchUid, imap.FetchFlags, section.FetchItem()}
	ch := make(chan *imap.Message, cfg.MaxMessages)
	done := make(chan error, 1)
	go func() {
		done <- c.Fetch(seqset, items, ch)
	}()

	messages := make([]EmailMessage, 0, cfg.MaxMessages)
	for msg := range ch {
		if msg == nil {
			continue
		}
		if cfg.UnreadOnly && hasIMAPFlag(msg.Flags, imap.SeenFlag) {
			continue
		}
		body := msg.GetBody(section)
		if body == nil {
			continue
		}
		parsed, err := ParseEmailMessage(body)
		if err != nil {
			continue
		}
		if parsed.Subject == "" && msg.Envelope != nil {
			parsed.Subject = msg.Envelope.Subject
		}
		if parsed.ReceivedAt.IsZero() && msg.Envelope != nil {
			parsed.ReceivedAt = msg.Envelope.Date
		}
		if !cfg.Since.IsZero() && !parsed.ReceivedAt.IsZero() && parsed.ReceivedAt.Before(cfg.Since) {
			continue
		}
		parsed.UID = msg.Uid
		parsed.Flags = append([]string{}, msg.Flags...)
		parsed.ExternalID = parsed.MessageID
		if parsed.ExternalID == "" {
			parsed.ExternalID = fmt.Sprintf("%s:%s:%d", cfg.Account, cfg.Mailbox, msg.Uid)
		}
		parsed.URL = stableEmailURL(parsed.ExternalID)
		messages = append(messages, parsed)
	}
	if err := <-done; err != nil {
		return messages, fmt.Errorf("email inbox: fetch messages: %w", err)
	}
	sort.SliceStable(messages, func(i, j int) bool {
		return messages[i].ReceivedAt.After(messages[j].ReceivedAt)
	})
	return messages, nil
}

func ParseEmailMessage(r io.Reader) (EmailMessage, error) {
	mr, err := messagemail.CreateReader(r)
	if err != nil {
		return EmailMessage{}, fmt.Errorf("parse email: %w", err)
	}
	msg := EmailMessage{
		MessageID: strings.TrimSpace(mr.Header.Get("Message-ID")),
		From:      formatAddressList(mr.Header.AddressList("From")),
		To:        formatAddressList(mr.Header.AddressList("To")),
	}
	if subject, err := mr.Header.Subject(); err == nil {
		msg.Subject = strings.TrimSpace(subject)
	}
	if date, err := mr.Header.Date(); err == nil {
		msg.ReceivedAt = date
	}

	var plainParts []string
	var htmlParts []string
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		inline, ok := part.Header.(*messagemail.InlineHeader)
		if !ok {
			continue
		}
		contentType, _, _ := inline.ContentType()
		raw, _ := io.ReadAll(io.LimitReader(part.Body, 160*1024))
		switch strings.ToLower(strings.TrimSpace(contentType)) {
		case "text/plain", "":
			plainParts = append(plainParts, string(raw))
		case "text/html":
			htmlParts = append(htmlParts, htmlToText(string(raw)))
		}
	}
	if len(plainParts) > 0 {
		msg.Body = normalizeEmailText(strings.Join(plainParts, "\n\n"))
	} else {
		msg.Body = normalizeEmailText(strings.Join(htmlParts, "\n\n"))
	}
	msg.Snippet = truncateRunes(msg.Body, 220)
	return msg, nil
}

func normalizeEmailInboxConfig(cfg EmailInboxConfig) EmailInboxConfig {
	cfg.Account = strings.TrimSpace(cfg.Account)
	cfg.Host = strings.TrimSpace(cfg.Host)
	cfg.Mailbox = strings.TrimSpace(cfg.Mailbox)
	if cfg.Mailbox == "" {
		cfg.Mailbox = "INBOX"
	}
	if cfg.Port <= 0 {
		cfg.Port = 993
	}
	if cfg.MaxMessages <= 0 {
		cfg.MaxMessages = 20
	}
	if cfg.MaxMessages > 100 {
		cfg.MaxMessages = 100
	}
	if cfg.ConnectTimeout <= 0 {
		cfg.ConnectTimeout = 15 * time.Second
	}
	return cfg
}

func dialIMAP(ctx context.Context, cfg EmailInboxConfig) (*client.Client, error) {
	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", cfg.Port))
	dialer := tls.Dialer{
		NetDialer: &net.Dialer{Timeout: cfg.ConnectTimeout},
		Config:    &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12},
	}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("email inbox: connect imap: %w", err)
	}
	c, err := client.New(conn)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("email inbox: create client: %w", err)
	}
	return c, nil
}

func formatAddressList(addrs []*messagemail.Address, err error) string {
	if err != nil || len(addrs) == 0 {
		return ""
	}
	values := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		if addr == nil {
			continue
		}
		values = append(values, addr.String())
	}
	return strings.Join(values, ", ")
}

func htmlToText(raw string) string {
	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return raw
	}
	var parts []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				parts = append(parts, text)
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return strings.Join(parts, " ")
}

func normalizeEmailText(value string) string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if !blank && len(out) > 0 {
				out = append(out, "")
			}
			blank = true
			continue
		}
		out = append(out, line)
		blank = false
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func hasIMAPFlag(flags []string, target string) bool {
	for _, flag := range flags {
		if strings.EqualFold(flag, target) {
			return true
		}
	}
	return false
}

func stableEmailURL(externalID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(externalID)))
	return "email://message/" + url.PathEscape(hex.EncodeToString(sum[:12]))
}

func truncateRunes(value string, maxRunes int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes])
}
