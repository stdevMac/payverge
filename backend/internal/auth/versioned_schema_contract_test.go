package auth

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/schema/genesis"
)

// TestAuthSchemaIsOwnedByGenesisBaseline prevents auth startup from quietly
// reintroducing GORM/ALTER-based schema ownership: the auth tables, columns
// and indexes must come from the versioned genesis baseline.
func TestAuthSchemaIsOwnedByGenesisBaseline(t *testing.T) {
	t.Parallel()
	sql := genesis.SchemaSQL
	for _, required := range []string{
		"CREATE TABLE public.user_auths (",
		"CREATE TABLE public.user_sessions (",
		"idx_users_google_id",
	} {
		if !strings.Contains(sql, required) {
			t.Errorf("genesis baseline missing %q", required)
		}
	}
	users := regexp.MustCompile(`(?s)CREATE TABLE public\.users \((.*?)\n\);`).FindStringSubmatch(sql)
	if users == nil {
		t.Fatal("genesis baseline has no users table")
	}
	for _, col := range []string{"google_id", "email_verified", "username"} {
		if !regexp.MustCompile(`(?m)^\s+` + col + ` `).MatchString(users[1]) {
			t.Errorf("users table missing column %q", col)
		}
	}
	for _, col := range []string{"instagram_id", "token_id"} {
		if strings.Contains(users[1], col) {
			t.Errorf("users table still contains removed column %q", col)
		}
	}
	if strings.Contains(sql, "idx_users_instagram_id") {
		t.Error("genesis baseline still contains idx_users_instagram_id")
	}
	// Email/OAuth users have no wallet address.
	if !regexp.MustCompile(`(?m)^\s+address text,$`).MatchString(users[1]) {
		t.Error("users.address must be nullable")
	}
}
