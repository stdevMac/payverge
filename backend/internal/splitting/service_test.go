package splitting

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ──────────────────────────────────────────────────
// Test helpers
// ──────────────────────────────────────────────────

func setupSplitTestDB(t *testing.T) *database.DB {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	sqlDB, _ := gormDB.DB()
	sqlDB.SetMaxOpenConns(1)

	err = gormDB.AutoMigrate(&database.Business{}, &database.Table{}, &database.Bill{}, &database.Payment{})
	if err != nil {
		t.Fatalf("Failed to migrate: %v", err)
	}

	// Create bill_items table for SQLite
	gormDB.Exec("DROP TABLE IF EXISTS bill_items")
	gormDB.Exec(`CREATE TABLE bill_items (
		id TEXT PRIMARY KEY,
		bill_id INTEGER NOT NULL,
		menu_item_id TEXT DEFAULT '',
		name TEXT NOT NULL,
		price REAL NOT NULL,
		quantity INTEGER NOT NULL,
		options TEXT,
		item_type TEXT DEFAULT 'menu_item',
		bundle_id INTEGER,
		parent_bundle_id INTEGER,
		source_offer_id INTEGER,
		order_id INTEGER,
		subtotal REAL NOT NULL,
		created_at DATETIME
	)`)

	// Set the package-level db used by GetBillByID, etc.
	database.SetTestDB(gormDB)

	return database.NewDB()
}

func createTestBill(t *testing.T, gormDB *gorm.DB, subtotal, tax, serviceFee, total float64, items []database.BillItem) uint {
	t.Helper()
	itemsJSON, _ := json.Marshal(items)
	bill := database.Bill{
		BusinessID:       1,
		Subtotal:         int64(math.Round(subtotal * 100)),
		TaxAmount:        int64(math.Round(tax * 100)),
		ServiceFeeAmount: int64(math.Round(serviceFee * 100)),
		TotalAmount:      int64(math.Round(total * 100)),
		Status:           database.BillStatusOpen,
		Items:            string(itemsJSON),
	}
	if err := gormDB.Create(&bill).Error; err != nil {
		t.Fatalf("Failed to create test bill: %v", err)
	}

	// Also insert items into relational table
	for i := range items {
		items[i].BillID = bill.ID
		if items[i].ID == "" {
			items[i].ID = fmt.Sprintf("item-%d-%d", bill.ID, i)
		}
	}
	if len(items) > 0 {
		gormDB.Exec("INSERT INTO bill_items (id, bill_id, name, price, quantity, subtotal) VALUES (?, ?, ?, ?, ?, ?)",
			items[0].ID, bill.ID, items[0].Name, items[0].Price, items[0].Quantity, items[0].Subtotal)
		for _, item := range items[1:] {
			gormDB.Exec("INSERT INTO bill_items (id, bill_id, name, price, quantity, subtotal) VALUES (?, ?, ?, ?, ?, ?)",
				item.ID, bill.ID, item.Name, item.Price, item.Quantity, item.Subtotal)
		}
	}

	return bill.ID
}

// ──────────────────────────────────────────────────
// roundToTwoDecimals
// ──────────────────────────────────────────────────

func TestRoundToTwoDecimals(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{1.005, 1.0},  // 1.005 is actually 1.00499... in IEEE754, rounds down
		{1.006, 1.01}, // this one genuinely rounds up
		{1.004, 1.0},
		{0.0, 0.0},
		{-1.005, -1.0}, // negative
		{99.999, 100.0},
		{33.33333, 33.33},
		{0.1 + 0.2, 0.3}, // floating point addition
		{100.0 / 3.0, 33.33},
	}

	for _, tt := range tests {
		result := roundToTwoDecimals(tt.input)
		assert.InDelta(t, tt.expected, result, 0.005, "roundToTwoDecimals(%v)", tt.input)
	}
}

// ──────────────────────────────────────────────────
// CalculateEqualSplit
// ──────────────────────────────────────────────────

func TestEqualSplit_ZeroPeople(t *testing.T) {
	svc := &SplittingService{}
	_, err := svc.CalculateEqualSplit(1, 0, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "greater than 0")
}

func TestEqualSplit_NegativePeople(t *testing.T) {
	svc := &SplittingService{}
	_, err := svc.CalculateEqualSplit(1, -3, nil)
	assert.Error(t, err)
}

