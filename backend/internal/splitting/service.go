package splitting

import (
	"errors"
	"fmt"
	"github.com/stdevmac/payverge/backend/internal/database"
	"math"
	"sort"
	"time"
)

// SplittingService handles bill splitting calculations
type SplittingService struct {
	db *database.DB
}

// NewSplittingService creates a new splitting service
func NewSplittingService(db *database.DB) *SplittingService {
	return &SplittingService{
		db: db,
	}
}

// splitPayableBuckets is the amount the guest split calculators must partition:
// remaining balance (total − paid), with tax/fee scaled the same way. Holds
// and split/state already use remaining; calculators used to partition the
// gross total and reject the only correct custom split (#522).
func splitPayableBuckets(bill *database.Bill) (total, tax, fee, sub int64) {
	if bill == nil {
		return 0, 0, 0, 0
	}
	remaining := bill.TotalAmount - bill.PaidAmount
	if remaining < 0 {
		remaining = 0
	}
	if bill.PaidAmount <= 0 || remaining == bill.TotalAmount {
		return bill.TotalAmount, bill.TaxAmount, bill.ServiceFeeAmount, bill.Subtotal
	}
	if bill.TotalAmount <= 0 {
		return 0, 0, 0, 0
	}
	tax = (bill.TaxAmount * remaining) / bill.TotalAmount
	fee = (bill.ServiceFeeAmount * remaining) / bill.TotalAmount
	sub = remaining - tax - fee
	return remaining, tax, fee, sub
}

func (s *SplittingService) getBillSplitSummary(billID uint) (*database.Bill, error) {
	var bill database.Bill
	if err := s.db.GetGorm().
		Select("id", "subtotal", "tax_amount", "service_fee_amount", "total_amount", "paid_amount", "status").
		First(&bill, billID).Error; err != nil {
		return nil, err
	}
	return &bill, nil
}

// SplitResult represents the result of a bill split calculation
type SplitResult struct {
	Method      string                 `json:"split_method"`
	TotalAmount float64                `json:"total_amount"`
	TipAmount   float64                `json:"tip_amount"`
	GrandTotal  float64                `json:"grand_total"`
	Splits      []PersonSplit          `json:"people"`
	CreatedAt   time.Time              `json:"created_at"`
	Breakdown   map[string]interface{} `json:"breakdown"`
}

// PersonSplit represents one person's portion of the bill
type PersonSplit struct {
	PersonID   string      `json:"person_id"`
	PersonName string      `json:"name,omitempty"`
	Amount     float64     `json:"total_amount"`
	TipAmount  float64     `json:"tip_amount"`
	Items      []SplitItem `json:"items,omitempty"`
	TaxAmount  float64     `json:"tax_amount"`
	ServiceFee float64     `json:"service_fee_amount"`
	Subtotal   float64     `json:"base_amount"`
}

// SplitItem represents an item in a person's split
type SplitItem struct {
	ItemID   string  `json:"id"`
	Name     string  `json:"name"`
	Price    float64 `json:"price"`
	Quantity int     `json:"quantity"`
	Subtotal float64 `json:"subtotal"`
}

// EqualSplitRequest represents a request for equal bill splitting
type EqualSplitRequest struct {
	BillID    uint              `json:"bill_id"`
	NumPeople int               `json:"num_people"`
	People    map[string]string `json:"people"`
}

// CustomSplitRequest represents a request for custom bill splitting
type CustomSplitRequest struct {
	BillID  uint               `json:"bill_id"`
	Amounts map[string]float64 `json:"amounts"` // person_id -> amount
	People  map[string]string  `json:"people"`  // person_id -> name
}

// ItemSplitRequest represents a request for item-based bill splitting
type ItemSplitRequest struct {
	BillID         uint                `json:"bill_id"`
	ItemSelections map[string][]string `json:"item_selections"` // person_id -> [item_ids]
	People         map[string]string   `json:"people"`          // person_id -> name
}

// allocateCentsEqual divides total cents across n people. The first n-1 people
// receive floor(total/n); the last person receives the remainder so the sum is
// exact. total may be negative only in pathological data — still preserves sum.
func allocateCentsEqual(total int64, n int) []int64 {
	out := make([]int64, n)
	if n <= 0 {
		return out
	}
	base := total / int64(n)
	for i := 0; i < n-1; i++ {
		out[i] = base
	}
	out[n-1] = total - base*int64(n-1)
	return out
}

