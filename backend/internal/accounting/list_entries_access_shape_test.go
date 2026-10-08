package accounting

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// sqlCapture mirrors other access-shape tests: records SQL strings.
type listEntriesSQLCapture struct {
	logger.Interface
	sqls []string
}

func (c *listEntriesSQLCapture) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	sql, _ := fc()
	c.sqls = append(c.sqls, sql)
}

// TestListEntries_AccessShape_ActorsAndAttachmentCount is the L6-15 gate:
// list path must preload created_by_user/staff with selected columns and
// resolve attachment_count via one grouped query (not N+1 per row).
func TestListEntries_AccessShape_ActorsAndAttachmentCount(t *testing.T) {
	cap := &listEntriesSQLCapture{Interface: logger.Default.LogMode(logger.Silent)}
	dsn := fmt.Sprintf("file:list_entries_shape_%d?mode=memory&cache=shared", time.Now().UnixNano())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: cap})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{}, &database.User{}, &database.Staff{},
		&database.ManualLedgerEntry{}, &database.LedgerEntryAttachment{},
	))
	prev := database.GetDB()
	database.SetTestDB(gormDB)
	t.Cleanup(func() { database.SetTestDB(prev) })

	biz := &database.Business{
		BusinessId: "list-shape", Name: "ListShape", OwnerAddress: "0xListShape",
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD", DisplayCurrency: "USD",
	}
	require.NoError(t, gormDB.Create(biz).Error)
	owner := &database.User{Email: "owner@example.com", Name: "Owner Ada", Role: "user"}
	require.NoError(t, gormDB.Create(owner).Error)
	staff := &database.Staff{BusinessID: biz.ID, Name: "Server Sam", Email: "sam@example.com", Role: "server"}
	require.NoError(t, gormDB.Create(staff).Error)

	occ := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	ownerEntry := &database.ManualLedgerEntry{
		BusinessID: biz.ID, EntryType: database.AccountingEntryTypeExpense,
		Category: "rent", Amount: 10000, Currency: "USD", OccurredAt: occ,
		Description: "Owner rent", CreatedByUserID: &owner.ID,
	}
	staffEntry := &database.ManualLedgerEntry{
		BusinessID: biz.ID, EntryType: database.AccountingEntryTypeExpense,
		Category: "supplies", Amount: 5000, Currency: "USD", OccurredAt: occ,
		Description: "Staff supplies", CreatedByStaffID: &staff.ID,
	}
	voidedAt := occ.Add(time.Hour)
	voidedEntry := &database.ManualLedgerEntry{
		BusinessID: biz.ID, EntryType: database.AccountingEntryTypeExpense,
		Category: "waste", Amount: 2500, Currency: "USD", OccurredAt: occ,
		Description: "Voided waste", CreatedByStaffID: &staff.ID,
		VoidedAt: &voidedAt, VoidedByStaffID: &staff.ID,
	}
	require.NoError(t, gormDB.Create(ownerEntry).Error)
	require.NoError(t, gormDB.Create(staffEntry).Error)
	require.NoError(t, gormDB.Create(voidedEntry).Error)
	// Two attachments on owner entry only.
	require.NoError(t, gormDB.Create(&database.LedgerEntryAttachment{
		EntryID: ownerEntry.ID, BusinessID: biz.ID, S3Key: "k1", FileName: "a.pdf",
	}).Error)
	require.NoError(t, gormDB.Create(&database.LedgerEntryAttachment{
		EntryID: ownerEntry.ID, BusinessID: biz.ID, S3Key: "k2", FileName: "b.pdf",
	}).Error)

	cap.sqls = nil
	svc := NewService(database.GetDBWrapper())
	start := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	page, err := svc.ListEntries(ListEntriesInput{
		BusinessID: biz.ID, StartDate: &start, EndDate: &end, Page: 1, PageSize: 20,
	})
	require.NoError(t, err)
	require.Len(t, page.Entries, 3)

	byID := map[uint]database.ManualLedgerEntry{}
	for _, e := range page.Entries {
		byID[e.ID] = e
	}
	require.Equal(t, 2, byID[ownerEntry.ID].AttachmentCount)
	require.Equal(t, 0, byID[staffEntry.ID].AttachmentCount)
	require.NotNil(t, byID[ownerEntry.ID].CreatedByUser)
	require.Equal(t, "Owner Ada", byID[ownerEntry.ID].CreatedByUser.Name)
	require.NotNil(t, byID[staffEntry.ID].CreatedByStaff)
	require.Equal(t, "Server Sam", byID[staffEntry.ID].CreatedByStaff.Name)
	require.NotNil(t, byID[voidedEntry.ID].VoidedByStaff)
	require.Equal(t, "Server Sam", byID[voidedEntry.ID].VoidedByStaff.Name)

	// --- Strengthened shape assertions (L6-15) -----------------------------
	// Classify every captured SELECT by target table so assertions bind to the
	// actual preload / count queries instead of a joined haystack (where the
	// entries COUNT(*) satisfied "count(" and "password" was inert vs SELECT *).
	fromRe := regexp.MustCompile("(?i)from\\s+[\"'`]?([a-z_]+)[\"'`]?")
	selectListRe := regexp.MustCompile(`(?is)^\s*select\s+(.+?)\s+from\s`)
	normalizeColumns := func(list string) string {
		list = strings.ReplaceAll(list, "\"", "")
		list = strings.ReplaceAll(list, "`", "")
		fields := strings.Split(list, ",")
		for i := range fields {
			fields[i] = strings.ToLower(strings.TrimSpace(fields[i]))
		}
		return strings.Join(fields, ",")
	}

	var userQueries, staffQueries, attachmentQueries []string
	for _, raw := range cap.sqls {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(raw)), "select") {
			continue
		}
		m := fromRe.FindStringSubmatch(raw)
		if m == nil {
			continue
		}
		switch strings.ToLower(m[1]) {
		case "users":
			userQueries = append(userQueries, raw)
		case "staff":
			staffQueries = append(staffQueries, raw)
		case "ledger_entry_attachments":
			attachmentQueries = append(attachmentQueries, raw)
		}
	}

	// (a) Actor batch loads run and project ONLY the expected named columns.
	// Decision-14: at most ONE users query and ONE staff query (CreatedByStaff
	// + VoidedByStaff share a single staff IN list — no second staff Preload).
	// (c) By exact-match, no SELECT * (nor password/auth columns) can hit
	//     users or staff.
	require.Lenf(t, userQueries, 1, "expected exactly one users batch query, got %v", userQueries)
	require.Lenf(t, staffQueries, 1, "expected exactly one staff batch query, got %v", staffQueries)
	actorQueries := append(append([]string{}, userQueries...), staffQueries...)
	for _, q := range actorQueries {
		m := selectListRe.FindStringSubmatch(q)
		require.NotNilf(t, m, "unparsable actor SELECT: %s", q)
		cols := normalizeColumns(m[1])
		require.NotContainsf(t, cols, "*", "SELECT * against actor table: %s", q)
		require.Equalf(t, "id,name,email", cols,
			"actor batch load must select exactly id,name,email: %s", q)
	}

	// (b) EXACTLY one grouped COUNT resolves attachment counts — a per-row
	// N+1 would capture len(entries) queries here.
	require.Lenf(t, attachmentQueries, 1,
		"attachment counts must be ONE grouped query, got %d: %v",
		len(attachmentQueries), attachmentQueries)
	attach := strings.ToLower(attachmentQueries[0])
	require.Contains(t, attach, "count(")
	require.Contains(t, attach, "group by")
	require.NotContains(t, attach, "select *")
}