func TestEqualSplit_TwoWayEven(t *testing.T) {
	db := setupSplitTestDB(t)
	gormDB := database.GetDB()

	billID := createTestBill(t, gormDB, 80.0, 16.0, 4.0, 100.0, nil)
	svc := NewSplittingService(db)

	result, err := svc.CalculateEqualSplit(billID, 2, nil)
	assert.NoError(t, err)
	assert.Equal(t, "equal", result.Method)
	assert.Equal(t, 100.0, result.TotalAmount)
	assert.Len(t, result.Splits, 2)

	// Each person gets $50
	for _, split := range result.Splits {
		assert.Equal(t, 50.0, split.Amount)
		assert.Equal(t, 8.0, split.TaxAmount)
		assert.Equal(t, 2.0, split.ServiceFee)
		assert.Equal(t, 40.0, split.Subtotal)
	}
}

func TestEqualSplit_ThreeWay_RoundingAdjustment(t *testing.T) {
	db := setupSplitTestDB(t)
	gormDB := database.GetDB()

	// $100 / 3 = $33.33 each, with $0.01 remainder
	billID := createTestBill(t, gormDB, 80.0, 16.0, 4.0, 100.0, nil)
	svc := NewSplittingService(db)

	result, err := svc.CalculateEqualSplit(billID, 3, nil)
	assert.NoError(t, err)
	assert.Len(t, result.Splits, 3)

	// Verify total sums exactly to bill total
	totalSplit := 0.0
	for _, split := range result.Splits {
		totalSplit += split.Amount
	}
	assert.InDelta(t, 100.0, totalSplit, 0.01, "Split total should match bill total")

	// First two should be 33.33, last one gets the adjustment
	assert.Equal(t, 33.33, result.Splits[0].Amount)
	assert.Equal(t, 33.33, result.Splits[1].Amount)
	// Last person gets the rounding remainder
	assert.InDelta(t, 33.34, result.Splits[2].Amount, 0.01)
}

// FIND-063: independently rounding tax/subtotal/fee in float dollars made each
// person's component line items fail to sum to total_amount (live demo bill
// 31.45+2.79+1.26=35.50 split 2-ways: base+tax+fee was 17.76 vs total 17.75).
func TestEqualSplit_ComponentsSumToPersonTotal(t *testing.T) {
	db := setupSplitTestDB(t)
	gormDB := database.GetDB()

	billID := createTestBill(t, gormDB, 31.45, 2.79, 1.26, 35.50, nil)
	svc := NewSplittingService(db)

	result, err := svc.CalculateEqualSplit(billID, 2, nil)
	assert.NoError(t, err)
	assert.Len(t, result.Splits, 2)

	var sumAmount, sumTax, sumFee, sumSub float64
	for i, split := range result.Splits {
		componentSum := roundToTwoDecimals(split.Subtotal + split.TaxAmount + split.ServiceFee)
		assert.Equalf(t, split.Amount, componentSum,
			"person %d: base+tax+fee (%.2f) must equal total_amount (%.2f)", i+1, componentSum, split.Amount)
		sumAmount += split.Amount
		sumTax += split.TaxAmount
		sumFee += split.ServiceFee
		sumSub += split.Subtotal
	}
	assert.InDelta(t, 35.50, sumAmount, 0.001)
	assert.InDelta(t, 2.79, sumTax, 0.001)
	assert.InDelta(t, 1.26, sumFee, 0.001)
	assert.InDelta(t, 31.45, sumSub, 0.001)
}

func TestEqualSplit_ThreeWay_ComponentsConsistent(t *testing.T) {
	db := setupSplitTestDB(t)
	gormDB := database.GetDB()
	billID := createTestBill(t, gormDB, 80.0, 16.0, 4.0, 100.0, nil)
	svc := NewSplittingService(db)

	result, err := svc.CalculateEqualSplit(billID, 3, nil)
	assert.NoError(t, err)
	var sumAmount, sumTax, sumFee, sumSub float64
	for i, split := range result.Splits {
		assert.Equalf(t, split.Amount, roundToTwoDecimals(split.Subtotal+split.TaxAmount+split.ServiceFee),
			"person %d components must sum to total", i+1)
		sumAmount += split.Amount
		sumTax += split.TaxAmount
		sumFee += split.ServiceFee
		sumSub += split.Subtotal
	}
	assert.InDelta(t, 100.0, sumAmount, 0.001)
	assert.InDelta(t, 16.0, sumTax, 0.001)
	assert.InDelta(t, 4.0, sumFee, 0.001)
	assert.InDelta(t, 80.0, sumSub, 0.001)
}

