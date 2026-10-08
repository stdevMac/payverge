package emails

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sirupsen/logrus"
)

func newCapturingLogger() (*logrus.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	l := logrus.New()
	l.SetOutput(&buf)
	l.SetFormatter(&logrus.JSONFormatter{})
	l.SetLevel(logrus.InfoLevel)
	return l, &buf
}

func decodeLogEntries(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("log line is not JSON: %q", line)
		}
		out = append(out, entry)
	}
	return out
}

func TestLogProviderMetadataOmitsRecipientsAndLinks(t *testing.T) {
	l, buf := newCapturingLogger()
	p := NewLogProvider(l)

	msg := EmailMessage{
		From:         "Payverge <noreply@example.test>",
		To:           []string{"Owner@Example.TEST", " other@example.test "},
		ReplyTo:      "reply@example.test",
		Subject:      "Reset your password",
		TextBody:     "open https://app.example.test/reset?token=abc",
		TemplateName: "password_reset",
		Attachments:  []EmailAttachment{{Filename: "a.pdf", Content: []byte("x")}},
	}
	if _, err := p.SendWithReceipt(context.Background(), msg); err != nil {
		t.Fatal(err)
	}

	raw := buf.String()
	if strings.Contains(raw, "@") || strings.Contains(raw, "http") {
		t.Fatalf("metadata log leaked content:\n%s", raw)
	}
	e := decodeLogEntries(t, buf)[0]
	if e["recipient_count"] != float64(2) {
		t.Errorf("recipient_count=%v", e["recipient_count"])
	}
	sum := sha256.Sum256([]byte("other@example.test,owner@example.test"))
	wantHash := hex.EncodeToString(sum[:])[:16]
	if e["recipient_hash"] != wantHash {
		t.Errorf("recipient_hash=%v want %s", e["recipient_hash"], wantHash)
	}
	if e["template"] != "password_reset" || e["attachment_count"] != float64(1) {
		t.Errorf("metadata fields: %+v", e)
	}
	for _, key := range []string{"to", "from", "subject", "reply_to", "body_preview", "links"} {
		if _, ok := e[key]; ok {
			t.Errorf("metadata mode logged %s: %+v", key, e)
		}
	}
	if msgText, _ := e["msg"].(string); msgText != "email not sent (EMAIL_PROVIDER=log): metadata logged instead of delivered" {
		t.Errorf("msg=%v", e["msg"])
	}
}

func TestLogContentEnabled(t *testing.T) {
	t.Setenv("EMAIL_LOG_CONTENT", "")
	if logContentEnabled() {
		t.Fatal("empty EMAIL_LOG_CONTENT must be false")
	}
	t.Setenv("EMAIL_LOG_CONTENT", "not-a-bool")
	if logContentEnabled() {
		t.Fatal("invalid EMAIL_LOG_CONTENT must be false")
	}
	t.Setenv("EMAIL_LOG_CONTENT", "0")
	if logContentEnabled() {
		t.Fatal("EMAIL_LOG_CONTENT=0 must be false")
	}
	t.Setenv("EMAIL_LOG_CONTENT", "true")
	if !logContentEnabled() {
		t.Fatal("EMAIL_LOG_CONTENT=true must be true")
	}
}

func TestLogProviderLogsRecipientSubjectPreviewAndLinks(t *testing.T) {
	l, buf := newCapturingLogger()
	p := NewLogProviderWithContent(l, true)

	msg := sampleMessage()
	msg.TemplateName = "email_verification"
	msg.Attachments = []EmailAttachment{{Filename: "a.pdf", Content: []byte("x")}}
	res, err := p.SendWithReceipt(context.Background(), msg)
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if !strings.HasPrefix(res.ProviderMessageID, "log-") {
		t.Fatalf("provider message id=%q", res.ProviderMessageID)
	}

	entries := decodeLogEntries(t, buf)
	if len(entries) != 1 {
		t.Fatalf("want exactly one log entry per email, got %d", len(entries))
	}
	e := entries[0]
	if e["level"] != "info" {
		t.Errorf("level=%v (must stay below the Sentry error hook)", e["level"])
	}
	if e["to"] != "owner@example.test" || e["subject"] != msg.Subject || e["template"] != "email_verification" {
		t.Errorf("entry fields: %+v", e)
	}
	if e["provider"] != "log" || e["provider_message_id"] != res.ProviderMessageID {
		t.Errorf("provider fields: %+v", e)
	}
	preview, _ := e["body_preview"].(string)
	if !strings.Contains(preview, "Hello & welcome") || strings.Contains(preview, "<p>") || strings.Contains(preview, "color:red") {
		t.Errorf("body_preview=%q want readable text without markup/styles", preview)
	}
	if links, _ := e["links"].(string); links != "https://app.example.test/verify-email?token=abc&x=1" {
		t.Errorf("links=%q want the unescaped verification link", links)
	}
	if e["attachment_count"] != float64(1) {
		t.Errorf("attachment_count=%v", e["attachment_count"])
	}
}

