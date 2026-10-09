package print

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/fiscal/display"
	"github.com/stdevmac/payverge/backend/internal/services/print/formatters"
)

// buildBillInputFromBill loads a Bill (with Business and Table preloaded) and
// maps it to a formatters.BillInput. The businessID parameter scopes the query
// as defense-in-depth so cross-tenant source IDs cannot leak data. (PRINT-XTENANT-SOURCEID-1)
func buildBillInputFromBill(ctx context.Context, db *gorm.DB, billID uint, businessID uint, lang string) (formatters.BillInput, error) {
	var bill database.Bill
	if err := db.WithContext(ctx).
		Preload("Business").
		Preload("Table").
		Where("business_id = ?", businessID).
		First(&bill, billID).Error; err != nil {
		return formatters.BillInput{}, fmt.Errorf("load bill %d: %w", billID, err)
	}

	return billInputFromLoadedBill(bill, lang)
}

func billInputFromLoadedBill(bill database.Bill, lang string) (formatters.BillInput, error) {
	items, err := decodeBillItems(bill.Items)
	if err != nil {
		return formatters.BillInput{}, err
	}

	currency := bill.Business.DisplayCurrency
	if currency == "" {
		currency = bill.Business.DefaultCurrency
	}
	if currency == "" {
		currency = "USD"
	}

	// Compose address from the embedded BusinessAddress struct.
	a := bill.Business.Address
	address := strings.TrimSpace(strings.Join([]string{a.Street, a.City, a.State}, " "))

	return formatters.BillInput{
		BusinessName:     bill.Business.Name,
		BusinessAddress:  address,
		TableName:        bill.Table.Name,
		BillNumber:       bill.BillNumber,
		CreatedAt:        formatters.FormatTicketTime(lang, bill.CreatedAt),
		Items:            items,
		Subtotal:         centsToDollars(bill.Subtotal),
		TaxAmount:        centsToDollars(bill.TaxAmount),
		ServiceFeeAmount: centsToDollars(bill.ServiceFeeAmount),
		Total:            centsToDollars(bill.TotalAmount),
		Currency:         currency,
		Language:         lang,
	}, nil
}

// buildReceiptInputFromBill extends buildBillInputFromBill with payment details.
// The businessID parameter scopes the query for defense-in-depth. (PRINT-XTENANT-SOURCEID-1)
func buildReceiptInputFromBill(ctx context.Context, db *gorm.DB, billID uint, businessID uint, lang string) (formatters.ReceiptInput, error) {
	var bill database.Bill
	if err := db.WithContext(ctx).
		Preload("Business").
		Preload("Table").
		Preload("Payments", func(tx *gorm.DB) *gorm.DB {
			return tx.Where("status = ?", database.PaymentStatusConfirmed).
				Order("confirmed_at DESC").
				Order("id DESC")
		}).
		Where("business_id = ?", businessID).
		First(&bill, billID).Error; err != nil {
		return formatters.ReceiptInput{}, fmt.Errorf("load receipt bill %d: %w", billID, err)
	}

	// Alt tenders are best-effort: some test/minimal DBs lack the table; missing
	// cash/card metadata is preferable to failing the whole receipt render.
	var alts []database.AlternativePayment
	_ = db.WithContext(ctx).
		Where("bill_id = ? AND status = ?", billID, database.AltPaymentStatusConfirmed).
		Order("confirmed_at DESC").
		Order("id DESC").
		Find(&alts).Error

	base, err := billInputFromLoadedBill(bill, lang)
	if err != nil {
		return formatters.ReceiptInput{}, err
	}

	paymentMethod, txID := latestTenderDetails(bill.Payments, alts)

	// Ledger is the source of truth: print what was collected, not what was owed.
	// Partial payments must not stamp the full bill total as "Total Paid".
	out := formatters.ReceiptInput{
		BillInput:     base,
		TipAmount:     centsToDollars(bill.TipAmount),
		TotalPaid:     centsToDollars(bill.PaidAmount + bill.TipAmount),
		PaymentMethod: formatters.PaymentMethodLabel(lang, paymentMethod),
		TransactionID: txID,
	}
	// Legal fiscal block (CAE/número/QR/RG 5614) when this bill has an
	// AUTHORIZED AFIP receipt. Best-effort by design: any miss falls back to
	// the plain courtesy ticket — printing must never block on fiscal lookups.
	out.Fiscal = loadFiscalTicketInfo(ctx, db, billID, businessID)
	return out, nil
}

