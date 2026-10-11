package database

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stdevmac/payverge/backend/internal/logger"
)

// RBACActionAnnouncementDeleted is the audit Action recorded when a manager
// hard-deletes an announcement. The announcements table has no soft-delete
// column, so the deletion is captured in the RBAC audit trail (who removed which
// announcement) — the accountability substitute for a tombstone row.
const RBACActionAnnouncementDeleted RBACAction = "announcement_deleted"

// Announcement audience tags. "all" targets every active staff member; "role:<r>"
// targets a single staff role; "dept:<d>" targets every staff member holding an
// active position in that department.
const (
	AnnouncementAudienceAll        = "all"
	announcementAudienceRolePrefix = "role:"
	announcementAudienceDeptPrefix = "dept:"

	// chatAnnouncementListMax bounds the per-caller announcement feed; the ack
	// roster is bounded by chatAnnouncementAudienceMax (perf gate).
	chatAnnouncementListMax     = 200
	chatAnnouncementAudienceMax = 1000

	// AnnouncementTitleMaxRunes caps an announcement title. The body shares
	// ChatContentMaxRunes with chat messages.
	AnnouncementTitleMaxRunes = 200
)

var (
	// ErrAnnouncementNotFound is returned when an announcement does not exist in
	// the caller's business (tenant-scoped lookups).
	ErrAnnouncementNotFound = errors.New("announcement not found")
	// ErrAnnouncementTitleRequired is a validation error for an empty title.
	ErrAnnouncementTitleRequired = errors.New("announcement title is required")
	// ErrAnnouncementTooLong is a validation error for a title over
	// AnnouncementTitleMaxRunes or content over ChatContentMaxRunes.
	ErrAnnouncementTooLong = fmt.Errorf("announcement title must be %d characters or fewer and content %d or fewer", AnnouncementTitleMaxRunes, ChatContentMaxRunes)
	// ErrAnnouncementNotEligible is returned when a staff member tries to ack an
	// announcement whose audience does not include them.
	ErrAnnouncementNotEligible = errors.New("staff is not in the announcement audience")
	// ErrInvalidAudienceFilter is returned for an audience filter that is not
	// "all", "role:<role>" or "dept:<dept>".
	ErrInvalidAudienceFilter = errors.New("invalid announcement audience filter")
)

// validateAnnouncementText trims the title and enforces the required title and
// the title/content length caps shared by create and update.
func validateAnnouncementText(title, content string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", ErrAnnouncementTitleRequired
	}
	if utf8.RuneCountInString(title) > AnnouncementTitleMaxRunes || utf8.RuneCountInString(content) > ChatContentMaxRunes {
		return "", ErrAnnouncementTooLong
	}
	return title, nil
}

// normalizeAudienceFilter validates and canonicalizes the audience tag. Empty
// defaults to "all". A bare "role:"/"dept:" (no suffix) is rejected.
func normalizeAudienceFilter(filter string) (string, error) {
	f := strings.TrimSpace(filter)
	if f == "" || f == AnnouncementAudienceAll {
		return AnnouncementAudienceAll, nil
	}
	if strings.HasPrefix(f, announcementAudienceRolePrefix) && strings.TrimPrefix(f, announcementAudienceRolePrefix) != "" {
		return f, nil
	}
	if strings.HasPrefix(f, announcementAudienceDeptPrefix) && strings.TrimPrefix(f, announcementAudienceDeptPrefix) != "" {
		return f, nil
	}
	return "", ErrInvalidAudienceFilter
}

// AnnouncementAudienceMember is one entry in the unacked roster.
type AnnouncementAudienceMember struct {
	StaffID uint   `json:"staff_id"`
	Name    string `json:"name"`
}

// AnnouncementAckStatus is the "X of Y confirmed" aggregate: how many of the
// eligible audience have acknowledged, the total eligible count, and the list of
// staff who have NOT yet acknowledged.
type AnnouncementAckStatus struct {
	AnnouncementID uint                         `json:"announcement_id"`
	Acked          int                          `json:"acked"`
	TotalEligible  int                          `json:"total_eligible"`
	Unacked        []AnnouncementAudienceMember `json:"unacked"`
}