// allocateProportionalCents divides bucketTotal across people in proportion to
// shares (same order). Integer floor is used for everyone except the last
// person, who receives the remainder so the bucket sums exactly. When the
// share total is zero, every allocation is zero.
//
// FIND-063 residual: used by custom/item splits so tax/fee/base reconcile.
func allocateProportionalCents(bucketTotal int64, shares []int64) []int64 {
	n := len(shares)
	out := make([]int64, n)
	if n == 0 {
		return out
	}
	var shareTotal int64
	for _, s := range shares {
		shareTotal += s
	}
	if shareTotal == 0 || bucketTotal == 0 {
		return out
	}
	var allocated int64
	for i := 0; i < n-1; i++ {
		out[i] = (bucketTotal * shares[i]) / shareTotal
		allocated += out[i]
	}
	out[n-1] = bucketTotal - allocated
	return out
}

// dollarsToCents rounds a dollar float to integer cents (banker's-safe for
// money inputs already quantized to 2 decimals via Round).
func dollarsToCents(dollars float64) int64 {
	return int64(math.Round(dollars * 100))
}

// centsToDollars converts integer cents to a 2-decimal dollar float without a
// second rounding pass (cents are already discrete).
func centsToDollars(cents int64) float64 {
	return float64(cents) / 100.0
}

// CalculateEqualSplit calculates equal split for a bill.
//
// FIND-063: all allocation happens in integer cents so (1) person totals sum to
// the bill total and (2) each person's base+tax+service_fee equals their
// total_amount. Independent float rounding of each bucket violated (2) on
// odd-cent tax/subtotals (e.g. $2.79 tax / 2 people).
func (s *SplittingService) CalculateEqualSplit(billID uint, numPeople int, people map[string]string) (*SplitResult, error) {
	if numPeople <= 0 {
		return nil, errors.New("number of people must be greater than 0")
	}

	bill, err := s.getBillSplitSummary(billID)
	if err != nil {
		return nil, fmt.Errorf("failed to get bill: %w", err)
	}

	payableTotal, payableTax, payableFee, _ := splitPayableBuckets(bill)
	amountCents := allocateCentsEqual(payableTotal, numPeople)
	taxCents := allocateCentsEqual(payableTax, numPeople)
	feeCents := allocateCentsEqual(payableFee, numPeople)
	// Derive subtotal from the other three so components always sum to the
	// person's total. Across people this still sums to bill.Subtotal when
	// TotalAmount = Subtotal + Tax + ServiceFee (tip lives outside total).
	subtotalCents := make([]int64, numPeople)
	for i := 0; i < numPeople; i++ {
		subtotalCents[i] = amountCents[i] - taxCents[i] - feeCents[i]
	}

	providedPersonIDs := sortedPersonIDs(people)

	splits := make([]PersonSplit, numPeople)
	for i := 0; i < numPeople; i++ {
		personID := fmt.Sprintf("person_%d", i+1)
		personName := fmt.Sprintf("Person %d", i+1)
		if len(providedPersonIDs) == numPeople {
			personID = providedPersonIDs[i]
			if people[personID] != "" {
				personName = people[personID]
			} else {
				personName = personID
			}
		}

		splits[i] = PersonSplit{
			PersonID:   personID,
			PersonName: personName,
			Amount:     centsToDollars(amountCents[i]),
			TaxAmount:  centsToDollars(taxCents[i]),
			ServiceFee: centsToDollars(feeCents[i]),
			Subtotal:   centsToDollars(subtotalCents[i]),
		}
	}

	// Breakdown shows the floor share (first person) — last person may differ
	// by the remainder cents. Callers that need exact per-row values use Splits.
	return &SplitResult{
		Method:      "equal",
		TotalAmount: centsToDollars(payableTotal),
		TipAmount:   0,
		GrandTotal:  centsToDollars(payableTotal),
		Splits:      splits,
		CreatedAt:   time.Now().UTC(),
		Breakdown: map[string]interface{}{
			"num_people":             numPeople,
			"amount_per_person":      centsToDollars(amountCents[0]),
			"tax_per_person":         centsToDollars(taxCents[0]),
			"service_fee_per_person": centsToDollars(feeCents[0]),
		},
	}, nil
}

