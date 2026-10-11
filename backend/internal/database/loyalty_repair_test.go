package database

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// The loyalty tier repair helpers (LoyaltyTierDuplicateIDs +
// NormalizeLoyaltyTierSortOrders) back the runtime loader in
// internal/crm/loyalty.go. These tests drive them the same way that loader
// does: load the program's tiers, delete the duplicates, compact sort orders.

type loyaltyRepairTestDB struct {
	conn *gorm.DB
}

func setupLoyaltyRepairTestDB(t *testing.T) *loyaltyRepairTestDB {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite test db: %v", err)
	}
	if err := gormDB.AutoMigrate(&Business{}, &LoyaltyProgram{}, &LoyaltyTier{}); err != nil {
		t.Fatalf("auto-migrate loyalty tables: %v", err)
	}
	return &loyaltyRepairTestDB{conn: gormDB}
}

// seedDefaultLoyaltyProgram creates an enabled program for the business with
// the Bronze/Silver/Gold defaults.
func (tdb *loyaltyRepairTestDB) seedDefaultLoyaltyProgram(t *testing.T, businessID uint) {
	t.Helper()
	program := LoyaltyProgram{BusinessID: businessID, Enabled: true, PointsPerDollar: 1}
	if err := tdb.conn.Create(&program).Error; err != nil {
		t.Fatalf("seed program: %v", err)
	}
	for _, tier := range []LoyaltyTier{
		{LoyaltyProgramID: program.ID, Name: "Bronze", MinLifetimeSpentCents: 0, SortOrder: 0, Color: "#A16207"},
		{LoyaltyProgramID: program.ID, Name: "Silver", MinLifetimeSpentCents: 20000, SortOrder: 1, Color: "#94A3B8"},
		{LoyaltyProgramID: program.ID, Name: "Gold", MinLifetimeSpentCents: 45000, SortOrder: 2, Color: "#CA8A04"},
	} {
		if err := tdb.conn.Create(&tier).Error; err != nil {
			t.Fatalf("seed default tier: %v", err)
		}
	}
}

// repairLoyaltyTiers mirrors the runtime repair in internal/crm/loyalty.go.
func (tdb *loyaltyRepairTestDB) repairLoyaltyTiers(t *testing.T) {
	t.Helper()
	var tiers []LoyaltyTier
	if err := tdb.conn.Order("loyalty_program_id ASC, sort_order ASC, id ASC").Find(&tiers).Error; err != nil {
		t.Fatalf("load tiers: %v", err)
	}
	programIDs := map[uint]struct{}{}
	for _, tier := range tiers {
		programIDs[tier.LoyaltyProgramID] = struct{}{}
	}
	if ids := LoyaltyTierDuplicateIDs(tiers); len(ids) > 0 {
		if err := tdb.conn.Where("id IN ?", ids).Delete(&LoyaltyTier{}).Error; err != nil {
			t.Fatalf("delete duplicate tiers: %v", err)
		}
	}
	for programID := range programIDs {
		if err := NormalizeLoyaltyTierSortOrders(tdb.conn, programID); err != nil {
			t.Fatalf("normalize sort orders: %v", err)
		}
	}
}

func TestLoyaltyTierRepairRemovesDuplicateTierNamesAndThresholds(t *testing.T) {
	tdb := setupLoyaltyRepairTestDB(t)

	biz := Business{
		BusinessId:     "test-biz-drift",
		OwnerAddress:   "0xowner-drift",
		Name:           "Drifted Loyalty Biz",
		SettlementAddr: "0xset",
		TippingAddr:    "0xtip",
	}
	if err := tdb.conn.Create(&biz).Error; err != nil {
		t.Fatalf("seed business: %v", err)
	}
	tdb.seedDefaultLoyaltyProgram(t, biz.ID)

	var program LoyaltyProgram
	if err := tdb.conn.Where("business_id = ?", biz.ID).First(&program).Error; err != nil {
		t.Fatalf("load program: %v", err)
	}
	if err := tdb.conn.Create(&LoyaltyTier{
		LoyaltyProgramID:      program.ID,
		Name:                  " bronze ",
		MinLifetimeSpentCents: 0,
		SortOrder:             99,
	}).Error; err != nil {
		t.Fatalf("seed duplicate tier: %v", err)
	}

	tdb.repairLoyaltyTiers(t)

	var tiers []LoyaltyTier
	if err := tdb.conn.Where("loyalty_program_id = ?", program.ID).
		Order("sort_order ASC").Find(&tiers).Error; err != nil {
		t.Fatalf("load repaired tiers: %v", err)
	}
	if len(tiers) != 3 {
		t.Fatalf("expected 3 repaired tiers, got %d", len(tiers))
	}
	if got := []string{tiers[0].Name, tiers[1].Name, tiers[2].Name}; got[0] != "Bronze" || got[1] != "Silver" || got[2] != "Gold" {
		t.Fatalf("unexpected repaired tier names: %v", got)
	}
}

