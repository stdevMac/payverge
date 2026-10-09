package services

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrPushEndpointNotAllowed is returned for a Web Push endpoint that is not
// an https URL on a known browser push service.
var ErrPushEndpointNotAllowed = errors.New("push endpoint must be an https URL on a supported browser push service")

// maxPushEndpointLength bounds stored endpoint URLs (real ones are < 1 KB).
const maxPushEndpointLength = 2048

// pushServiceHosts / pushServiceHostSuffixes are the browser push services a
// PushSubscription.endpoint can legitimately point at (Chrome/Edge via FCM,
// Firefox autopush, Windows WNS, Safari/Apple). Anything else would turn the
// server-side notification sender into a blind SSRF primitive (M-push).
var (
	pushServiceHosts = []string{
		"fcm.googleapis.com",
		"updates.push.services.mozilla.com",
		"web.push.apple.com",
	}
	pushServiceHostSuffixes = []string{
		".notify.windows.com",
		".push.apple.com",
	}
)

// ValidatePushEndpoint reports whether raw is an acceptable Web Push
// endpoint: https, default port, no userinfo, and a host on the push-service
// allowlist. IP-literal hosts are always rejected.
func ValidatePushEndpoint(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxPushEndpointLength {
		return ErrPushEndpointNotAllowed
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.User != nil || u.Opaque != "" {
		return ErrPushEndpointNotAllowed
	}
	if port := u.Port(); port != "" && port != "443" {
		return ErrPushEndpointNotAllowed
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if host == "" || net.ParseIP(host) != nil {
		return ErrPushEndpointNotAllowed
	}
	for _, allowed := range pushServiceHosts {
		if host == allowed {
			return nil
		}
	}
	for _, suffix := range pushServiceHostSuffixes {
		if len(host) > len(suffix) && strings.HasSuffix(host, suffix) {
			return nil
		}
	}
	return ErrPushEndpointNotAllowed
}

// newPushHTTPClient returns the client used for push deliveries: bounded
// time and no redirect following, so an allowlisted host cannot bounce the
// request somewhere else.
func newPushHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
