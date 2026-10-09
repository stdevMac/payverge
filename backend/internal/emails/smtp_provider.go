package emails

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SMTP environment contract (see docs/self-hosting/email.md).
const (
	SMTPHostEnv     = "SMTP_HOST"
	SMTPPortEnv     = "SMTP_PORT"
	SMTPUsernameEnv = "SMTP_USERNAME"
	SMTPPasswordEnv = "SMTP_PASSWORD"
	SMTPFromEnv     = "SMTP_FROM"
	// SMTPTLSEnv optionally overrides the port-derived transport security:
	// "starttls" (require STARTTLS), "tls" (implicit TLS), "none" (never
	// upgrade; for a trusted local relay without a valid certificate) or
	// "auto"/empty (465 => tls, anything else => required STARTTLS, except a
	// loopback host, which upgrades only when offered).
	SMTPTLSEnv = "SMTP_TLS"

	defaultSMTPPort    = 587
	defaultSMTPTimeout = 30 * time.Second
)

// ErrSMTPOutcomeUnknown marks a send whose message was written in full but
// whose final reply to the end-of-data marker never arrived (connection
// dropped, timeout, cancellation). The relay may already have queued it and
// SMTP has no idempotency key, so the outbox must not retry it: a retry is a
// duplicate receipt or invite. classifyEmailFailure reports it as
// "outcome_unknown", which is not retryable.
var ErrSMTPOutcomeUnknown = errors.New("smtp: delivery outcome unknown after end of data")

// SMTPSecurity is the transport-security policy for an SMTP connection.
type SMTPSecurity string

const (
	// SMTPSecurityAuto derives the policy from the port and host.
	SMTPSecurityAuto SMTPSecurity = "auto"
	// SMTPSecurityStartTLS requires a STARTTLS upgrade (submission, 587).
	SMTPSecurityStartTLS SMTPSecurity = "starttls"
	// SMTPSecurityTLS dials TLS directly (SMTPS, 465).
	SMTPSecurityTLS SMTPSecurity = "tls"
	// SMTPSecurityOpportunistic upgrades with STARTTLS only when offered.
	SMTPSecurityOpportunistic SMTPSecurity = "opportunistic"
	// SMTPSecurityNone never upgrades. net/smtp still refuses to send
	// credentials over an unencrypted connection to a non-loopback host.
	SMTPSecurityNone SMTPSecurity = "none"
)

// SMTPConfig configures SMTPProvider.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	// From, when set (SMTP_FROM), replaces EmailMessage.From in both the
	// From header and the envelope sender. Relays such as Gmail/SES only
	// accept senders they have verified.
	From     string
	Security SMTPSecurity
	// Timeout bounds one send when the context has no deadline.
	Timeout time.Duration
	// HelloName is the EHLO name; empty uses net/smtp's "localhost".
	HelloName string

	// tlsConfig overrides the TLS client config (tests trust a local CA).
	tlsConfig *tls.Config
}

// SMTPConfigFromEnv reads SMTP_HOST, SMTP_PORT (default 587), SMTP_USERNAME,
// SMTP_PASSWORD, SMTP_FROM and SMTP_TLS.
func SMTPConfigFromEnv() (SMTPConfig, error) {
	cfg := SMTPConfig{
		Host:     strings.TrimSpace(os.Getenv(SMTPHostEnv)),
		Port:     defaultSMTPPort,
		Username: strings.TrimSpace(os.Getenv(SMTPUsernameEnv)),
		// Passwords may legitimately contain leading/trailing spaces.
		Password: os.Getenv(SMTPPasswordEnv),
		From:     strings.TrimSpace(os.Getenv(SMTPFromEnv)),
	}
	if raw := strings.TrimSpace(os.Getenv(SMTPPortEnv)); raw != "" {
		port, err := strconv.Atoi(raw)
		if err != nil || port <= 0 || port > 65535 {
			return SMTPConfig{}, fmt.Errorf("invalid %s %q: want a TCP port number", SMTPPortEnv, raw)
		}
		cfg.Port = port
	}
	switch v := SMTPSecurity(strings.ToLower(strings.TrimSpace(os.Getenv(SMTPTLSEnv)))); v {
	case "", SMTPSecurityAuto:
		cfg.Security = SMTPSecurityAuto
	case SMTPSecurityStartTLS, SMTPSecurityTLS, SMTPSecurityNone, SMTPSecurityOpportunistic:
		cfg.Security = v
	default:
		return SMTPConfig{}, fmt.Errorf("invalid %s %q: want auto|starttls|tls|none", SMTPTLSEnv, v)
	}
	return cfg, nil
}

