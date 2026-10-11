package server

import "testing"

func TestStaffRoleDisplayName(t *testing.T) {
	if got := staffRoleDisplayName("kitchen", "es-AR"); got != "Cocina" {
		t.Fatalf("got %q", got)
	}
	if got := staffRoleDisplayName("server", "es-AR"); got != "Mozo" {
		t.Fatalf("got %q", got)
	}
	if got := staffRoleDisplayName("manager", "en"); got != "Manager" {
		t.Fatalf("got %q", got)
	}
}
