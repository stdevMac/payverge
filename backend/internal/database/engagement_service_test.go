package database

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var engagementBenchSeq atomic.Uint64

// newEngagementTestDB migrates Business + Staff + the 10 engagement tables.
// The composite unique indexes (idx_run_item, idx_doc_ack, idx_poll_one_vote)
// are PLAIN composite uniques (no WHERE predicate), so GORM AutoMigrate
// materializes them from the `uniqueIndex:` tags — unlike Slice-0's PARTIAL
// idx_staff_one_primary which needed a raw Exec. No manual Exec needed here.
func newEngagementTestDB(t *testing.T) *DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(
		&Business{}, &Staff{}, &Position{}, &StaffPosition{}, &Shift{},
		&ChecklistTemplate{}, &ChecklistItem{}, &ChecklistRun{}, &ChecklistItemCompletion{},
		&Document{}, &DocumentAck{}, &Shoutout{}, &Poll{}, &PollOption{}, &PollVote{},
	))
	prev := db
	SetTestDB(gormDB)
	t.Cleanup(func() { SetTestDB(prev) })
	return GetDBWrapper()
}

func TestEngagementSchemaMaterializes(t *testing.T) {
	d := newEngagementTestDB(t)
	g := d.GetGorm()
	for _, tbl := range []string{
		"checklist_templates", "checklist_items", "checklist_runs", "checklist_item_completions",
		"documents", "document_acks", "shoutouts", "polls", "poll_options", "poll_votes",
	} {
		require.True(t, g.Migrator().HasTable(tbl), "table %s must materialize from struct tags", tbl)
	}
	// Per-table business_id secondary indexes (named to match the DDL exactly).
	require.True(t, g.Migrator().HasIndex(&ChecklistTemplate{}, "idx_checklist_templates_business_id"))
	require.True(t, g.Migrator().HasIndex(&ChecklistItem{}, "idx_checklist_items_business_id"))
	require.True(t, g.Migrator().HasIndex(&ChecklistItem{}, "idx_checklist_items_template_id"))
	require.True(t, g.Migrator().HasIndex(&ChecklistRun{}, "idx_checklist_runs_business_id"))
	require.True(t, g.Migrator().HasIndex(&ChecklistRun{}, "idx_checklist_runs_assigned"))
	require.True(t, g.Migrator().HasIndex(&ChecklistRun{}, "idx_checklist_runs_shift"))
	require.True(t, g.Migrator().HasIndex(&ChecklistItemCompletion{}, "idx_checklist_item_completions_business_id"))
	require.True(t, g.Migrator().HasIndex(&Document{}, "idx_documents_business_id"))
	require.True(t, g.Migrator().HasIndex(&DocumentAck{}, "idx_document_acks_business_id"))
	require.True(t, g.Migrator().HasIndex(&Shoutout{}, "idx_shoutouts_business_id"))
	require.True(t, g.Migrator().HasIndex(&Shoutout{}, "idx_shoutouts_to_staff"))
	require.True(t, g.Migrator().HasIndex(&Poll{}, "idx_polls_business_id"))
	require.True(t, g.Migrator().HasIndex(&PollOption{}, "idx_poll_options_business_id"))
	require.True(t, g.Migrator().HasIndex(&PollOption{}, "idx_poll_options_poll_id"))
	require.True(t, g.Migrator().HasIndex(&PollVote{}, "idx_poll_votes_business_id"))
	require.True(t, g.Migrator().HasIndex(&PollVote{}, "idx_poll_votes_option"))
	// Composite unique indexes (dedupe + versioned-ack + 1:1 completion).
	require.True(t, g.Migrator().HasIndex(&PollVote{}, "idx_poll_one_vote"))
	require.True(t, g.Migrator().HasIndex(&DocumentAck{}, "idx_doc_ack"))
	require.True(t, g.Migrator().HasIndex(&ChecklistItemCompletion{}, "idx_run_item"))

	// idx_run_item rejects a duplicate (run_id, item_id) completion (NIT-1).
	require.NoError(t, g.Create(&ChecklistItemCompletion{BusinessID: 1, RunID: 1, ItemID: 1}).Error)
	require.Error(t, g.Create(&ChecklistItemCompletion{BusinessID: 1, RunID: 1, ItemID: 1}).Error,
		"idx_run_item must reject a duplicate (run_id, item_id) completion")
}

// ---- Slice 9.3: checklist service tests ----

// ptrUint is shared with referral_record_json_test.go (same package).
func timeNow() time.Time { return time.Now().UTC() }

