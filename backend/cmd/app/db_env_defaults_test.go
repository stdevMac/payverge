package main

import "testing"

func TestEnvOrDefault_DBFlagsFallBackToEnv(t *testing.T) {
	t.Setenv("DB_HOST", "  db.internal ")
	if got := envOrDefault("DB_HOST", "localhost"); got != "db.internal" {
		t.Fatalf("DB_HOST env not used: %q", got)
	}
	t.Setenv("DB_HOST", "   ")
	if got := envOrDefault("DB_HOST", "localhost"); got != "localhost" {
		t.Fatalf("blank env must fall back: %q", got)
	}
}
