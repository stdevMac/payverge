package emails

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSMTPServer is a minimal in-process ESMTP server (net.Listen on
// loopback) that records one transaction per connection. It supports EHLO,
// STARTTLS, implicit TLS, AUTH PLAIN/LOGIN, MAIL/RCPT/DATA and QUIT —
// enough to exercise SMTPProvider end to end without a network.
type fakeSMTPServer struct {
	t           *testing.T
	ln          net.Listener
	tlsConfig   *tls.Config
	implicitTLS bool
	offerTLS    bool   // advertise STARTTLS
	authMechs   string // e.g. "PLAIN LOGIN"; "" = no AUTH extension
	rejectRcpt  string // RCPT TO address answered with 550
	// dropAfterData records the message, then closes the connection
	// without answering the end-of-data marker (reply lost in transit).
	dropAfterData bool

	mu    sync.Mutex
	txns  []fakeSMTPTxn
	errCh chan error
	wg    sync.WaitGroup
}

type fakeSMTPTxn struct {
	tls      bool
	authUser string
	authPass string
	authMech string
	from     string
	rcpts    []string
	data     string
}

func newFakeSMTPServer(t *testing.T, configure func(*fakeSMTPServer)) *fakeSMTPServer {
	t.Helper()
	s := &fakeSMTPServer{t: t, errCh: make(chan error, 16)}
	if configure != nil {
		configure(s)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if s.implicitTLS {
		ln = tls.NewListener(ln, s.tlsConfig)
	}
	s.ln = ln
	s.wg.Add(1)
	go s.serve()
	t.Cleanup(func() {
		_ = ln.Close()
		s.wg.Wait()
		close(s.errCh)
		for err := range s.errCh {
			t.Errorf("fake smtp server: %v", err)
		}
	})
	return s
}

func (s *fakeSMTPServer) port() int {
	return s.ln.Addr().(*net.TCPAddr).Port
}

func (s *fakeSMTPServer) transactions() []fakeSMTPTxn {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]fakeSMTPTxn(nil), s.txns...)
}

func (s *fakeSMTPServer) serve() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer func() { _ = conn.Close() }()
			_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
			s.handle(conn)
		}()
	}
}

func (s *fakeSMTPServer) handle(conn net.Conn) {
	txn := fakeSMTPTxn{tls: s.implicitTLS}
	r := bufio.NewReader(conn)
	reply := func(line string) { _, _ = io.WriteString(conn, line+"\r\n") }
	readLine := func() (string, bool) {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", false
		}
		return strings.TrimRight(line, "\r\n"), true
	}

	reply("220 fake.test ESMTP ready")
	for {
		line, ok := readLine()
		if !ok {
			return
		}
		verb := strings.ToUpper(strings.SplitN(line, " ", 2)[0])
		arg := ""
		if i := strings.IndexByte(line, ' '); i >= 0 {
			arg = line[i+1:]
		}
		switch verb {
		case "EHLO", "HELO":
			exts := []string{"fake.test", "8BITMIME"}
			if s.offerTLS && !txn.tls {
				exts = append(exts, "STARTTLS")
			}
			if s.authMechs != "" {
				exts = append(exts, "AUTH "+s.authMechs)
			}
			for i, ext := range exts {
				sep := "-"
				if i == len(exts)-1 {
					sep = " "
				}
				reply("250" + sep + ext)
			}
		case "STARTTLS":
			reply("220 go ahead")
			tlsConn := tls.Server(conn, s.tlsConfig)
			if err := tlsConn.Handshake(); err != nil {
				return // client rejected our certificate
			}
			conn = tlsConn
			r = bufio.NewReader(conn)
			txn.tls = true
		case "AUTH":
			fields := strings.Fields(arg)
			txn.authMech = strings.ToUpper(fields[0])
			switch txn.authMech {
			case "PLAIN":
				payload := ""
				if len(fields) > 1 {
					payload = fields[1]
				} else {
					reply("334 ")
					payload, _ = readLine()
				}
				raw, err := base64.StdEncoding.DecodeString(payload)
				if err != nil {
					reply("501 bad base64")
					continue
				}
				parts := strings.Split(string(raw), "\x00")
				if len(parts) == 3 {
					txn.authUser, txn.authPass = parts[1], parts[2]
				}
			case "LOGIN":
				reply("334 " + base64.StdEncoding.EncodeToString([]byte("Username:")))
				u, _ := readLine()
				reply("334 " + base64.StdEncoding.EncodeToString([]byte("Password:")))
				p, _ := readLine()
				ub, _ := base64.StdEncoding.DecodeString(u)
				pb, _ := base64.StdEncoding.DecodeString(p)
				txn.authUser, txn.authPass = string(ub), string(pb)
			default:
				reply("504 unsupported")
				continue
			}
			reply("235 authenticated")
		case "MAIL":
			txn.from = angleAddr(arg)
			reply("250 ok")
		case "RCPT":
			rcpt := angleAddr(arg)
			if s.rejectRcpt != "" && strings.EqualFold(rcpt, s.rejectRcpt) {
				reply("550 5.1.1 no such user")
				continue
			}
			txn.rcpts = append(txn.rcpts, rcpt)
			reply("250 ok")
		case "DATA":
			reply("354 end with .")
			var b strings.Builder
			for {
				l, ok := readLine()
				if !ok {
					return
				}
				if l == "." {
					break
				}
				l = strings.TrimPrefix(l, ".") // undo dot-stuffing
				b.WriteString(l + "\r\n")
			}
			txn.data = b.String()
			s.mu.Lock()
			s.txns = append(s.txns, txn)
			s.mu.Unlock()
			if s.dropAfterData {
				return
			}
			reply("250 queued")
		case "RSET", "NOOP":
			reply("250 ok")
		case "QUIT":
			reply("221 bye")
			return
		default:
			reply("502 not implemented")
		}
	}
}