func TestLoyaltyTierRepairDoesNotUseDeletedTierAsKeeper(t *testing.T) {
	tdb := setupLoyaltyRepairTestDB(t)
	biz := Business{
		BusinessId:     "test-biz-chained-drift",
		OwnerAddress:   "0xowner-chained-drift",
		Name:           "Chained Drift Loyalty Biz",
		SettlementAddr: "0xset",
		TippingAddr:    "0xtip",
	}
	if err := tdb.conn.Create(&biz).Error; err != nil {
		t.Fatalf("seed business: %v", err)
	}
	tdb.seedDefaultLoyaltyProgram(t, biz.ID)

	var program LoyaltyProgram
	if err := tdb.conn.Where("business_id = ?", biz.ID).First(&program).Error; err != nil {
		t.Fatalf("load program: %v", err)
	}
	if err := tdb.conn.Where("loyalty_program_id = ?", program.ID).Delete(&LoyaltyTier{}).Error; err != nil {
		t.Fatalf("clear default tiers: %v", err)
	}
	for _, tier := range []LoyaltyTier{
		{Name: "Bronze", MinLifetimeSpentCents: 0, SortOrder: 0, LoyaltyProgramID: program.ID},
		{Name: "Bronze", MinLifetimeSpentCents: 100, SortOrder: 1, LoyaltyProgramID: program.ID},
		{Name: "Silver", MinLifetimeSpentCents: 100, SortOrder: 2, LoyaltyProgramID: program.ID},
	} {
		if err := tdb.conn.Create(&tier).Error; err != nil {
			t.Fatalf("seed chained duplicate tier: %v", err)
		}
	}

	tdb.repairLoyaltyTiers(t)

	var tiers []LoyaltyTier
	if err := tdb.conn.Where("loyalty_program_id = ?", program.ID).
		Order("sort_order ASC").Find(&tiers).Error; err != nil {
		t.Fatalf("load repaired tiers: %v", err)
	}
	if len(tiers) != 2 {
		t.Fatalf("expected Bronze and Silver survivors, got %d rows: %+v", len(tiers), tiers)
	}
	if tiers[0].Name != "Bronze" || tiers[0].MinLifetimeSpentCents != 0 ||
		tiers[1].Name != "Silver" || tiers[1].MinLifetimeSpentCents != 100 {
		t.Fatalf("unexpected chained survivors: %+v", tiers)
	}
}

func TestLoyaltyTierRepairPrefersNamedTierWhenNameIsBlank(t *testing.T) {
	tdb := setupLoyaltyRepairTestDB(t)
	biz := Business{
		BusinessId:     "test-biz-blank-name-drift",
		OwnerAddress:   "0xowner-blank-name-drift",
		Name:           "Blank Name Drift Loyalty Biz",
		SettlementAddr: "0xset",
		TippingAddr:    "0xtip",
	}
	if err := tdb.conn.Create(&biz).Error; err != nil {
		t.Fatalf("seed business: %v", err)
	}
	tdb.seedDefaultLoyaltyProgram(t, biz.ID)

	var program LoyaltyProgram
	if err := tdb.conn.Where("business_id = ?", biz.ID).First(&program).Error; err != nil {
		t.Fatalf("load program: %v", err)
	}
	if err := tdb.conn.Where("loyalty_program_id = ?", program.ID).Delete(&LoyaltyTier{}).Error; err != nil {
		t.Fatalf("clear default tiers: %v", err)
	}
	for _, tier := range []LoyaltyTier{
		{Name: "", MinLifetimeSpentCents: 100, SortOrder: 0, LoyaltyProgramID: program.ID},
		{Name: "Silver", MinLifetimeSpentCents: 100, SortOrder: 1, LoyaltyProgramID: program.ID},
		{Name: "Gold", MinLifetimeSpentCents: 200, SortOrder: 2, LoyaltyProgramID: program.ID},
	} {
		if err := tdb.conn.Create(&tier).Error; err != nil {
			t.Fatalf("seed blank-name duplicate tier: %v", err)
		}
	}

	tdb.repairLoyaltyTiers(t)

	var tiers []LoyaltyTier
	if err := tdb.conn.Where("loyalty_program_id = ?", program.ID).
		Order("sort_order ASC").Find(&tiers).Error; err != nil {
		t.Fatalf("load repaired tiers: %v", err)
	}
	if len(tiers) != 2 {
		t.Fatalf("expected named Silver keeper and Gold, got %d rows: %+v", len(tiers), tiers)
	}
	if tiers[0].Name != "Silver" || tiers[0].MinLifetimeSpentCents != 100 ||
		tiers[1].Name != "Gold" || tiers[1].MinLifetimeSpentCents != 200 {
		t.Fatalf("unexpected named survivors: %+v", tiers)
	}
}