// seedStaff inserts an active staff member so checklist-assignment + shoutout
// validations (which now reject non-business targets) are satisfied.
func seedStaff(t *testing.T, biz, id uint) {
	t.Helper()
	require.NoError(t, db.Create(&Staff{ID: id, BusinessID: biz, Email: fmt.Sprintf("s%d@x.co", id), Name: fmt.Sprintf("S%d", id), Role: StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error)
}

func seedTemplate(t *testing.T, d *DB, biz uint) (uint, []uint) {
	t.Helper()
	tpl := &ChecklistTemplate{BusinessID: biz, Name: "Open", Kind: ChecklistKindOpening, CreatedByStaffID: 1, IsActive: true}
	require.NoError(t, d.GetGorm().Create(tpl).Error)
	a := &ChecklistItem{BusinessID: biz, TemplateID: tpl.ID, Label: "Lights", SortOrder: 0, IsRequired: true}
	b := &ChecklistItem{BusinessID: biz, TemplateID: tpl.ID, Label: "Music", SortOrder: 1, IsRequired: false}
	require.NoError(t, d.GetGorm().Create(a).Error)
	require.NoError(t, d.GetGorm().Create(b).Error)
	return tpl.ID, []uint{a.ID, b.ID}
}

func TestInstantiateChecklistRunCreatesCompletions(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	seedStaff(t, 1, 7)
	tplID, items := seedTemplate(t, d, 1)
	run, err := d.InstantiateChecklistRun(1, tplID, ptrUint(7), nil, timeNow())
	require.NoError(t, err)
	require.Equal(t, ChecklistRunPending, run.Status)
	var comps []ChecklistItemCompletion
	require.NoError(t, db.Where("run_id = ?", run.ID).Order("item_id asc").Find(&comps).Error)
	require.Len(t, comps, len(items))
	for _, c := range comps {
		require.False(t, c.Done)
	}
}

func TestTickFlipsRunCompleteWhenAllRequiredDone(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	seedStaff(t, 1, 7)
	tplID, items := seedTemplate(t, d, 1) // items[0]=required, items[1]=optional
	run, err := d.InstantiateChecklistRun(1, tplID, ptrUint(7), nil, timeNow())
	require.NoError(t, err)

	// Tick only the optional item → still in_progress (required one open).
	r1, err := d.TickChecklistItem(1, run.ID, items[1], 7, true, "")
	require.NoError(t, err)
	require.Equal(t, ChecklistRunInProgress, r1.Status)

	// Tick the required item → complete.
	r2, err := d.TickChecklistItem(1, run.ID, items[0], 7, true, "")
	require.NoError(t, err)
	require.Equal(t, ChecklistRunComplete, r2.Status)
	require.NotNil(t, r2.CompletedAt)

	// Idempotent re-tick of the same item: no error, stays complete.
	r3, err := d.TickChecklistItem(1, run.ID, items[0], 7, true, "")
	require.NoError(t, err)
	require.Equal(t, ChecklistRunComplete, r3.Status)
}

// TestTickAllOptionalChecklistCompletes (MIN-1): a template with ZERO required
// items completes once every (optional) item is ticked.
func TestTickAllOptionalChecklistCompletes(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	seedStaff(t, 1, 7)
	tpl := &ChecklistTemplate{BusinessID: 1, Name: "Closing", Kind: ChecklistKindClosing, CreatedByStaffID: 1, IsActive: true}
	require.NoError(t, db.Create(tpl).Error)
	a := &ChecklistItem{BusinessID: 1, TemplateID: tpl.ID, Label: "Wipe", SortOrder: 0, IsRequired: false}
	b := &ChecklistItem{BusinessID: 1, TemplateID: tpl.ID, Label: "Lock", SortOrder: 1, IsRequired: false}
	require.NoError(t, db.Create(a).Error)
	require.NoError(t, db.Create(b).Error)
	run, err := d.InstantiateChecklistRun(1, tpl.ID, ptrUint(7), nil, timeNow())
	require.NoError(t, err)

	r1, err := d.TickChecklistItem(1, run.ID, a.ID, 7, true, "")
	require.NoError(t, err)
	require.Equal(t, ChecklistRunInProgress, r1.Status, "one of two optional done → in_progress")

	r2, err := d.TickChecklistItem(1, run.ID, b.ID, 7, true, "")
	require.NoError(t, err)
	require.Equal(t, ChecklistRunComplete, r2.Status, "all optional items ticked → complete")
	require.NotNil(t, r2.CompletedAt)
}

// TestInstantiateChecklistRunIdempotent (MIN-2): instantiating twice for the same
// (template, assigned staff) returns the SAME run with no duplicated completions.
func TestInstantiateChecklistRunIdempotent(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	seedStaff(t, 1, 7)
	tplID, items := seedTemplate(t, d, 1)
	run1, err := d.InstantiateChecklistRun(1, tplID, ptrUint(7), nil, timeNow())
	require.NoError(t, err)
	run2, err := d.InstantiateChecklistRun(1, tplID, ptrUint(7), nil, timeNow())
	require.NoError(t, err)
	require.Equal(t, run1.ID, run2.ID, "second instantiate returns the existing run")

	var runCount, compCount int64
	require.NoError(t, db.Model(&ChecklistRun{}).Where("business_id = ? AND template_id = ?", 1, tplID).Count(&runCount).Error)
	require.Equal(t, int64(1), runCount, "no duplicate run")
	require.NoError(t, db.Model(&ChecklistItemCompletion{}).Where("run_id = ?", run1.ID).Count(&compCount).Error)
	require.Equal(t, int64(len(items)), compCount, "completions not duplicated")
}

// TestInstantiateChecklistRunRejectsForeignStaff (MIN-3): an assigned staff not
// in the business is rejected.
func TestInstantiateChecklistRunRejectsForeignStaff(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	tplID, _ := seedTemplate(t, d, 1)
	_, err := d.InstantiateChecklistRun(1, tplID, ptrUint(999), nil, timeNow())
	require.ErrorIs(t, err, ErrInvalidChecklistAssignment)
}

// TestInstantiateChecklistRunRejectsForeignShift (MIN-3): a shift not in the
// business is rejected.
func TestInstantiateChecklistRunRejectsForeignShift(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	tplID, _ := seedTemplate(t, d, 1)
	_, err := d.InstantiateChecklistRun(1, tplID, nil, ptrUint(999), timeNow())
	require.ErrorIs(t, err, ErrInvalidChecklistAssignment)
}

// TestCreateChecklistTemplateRejectsForeignPosition (MIN-3): a position-scoped
// template referencing a non-business position is rejected.
func TestCreateChecklistTemplateRejectsForeignPosition(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	tpl := &ChecklistTemplate{BusinessID: 1, Name: "Server onboarding", Kind: ChecklistKindOnboarding, PositionID: ptrUint(999), CreatedByStaffID: 1, IsActive: true}
	err := d.CreateChecklistTemplate(tpl, nil)
	require.ErrorIs(t, err, ErrPositionNotFound)
}

// TestCreateShoutoutRejectsSelf (MIN-4): a self-shoutout is rejected.
func TestCreateShoutoutRejectsSelf(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	seedStaff(t, 1, 7)
	err := d.CreateShoutout(&Shoutout{BusinessID: 1, FromStaffID: 7, ToStaffID: 7, Message: "me", Visibility: ShoutoutVisibilityTeam})
	require.ErrorIs(t, err, ErrShoutoutSelf)
}

// TestCreateShoutoutRejectsForeignRecipient (MIN-4): a recipient outside the
// business is rejected.
func TestCreateShoutoutRejectsForeignRecipient(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	seedStaff(t, 1, 7)
	err := d.CreateShoutout(&Shoutout{BusinessID: 1, FromStaffID: 7, ToStaffID: 999, Message: "hi", Visibility: ShoutoutVisibilityTeam})
	require.ErrorIs(t, err, ErrShoutoutRecipient)
}

// TestGetChecklistRunDetailReturnsItemsInOrder: detail returns the run's items
// with completion state, in sort order, reflecting a tick.
func TestGetChecklistRunDetailReturnsItemsInOrder(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	seedStaff(t, 1, 7)
	tplID, items := seedTemplate(t, d, 1) // items[0]=required "Lights" sort 0, items[1]=optional "Music" sort 1
	run, err := d.InstantiateChecklistRun(1, tplID, ptrUint(7), nil, timeNow())
	require.NoError(t, err)
	_, err = d.TickChecklistItem(1, run.ID, items[1], 7, true, "done it")
	require.NoError(t, err)

	detail, err := d.GetChecklistRunDetail(1, run.ID)
	require.NoError(t, err)
	require.Equal(t, run.ID, detail.Run.ID)
	require.Len(t, detail.Items, 2)
	// Sort order: "Lights" (0) then "Music" (1).
	require.Equal(t, "Lights", detail.Items[0].Label)
	require.True(t, detail.Items[0].IsRequired)
	require.False(t, detail.Items[0].Done)
	require.Equal(t, "Music", detail.Items[1].Label)
	require.True(t, detail.Items[1].Done)
	require.Equal(t, "done it", detail.Items[1].Note)
	require.NotNil(t, detail.Items[1].CompletedAt)
}

// TestGetChecklistRunDetailNotFound: a missing run → ErrChecklistRunNotFound.
func TestGetChecklistRunDetailNotFound(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	_, err := d.GetChecklistRunDetail(1, 9999)
	require.ErrorIs(t, err, ErrChecklistRunNotFound)
}

// TestGetChecklistRunDetailAccessShape: the items come back via ONE join query
// (no per-item reload), narrow projection, bounded.
func TestGetChecklistRunDetailAccessShape(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	seedStaff(t, 1, 7)
	tplID, _ := seedTemplate(t, d, 1)
	run, err := d.InstantiateChecklistRun(1, tplID, ptrUint(7), nil, timeNow())
	require.NoError(t, err)
	var sqls []string
	d.GetGorm().Session(&gorm.Session{DryRun: true}).
		Callback().Query().After("gorm:query").Register("capture", func(tx *gorm.DB) {
		sqls = append(sqls, tx.Statement.SQL.String())
	})
	_, err = d.GetChecklistRunDetail(1, run.ID)
	require.NoError(t, err)
	var joinCount int
	for _, s := range sqls {
		require.NotContains(t, s, "SELECT *", "must use a narrow projection")
		if strings.Contains(s, "JOIN checklist_items") {
			joinCount++
			require.Contains(t, s, "LIMIT", "items read must be bounded")
		}
	}
	require.Equal(t, 1, joinCount, "items must load via exactly ONE join query (no per-item reload)")
}

func BenchmarkGetChecklistRunDetail(b *testing.B) {
	prev := db
	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", b.Name(), engagementBenchSeq.Add(1))
	gdb, _ := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	sqlDB, _ := gdb.DB()
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	_ = gdb.AutoMigrate(&Business{}, &Staff{}, &ChecklistTemplate{}, &ChecklistItem{}, &ChecklistRun{}, &ChecklistItemCompletion{})
	SetTestDB(gdb)
	defer func() { SetTestDB(prev) }()
	_ = db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error
	_ = db.Create(&Staff{ID: 7, BusinessID: 1, Email: "s7@x.co", Name: "S7", Role: StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error
	d := GetDBWrapper()
	tpl := &ChecklistTemplate{BusinessID: 1, Name: "x", Kind: ChecklistKindOpening, CreatedByStaffID: 1, IsActive: true}
	_ = d.GetGorm().Create(tpl).Error
	for i := 0; i < 25; i++ {
		_ = d.GetGorm().Create(&ChecklistItem{BusinessID: 1, TemplateID: tpl.ID, Label: "Item", SortOrder: i, IsRequired: i%2 == 0}).Error
	}
	run, _ := d.InstantiateChecklistRun(1, tpl.ID, ptrUint(7), nil, time.Now())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = d.GetChecklistRunDetail(1, run.ID)
	}
}

// ---- Slice 9.4: checklist runs access-shape + benchmark ----

func TestListChecklistRunsAccessShape(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	seedStaff(t, 1, 7)
	tplID, _ := seedTemplate(t, d, 1)
	for i := 0; i < 3; i++ {
		_, _ = d.InstantiateChecklistRun(1, tplID, ptrUint(7), nil, timeNow())
	}
	var sqls []string
	d.GetGorm().Session(&gorm.Session{DryRun: true}).
		Callback().Query().After("gorm:query").Register("capture", func(tx *gorm.DB) {
		sqls = append(sqls, tx.Statement.SQL.String())
	})
	_, err := d.ListChecklistRunsForStaff(1, 7, 100)
	require.NoError(t, err)
	for _, s := range sqls {
		require.NotContains(t, s, "SELECT *", "must use a narrow projection")
		require.Contains(t, s, "LIMIT", "must be bounded")
	}
}

func BenchmarkListChecklistRunsForStaff(b *testing.B) {
	prev := db
	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", b.Name(), engagementBenchSeq.Add(1))
	gdb, _ := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	sqlDB, _ := gdb.DB()
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	_ = gdb.AutoMigrate(&Business{}, &Staff{}, &ChecklistTemplate{}, &ChecklistItem{}, &ChecklistRun{}, &ChecklistItemCompletion{})
	SetTestDB(gdb)
	defer func() { SetTestDB(prev) }()
	_ = db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error
	_ = db.Create(&Staff{ID: 7, BusinessID: 1, Email: "s7@x.co", Name: "S7", Role: StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error
	d := GetDBWrapper()
	tpl := &ChecklistTemplate{BusinessID: 1, Name: "x", Kind: ChecklistKindOpening, CreatedByStaffID: 1, IsActive: true}
	_ = d.GetGorm().Create(tpl).Error
	// Seed runs directly (InstantiateChecklistRun is now idempotent on
	// template+assignee, so it would collapse to a single run here).
	for i := 0; i < 50; i++ {
		_ = db.Create(&ChecklistRun{BusinessID: 1, TemplateID: tpl.ID, AssignedStaffID: ptrUint(7), ForDate: time.Now(), Status: ChecklistRunPending}).Error
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = d.ListChecklistRunsForStaff(1, 7, 100)
	}
}

// TestCountPendingChecklistRuns proves the badge count tallies a staffer's own
// runs that are not yet complete (pending + in_progress) and excludes completed
// runs and other staffers' runs. It is a single indexed COUNT — tenant-scoped.
func TestCountPendingChecklistRuns(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	day := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	mk := func(assigned uint, status string) {
		require.NoError(t, db.Create(&ChecklistRun{
			BusinessID: 1, TemplateID: 1, AssignedStaffID: ptrUint(assigned), ForDate: day, Status: status,
		}).Error)
	}
	mk(5, ChecklistRunPending)
	mk(5, ChecklistRunInProgress)
	mk(5, ChecklistRunComplete) // excluded — done
	mk(6, ChecklistRunPending)  // excluded — another staffer

	n, err := d.CountPendingChecklistRuns(1, 5)
	require.NoError(t, err)
	require.Equal(t, int64(2), n, "pending + in_progress for this staffer; complete excluded")

	// Tenant scoping — a foreign business sees nothing.
	n, err = d.CountPendingChecklistRuns(2, 5)
	require.NoError(t, err)
	require.Equal(t, int64(0), n)

	// An owner (no staff row) has no assigned runs.
	n, err = d.CountPendingChecklistRuns(1, 0)
	require.NoError(t, err)
	require.Equal(t, int64(0), n)
}

// ---- Slice 9.6: documents service tests (versioned ack) ----

func TestDocumentAckRecordsVersion(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	doc := &Document{BusinessID: 1, CreatedByStaffID: 1, Title: "Handbook", Content: "v1 body", Version: 1, RequireAck: true, AudienceFilter: "all", IsActive: true}
	require.NoError(t, db.Create(doc).Error)

	require.NoError(t, d.AckDocument(1, doc.ID, 7)) // ack v1
	require.NoError(t, db.Model(&Document{}).Where("id = ?", doc.ID).
		Updates(map[string]any{"version": 2, "content": "v2 body"}).Error) // bump
	require.NoError(t, d.AckDocument(1, doc.ID, 7)) // ack v2

	var acks []DocumentAck
	require.NoError(t, db.Where("document_id = ? AND staff_id = ?", doc.ID, 7).Order("version asc").Find(&acks).Error)
	require.Len(t, acks, 2)
	require.Equal(t, 1, acks[0].Version)
	require.Equal(t, 2, acks[1].Version)

	require.NoError(t, d.AckDocument(1, doc.ID, 7)) // re-ack v2 → idempotent, still 2 rows
	var count int64
	require.NoError(t, db.Model(&DocumentAck{}).Where("document_id = ? AND staff_id = ?", doc.ID, 7).Count(&count).Error)
	require.Equal(t, int64(2), count)
}

// TestDocumentsAckedByTracksCurrentVersion: ack at v1 → true; bump to v2 → the
// old ack no longer satisfies the current version (false) until re-acked at v2.
func TestDocumentsAckedByTracksCurrentVersion(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	doc := &Document{BusinessID: 1, CreatedByStaffID: 1, Title: "Handbook", Content: "v1", Version: 1, RequireAck: true, AudienceFilter: "all", IsActive: true}
	require.NoError(t, db.Create(doc).Error)

	require.NoError(t, d.AckDocument(1, doc.ID, 7)) // ack v1
	v1Docs := []Document{*doc}
	acked, err := d.DocumentsAckedBy(1, 7, v1Docs)
	require.NoError(t, err)
	require.True(t, acked[doc.ID], "ack at current version (1) is satisfied")

	// Bump to v2 via UpdateDocument.
	updated, err := d.UpdateDocument(1, doc.ID, "Handbook", "", "v2", true, "all")
	require.NoError(t, err)
	require.Equal(t, 2, updated.Version)

	acked, err = d.DocumentsAckedBy(1, 7, []Document{*updated})
	require.NoError(t, err)
	require.False(t, acked[doc.ID], "old v1 ack does NOT satisfy the new current version (2)")

	require.NoError(t, d.AckDocument(1, doc.ID, 7)) // re-ack at v2
	acked, err = d.DocumentsAckedBy(1, 7, []Document{*updated})
	require.NoError(t, err)
	require.True(t, acked[doc.ID], "re-ack at v2 satisfies the current version again")
}

// TestDocumentsAckedByEmpty: zero staffID or empty list yields an empty map.
func TestDocumentsAckedByEmpty(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	doc := &Document{BusinessID: 1, CreatedByStaffID: 1, Title: "D", Content: "x", Version: 1, AudienceFilter: "all", IsActive: true}
	require.NoError(t, db.Create(doc).Error)
	require.NoError(t, d.AckDocument(1, doc.ID, 7))

	m, err := d.DocumentsAckedBy(1, 0, []Document{*doc})
	require.NoError(t, err)
	require.Empty(t, m, "owner (staffID 0) cannot ack — empty map")

	m, err = d.DocumentsAckedBy(1, 7, nil)
	require.NoError(t, err)
	require.Empty(t, m, "empty doc list — empty map")
}

// TestUpdateDocumentMissingOrForeign: updating a missing or cross-tenant doc →
// ErrDocumentNotFound.
func TestUpdateDocumentMissingOrForeign(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, db.Create(&Business{ID: 2, BusinessId: "biz-2"}).Error)
	doc := &Document{BusinessID: 2, CreatedByStaffID: 1, Title: "D", Content: "x", Version: 1, AudienceFilter: "all", IsActive: true}
	require.NoError(t, db.Create(doc).Error)

	_, err := d.UpdateDocument(1, 9999, "T", "", "body", false, "all")
	require.ErrorIs(t, err, ErrDocumentNotFound, "missing doc")

	_, err = d.UpdateDocument(1, doc.ID, "T", "", "body", false, "all")
	require.ErrorIs(t, err, ErrDocumentNotFound, "cross-tenant doc is invisible")
}

// TestDocumentsAckedByAccessShape: one bounded IN query, narrow projection.
func TestDocumentsAckedByAccessShape(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	docs := make([]Document, 0, 3)
	for i := 0; i < 3; i++ {
		doc := &Document{BusinessID: 1, CreatedByStaffID: 1, Title: "Doc", Content: "x", Version: 1, AudienceFilter: "all", IsActive: true}
		require.NoError(t, db.Create(doc).Error)
		docs = append(docs, *doc)
	}
	var sqls []string
	d.GetGorm().Session(&gorm.Session{DryRun: true}).
		Callback().Query().After("gorm:query").Register("capture", func(tx *gorm.DB) {
		sqls = append(sqls, tx.Statement.SQL.String())
	})
	_, err := d.DocumentsAckedBy(1, 7, docs)
	require.NoError(t, err)
	require.Len(t, sqls, 1, "must be ONE query (no N+1 over documents)")
	for _, s := range sqls {
		require.NotContains(t, s, "SELECT *", "must use a narrow projection")
		require.Contains(t, s, "IN", "must batch via IN (?)")
	}
}

func BenchmarkDocumentsAckedBy(b *testing.B) {
	prev := db
	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", b.Name(), engagementBenchSeq.Add(1))
	gdb, _ := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	sqlDB, _ := gdb.DB()
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	_ = gdb.AutoMigrate(&Business{}, &Staff{}, &Document{}, &DocumentAck{})
	SetTestDB(gdb)
	defer func() { SetTestDB(prev) }()
	_ = db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error
	d := GetDBWrapper()
	docs := make([]Document, 0, 50)
	for i := 0; i < 50; i++ {
		doc := &Document{BusinessID: 1, CreatedByStaffID: 1, Title: "Doc", Content: "policy body", Version: 1, AudienceFilter: "all", IsActive: true}
		_ = db.Create(doc).Error
		_ = d.AckDocument(1, doc.ID, 7)
		docs = append(docs, *doc)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = d.DocumentsAckedBy(1, 7, docs)
	}
}

func TestListDocumentsAccessShape(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&Document{BusinessID: 1, CreatedByStaffID: 1, Title: "Doc", Content: "x", Version: 1, AudienceFilter: "all", IsActive: true}).Error)
	}
	var sqls []string
	d.GetGorm().Session(&gorm.Session{DryRun: true}).
		Callback().Query().After("gorm:query").Register("capture", func(tx *gorm.DB) {
		sqls = append(sqls, tx.Statement.SQL.String())
	})
	_, err := d.ListDocuments(1, 0, "", true, 200)
	require.NoError(t, err)
	for _, s := range sqls {
		require.NotContains(t, s, "SELECT *", "must use a narrow projection")
		require.Contains(t, s, "LIMIT", "must be bounded")
	}
}

// TestListDocumentsAudienceFilterAccessShape: the line-staff (IN-filtered) read
// path keeps the narrow projection + LIMIT and adds the audience IN predicate —
// no SELECT * regression once privacy filtering is applied.
func TestListDocumentsAudienceFilterAccessShape(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&Document{BusinessID: 1, CreatedByStaffID: 1, Title: "Doc", Content: "x", Version: 1, AudienceFilter: "all", IsActive: true}).Error)
	}
	var sqls []string
	d.GetGorm().Session(&gorm.Session{DryRun: true}).
		Callback().Query().After("gorm:query").Register("capture", func(tx *gorm.DB) {
		sqls = append(sqls, tx.Statement.SQL.String())
	})
	_, err := d.ListDocuments(1, 7, "server", false, 200)
	require.NoError(t, err)
	var sawDocRead bool
	for _, s := range sqls {
		require.NotContains(t, s, "SELECT *", "must use a narrow projection")
		if strings.Contains(s, "documents") {
			sawDocRead = true
			require.Contains(t, s, "audience_filter IN", "line-staff read must apply the audience IN predicate")
			require.Contains(t, s, "LIMIT", "must be bounded")
		}
	}
	require.True(t, sawDocRead, "expected a documents read in the captured SQL")
}