func angleAddr(arg string) string {
	start := strings.IndexByte(arg, '<')
	end := strings.IndexByte(arg, '>')
	if start < 0 || end < start {
		return arg
	}
	return arg[start+1 : end]
}

// testTLS returns a server config (httptest's cert, valid for 127.0.0.1)
// and a client config trusting it.
func testTLS(t *testing.T) (server *tls.Config, client *tls.Config) {
	t.Helper()
	ts := httptest.NewTLSServer(http.NotFoundHandler())
	certs := ts.TLS.Certificates
	pool := x509.NewCertPool()
	pool.AddCert(ts.Certificate())
	ts.Close()
	return &tls.Config{Certificates: certs, MinVersion: tls.VersionTLS12},
		&tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
}

func sampleMessage() EmailMessage {
	return EmailMessage{
		From:           "Payverge <noreply@example.test>",
		To:             []string{"owner@example.test"},
		Subject:        "Verify your email — Café",
		HTMLBody:       `<html><head><style>p{color:red}</style></head><body><p>Hello &amp; welcome</p><a href="https://app.example.test/verify-email?token=abc&amp;x=1">Verify</a></body></html>`,
		MessageType:    MessageTypeTransactional,
		IdempotencyKey: "verify:42",
		Headers:        map[string]string{"List-Unsubscribe": "<https://app.example.test/u>"},
	}
}

func newTestSMTPProvider(t *testing.T, cfg SMTPConfig) *SMTPProvider {
	t.Helper()
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}
	p, err := NewSMTPProvider(cfg)
	if err != nil {
		t.Fatalf("NewSMTPProvider: %v", err)
	}
	return p
}

func onlyTxn(t *testing.T, s *fakeSMTPServer) fakeSMTPTxn {
	t.Helper()
	txns := s.transactions()
	if len(txns) != 1 {
		t.Fatalf("server recorded %d transactions, want 1", len(txns))
	}
	return txns[0]
}

