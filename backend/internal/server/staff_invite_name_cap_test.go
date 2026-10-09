package server

import (
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin/binding"
)

// A staff name is rendered into invitation mail and every staff list; an
// unbounded value let one request store megabytes per row.
func TestStaffNameBindingIsCapped(t *testing.T) {
	ok := InviteStaffRequest{Email: "a@example.com", Name: strings.Repeat("n", 120), Role: database.StaffRole("waiter")}
	if err := binding.Validator.ValidateStruct(ok); err != nil {
		t.Fatalf("120-char name rejected: %v", err)
	}
	long := ok
	long.Name = strings.Repeat("n", 121)
	if err := binding.Validator.ValidateStruct(long); err == nil {
		t.Fatal("121-char invite name accepted")
	}
	if err := binding.Validator.ValidateStruct(AcceptInvitationRequest{Token: "t", Name: strings.Repeat("n", 121)}); err == nil {
		t.Fatal("121-char accept name accepted")
	}
}