// loadFiscalTicketInfo builds the printed ticket's fiscal block from the
// bill's latest AUTHORIZED fiscal receipt. Returns nil (courtesy ticket) when
// there is no authorized receipt or any lookup fails. Narrow projections only:
// this runs on the print-enqueue hot path (Backend Performance Gate).
func loadFiscalTicketInfo(ctx context.Context, db *gorm.DB, billID, businessID uint) *formatters.FiscalTicketInfo {
	var receipt database.FiscalReceipt
	if err := db.WithContext(ctx).
		Select("id", "receipt_type", "receipt_number", "auth_code", "auth_expires_at", "qr_payload", "total_amount_cents").
		Where("bill_id = ? AND business_id = ? AND status = ?", billID, businessID, database.FiscalStatusAuthorized).
		Order("id DESC").
		First(&receipt).Error; err != nil {
		return nil
	}
	var settings database.BusinessFiscalSettings
	if err := db.WithContext(ctx).
		Select("point_of_sale", "tax_id", "tax_condition").
		Where("business_id = ?", businessID).
		Order("id DESC").
		First(&settings).Error; err != nil {
		return nil
	}

	number := strings.TrimSpace(strDeref(receipt.ReceiptNumber))
	if settings.PointOfSale != nil && number != "" {
		number = fmt.Sprintf("%04d-%s", *settings.PointOfSale, number)
	}
	info := &formatters.FiscalTicketInfo{
		ReceiptTitle:  display.ReceiptTitle(receipt.ReceiptType),
		ReceiptNumber: number,
		EmitterCUIT:   strings.TrimSpace(settings.TaxID),
		EmitterIVA:    display.HumanTaxCondition(settings.TaxCondition),
		CAE:           strings.TrimSpace(strDeref(receipt.AuthCode)),
	}
	if receipt.AuthExpiresAt != nil && !receipt.AuthExpiresAt.IsZero() {
		info.CAEExpiry = receipt.AuthExpiresAt.Format("02/01/2006")
	}
	if display.IsTransparencyLegendType(receipt.ReceiptType) {
		_, vat := display.SplitInclusiveVAT(receipt.TotalAmountCents, display.DefaultArgentinaVATRate)
		info.IVAContained = display.FormatARS(vat)
		info.OtherTaxes = display.FormatARS(0)
	}
	if payload := strings.TrimSpace(strDeref(receipt.QRPayload)); payload != "" {
		if png, err := qrcode.Encode(payload, qrcode.Medium, 256); err == nil {
			info.QRDataURI = template.URL("data:image/png;base64," + base64.StdEncoding.EncodeToString(png))
		}
	}
	return info
}

// strDeref returns the dereferenced string or "" for nil.
func strDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// latestTenderDetails picks the most recently confirmed tender across crypto
// Payments and AlternativePayments (cash/card/venmo/etc.) for method + external id.
func latestTenderDetails(payments []database.Payment, alts []database.AlternativePayment) (method, txID string) {
	type tender struct {
		method string
		txID   string
		at     int64
		id     uint
	}
	var best *tender
	consider := func(t tender) {
		if best == nil || t.at > best.at || (t.at == best.at && t.id > best.id) {
			cp := t
			best = &cp
		}
	}
	for _, p := range payments {
		if p.Status != database.PaymentStatusConfirmed {
			continue
		}
		at := int64(0)
		if p.ConfirmedAt != nil {
			at = p.ConfirmedAt.UnixNano()
		} else {
			at = p.CreatedAt.UnixNano()
		}
		consider(tender{method: p.PaymentMethod, txID: p.TxHash, at: at, id: p.ID})
	}
	for _, ap := range alts {
		if ap.Status != database.AltPaymentStatusConfirmed {
			continue
		}
		at := int64(0)
		if ap.ConfirmedAt != nil {
			at = ap.ConfirmedAt.UnixNano()
		} else {
			at = ap.CreatedAt.UnixNano()
		}
		// Alt tenders (cash/card/venmo) have no on-chain tx hash; method label is enough.
		consider(tender{method: string(ap.PaymentMethod), txID: "", at: at, id: ap.ID})
	}
	if best == nil {
		return "", ""
	}
	return best.method, best.txID
}

