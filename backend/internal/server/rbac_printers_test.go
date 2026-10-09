package server

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestPrinterPermissions_Exist(t *testing.T) {
	for _, p := range []Permission{
		PermPrintersRead, PermPrintersWrite, PermPrintBill, PermPrintReceipt,
	} {
		if string(p) == "" {
			t.Fatalf("permission constant not defined")
		}
	}
}

func TestStaffRolePermissions_PrinterDefaults(t *testing.T) {
	cases := []struct {
		role     database.StaffRole
		wantHas  []Permission
		wantMiss []Permission
	}{
		{
			role:    database.StaffRoleManager,
			wantHas: []Permission{PermPrintersRead, PermPrintersWrite, PermPrintBill, PermPrintReceipt},
		},
		{
			role:     database.StaffRoleServer,
			wantHas:  []Permission{PermPrintBill, PermPrintReceipt},
			wantMiss: []Permission{PermPrintersRead, PermPrintersWrite},
		},
		{
			role:     database.StaffRoleHost,
			wantHas:  []Permission{PermPrintBill},
			wantMiss: []Permission{PermPrintersWrite, PermPrintReceipt},
		},
		{
			role:     database.StaffRoleKitchen,
			wantMiss: []Permission{PermPrintersRead, PermPrintersWrite, PermPrintBill, PermPrintReceipt},
		},
	}
	for _, tc := range cases {
		t.Run(string(tc.role), func(t *testing.T) {
			perms := StaffRolePermissions[tc.role]
			set := map[Permission]bool{}
			for _, p := range perms {
				set[p] = true
			}
			for _, want := range tc.wantHas {
				if !set[want] {
					t.Fatalf("%s missing permission %q", tc.role, want)
				}
			}
			for _, miss := range tc.wantMiss {
				if set[miss] {
					t.Fatalf("%s unexpectedly has permission %q", tc.role, miss)
				}
			}
		})
	}
}
