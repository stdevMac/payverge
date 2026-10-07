package main

import "strings"

// resolveGuardrailStrict decides the public-waiter guardrail failure posture.
// In production the default is fail-CLOSED (strict=true): when the guardrail
// model is down/rate-limited, borderline messages are redirected rather than
// forwarded unscreened to the answering model. The GUARDRAIL_STRICT env var
// (true/false) overrides in either direction. In dev the default stays
// fail-open to avoid blocking local testing.
func resolveGuardrailStrict(env string, production bool) bool {
	switch strings.ToLower(strings.TrimSpace(env)) {
	case "true":
		return true
	case "false":
		return false
	default:
		return production
	}
}