// TestListDocumentsAudiencePrivacy: a line-staff caller (server, "kitchen" dept)
// sees only "all" + their role + their dept; a manager sees every document.
func TestListDocumentsAudiencePrivacy(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, db.Create(&Staff{ID: 7, BusinessID: 1, Email: "s7@x.co", Name: "Cy", Role: StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error)
	pos := &Position{BusinessID: 1, Name: "Line Cook", Department: "kitchen", IsActive: true}
	require.NoError(t, db.Create(pos).Error)
	require.NoError(t, db.Create(&StaffPosition{BusinessID: 1, StaffID: 7, PositionID: pos.ID, IsPrimary: true}).Error)

	mk := func(audience string) {
		require.NoError(t, db.Create(&Document{BusinessID: 1, CreatedByStaffID: 1, Title: "D", Content: "x", Version: 1, AudienceFilter: audience, IsActive: true}).Error)
	}
	mk("all")
	mk("role:server")
	mk("role:manager")
	mk("dept:kitchen")
	mk("dept:bar")

	staffDocs, err := d.ListDocuments(1, 7, "server", false, 200)
	require.NoError(t, err)
	got := map[string]bool{}
	for _, doc := range staffDocs {
		got[doc.AudienceFilter] = true
	}
	require.True(t, got["all"])
	require.True(t, got["role:server"])
	require.True(t, got["dept:kitchen"])
	require.False(t, got["role:manager"], "must NOT see a manager-only doc")
	require.False(t, got["dept:bar"], "must NOT see another department's doc")
	require.Len(t, staffDocs, 3)

	mgrDocs, err := d.ListDocuments(1, 0, "", true, 200)
	require.NoError(t, err)
	require.Len(t, mgrDocs, 5, "manager sees every document")
}

// TestCreateDocumentValidatesAudience: a malformed audience is rejected.
func TestCreateDocumentValidatesAudience(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	err := d.CreateDocument(&Document{BusinessID: 1, CreatedByStaffID: 1, Title: "D", Content: "x", Version: 1, AudienceFilter: "role:"})
	require.ErrorIs(t, err, ErrInvalidAudienceFilter)

	doc := &Document{BusinessID: 1, CreatedByStaffID: 1, Title: "D", Content: "x", Version: 1, AudienceFilter: ""}
	require.NoError(t, d.CreateDocument(doc))
	require.Equal(t, "all", doc.AudienceFilter, "empty audience normalizes to all")
}

func BenchmarkListDocuments(b *testing.B) {
	prev := db
	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", b.Name(), engagementBenchSeq.Add(1))
	gdb, _ := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	sqlDB, _ := gdb.DB()
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	_ = gdb.AutoMigrate(&Business{}, &Staff{}, &Document{}, &DocumentAck{})
	SetTestDB(gdb)
	defer func() { SetTestDB(prev) }()
	_ = db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error
	d := GetDBWrapper()
	for i := 0; i < 50; i++ {
		_ = db.Create(&Document{BusinessID: 1, CreatedByStaffID: 1, Title: "Doc", Content: "policy body", Version: 1, AudienceFilter: "all", IsActive: true}).Error
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = d.ListDocuments(1, 0, "", true, 200)
	}
}

// BenchmarkListDocumentsAudience measures the line-staff (audience IN-filtered)
// read path: the dept-resolution query plus the IN-filtered document read.
func BenchmarkListDocumentsAudience(b *testing.B) {
	prev := db
	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", b.Name(), engagementBenchSeq.Add(1))
	gdb, _ := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	sqlDB, _ := gdb.DB()
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	_ = gdb.AutoMigrate(&Business{}, &Staff{}, &Position{}, &StaffPosition{}, &Document{}, &DocumentAck{})
	SetTestDB(gdb)
	defer func() { SetTestDB(prev) }()
	_ = db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error
	d := GetDBWrapper()
	pos := &Position{BusinessID: 1, Name: "Line Cook", Department: "kitchen", IsActive: true}
	_ = db.Create(pos).Error
	_ = db.Create(&StaffPosition{BusinessID: 1, StaffID: 7, PositionID: pos.ID, IsPrimary: true}).Error
	for i := 0; i < 50; i++ {
		_ = db.Create(&Document{BusinessID: 1, CreatedByStaffID: 1, Title: "Doc", Content: "policy body", Version: 1, AudienceFilter: "all", IsActive: true}).Error
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = d.ListDocuments(1, 7, "server", false, 200)
	}
}

// ---- Slice 9.8: shoutouts service tests ----

func TestShoutoutTeamFeedHidesUnrelatedPrivate(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, db.Create(&Shoutout{BusinessID: 1, FromStaffID: 2, ToStaffID: 3, Message: "team one", Visibility: ShoutoutVisibilityTeam}).Error)
	require.NoError(t, db.Create(&Shoutout{BusinessID: 1, FromStaffID: 2, ToStaffID: 3, Message: "private one", Visibility: ShoutoutVisibilityPrivate}).Error)
	// Caller 9 is uninvolved in the private one → sees only the team shoutout.
	feed, err := d.ListShoutouts(1, 9, 100)
	require.NoError(t, err)
	require.Len(t, feed, 1)
	require.Equal(t, "team one", feed[0].Message)
}

