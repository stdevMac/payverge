package database

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ---- Slice 9: engagement status/discriminator constants ----
const (
	ChecklistKindOnboarding = "onboarding"
	ChecklistKindOpening    = "opening"
	ChecklistKindClosing    = "closing"
	ChecklistKindCustom     = "custom"

	ChecklistRunPending    = "pending"
	ChecklistRunInProgress = "in_progress"
	ChecklistRunComplete   = "complete"

	PollStatusOpen   = "open"
	PollStatusClosed = "closed"

	ShoutoutVisibilityTeam    = "team"
	ShoutoutVisibilityPrivate = "private"
)

// ChecklistTemplate is the single engine for onboarding AND opening/closing
// checklists; Kind discriminates. PositionID optionally scopes a template to a
// role. No money fields.
type ChecklistTemplate struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	BusinessID       uint      `gorm:"index;not null" json:"business_id"`
	Name             string    `gorm:"type:varchar(255);not null" json:"name"`
	Kind             string    `gorm:"type:varchar(16);not null;default:'custom'" json:"kind"`
	PositionID       *uint     `json:"position_id"`
	IsActive         bool      `gorm:"not null;default:true" json:"is_active"`
	CreatedByStaffID uint      `gorm:"not null" json:"created_by_staff_id"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (ChecklistTemplate) TableName() string { return "checklist_templates" }

type ChecklistItem struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	BusinessID uint      `gorm:"index;not null" json:"business_id"`
	TemplateID uint      `gorm:"index;not null" json:"template_id"`
	Label      string    `gorm:"type:varchar(500);not null" json:"label"`
	SortOrder  int       `gorm:"not null;default:0" json:"sort_order"`
	IsRequired bool      `gorm:"not null;default:false" json:"is_required"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (ChecklistItem) TableName() string { return "checklist_items" }

// ChecklistRun is one instance to complete. AssignedStaffID is set for
// onboarding runs (per new hire); ShiftID is set for opening/closing runs (per
// shift). Status walks pending → in_progress → complete (see consts).
type ChecklistRun struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	BusinessID      uint       `gorm:"index;not null" json:"business_id"`
	TemplateID      uint       `gorm:"index;not null" json:"template_id"`
	AssignedStaffID *uint      `gorm:"index:idx_checklist_runs_assigned" json:"assigned_staff_id"`
	ShiftID         *uint      `gorm:"index:idx_checklist_runs_shift" json:"shift_id"`
	ForDate         time.Time  `gorm:"not null" json:"for_date"`
	Status          string     `gorm:"type:varchar(16);not null;default:'pending'" json:"status"`
	CompletedAt     *time.Time `json:"completed_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (ChecklistRun) TableName() string { return "checklist_runs" }

// ChecklistItemCompletion is one row per (run, item), created up-front at
// instantiation. Ticking flips Done. The unique idx_run_item keeps it 1:1.
type ChecklistItemCompletion struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	BusinessID  uint       `gorm:"index;not null" json:"business_id"`
	RunID       uint       `gorm:"not null;uniqueIndex:idx_run_item" json:"run_id"`
	ItemID      uint       `gorm:"not null;uniqueIndex:idx_run_item" json:"item_id"`
	StaffID     uint       `gorm:"not null;default:0" json:"staff_id"`
	Done        bool       `gorm:"not null;default:false" json:"done"`
	Note        string     `gorm:"type:text;not null;default:''" json:"note"`
	CompletedAt *time.Time `json:"completed_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (ChecklistItemCompletion) TableName() string { return "checklist_item_completions" }

