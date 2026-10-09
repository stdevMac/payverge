package config

import (
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// DefaultRPCURL is the public Base mainnet RPC used for USDC verification
// when RPC_URL is unset. A self-hosted instance needs no RPC account; an
// unreachable RPC only disables crypto settlement (startup warns).
const DefaultRPCURL = "https://mainnet.base.org"

// ProductionSettlementChainID is Base mainnet. Production deploys verify
// guest USDC settlement on this chain only: testnet USDC (Base Sepolia,
// 84532) is free to mint, so accepting it would settle real bills for
// nothing. Development and test builds may keep Sepolia.
const ProductionSettlementChainID int64 = 8453

// DefaultTrustedProxies is the TRUSTED_PROXIES value applied when the variable
// is unset: loopback only (coordinator decision D-1). The backend listens on
// every interface, so trusting all private ranges by default would let any
// LAN or container peer forge X-Forwarded-For (including 127.0.0.1) and
// dodge per-IP limits. A reverse proxy on another host or container network
// must be listed explicitly; the bundled compose files set TRUSTED_PROXIES
// for their own network.
const DefaultTrustedProxies = "127.0.0.0/8,::1"

// RPCURLOrDefault returns raw (trimmed) or DefaultRPCURL when it is empty.
func RPCURLOrDefault(raw string) string {
	if value := strings.TrimSpace(raw); value != "" {
		return value
	}
	return DefaultRPCURL
}

// DataDir is the writable directory for instance-local state that must
// survive restarts but must never be served over HTTP (for example the
// generated development PLUGIN_SECRET_KEY). Resolution: DATA_DIR, else
// ./data, which in the container image is /app/data (the backend_data
// volume, owned by the runtime user).
//
// It is deliberately not derived from STORAGE_DIR: STORAGE_DIR itself is
// served publicly by the local media handler, and its parent (e.g. /data for
// /data/storage) is neither a volume nor writable in the image, so a key
// written there would be lost on every restart.
func DataDir() string {
	if dir := strings.TrimSpace(os.Getenv("DATA_DIR")); dir != "" {
		return filepath.Clean(dir)
	}
	return "data"
}

// PathWithin reports whether path is dir itself or lies below it, after
// resolving both to absolute paths. Used to keep instance secrets out of the
// publicly served STORAGE_DIR.
func PathWithin(path, dir string) bool {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(dir) == "" {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absDir, absPath)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

// PublicOrigin validates a PUBLIC_URL value and returns its canonical origin
// (lowercase scheme://host[:port], no trailing slash). It accepts only an
// absolute origin: no path (a lone "/" is tolerated), query, fragment, or
// credentials. HTTPS is required except for loopback hosts, which keeps
// single-machine trials and CI on http://localhost working.
func PublicOrigin(raw string) (string, error) {
	origin, code, message := parsePublicOrigin(raw)
	if code != "" {
		return "", &PublicURLError{Code: code, Message: message}
	}
	return origin, nil
}

// PublicURLError is the typed PUBLIC_URL validation failure. Message never
// echoes the configured value.
type PublicURLError struct {
	Code    string
	Message string
}

func (e *PublicURLError) Error() string { return e.Message }

func parsePublicOrigin(raw string) (origin, code, message string) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", "public_url.missing", "PUBLIC_URL is required in production (the https origin users open, e.g. https://restaurant.example.com)"
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Hostname() == "" || parsed.Opaque != "" {
		return "", "public_url.invalid", "PUBLIC_URL must be an absolute origin such as https://restaurant.example.com"
	}
	if parsed.User != nil {
		return "", "public_url.invalid", "PUBLIC_URL must not contain credentials"
	}
	if (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.ForceQuery {
		return "", "public_url.invalid", "PUBLIC_URL must be an origin without a path, query, or fragment (the API is served at PUBLIC_URL/api/v1)"
	}
	scheme := strings.ToLower(parsed.Scheme)
	switch scheme {
	case "https":
	case "http":
		if !isLoopbackHost(parsed.Hostname()) {
			return "", "public_url.insecure", "PUBLIC_URL must use https in production (http is accepted only for localhost)"
		}
	default:
		return "", "public_url.invalid", "PUBLIC_URL must use the https scheme"
	}
	return scheme + "://" + strings.ToLower(parsed.Host), "", ""
}

func isLoopbackHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
