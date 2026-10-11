package main

import (
	"os"
	"strings"
	"testing"
)

func TestLegacyHostedAdminBootstrapRemoved(t *testing.T) {
	src, err := os.ReadFile("oss_admin.go")
	if err != nil {
		t.Fatalf("read oss_admin.go: %v", err)
	}
	body := string(src)
	for _, forbidden := range []string{
		"BOOTSTRAP_DEFAULT_ADMINS",
		"DEFAULT_ADMIN_WALLET",
		"DEFAULT_ADMIN_EMAIL",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("oss_admin.go still contains %q", forbidden)
		}
	}
}