func TestLoyaltyTierRepairUsesUnicodeWhitespaceAndCaseNormalization(t *testing.T) {
	tdb := setupLoyaltyRepairTestDB(t)
	biz := Business{
		BusinessId:     "test-biz-unicode-drift",
		OwnerAddress:   "0xowner-unicode-drift",
		Name:           "Unicode Drift Loyalty Biz",
		SettlementAddr: "0xset",
		TippingAddr:    "0xtip",
	}
	if err := tdb.conn.Create(&biz).Error; err != nil {
		t.Fatalf("seed business: %v", err)
	}
	tdb.seedDefaultLoyaltyProgram(t, biz.ID)

	var program LoyaltyProgram
	if err := tdb.conn.Where("business_id = ?", biz.ID).First(&program).Error; err != nil {
		t.Fatalf("load program: %v", err)
	}
	if err := tdb.conn.Where("loyalty_program_id = ?", program.ID).Delete(&LoyaltyTier{}).Error; err != nil {
		t.Fatalf("clear default tiers: %v", err)
	}
	for _, tier := range []LoyaltyTier{
		{Name: "\u2003", MinLifetimeSpentCents: 100, SortOrder: 0, LoyaltyProgramID: program.ID},
		{Name: " ÉLITE\u00a0", MinLifetimeSpentCents: 100, SortOrder: 1, LoyaltyProgramID: program.ID},
		{Name: "élite", MinLifetimeSpentCents: 200, SortOrder: 2, LoyaltyProgramID: program.ID},
	} {
		if err := tdb.conn.Create(&tier).Error; err != nil {
			t.Fatalf("seed unicode duplicate tier: %v", err)
		}
	}

	tdb.repairLoyaltyTiers(t)

	var tiers []LoyaltyTier
	if err := tdb.conn.Where("loyalty_program_id = ?", program.ID).
		Order("sort_order ASC").Find(&tiers).Error; err != nil {
		t.Fatalf("load repaired tiers: %v", err)
	}
	if len(tiers) != 1 || tiers[0].Name != " ÉLITE\u00a0" || tiers[0].MinLifetimeSpentCents != 100 {
		t.Fatalf("repair did not match Unicode runtime normalization: %+v", tiers)
	}
}

func TestLoyaltyTierRepairCompactsSortOrdersAfterNamedDuplicateReplacement(t *testing.T) {
	tdb := setupLoyaltyRepairTestDB(t)
	biz := Business{
		BusinessId:     "test-biz-sort-order-drift",
		OwnerAddress:   "0xowner-sort-order-drift",
		Name:           "Sort Order Drift Loyalty Biz",
		SettlementAddr: "0xset",
		TippingAddr:    "0xtip",
	}
	if err := tdb.conn.Create(&biz).Error; err != nil {
		t.Fatalf("seed business: %v", err)
	}
	tdb.seedDefaultLoyaltyProgram(t, biz.ID)

	var program LoyaltyProgram
	if err := tdb.conn.Where("business_id = ?", biz.ID).First(&program).Error; err != nil {
		t.Fatalf("load program: %v", err)
	}
	if err := tdb.conn.Where("loyalty_program_id = ?", program.ID).Delete(&LoyaltyTier{}).Error; err != nil {
		t.Fatalf("clear default tiers: %v", err)
	}
	for _, tier := range []LoyaltyTier{
		{Name: "", MinLifetimeSpentCents: 0, SortOrder: 0, LoyaltyProgramID: program.ID},
		{Name: "Bronze", MinLifetimeSpentCents: 0, SortOrder: 1, LoyaltyProgramID: program.ID},
		{Name: "Silver", MinLifetimeSpentCents: 20000, SortOrder: 3, LoyaltyProgramID: program.ID},
		{Name: "Gold", MinLifetimeSpentCents: 50000, SortOrder: 7, LoyaltyProgramID: program.ID},
	} {
		if err := tdb.conn.Create(&tier).Error; err != nil {
			t.Fatalf("seed sort-order drift: %v", err)
		}
	}

	tdb.repairLoyaltyTiers(t)

	var tiers []LoyaltyTier
	if err := tdb.conn.Where("loyalty_program_id = ?", program.ID).
		Order("sort_order ASC, id ASC").Find(&tiers).Error; err != nil {
		t.Fatalf("load repaired tiers: %v", err)
	}
	if len(tiers) != 3 {
		t.Fatalf("expected 3 repaired tiers, got %d", len(tiers))
	}
	for i, tier := range tiers {
		if tier.SortOrder != i {
			t.Fatalf("tier %q retained non-contiguous sort_order %d, want %d", tier.Name, tier.SortOrder, i)
		}
	}
}

func TestNormalizeLoyaltyTierNamePreservesBackendUnicodeSemantics(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "Unicode whitespace", input: "\u0085\u2003Gold\u0085\u2003", want: "gold"},
		{name: "dotted I uses simple lowercase", input: "İ", want: "i"},
		{name: "Greek sigma uses simple lowercase", input: "ΟΣ", want: "οσ"},
		{name: "FEFF remains content", input: "\ufeffGold\ufeff", want: "\ufeffgold\ufeff"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeLoyaltyTierName(tt.input); got != tt.want {
				t.Fatalf("NormalizeLoyaltyTierName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
