package config

import "testing"

func TestAIProviderConfiguredDefaultsFalseAndOverrideRestores(t *testing.T) {
	prev := AIProviderConfigured()
	t.Cleanup(func() { SetAIProviderConfigured(prev) })

	SetAIProviderConfigured(false)
	if AIProviderConfigured() {
		t.Fatal("expected false after SetAIProviderConfigured(false)")
	}
	t.Run("override", func(t *testing.T) {
		SetAIProviderConfiguredForTesting(t, true)
		if !AIProviderConfigured() {
			t.Fatal("override not applied")
		}
	})
	if AIProviderConfigured() {
		t.Fatal("override cleanup should restore false")
	}
}