func TestEqualSplit_OnePerson(t *testing.T) {
	db := setupSplitTestDB(t)
	gormDB := database.GetDB()

	billID := createTestBill(t, gormDB, 50.0, 5.0, 2.50, 57.50, nil)
	svc := NewSplittingService(db)

	result, err := svc.CalculateEqualSplit(billID, 1, nil)
	assert.NoError(t, err)
	assert.Len(t, result.Splits, 1)
	assert.Equal(t, 57.50, result.Splits[0].Amount)
	assert.Equal(t, 5.0, result.Splits[0].TaxAmount)
}

func TestEqualSplit_LargeGroup(t *testing.T) {
	db := setupSplitTestDB(t)
	gormDB := database.GetDB()

	billID := createTestBill(t, gormDB, 1000.0, 100.0, 50.0, 1150.0, nil)
	svc := NewSplittingService(db)

	result, err := svc.CalculateEqualSplit(billID, 7, nil)
	assert.NoError(t, err)
	assert.Len(t, result.Splits, 7)

	totalSplit := 0.0
	for _, split := range result.Splits {
		totalSplit += split.Amount
	}
	assert.InDelta(t, 1150.0, totalSplit, 0.01)
}

func TestEqualSplit_ZeroBill(t *testing.T) {
	db := setupSplitTestDB(t)
	gormDB := database.GetDB()

	billID := createTestBill(t, gormDB, 0.0, 0.0, 0.0, 0.0, nil)
	svc := NewSplittingService(db)

	result, err := svc.CalculateEqualSplit(billID, 3, nil)
	assert.NoError(t, err)
	for _, split := range result.Splits {
		assert.Equal(t, 0.0, split.Amount)
	}
}

func TestEqualSplit_PersonIDsAndNames(t *testing.T) {
	db := setupSplitTestDB(t)
	gormDB := database.GetDB()

	billID := createTestBill(t, gormDB, 100.0, 0.0, 0.0, 100.0, nil)
	svc := NewSplittingService(db)

	result, err := svc.CalculateEqualSplit(billID, 3, map[string]string{
		"person_1": "Alice",
		"person_2": "Bob",
		"person_3": "Carol",
	})
	assert.NoError(t, err)
	assert.Equal(t, "person_1", result.Splits[0].PersonID)
	assert.Equal(t, "Alice", result.Splits[0].PersonName)
	assert.Equal(t, "person_3", result.Splits[2].PersonID)
	assert.Equal(t, "Carol", result.Splits[2].PersonName)
}

