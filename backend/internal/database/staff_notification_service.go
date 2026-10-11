package database

import "time"

// StaffNotification is one durable inbox row: the source of truth behind the
// staff notification bell. SSE + web push are best-effort layers on top; this
// row survives a phone being off. No money fields — staff surfaces are money-free
// (CLAUDE.md). read_at NULL = unread. Tags match the genesis
// staff_notifications table.
type StaffNotification struct {
	ID         uint       `gorm:"primaryKey;index:idx_staff_notifications_inbox,priority:3" json:"id"`
	BusinessID uint       `gorm:"not null;index:idx_staff_notifications_inbox,priority:1" json:"business_id"`
	StaffID    uint       `gorm:"not null;index:idx_staff_notifications_inbox,priority:2" json:"staff_id"`
	Kind       string     `gorm:"type:varchar(32);not null" json:"kind"`
	Title      string     `gorm:"type:varchar(255);not null" json:"title"`
	Body       string     `gorm:"type:varchar(500);not null;default:''" json:"body"`
	URL        string     `gorm:"type:varchar(512);not null;default:''" json:"url"`
	ReadAt     *time.Time `json:"read_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// TableName pins the table name so GORM does not guess; equals the 000111 DDL.
func (StaffNotification) TableName() string { return "staff_notifications" }

// staffNotificationColumns is the explicit projection used by the inbox read so
// it never widens to SELECT * (perf access-shape gate, CLAUDE.md).
const staffNotificationColumns = "id, business_id, staff_id, kind, title, body, url, read_at, created_at"

// staffNotificationListCap bounds the inbox page (and the default page size).
const staffNotificationListCap = 50

// Column limits from the 000111 DDL. Enforced app-side because the batch
// INSERT is all-or-nothing: one oversized announcement title would otherwise
// nuke the entire inbox batch while SSE + push still fire.
const (
	staffNotificationTitleMaxRunes = 255
	staffNotificationBodyMaxRunes  = 500
)

// clampStaffNotificationText rune-safely truncates s to max runes, replacing
// the final rune with an ellipsis when clamped. VARCHAR(n) counts characters
// (runes), so the budget is runes, not bytes.
func clampStaffNotificationText(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}

// CreateStaffNotifications batch-inserts inbox rows (one event can target many
// staff; a single multi-row INSERT). A no-op on an empty slice. Title/body are
// clamped to their column limits so an unbounded source (announcement titles)
// can never fail the whole batch.
func (d *DB) CreateStaffNotifications(rows []StaffNotification) error {
	if len(rows) == 0 {
		return nil
	}
	for i := range rows {
		rows[i].Title = clampStaffNotificationText(rows[i].Title, staffNotificationTitleMaxRunes)
		rows[i].Body = clampStaffNotificationText(rows[i].Body, staffNotificationBodyMaxRunes)
	}
	return d.GetGorm().Create(&rows).Error
}

// ListStaffNotifications returns a staff member's own inbox, newest first, over
// the (business_id, staff_id, id) index. Keyset-paginated by id (beforeID==0 =
// first page), explicit projection, bounded. Tenant + row scoped in SQL.
func (d *DB) ListStaffNotifications(businessID, staffID uint, limit int, beforeID uint) ([]StaffNotification, error) {
	if limit <= 0 || limit > staffNotificationListCap {
		limit = staffNotificationListCap
	}
	q := d.GetGorm().
		Select(staffNotificationColumns).
		Where("business_id = ? AND staff_id = ?", businessID, staffID)
	if beforeID > 0 {
		q = q.Where("id < ?", beforeID)
	}
	var out []StaffNotification
	err := q.Order("id desc").Limit(limit).Find(&out).Error
	return out, err
}

// CountUnreadStaffNotifications returns the unread (read_at IS NULL) count for a
// staff member's own inbox.
func (d *DB) CountUnreadStaffNotifications(businessID, staffID uint) (int64, error) {
	var n int64
	err := d.GetGorm().Model(&StaffNotification{}).
		Where("business_id = ? AND staff_id = ? AND read_at IS NULL", businessID, staffID).
		Count(&n).Error
	return n, err
}

// MarkStaffNotificationsRead stamps read_at on the caller's own unread rows. With
// all==true it clears the whole inbox; otherwise it clears the given ids. The
// `read_at IS NULL` predicate makes it idempotent (re-marking is a no-op).
// Returns rows affected.
func (d *DB) MarkStaffNotificationsRead(businessID, staffID uint, ids []uint, all bool) (int64, error) {
	now := time.Now().UTC()
	q := d.GetGorm().Model(&StaffNotification{}).
		Where("business_id = ? AND staff_id = ? AND read_at IS NULL", businessID, staffID)
	if !all {
		if len(ids) == 0 {
			return 0, nil
		}
		q = q.Where("id IN ?", ids)
	}
	res := q.Update("read_at", now)
	return res.RowsAffected, res.Error
}