// effectiveSecurity resolves "auto" against the port and host. Port 465 is
// implicit TLS. Every other port requires STARTTLS: an opportunistic upgrade
// to a remote relay (2525, 25, ...) can be stripped by anyone on the path,
// exposing the verification and password-reset links in the body. Only a
// loopback relay, which never crosses a network, upgrades opportunistically;
// a trusted relay without TLS on a private network needs SMTP_TLS=none.
func (c SMTPConfig) effectiveSecurity() SMTPSecurity {
	switch c.Security {
	case SMTPSecurityStartTLS, SMTPSecurityTLS, SMTPSecurityNone, SMTPSecurityOpportunistic:
		return c.Security
	}
	switch {
	case c.Port == 465:
		return SMTPSecurityTLS
	case c.Port != 587 && isLoopbackHost(c.Host):
		return SMTPSecurityOpportunistic
	default:
		return SMTPSecurityStartTLS
	}
}

// SMTPProvider sends mail through any SMTP relay with net/smtp.
type SMTPProvider struct {
	cfg SMTPConfig
}

// NewSMTPProvider validates cfg and builds the provider. Only Host is
// mandatory; Port defaults to 587.
func NewSMTPProvider(cfg SMTPConfig) (*SMTPProvider, error) {
	cfg.Host = strings.TrimSpace(cfg.Host)
	if cfg.Host == "" {
		return nil, fmt.Errorf("smtp email provider requires %s", SMTPHostEnv)
	}
	if strings.ContainsAny(cfg.Host, "\r\n/ ") {
		return nil, fmt.Errorf("invalid %s %q", SMTPHostEnv, cfg.Host)
	}
	if cfg.Port == 0 {
		cfg.Port = defaultSMTPPort
	}
	if cfg.Port < 0 || cfg.Port > 65535 {
		return nil, fmt.Errorf("invalid %s %d", SMTPPortEnv, cfg.Port)
	}
	if cfg.Security == "" {
		cfg.Security = SMTPSecurityAuto
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultSMTPTimeout
	}
	if cfg.From != "" {
		if _, err := mail.ParseAddress(cfg.From); err != nil {
			return nil, fmt.Errorf("invalid %s %q: %w", SMTPFromEnv, cfg.From, err)
		}
	}
	if cfg.Password != "" && cfg.Username == "" {
		return nil, fmt.Errorf("%s is set but %s is empty", SMTPPasswordEnv, SMTPUsernameEnv)
	}
	return &SMTPProvider{cfg: cfg}, nil
}

func (p *SMTPProvider) Send(ctx context.Context, msg EmailMessage) error {
	_, err := p.SendWithReceipt(ctx, msg)
	return err
}

// SendWithReceipt delivers msg in one SMTP transaction and returns the
// Message-ID (without angle brackets) as the provider message ID. The
// Message-ID derives from msg.IdempotencyKey, so an outbox retry of the same
// logical send carries the same Message-ID (receivers that deduplicate on it
// collapse a duplicate).
func (p *SMTPProvider) SendWithReceipt(ctx context.Context, msg EmailMessage) (SendResult, error) {
	fromRaw := p.cfg.From
	if fromRaw == "" {
		fromRaw = msg.From
	}
	from, err := mail.ParseAddress(sanitizeHeaderValue(fromRaw))
	if err != nil {
		return SendResult{}, fmt.Errorf("smtp: invalid sender %q: %w", fromRaw, err)
	}
	recipients := make([]*mail.Address, 0, len(msg.To))
	for _, raw := range msg.To {
		if strings.ContainsAny(raw, "\r\n") {
			return SendResult{}, errors.New("smtp: recipient contains a line break")
		}
		addr, err := mail.ParseAddress(raw)
		if err != nil {
			return SendResult{}, fmt.Errorf("smtp: invalid recipient: %w", err)
		}
		recipients = append(recipients, addr)
	}
	if len(recipients) == 0 {
		return SendResult{}, errors.New("smtp: no recipients")
	}

	messageID := smtpMessageID(msg.IdempotencyKey, from.Address)
	raw, err := buildSMTPMessage(msg, from, recipients, messageID, time.Now())
	if err != nil {
		return SendResult{}, err
	}

	if err := p.deliver(ctx, from.Address, recipients, raw); err != nil {
		return SendResult{}, err
	}
	return SendResult{ProviderMessageID: strings.Trim(messageID, "<>")}, nil
}