func TestEqualSplit_NonExistentBill(t *testing.T) {
	db := setupSplitTestDB(t)
	svc := NewSplittingService(db)

	_, err := svc.CalculateEqualSplit(99999, 2, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get bill")
}

// ──────────────────────────────────────────────────
// CalculateCustomSplit
// ──────────────────────────────────────────────────

func TestCustomSplit_EmptyAmounts(t *testing.T) {
	svc := &SplittingService{}
	_, err := svc.CalculateCustomSplit(1, map[string]float64{}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")
}

func TestCustomSplit_NegativeAmount(t *testing.T) {
	db := setupSplitTestDB(t)
	gormDB := database.GetDB()

	billID := createTestBill(t, gormDB, 100.0, 0.0, 0.0, 100.0, nil)
	svc := NewSplittingService(db)

	_, err := svc.CalculateCustomSplit(billID, map[string]float64{
		"alice": 120.0,
		"bob":   -20.0,
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "negative")
}

func TestCustomSplit_TotalMismatch(t *testing.T) {
	db := setupSplitTestDB(t)
	gormDB := database.GetDB()

	billID := createTestBill(t, gormDB, 100.0, 0.0, 0.0, 100.0, nil)
	svc := NewSplittingService(db)

	_, err := svc.CalculateCustomSplit(billID, map[string]float64{
		"alice": 60.0,
		"bob":   30.0, // Total = 90, bill = 100
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "does not match")
}

func TestEqualSplit_PartialBillUsesRemaining(t *testing.T) {
	db := setupSplitTestDB(t)
	gormDB := database.GetDB()
	billID := createTestBill(t, gormDB, 31.00, 2.75, 1.24, 34.99, nil)
	require.NoError(t, gormDB.Model(&database.Bill{}).Where("id = ?", billID).Updates(map[string]any{
		"paid_amount": int64(1749),
		"status":      database.BillStatusPartial,
	}).Error)
	svc := NewSplittingService(db)

	result, err := svc.CalculateEqualSplit(billID, 2, nil)
	assert.NoError(t, err)
	assert.InDelta(t, 17.50, result.TotalAmount, 0.001)
	require.Len(t, result.Splits, 2)
	sum := result.Splits[0].Amount + result.Splits[1].Amount
	assert.InDelta(t, 17.50, sum, 0.001)
	assert.InDelta(t, 8.75, result.Splits[0].Amount, 0.011)
}

func TestCustomSplit_AcceptsRemainingBalance(t *testing.T) {
	db := setupSplitTestDB(t)
	gormDB := database.GetDB()
	billID := createTestBill(t, gormDB, 31.00, 2.75, 1.24, 34.99, nil)
	require.NoError(t, gormDB.Model(&database.Bill{}).Where("id = ?", billID).Updates(map[string]any{
		"paid_amount": int64(1749),
		"status":      database.BillStatusPartial,
	}).Error)
	svc := NewSplittingService(db)

	result, err := svc.CalculateCustomSplit(billID, map[string]float64{
		"a": 8.75,
		"b": 8.75,
	}, nil)
	assert.NoError(t, err)
	assert.InDelta(t, 17.50, result.TotalAmount, 0.001)

	_, err = svc.CalculateCustomSplit(billID, map[string]float64{
		"a": 17.49,
		"b": 17.50,
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "remaining balance")
}

func TestCustomSplit_ValidSplit(t *testing.T) {
	db := setupSplitTestDB(t)
	gormDB := database.GetDB()

	billID := createTestBill(t, gormDB, 80.0, 10.0, 10.0, 100.0, nil)
	svc := NewSplittingService(db)

	result, err := svc.CalculateCustomSplit(billID, map[string]float64{
		"alice": 70.0,
		"bob":   30.0,
	}, map[string]string{
		"alice": "Alice",
		"bob":   "Bob",
	})
	assert.NoError(t, err)
	assert.Equal(t, "custom", result.Method)
	assert.Len(t, result.Splits, 2)

	// Find Alice's split
	var aliceSplit *PersonSplit
	for i := range result.Splits {
		if result.Splits[i].PersonID == "alice" {
			aliceSplit = &result.Splits[i]
		}
	}
	assert.NotNil(t, aliceSplit)
	assert.Equal(t, "Alice", aliceSplit.PersonName)
	assert.Equal(t, 70.0, aliceSplit.Amount)
	// Alice pays 70% of bill → 70% of tax = 7.0
	assert.Equal(t, 7.0, aliceSplit.TaxAmount)
	assert.Equal(t, 7.0, aliceSplit.ServiceFee)
}

func TestCustomSplit_PersonNameFallback(t *testing.T) {
	db := setupSplitTestDB(t)
	gormDB := database.GetDB()

	billID := createTestBill(t, gormDB, 100.0, 0.0, 0.0, 100.0, nil)
	svc := NewSplittingService(db)

	result, err := svc.CalculateCustomSplit(billID, map[string]float64{
		"person_abc": 100.0,
	}, nil) // No people map
	assert.NoError(t, err)
	assert.Equal(t, "person_abc", result.Splits[0].PersonName) // Falls back to ID
}

func TestCustomSplit_WithinTolerance(t *testing.T) {
	db := setupSplitTestDB(t)
	gormDB := database.GetDB()

	billID := createTestBill(t, gormDB, 100.0, 0.0, 0.0, 100.0, nil)
	svc := NewSplittingService(db)

	// Off by $0.005 — within $0.01 tolerance
	_, err := svc.CalculateCustomSplit(billID, map[string]float64{
		"a": 33.335,
		"b": 33.335,
		"c": 33.335,
	}, nil)
	assert.NoError(t, err)
}

func TestCustomSplit_NonExistentBill(t *testing.T) {
	db := setupSplitTestDB(t)
	svc := NewSplittingService(db)

	_, err := svc.CalculateCustomSplit(99999, map[string]float64{"a": 100.0}, nil)
	assert.Error(t, err)
}

// FIND-063 residual: odd-cent tax/fee on custom split must keep
// base+tax+fee == total_amount per person and buckets sum to the bill.
func TestCustomSplit_ComponentsSumToPersonTotal(t *testing.T) {
	db := setupSplitTestDB(t)
	gormDB := database.GetDB()

	// Live-shaped odd-cent bill (same numbers as equal-split FIND-063).
	billID := createTestBill(t, gormDB, 31.45, 2.79, 1.26, 35.50, nil)
	svc := NewSplittingService(db)

	result, err := svc.CalculateCustomSplit(billID, map[string]float64{
		"alice": 20.00,
		"bob":   15.50,
	}, map[string]string{"alice": "Alice", "bob": "Bob"})
	assert.NoError(t, err)
	assert.Len(t, result.Splits, 2)

	var sumAmount, sumTax, sumFee, sumSub float64
	for i, split := range result.Splits {
		componentSum := roundToTwoDecimals(split.Subtotal + split.TaxAmount + split.ServiceFee)
		assert.Equalf(t, split.Amount, componentSum,
			"person %s: base+tax+fee (%.2f) must equal total_amount (%.2f)",
			split.PersonID, componentSum, split.Amount)
		_ = i
		sumAmount += split.Amount
		sumTax += split.TaxAmount
		sumFee += split.ServiceFee
		sumSub += split.Subtotal
	}
	assert.InDelta(t, 35.50, sumAmount, 0.001)
	assert.InDelta(t, 2.79, sumTax, 0.001)
	assert.InDelta(t, 1.26, sumFee, 0.001)
	assert.InDelta(t, 31.45, sumSub, 0.001)
}

func TestAllocateProportionalCents_RemainderOnLast(t *testing.T) {
	// 279 tax cents across shares 2000 and 1550 of 3550.
	out := allocateProportionalCents(279, []int64{2000, 1550})
	assert.Equal(t, int64(279), out[0]+out[1])
	assert.Equal(t, int64((279*2000)/3550), out[0])
	assert.Equal(t, int64(279)-out[0], out[1])
}

// ──────────────────────────────────────────────────
// CalculateItemSplit
// ──────────────────────────────────────────────────

func TestItemSplit_EmptySelections(t *testing.T) {
	svc := &SplittingService{}
	_, err := svc.CalculateItemSplit(1, map[string][]string{}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")
}

func TestItemSplit_NonExistentItem(t *testing.T) {
	db := setupSplitTestDB(t)
	gormDB := database.GetDB()

	items := []database.BillItem{
		{ID: "burger-1", Name: "Burger", Price: 10.0, Quantity: 1, Subtotal: 10.0},
	}
	billID := createTestBill(t, gormDB, 10.0, 1.0, 0.5, 11.50, items)
	svc := NewSplittingService(db)

	_, err := svc.CalculateItemSplit(billID, map[string][]string{
		"alice": {"burger-1", "nonexistent-id"},
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestItemSplit_UnassignedItem(t *testing.T) {
	db := setupSplitTestDB(t)
	gormDB := database.GetDB()

	items := []database.BillItem{
		{ID: "burger-1", Name: "Burger", Price: 10.0, Quantity: 1, Subtotal: 10.0},
		{ID: "fries-1", Name: "Fries", Price: 5.0, Quantity: 1, Subtotal: 5.0},
	}
	billID := createTestBill(t, gormDB, 15.0, 1.50, 0.75, 17.25, items)
	svc := NewSplittingService(db)

	// Only assign burger, fries unassigned
	_, err := svc.CalculateItemSplit(billID, map[string][]string{
		"alice": {"burger-1"},
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not assigned")
}

func TestItemSplit_DuplicateAssignment(t *testing.T) {
	db := setupSplitTestDB(t)
	gormDB := database.GetDB()

	items := []database.BillItem{
		{ID: "burger-1", Name: "Burger", Price: 10.0, Quantity: 1, Subtotal: 10.0},
		{ID: "fries-1", Name: "Fries", Price: 5.0, Quantity: 1, Subtotal: 5.0},
	}
	billID := createTestBill(t, gormDB, 15.0, 1.50, 0.75, 17.25, items)
	svc := NewSplittingService(db)

	_, err := svc.CalculateItemSplit(billID, map[string][]string{
		"alice": {"burger-1"},
		"bob":   {"burger-1", "fries-1"},
	}, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "assigned to multiple people")
}

func TestItemSplit_ValidSplit(t *testing.T) {
	db := setupSplitTestDB(t)
	gormDB := database.GetDB()

	items := []database.BillItem{
		{ID: "burger-1", Name: "Burger", Price: 10.0, Quantity: 1, Subtotal: 10.0},
		{ID: "fries-1", Name: "Fries", Price: 5.0, Quantity: 1, Subtotal: 5.0},
		{ID: "drink-1", Name: "Cola", Price: 3.0, Quantity: 1, Subtotal: 3.0},
	}
	// Subtotal=18, tax=1.80, serviceFee=0.90, total=20.70
	billID := createTestBill(t, gormDB, 18.0, 1.80, 0.90, 20.70, items)
	svc := NewSplittingService(db)

	result, err := svc.CalculateItemSplit(billID, map[string][]string{
		"alice": {"burger-1", "drink-1"}, // subtotal = 13.0
		"bob":   {"fries-1"},             // subtotal = 5.0
	}, map[string]string{
		"alice": "Alice",
		"bob":   "Bob",
	})
	assert.NoError(t, err)
	assert.Equal(t, "items", result.Method)
	assert.Len(t, result.Splits, 2)

	// Find Alice's split
	var aliceSplit *PersonSplit
	for i := range result.Splits {
		if result.Splits[i].PersonID == "alice" {
			aliceSplit = &result.Splits[i]
		}
	}
	assert.NotNil(t, aliceSplit)
	assert.Equal(t, 13.0, aliceSplit.Subtotal)
	assert.Len(t, aliceSplit.Items, 2)

	// Alice's proportion: 13/18 ≈ 0.7222
	expectedTax := roundToTwoDecimals(1.80 * (13.0 / 18.0))
	assert.Equal(t, expectedTax, aliceSplit.TaxAmount)
}

func TestItemSplit_NonExistentBill(t *testing.T) {
	db := setupSplitTestDB(t)
	svc := NewSplittingService(db)

	_, err := svc.CalculateItemSplit(99999, map[string][]string{
		"alice": {"item-1"},
	}, nil)
	assert.Error(t, err)
}

// FIND-063 residual: item split with odd-cent tax/fee must keep components
// consistent and person totals sum to the bill total.
func TestItemSplit_ComponentsSumToPersonTotal(t *testing.T) {
	db := setupSplitTestDB(t)
	gormDB := database.GetDB()

	items := []database.BillItem{
		{ID: "bowl-1", Name: "Harvest Bowl", Price: 18.50, Quantity: 1, Subtotal: 18.50},
		{ID: "tart-1", Name: "Chocolate Tart", Price: 12.95, Quantity: 1, Subtotal: 12.95},
	}
	// subtotal 31.45, tax 2.79, fee 1.26, total 35.50
	billID := createTestBill(t, gormDB, 31.45, 2.79, 1.26, 35.50, items)
	svc := NewSplittingService(db)

	result, err := svc.CalculateItemSplit(billID, map[string][]string{
		"alice": {"bowl-1"},
		"bob":   {"tart-1"},
	}, nil)
	assert.NoError(t, err)
	assert.Len(t, result.Splits, 2)

	var sumAmount, sumTax, sumFee, sumSub float64
	for _, split := range result.Splits {
		componentSum := roundToTwoDecimals(split.Subtotal + split.TaxAmount + split.ServiceFee)
		assert.Equalf(t, split.Amount, componentSum,
			"person %s: base+tax+fee (%.2f) must equal total_amount (%.2f)",
			split.PersonID, componentSum, split.Amount)
		sumAmount += split.Amount
		sumTax += split.TaxAmount
		sumFee += split.ServiceFee
		sumSub += split.Subtotal
	}
	assert.InDelta(t, 35.50, sumAmount, 0.001)
	assert.InDelta(t, 2.79, sumTax, 0.001)
	assert.InDelta(t, 1.26, sumFee, 0.001)
	assert.InDelta(t, 31.45, sumSub, 0.001)
}
