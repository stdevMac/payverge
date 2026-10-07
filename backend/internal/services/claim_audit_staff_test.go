package services

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveClaimAuditStaffID_NeverReturnsZeroWhenFallbackExists(t *testing.T) {
	// Owner principal (nil StaffID) force-releasing a staff hold must use the
	// previous holder's staff id — never 0 (Postgres FK on rbac_audit_logs).
	prev := uint(42)
	require.Equal(t, uint(42), resolveClaimAuditStaffID(nil, &prev))

	actor := uint(7)
	require.Equal(t, uint(7), resolveClaimAuditStaffID(&actor, &prev))

	require.Equal(t, uint(0), resolveClaimAuditStaffID(nil, nil))
	zero := uint(0)
	require.Equal(t, uint(42), resolveClaimAuditStaffID(&zero, &prev))
}
