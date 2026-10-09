// Package httpclientx exposes one process-wide *http.Client with a sane
// connection-pool config plus a Do helper that threads a context and applies a
// per-call deadline. Outbound integrations (plugins, AFIP SOAP, webhooks)
// should use this instead of http.DefaultClient (which has no timeout).
package httpclientx

import (
	"context"
	"net"
	"net/http"
	"time"
)

// DefaultTimeout is the hard ceiling on any single request when a caller does
// not pass a shorter per-feature deadline.
const DefaultTimeout = 30 * time.Second

var shared = &http.Client{
	Timeout: DefaultTimeout,
	Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
}

// Shared returns the process-wide *http.Client. Reuse it; do not construct
// per-call clients (that leaks connections and skips the pool config).
func Shared() *http.Client { return shared }

// Do executes req under ctx with a per-feature deadline. When deadline > 0 a
// child context bounds the call; the returned cancel runs when the response
// body is closed. Pass deadline = 0 to fall back to the client Timeout only.
func Do(ctx context.Context, req *http.Request, deadline time.Duration) (*http.Response, error) {
	if deadline > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, deadline)
		resp, err := shared.Do(req.WithContext(ctx))
		if err != nil {
			cancel()
			return nil, err
		}
		resp.Body = &cancelOnClose{ReadCloser: resp.Body, cancel: cancel}
		return resp, nil
	}
	return shared.Do(req.WithContext(ctx))
}
