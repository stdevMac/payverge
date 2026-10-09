package emails

import (
	"strings"
	"testing"
)

// The day-3 setup nudge must carry a "verify your email" reminder when the
// owner's email-auth record is still unverified — an unverified owner is one
// expired session away from a login lockout (LoginWithEmail rejects
// unverified). The reminder must render in every email family and must be
// absent when verify_email is false.
func TestSetupNudgeVerifyReminderRendersPerFamily(t *testing.T) {
	tm, err := NewTemplateManager(realTemplatesRoot(t))
	if err != nil {
		t.Fatalf("new template manager: %v", err)
	}

	markers := map[string]string{
		"en":    "still unverified",
		"es":    "todavía no está verificada",
		"es_ar": "todavía no está verificada",
	}

	for lang, marker := range markers {
		t.Run(lang, func(t *testing.T) {
			base := map[string]interface{}{
				"owner_name":    "Alex",
				"missing_step":  "menu",
				"dashboard_url": "https://payverge.io/business/1/dashboard",
			}

			base["verify_email"] = true
			withReminder, err := tm.Render(lang, "setup_nudge", base)
			if err != nil {
				t.Fatalf("render with reminder: %v", err)
			}
			if !strings.Contains(withReminder, marker) {
				t.Fatalf("expected verify reminder marker %q in %s body:\n%s", marker, lang, withReminder)
			}

			base["verify_email"] = false
			withoutReminder, err := tm.Render(lang, "setup_nudge", base)
			if err != nil {
				t.Fatalf("render without reminder: %v", err)
			}
			if strings.Contains(withoutReminder, marker) {
				t.Fatalf("verify reminder must be absent when verify_email=false (%s)", lang)
			}
		})
	}
}