// CreateAnnouncement persists a broadcast. AuthorStaffID may be 0 for a business
// owner (who authenticates by wallet and has no staff row). The audience filter is
// validated; an empty title, an over-long title or content, or a malformed
// audience is rejected. CreatedAt is UTC.
func (d *DB) CreateAnnouncement(businessID, authorStaffID uint, title, content string, requireAck bool, audienceFilter string) (*Announcement, error) {
	if businessID == 0 {
		return nil, ErrAnnouncementNotFound
	}
	title, err := validateAnnouncementText(title, content)
	if err != nil {
		return nil, err
	}
	filter, err := normalizeAudienceFilter(audienceFilter)
	if err != nil {
		return nil, err
	}
	ann := Announcement{
		BusinessID:     businessID,
		AuthorStaffID:  authorStaffID,
		Title:          title,
		Content:        content,
		RequireAck:     requireAck,
		AudienceFilter: filter,
		CreatedAt:      time.Now().UTC(),
	}
	if err := d.GetGorm().Create(&ann).Error; err != nil {
		return nil, err
	}
	return &ann, nil
}

// UpdateAnnouncement edits a broadcast in place: title/content/require_ack/
// audience are replaced, UpdatedAt bumps, and the row id + its ack rows are
// preserved (unlike a delete+recreate, an edit does NOT reset who has already
// confirmed). The announcement must exist in the caller's business
// (ErrAnnouncementNotFound), the title must be non-empty, title and content
// must fit the length caps (ErrAnnouncementTooLong), and the audience is
// validated/canonicalized (empty → "all", malformed → ErrInvalidAudienceFilter).
func (d *DB) UpdateAnnouncement(businessID, announcementID uint, title, content string, requireAck bool, audienceFilter string) (*Announcement, error) {
	if businessID == 0 || announcementID == 0 {
		return nil, ErrAnnouncementNotFound
	}
	title, err := validateAnnouncementText(title, content)
	if err != nil {
		return nil, err
	}
	filter, err := normalizeAudienceFilter(audienceFilter)
	if err != nil {
		return nil, err
	}
	var ann Announcement
	if err := d.GetGorm().
		Where("id = ? AND business_id = ?", announcementID, businessID).
		First(&ann).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAnnouncementNotFound
		}
		return nil, err
	}
	if err := d.GetGorm().Model(&Announcement{}).
		Where("id = ? AND business_id = ?", announcementID, businessID).
		Updates(map[string]interface{}{
			"title":           title,
			"content":         content,
			"require_ack":     requireAck,
			"audience_filter": filter,
		}).Error; err != nil {
		return nil, err
	}
	ann.Title, ann.Content, ann.RequireAck, ann.AudienceFilter = title, content, requireAck, filter
	return &ann, nil
}

