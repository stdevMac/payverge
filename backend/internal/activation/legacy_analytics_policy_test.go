package activation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyActivationAndPIIAnalyticsAreAbsent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		path      string
		forbidden []string
	}{
		{filepath.Join("..", "auth", "handlers.go"), []string{"metrics.TrackEvent("}},
		{filepath.Join("..", "server", "auth_handlers.go"), []string{"metrics.TrackEvent(", "metrics.TrackGeneralEvent("}},
		{filepath.Join("..", "server", "user_handlers.go"), []string{"metrics.TrackGeneralEvent("}},
		{filepath.Join("..", "server", "business_handlers.go"), []string{
			`"business_created"`, `"trial_activated"`, "metrics.IdentifyUser", "metrics.AliasUser",
		}},
		{filepath.Join("..", "server", "business_onboarding_handlers.go"), []string{`"onboarding_completed"`}},
		{filepath.Join("..", "services", "onboarding_stamp.go"), []string{`"onboarding_completed"`}},
		{filepath.Join("..", "services", "milestone_tracker.go"), []string{`"first_order_completed"`}},
		{filepath.Join("..", "metrics", "posthog.go"), []string{
			"func IdentifyUser(", "func AliasUser(", "func TrackGeneralEvent(",
		}},
		{filepath.Join("..", "..", "..", "frontend", "src", "app", "(shop)", "business", "register", "page.tsx"), []string{
			`trackConversion("registration_started"`, "useConversionTracking",
		}},
	}

	// The post-checkout success page is gone with subscription billing. Every
	// remaining source file under register/ must stay free of conversion
	// tracking, so a recreated success step cannot bring it back.
	registerDir := filepath.Join("..", "..", "..", "frontend", "src", "app", "(shop)", "business", "register")
	if err := filepath.WalkDir(registerDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !(strings.HasSuffix(path, ".tsx") || strings.HasSuffix(path, ".ts")) {
			return nil
		}
		if strings.Contains(path, "__tests__") {
			return nil
		}
		tests = append(tests, struct {
			path      string
			forbidden []string
		}{path, []string{"trackConversion", `conversion_type: "registration_completed"`}})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	for _, test := range tests {
		test := test
		t.Run(filepath.Base(test.path), func(t *testing.T) {
			source, err := os.ReadFile(test.path)
			if err != nil {
				t.Fatal(err)
			}
			for _, forbidden := range test.forbidden {
				if strings.Contains(string(source), forbidden) {
					t.Errorf("%s contains legacy analytics marker %q", test.path, forbidden)
				}
			}
		})
	}
}
