package splitting

import (
	"errors"
	"fmt"
	"math"
	"payverge/internal/database"
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

// SplitResult represents the result of a bill split calculation
type SplitResult struct {
	Method      string                 `json:"method"`
	TotalAmount float64                `json:"total_amount"`
	Splits      []PersonSplit          `json:"splits"`
	Breakdown   map[string]interface{} `json:"breakdown"`
}

// PersonSplit represents one person's portion of the bill
type PersonSplit struct {
	PersonID    string  `json:"person_id"`
	PersonName  string  `json:"person_name,omitempty"`
	Amount      float64 `json:"amount"`
	Items       []SplitItem `json:"items,omitempty"`
	TaxAmount   float64 `json:"tax_amount"`
	ServiceFee  float64 `json:"service_fee"`
	Subtotal    float64 `json:"subtotal"`
}

// SplitItem represents an item in a person's split
type SplitItem struct {
	ItemID   string  `json:"item_id"`
	Name     string  `json:"name"`
	Price    float64 `json:"price"`
	Quantity int     `json:"quantity"`
	Subtotal float64 `json:"subtotal"`
}

// EqualSplitRequest represents a request for equal bill splitting
type EqualSplitRequest struct {
	BillID    uint `json:"bill_id"`
	NumPeople int  `json:"num_people"`
}

// CustomSplitRequest represents a request for custom bill splitting
type CustomSplitRequest struct {
	BillID  uint                   `json:"bill_id"`
	Amounts map[string]float64     `json:"amounts"` // person_id -> amount
	People  map[string]string      `json:"people"`  // person_id -> name
}

// ItemSplitRequest represents a request for item-based bill splitting
type ItemSplitRequest struct {
	BillID         uint                       `json:"bill_id"`
	ItemSelections map[string][]string        `json:"item_selections"` // person_id -> [item_ids]
	People         map[string]string          `json:"people"`          // person_id -> name
}

// CalculateEqualSplit calculates equal split for a bill
func (s *SplittingService) CalculateEqualSplit(billID uint, numPeople int) (*SplitResult, error) {
	if numPeople <= 0 {
		return nil, errors.New("number of people must be greater than 0")
	}

	// Get bill from database
	bill, err := s.db.GetBill(billID)
	if err != nil {
		return nil, fmt.Errorf("failed to get bill: %w", err)
	}

	// Calculate equal split amount
	amountPerPerson := bill.TotalAmount / float64(numPeople)
	taxPerPerson := bill.TaxAmount / float64(numPeople)
	serviceFeePerPerson := bill.ServiceFeeAmount / float64(numPeople)
	subtotalPerPerson := bill.Subtotal / float64(numPeople)

	// Create splits
	splits := make([]PersonSplit, numPeople)
	for i := 0; i < numPeople; i++ {
		splits[i] = PersonSplit{
			PersonID:   fmt.Sprintf("person_%d", i+1),
			PersonName: fmt.Sprintf("Person %d", i+1),
			Amount:     roundToTwoDecimals(amountPerPerson),
			TaxAmount:  roundToTwoDecimals(taxPerPerson),
			ServiceFee: roundToTwoDecimals(serviceFeePerPerson),
			Subtotal:   roundToTwoDecimals(subtotalPerPerson),
		}
	}

	// Handle rounding differences by adjusting the last person's amount
	totalSplitAmount := 0.0
	for _, split := range splits {
		totalSplitAmount += split.Amount
	}
	
	if diff := bill.TotalAmount - totalSplitAmount; diff != 0 {
		splits[len(splits)-1].Amount = roundToTwoDecimals(splits[len(splits)-1].Amount + diff)
	}

	return &SplitResult{
		Method:      "equal",
		TotalAmount: bill.TotalAmount,
		Splits:      splits,
		Breakdown: map[string]interface{}{
			"num_people":           numPeople,
			"amount_per_person":    roundToTwoDecimals(amountPerPerson),
			"tax_per_person":       roundToTwoDecimals(taxPerPerson),
			"service_fee_per_person": roundToTwoDecimals(serviceFeePerPerson),
		},
	}, nil
}

// CalculateCustomSplit calculates custom split for a bill
func (s *SplittingService) CalculateCustomSplit(billID uint, amounts map[string]float64, people map[string]string) (*SplitResult, error) {
	if len(amounts) == 0 {
		return nil, errors.New("amounts map cannot be empty")
	}

	// Get bill from database
	bill, err := s.db.GetBill(billID)
	if err != nil {
		return nil, fmt.Errorf("failed to get bill: %w", err)
	}

	// Validate total amounts
	totalCustomAmount := 0.0
	for _, amount := range amounts {
		if amount < 0 {
			return nil, errors.New("amounts cannot be negative")
		}
		totalCustomAmount += amount
	}

	if math.Abs(totalCustomAmount-bill.TotalAmount) > 0.01 {
		return nil, fmt.Errorf("custom amounts total (%.2f) does not match bill total (%.2f)", 
			totalCustomAmount, bill.TotalAmount)
	}

	// Create splits
	splits := make([]PersonSplit, 0, len(amounts))
	for personID, amount := range amounts {
		personName := people[personID]
		if personName == "" {
			personName = personID
		}

		// Calculate proportional tax and service fee
		proportion := amount / bill.TotalAmount
		taxAmount := bill.TaxAmount * proportion
		serviceFee := bill.ServiceFeeAmount * proportion
		subtotal := bill.Subtotal * proportion

		splits = append(splits, PersonSplit{
			PersonID:   personID,
			PersonName: personName,
			Amount:     roundToTwoDecimals(amount),
			TaxAmount:  roundToTwoDecimals(taxAmount),
			ServiceFee: roundToTwoDecimals(serviceFee),
			Subtotal:   roundToTwoDecimals(subtotal),
		})
	}

	return &SplitResult{
		Method:      "custom",
		TotalAmount: bill.TotalAmount,
		Splits:      splits,
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

	// Get bill from database
	bill, err := s.db.GetBill(billID)
	if err != nil {
		return nil, fmt.Errorf("failed to get bill: %w", err)
	}

	// Parse bill items
	billItems, err := s.db.GetBillItems(billID)
	if err != nil {
		return nil, fmt.Errorf("failed to get bill items: %w", err)
	}

	// Create item lookup map
	itemMap := make(map[string]database.BillItem)
	for _, item := range billItems {
		itemMap[item.ID] = item
	}

	// Validate all selected items exist
	allSelectedItems := make(map[string]bool)
	for _, itemIDs := range itemSelections {
		for _, itemID := range itemIDs {
			if _, exists := itemMap[itemID]; !exists {
				return nil, fmt.Errorf("item with ID %s not found in bill", itemID)
			}
			allSelectedItems[itemID] = true
		}
	}

	// Ensure all bill items are selected by someone
	for _, item := range billItems {
		if !allSelectedItems[item.ID] {
			return nil, fmt.Errorf("item %s (%s) is not assigned to anyone", item.ID, item.Name)
		}
	}

	// Calculate splits
	splits := make([]PersonSplit, 0, len(itemSelections))
	totalItemsSubtotal := 0.0

	for personID, itemIDs := range itemSelections {
		personName := people[personID]
		if personName == "" {
			personName = personID
		}

		personSubtotal := 0.0
		personItems := make([]SplitItem, 0, len(itemIDs))

		for _, itemID := range itemIDs {
			item := itemMap[itemID]
			personSubtotal += item.Subtotal
			
			personItems = append(personItems, SplitItem{
				ItemID:   item.ID,
				Name:     item.Name,
				Price:    item.Price,
				Quantity: item.Quantity,
				Subtotal: item.Subtotal,
			})
		}

		totalItemsSubtotal += personSubtotal

		// Calculate proportional tax and service fee
		proportion := personSubtotal / bill.Subtotal
		taxAmount := bill.TaxAmount * proportion
		serviceFee := bill.ServiceFeeAmount * proportion
		totalAmount := personSubtotal + taxAmount + serviceFee

		splits = append(splits, PersonSplit{
			PersonID:   personID,
			PersonName: personName,
			Amount:     roundToTwoDecimals(totalAmount),
			Items:      personItems,
			TaxAmount:  roundToTwoDecimals(taxAmount),
			ServiceFee: roundToTwoDecimals(serviceFee),
			Subtotal:   roundToTwoDecimals(personSubtotal),
		})
	}

	return &SplitResult{
		Method:      "items",
		TotalAmount: bill.TotalAmount,
		Splits:      splits,
		Breakdown: map[string]interface{}{
			"num_people":           len(itemSelections),
			"total_items_subtotal": totalItemsSubtotal,
			"item_assignments":     itemSelections,
		},
	}, nil
}

// roundToTwoDecimals rounds a float64 to 2 decimal places
func roundToTwoDecimals(value float64) float64 {
	return math.Round(value*100) / 100
}
