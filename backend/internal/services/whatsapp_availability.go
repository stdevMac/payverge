package services

// Build-tag-neutral answer to "does this process run the WhatsApp channel?".
// Compiled into every binary; it must NOT import go.mau.fi/* (GPL-3.0).

import (
	"os"
	"strings"
)

// WhatsAppEnabledEnv opts a process into the WhatsApp channel. It only takes
// effect in binaries built with `-tags whatsapp` (see WhatsAppBuilt).
const WhatsAppEnabledEnv = "WHATSAPP_ENABLED"

// whatsAppEnabledValue parses WHATSAPP_ENABLED: only "true" (case-insensitive,
// trimmed) opts in; everything else, including "1", "yes" and "", is off.
func whatsAppEnabledValue(env string) bool {
	return strings.EqualFold(strings.TrimSpace(env), "true")
}

// WhatsAppRequested reports whether the operator set WHATSAPP_ENABLED=true,
// regardless of whether this binary can honour it.
func WhatsAppRequested() bool {
	return whatsAppEnabledValue(os.Getenv(WhatsAppEnabledEnv))
}

// WhatsAppAvailable reports whether this process runs the WhatsApp channel:
// the binary was built with `-tags whatsapp` AND WHATSAPP_ENABLED=true. It
// gates manager start-up and the connect/disconnect routes, and is the value
// to publish as features.whatsapp.
func WhatsAppAvailable() bool {
	return WhatsAppBuilt && WhatsAppRequested()
}

// WhatsAppStartupMessage describes the WhatsApp channel's start-up state for
// the boot log. warn is true when the operator asked for it but the binary was
// compiled without the tag: silently ignoring WHATSAPP_ENABLED=true would look
// like a broken integration.
func WhatsAppStartupMessage() (msg string, warn bool) {
	return whatsAppStartupMessage(WhatsAppRequested(), WhatsAppBuilt)
}

func whatsAppStartupMessage(requested, built bool) (string, bool) {
	switch {
	case requested && built:
		return "WhatsApp integration enabled (WHATSAPP_ENABLED=true, whatsapp build tag)", false
	case requested:
		return "WHATSAPP_ENABLED=true is ignored: this binary was built without the `whatsapp` Go build tag, " +
			"so the WhatsApp channel is not compiled in. Rebuild the backend with `--build-arg GO_TAGS=whatsapp` " +
			"(links GPL-3.0 code; see docs/self-hosting/whatsapp.md) or unset WHATSAPP_ENABLED.", true
	case built:
		return "WhatsApp integration disabled (WHATSAPP_ENABLED != true; this binary includes the whatsapp tag)", false
	default:
		return "WhatsApp integration not compiled in (default build; see docs/self-hosting/whatsapp.md)", false
	}
}