func (p *SMTPProvider) tlsConfig() *tls.Config {
	if p.cfg.tlsConfig != nil {
		cfg := p.cfg.tlsConfig.Clone()
		if cfg.ServerName == "" {
			cfg.ServerName = p.cfg.Host
		}
		return cfg
	}
	return &tls.Config{ServerName: p.cfg.Host, MinVersion: tls.VersionTLS12}
}

func (p *SMTPProvider) deliver(ctx context.Context, envelopeFrom string, recipients []*mail.Address, raw []byte) (err error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(p.cfg.Timeout)
	}
	addr := net.JoinHostPort(p.cfg.Host, strconv.Itoa(p.cfg.Port))
	security := p.cfg.effectiveSecurity()

	dialer := &net.Dialer{Deadline: deadline}
	var conn net.Conn
	if security == SMTPSecurityTLS {
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: p.tlsConfig()}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("smtp: dial %s: %w", addr, err)
	}
	_ = conn.SetDeadline(deadline)
	// Abort a blocked read/write promptly when the caller cancels.
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	client, err := smtp.NewClient(conn, p.cfg.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("smtp: greeting from %s: %w", addr, err)
	}
	defer func() { _ = client.Close() }()

	if p.cfg.HelloName != "" {
		if err := client.Hello(p.cfg.HelloName); err != nil {
			return fmt.Errorf("smtp: EHLO: %w", err)
		}
	}

	if security == SMTPSecurityStartTLS || security == SMTPSecurityOpportunistic {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(p.tlsConfig()); err != nil {
				return fmt.Errorf("smtp: STARTTLS: %w", err)
			}
		} else if security == SMTPSecurityStartTLS {
			return fmt.Errorf("smtp: %s does not offer STARTTLS, which is required for a non-loopback relay on port %d (set %s=none only for a trusted relay on a private network)", addr, p.cfg.Port, SMTPTLSEnv)
		}
	}

	if p.cfg.Username != "" {
		auth, err := p.pickAuth(client)
		if err != nil {
			return err
		}
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("smtp: AUTH: %w", err)
		}
	}

	if err := client.Mail(envelopeFrom); err != nil {
		return fmt.Errorf("smtp: MAIL FROM: %w", err)
	}
	for _, rcpt := range recipients {
		if err := client.Rcpt(rcpt.Address); err != nil {
			return fmt.Errorf("smtp: RCPT TO: %w", err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp: DATA: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		_ = w.Close()
		return fmt.Errorf("smtp: write message: %w", err)
	}
	if err := w.Close(); err != nil {
		// A reply code is a definite answer (4xx retry, 5xx refuse). Anything
		// else means the full message went out and the answer was lost.
		var reply *textproto.Error
		if errors.As(err, &reply) {
			return fmt.Errorf("smtp: end of data: %w", err)
		}
		return fmt.Errorf("%w (the relay may have accepted it; not retried to avoid a duplicate): %v", ErrSMTPOutcomeUnknown, err)
	}
	// The message is accepted once DATA completes; a failed QUIT must not
	// turn a delivered email into a retry (duplicate send).
	_ = client.Quit()
	return nil
}

// pickAuth chooses PLAIN, then LOGIN, then CRAM-MD5 from the advertised
// mechanisms. PLAIN and LOGIN refuse to run over an unencrypted connection
// unless the server is loopback (net/smtp's rule).
func (p *SMTPProvider) pickAuth(client *smtp.Client) (smtp.Auth, error) {
	ok, mechs := client.Extension("AUTH")
	if !ok {
		return nil, fmt.Errorf("smtp: %s is set but the server does not advertise AUTH (is STARTTLS required first?)", SMTPUsernameEnv)
	}
	offered := map[string]bool{}
	for _, m := range strings.Fields(strings.ToUpper(mechs)) {
		offered[m] = true
	}
	switch {
	case offered["PLAIN"]:
		return smtp.PlainAuth("", p.cfg.Username, p.cfg.Password, p.cfg.Host), nil
	case offered["LOGIN"]:
		return &loginAuth{username: p.cfg.Username, password: p.cfg.Password, host: p.cfg.Host}, nil
	case offered["CRAM-MD5"]:
		return smtp.CRAMMD5Auth(p.cfg.Username, p.cfg.Password), nil
	default:
		return nil, fmt.Errorf("smtp: no supported AUTH mechanism (server offers %q; want PLAIN, LOGIN or CRAM-MD5)", mechs)
	}
}

// loginAuth implements the non-standard but widespread AUTH LOGIN mechanism
// (Office 365, some cPanel hosts) with the same TLS guard as PlainAuth.
type loginAuth struct {
	username, password, host string
}

func (a *loginAuth) Start(server *smtp.ServerInfo) (string, []byte, error) {
	if !server.TLS && !isLoopbackHost(server.Name) {
		return "", nil, errors.New("unencrypted connection")
	}
	if server.Name != a.host {
		return "", nil, errors.New("wrong host name")
	}
	return "LOGIN", nil, nil
}

func (a *loginAuth) Next(fromServer []byte, more bool) ([]byte, error) {
	if !more {
		return nil, nil
	}
	prompt := strings.ToLower(strings.TrimSpace(string(fromServer)))
	switch {
	case strings.Contains(prompt, "username"):
		return []byte(a.username), nil
	case strings.Contains(prompt, "password"):
		return []byte(a.password), nil
	default:
		return nil, fmt.Errorf("unexpected AUTH LOGIN challenge %q", fromServer)
	}
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// reservedSMTPHeaders are owned by buildSMTPMessage; caller-supplied
// EmailMessage.Headers cannot override them.
var reservedSMTPHeaders = map[string]bool{
	"From": true, "To": true, "Cc": true, "Bcc": true, "Subject": true,
	"Date": true, "Message-Id": true, "Mime-Version": true, "Content-Type": true,
	"Content-Transfer-Encoding": true, "Reply-To": true, "Sender": true,
	"Return-Path": true,
}

// buildSMTPMessage renders msg as an RFC 5322 message with CRLF line endings:
// multipart/alternative (text + HTML) when there is HTML, wrapped in
// multipart/mixed when there are attachments.
func buildSMTPMessage(msg EmailMessage, from *mail.Address, to []*mail.Address, messageID string, now time.Time) ([]byte, error) {
	var hdr bytes.Buffer
	writeHeader := func(name, value string) {
		hdr.WriteString(name)
		hdr.WriteString(": ")
		hdr.WriteString(value)
		hdr.WriteString("\r\n")
	}

	writeHeader("From", from.String())
	toList := make([]string, len(to))
	for i, addr := range to {
		toList[i] = addr.String()
	}
	writeHeader("To", strings.Join(toList, ", "))
	if reply := strings.TrimSpace(msg.ReplyTo); reply != "" {
		if addr, err := mail.ParseAddress(sanitizeHeaderValue(reply)); err == nil {
			writeHeader("Reply-To", addr.String())
		}
	}
	writeHeader("Subject", mime.QEncoding.Encode("utf-8", sanitizeHeaderValue(msg.Subject)))
	writeHeader("Date", now.Format(time.RFC1123Z))
	writeHeader("Message-ID", messageID)
	writeHeader("MIME-Version", "1.0")
	if key := sanitizeHeaderValue(msg.IdempotencyKey); key != "" {
		writeHeader("X-Payverge-Idempotency-Key", key)
	}
	names := make([]string, 0, len(msg.Headers))
	for name := range msg.Headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		canonical := textproto.CanonicalMIMEHeaderKey(strings.TrimSpace(name))
		if !validHeaderName(canonical) || reservedSMTPHeaders[canonical] {
			continue
		}
		value := sanitizeHeaderValue(msg.Headers[name])
		if value == "" {
			continue
		}
		writeHeader(canonical, mime.QEncoding.Encode("utf-8", value))
	}

	contentType, cte, body, err := buildSMTPBody(msg)
	if err != nil {
		return nil, err
	}
	writeHeader("Content-Type", contentType)
	if cte != "" {
		writeHeader("Content-Transfer-Encoding", cte)
	}
	hdr.WriteString("\r\n")
	hdr.Write(body)
	return hdr.Bytes(), nil
}

// buildSMTPBody returns the top-level Content-Type, transfer encoding (empty
// for multipart) and encoded body.
func buildSMTPBody(msg EmailMessage) (string, string, []byte, error) {
	text := plainTextBody(msg)
	htmlBody := msg.HTMLBody

	var (
		contentType, cte string
		content          []byte
		err              error
	)
	if strings.TrimSpace(htmlBody) != "" {
		var buf bytes.Buffer
		alt := multipart.NewWriter(&buf)
		if err = writeQPPart(alt, "text/plain", text); err != nil {
			return "", "", nil, err
		}
		if err = writeQPPart(alt, "text/html", htmlBody); err != nil {
			return "", "", nil, err
		}
		if err = alt.Close(); err != nil {
			return "", "", nil, err
		}
		contentType = mime.FormatMediaType("multipart/alternative", map[string]string{"boundary": alt.Boundary()})
		content = buf.Bytes()
	} else {
		contentType = "text/plain; charset=utf-8"
		cte = "quoted-printable"
		if content, err = qpEncode(text); err != nil {
			return "", "", nil, err
		}
	}

	if len(msg.Attachments) == 0 {
		return contentType, cte, content, nil
	}

	var buf bytes.Buffer
	mixed := multipart.NewWriter(&buf)
	part := textproto.MIMEHeader{}
	part.Set("Content-Type", contentType)
	if cte != "" {
		part.Set("Content-Transfer-Encoding", cte)
	}
	w, err := mixed.CreatePart(part)
	if err != nil {
		return "", "", nil, err
	}
	if _, err := w.Write(content); err != nil {
		return "", "", nil, err
	}
	for _, att := range msg.Attachments {
		if err := writeAttachmentPart(mixed, att); err != nil {
			return "", "", nil, err
		}
	}
	if err := mixed.Close(); err != nil {
		return "", "", nil, err
	}
	return mime.FormatMediaType("multipart/mixed", map[string]string{"boundary": mixed.Boundary()}), "", buf.Bytes(), nil
}

func writeQPPart(w *multipart.Writer, mediaType, content string) error {
	h := textproto.MIMEHeader{}
	h.Set("Content-Type", mediaType+"; charset=utf-8")
	h.Set("Content-Transfer-Encoding", "quoted-printable")
	pw, err := w.CreatePart(h)
	if err != nil {
		return err
	}
	encoded, err := qpEncode(content)
	if err != nil {
		return err
	}
	_, err = pw.Write(encoded)
	return err
}

func qpEncode(content string) ([]byte, error) {
	var buf bytes.Buffer
	qp := quotedprintable.NewWriter(&buf)
	if _, err := qp.Write([]byte(content)); err != nil {
		return nil, err
	}
	if err := qp.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeAttachmentPart(w *multipart.Writer, att EmailAttachment) error {
	name := sanitizeHeaderValue(strings.ReplaceAll(att.Filename, `"`, ""))
	if name == "" {
		name = "attachment"
	}
	mediaType := strings.TrimSpace(sanitizeHeaderValue(att.ContentType))
	if _, _, err := mime.ParseMediaType(mediaType); err != nil || mediaType == "" {
		mediaType = "application/octet-stream"
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Type", mediaType)
	h.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	h.Set("Content-Transfer-Encoding", "base64")
	pw, err := w.CreatePart(h)
	if err != nil {
		return err
	}
	encoded := base64.StdEncoding.EncodeToString(att.Content)
	for len(encoded) > 76 {
		if _, err := pw.Write([]byte(encoded[:76] + "\r\n")); err != nil {
			return err
		}
		encoded = encoded[76:]
	}
	if encoded != "" {
		_, err = pw.Write([]byte(encoded + "\r\n"))
	}
	return err
}

// sanitizeHeaderValue strips CR/LF (header injection) and trims.
func sanitizeHeaderValue(v string) string {
	if strings.ContainsAny(v, "\r\n") {
		v = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(v)
	}
	return strings.TrimSpace(v)
}

// validHeaderName reports whether name is a legal RFC 5322 field name.
func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if r <= 32 || r >= 127 || r == ':' {
			return false
		}
	}
	return true
}

// newMessageID returns "<random@domain>" using the sender's domain.
func newMessageID(sender string) string {
	return "<" + randomHex(16) + "@" + messageIDDomain(sender) + ">"
}

// smtpMessageID returns a Message-ID that is stable for one idempotency key
// ("<pv-<sha256 prefix>@domain>"), or a random one when there is no key.
func smtpMessageID(idempotencyKey, sender string) string {
	key := strings.TrimSpace(idempotencyKey)
	if key == "" {
		return newMessageID(sender)
	}
	sum := sha256.Sum256([]byte(key))
	return "<pv-" + hex.EncodeToString(sum[:16]) + "@" + messageIDDomain(sender) + ">"
}

func messageIDDomain(sender string) string {
	if at := strings.LastIndex(sender, "@"); at >= 0 && at < len(sender)-1 {
		return sender[at+1:]
	}
	return "localhost"
}