// DeleteAnnouncement hard-deletes an announcement and its ack rows in one
// transaction (the announcements table has no soft-delete column), then records
// an RBAC audit entry capturing who removed which announcement — the
// accountability substitute for a tombstone. Deleting stops the announcement's
// ack tracking (the acks cascade). Tenant-scoped: a cross-tenant/unknown id
// touches zero rows and returns ErrAnnouncementNotFound. byStaffID is 0 for an
// owner (who authenticates by wallet).
func (d *DB) DeleteAnnouncement(businessID, announcementID, byStaffID uint) error {
	if businessID == 0 || announcementID == 0 {
		return ErrAnnouncementNotFound
	}
	return d.GetGorm().Transaction(func(tx *gorm.DB) error {
		// Load the row (author) before the delete so the audit can attribute it and
		// so a cross-tenant/unknown id is a clean not-found before any write.
		var ann Announcement
		if err := tx.Select("id", "business_id", "author_staff_id").
			Where("id = ? AND business_id = ?", announcementID, businessID).
			First(&ann).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAnnouncementNotFound
			}
			return err
		}
		if err := tx.Where("id = ? AND business_id = ?", announcementID, businessID).
			Delete(&Announcement{}).Error; err != nil {
			return err
		}
		// Stop ack tracking: remove the acks for the deleted announcement.
		if err := tx.Where("announcement_id = ? AND business_id = ?", announcementID, businessID).
			Delete(&AnnouncementAck{}).Error; err != nil {
			return err
		}
		// Audit trail (the table has no tombstone). rbac_audit_logs.staff_id has a
		// NOT-NULL FK to staff(id), so the row's subject staff_id must reference a
		// real staff row. We attribute to the actor when they are a staff member,
		// else the announcement author when they were a staff member; when NEITHER
		// exists (an owner deleting an owner-authored notice — both ids 0) we skip
		// the audit row to avoid an FK violation. The delete itself always stands.
		subjectStaffID := byStaffID
		if subjectStaffID == 0 {
			subjectStaffID = ann.AuthorStaffID
		}
		if subjectStaffID != 0 {
			audit := RBACAuditLog{
				StaffID:    subjectStaffID,
				BusinessID: businessID,
				Action:     RBACActionAnnouncementDeleted,
				ChangedBy:  fmt.Sprintf("staff:%d", byStaffID),
				Reason:     fmt.Sprintf("announcement deleted: id %d author %d", announcementID, ann.AuthorStaffID),
				CreatedAt:  time.Now().UTC(),
			}
			if aerr := tx.Create(&audit).Error; aerr != nil {
				logger.Logger.Warnf("announcement delete audit write failed (id %d): %v", announcementID, aerr)
			}
		}
		return nil
	})
}

// ListAnnouncements returns announcements newest first. When isManager is true
// (chat:announce holder / manager-or-owner), every business announcement is
// returned so operators can re-list role:/dept: notices they created. When false,
// the list is audience-scoped: "all", "role:<role>", and "dept:<d>" for each
// department the caller holds an active position in. Staff path is two bounded
// queries; manager path is one. No N+1 per announcement.
func (d *DB) ListAnnouncements(businessID, staffID uint, role string, isManager bool) ([]Announcement, error) {
	g := d.GetGorm()
	out := make([]Announcement, 0, 8)

	if isManager {
		err := g.Where("business_id = ?", businessID).
			Order("created_at DESC, id DESC").
			Limit(chatAnnouncementListMax).
			Find(&out).Error
		return out, err
	}

	var depts []string
	if err := g.Table("staff_positions AS sp").
		Joins("JOIN positions ON positions.id = sp.position_id AND positions.business_id = sp.business_id").
		Where("sp.business_id = ? AND sp.staff_id = ? AND positions.is_active = ? AND positions.department <> ''", businessID, staffID, true).
		Distinct().
		Limit(chatAnnouncementListMax).
		Pluck("positions.department", &depts).Error; err != nil {
		return nil, err
	}

	audiences := make([]string, 0, len(depts)+2)
	audiences = append(audiences, AnnouncementAudienceAll)
	if role != "" {
		audiences = append(audiences, announcementAudienceRolePrefix+role)
	}
	for _, dt := range depts {
		audiences = append(audiences, announcementAudienceDeptPrefix+dt)
	}

	err := g.Where("business_id = ? AND audience_filter IN ?", businessID, audiences).
		Order("created_at DESC, id DESC").
		Limit(chatAnnouncementListMax).
		Find(&out).Error
	return out, err
}