// CalculateCustomSplit calculates custom split for a bill.
//
// FIND-063 residual: tax/fee/base are allocated in integer cents proportional
// to each person's requested total so component lines always sum to that total
// and buckets sum to the bill.
func (s *SplittingService) CalculateCustomSplit(billID uint, amounts map[string]float64, people map[string]string) (*SplitResult, error) {
	if len(amounts) == 0 {
		return nil, errors.New("amounts map cannot be empty")
	}

	bill, err := s.getBillSplitSummary(billID)
	if err != nil {
		return nil, fmt.Errorf("failed to get bill: %w", err)
	}

	// Validate total amounts (float dollars — keep legacy $0.01 tolerance)
	totalCustomAmount := 0.0
	for _, amount := range amounts {
		if amount < 0 {
			return nil, errors.New("amounts cannot be negative")
		}
		totalCustomAmount += amount
	}

	payableTotal, payableTax, payableFee, _ := splitPayableBuckets(bill)
	payableDollars := centsToDollars(payableTotal)
	if math.Abs(totalCustomAmount-payableDollars) > 0.01 {
		label := "bill total"
		if bill.PaidAmount > 0 {
			label = "remaining balance"
		}
		return nil, fmt.Errorf("custom amounts total (%.2f) does not match %s (%.2f)",
			totalCustomAmount, label, payableDollars)
	}

	personIDs := sortedPersonIDs(amounts)
	amountCents := make([]int64, len(personIDs))
	var sumAmountCents int64
	for i, personID := range personIDs {
		amountCents[i] = dollarsToCents(amounts[personID])
		sumAmountCents += amountCents[i]
	}
	// Absorb float→cent drift on the last person so requested totals still
	// cover the payable amount exactly when the float sum was within tolerance.
	if n := len(amountCents); n > 0 && sumAmountCents != payableTotal {
		amountCents[n-1] += payableTotal - sumAmountCents
	}

	taxCents := allocateProportionalCents(payableTax, amountCents)
	feeCents := allocateProportionalCents(payableFee, amountCents)

	splits := make([]PersonSplit, 0, len(personIDs))
	for i, personID := range personIDs {
		personName := people[personID]
		if personName == "" {
			personName = personID
		}
		// Derive base so base+tax+fee always equals the person's total.
		baseCents := amountCents[i] - taxCents[i] - feeCents[i]
		splits = append(splits, PersonSplit{
			PersonID:   personID,
			PersonName: personName,
			Amount:     centsToDollars(amountCents[i]),
			TaxAmount:  centsToDollars(taxCents[i]),
			ServiceFee: centsToDollars(feeCents[i]),
			Subtotal:   centsToDollars(baseCents),
		})
	}

	return &SplitResult{
		Method:      "custom",
		TotalAmount: centsToDollars(payableTotal),
		TipAmount:   0,
		GrandTotal:  centsToDollars(payableTotal),
		Splits:      splits,
		CreatedAt:   time.Now().UTC(),
		Breakdown: map[string]interface{}{
			"num_people":     len(amounts),
			"custom_amounts": amounts,
		},
	}, nil
}