func TestListShoutoutsAccessShape(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&Shoutout{BusinessID: 1, FromStaffID: 2, ToStaffID: 3, Message: "x", Visibility: ShoutoutVisibilityTeam}).Error)
	}
	var sqls []string
	d.GetGorm().Session(&gorm.Session{DryRun: true}).
		Callback().Query().After("gorm:query").Register("capture", func(tx *gorm.DB) {
		sqls = append(sqls, tx.Statement.SQL.String())
	})
	_, err := d.ListShoutouts(1, 9, 100)
	require.NoError(t, err)
	for _, s := range sqls {
		require.NotContains(t, s, "SELECT *", "must use a narrow projection")
		require.Contains(t, s, "LIMIT", "must be bounded")
	}
}

func BenchmarkListShoutouts(b *testing.B) {
	prev := db
	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", b.Name(), engagementBenchSeq.Add(1))
	gdb, _ := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	sqlDB, _ := gdb.DB()
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	_ = gdb.AutoMigrate(&Business{}, &Staff{}, &Shoutout{})
	SetTestDB(gdb)
	defer func() { SetTestDB(prev) }()
	_ = db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error
	d := GetDBWrapper()
	for i := 0; i < 50; i++ {
		_ = db.Create(&Shoutout{BusinessID: 1, FromStaffID: 2, ToStaffID: 3, Message: "great work", Visibility: ShoutoutVisibilityTeam}).Error
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = d.ListShoutouts(1, 9, 100)
	}
}