func TestSMTPProviderDeliversMultipartMessageOverPlainLoopback(t *testing.T) {
	srv := newFakeSMTPServer(t, nil)
	p := newTestSMTPProvider(t, SMTPConfig{Port: srv.port(), Security: SMTPSecurityNone})

	res, err := p.SendWithReceipt(context.Background(), sampleMessage())
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	txn := onlyTxn(t, srv)
	if txn.from != "noreply@example.test" {
		t.Errorf("envelope from=%q", txn.from)
	}
	if len(txn.rcpts) != 1 || txn.rcpts[0] != "owner@example.test" {
		t.Errorf("rcpts=%v", txn.rcpts)
	}
	if res.ProviderMessageID == "" || !strings.Contains(txn.data, "Message-ID: <"+res.ProviderMessageID+">") {
		t.Errorf("provider message id %q not the Message-ID header", res.ProviderMessageID)
	}

	parsed, err := mail.ReadMessage(strings.NewReader(txn.data))
	if err != nil {
		t.Fatalf("parse sent message: %v", err)
	}
	dec := new(mime.WordDecoder)
	subject, err := dec.DecodeHeader(parsed.Header.Get("Subject"))
	if err != nil || subject != "Verify your email — Café" {
		t.Errorf("subject=%q err=%v", subject, err)
	}
	if got := parsed.Header.Get("X-Payverge-Idempotency-Key"); got != "verify:42" {
		t.Errorf("idempotency header=%q", got)
	}
	if got := parsed.Header.Get("List-Unsubscribe"); got != "<https://app.example.test/u>" {
		t.Errorf("custom header=%q", got)
	}
	mediaType, params, err := mime.ParseMediaType(parsed.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/alternative" {
		t.Fatalf("content-type=%q err=%v", mediaType, err)
	}
	mr := multipart.NewReader(parsed.Body, params["boundary"])
	parts := map[string]string{}
	for {
		part, err := mr.NextPart() // decodes quoted-printable transparently
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("next part: %v", err)
		}
		body, _ := io.ReadAll(part)
		ct, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		parts[ct] = string(body)
	}
	if !strings.Contains(parts["text/html"], "<p>Hello &amp; welcome</p>") {
		t.Errorf("html part=%q", parts["text/html"])
	}
	text := parts["text/plain"]
	if !strings.Contains(text, "Hello & welcome") || !strings.Contains(text, "Verify (https://app.example.test/verify-email?token=abc&x=1)") {
		t.Errorf("derived text part=%q", text)
	}
	if strings.Contains(text, "color:red") {
		t.Errorf("text part leaked <style>: %q", text)
	}
}

func TestSMTPProviderSTARTTLSWithAuth(t *testing.T) {
	serverTLS, clientTLS := testTLS(t)
	for _, mech := range []string{"PLAIN", "LOGIN"} {
		t.Run(mech, func(t *testing.T) {
			srv := newFakeSMTPServer(t, func(s *fakeSMTPServer) {
				s.tlsConfig = serverTLS
				s.offerTLS = true
				s.authMechs = mech
			})
			p := newTestSMTPProvider(t, SMTPConfig{
				Port: srv.port(), Security: SMTPSecurityStartTLS,
				Username: "mailer", Password: "s3cret pass", tlsConfig: clientTLS,
			})
			if err := p.Send(context.Background(), sampleMessage()); err != nil {
				t.Fatalf("send: %v", err)
			}
			txn := onlyTxn(t, srv)
			if !txn.tls {
				t.Error("message was sent before STARTTLS")
			}
			if txn.authMech != mech || txn.authUser != "mailer" || txn.authPass != "s3cret pass" {
				t.Errorf("auth mech=%q user=%q pass=%q", txn.authMech, txn.authUser, txn.authPass)
			}
		})
	}
}

func TestSMTPProviderImplicitTLS(t *testing.T) {
	serverTLS, clientTLS := testTLS(t)
	srv := newFakeSMTPServer(t, func(s *fakeSMTPServer) {
		s.tlsConfig = serverTLS
		s.implicitTLS = true
		s.authMechs = "PLAIN"
	})
	p := newTestSMTPProvider(t, SMTPConfig{
		Port: srv.port(), Security: SMTPSecurityTLS,
		Username: "u", Password: "p", tlsConfig: clientTLS,
	})
	if err := p.Send(context.Background(), sampleMessage()); err != nil {
		t.Fatalf("send: %v", err)
	}
	if txn := onlyTxn(t, srv); !txn.tls || txn.authUser != "u" {
		t.Errorf("txn tls=%v user=%q", txn.tls, txn.authUser)
	}
}

func TestSMTPProviderRequiredSTARTTLSMissingFails(t *testing.T) {
	srv := newFakeSMTPServer(t, nil) // no STARTTLS offered
	p := newTestSMTPProvider(t, SMTPConfig{Port: srv.port(), Security: SMTPSecurityStartTLS})
	err := p.Send(context.Background(), sampleMessage())
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("want STARTTLS-required error, got %v", err)
	}
	if n := len(srv.transactions()); n != 0 {
		t.Fatalf("message must not be sent in cleartext, got %d txns", n)
	}
}

func TestSMTPProviderRejectsUntrustedCertificate(t *testing.T) {
	serverTLS, _ := testTLS(t)
	srv := newFakeSMTPServer(t, func(s *fakeSMTPServer) {
		s.tlsConfig = serverTLS
		s.offerTLS = true
	})
	// Default client config (system roots) must not trust the test cert.
	p := newTestSMTPProvider(t, SMTPConfig{Port: srv.port(), Security: SMTPSecurityStartTLS})
	if err := p.Send(context.Background(), sampleMessage()); err == nil {
		t.Fatal("expected certificate verification failure")
	}
	if n := len(srv.transactions()); n != 0 {
		t.Fatalf("message sent despite TLS failure (%d txns)", n)
	}
}

