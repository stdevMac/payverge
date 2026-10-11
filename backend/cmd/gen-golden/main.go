// gen-golden generates the formatter golden files used by the print package tests.
// Usage:
//
//	go run ./cmd/gen-golden                   # emits bill golden (default)
//	go run ./cmd/gen-golden -kind=bill        # same as above
//	go run ./cmd/gen-golden -kind=receipt     # emits receipt golden
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/stdevmac/payverge/backend/internal/services/print/formatters"
)

func main() {
	kind := flag.String("kind", "bill", "golden kind: bill | receipt")
	flag.Parse()

	switch *kind {
	case "bill":
		in := formatters.BillInput{
			BusinessName: "Test Bistro",
			TableName:    "Table 4",
			BillNumber:   "B-0001",
			CreatedAt:    "2026-05-19T19:32:00Z",
			Items: []formatters.BillLineItem{
				{Name: "Burger", Quantity: 2, Subtotal: 24.00},
				{Name: "Fries", Quantity: 1, Subtotal: 5.50},
			},
			Subtotal:         29.50,
			TaxAmount:        2.95,
			ServiceFeeAmount: 1.48,
			Total:            33.93,
			Currency:         "USD",
			Language:         "en",
		}
		out, err := formatters.Bill(in, 80)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Print(out)

	case "receipt":
		in := formatters.ReceiptInput{
			BillInput: formatters.BillInput{
				BusinessName: "Test Bistro",
				TableName:    "Table 4",
				BillNumber:   "B-0001",
				CreatedAt:    "2026-05-19T19:38:00Z",
				Items:        []formatters.BillLineItem{{Name: "Burger", Quantity: 2, Subtotal: 24.00}},
				Subtotal:     24.00,
				TaxAmount:    2.40,
				Total:        29.40,
				Currency:     "USD",
				Language:     "en",
			},
			TipAmount:     3.00,
			PaymentMethod: "card_visa",
			TransactionID: "txn_abc123",
			TotalPaid:     29.40,
		}
		out, err := formatters.Receipt(in, 80)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Print(out)

	default:
		fmt.Fprintf(os.Stderr, "unknown kind %q — use bill or receipt\n", *kind)
		os.Exit(1)
	}
}