// ---- Slice 9.10: polls service tests ----

func seedPoll(t *testing.T, d *DB, biz uint, anon bool) (*Poll, []uint) {
	t.Helper()
	p := &Poll{BusinessID: biz, AuthorStaffID: 1, Question: "Pizza night?", IsAnonymous: anon, AudienceFilter: "all", Status: PollStatusOpen}
	require.NoError(t, d.GetGorm().Create(p).Error)
	o1 := &PollOption{BusinessID: biz, PollID: p.ID, Label: "Yes", SortOrder: 0}
	o2 := &PollOption{BusinessID: biz, PollID: p.ID, Label: "No", SortOrder: 1}
	require.NoError(t, d.GetGorm().Create(o1).Error)
	require.NoError(t, d.GetGorm().Create(o2).Error)
	return p, []uint{o1.ID, o2.ID}
}

func TestPollDoubleVoteRejected(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	p, opts := seedPoll(t, d, 1, false)
	require.NoError(t, d.CastVote(1, p.ID, opts[0], 7))
	err := d.CastVote(1, p.ID, opts[1], 7) // same staff, second vote
	require.ErrorIs(t, err, ErrAlreadyVoted)
	var n int64
	require.NoError(t, db.Model(&PollVote{}).Where("poll_id = ? AND staff_id = ?", p.ID, 7).Count(&n).Error)
	require.Equal(t, int64(1), n)
}