// CountUnackedAnnouncements returns how many require_ack announcements are
// targeted at the caller (audiences all + role:<role> + dept:<each active dept>,
// exactly as ListAnnouncements resolves them) that the caller has NOT yet
// acknowledged — the badge count for the buried announcements surface. It is TWO
// bounded queries (the caller's depts, then one NOT-EXISTS count) — never N+1 per
// announcement. An owner (staffID 0, who cannot ack) counts nothing.
func (d *DB) CountUnackedAnnouncements(businessID, staffID uint, role string) (int64, error) {
	if businessID == 0 || staffID == 0 {
		return 0, nil
	}
	g := d.GetGorm()

	var depts []string
	if err := g.Table("staff_positions AS sp").
		Joins("JOIN positions ON positions.id = sp.position_id AND positions.business_id = sp.business_id").
		Where("sp.business_id = ? AND sp.staff_id = ? AND positions.is_active = ? AND positions.department <> ''", businessID, staffID, true).
		Distinct().
		Limit(chatAnnouncementListMax).
		Pluck("positions.department", &depts).Error; err != nil {
		return 0, err
	}

	audiences := make([]string, 0, len(depts)+2)
	audiences = append(audiences, AnnouncementAudienceAll)
	if role != "" {
		audiences = append(audiences, announcementAudienceRolePrefix+role)
	}
	for _, dt := range depts {
		audiences = append(audiences, announcementAudienceDeptPrefix+dt)
	}

	var n int64
	err := g.Model(&Announcement{}).
		Where("business_id = ? AND require_ack = ? AND audience_filter IN ?", businessID, true, audiences).
		Where("NOT EXISTS (SELECT 1 FROM announcement_acks a WHERE a.announcement_id = announcements.id AND a.staff_id = ? AND a.business_id = ?)", staffID, businessID).
		Count(&n).Error
	return n, err
}

// AnnouncementsAckedBy returns the set of announcement ids (from the given bounded
// list) the staff member has ALREADY acknowledged — the per-caller "already
// confirmed" flag the feed needs so an acked notice stops nagging after reload. It
// is ONE query, never N+1; an empty/zero staffID (an owner, who cannot ack) or an
// empty id list yields an empty set.
func (d *DB) AnnouncementsAckedBy(businessID, staffID uint, announcementIDs []uint) (map[uint]bool, error) {
	out := make(map[uint]bool, len(announcementIDs))
	if staffID == 0 || len(announcementIDs) == 0 {
		return out, nil
	}
	var ids []uint
	err := d.GetGorm().Model(&AnnouncementAck{}).
		Where("business_id = ? AND staff_id = ? AND announcement_id IN ?", businessID, staffID, announcementIDs).
		Pluck("announcement_id", &ids).Error
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// AckAnnouncement records a staff member's acknowledgment, idempotent on the
// composite PK (announcement_id, staff_id): a repeat ack is a no-op that preserves
// the original AcknowledgedAt. The announcement must exist in the caller's
// business (tenant-scoped) or ErrAnnouncementNotFound is returned. The staff
// member must be in the announcement audience (active + audience filter) or
// ErrAnnouncementNotEligible is returned — matching GetAnnouncementAckStatus.
func (d *DB) AckAnnouncement(businessID, announcementID, staffID uint) (*AnnouncementAck, error) {
	if businessID == 0 || announcementID == 0 || staffID == 0 {
		return nil, ErrAnnouncementNotFound
	}
	g := d.GetGorm()
	var ann Announcement
	if err := g.Where("id = ? AND business_id = ?", announcementID, businessID).First(&ann).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAnnouncementNotFound
		}
		return nil, err
	}
	eligible, err := d.staffInAnnouncementAudience(businessID, staffID, ann.AudienceFilter)
	if err != nil {
		return nil, err
	}
	if !eligible {
		return nil, ErrAnnouncementNotEligible
	}
	ack := AnnouncementAck{
		AnnouncementID: announcementID,
		StaffID:        staffID,
		BusinessID:     businessID,
		AcknowledgedAt: time.Now().UTC(),
	}
	// First ack wins — DoNothing preserves the original acknowledged_at timestamp.
	if err := g.Clauses(clause.OnConflict{DoNothing: true}).Create(&ack).Error; err != nil {
		return nil, err
	}
	var out AnnouncementAck
	if err := g.Where("announcement_id = ? AND staff_id = ?", announcementID, staffID).First(&out).Error; err != nil {
		return nil, err
	}
	return &out, nil
}