func TestSMTPProviderOpportunisticUpgradesWhenOffered(t *testing.T) {
	serverTLS, clientTLS := testTLS(t)
	srv := newFakeSMTPServer(t, func(s *fakeSMTPServer) {
		s.tlsConfig = serverTLS
		s.offerTLS = true
	})
	p := newTestSMTPProvider(t, SMTPConfig{Port: srv.port(), tlsConfig: clientTLS}) // auto on a non-587/465 port
	if err := p.Send(context.Background(), sampleMessage()); err != nil {
		t.Fatalf("send: %v", err)
	}
	if !onlyTxn(t, srv).tls {
		t.Error("opportunistic mode did not upgrade when STARTTLS was offered")
	}
}

func TestSMTPProviderSMTPFromOverridesSender(t *testing.T) {
	srv := newFakeSMTPServer(t, nil)
	p := newTestSMTPProvider(t, SMTPConfig{Port: srv.port(), Security: SMTPSecurityNone, From: "Relay Approved <mailer@relay.test>"})
	if err := p.Send(context.Background(), sampleMessage()); err != nil {
		t.Fatalf("send: %v", err)
	}
	txn := onlyTxn(t, srv)
	if txn.from != "mailer@relay.test" {
		t.Errorf("envelope from=%q want SMTP_FROM", txn.from)
	}
	parsed, _ := mail.ReadMessage(strings.NewReader(txn.data))
	if got := parsed.Header.Get("From"); !strings.Contains(got, "mailer@relay.test") {
		t.Errorf("From header=%q want SMTP_FROM", got)
	}
}

func TestSMTPProviderBlocksHeaderInjection(t *testing.T) {
	srv := newFakeSMTPServer(t, nil)
	p := newTestSMTPProvider(t, SMTPConfig{Port: srv.port(), Security: SMTPSecurityNone})
	msg := sampleMessage()
	msg.Subject = "Hi\r\nBcc: victim@example.test"
	msg.Headers = map[string]string{
		"X-Note":        "a\r\nBcc: victim2@example.test",
		"Bcc":           "victim3@example.test", // reserved, dropped
		"Bad Header:\r": "x",                    // invalid name, dropped
	}
	if err := p.Send(context.Background(), msg); err != nil {
		t.Fatalf("send: %v", err)
	}
	txn := onlyTxn(t, srv)
	head := txn.data[:strings.Index(txn.data, "\r\n\r\n")]
	for _, line := range strings.Split(head, "\r\n") {
		if strings.HasPrefix(strings.ToLower(line), "bcc:") {
			t.Fatalf("header injection produced %q", line)
		}
	}
	if len(txn.rcpts) != 1 {
		t.Fatalf("injected recipients reached RCPT: %v", txn.rcpts)
	}

	msg = sampleMessage()
	msg.To = []string{"owner@example.test\r\nRCPT TO:<x@example.test>"}
	if err := p.Send(context.Background(), msg); err == nil {
		t.Fatal("recipient with CRLF must be rejected")
	}
}

func TestSMTPProviderSurfacesRecipientRejection(t *testing.T) {
	srv := newFakeSMTPServer(t, func(s *fakeSMTPServer) { s.rejectRcpt = "owner@example.test" })
	p := newTestSMTPProvider(t, SMTPConfig{Port: srv.port(), Security: SMTPSecurityNone})
	err := p.Send(context.Background(), sampleMessage())
	if err == nil || !strings.Contains(err.Error(), "550") {
		t.Fatalf("want 550 RCPT error, got %v", err)
	}
	if n := len(srv.transactions()); n != 0 {
		t.Fatalf("rejected message must not be queued, got %d", n)
	}
	// A permanent refusal must not burn the retry budget.
	if got := classifyEmailFailure(err); got != "rejected" || isRetryableEmailFailure(err) {
		t.Fatalf("classification=%q retryable=%v, want rejected/non-retryable", got, isRetryableEmailFailure(err))
	}
}