func TestAnonymousPollResultsHideVoter(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	p, opts := seedPoll(t, d, 1, true) // anonymous
	require.NoError(t, d.CastVote(1, p.ID, opts[0], 7))
	require.NoError(t, d.CastVote(1, p.ID, opts[0], 8))
	res, err := d.PollResults(1, p.ID)
	require.NoError(t, err)
	require.True(t, res.IsAnonymous)
	require.Nil(t, res.Options[0].Voters, "anonymous results must NOT carry voter ids")
	require.Equal(t, int64(2), res.Options[0].Votes)
	// And the marshaled JSON never carries a staff id.
	raw, _ := json.Marshal(res)
	require.NotContains(t, string(raw), "voter")
}

func TestNonAnonymousPollResultsExposeVoters(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	p, opts := seedPoll(t, d, 1, false)
	require.NoError(t, d.CastVote(1, p.ID, opts[0], 7))
	res, err := d.PollResults(1, p.ID)
	require.NoError(t, err)
	require.Equal(t, []uint{7}, res.Options[0].Voters)
}

// TestCastVoteRejectsPastCloseTime: voting on an open poll whose closes_at is in
// the past returns ErrPollClosed (no DB write flipped the status).
func TestCastVoteRejectsPastCloseTime(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	past := time.Now().UTC().Add(-time.Hour)
	p := &Poll{BusinessID: 1, AuthorStaffID: 1, Question: "Q", AudienceFilter: "all", Status: PollStatusOpen, ClosesAt: &past}
	require.NoError(t, db.Create(p).Error)
	o1 := &PollOption{BusinessID: 1, PollID: p.ID, Label: "Yes", SortOrder: 0}
	require.NoError(t, db.Create(o1).Error)

	err := d.CastVote(1, p.ID, o1.ID, 7)
	require.ErrorIs(t, err, ErrPollClosed)
	var n int64
	require.NoError(t, db.Model(&PollVote{}).Where("poll_id = ?", p.ID).Count(&n).Error)
	require.Equal(t, int64(0), n, "no vote recorded on a past-its-close poll")
}