// buildKitchenTicketInputFromOrder loads an Order (with Bill+Business+Table
// preloaded) and maps each OrderItem to a KitchenLineItem. Modifiers come
// from OrderItem.Options (flat label list), Notes from OrderItem.SpecialRequests,
// and Allergens are joined from the active Menu by MenuItemID. IMP-13.
// The businessID parameter scopes the query for defense-in-depth. (PRINT-XTENANT-SOURCEID-1)
func buildKitchenTicketInputFromOrder(ctx context.Context, db *gorm.DB, orderID uint, businessID uint, lang string) (formatters.KitchenTicketInput, error) {
	var order database.Order
	if err := db.WithContext(ctx).
		Preload("Bill.Table").
		Preload("Business").
		Where("business_id = ?", businessID).
		First(&order, orderID).Error; err != nil {
		return formatters.KitchenTicketInput{}, fmt.Errorf("load order %d: %w", orderID, err)
	}

	orderItems, err := decodeOrderItems(order.Items)
	if err != nil {
		return formatters.KitchenTicketInput{}, err
	}

	// Build a menu_item_id → allergens lookup from the active menu. We tolerate
	// missing menus (the ticket still renders without allergens — fail open
	// rather than block the kitchen on a menu lookup).
	allergensByItemID := loadAllergenIndex(ctx, db, order.BusinessID)

	lines := make([]formatters.KitchenLineItem, 0, len(orderItems))
	for _, oi := range orderItems {
		mods := make([]string, 0, len(oi.Options))
		for _, opt := range oi.Options {
			if opt.Name != "" {
				mods = append(mods, opt.Name)
			}
		}
		var allergens []string
		if oi.MenuItemID != "" {
			allergens = allergensByItemID[oi.MenuItemID]
		}
		lines = append(lines, formatters.KitchenLineItem{
			Name:      oi.MenuItemName,
			Quantity:  oi.Quantity,
			Modifiers: mods,
			Notes:     oi.SpecialRequests,
			Allergens: allergens,
		})
	}

	tableName := ""
	if order.Bill.Table.ID != 0 {
		tableName = order.Bill.Table.Name
	}

	return formatters.KitchenTicketInput{
		BusinessName: order.Business.Name,
		TableName:    tableName,
		BillNumber:   order.Bill.BillNumber,
		OrderNumber:  order.OrderNumber,
		CreatedAt:    formatters.FormatTicketTime(lang, order.CreatedAt),
		OrderNotes:   order.Notes,
		Items:        lines,
		Language:     lang,
	}, nil
}

// loadAllergenIndex returns a map of menu_item_id → allergens for the
// business's active menu. Returns an empty map (never nil) on any failure so
// the caller can safely lookup without nil checks.
func loadAllergenIndex(ctx context.Context, db *gorm.DB, businessID uint) map[string][]string {
	out := make(map[string][]string)
	var menu database.Menu
	if err := db.WithContext(ctx).
		Where("business_id = ? AND is_active = ?", businessID, true).
		First(&menu).Error; err != nil {
		return out
	}
	if strings.TrimSpace(menu.Categories) == "" {
		return out
	}
	var cats []database.MenuCategory
	if err := json.Unmarshal([]byte(menu.Categories), &cats); err != nil {
		return out
	}
	for _, c := range cats {
		for _, it := range c.Items {
			if it.ID == "" || len(it.Allergens) == 0 {
				continue
			}
			out[it.ID] = it.Allergens
		}
	}
	return out
}

// decodeOrderItems JSON-decodes the Order.Items string into an OrderItem slice.
func decodeOrderItems(raw string) ([]database.OrderItem, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var items []database.OrderItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("decode order items: %w", err)
	}
	return items, nil
}

