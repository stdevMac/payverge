package main

import (
	"net"
	"strings"
)

// warnDBSSLDisabled reports whether to warn about DB_SSLMODE=disable in
// production. A single-label host such as "postgres" or "localhost" is a
// compose service name or the loopback: the bundled stack keeps Postgres on
// an internal network with no route out, so TLS adds nothing there and the
// warning would fire on every stock install. Any dotted name or IP address
// (a managed or remote database) still warns.
func warnDBSSLDisabled(production bool, sslMode, host string) bool {
	if !production || !strings.EqualFold(strings.TrimSpace(sslMode), "disable") {
		return false
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return true
	}
	if ip := net.ParseIP(strings.Trim(host, "[]")); ip != nil {
		return !ip.IsLoopback()
	}
	return strings.Contains(host, ".")
}