// Document is a versioned policy/handbook. URL points at the existing protected
// S3 bucket OR Content holds an inline body (v1 simplification — no new upload
// handler). Bumping Version re-opens acknowledgement (DocumentAck is keyed by
// version). No money fields.
type Document struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	BusinessID       uint      `gorm:"index;not null" json:"business_id"`
	CreatedByStaffID uint      `gorm:"not null" json:"created_by_staff_id"`
	Title            string    `gorm:"type:varchar(255);not null" json:"title"`
	URL              string    `gorm:"type:varchar(1024);not null;default:''" json:"url"`
	Content          string    `gorm:"type:text;not null;default:''" json:"content"`
	Version          int       `gorm:"not null;default:1" json:"version"`
	RequireAck       bool      `gorm:"not null;default:false" json:"require_ack"`
	AudienceFilter   string    `gorm:"type:varchar(64);not null;default:'all'" json:"audience_filter"`
	IsActive         bool      `gorm:"not null;default:true" json:"is_active"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (Document) TableName() string { return "documents" }

// DocumentAck is proof-of-read. The unique idx_doc_ack on (document_id,
// staff_id, version) makes ack idempotent for a given version AND lets a new
// version legitimately record a fresh ack row.
type DocumentAck struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	BusinessID     uint      `gorm:"index;not null" json:"business_id"`
	DocumentID     uint      `gorm:"not null;uniqueIndex:idx_doc_ack" json:"document_id"`
	StaffID        uint      `gorm:"not null;uniqueIndex:idx_doc_ack" json:"staff_id"`
	Version        int       `gorm:"not null;uniqueIndex:idx_doc_ack" json:"version"`
	AcknowledgedAt time.Time `json:"acknowledged_at"`
	CreatedAt      time.Time `json:"created_at"`
}

func (DocumentAck) TableName() string { return "document_acks" }

// Shoutout is peer recognition. Visibility team feeds the public ShoutoutsFeed;
// private is sender↔recipient only. No money fields.
type Shoutout struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	BusinessID  uint      `gorm:"index;not null" json:"business_id"`
	FromStaffID uint      `gorm:"not null" json:"from_staff_id"`
	ToStaffID   uint      `gorm:"index:idx_shoutouts_to_staff;not null" json:"to_staff_id"`
	Message     string    `gorm:"type:varchar(500);not null" json:"message"`
	Emoji       string    `gorm:"type:varchar(16);not null;default:''" json:"emoji"`
	Visibility  string    `gorm:"type:varchar(16);not null;default:'team'" json:"visibility"`
	CreatedAt   time.Time `json:"created_at"`
}

func (Shoutout) TableName() string { return "shoutouts" }

type Poll struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	BusinessID     uint       `gorm:"index;not null" json:"business_id"`
	AuthorStaffID  uint       `gorm:"not null" json:"author_staff_id"`
	Question       string     `gorm:"type:varchar(500);not null" json:"question"`
	IsAnonymous    bool       `gorm:"not null;default:false" json:"is_anonymous"`
	AudienceFilter string     `gorm:"type:varchar(64);not null;default:'all'" json:"audience_filter"`
	Status         string     `gorm:"type:varchar(16);not null;default:'open'" json:"status"`
	ClosesAt       *time.Time `json:"closes_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (Poll) TableName() string { return "polls" }