// TestPastClosePollReadsClosed: ListPolls + PollResults report "closed" for an
// open poll past its close time, without a status write.
func TestPastClosePollReadsClosed(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	past := time.Now().UTC().Add(-time.Hour)
	p := &Poll{BusinessID: 1, AuthorStaffID: 1, Question: "Q", AudienceFilter: "all", Status: PollStatusOpen, ClosesAt: &past}
	require.NoError(t, db.Create(p).Error)
	o1 := &PollOption{BusinessID: 1, PollID: p.ID, Label: "Yes", SortOrder: 0}
	require.NoError(t, db.Create(o1).Error)

	polls, err := d.ListPolls(1, 0, "", true, 100)
	require.NoError(t, err)
	require.Len(t, polls, 1)
	require.Equal(t, PollStatusClosed, polls[0].Status, "ListPolls reflects the effective (closed) status")

	res, err := d.PollResults(1, p.ID)
	require.NoError(t, err)
	require.Equal(t, PollStatusClosed, res.Status, "PollResults reflects the effective (closed) status")

	// The stored row is untouched (still 'open').
	var stored Poll
	require.NoError(t, db.Select("status").Where("id = ?", p.ID).First(&stored).Error)
	require.Equal(t, PollStatusOpen, stored.Status, "no write flipped the stored status")
}

func TestListPollsAccessShape(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	for i := 0; i < 3; i++ {
		_, _ = seedPoll(t, d, 1, false)
	}
	var sqls []string
	d.GetGorm().Session(&gorm.Session{DryRun: true}).
		Callback().Query().After("gorm:query").Register("capture", func(tx *gorm.DB) {
		sqls = append(sqls, tx.Statement.SQL.String())
	})
	_, err := d.ListPolls(1, 0, "", true, 100)
	require.NoError(t, err)
	for _, s := range sqls {
		require.NotContains(t, s, "SELECT *", "must use a narrow projection")
		require.Contains(t, s, "LIMIT", "must be bounded")
	}
}

// TestListPollsAudienceFilterAccessShape: the line-staff (IN-filtered) read path
// keeps the narrow projection + LIMIT and adds the audience IN predicate.
func TestListPollsAudienceFilterAccessShape(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	for i := 0; i < 3; i++ {
		_, _ = seedPoll(t, d, 1, false)
	}
	var sqls []string
	d.GetGorm().Session(&gorm.Session{DryRun: true}).
		Callback().Query().After("gorm:query").Register("capture", func(tx *gorm.DB) {
		sqls = append(sqls, tx.Statement.SQL.String())
	})
	_, err := d.ListPolls(1, 7, "server", false, 100)
	require.NoError(t, err)
	var sawPollRead bool
	for _, s := range sqls {
		require.NotContains(t, s, "SELECT *", "must use a narrow projection")
		if strings.Contains(s, "polls") && !strings.Contains(s, "poll_") {
			sawPollRead = true
			require.Contains(t, s, "audience_filter IN", "line-staff read must apply the audience IN predicate")
			require.Contains(t, s, "LIMIT", "must be bounded")
		}
	}
	require.True(t, sawPollRead, "expected a polls read in the captured SQL")
}