// staffInAnnouncementAudience reports whether active staffID is in the audience
// for filter (same rules as GetAnnouncementAckStatus eligibility).
func (d *DB) staffInAnnouncementAudience(businessID, staffID uint, audienceFilter string) (bool, error) {
	g := d.GetGorm()
	q := g.Model(&Staff{}).
		Where("id = ? AND business_id = ? AND is_active = ?", staffID, businessID, true)
	switch {
	case strings.HasPrefix(audienceFilter, announcementAudienceRolePrefix):
		q = q.Where("role = ?", strings.TrimPrefix(audienceFilter, announcementAudienceRolePrefix))
	case strings.HasPrefix(audienceFilter, announcementAudienceDeptPrefix):
		dept := strings.TrimPrefix(audienceFilter, announcementAudienceDeptPrefix)
		q = q.Where("EXISTS (SELECT 1 FROM staff_positions sp JOIN positions p ON p.id = sp.position_id AND p.business_id = sp.business_id WHERE sp.staff_id = staff.id AND sp.business_id = ? AND p.is_active = ? AND p.department = ?)",
			businessID, true, dept)
	default:
		// "all" — any active staff member.
	}
	var n int64
	if err := q.Limit(1).Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

// GetAnnouncementAckStatus computes the "X of Y confirmed" aggregate for an
// announcement in ONE bounded query set (the announcement fetch, then a single
// eligible-staff LEFT JOIN announcement_acks) — never an N+1 per staff member.
// Eligibility is resolved from the announcement's audience filter: "all" =>
// every active staff member; "role:<r>" => staff with that role; "dept:<d>" =>
// staff holding an active position in that department.
func (d *DB) GetAnnouncementAckStatus(businessID, announcementID uint) (*AnnouncementAckStatus, error) {
	g := d.GetGorm()
	var ann Announcement
	if err := g.Where("id = ? AND business_id = ?", announcementID, businessID).First(&ann).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAnnouncementNotFound
		}
		return nil, err
	}

	type memberRow struct {
		StaffID uint
		Name    string
		Acked   bool
	}
	q := g.Table("staff AS s").
		Select("s.id AS staff_id, s.name AS name, (a.staff_id IS NOT NULL) AS acked").
		Joins("LEFT JOIN announcement_acks a ON a.announcement_id = ? AND a.staff_id = s.id AND a.business_id = ?", announcementID, businessID).
		Where("s.business_id = ? AND s.is_active = ?", businessID, true)
	switch {
	case strings.HasPrefix(ann.AudienceFilter, announcementAudienceRolePrefix):
		q = q.Where("s.role = ?", strings.TrimPrefix(ann.AudienceFilter, announcementAudienceRolePrefix))
	case strings.HasPrefix(ann.AudienceFilter, announcementAudienceDeptPrefix):
		dept := strings.TrimPrefix(ann.AudienceFilter, announcementAudienceDeptPrefix)
		q = q.Where("EXISTS (SELECT 1 FROM staff_positions sp JOIN positions p ON p.id = sp.position_id AND p.business_id = sp.business_id WHERE sp.staff_id = s.id AND sp.business_id = ? AND p.is_active = ? AND p.department = ?)",
			businessID, true, dept)
	default:
		// "all" — every active staff member is eligible; no extra predicate.
	}

	var rows []memberRow
	if err := q.Order("s.id ASC").Limit(chatAnnouncementAudienceMax).Scan(&rows).Error; err != nil {
		return nil, err
	}

	status := &AnnouncementAckStatus{AnnouncementID: announcementID, Unacked: []AnnouncementAudienceMember{}}
	for _, r := range rows {
		status.TotalEligible++
		if r.Acked {
			status.Acked++
		} else {
			status.Unacked = append(status.Unacked, AnnouncementAudienceMember{StaffID: r.StaffID, Name: r.Name})
		}
	}
	return status, nil
}

