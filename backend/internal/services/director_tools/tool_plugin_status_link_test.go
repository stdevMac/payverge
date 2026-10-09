package director_tools

import (
	"regexp"
	"testing"
)

// canonicalDeepLink mirrors the prompt-mandated allowed deep-link shape
// (buildTabDeepLink in the service): /business/<id>/dashboard?tab=<allowed>.
var canonicalDeepLink = regexp.MustCompile(`^/business/\d+/dashboard\?tab=(overview|analytics|menu|tables|bills|kitchen|counter|crm|reservations|delivery|plugins|staff|subscriptions|settings|business-page|ai-waiter)$`)

// TestPluginDeepLink_CanonicalFormat guards against the tool emitting the old
// /business/<id>?tab=plugins shape (missing /dashboard), which the model would
// otherwise quote into its actions and which diverges from every other link.
func TestPluginDeepLink_CanonicalFormat(t *testing.T) {
	link := pluginDeepLink(42)
	if link != "/business/42/dashboard?tab=plugins" {
		t.Fatalf("pluginDeepLink = %q, want /business/42/dashboard?tab=plugins", link)
	}
	if !canonicalDeepLink.MatchString(link) {
		t.Fatalf("pluginDeepLink %q does not match the canonical deep-link format", link)
	}
}