func TestLogProviderTruncatesLongBodies(t *testing.T) {
	l, buf := newCapturingLogger()
	msg := EmailMessage{
		From: "noreply@example.test", To: []string{"a@example.test", "b@example.test"},
		Subject: "big", TextBody: strings.Repeat("é", 5000) + " https://example.test/tail",
	}
	if err := NewLogProviderWithContent(l, true).Send(context.Background(), msg); err != nil {
		t.Fatal(err)
	}
	e := decodeLogEntries(t, buf)[0]
	preview := e["body_preview"].(string)
	if n := len([]rune(preview)); n > logProviderBodyPreviewRunes+len([]rune("…[truncated]")) {
		t.Fatalf("preview has %d runes, cap is %d", n, logProviderBodyPreviewRunes)
	}
	if !strings.HasSuffix(preview, "…[truncated]") {
		t.Errorf("truncation marker missing")
	}
	if e["links"] != "https://example.test/tail" {
		t.Errorf("links beyond the preview must still be extracted, got %v", e["links"])
	}
	if e["to"] != "a@example.test, b@example.test" {
		t.Errorf("to=%v", e["to"])
	}
}

func TestLogProviderHonoursCancelledContext(t *testing.T) {
	l, buf := newCapturingLogger()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := NewLogProvider(l).Send(ctx, sampleMessage()); err == nil {
		t.Fatal("cancelled context must fail like a real transport")
	}
	if buf.Len() != 0 {
		t.Fatal("nothing should be logged for a cancelled send")
	}
}

// TestLogProviderNeverTouchesTheNetwork pins the core OSS guarantee: a box
// with no email configuration never calls a third-party API. The default
// HTTP transport is swapped for one that counts requests.
func TestLogProviderNeverTouchesTheNetwork(t *testing.T) {
	var calls atomic.Int32
	prev := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("network disabled in test")
	})
	defer func() { http.DefaultTransport = prev }()

	provider, err := NewProvider("", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := provider.(*LogProvider); !ok {
		t.Fatalf("blank provider without key resolved to %T, want *LogProvider", provider)
	}
	l, _ := newCapturingLogger()
	provider.(*LogProvider).log = l
	for i := 0; i < 3; i++ {
		if err := provider.Send(context.Background(), sampleMessage()); err != nil {
			t.Fatal(err)
		}
	}
	if n := calls.Load(); n != 0 {
		t.Fatalf("log provider made %d HTTP calls", n)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestHTMLToPlainText(t *testing.T) {
	in := `<!DOCTYPE html><html><head><title>T</title><style>.x{}</style></head>
<body><!--[if mso]>MSO<![endif]--><h1>Welcome,&nbsp;Ana</h1>
<p>Line one<br>Line two</p>
<a href="https://a.test/x?y=1&amp;z=2">Open</a> <a href="https://a.test/same">https://a.test/same</a>
<a href="mailto:help@a.test">help@a.test</a><script>alert(1)</script></body></html>`
	got := htmlToPlainText(in)
	for _, want := range []string{"Welcome, Ana", "Line one\nLine two", "Open (https://a.test/x?y=1&z=2)", "https://a.test/same", "help@a.test"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, bad := range []string{"<", ".x{}", "alert", "MSO", "T\n"} {
		if strings.Contains(got, bad) {
			t.Errorf("unexpected %q in:\n%s", bad, got)
		}
	}
}
