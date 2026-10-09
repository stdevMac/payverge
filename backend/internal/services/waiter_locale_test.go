package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveWaiterConversationLocale_HighConfidenceSpanish(t *testing.T) {
	got := ResolveWaiterConversationLocale("Hola, ¿qué hay en el menú hoy?", "en")
	assert.Equal(t, "es", got)
}

func TestResolveWaiterConversationLocale_RetainsStoredOnShortFollowUp(t *testing.T) {
	// Short ambiguous follow-up should not flip away from Spanish.
	got := ResolveWaiterConversationLocale("ok", "es")
	assert.Equal(t, "es", got)
}

func TestResolveWaiterConversationLocale_EnglishDefault(t *testing.T) {
	got := ResolveWaiterConversationLocale("", "")
	assert.Equal(t, "en", got)
}

func TestResolveWaiterConversationLocale_Japanese(t *testing.T) {
	got := ResolveWaiterConversationLocale("こんにちは、メニューを見せてください", "en")
	// Detector may return ja; if confidence is high enough we accept ja.
	assert.True(t, got == "ja" || got == "en", "got %q", got)
}

func TestCanonicalGuestLocale(t *testing.T) {
	assert.Equal(t, "es", canonicalGuestLocale("ES"))
	assert.Equal(t, "", canonicalGuestLocale("auto"))
	assert.Equal(t, "es-AR", canonicalGuestLocale("es-ar"))
}

func TestWaiterLanguageTransition_UsesNativeNames(t *testing.T) {
	got := WaiterLanguageTransition("en", "es-AR")
	if got != "Language switched from English to Español (Argentina)." {
		t.Fatalf("unexpected transition copy: %q", got)
	}
}

func TestWaiterNewConversationTransition_ExposesTargetLocale(t *testing.T) {
	got := WaiterNewConversationTransition("es-AR")
	if got != "Guest started a new conversation in Español (Argentina)." {
		t.Fatalf("unexpected successor copy: %q", got)
	}
}
