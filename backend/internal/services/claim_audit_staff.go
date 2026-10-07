package services

// resolveClaimAuditStaffID picks a staff id that is safe for
// rbac_audit_logs.staff_id (NOT NULL + FK to staff).
//
// Owner principals (actor.StaffID == nil) have CanSteal but no staff row. Writing
// staff_id=0 500s on Postgres. Prefer the acting staff; otherwise the previous
// holder so the audit still names the claim subject. Returns 0 when neither is
// available — callers must skip the insert (ChangedBy/reason still capture the
// owner name when they do write).
func resolveClaimAuditStaffID(actorStaffID, fallbackStaffID *uint) uint {
	if actorStaffID != nil && *actorStaffID != 0 {
		return *actorStaffID
	}
	if fallbackStaffID != nil && *fallbackStaffID != 0 {
		return *fallbackStaffID
	}
	return 0
}