func TestSMTPProviderAttachmentsUseMultipartMixed(t *testing.T) {
	srv := newFakeSMTPServer(t, nil)
	p := newTestSMTPProvider(t, SMTPConfig{Port: srv.port(), Security: SMTPSecurityNone})
	msg := sampleMessage()
	payload := []byte(strings.Repeat("receipt-bytes-", 20))
	msg.Attachments = []EmailAttachment{{Filename: "receipt.pdf", Content: payload, ContentType: "application/pdf"}}
	if err := p.Send(context.Background(), msg); err != nil {
		t.Fatalf("send: %v", err)
	}
	parsed, err := mail.ReadMessage(strings.NewReader(onlyTxn(t, srv).data))
	if err != nil {
		t.Fatal(err)
	}
	mediaType, params, _ := mime.ParseMediaType(parsed.Header.Get("Content-Type"))
	if mediaType != "multipart/mixed" {
		t.Fatalf("content-type=%q", mediaType)
	}
	mr := multipart.NewReader(parsed.Body, params["boundary"])
	var sawAlt, sawPDF bool
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		ct, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		switch ct {
		case "multipart/alternative":
			sawAlt = true
		case "application/pdf":
			sawPDF = true
			if part.FileName() != "receipt.pdf" {
				t.Errorf("filename=%q", part.FileName())
			}
			raw, _ := io.ReadAll(part)
			decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(string(raw), "\r\n", ""))
			if err != nil || string(decoded) != string(payload) {
				t.Errorf("attachment round-trip failed: %v", err)
			}
		}
	}
	if !sawAlt || !sawPDF {
		t.Fatalf("parts alt=%v pdf=%v", sawAlt, sawPDF)
	}
}

func TestSMTPProviderHonoursContextDeadline(t *testing.T) {
	// A server that accepts but never greets: the send must give up at the
	// context deadline instead of hanging the email goroutine.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			defer func() { _ = conn.Close() }()
			time.Sleep(2 * time.Second)
		}
	}()
	p := newTestSMTPProvider(t, SMTPConfig{Port: ln.Addr().(*net.TCPAddr).Port, Security: SMTPSecurityNone})
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := p.Send(ctx, sampleMessage()); err == nil {
		t.Fatal("expected timeout")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("send blocked %v past the deadline", elapsed)
	}
}

func TestSMTPConfigFromEnvAndPortSecurity(t *testing.T) {
	for _, key := range []string{SMTPHostEnv, SMTPPortEnv, SMTPUsernameEnv, SMTPPasswordEnv, SMTPFromEnv, SMTPTLSEnv} {
		t.Setenv(key, "")
	}
	t.Setenv(SMTPHostEnv, " mail.example.test ")
	cfg, err := SMTPConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Host != "mail.example.test" || cfg.Port != 587 || cfg.effectiveSecurity() != SMTPSecurityStartTLS {
		t.Fatalf("defaults: %+v security=%s", cfg, cfg.effectiveSecurity())
	}

	// EMAIL-4: a remote relay on a non-standard port must not fall back to
	// strippable opportunistic STARTTLS; only loopback may.
	cases := map[int]SMTPSecurity{465: SMTPSecurityTLS, 587: SMTPSecurityStartTLS, 25: SMTPSecurityStartTLS, 2525: SMTPSecurityStartTLS, 1025: SMTPSecurityStartTLS}
	for port, want := range cases {
		t.Setenv(SMTPPortEnv, strconv.Itoa(port))
		cfg, err := SMTPConfigFromEnv()
		if err != nil {
			t.Fatal(err)
		}
		if got := cfg.effectiveSecurity(); got != want {
			t.Errorf("port %d security=%s want %s", port, got, want)
		}
	}
	for _, host := range []string{"localhost", "127.0.0.1", "::1"} {
		t.Setenv(SMTPHostEnv, host)
		for port, want := range map[int]SMTPSecurity{465: SMTPSecurityTLS, 587: SMTPSecurityStartTLS, 25: SMTPSecurityOpportunistic, 1025: SMTPSecurityOpportunistic} {
			t.Setenv(SMTPPortEnv, strconv.Itoa(port))
			cfg, err := SMTPConfigFromEnv()
			if err != nil {
				t.Fatal(err)
			}
			if got := cfg.effectiveSecurity(); got != want {
				t.Errorf("%s:%d security=%s want %s", host, port, got, want)
			}
		}
	}
	t.Setenv(SMTPHostEnv, "mail.example.test")

	t.Setenv(SMTPPortEnv, "587")
	t.Setenv(SMTPTLSEnv, "none")
	if cfg, _ := SMTPConfigFromEnv(); cfg.effectiveSecurity() != SMTPSecurityNone {
		t.Errorf("SMTP_TLS=none not honoured: %s", cfg.effectiveSecurity())
	}
	t.Setenv(SMTPTLSEnv, "ssl3")
	if _, err := SMTPConfigFromEnv(); err == nil {
		t.Error("invalid SMTP_TLS must error")
	}
	t.Setenv(SMTPTLSEnv, "")
	t.Setenv(SMTPPortEnv, "smtp")
	if _, err := SMTPConfigFromEnv(); err == nil {
		t.Error("non-numeric SMTP_PORT must error")
	}
}

