package sources

import (
	"strings"
	"testing"
)

func TestParseEmailMessagePrefersPlainTextAndSkipsAttachments(t *testing.T) {
	raw := strings.ReplaceAll(`From: Alice <alice@example.com>
To: Bob <bob@example.com>
Subject: Weekly plan
Message-ID: <msg-1@example.com>
Date: Mon, 08 Jun 2026 09:30:00 +0800
Content-Type: multipart/mixed; boundary="outer"

--outer
Content-Type: multipart/alternative; boundary="alt"

--alt
Content-Type: text/plain; charset=utf-8

Plain body line 1.
Plain body line 2.
--alt
Content-Type: text/html; charset=utf-8

<p>HTML body should not win.</p>
--alt--
--outer
Content-Type: application/pdf
Content-Disposition: attachment; filename="invoice.pdf"

attachment bytes
--outer--`, "\n", "\r\n")

	msg, err := ParseEmailMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("ParseEmailMessage() error = %v", err)
	}
	if msg.Subject != "Weekly plan" || !strings.Contains(msg.From, "alice@example.com") {
		t.Fatalf("unexpected headers: %#v", msg)
	}
	if !strings.Contains(msg.Body, "Plain body line 1") || strings.Contains(msg.Body, "attachment bytes") {
		t.Fatalf("unexpected body: %q", msg.Body)
	}
	if msg.Snippet == "" {
		t.Fatal("snippet should be populated")
	}
}

func TestParseEmailMessageUsesHTMLWhenPlainTextMissing(t *testing.T) {
	raw := strings.ReplaceAll(`From: News <news@example.com>
Subject: HTML only
Content-Type: text/html; charset=utf-8

<html><body><h1>Title</h1><p>Useful HTML text.</p></body></html>`, "\n", "\r\n")

	msg, err := ParseEmailMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("ParseEmailMessage() error = %v", err)
	}
	if !strings.Contains(msg.Body, "Useful HTML text.") {
		t.Fatalf("expected html fallback text, got %q", msg.Body)
	}
}
