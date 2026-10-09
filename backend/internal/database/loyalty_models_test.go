package database

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLoyaltyTier_MarshalJSON_EmitsDollars(t *testing.T) {
	tier := LoyaltyTier{Name: "Silver", MinLifetimeSpentCents: 20000}
	b, err := json.Marshal(tier)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"min_lifetime_spent":200`) {
		t.Fatalf("expected dollars in JSON, got %s", s)
	}
	if strings.Contains(s, `min_lifetime_spent_cents`) {
		t.Fatalf("cents field should not appear in JSON, got %s", s)
	}
}