func TestNewSMTPProviderValidation(t *testing.T) {
	if _, err := NewSMTPProvider(SMTPConfig{}); err == nil {
		t.Error("missing host must error")
	}
	if _, err := NewSMTPProvider(SMTPConfig{Host: "h", From: "not an address"}); err == nil {
		t.Error("invalid SMTP_FROM must error")
	}
	if _, err := NewSMTPProvider(SMTPConfig{Host: "h", Password: "p"}); err == nil {
		t.Error("password without username must error")
	}
	if _, err := NewSMTPProvider(SMTPConfig{Host: "h", Port: 70000}); err == nil {
		t.Error("out-of-range port must error")
	}
}

func TestLoginAuthRefusesCleartextToRemoteHost(t *testing.T) {
	a := &loginAuth{username: "u", password: "p", host: "mail.example.test"}
	if _, _, err := a.Start(&smtp.ServerInfo{Name: "mail.example.test", TLS: false}); err == nil {
		t.Fatal("LOGIN over cleartext to a remote host must be refused")
	}
	if _, _, err := a.Start(&smtp.ServerInfo{Name: "mail.example.test", TLS: true}); err != nil {
		t.Fatalf("LOGIN over TLS: %v", err)
	}
	if _, _, err := a.Start(&smtp.ServerInfo{Name: "other.test", TLS: true}); err == nil {
		t.Fatal("host mismatch must be refused")
	}
}

// EMAIL-6: once the full message is written, a lost reply means the relay may
// have queued it. That must not be retried (SMTP has no idempotency key).
func TestSMTPProviderDoesNotRetryWhenTheFinalReplyIsLost(t *testing.T) {
	srv := newFakeSMTPServer(t, func(s *fakeSMTPServer) { s.dropAfterData = true })
	p := newTestSMTPProvider(t, SMTPConfig{Port: srv.port(), Security: SMTPSecurityNone})
	err := p.Send(context.Background(), sampleMessage())
	if !errors.Is(err, ErrSMTPOutcomeUnknown) {
		t.Fatalf("want ErrSMTPOutcomeUnknown, got %v", err)
	}
	if n := len(srv.transactions()); n != 1 {
		t.Fatalf("the relay received the message (%d txns)", n)
	}
	if got := classifyEmailFailure(err); got != "outcome_unknown" || isRetryableEmailFailure(err) {
		t.Fatalf("classification=%q retryable=%v, want outcome_unknown/non-retryable", got, isRetryableEmailFailure(err))
	}
}

func TestSMTPProviderMessageIDIsStablePerIdempotencyKey(t *testing.T) {
	srv := newFakeSMTPServer(t, nil)
	p := newTestSMTPProvider(t, SMTPConfig{Port: srv.port(), Security: SMTPSecurityNone})
	send := func(key string) string {
		t.Helper()
		msg := sampleMessage()
		msg.IdempotencyKey = key
		res, err := p.SendWithReceipt(context.Background(), msg)
		if err != nil {
			t.Fatalf("send: %v", err)
		}
		return res.ProviderMessageID
	}
	first, retry, other := send("pv-invite-42"), send("pv-invite-42"), send("pv-invite-43")
	if first != retry {
		t.Fatalf("retry of the same logical send changed Message-ID: %q vs %q", first, retry)
	}
	if first == other {
		t.Fatalf("different sends share a Message-ID %q", first)
	}
	if !strings.HasPrefix(first, "pv-") || !strings.Contains(first, "@") {
		t.Fatalf("unexpected Message-ID %q", first)
	}
	txns := srv.transactions()
	parsed, err := mail.ReadMessage(strings.NewReader(txns[0].data))
	if err != nil {
		t.Fatal(err)
	}
	if got := parsed.Header.Get("Message-Id"); got != "<"+first+">" {
		t.Fatalf("Message-ID header=%q want <%s>", got, first)
	}
	if send("") == send("") {
		t.Fatal("sends without a key must get distinct Message-IDs")
	}
}
