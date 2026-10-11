package database

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── PAGINATION PARAMS ───

func TestPaginationParams_Normalize(t *testing.T) {
	tests := []struct {
		name     string
		input    PaginationParams
		expected PaginationParams
	}{
		{"defaults", PaginationParams{0, 0}, PaginationParams{1, 20}},
		{"negative page", PaginationParams{-1, 10}, PaginationParams{1, 10}},
		{"over max page size", PaginationParams{1, 200}, PaginationParams{1, 100}},
		{"valid", PaginationParams{3, 50}, PaginationParams{3, 50}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.input.Normalize()
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestPaginationParams_Offset(t *testing.T) {
	p := PaginationParams{Page: 3, PageSize: 10}
	assert.Equal(t, 20, p.Offset())
}

// ─── BILLS PAGINATION ───

func TestGetAllBills_Pagination(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)

	// Create 25 bills
	for i := 0; i < 25; i++ {
		bill := &Bill{
			BusinessID: biz.ID,
			BillNumber: fmt.Sprintf("B-page-%d", i),
			Status:     BillStatusOpen,
			Items:      "[]",
			CreatedAt:  time.Now().Add(time.Duration(i) * time.Second),
		}
		require.NoError(t, db.Create(bill).Error)
	}

	// Page 1
	result, err := GetBillListRowsByBusinessIDFilteredPaginated(biz.ID, BillListFilters{}, PaginationParams{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, 10, len(result.Data))
	assert.Equal(t, int64(25), result.Total)
	assert.Equal(t, 3, result.TotalPages)
	assert.Equal(t, 1, result.Page)

	// Page 3 (last page, 5 items)
	result, err = GetBillListRowsByBusinessIDFilteredPaginated(biz.ID, BillListFilters{}, PaginationParams{Page: 3, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, 5, len(result.Data))
	assert.Equal(t, int64(25), result.Total)
	assert.Equal(t, 3, result.Page)

	// Beyond last page
	result, err = GetBillListRowsByBusinessIDFilteredPaginated(biz.ID, BillListFilters{}, PaginationParams{Page: 10, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, 0, len(result.Data))
	assert.Equal(t, int64(25), result.Total)
}

func TestGetOpenBills_Pagination(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)

	// Create mix of statuses
	for i := 0; i < 15; i++ {
		status := BillStatusOpen
		if i%3 == 0 {
			status = BillStatusClosed
		}
		bill := &Bill{
			BusinessID: biz.ID,
			BillNumber: fmt.Sprintf("B-open-%d", i),
			Status:     status,
			Items:      "[]",
			CreatedAt:  time.Now().Add(time.Duration(i) * time.Second),
		}
		require.NoError(t, db.Create(bill).Error)
	}

	// 15 bills, 5 closed (i=0,3,6,9,12), 10 open
	result, err := GetOpenBillListRowsByBusinessIDFilteredPaginated(biz.ID, BillListFilters{}, PaginationParams{Page: 1, PageSize: 5})
	require.NoError(t, err)
	assert.Equal(t, 5, len(result.Data))
	assert.Equal(t, int64(10), result.Total)
	assert.Equal(t, 2, result.TotalPages)

	// Verify all returned are open
	for _, b := range result.Data {
		assert.Equal(t, BillStatusOpen, b.Status)
	}
}

func TestGetBills_PaginationWithStatusFilter(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)

	for i := 0; i < 12; i++ {
		status := BillStatusPaid
		if i%2 == 0 {
			status = BillStatusClosed
		}
		if i == 11 {
			status = BillStatusOpen
		}
		require.NoError(t, db.Create(&Bill{
			BusinessID:  biz.ID,
			BillNumber:  fmt.Sprintf("B-status-filter-%d", i),
			Status:      status,
			TotalAmount: 1000,
			Items:       "[]",
			CreatedAt:   time.Now().Add(time.Duration(i) * time.Second),
		}).Error)
	}

	result, err := GetBillListRowsByBusinessIDFilteredPaginated(biz.ID, BillListFilters{Status: "paid,closed"}, PaginationParams{Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(11), result.Total)
	for _, bill := range result.Data {
		assert.NotEqual(t, BillStatusOpen, bill.Status)
	}
}

func TestGetBills_FilteredPaginationSearchesAndDatesInDatabase(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	may10 := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	may11 := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	may12 := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)

	bills := []Bill{
		{BusinessID: biz.ID, BillNumber: "PV-older-match", Status: BillStatusPaid, Items: "[]", TotalAmount: 4200, TableID: 9, CreatedAt: may10},
		{BusinessID: biz.ID, BillNumber: "PV-target", Status: BillStatusPaid, Items: "[]", TotalAmount: 4200, TableID: 9, CreatedAt: may11},
		{BusinessID: biz.ID, BillNumber: "PV-target-open", Status: BillStatusOpen, Items: "[]", TotalAmount: 4200, TableID: 9, CreatedAt: may11},
		{BusinessID: biz.ID, BillNumber: "PV-newer-match", Status: BillStatusPaid, Items: "[]", TotalAmount: 4200, TableID: 9, CreatedAt: may12},
	}
	for i := range bills {
		require.NoError(t, db.Create(&bills[i]).Error)
	}

	from := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	result, err := GetBillListRowsByBusinessIDFilteredPaginated(biz.ID, BillListFilters{
		Status:      "paid,closed",
		Search:      "table 9",
		CreatedFrom: &from,
		CreatedTo:   &to,
	}, PaginationParams{Page: 1, PageSize: 10})
	require.NoError(t, err)

	require.Len(t, result.Data, 1)
	assert.Equal(t, "PV-target", result.Data[0].BillNumber)
	assert.Equal(t, int64(1), result.Total)
}

func TestGetBillListRows_FilteredProjectionPreservesListContract(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	table := &Table{
		BusinessID: biz.ID,
		TableCode:  "bill-list-projection-t01",
		Name:       "Patio 1",
		IsActive:   true,
	}
	require.NoError(t, db.Create(table).Error)

	createdAt := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	bill := &Bill{
		BusinessID:     biz.ID,
		TableID:        table.ID,
		BillNumber:     "PV-projection",
		Notes:          "keep list contract",
		Items:          `[{"name":"Burger","quantity":1}]`,
		Subtotal:       1000,
		TaxAmount:      100,
		TotalAmount:    1100,
		Status:         BillStatusPaid,
		SettlementAddr: "settlement",
		TippingAddr:    "tipping",
		CreatedAt:      createdAt,
	}
	require.NoError(t, db.Create(bill).Error)

	result, err := GetBillListRowsByBusinessIDFilteredPaginated(biz.ID, BillListFilters{
		Status: "paid",
		Search: "projection",
	}, PaginationParams{Page: 1, PageSize: 10})
	require.NoError(t, err)

	require.Len(t, result.Data, 1)
	row := result.Data[0]
	assert.Equal(t, bill.ID, row.ID)
	assert.Equal(t, biz.ID, row.BusinessID)
	assert.Equal(t, table.ID, row.TableID)
	assert.Equal(t, "PV-projection", row.BillNumber)
	assert.Equal(t, "keep list contract", row.Notes)
	assert.Equal(t, `[{"name":"Burger","quantity":1}]`, row.Items)
	assert.Equal(t, int64(1100), row.TotalAmount)
	assert.Equal(t, BillStatusPaid, row.Status)
	assert.Equal(t, "Patio 1", row.TableName)
	assert.Equal(t, int64(1), result.Total)
}

func TestGetBills_FilteredSearchUsesExactTableAndCounterIdentifiers(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	now := time.Date(2026, 5, 13, 12, 0, 0, 0, time.UTC)
	counter9 := uint(9)
	counter19 := uint(19)

	bills := []Bill{
		{BusinessID: biz.ID, BillNumber: "PV-table-nine", Status: BillStatusPaid, Items: "[]", TotalAmount: 4200, TableID: 9, CreatedAt: now},
		{BusinessID: biz.ID, BillNumber: "PV-table-nineteen", Status: BillStatusPaid, Items: "[]", TotalAmount: 4200, TableID: 19, CreatedAt: now.Add(time.Second)},
		{BusinessID: biz.ID, BillNumber: "PV-counter-nine", Status: BillStatusPaid, Items: "[]", TotalAmount: 4200, CounterID: &counter9, CreatedAt: now.Add(2 * time.Second)},
		{BusinessID: biz.ID, BillNumber: "PV-counter-nineteen", Status: BillStatusPaid, Items: "[]", TotalAmount: 4200, CounterID: &counter19, CreatedAt: now.Add(3 * time.Second)},
	}
	for i := range bills {
		require.NoError(t, db.Create(&bills[i]).Error)
	}

	tableResult, err := GetBillListRowsByBusinessIDFilteredPaginated(biz.ID, BillListFilters{
		Status: "paid,closed",
		Search: "table 9",
	}, PaginationParams{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, tableResult.Data, 1)
	assert.Equal(t, "PV-table-nine", tableResult.Data[0].BillNumber)
	assert.Equal(t, int64(1), tableResult.Total)

	counterResult, err := GetBillListRowsByBusinessIDFilteredPaginated(biz.ID, BillListFilters{
		Status: "paid,closed",
		Search: "counter 9",
	}, PaginationParams{Page: 1, PageSize: 10})
	require.NoError(t, err)
	require.Len(t, counterResult.Data, 1)
	assert.Equal(t, "PV-counter-nine", counterResult.Data[0].BillNumber)
	assert.Equal(t, int64(1), counterResult.Total)
}

// ─── ORDERS PAGINATION ───

func TestGetOrders_Pagination(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)

	for i := 0; i < 30; i++ {
		itemsJSON, _ := json.Marshal([]OrderItem{})
		order := &Order{
			BillID:      bill.ID,
			BusinessID:  biz.ID,
			OrderNumber: fmt.Sprintf("O-page-%d", i),
			Status:      OrderStatusPending,
			CreatedBy:   "guest",
			Items:       string(itemsJSON),
			CreatedAt:   time.Now().Add(time.Duration(i) * time.Second),
			UpdatedAt:   time.Now(),
		}
		require.NoError(t, db.Create(order).Error)
	}

	result, err := GetOrdersByBusinessIDPaginated(biz.ID, "", PaginationParams{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, 10, len(result.Data))
	assert.Equal(t, int64(30), result.Total)
	assert.Equal(t, 3, result.TotalPages)

	// Verify ordering (created_at DESC)
	for i := 1; i < len(result.Data); i++ {
		assert.True(t, result.Data[i-1].CreatedAt.After(result.Data[i].CreatedAt) ||
			result.Data[i-1].CreatedAt.Equal(result.Data[i].CreatedAt),
			"results should be ordered by created_at DESC")
	}
}

func TestGetOrders_PaginationWithStatusFilter(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)

	// Create 20 pending, 10 approved
	for i := 0; i < 30; i++ {
		status := OrderStatusPending
		if i >= 20 {
			status = OrderStatusApproved
		}
		itemsJSON, _ := json.Marshal([]OrderItem{})
		order := &Order{
			BillID:      bill.ID,
			BusinessID:  biz.ID,
			OrderNumber: fmt.Sprintf("O-filter-%d", i),
			Status:      status,
			CreatedBy:   "guest",
			Items:       string(itemsJSON),
			CreatedAt:   time.Now().Add(time.Duration(i) * time.Second),
			UpdatedAt:   time.Now(),
		}
		require.NoError(t, db.Create(order).Error)
	}

	// Paginate only pending
	result, err := GetOrdersByBusinessIDPaginated(biz.ID, string(OrderStatusPending), PaginationParams{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, 10, len(result.Data))
	assert.Equal(t, int64(20), result.Total)
	assert.Equal(t, 2, result.TotalPages)

	// Paginate only approved
	result, err = GetOrdersByBusinessIDPaginated(biz.ID, string(OrderStatusApproved), PaginationParams{Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Equal(t, 10, len(result.Data))
	assert.Equal(t, int64(10), result.Total)
}