// decodeBillItems JSON-decodes the Bill.Items string into BillLineItem slice.
func decodeBillItems(raw string) ([]formatters.BillLineItem, error) {
	if raw == "" {
		return nil, nil
	}
	type rawItem struct {
		Name           string  `json:"name"`
		Quantity       int     `json:"quantity"`
		Subtotal       float64 `json:"subtotal"`
		ItemType       string  `json:"item_type"`
		BundleID       *uint   `json:"bundle_id"`
		ParentBundleID *uint   `json:"parent_bundle_id"`
		OrderID        *uint   `json:"order_id"`
		OccurrenceID   string  `json:"bundle_occurrence_id"`
	}
	var parsed []rawItem
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("decode bill items: %w", err)
	}
	baseKey := func(bundleID uint, orderID *uint) string {
		order := uint(0)
		if orderID != nil {
			order = *orderID
		}
		return fmt.Sprintf("%d:%d", order, bundleID)
	}
	occurrenceByIndex := make([]string, len(parsed))
	bundleParents := make(map[string]struct{})
	bundleChildren := make(map[string][]rawItem)
	// Explicit occurrence IDs are generated on new orders and remain correct
	// even when identical bundles appear multiple times in one order.
	for _, item := range parsed {
		if strings.EqualFold(strings.TrimSpace(item.ItemType), "bundle") && item.BundleID != nil && item.OccurrenceID != "" {
			bundleParents["explicit:"+item.OccurrenceID] = struct{}{}
		}
	}
	// Snapshots without an occurrence ID (manual or hand-built rows) keep the
	// promotion expansion order of parent followed by its children, so associate
	// each child with the nearest preceding matching parent. Children before the
	// first parent are buffered until one appears.
	latestLegacy := make(map[string]string)
	legacyCounts := make(map[string]int)
	pendingLegacy := make(map[string][]int)
	for index, item := range parsed {
		itemType := strings.ToLower(strings.TrimSpace(item.ItemType))
		if itemType == "bundle" && item.BundleID != nil {
			if item.OccurrenceID != "" {
				occurrenceByIndex[index] = "explicit:" + item.OccurrenceID
				continue
			}
			base := baseKey(*item.BundleID, item.OrderID)
			legacyCounts[base]++
			key := fmt.Sprintf("legacy:%s:%d", base, legacyCounts[base])
			occurrenceByIndex[index] = key
			bundleParents[key] = struct{}{}
			latestLegacy[base] = key
			for _, childIndex := range pendingLegacy[base] {
				occurrenceByIndex[childIndex] = key
				bundleChildren[key] = append(bundleChildren[key], parsed[childIndex])
			}
			delete(pendingLegacy, base)
			continue
		}
		if itemType != "bundle_item" || item.ParentBundleID == nil {
			continue
		}
		if item.OccurrenceID != "" {
			key := "explicit:" + item.OccurrenceID
			if _, ok := bundleParents[key]; ok {
				occurrenceByIndex[index] = key
				bundleChildren[key] = append(bundleChildren[key], item)
			}
			continue
		}
		base := baseKey(*item.ParentBundleID, item.OrderID)
		if key := latestLegacy[base]; key != "" {
			occurrenceByIndex[index] = key
			bundleChildren[key] = append(bundleChildren[key], item)
		} else {
			pendingLegacy[base] = append(pendingLegacy[base], index)
		}
	}

	out := make([]formatters.BillLineItem, 0, len(parsed))
	for index, item := range parsed {
		itemType := strings.ToLower(strings.TrimSpace(item.ItemType))
		occurrenceKey := occurrenceByIndex[index]
		if itemType == "bundle_item" && occurrenceKey != "" {
			continue
		}
		kind := formatters.BillLineKindItem
		if itemType == "discount" {
			kind = formatters.BillLineKindDiscount
		}
		out = append(out, formatters.BillLineItem{
			Name:     item.Name,
			Quantity: item.Quantity,
			Subtotal: item.Subtotal,
			Kind:     kind,
		})
		if itemType == "bundle" && occurrenceKey != "" {
			for _, child := range bundleChildren[occurrenceKey] {
				out = append(out, formatters.BillLineItem{
					Name:     child.Name,
					Quantity: child.Quantity,
					Kind:     formatters.BillLineKindDetail,
				})
			}
		}
	}
	return out, nil
}

// centsToDollars converts int64 cents to float64 dollars.
func centsToDollars(c int64) float64 {
	return float64(c) / 100.0
}
