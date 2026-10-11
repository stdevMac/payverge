package database

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBillListFilters_CustomerID(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)

	customerID := uint(7)
	otherID := uint(8)
	mk := func(number string, cust *uint) {
		require.NoError(t, db.Create(&Bill{
			BusinessID:     biz.ID,
			BillNumber:     number,
			Items:          "[]",
			Status:         BillStatusPaid,
			SettlementAddr: "0x1111111111111111111111111111111111111111",
			TippingAddr:    "0x2222222222222222222222222222222222222222",
			CRMCustomerID:  cust,
		}).Error)
	}
	mk("CUST-1", &customerID)
	mk("CUST-2", &customerID)
	mk("OTHER-1", &otherID)
	mk("ANON-1", nil)

	result, err := GetBillListRowsByBusinessIDFilteredPaginated(
		biz.ID,
		BillListFilters{CustomerID: &customerID},
		DefaultPagination(),
	)
	require.NoError(t, err)
	require.EqualValues(t, 2, result.Total)
	for _, row := range result.Data {
		require.NotNil(t, row.CRMCustomerID)
		require.Equal(t, customerID, *row.CRMCustomerID)
	}
}

func TestBillListHidesZeroClosedWalkIns(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)

	require.NoError(t, db.Create(&Bill{
		BusinessID: biz.ID, BillNumber: "ZERO-CLOSED", Status: BillStatusClosed, TotalAmount: 0, Items: "[]",
	}).Error)
	require.NoError(t, db.Create(&Bill{
		BusinessID: biz.ID, BillNumber: "ZERO-VOID", Status: BillStatusVoided, TotalAmount: 0, Items: "[]",
	}).Error)
	require.NoError(t, db.Create(&Bill{
		BusinessID: biz.ID, BillNumber: "REAL-CLOSED", Status: BillStatusClosed, TotalAmount: 3782, Items: "[]",
	}).Error)
	require.NoError(t, db.Create(&Bill{
		BusinessID: biz.ID, BillNumber: "OPEN-ZERO", Status: BillStatusOpen, TotalAmount: 0, Items: "[]",
	}).Error)

	result, err := GetBillListRowsByBusinessIDFilteredPaginated(biz.ID, BillListFilters{}, DefaultPagination())
	require.NoError(t, err)
	got := map[string]struct{}{}
	for _, row := range result.Data {
		got[row.BillNumber] = struct{}{}
	}
	require.Contains(t, got, "REAL-CLOSED")
	require.Contains(t, got, "OPEN-ZERO")
	require.NotContains(t, got, "ZERO-CLOSED")
	require.NotContains(t, got, "ZERO-VOID")
}

func TestBillListKeepsZeroClosedCompsWithItems(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)

	comp := &Bill{
		BusinessID: biz.ID, BillNumber: "COMP-ZERO", Status: BillStatusClosed, TotalAmount: 0, Items: "[]",
	}
	require.NoError(t, db.Create(comp).Error)
	require.NoError(t, db.Create(&BillItem{
		ID: "comp-item-1", BillID: comp.ID, Name: "Comp steak", Price: 0, Quantity: 1, Subtotal: 0, ItemType: "menu_item",
	}).Error)
	require.NoError(t, db.Create(&Bill{
		BusinessID: biz.ID, BillNumber: "EMPTY-ZERO", Status: BillStatusClosed, TotalAmount: 0, Items: "[]",
	}).Error)

	result, err := GetBillListRowsByBusinessIDFilteredPaginated(biz.ID, BillListFilters{}, DefaultPagination())
	require.NoError(t, err)
	got := map[string]struct{}{}
	for _, row := range result.Data {
		got[row.BillNumber] = struct{}{}
	}
	require.Contains(t, got, "COMP-ZERO", "real $0 comps with items must stay in history")
	require.NotContains(t, got, "EMPTY-ZERO", "empty $0 closed walk-ins stay hidden")
}
