package database

// StaffNotifyTarget is one resolved recipient for notifyStaff: the staff id, the
// User id behind it (0 = no account → inbox only, no push), and the recipient's
// language tag ("" = use the business default). No money fields.
type StaffNotifyTarget struct {
	StaffID  uint   `gorm:"column:staff_id"`
	UserID   uint   `gorm:"column:user_id"`
	Language string `gorm:"column:language"`
}

// ResolveStaffNotifyTargets maps active staff ids to (user_id, language) in ONE
// LEFT JOIN over the globally-unique staff.email → users.email seam. Inactive
// staff are dropped. A staffer with no User account yields user_id 0 (inbox +
// SSE, no push). Single query — no per-staff N+1.
func (d *DB) ResolveStaffNotifyTargets(businessID uint, staffIDs []uint) ([]StaffNotifyTarget, error) {
	if len(staffIDs) == 0 {
		return nil, nil
	}
	var rows []StaffNotifyTarget
	err := d.GetGorm().
		Table("staff AS s").
		Select("s.id AS staff_id, COALESCE(u.id, 0) AS user_id, COALESCE(u.language_selected, '') AS language").
		// Case-insensitive on BOTH sides: staff emails are lowercased at write,
		// but user emails are stored verbatim (email/password registration), so a
		// verbatim join silently drops push for mixed-case user accounts.
		Joins("LEFT JOIN users u ON LOWER(u.email) = LOWER(s.email)").
		Where("s.business_id = ? AND s.is_active AND s.id IN ?", businessID, staffIDs).
		Scan(&rows).Error
	return rows, err
}