// AnnouncementAckSummary is the compact per-announcement ack aggregate the feed
// renders inline: acked/total plus the first few acker names (for the "Ana, Bo
// +3" affordance). The full unacked roster stays behind GetAnnouncementAckStatus
// (loaded on expand). No per-announcement N+1 — see GetAnnouncementAckSummaries.
type AnnouncementAckSummary struct {
	AnnouncementID  uint     `json:"announcement_id"`
	Acked           int      `json:"acked"`
	TotalEligible   int      `json:"total_eligible"`
	FirstAckerNames []string `json:"first_acker_names"`
}

// GetAnnouncementAckSummaries returns the compact {acked, total_eligible, first
// acker names} aggregate for a BOUNDED list of announcement ids in a SMALL
// CONSTANT number of queries — never one per announcement. It replaces the
// per-announcement AckRoster poll (the audit's one true HTTP N+1): the feed calls
// this once per refresh for the VISIBLE announcements instead of mounting a
// 30s-poll roster per require_ack notice.
//
// Eligibility is resolved with the SAME predicate GetAnnouncementAckStatus uses,
// but batched by audience family so announcement count never drives query count:
//   - all announcements share one active-staff count,
//   - role:<r> announcements share a per-role active-staff count,
//   - dept:<d> announcements share a per-department active-position-staff count.
//
// firstNamesLimit bounds the acker-name preview per announcement (clamped 0..10).
// Tenant-scoped; an empty id list yields an empty map (no query).
func (d *DB) GetAnnouncementAckSummaries(businessID uint, announcementIDs []uint, firstNamesLimit int) (map[uint]AnnouncementAckSummary, error) {
	out := make(map[uint]AnnouncementAckSummary, len(announcementIDs))
	if businessID == 0 || len(announcementIDs) == 0 {
		return out, nil
	}
	if firstNamesLimit < 0 {
		firstNamesLimit = 0
	}
	if firstNamesLimit > 10 {
		firstNamesLimit = 10
	}
	g := d.GetGorm()

	// Q1: the announcements + their audience filters (tenant-scoped). Only ids that
	// exist in this business are returned, so a cross-tenant/unknown id drops out.
	type annRow struct {
		ID             uint
		AudienceFilter string
	}
	var anns []annRow
	if err := g.Model(&Announcement{}).
		Select("id", "audience_filter").
		Where("business_id = ? AND id IN ?", businessID, announcementIDs).
		Find(&anns).Error; err != nil {
		return nil, err
	}
	if len(anns) == 0 {
		return out, nil
	}

	// Group the resolved ids by audience family so the total-eligible math is a
	// small constant number of queries regardless of announcement count.
	needAll := false
	roleWanted := map[string]bool{}
	deptWanted := map[string]bool{}
	audienceByID := make(map[uint]string, len(anns))
	presentIDs := make([]uint, 0, len(anns))
	for _, a := range anns {
		audienceByID[a.ID] = a.AudienceFilter
		presentIDs = append(presentIDs, a.ID)
		switch {
		case strings.HasPrefix(a.AudienceFilter, announcementAudienceRolePrefix):
			roleWanted[strings.TrimPrefix(a.AudienceFilter, announcementAudienceRolePrefix)] = true
		case strings.HasPrefix(a.AudienceFilter, announcementAudienceDeptPrefix):
			deptWanted[strings.TrimPrefix(a.AudienceFilter, announcementAudienceDeptPrefix)] = true
		default:
			needAll = true
		}
	}

	// Q2: acked counts per announcement from ACTIVE eligible staff only (same
	// audience predicates as GetAnnouncementAckStatus). Raw COUNT(*) of all acks
	// could exceed total_eligible when inactive/out-of-audience rows exist.
	type countRow struct {
		AnnouncementID uint
		N              int
	}
	ackedByID := make(map[uint]int, len(presentIDs))
	// "all" + default audiences: active staff acks only.
	var allAcked []countRow
	if err := g.Table("announcement_acks AS a").
		Select("a.announcement_id AS announcement_id, COUNT(*) AS n").
		Joins("JOIN staff s ON s.id = a.staff_id AND s.business_id = a.business_id AND s.is_active = ?", true).
		Joins("JOIN announcements ann ON ann.id = a.announcement_id AND ann.business_id = a.business_id").
		Where("a.business_id = ? AND a.announcement_id IN ?", businessID, presentIDs).
		Where("ann.audience_filter = ? OR (ann.audience_filter NOT LIKE ? AND ann.audience_filter NOT LIKE ?)",
			AnnouncementAudienceAll, announcementAudienceRolePrefix+"%", announcementAudienceDeptPrefix+"%").
		Group("a.announcement_id").
		Scan(&allAcked).Error; err != nil {
		return nil, err
	}
	for _, r := range allAcked {
		ackedByID[r.AnnouncementID] = r.N
	}
	// role:<r> audiences — one query per distinct role (small constant fan-out).
	for roleName := range roleWanted {
		filter := announcementAudienceRolePrefix + roleName
		var roleAcked []countRow
		if err := g.Table("announcement_acks AS a").
			Select("a.announcement_id AS announcement_id, COUNT(*) AS n").
			Joins("JOIN staff s ON s.id = a.staff_id AND s.business_id = a.business_id AND s.is_active = ? AND s.role = ?", true, roleName).
			Joins("JOIN announcements ann ON ann.id = a.announcement_id AND ann.business_id = a.business_id AND ann.audience_filter = ?", filter).
			Where("a.business_id = ? AND a.announcement_id IN ?", businessID, presentIDs).
			Group("a.announcement_id").
			Scan(&roleAcked).Error; err != nil {
			return nil, err
		}
		for _, r := range roleAcked {
			ackedByID[r.AnnouncementID] = r.N
		}
	}
	// dept:<d> audiences — one query per distinct department.
	for deptName := range deptWanted {
		filter := announcementAudienceDeptPrefix + deptName
		var deptAcked []countRow
		if err := g.Table("announcement_acks AS a").
			Select("a.announcement_id AS announcement_id, COUNT(DISTINCT a.staff_id) AS n").
			Joins("JOIN staff s ON s.id = a.staff_id AND s.business_id = a.business_id AND s.is_active = ?", true).
			Joins("JOIN announcements ann ON ann.id = a.announcement_id AND ann.business_id = a.business_id AND ann.audience_filter = ?", filter).
			Joins("JOIN staff_positions sp ON sp.staff_id = s.id AND sp.business_id = s.business_id").
			Joins("JOIN positions p ON p.id = sp.position_id AND p.business_id = sp.business_id AND p.is_active = ? AND p.department = ?", true, deptName).
			Where("a.business_id = ? AND a.announcement_id IN ?", businessID, presentIDs).
			Group("a.announcement_id").
			Scan(&deptAcked).Error; err != nil {
			return nil, err
		}
		for _, r := range deptAcked {
			ackedByID[r.AnnouncementID] = r.N
		}
	}

	// Q3: first acker names per announcement — one bounded join ordered so the
	// earliest acks come first; we take firstNamesLimit per announcement in Go.
	firstNames := make(map[uint][]string, len(presentIDs))
	if firstNamesLimit > 0 {
		type nameRow struct {
			AnnouncementID uint
			Name           string
		}
		var rows []nameRow
		if err := g.Table("announcement_acks AS a").
			Select("a.announcement_id AS announcement_id, s.name AS name").
			Joins("JOIN staff s ON s.id = a.staff_id AND s.business_id = a.business_id").
			Where("a.business_id = ? AND a.announcement_id IN ?", businessID, presentIDs).
			Order("a.announcement_id ASC, a.acknowledged_at ASC, a.staff_id ASC").
			Limit(len(presentIDs) * (firstNamesLimit + 1)). // bounded: a few per announcement
			Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			if len(firstNames[r.AnnouncementID]) < firstNamesLimit {
				firstNames[r.AnnouncementID] = append(firstNames[r.AnnouncementID], r.Name)
			}
		}
	}

	// Total-eligible per audience family (each is one bounded query, added only
	// when that family is actually present in the requested set).
	totalAll := 0
	if needAll {
		var n int64
		if err := g.Model(&Staff{}).
			Where("business_id = ? AND is_active = ?", businessID, true).
			Count(&n).Error; err != nil {
			return nil, err
		}
		totalAll = int(n)
	}
	totalByRole := map[string]int{}
	if len(roleWanted) > 0 {
		roles := make([]string, 0, len(roleWanted))
		for r := range roleWanted {
			roles = append(roles, r)
		}
		type roleCount struct {
			Role string
			N    int
		}
		var rows []roleCount
		if err := g.Model(&Staff{}).
			Select("role, COUNT(*) AS n").
			Where("business_id = ? AND is_active = ? AND role IN ?", businessID, true, roles).
			Group("role").Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			totalByRole[r.Role] = r.N
		}
	}
	totalByDept := map[string]int{}
	if len(deptWanted) > 0 {
		depts := make([]string, 0, len(deptWanted))
		for dt := range deptWanted {
			depts = append(depts, dt)
		}
		type deptCount struct {
			Department string
			N          int
		}
		var rows []deptCount
		if err := g.Table("staff AS s").
			Select("p.department AS department, COUNT(DISTINCT s.id) AS n").
			Joins("JOIN staff_positions sp ON sp.staff_id = s.id AND sp.business_id = s.business_id").
			Joins("JOIN positions p ON p.id = sp.position_id AND p.business_id = sp.business_id").
			Where("s.business_id = ? AND s.is_active = ? AND p.is_active = ? AND p.department IN ?",
				businessID, true, true, depts).
			Group("p.department").Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			totalByDept[r.Department] = r.N
		}
	}

	for _, id := range presentIDs {
		filter := audienceByID[id]
		total := 0
		switch {
		case strings.HasPrefix(filter, announcementAudienceRolePrefix):
			total = totalByRole[strings.TrimPrefix(filter, announcementAudienceRolePrefix)]
		case strings.HasPrefix(filter, announcementAudienceDeptPrefix):
			total = totalByDept[strings.TrimPrefix(filter, announcementAudienceDeptPrefix)]
		default:
			total = totalAll
		}
		names := firstNames[id]
		if names == nil {
			names = []string{}
		}
		out[id] = AnnouncementAckSummary{
			AnnouncementID:  id,
			Acked:           ackedByID[id],
			TotalEligible:   total,
			FirstAckerNames: names,
		}
	}
	return out, nil
}

