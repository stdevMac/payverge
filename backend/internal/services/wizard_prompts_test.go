package services

import (
	"strings"
	"testing"
)

func TestGetWizardPromptLoadsRegionalPrompt(t *testing.T) {
	prompt := GetWizardPrompt("es-AR")

	if !strings.Contains(prompt, "Argentina") {
		t.Fatalf("expected regional Spanish prompt content, got %q", prompt)
	}
	if !strings.Contains(prompt, "is_complete") {
		t.Fatalf("expected prompt to preserve JSON schema instructions, got %q", prompt)
	}
	if !strings.Contains(prompt, "Español (Argentina)") {
		t.Fatalf("expected regional native language instruction, got %q", prompt)
	}
}

func TestGetWizardPromptAcceptsRegionalPathSegment(t *testing.T) {
	prompt := GetWizardPrompt("es-ar")

	if !strings.Contains(prompt, "Argentina") {
		t.Fatalf("expected path segment to resolve regional prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "Español (Argentina)") {
		t.Fatalf("expected regional native language instruction, got %q", prompt)
	}
}

func TestGetWizardPromptFallsBackToEnglish(t *testing.T) {
	prompt := GetWizardPrompt("unsupported")

	if !strings.Contains(prompt, "digital menu") {
		t.Fatalf("expected English fallback prompt, got %q", prompt)
	}
	if !strings.Contains(prompt, "English") {
		t.Fatalf("expected English language instruction, got %q", prompt)
	}
}

func TestDirectorFallbackCopyUsesRegionalSpanish(t *testing.T) {
	fallback := (&DirectorConsoleService{}).buildFallbackResponse(7, "es-AR", nil)
	if !strings.Contains(fallback.Summary, "plan práctico") {
		t.Fatalf("expected Spanish fallback copy for es-AR, got %q", fallback.Summary)
	}

	scope := buildScopeRedirectResponse(7, "es-AR")
	if !strings.Contains(scope.Summary, "ayudarte") {
		t.Fatalf("expected Spanish scope redirect copy for es-AR, got %q", scope.Summary)
	}

	followUps := defaultFollowUps("es-AR")
	if len(followUps) == 0 || !strings.Contains(followUps[0], "franja horaria") {
		t.Fatalf("expected Spanish follow-ups for es-AR, got %#v", followUps)
	}
}

func TestDirectorNormalizesRegionalPathSegment(t *testing.T) {
	prompt := buildDirectorSystemPrompt("Sage", 7, "es-ar")
	if !strings.Contains(prompt, "Argentina") {
		t.Fatalf("expected path segment to resolve regional director prompt, got %q", prompt)
	}

	prompt = buildDirectorSystemPrompt("Sage", 7, "ES_AR")
	if !strings.Contains(prompt, "Argentina") {
		t.Fatalf("expected mixed-case underscore locale to resolve regional director prompt, got %q", prompt)
	}
}

func TestDirectorNormalizeStructuredOutputUsesRegionalSpanishFallbackActions(t *testing.T) {
	output := (&DirectorConsoleService{}).normalizeStructuredOutput(7, "es-AR", "test question", DirectorStructuredResponse{
		ActionPlan: []DirectorAction{{DeepLink: "/business/7/dashboard?tab=not-real"}},
	}, nil)

	if len(output.ActionPlan) == 0 {
		t.Fatal("expected fallback action")
	}
	if output.ActionPlan[0].Title != "Acción (abre analytics)" {
		t.Fatalf("expected regional Spanish fallback action title with transparent deep-link suffix, got %q", output.ActionPlan[0].Title)
	}
	if strings.Contains(output.ActionPlan[0].Description, "Apply") {
		t.Fatalf("expected Spanish fallback action description, got %q", output.ActionPlan[0].Description)
	}
}