type PollOption struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	BusinessID uint      `gorm:"index;not null" json:"business_id"`
	PollID     uint      `gorm:"index;not null" json:"poll_id"`
	Label      string    `gorm:"type:varchar(255);not null" json:"label"`
	SortOrder  int       `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt  time.Time `json:"created_at"`
}

func (PollOption) TableName() string { return "poll_options" }

// PollVote: the unique idx_poll_one_vote on (poll_id, staff_id) enforces exactly
// one vote per staff per poll. StaffID is stored for dedupe + (non-anon) voter
// lists, but is NEVER selected into an anonymous poll's result set (see
// PollResults). No money fields.
type PollVote struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	BusinessID uint      `gorm:"index;not null" json:"business_id"`
	PollID     uint      `gorm:"not null;uniqueIndex:idx_poll_one_vote" json:"poll_id"`
	OptionID   uint      `gorm:"index:idx_poll_votes_option;not null" json:"option_id"`
	StaffID    uint      `gorm:"not null;uniqueIndex:idx_poll_one_vote" json:"staff_id"`
	CreatedAt  time.Time `json:"created_at"`
}

func (PollVote) TableName() string { return "poll_votes" }

// ---- Slice 9.3: checklist service ----

var (
	ErrChecklistTemplateNotFound = errors.New("checklist template not found")
	ErrChecklistRunNotFound      = errors.New("checklist run not found")
	ErrChecklistItemNotFound     = errors.New("checklist item not found")
	// ErrInvalidChecklistAssignment is returned when a run's assigned staff or
	// shift does not belong to the business (handler → 400).
	ErrInvalidChecklistAssignment = errors.New("checklist assignment target not in business")
)

// staffInBusiness reports whether a staff row with that id belongs to the
// business (count-based; does not load the row).
func (d *DB) staffInBusiness(businessID, staffID uint) (bool, error) {
	var n int64
	if err := d.GetGorm().Model(&Staff{}).
		Where("id = ? AND business_id = ?", staffID, businessID).
		Limit(1).Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

// shiftInBusiness reports whether a shift row with that id belongs to the
// business (count-based; does not load the row).
func (d *DB) shiftInBusiness(businessID, shiftID uint) (bool, error) {
	var n int64
	if err := d.GetGorm().Model(&Shift{}).
		Where("id = ? AND business_id = ?", shiftID, businessID).
		Limit(1).Count(&n).Error; err != nil {
		return false, err
	}
	return n > 0, nil
}

// InstantiateChecklistRun materializes a template into a run + one completion
// row per item, all in one transaction. Onboarding callers pass assignedStaffID;
// opening/closing callers pass shiftID. The run starts pending.
func (d *DB) InstantiateChecklistRun(businessID, templateID uint, assignedStaffID, shiftID *uint, forDate time.Time) (*ChecklistRun, error) {
	var tpl ChecklistTemplate
	if err := d.GetGorm().
		Where("id = ? AND business_id = ? AND is_active = ?", templateID, businessID, true).
		First(&tpl).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrChecklistTemplateNotFound
		}
		return nil, err
	}
	// Reject assignment targets that do not belong to the business.
	if assignedStaffID != nil {
		ok, err := d.staffInBusiness(businessID, *assignedStaffID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrInvalidChecklistAssignment
		}
	}
	if shiftID != nil {
		ok, err := d.shiftInBusiness(businessID, *shiftID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrInvalidChecklistAssignment
		}
	}
	// Idempotency: an existing run for the same (template, assignee) or (template,
	// shift) is returned as-is rather than creating a duplicate run + completions.
	if assignedStaffID != nil {
		var existing ChecklistRun
		err := d.GetGorm().
			Where("business_id = ? AND template_id = ? AND assigned_staff_id = ?", businessID, templateID, *assignedStaffID).
			First(&existing).Error
		if err == nil {
			return &existing, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	if shiftID != nil {
		var existing ChecklistRun
		err := d.GetGorm().
			Where("business_id = ? AND template_id = ? AND shift_id = ?", businessID, templateID, *shiftID).
			First(&existing).Error
		if err == nil {
			return &existing, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}
	var items []ChecklistItem
	if err := d.GetGorm().
		Select("id").
		Where("template_id = ? AND business_id = ?", templateID, businessID).
		Order("sort_order asc, id asc").
		Find(&items).Error; err != nil {
		return nil, err
	}
	run := &ChecklistRun{
		BusinessID: businessID, TemplateID: templateID,
		AssignedStaffID: assignedStaffID, ShiftID: shiftID,
		ForDate: forDate, Status: ChecklistRunPending,
	}
	err := d.GetGorm().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(run).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		comps := make([]ChecklistItemCompletion, 0, len(items))
		for _, it := range items {
			comps = append(comps, ChecklistItemCompletion{
				BusinessID: businessID, RunID: run.ID, ItemID: it.ID,
			})
		}
		return tx.Create(&comps).Error
	})
	if err != nil {
		return nil, err
	}
	return run, nil
}

// TickChecklistItem flips a completion's Done flag (idempotent — the row already
// exists from instantiation) then recomputes the run status from REQUIRED items
// only: complete when every required item is done, in_progress when any item is
// done, else pending. Tenant-guarded. Ownership is enforced in the HANDLER (own
// run vs manager), not here.
func (d *DB) TickChecklistItem(businessID, runID, itemID, staffID uint, done bool, note string) (*ChecklistRun, error) {
	var run ChecklistRun
	if err := d.GetGorm().
		Where("id = ? AND business_id = ?", runID, businessID).
		First(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrChecklistRunNotFound
		}
		return nil, err
	}
	err := d.GetGorm().Transaction(func(tx *gorm.DB) error {
		set := map[string]interface{}{"done": done, "staff_id": staffID, "note": note}
		if done {
			now := time.Now().UTC()
			set["completed_at"] = now
		} else {
			set["completed_at"] = nil
		}
		res := tx.Model(&ChecklistItemCompletion{}).
			Where("run_id = ? AND item_id = ? AND business_id = ?", runID, itemID, businessID).
			Updates(set)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrChecklistItemNotFound
		}
		// Recompute status from required items via SQL aggregates (no N+1).
		var requiredTotal, requiredDone int64
		if err := tx.Model(&ChecklistItemCompletion{}).
			Joins("JOIN checklist_items ci ON ci.id = checklist_item_completions.item_id").
			Where("checklist_item_completions.run_id = ? AND ci.is_required = ?", runID, true).
			Count(&requiredTotal).Error; err != nil {
			return err
		}
		if err := tx.Model(&ChecklistItemCompletion{}).
			Joins("JOIN checklist_items ci ON ci.id = checklist_item_completions.item_id").
			Where("checklist_item_completions.run_id = ? AND ci.is_required = ? AND checklist_item_completions.done = ?", runID, true, true).
			Count(&requiredDone).Error; err != nil {
			return err
		}
		var anyDone int64
		if err := tx.Model(&ChecklistItemCompletion{}).
			Where("run_id = ? AND done = ?", runID, true).Count(&anyDone).Error; err != nil {
			return err
		}
		var totalItems int64
		if err := tx.Model(&ChecklistItemCompletion{}).
			Where("run_id = ?", runID).Count(&totalItems).Error; err != nil {
			return err
		}
		newStatus := ChecklistRunPending
		var completedAt *time.Time
		switch {
		case requiredTotal > 0 && requiredDone >= requiredTotal:
			newStatus = ChecklistRunComplete
			now := time.Now().UTC()
			completedAt = &now
		case requiredTotal == 0 && totalItems > 0 && anyDone >= totalItems:
			// All-optional template: complete once every item is ticked.
			newStatus = ChecklistRunComplete
			now := time.Now().UTC()
			completedAt = &now
		case anyDone > 0:
			newStatus = ChecklistRunInProgress
		}
		run.Status, run.CompletedAt = newStatus, completedAt
		return tx.Model(&ChecklistRun{}).Where("id = ?", runID).
			Updates(map[string]interface{}{"status": newStatus, "completed_at": completedAt}).Error
	})
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// ---- Slice 9.4: checklist template/run list + ownership ----

// CreateChecklistTemplate inserts a template + its items in one tx. A
// position-scoped template (PositionID set) must reference an active position in
// the business (else assertPositionInBusiness's error surfaces, handler → 400).
func (d *DB) CreateChecklistTemplate(tpl *ChecklistTemplate, items []ChecklistItem) error {
	if tpl.PositionID != nil {
		if err := d.assertPositionInBusiness(tpl.BusinessID, *tpl.PositionID); err != nil {
			return err
		}
	}
	return d.GetGorm().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(tpl).Error; err != nil {
			return err
		}
		for i := range items {
			items[i].BusinessID = tpl.BusinessID
			items[i].TemplateID = tpl.ID
		}
		if len(items) > 0 {
			return tx.Create(&items).Error
		}
		return nil
	})
}

// ListChecklistTemplates: narrow projection, active-only, bounded.
func (d *DB) ListChecklistTemplates(businessID uint, limit int) ([]ChecklistTemplate, error) {
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	var out []ChecklistTemplate
	err := d.GetGorm().
		Select("id", "business_id", "name", "kind", "position_id", "is_active", "created_by_staff_id", "created_at", "updated_at").
		Where("business_id = ? AND is_active = ?", businessID, true).
		Order("kind asc, name asc").Limit(limit).Find(&out).Error
	return out, err
}

// ListChecklistRunsForStaff returns a staff member's own runs (onboarding
// assigned to them) — bounded, narrow projection, newest first.
// CountPendingChecklistRuns returns how many of a staff member's own runs are not
// yet complete (pending + in_progress) — the badge count for the buried checklists
// surface. One indexed COUNT over idx_checklist_runs_assigned, tenant-scoped.
func (d *DB) CountPendingChecklistRuns(businessID, staffID uint) (int64, error) {
	if businessID == 0 || staffID == 0 {
		return 0, nil
	}
	var n int64
	err := d.GetGorm().Model(&ChecklistRun{}).
		Where("business_id = ? AND assigned_staff_id = ? AND status <> ?", businessID, staffID, ChecklistRunComplete).
		Count(&n).Error
	return n, err
}

func (d *DB) ListChecklistRunsForStaff(businessID, staffID uint, limit int) ([]ChecklistRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	var out []ChecklistRun
	err := d.GetGorm().
		Select("id", "business_id", "template_id", "assigned_staff_id", "shift_id", "for_date", "status", "completed_at", "created_at", "updated_at").
		Where("business_id = ? AND assigned_staff_id = ?", businessID, staffID).
		Order("for_date desc, id desc").Limit(limit).Find(&out).Error
	return out, err
}

// ChecklistRunView is a run enriched with the display context an operator needs
// to read the business-wide run list at a glance: the template name and the
// assignee's name (empty for a shift-based run). No per-item detail — that stays
// behind GetChecklistRunDetail.
type ChecklistRunView struct {
	ChecklistRun
	TemplateName      string `json:"template_name"`
	AssignedStaffName string `json:"assigned_staff_name"`
}

// ListChecklistRunsForBusiness returns EVERY run in the business (the operator
// run-status list — unlike ListChecklistRunsForStaff which is one staffer's own
// runs), newest first, bounded, with the template + assignee names joined in ONE
// query (LEFT JOIN staff so a shift-based/unassigned run still returns). Narrow
// projection, tenant-scoped. Managers/owners consume this to see who has and
// hasn't finished their assigned checklists.
func (d *DB) ListChecklistRunsForBusiness(businessID uint, limit int) ([]ChecklistRunView, error) {
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	var out []ChecklistRunView
	err := d.GetGorm().
		Table("checklist_runs AS r").
		Select("r.id, r.business_id, r.template_id, r.assigned_staff_id, r.shift_id, r.for_date, r.status, r.completed_at, r.created_at, r.updated_at, "+
			"t.name AS template_name, COALESCE(s.name, '') AS assigned_staff_name").
		Joins("JOIN checklist_templates t ON t.id = r.template_id AND t.business_id = r.business_id").
		Joins("LEFT JOIN staff s ON s.id = r.assigned_staff_id AND s.business_id = r.business_id").
		Where("r.business_id = ?", businessID).
		Order("r.for_date desc, r.id desc").
		Limit(limit).
		Scan(&out).Error
	return out, err
}

// RunOwnedByStaff reports whether a run is assigned to staffID. For shift-based
// runs (assigned_staff_id NULL) this returns false — opening/closing ticking by
// shift membership is gated by the manager bypass + (future) Slice-2 shift join.
func (d *DB) RunOwnedByStaff(businessID, runID, staffID uint) (bool, error) {
	var run ChecklistRun
	if err := d.GetGorm().Select("assigned_staff_id").
		Where("id = ? AND business_id = ?", runID, businessID).First(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, ErrChecklistRunNotFound
		}
		return false, err
	}
	return run.AssignedStaffID != nil && *run.AssignedStaffID == staffID, nil
}

// ChecklistRunItem is one item on a run with its per-run completion state, shaped
// for the run-detail read that lets staff tick items.
type ChecklistRunItem struct {
	ItemID      uint       `json:"item_id"`
	Label       string     `json:"label"`
	IsRequired  bool       `json:"is_required"`
	SortOrder   int        `json:"sort_order"`
	Done        bool       `json:"done"`
	Note        string     `json:"note"`
	CompletedAt *time.Time `json:"completed_at"`
}

// ChecklistRunDetail is a run plus its items + completion state, in sort order.
type ChecklistRunDetail struct {
	Run   ChecklistRun       `json:"run"`
	Items []ChecklistRunItem `json:"items"`
}

// GetChecklistRunDetail loads a run (tenant-scoped, else ErrChecklistRunNotFound)
// plus its items + completion state in ONE join query (no per-item reload). Items
// come back in sort order, bounded. Ownership is enforced in the HANDLER.
func (d *DB) GetChecklistRunDetail(businessID, runID uint) (*ChecklistRunDetail, error) {
	var run ChecklistRun
	if err := d.GetGorm().
		Select("id", "business_id", "template_id", "assigned_staff_id", "shift_id", "for_date", "status", "completed_at", "created_at", "updated_at").
		Where("id = ? AND business_id = ?", runID, businessID).
		First(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrChecklistRunNotFound
		}
		return nil, err
	}
	var items []ChecklistRunItem
	if err := d.GetGorm().
		Table("checklist_item_completions AS cc").
		Select("ci.id AS item_id, ci.label AS label, ci.is_required AS is_required, ci.sort_order AS sort_order, cc.done AS done, cc.note AS note, cc.completed_at AS completed_at").
		Joins("JOIN checklist_items ci ON ci.id = cc.item_id").
		Where("cc.run_id = ? AND cc.business_id = ?", runID, businessID).
		Order("ci.sort_order asc, ci.id asc").
		Limit(500).
		Find(&items).Error; err != nil {
		return nil, err
	}
	return &ChecklistRunDetail{Run: run, Items: items}, nil
}

// ---- Slice 9.6: documents service (versioned ack) ----

var ErrDocumentNotFound = errors.New("document not found")

// callerAudiences resolves the audience tags a non-manager caller is targeted by:
// "all" always, "role:<role>" when role != "", and "dept:<d>" for each department
// the caller holds an active position in. Mirrors ListAnnouncements (one bounded
// distinct-department query) — no N+1. Shared by ListDocuments + ListPolls so a
// manager-only document/poll never leaks to line staff once surfaced.
func (d *DB) callerAudiences(businessID, staffID uint, role string) ([]string, error) {
	var depts []string
	if err := d.GetGorm().Table("staff_positions AS sp").
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
	return audiences, nil
}

// CreateDocument validates + canonicalizes the audience filter (empty → "all",
// malformed → ErrInvalidAudienceFilter) before persisting.
func (d *DB) CreateDocument(doc *Document) error {
	filter, err := normalizeAudienceFilter(doc.AudienceFilter)
	if err != nil {
		return err
	}
	doc.AudienceFilter = filter
	return d.GetGorm().Create(doc).Error
}

// ListDocuments: narrow projection, active-only, bounded. Content is included
// (docs are short policy bodies); for large bodies switch to a detail fetch.
// Managers/owners (isManager) see every document to manage them; line staff see
// only documents whose audience_filter matches "all", their role, or one of their
// departments (one extra bounded IN predicate — no N+1).
func (d *DB) ListDocuments(businessID, staffID uint, role string, isManager bool, limit int) ([]Document, error) {
	if limit <= 0 || limit > 200 {
		limit = 200
	}
	q := d.GetGorm().
		Select("id", "business_id", "created_by_staff_id", "title", "url", "content", "version", "require_ack", "audience_filter", "is_active", "created_at", "updated_at").
		Where("business_id = ? AND is_active = ?", businessID, true)
	if !isManager {
		audiences, err := d.callerAudiences(businessID, staffID, role)
		if err != nil {
			return nil, err
		}
		q = q.Where("audience_filter IN ?", audiences)
	}
	var out []Document
	err := q.Order("created_at desc").Limit(limit).Find(&out).Error
	return out, err
}

// UpdateDocument bumps a document to the next version (re-opening acknowledgement,
// since DocumentAck is keyed by version). The active doc is loaded tenant-scoped
// (else ErrDocumentNotFound); the audience filter is validated/canonicalized
// (malformed → ErrInvalidAudienceFilter). Version is incremented; title/url/
// content/require_ack/audience_filter are replaced.
func (d *DB) UpdateDocument(businessID, documentID uint, title, url, content string, requireAck bool, audienceFilter string) (*Document, error) {
	filter, err := normalizeAudienceFilter(audienceFilter)
	if err != nil {
		return nil, err
	}
	var doc Document
	if err := d.GetGorm().
		Where("id = ? AND business_id = ? AND is_active = ?", documentID, businessID, true).
		First(&doc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDocumentNotFound
		}
		return nil, err
	}
	newVersion := doc.Version + 1
	if err := d.GetGorm().Model(&Document{}).
		Where("id = ? AND business_id = ?", documentID, businessID).
		Updates(map[string]interface{}{
			"title": title, "url": url, "content": content,
			"require_ack": requireAck, "audience_filter": filter, "version": newVersion,
		}).Error; err != nil {
		return nil, err
	}
	doc.Title, doc.URL, doc.Content = title, url, content
	doc.RequireAck, doc.AudienceFilter, doc.Version = requireAck, filter, newVersion
	return &doc, nil
}

// DocumentsAckedBy returns the per-caller ack state for the given bounded doc
// list: acked[id]=true only when the staff member has an ack row whose version
// equals that document's CURRENT version (a stale ack from an older version does
// NOT satisfy the current one — that is how a version bump re-opens ack). It is
// ONE query over document_acks; an empty/zero staffID or empty list yields an
// empty map (no N+1).
func (d *DB) DocumentsAckedBy(businessID, staffID uint, docs []Document) (map[uint]bool, error) {
	out := make(map[uint]bool, len(docs))
	if staffID == 0 || len(docs) == 0 {
		return out, nil
	}
	ids := make([]uint, 0, len(docs))
	currentVersion := make(map[uint]int, len(docs))
	for _, doc := range docs {
		ids = append(ids, doc.ID)
		currentVersion[doc.ID] = doc.Version
	}
	type ackRow struct {
		DocumentID uint
		Version    int
	}
	var rows []ackRow
	if err := d.GetGorm().Model(&DocumentAck{}).
		Select("document_id", "version").
		Where("business_id = ? AND staff_id = ? AND document_id IN ?", businessID, staffID, ids).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		if v, ok := currentVersion[r.DocumentID]; ok && v == r.Version {
			out[r.DocumentID] = true
		}
	}
	return out, nil
}

// AckDocument records proof-of-read for the document's CURRENT version. The
// unique idx_doc_ack makes re-ack of the same version a no-op (OnConflict
// DoNothing), while a new version legitimately inserts a fresh row.
func (d *DB) AckDocument(businessID, documentID, staffID uint) error {
	var doc Document
	if err := d.GetGorm().Select("id", "version").
		Where("id = ? AND business_id = ? AND is_active = ?", documentID, businessID, true).
		First(&doc).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrDocumentNotFound
		}
		return err
	}
	ack := DocumentAck{
		BusinessID: businessID, DocumentID: documentID, StaffID: staffID,
		Version: doc.Version, AcknowledgedAt: time.Now().UTC(),
	}
	return d.GetGorm().Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "document_id"}, {Name: "staff_id"}, {Name: "version"}},
		DoNothing: true,
	}).Create(&ack).Error
}

// ---- Slice 9.8: shoutouts (recognition) service ----

var (
	// ErrShoutoutSelf is returned when a staff member shouts out themselves.
	ErrShoutoutSelf = errors.New("cannot shout out yourself")
	// ErrShoutoutRecipient is returned when the recipient is not in the business.
	ErrShoutoutRecipient = errors.New("shoutout recipient not in business")
)

// CreateShoutout rejects a self-shoutout and a recipient outside the business
// (both → 400 at the handler), then persists.
func (d *DB) CreateShoutout(s *Shoutout) error {
	if s.ToStaffID == s.FromStaffID {
		return ErrShoutoutSelf
	}
	ok, err := d.staffInBusiness(s.BusinessID, s.ToStaffID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrShoutoutRecipient
	}
	return d.GetGorm().Create(s).Error
}

// ListShoutouts returns the team feed plus any PRIVATE shoutout the caller sent
// or received. Narrow projection, bounded, newest first.
func (d *DB) ListShoutouts(businessID, staffID uint, limit int) ([]Shoutout, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	var out []Shoutout
	err := d.GetGorm().
		Select("id", "business_id", "from_staff_id", "to_staff_id", "message", "emoji", "visibility", "created_at").
		Where("business_id = ? AND (visibility = ? OR from_staff_id = ? OR to_staff_id = ?)",
			businessID, ShoutoutVisibilityTeam, staffID, staffID).
		Order("created_at desc").Limit(limit).Find(&out).Error
	return out, err
}

// ---- Slice 9.10: polls service (one-vote dedupe, anonymous results, close) ----

var (
	ErrPollNotFound      = errors.New("poll not found")
	ErrPollClosed        = errors.New("poll is closed")
	ErrPollOptionInvalid = errors.New("option does not belong to poll")
	ErrAlreadyVoted      = errors.New("already voted")
)

// PollOptionResult: Votes is always present; Voters is populated ONLY for
// non-anonymous polls (nil + `omitempty` for anon, so it never serializes).
type PollOptionResult struct {
	OptionID uint   `json:"option_id"`
	Label    string `json:"label"`
	Votes    int64  `json:"votes"`
	Voters   []uint `json:"voters,omitempty"`
}
type PollResultView struct {
	PollID      uint               `json:"poll_id"`
	Question    string             `json:"question"`
	IsAnonymous bool               `json:"is_anonymous"`
	Status      string             `json:"status"`
	Options     []PollOptionResult `json:"options"`
}

// CreatePoll validates + canonicalizes the audience filter (empty → "all",
// malformed → ErrInvalidAudienceFilter) before persisting the poll + options.
func (d *DB) CreatePoll(p *Poll, options []PollOption) error {
	filter, err := normalizeAudienceFilter(p.AudienceFilter)
	if err != nil {
		return err
	}
	p.AudienceFilter = filter
	return d.GetGorm().Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(p).Error; err != nil {
			return err
		}
		for i := range options {
			options[i].BusinessID = p.BusinessID
			options[i].PollID = p.ID
		}
		if len(options) > 0 {
			return tx.Create(&options).Error
		}
		return nil
	})
}

// ListPolls: narrow projection, bounded. Managers/owners (isManager) see every
// poll to manage them; line staff see only polls whose audience_filter matches
// "all", their role, or one of their departments (one extra bounded IN predicate
// — no N+1).
func (d *DB) ListPolls(businessID, staffID uint, role string, isManager bool, limit int) ([]Poll, error) {
	if limit <= 0 || limit > 100 {
		limit = 100
	}
	q := d.GetGorm().
		Select("id", "business_id", "author_staff_id", "question", "is_anonymous", "audience_filter", "status", "closes_at", "created_at", "updated_at").
		Where("business_id = ?", businessID)
	if !isManager {
		audiences, err := d.callerAudiences(businessID, staffID, role)
		if err != nil {
			return nil, err
		}
		q = q.Where("audience_filter IN ?", audiences)
	}
	var out []Poll
	if err := q.Order("status asc, created_at desc").Limit(limit).Find(&out).Error; err != nil {
		return nil, err
	}
	// A poll past its close time reads as closed without a write.
	for i := range out {
		out[i].Status = effectivePollStatus(out[i].Status, out[i].ClosesAt)
	}
	return out, nil
}

// effectivePollStatus reports the poll's status as a reader sees it: an open poll
// whose closes_at is in the past reads as "closed" without a DB write (the row
// stays open until ClosePoll or a sweeper flips it). Any non-open status, or an
// open poll with no/future close time, is returned unchanged.
func effectivePollStatus(status string, closesAt *time.Time) string {
	if status == PollStatusOpen && closesAt != nil && closesAt.Before(time.Now().UTC()) {
		return PollStatusClosed
	}
	return status
}

// CastVote enforces poll-open (status AND closes_at) + option-belongs-to-poll,
// then one-vote-per-staff via the unique idx_poll_one_vote. OnConflict DoNothing
// makes the dup a no-op across SQLite + Postgres; RowsAffected==0 →
// ErrAlreadyVoted (handler → 409).
func (d *DB) CastVote(businessID, pollID, optionID, staffID uint) error {
	var poll Poll
	if err := d.GetGorm().Select("id", "status", "closes_at").
		Where("id = ? AND business_id = ?", pollID, businessID).First(&poll).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPollNotFound
		}
		return err
	}
	if poll.Status != PollStatusOpen {
		return ErrPollClosed
	}
	if poll.ClosesAt != nil && poll.ClosesAt.Before(time.Now().UTC()) {
		return ErrPollClosed
	}
	var optCount int64
	if err := d.GetGorm().Model(&PollOption{}).
		Where("id = ? AND poll_id = ? AND business_id = ?", optionID, pollID, businessID).
		Count(&optCount).Error; err != nil {
		return err
	}
	if optCount == 0 {
		return ErrPollOptionInvalid
	}
	res := d.GetGorm().Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "poll_id"}, {Name: "staff_id"}},
		DoNothing: true,
	}).Create(&PollVote{BusinessID: businessID, PollID: pollID, OptionID: optionID, StaffID: staffID})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrAlreadyVoted
	}
	return nil
}

// PollResults aggregates per-option counts. The COUNT(*) GROUP BY option_id
// aggregate NEVER selects staff_id, so an anonymous poll's result set is
// structurally incapable of exposing a voter. Voter ids are attached ONLY when
// the poll is non-anonymous.
func (d *DB) PollResults(businessID, pollID uint) (*PollResultView, error) {
	var poll Poll
	if err := d.GetGorm().Select("id", "question", "is_anonymous", "status", "closes_at").
		Where("id = ? AND business_id = ?", pollID, businessID).First(&poll).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPollNotFound
		}
		return nil, err
	}
	var options []PollOption
	if err := d.GetGorm().Select("id", "label").
		Where("poll_id = ? AND business_id = ?", pollID, businessID).
		Order("sort_order asc, id asc").Find(&options).Error; err != nil {
		return nil, err
	}
	// Voter-identity-free count aggregate.
	type tally struct {
		OptionID uint
		Votes    int64
	}
	var tallies []tally
	if err := d.GetGorm().Model(&PollVote{}).
		Select("option_id, COUNT(*) AS votes").
		Where("poll_id = ? AND business_id = ?", pollID, businessID).
		Group("option_id").Scan(&tallies).Error; err != nil {
		return nil, err
	}
	countByOption := map[uint]int64{}
	for _, t := range tallies {
		countByOption[t.OptionID] = t.Votes
	}
	// Voter ids ONLY for non-anonymous polls.
	votersByOption := map[uint][]uint{}
	if !poll.IsAnonymous {
		var votes []PollVote
		if err := d.GetGorm().Select("option_id", "staff_id").
			Where("poll_id = ? AND business_id = ?", pollID, businessID).
			Order("staff_id asc").Find(&votes).Error; err != nil {
			return nil, err
		}
		for _, v := range votes {
			votersByOption[v.OptionID] = append(votersByOption[v.OptionID], v.StaffID)
		}
	}
	view := &PollResultView{PollID: poll.ID, Question: poll.Question, IsAnonymous: poll.IsAnonymous, Status: effectivePollStatus(poll.Status, poll.ClosesAt)}
	for _, o := range options {
		view.Options = append(view.Options, PollOptionResult{
			OptionID: o.ID, Label: o.Label, Votes: countByOption[o.ID], Voters: votersByOption[o.ID],
		})
	}
	return view, nil
}

func (d *DB) ClosePoll(businessID, pollID uint) error {
	res := d.GetGorm().Model(&Poll{}).
		Where("id = ? AND business_id = ?", pollID, businessID).
		Updates(map[string]interface{}{"status": PollStatusClosed})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrPollNotFound
	}
	return nil
}

// ListPollOptionsForPolls fetches every option for the given polls in ONE query
// (no N+1). Bounded by the caller passing a bounded pollIDs slice.
func (d *DB) ListPollOptionsForPolls(businessID uint, pollIDs []uint) ([]PollOption, error) {
	if len(pollIDs) == 0 {
		return nil, nil
	}
	var out []PollOption
	err := d.GetGorm().
		Select("id", "business_id", "poll_id", "label", "sort_order", "created_at").
		Where("business_id = ? AND poll_id IN ?", businessID, pollIDs).
		Order("poll_id asc, sort_order asc, id asc").Find(&out).Error
	return out, err
}

// ListStaffVotesForPolls returns the caller's OWN votes across the given polls
// in ONE query (no N+1) — used to pre-select the UI. Returns only this staff's
// rows, so it never leaks other voters' identities.
func (d *DB) ListStaffVotesForPolls(businessID, staffID uint, pollIDs []uint) ([]PollVote, error) {
	if len(pollIDs) == 0 {
		return nil, nil
	}
	var out []PollVote
	err := d.GetGorm().
		Select("poll_id", "option_id").
		Where("business_id = ? AND staff_id = ? AND poll_id IN ?", businessID, staffID, pollIDs).
		Find(&out).Error
	return out, err
}