// ListAnnouncementAudienceStaffIDs returns the active staff ids an announcement
// targets, resolving its audience_filter ("all" | "role:<r>" | "dept:<d>") with
// the SAME predicate GetAnnouncementAckStatus uses. Targeting only — ids, no ack
// state. Caller excludes the author if desired.
func (d *DB) ListAnnouncementAudienceStaffIDs(businessID, announcementID uint) ([]uint, error) {
	var ann Announcement
	if err := d.GetGorm().
		Select("id, business_id, audience_filter").
		Where("id = ? AND business_id = ?", announcementID, businessID).
		First(&ann).Error; err != nil {
		return nil, err
	}
	q := d.GetGorm().Table("staff AS s").
		Select("s.id").
		Where("s.business_id = ? AND s.is_active = ?", businessID, true)
	switch {
	case strings.HasPrefix(ann.AudienceFilter, announcementAudienceRolePrefix):
		q = q.Where("s.role = ?", strings.TrimPrefix(ann.AudienceFilter, announcementAudienceRolePrefix))
	case strings.HasPrefix(ann.AudienceFilter, announcementAudienceDeptPrefix):
		dept := strings.TrimPrefix(ann.AudienceFilter, announcementAudienceDeptPrefix)
		q = q.Where("EXISTS (SELECT 1 FROM staff_positions sp JOIN positions p ON p.id = sp.position_id AND p.business_id = sp.business_id WHERE sp.staff_id = s.id AND sp.business_id = ? AND p.is_active = ? AND p.department = ?)",
			businessID, true, dept)
	default:
		// "all" — every active staff member; no extra predicate.
	}
	var ids []uint
	err := q.Scan(&ids).Error
	return ids, err
}