// TestListPollsAudiencePrivacy: a line-staff caller sees only "all" + their role
// + their dept; a manager sees every poll.
func TestListPollsAudiencePrivacy(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	require.NoError(t, db.Create(&Staff{ID: 7, BusinessID: 1, Email: "s7@x.co", Name: "Cy", Role: StaffRoleServer, IsActive: true, InvitedBy: "o"}).Error)
	pos := &Position{BusinessID: 1, Name: "Line Cook", Department: "kitchen", IsActive: true}
	require.NoError(t, db.Create(pos).Error)
	require.NoError(t, db.Create(&StaffPosition{BusinessID: 1, StaffID: 7, PositionID: pos.ID, IsPrimary: true}).Error)

	mk := func(audience string) {
		p := &Poll{BusinessID: 1, AuthorStaffID: 1, Question: "Q", AudienceFilter: audience, Status: PollStatusOpen}
		require.NoError(t, db.Create(p).Error)
	}
	mk("all")
	mk("role:server")
	mk("role:manager")
	mk("dept:kitchen")
	mk("dept:bar")

	staffPolls, err := d.ListPolls(1, 7, "server", false, 100)
	require.NoError(t, err)
	got := map[string]bool{}
	for _, p := range staffPolls {
		got[p.AudienceFilter] = true
	}
	require.True(t, got["all"])
	require.True(t, got["role:server"])
	require.True(t, got["dept:kitchen"])
	require.False(t, got["role:manager"], "must NOT see a manager-only poll")
	require.False(t, got["dept:bar"], "must NOT see another department's poll")
	require.Len(t, staffPolls, 3)

	mgrPolls, err := d.ListPolls(1, 0, "", true, 100)
	require.NoError(t, err)
	require.Len(t, mgrPolls, 5, "manager sees every poll")
}

// TestCreatePollValidatesAudience: a malformed audience is rejected.
func TestCreatePollValidatesAudience(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	err := d.CreatePoll(&Poll{BusinessID: 1, AuthorStaffID: 1, Question: "Q", AudienceFilter: "dept:", Status: PollStatusOpen},
		[]PollOption{{Label: "Yes"}, {Label: "No"}})
	require.ErrorIs(t, err, ErrInvalidAudienceFilter)

	p := &Poll{BusinessID: 1, AuthorStaffID: 1, Question: "Q", AudienceFilter: "", Status: PollStatusOpen}
	require.NoError(t, d.CreatePoll(p, []PollOption{{Label: "Yes"}, {Label: "No"}}))
	require.Equal(t, "all", p.AudienceFilter, "empty audience normalizes to all")
}

func BenchmarkListPolls(b *testing.B) {
	prev := db
	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", b.Name(), engagementBenchSeq.Add(1))
	gdb, _ := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	sqlDB, _ := gdb.DB()
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	_ = gdb.AutoMigrate(&Business{}, &Staff{}, &Poll{}, &PollOption{}, &PollVote{})
	SetTestDB(gdb)
	defer func() { SetTestDB(prev) }()
	_ = db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error
	d := GetDBWrapper()
	for i := 0; i < 50; i++ {
		p := &Poll{BusinessID: 1, AuthorStaffID: 1, Question: "Q", IsAnonymous: false, AudienceFilter: "all", Status: PollStatusOpen}
		_ = d.GetGorm().Create(p).Error
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = d.ListPolls(1, 0, "", true, 100)
	}
}

// BenchmarkListPollsAudience measures the line-staff (audience IN-filtered) read
// path: the dept-resolution query plus the IN-filtered poll read.
func BenchmarkListPollsAudience(b *testing.B) {
	prev := db
	dsn := fmt.Sprintf("file:%s_%d?mode=memory&cache=shared", b.Name(), engagementBenchSeq.Add(1))
	gdb, _ := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Error)})
	sqlDB, _ := gdb.DB()
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	_ = gdb.AutoMigrate(&Business{}, &Staff{}, &Position{}, &StaffPosition{}, &Poll{}, &PollOption{}, &PollVote{})
	SetTestDB(gdb)
	defer func() { SetTestDB(prev) }()
	_ = db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error
	d := GetDBWrapper()
	pos := &Position{BusinessID: 1, Name: "Line Cook", Department: "kitchen", IsActive: true}
	_ = db.Create(pos).Error
	_ = db.Create(&StaffPosition{BusinessID: 1, StaffID: 7, PositionID: pos.ID, IsPrimary: true}).Error
	for i := 0; i < 50; i++ {
		p := &Poll{BusinessID: 1, AuthorStaffID: 1, Question: "Q", IsAnonymous: false, AudienceFilter: "all", Status: PollStatusOpen}
		_ = d.GetGorm().Create(p).Error
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = d.ListPolls(1, 7, "server", false, 100)
	}
}

// TestListChecklistRunsForBusiness proves the operator-visible run list returns
// every run in the business (not just one staffer's), newest first, bounded, with
// the template name + assignee name joined for display context.
func TestListChecklistRunsForBusiness(t *testing.T) {
	d := newEngagementTestDB(t)
	require.NoError(t, db.Create(&Business{ID: 1, BusinessId: "biz-1"}).Error)
	seedStaff(t, 1, 7)
	seedStaff(t, 1, 8)
	tplID, _ := seedTemplate(t, d, 1)

	// Two runs assigned to different staff.
	_, err := d.InstantiateChecklistRun(1, tplID, ptrUint(7), nil, timeNow())
	require.NoError(t, err)
	_, err = d.InstantiateChecklistRun(1, tplID, ptrUint(8), nil, timeNow())
	require.NoError(t, err)

	// A run in a DIFFERENT business must not leak.
	require.NoError(t, db.Create(&Business{ID: 2, BusinessId: "biz-2"}).Error)
	seedStaff(t, 2, 9)
	tpl2 := &ChecklistTemplate{BusinessID: 2, Name: "Other", Kind: ChecklistKindOpening, CreatedByStaffID: 1, IsActive: true}
	require.NoError(t, d.GetGorm().Create(tpl2).Error)
	_, err = d.InstantiateChecklistRun(2, tpl2.ID, ptrUint(9), nil, timeNow())
	require.NoError(t, err)

	runs, err := d.ListChecklistRunsForBusiness(1, 50)
	require.NoError(t, err)
	require.Len(t, runs, 2, "both business-1 runs returned, business-2 excluded")
	// Display context is joined (template + assignee names).
	byAssignee := map[uint]ChecklistRunView{}
	for _, r := range runs {
		require.Equal(t, "Open", r.TemplateName)
		if r.AssignedStaffID != nil {
			byAssignee[*r.AssignedStaffID] = r
		}
	}
	require.Equal(t, "S7", byAssignee[7].AssignedStaffName)
	require.Equal(t, "S8", byAssignee[8].AssignedStaffName)
}

func ptrUint(v uint) *uint { return &v }
