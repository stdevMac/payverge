package config

import (
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
)

var ErrRedirectNotAllowed = errors.New("redirect URL is not allowed")

func allowedRedirectDomains() []string {
	raw := strings.TrimSpace(os.Getenv("ALLOWED_REDIRECT_DOMAINS"))
	if raw == "" {
		// Default: exactly the PUBLIC_URL host (never APP_BASE_URL)
		// plus APP_DOMAIN. No www./apex twin: list
		// it in ALLOWED_REDIRECT_DOMAINS when the instance serves both. The
		// development localhost fallback is deliberately not added —
		// http://localhost is allowed separately outside production.
		raw = trustedPublicHostname()
		if appDomain := strings.TrimSpace(os.Getenv("APP_DOMAIN")); appDomain != "" {
			if raw != "" {
				raw += ","
			}
			raw += appDomain
		}
	}

	parts := strings.Split(raw, ",")
	domains := make([]string, 0, len(parts))
	for _, part := range parts {
		domain := normalizeAllowedRedirectDomain(part)
		if domain != "" {
			domains = append(domains, domain)
		}
	}
	return domains
}

func normalizeAllowedRedirectDomain(raw string) string {
	domain := strings.ToLower(strings.TrimSpace(raw))
	if domain == "" {
		return ""
	}

	if ip := net.ParseIP(strings.Trim(domain, "[]")); ip != nil {
		return ip.String()
	}

	if parsed, err := url.Parse(domain); err == nil && parsed.Scheme != "" && parsed.Hostname() != "" {
		return normalizeRedirectHost(parsed.Hostname())
	}
	if parsed, err := url.Parse("//" + domain); err == nil && parsed.Hostname() != "" {
		return normalizeRedirectHost(parsed.Hostname())
	}

	return normalizeRedirectHost(domain)
}

func normalizeRedirectHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

func isLoopbackRedirectHost(host string) bool {
	normalized := normalizeRedirectHost(host)
	if normalized == "localhost" {
		return true
	}
	if ip := net.ParseIP(normalized); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func ValidateRedirectURL(raw string, production bool) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ErrRedirectNotAllowed
	}

	if strings.HasPrefix(trimmed, "/") {
		if strings.HasPrefix(trimmed, "//") || strings.Contains(trimmed, `\`) {
			return "", ErrRedirectNotAllowed
		}
		return trimmed, nil
	}

	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", ErrRedirectNotAllowed
	}

	host := normalizeRedirectHost(parsed.Hostname())
	if production && isLoopbackRedirectHost(host) {
		return "", ErrRedirectNotAllowed
	}

	switch parsed.Scheme {
	case "http":
		if !production && (host == "localhost" || host == "127.0.0.1") {
			return parsed.String(), nil
		}
	case "https":
		for _, domain := range allowedRedirectDomains() {
			if host == domain {
				return parsed.String(), nil
			}
		}
	}

	return "", ErrRedirectNotAllowed
}

func AbsoluteRedirectURL(raw string, production bool) (string, error) {
	validated, err := ValidateRedirectURL(raw, production)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(validated, "/") {
		return validated, nil
	}
	return ValidateRedirectURL(FrontendURL(validated), production)
}

func FrontendURL(path string) string {
	base := strings.TrimRight(FrontendBaseURL(), "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}
