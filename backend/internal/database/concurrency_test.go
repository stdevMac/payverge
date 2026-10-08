package database

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── CONCURRENT APPROVE MULTIPLE ORDERS ───

func TestConcurrent_ApproveMultipleOrders(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)

	// Create 5 different orders for the same bill
	orders := make([]*Order, 5)
	for i := 0; i < 5; i++ {
		orders[i] = helperOrder(t, biz, bill, []OrderItem{
			{ID: fmt.Sprintf("conc-%d", i), MenuItemName: fmt.Sprintf("Item-%d", i), Price: float64(10 + i), Quantity: 1, Subtotal: float64(10 + i)},
		})
	}

	// Approve all concurrently
	var wg sync.WaitGroup
	errs := make([]error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = UpdateOrderStatus(orders[idx].ID, OrderStatusApproved, "staff", "")
		}(i)
	}
	wg.Wait()

	// All should succeed
	for i, err := range errs {
		assert.NoError(t, err, "order %d approval should succeed", i)
	}

	// Verify all orders are approved
	for _, o := range orders {
		var updated Order
		require.NoError(t, db.First(&updated, o.ID).Error)
		assert.Equal(t, OrderStatusApproved, updated.Status)
	}

	// Verify bill has items from all orders (note: with SQLite single-connection, this is sequential)
	var savedBill Bill
	require.NoError(t, db.First(&savedBill, bill.ID).Error)
	var billItems []BillItem
	require.NoError(t, json.Unmarshal([]byte(savedBill.Items), &billItems))
	assert.Equal(t, 5, len(billItems), "all 5 order items should be on the bill")

	// Expected subtotal: 10+11+12+13+14 = 60
	assert.InDelta(t, 6000.0, savedBill.Subtotal, 0.01)
}

// ─── CONCURRENT BILL NUMBER GENERATION ───

func TestConcurrent_BillNumberGeneration(t *testing.T) {
	numbers := make([]string, 10)
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			bn, err := generateUniqueBillNumber(1)
			require.NoError(t, err)
			numbers[idx] = bn
		}(i)
	}
	wg.Wait()

	// All numbers should be unique
	seen := make(map[string]bool)
	for _, n := range numbers {
		assert.False(t, seen[n], "duplicate bill number: %s", n)
		seen[n] = true
	}
}

// ─── CONCURRENT CREATE ORDERS FOR SAME BILL ───

func TestConcurrent_CreateOrdersForSameBill(t *testing.T) {
	setupOrderTestDB(t)
	biz := helperBusiness(t, 0, 0)
	bill := helperBill(t, biz, nil, 0)

	var wg sync.WaitGroup
	orderIDs := make([]uint, 10)
	errs := make([]error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			itemsJSON, _ := json.Marshal([]OrderItem{
				{ID: fmt.Sprintf("cc-%d", idx), MenuItemName: fmt.Sprintf("Dish-%d", idx), Price: 5, Quantity: 1, Subtotal: 5},
			})
			order := &Order{
				BillID:      bill.ID,
				BusinessID:  biz.ID,
				OrderNumber: fmt.Sprintf("O-conc-%d", idx),
				Status:      OrderStatusPending,
				CreatedBy:   "guest",
				Items:       string(itemsJSON),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}
			errs[idx] = db.Create(order).Error
			orderIDs[idx] = order.ID
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		assert.NoError(t, err, "creating order %d should succeed", i)
	}

	// Verify all 10 orders exist
	var count int64
	db.Model(&Order{}).Where("bill_id = ?", bill.ID).Count(&count)
	assert.Equal(t, int64(10), count)
}