// CalculateItemSplit calculates item-based split for a bill
func (s *SplittingService) CalculateItemSplit(billID uint, itemSelections map[string][]string, people map[string]string) (*SplitResult, error) {
	if len(itemSelections) == 0 {
		return nil, errors.New("item selections cannot be empty")
	}

	bill, err := s.getBillSplitSummary(billID)
	if err != nil {
		return nil, fmt.Errorf("failed to get bill: %w", err)
	}

	billItems, err := database.GetBillItemsPreferRelational(billID)
	if err != nil {
		return nil, fmt.Errorf("failed to get bill items: %w", err)
	}

	// Create item lookup map
	itemMap := make(map[string]database.BillItem)
	for _, item := range billItems {
		itemMap[item.ID] = item
	}

	// Validate all selected items exist
	itemAssignments := make(map[string]string)
	for personID, itemIDs := range itemSelections {
		for _, itemID := range itemIDs {
			if _, exists := itemMap[itemID]; !exists {
				return nil, fmt.Errorf("item with ID %s not found in bill", itemID)
			}
			if assignedTo, exists := itemAssignments[itemID]; exists {
				if assignedTo == personID {
					return nil, fmt.Errorf("item with ID %s is selected multiple times for %s", itemID, personID)
				}
				return nil, fmt.Errorf("item with ID %s is assigned to multiple people", itemID)
			}
			itemAssignments[itemID] = personID
		}
	}

	// Ensure all bill items are selected by someone
	for _, item := range billItems {
		if _, assigned := itemAssignments[item.ID]; !assigned {
			return nil, fmt.Errorf("item %s (%s) is not assigned to anyone", item.ID, item.Name)
		}
	}

	// FIND-063 residual: build base cents from item subtotals, then allocate
	// tax/fee in cents proportional to base so components and bill totals reconcile.
	personIDs := sortedPersonIDs(itemSelections)
	baseCents := make([]int64, len(personIDs))
	personItemsByIndex := make([][]SplitItem, len(personIDs))
	var totalItemsSubtotalCents int64

	for i, personID := range personIDs {
		itemIDs := itemSelections[personID]
		personItems := make([]SplitItem, 0, len(itemIDs))
		var personBase int64
		for _, itemID := range itemIDs {
			item := itemMap[itemID]
			lineCents := dollarsToCents(item.Subtotal)
			personBase += lineCents
			personItems = append(personItems, SplitItem{
				ItemID:   item.ID,
				Name:     item.Name,
				Price:    item.Price,
				Quantity: item.Quantity,
				Subtotal: item.Subtotal,
			})
		}
		baseCents[i] = personBase
		personItemsByIndex[i] = personItems
		totalItemsSubtotalCents += personBase
	}

	payableTotal, payableTax, payableFee, payableSub := splitPayableBuckets(bill)
	taxCents := allocateProportionalCents(payableTax, baseCents)
	feeCents := allocateProportionalCents(payableFee, baseCents)

	// Prefer the payable subtotal as the share basis when item float cents
	// drifted from the stored bill, or when part of the bill is already paid
	// (#522). Tax/fee still come from the payable buckets so person totals
	// sum to remaining, not the gross total.
	targetSub := payableSub
	if targetSub == 0 {
		targetSub = bill.Subtotal
	}
	if totalItemsSubtotalCents != targetSub && totalItemsSubtotalCents > 0 {
		baseCents = allocateProportionalCents(targetSub, baseCents)
		taxCents = allocateProportionalCents(payableTax, baseCents)
		feeCents = allocateProportionalCents(payableFee, baseCents)
		totalItemsSubtotalCents = targetSub
	}

	splits := make([]PersonSplit, 0, len(personIDs))
	for i, personID := range personIDs {
		personName := people[personID]
		if personName == "" {
			personName = personID
		}
		amountCents := baseCents[i] + taxCents[i] + feeCents[i]
		splits = append(splits, PersonSplit{
			PersonID:   personID,
			PersonName: personName,
			Amount:     centsToDollars(amountCents),
			Items:      personItemsByIndex[i],
			TaxAmount:  centsToDollars(taxCents[i]),
			ServiceFee: centsToDollars(feeCents[i]),
			Subtotal:   centsToDollars(baseCents[i]),
		})
	}

	return &SplitResult{
		Method:      "items",
		TotalAmount: centsToDollars(payableTotal),
		TipAmount:   0,
		GrandTotal:  centsToDollars(payableTotal),
		Splits:      splits,
		CreatedAt:   time.Now().UTC(),
		Breakdown: map[string]interface{}{
			"num_people":           len(itemSelections),
			"total_items_subtotal": centsToDollars(totalItemsSubtotalCents),
			"item_assignments":     itemSelections,
		},
	}, nil
}

// roundToTwoDecimals rounds a float64 to 2 decimal places
func roundToTwoDecimals(value float64) float64 {
	return math.Round(value*100) / 100
}

func sortedPersonIDs(collection interface{}) []string {
	var ids []string

	switch typed := collection.(type) {
	case map[string]string:
		ids = make([]string, 0, len(typed))
		for personID := range typed {
			ids = append(ids, personID)
		}
	case map[string]float64:
		ids = make([]string, 0, len(typed))
		for personID := range typed {
			ids = append(ids, personID)
		}
	case map[string][]string:
		ids = make([]string, 0, len(typed))
		for personID := range typed {
			ids = append(ids, personID)
		}
	default:
		return []string{}
	}

	sort.Strings(ids)
	return ids
}
