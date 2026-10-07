package main

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/auth"

	"golang.org/x/crypto/bcrypt"
)

func TestReadPasswordFromArgs(t *testing.T) {
	p, err := readPassword([]string{"--", "Olive-Oven-Midnight-42!"})
	if err != nil {
		t.Fatalf("readPassword: %v", err)
	}
	if p != "Olive-Oven-Midnight-42!" {
		t.Fatalf("got %q", p)
	}
}

func TestReadPasswordMissingAfterDashDash(t *testing.T) {
	_, err := readPassword([]string{"--"})
	if err == nil {
		t.Fatal("expected error for missing password after --")
	}
}

func TestHashPasswordMatchesAuthPackage(t *testing.T) {
	// Integration of the tool's sole purpose: auth.HashPassword output verifies.
	const password = "Olive-Oven-Midnight-42!"
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		t.Fatalf("bcrypt verify: %v", err)
	}
}
