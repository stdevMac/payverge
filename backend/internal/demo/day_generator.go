package demo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/fiscal"
)

func (s *Service) generateDays(ctx context.Context, tx *gorm.DB, instance *database.DemoInstance, start, end time.Time) error {
	businesses := []struct {
		id      *uint
		profile profile
	}{
		{instance.PrimaryBusinessID, profiles()[0]},
		{instance.SecondaryBusinessID, profiles()[1]},
	}
	for _, spec := range businesses {
		if spec.id == nil {
			return fmt.Errorf("missing business for profile %s", spec.profile.Key)
		}
		// A reservation carries the guest locale that host texts, confirmations
		// and the name board follow, so the seeder has to know the venue's
		// default language before it writes one (issue 853). Resolved once per
		// business, not once per simulated day.
		var venue database.Business
		if err := tx.WithContext(ctx).Select("id", "default_language", "source_language").First(&venue, *spec.id).Error; err != nil {
			return err
		}
		language := demoLocale(&venue)
		for _, day := range enumerateDays(start, end) {
			if err := s.generateBusinessDay(ctx, tx, instance.AdminUserID, *spec.id, spec.profile, day, language); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) generateBusinessDay(ctx context.Context, tx *gorm.DB, adminUserID, businessID uint, p profile, day time.Time, language string) error {
	if err := s.generateReservationsForDay(ctx, tx, businessID, day, language); err != nil {
		return err
	}
	rng := s.rng(adminUserID, p.Key, day, "bills")
	count := 4 + rng.Intn(3)
	if p.Key == "secondary" {
		count = 6 + rng.Intn(4)
	}
	for i := 0; i < count; i++ {
		// Each bill draws from its own stream. generateBill returns early for a
		// bill that already exists or has not closed yet, so a shared stream
		// would shift every later bill's close time on the hourly re-run and
		// break append idempotency.
		billRNG := s.rng(adminUserID, p.Key, day, fmt.Sprintf("bill:%d", i))
		if err := s.generateBill(ctx, tx, adminUserID, businessID, p, day, i, billRNG); err != nil {
			return err
		}
	}
	// The caja session derives its cash_sales from the day's confirmed cash
	// payments, so bills must exist first (#796).
	if err := s.generateCashSessionForDay(ctx, tx, businessID, day); err != nil {
		return err
	}
	return nil
}

func (s *Service) generateBill(ctx context.Context, tx *gorm.DB, adminUserID, businessID uint, p profile, day time.Time, index int, rng *rand.Rand) error {
	// Porteño service curve: lunch 12:00–14:00, dinner 20:00–21:00 (weighted to
	// dinner — nobody eats at 17:30 in Buenos Aires).
	billStartHours := [...]int{12, 13, 13, 14, 20, 20, 21, 21}
	createdAt := day.Add(time.Duration(billStartHours[rng.Intn(len(billStartHours))]) * time.Hour).Add(time.Duration(rng.Intn(50)) * time.Minute)
	closedAt := createdAt.Add(time.Duration(45+rng.Intn(70)) * time.Minute)
	// A bill only exists once it has closed. Writing today's full day up
	// front meant a 10am briefing divided the whole day's revenue by a ~0.1
	// elapsed-day fraction and read "964% ahead of a typical Friday". The
	// hourly append re-runs today (deterministic rng + bill_number skip), so
	// skipped bills materialize as showroom time passes them.
	if closedAt.After(s.now()) {
		return nil
	}
	// Real-path style (B{id}-{12hex}); deterministic for idempotent hourly append.
	billNumber := demoBillNumber(businessID, adminUserID, day.Format("20060102"), index+1)

	var existing database.Bill
	err := tx.WithContext(ctx).Where("bill_number = ?", billNumber).First(&existing).Error
	if err == nil {
		return nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}

	tableID, err := nthTableID(ctx, tx, businessID, index)
	if err != nil {
		return err
	}
	staffID, err := nthStaffID(ctx, tx, businessID, index)
	if err != nil {
		return err
	}
	customer, customerBusiness, err := s.ensureCustomer(ctx, tx, adminUserID, businessID, index)
	if err != nil {
		return err
	}
	lines := s.linesForBill(p, index, rng)
	// AR carta prices are IVA-final: tax is inside the listed price, so the
	// bill applies no added tax rate (TaxRate 0 on the business too).
	total := computeTotals(lines, 0, p.ServiceFeeRate)
	itemsJSON, err := json.Marshal(lines)
	if err != nil {
		return err
	}
	// Rotate the customer fiscal identity so AFIP receipt types vary the way a
	// real salon does: responsable inscripto with CUIT (factura A), consumidor
	// final with DNI (factura B), consumidor final without a document. Both
	// documents are FAKE placeholders (fake_contact.go); CUITs still must pass
	// the fiscal receiver's mod-11 check.
	docType, docNum, taxCondition, customerName := "CUIT", demoValidCUIT(30, 5000000+index), "responsable_inscripto", customer.Name
	switch index % 3 {
	case 1:
		docType, docNum, taxCondition = "DNI", fmt.Sprintf("%08d", 30111222+index*7), "consumidor_final"
	case 2:
		docType, docNum, taxCondition = "", "", "consumidor_final"
	}
	var docTypePtr, docNumPtr *string
	if docType != "" {
		docTypePtr, docNumPtr = &docType, &docNum
	}
	bill := database.Bill{
		BusinessID:                 businessID,
		TableID:                    tableID,
		BillNumber:                 billNumber,
		Notes:                      "Cuenta de salón (demo)",
		Items:                      string(itemsJSON),
		Subtotal:                   total.SubtotalCents,
		TaxAmount:                  total.TaxCents,
		ServiceFeeAmount:           total.ServiceCents,
		TotalAmount:                total.TotalCents,
		PaidAmount:                 total.TotalCents,
		TipAmount:                  int64(math.Round(float64(total.SubtotalCents) * 0.10)),
		Status:                     database.BillStatusPaid,
		SettlementAddr:             "",
		CreatedByStaffID:           &staffID,
		ClosedByStaffID:            &staffID,
		CRMCustomerID:              &customer.ID,
		FiscalCustomerDocType:      docTypePtr,
		FiscalCustomerDocNumber:    docNumPtr,
		FiscalCustomerTaxCondition: &taxCondition,
		FiscalCustomerName:         &customerName,
		CreatedAt:                  createdAt,
		UpdatedAt:                  closedAt,
		ClosedAt:                   &closedAt,
	}
	if err := tx.WithContext(ctx).Create(&bill).Error; err != nil {
		return err
	}
	if err := s.createBillItems(ctx, tx, &bill, lines, createdAt); err != nil {
		return err
	}
	// AR payment mix (ordered; first match wins). Mercado Pago is the default
	// rail — that's how porteños actually pay — with cash and card alternative
	// payments keeping the caja honest (#796). No crypto tenders: demo venues
	// seed no settlement wallet and no enabled crypto rail, so their books must
	// not claim on-chain settlements (#795, see createPayment). The default MP
	// branch fires at least once per day, so verifier floors on `payments` and
	// per-day alerts hold even though cash/card bills carry no Payment row.
	var payment *database.Payment
	switch {
	case index%3 == 0:
		err = s.createConfirmedAlternativePayment(ctx, tx, &bill, total.TotalCents, database.PaymentMethodCash, closedAt)
	case index%5 == 0:
		// Split table: 60% through Mercado Pago, the rest in cash.
		mpAmount := total.TotalCents * 60 / 100
		payment, err = s.createPayment(ctx, tx, &bill, adminUserID, mpAmount, index, "mercadopago", closedAt)
		if err == nil {
			err = s.createConfirmedAlternativePayment(ctx, tx, &bill, total.TotalCents-mpAmount, database.PaymentMethodCash, closedAt)
		}
	case index%7 == 0:
		// Counter-recorded card charge (#795) — a Payment row, not an
		// AlternativePayment, so this index slot also feeds the ≥8 distinct
		// payer-pool floor in TestSeededPaymentsHavePayerPoolAndNonExplorerTxHashes.
		payment, err = s.createPayment(ctx, tx, &bill, adminUserID, total.TotalCents, index, "card", closedAt)
	default:
		payment, err = s.createPayment(ctx, tx, &bill, adminUserID, total.TotalCents, index, "mercadopago", closedAt)
	}
	if err != nil {
		return err
	}
	order, err := s.createOrder(ctx, tx, &bill, lines, staffID, createdAt, closedAt)
	if err != nil {
		return err
	}
	if err := s.createCRMVisit(ctx, tx, customerBusiness.ID, &bill, tableID, total, lines, closedAt); err != nil {
		return err
	}
	if err := s.createInventoryMovements(ctx, tx, businessID, &bill, order.ID, lines, createdAt); err != nil {
		return err
	}
	if index%3 == 0 {
		if err := s.createDelivery(ctx, tx, businessID, &bill, order, customer, closedAt); err != nil {
			return err
		}
	}
	if err := s.createFiscalAndPrint(ctx, tx, businessID, &bill, payment, closedAt); err != nil {
		return err
	}
	if err := s.createBillHistoryAndAlert(ctx, tx, businessID, &bill, payment, staffID, createdAt); err != nil {
		return err
	}
	if err := s.createTimeEntry(ctx, tx, businessID, staffID, createdAt); err != nil {
		return err
	}
	return nil
}

func (s *Service) linesForBill(p profile, index int, rng *rand.Rand) []billLine {
	// Every line sells at its real MENU price — analytics, menu engineering,
	// and the Director all derive "underpriced / avg price" reads from these
	// rows, so synthetic prices (the old AverageBillCents backsolve) made the
	// showroom claim a $42 steak sells for $83 and propose a $99 reprice.
	if p.Key == "secondary" {
		// Parrilla Quebracho Azul: tables of two-plus ordering the star cut. Avg
		// ticket lands near the profile's AR$ 120.000.
		lines := []billLine{
			{MenuItemID: "demo-bife", Name: "Bife de chorizo", Price: 34000, Quantity: 2 + rng.Intn(2)},
			{MenuItemID: "demo-malbec-copa", Name: "Copa de Malbec", Price: 6500, Quantity: 2 + rng.Intn(3)},
		}
		if index%2 == 0 {
			lines = append(lines, billLine{MenuItemID: "demo-flan", Name: "Flan casero", Price: 8900, Quantity: 2})
		}
		if index%5 == 0 {
			// The ojo de bife is the "strong margin, sells slowly" puzzle —
			// it needs occasional real sales or every analytics surface
			// renders it as an AR$ 0 zero-sold row.
			lines = append(lines, billLine{MenuItemID: "demo-ojo-de-bife", Name: "Ojo de bife", Price: 39500, Quantity: 1})
		}
		return withLineSubtotals(lines)
	}
	// Bodegón Mesa Larga: milanesa-and-fritas territory, avg near AR$ 60.000.
	lines := []billLine{
		{MenuItemID: "demo-milanesa", Name: "Milanesa napolitana", Price: 21500, Quantity: 1 + rng.Intn(2)},
		{MenuItemID: "demo-fritas", Name: "Papas fritas", Price: 8900, Quantity: 1 + rng.Intn(2)},
	}
	if index%2 == 0 {
		lines = append(lines, billLine{MenuItemID: "demo-fernet", Name: "Fernet con coca", Price: 7800, Quantity: 1})
	}
	if index%3 == 0 {
		lines = append(lines, billLine{MenuItemID: "demo-choripan", Name: "Choripán", Price: 7500, Quantity: 1})
	}
	if index%4 == 1 {
		lines = append(lines, billLine{MenuItemID: "demo-ensalada", Name: "Ensalada mixta", Price: 12500, Quantity: 1})
	}
	return withLineSubtotals(lines)
}

func withLineSubtotals(lines []billLine) []billLine {
	for i := range lines {
		lines[i].Subtotal = lines[i].Price * float64(lines[i].Quantity)
	}
	return lines
}

func nthTableID(ctx context.Context, tx *gorm.DB, businessID uint, index int) (uint, error) {
	var tables []database.Table
	if err := tx.WithContext(ctx).Where("business_id = ? AND is_active = ?", businessID, true).Order("id asc").Find(&tables).Error; err != nil {
		return 0, err
	}
	if len(tables) == 0 {
		return 0, fmt.Errorf("no demo tables for business %d", businessID)
	}
	return tables[index%len(tables)].ID, nil
}

func nthStaffID(ctx context.Context, tx *gorm.DB, businessID uint, index int) (uint, error) {
	var staff []database.Staff
	if err := tx.WithContext(ctx).Where("business_id = ? AND is_active = ?", businessID, true).Order("id asc").Find(&staff).Error; err != nil {
		return 0, err
	}
	if len(staff) == 0 {
		return 0, fmt.Errorf("no demo staff for business %d", businessID)
	}
	return staff[index%len(staff)].ID, nil
}

func (s *Service) ensureCustomer(ctx context.Context, tx *gorm.DB, adminUserID, businessID uint, index int) (*database.Customer, *database.CustomerBusiness, error) {
	email := s.demoCustomerEmail(adminUserID, businessID, index%12)
	now := s.now().UTC()
	var customer database.Customer
	err := tx.WithContext(ctx).Where("email = ?", email).First(&customer).Error
	// Use the same payer-pool wallets payments write (slot = index + businessID,
	// mirroring createPayment) so Analytics Top Tippers can join CRM guest
	// names instead of raw 0x addresses.
	wantWallet := demoPayerPool[(index+int(businessID))%len(demoPayerPool)]
	if errors.Is(err, gorm.ErrRecordNotFound) {
		customer = database.Customer{
			Email:         email,
			Name:          demoCustomerNames[index%12],
			Phone:         demoFakeMobile(3100 + index%12),
			WalletAddress: wantWallet,
			IsActive:      true,
			EmailVerified: true,
			LastLoginAt:   &now,
		}
		if err := tx.WithContext(ctx).Create(&customer).Error; err != nil {
			return nil, nil, err
		}
	} else if err != nil {
		return nil, nil, err
	} else if customer.WalletAddress != wantWallet {
		if err := tx.WithContext(ctx).Model(&customer).Update("wallet_address", wantWallet).Error; err != nil {
			return nil, nil, err
		}
		customer.WalletAddress = wantWallet
	}
	// Lifetime spend must land inside the seeded ladder band for the assigned
	// tier (AR$ 0 / 150.000 / 450.000 thresholds) — otherwise the CRM list
	// chips contradict the Loyalty tab's ladder preview. Values are pesos.
	totalSpent := []float64{80000, 210000, 520000}[index%3] + float64((index%12)*1500)
	// Points track spend at the demo earn rate (0.01 pts/peso). Using a
	// spend-independent formula made high spenders look poorer on points.
	const demoEarnRate = 0.01
	loyaltyPoints := int(totalSpent * demoEarnRate)
	loyaltyTier := []string{"Bronce", "Plata", "Oro"}[index%3]
	allergies := `[]`
	if index%4 == 0 {
		allergies = `["mariscos"]`
	}

	var cb database.CustomerBusiness
	err = tx.WithContext(ctx).Where("customer_id = ? AND business_id = ?", customer.ID, businessID).First(&cb).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		cb = database.CustomerBusiness{
			CustomerID:         customer.ID,
			BusinessID:         businessID,
			LoyaltyPoints:      loyaltyPoints,
			LoyaltyTier:        loyaltyTier,
			TotalSpent:         totalSpent,
			VisitCount:         3 + index%9,
			LastVisitAt:        &now,
			FirstVisitAt:       now.AddDate(0, -2, 0),
			OptInMarketing:     true,
			OptInSMS:           index%2 == 0,
			OptInEmail:         true,
			FavoriteItems:      `["demo-bife","demo-flan"]`,
			DietaryPreferences: `["parrilla"]`,
			Allergies:          allergies,
			Notes:              "Cliente frecuente del salón (demo).",
			Tags:               `["vip","demo"]`,
			IsActive:           true,
		}
		if err := tx.WithContext(ctx).Create(&cb).Error; err != nil {
			return nil, nil, err
		}
	} else if err != nil {
		return nil, nil, err
	} else {
		// Reseed keeps points/spend coherent for existing demo rows so the
		// CRM roster never shows inverse points-vs-spend after a refresh.
		if err := tx.WithContext(ctx).Model(&cb).Updates(map[string]interface{}{
			"loyalty_points": loyaltyPoints,
			"loyalty_tier":   loyaltyTier,
			"total_spent":    totalSpent,
			"allergies":      allergies,
			"is_active":      true,
		}).Error; err != nil {
			return nil, nil, err
		}
		cb.LoyaltyPoints = loyaltyPoints
		cb.LoyaltyTier = loyaltyTier
		cb.TotalSpent = totalSpent
		cb.Allergies = allergies
	}
	var prefs database.CustomerPreferences
	err = tx.WithContext(ctx).Where("customer_id = ?", customer.ID).First(&prefs).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		prefs = database.CustomerPreferences{CustomerID: customer.ID, PreferredLanguage: "es", PreferredCurrency: "ARS", ReceivePromotions: true, ReceiveNewsletters: true, ReceiveBirthdayOffers: true, ShareDataWithBusinesses: true}
		if err := tx.WithContext(ctx).Create(&prefs).Error; err != nil {
			return nil, nil, err
		}
	} else if err != nil {
		return nil, nil, err
	}
	addressBusinessID := businessID
	lat, lng := -34.6205, -58.3719
	address := database.CustomerAddress{
		CustomerID:           customer.ID,
		BusinessID:           &addressBusinessID,
		Label:                "Casa",
		Street:               demoFakeStreet("Estados Unidos", 520+(index%12)*20),
		Apartment:            fmt.Sprintf("%d°%s", 2+index%5, []string{"A", "B", "C"}[index%3]),
		City:                 "Buenos Aires",
		State:                "CABA",
		PostalCode:           "C1065",
		Country:              "AR",
		FormattedAddress:     demoFakeStreet("Estados Unidos", 520+(index%12)*20) + ", San Telmo, Buenos Aires",
		Latitude:             &lat,
		Longitude:            &lng,
		DeliveryInstructions: "Tocar timbre, departamento al frente.",
		ContactlessDelivery:  true,
		LeaveAtDoor:          index%2 == 0,
		ContactName:          customer.Name,
		ContactPhone:         customer.Phone,
		IsDefault:            true,
		IsActive:             true,
		LastUsedAt:           &now,
		UsageCount:           1 + index%6,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := tx.WithContext(ctx).
		Where("customer_id = ? AND business_id = ? AND label = ?", customer.ID, businessID, address.Label).
		FirstOrCreate(&address, address).Error; err != nil {
		return nil, nil, err
	}
	return &customer, &cb, nil
}

func (s *Service) createBillItems(ctx context.Context, tx *gorm.DB, bill *database.Bill, lines []billLine, createdAt time.Time) error {
	for idx, line := range lines {
		item := database.BillItem{
			ID:         deterministicUUID("bill-item", bill.ID, idx),
			BillID:     bill.ID,
			MenuItemID: line.MenuItemID,
			Name:       line.Name,
			Price:      line.Price,
			Quantity:   line.Quantity,
			ItemType:   "menu_item",
			Subtotal:   line.Subtotal,
			CreatedAt:  createdAt.Add(time.Duration(idx+1) * time.Minute),
		}
		if err := tx.WithContext(ctx).Create(&item).Error; err != nil {
			return err
		}
	}
	return nil
}

// createPayment writes the demo bill's main settlement row.
//
// The rail is a counter-recorded charge, never crypto (#795). Demo venues seed
// SettlementAddr "" and every payment plugin disabled — see
// TestDemoEnablesMercadoPagoCardRail, which forbids both a placeholder
// settlement wallet and an enabled crypto rail — so requireGuestCryptoPlugin
// rejects guest crypto on these tenants with 422 plugin_unavailable. Stamping
// payment_method "crypto" made the demo's own books claim on-chain tenders a
// venue that structurally cannot take one, while the guest picker truthfully
// offered only "pay at counter". Every method passed here must canonicalize to
// the card rail ("mercadopago" does) — enforced by
// TestDemoBillsNeverSettleOnARailTheVenueCannotHonor.
//
// PayerAddr keeps the demo payer-pool wallet: it is the CRM guest identity
// Analytics Top Tippers joins on (index + businessID, so the two venues spread
// across different slots and the honesty gate sees ≥8 distinct payers), not a
// claim that a wallet settled the check.
func (s *Service) createPayment(ctx context.Context, tx *gorm.DB, bill *database.Bill, adminUserID uint, amount int64, index int, method string, closedAt time.Time) (*database.Payment, error) {
	payment := database.Payment{
		BillID:        bill.ID,
		PayerAddr:     demoPayerAddr(uint(index) + bill.BusinessID),
		Amount:        amount,
		TipAmount:     bill.TipAmount,
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: method,
		ConfirmedAt:   &closedAt,
		CreatedAt:     closedAt,
		UpdatedAt:     closedAt,
	}
	// payments.tx_hash carries a plain UNIQUE index, so every row needs a
	// distinct non-explorer-linkable value. Mercado Pago confirmations store
	// the plugin reference where crypto stores the tx hash — mirror the real
	// plugin's prefix; counter card charges keep the synthetic demo id.
	if method == "mercadopago" {
		payment.TxHash = "plugin_mercadopago_" + deterministicUUID("mp-pay", bill.ID, index)
	} else {
		payment.TxHash = demoSyntheticTxHash("pay", adminUserID, bill.ID, amount)
	}
	if err := tx.WithContext(ctx).Create(&payment).Error; err != nil {
		return nil, err
	}
	// settlement_chain carries a DB default of 'base' that GORM lets stand for
	// the zero value. A counter card charge never touched a chain, so clear the
	// leftover crypto artifact instead of shipping a card row settled on Base.
	if err := tx.WithContext(ctx).Model(&payment).Update("settlement_chain", "").Error; err != nil {
		return nil, err
	}
	payment.SettlementChain = ""
	return &payment, nil
}

func (s *Service) createConfirmedAlternativePayment(ctx context.Context, tx *gorm.DB, bill *database.Bill, amount int64, method database.AlternativePaymentMethod, confirmedAt time.Time) error {
	alt := database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: fmt.Sprintf("demo-table-%d", bill.ID),
		ParticipantName: "Demo table guest",
		Amount:          amount,
		BillAmountCents: amount,
		PaymentMethod:   method,
		Status:          database.AltPaymentStatusConfirmed,
		IdempotencyKey:  fmt.Sprintf("demo-confirmed-alt-%d", bill.ID),
		ConfirmedBy:     "demo-manager",
		CreatedAt:       confirmedAt.Add(-2 * time.Minute),
		UpdatedAt:       confirmedAt,
		ConfirmedAt:     &confirmedAt,
	}
	return tx.WithContext(ctx).
		Where("bill_id = ? AND idempotency_key = ?", bill.ID, alt.IdempotencyKey).
		Attrs(alt).FirstOrCreate(&alt).Error
}

func (s *Service) createOrder(ctx context.Context, tx *gorm.DB, bill *database.Bill, lines []billLine, staffID uint, createdAt, closedAt time.Time) (*database.Order, error) {
	orderItems := make([]database.OrderItem, 0, len(lines))
	for idx, line := range lines {
		orderItems = append(orderItems, database.OrderItem{ID: fmt.Sprintf("%s-%d", line.MenuItemID, idx), ItemType: "menu_item", MenuItemID: line.MenuItemID, MenuItemName: line.Name, Quantity: line.Quantity, Price: line.Price, Subtotal: line.Subtotal})
	}
	raw, err := json.Marshal(orderItems)
	if err != nil {
		return nil, err
	}
	requestID := fmt.Sprintf("demo-%d", bill.ID)
	order := database.Order{
		BillID:          bill.ID,
		BusinessID:      bill.BusinessID,
		OrderNumber:     fmt.Sprintf("ORD-%s", bill.BillNumber),
		Status:          database.OrderStatusOrderDelivered,
		CreatedBy:       "guest",
		ClientRequestID: &requestID,
		ApprovedBy:      fmt.Sprintf("staff:%d", staffID),
		Notes:           "Pedido de salón (demo)",
		Items:           string(raw),
		CreatedAt:       createdAt,
		UpdatedAt:       closedAt,
		ApprovedAt:      ptrTime(createdAt.Add(4 * time.Minute)),
		KitchenAckedAt:  ptrTime(createdAt.Add(6 * time.Minute)),
	}
	if err := tx.WithContext(ctx).Create(&order).Error; err != nil {
		return nil, err
	}
	return &order, nil
}

func (s *Service) createCRMVisit(ctx context.Context, tx *gorm.DB, customerBusinessID uint, bill *database.Bill, tableID uint, total totals, lines []billLine, closedAt time.Time) error {
	raw, err := json.Marshal(lines)
	if err != nil {
		return err
	}
	rating := 5
	// PointsEarned mirrors the 0.01 pts/peso earn rate: cents/100 = pesos, ×0.01.
	visit := database.CustomerVisit{CustomerBusinessID: customerBusinessID, BillID: &bill.ID, TableID: &tableID, AmountSpent: float64(total.TotalCents) / 100, PointsEarned: int(total.TotalCents / 10000), ItemsPurchased: string(raw), VisitDate: closedAt, VisitDuration: 75, Rating: &rating, Feedback: "Excelente atención, volvemos seguro.", CreatedAt: closedAt}
	if err := tx.WithContext(ctx).Create(&visit).Error; err != nil {
		return err
	}
	comm := database.CustomerCommunication{BusinessID: bill.BusinessID, CustomerBusinessID: customerBusinessID, Type: database.CommunicationTypeEmail, Status: database.CommunicationStatusSent, Subject: "Gracias por tu visita", Content: "Tu recibo y tus puntos de fidelidad ya están disponibles.", SentAt: &closedAt, DeliveredAt: &closedAt, CampaignID: "demo-post-visit"}
	return tx.WithContext(ctx).Create(&comm).Error
}

func (s *Service) createInventoryMovements(ctx context.Context, tx *gorm.DB, businessID uint, bill *database.Bill, orderID uint, lines []billLine, createdAt time.Time) error {
	var recipes []database.InventoryRecipe
	if err := tx.WithContext(ctx).Where("business_id = ?", businessID).Find(&recipes).Error; err != nil {
		return err
	}
	byMenu := make(map[string]database.InventoryRecipe, len(recipes))
	for _, recipe := range recipes {
		byMenu[recipe.MenuItemID] = recipe
	}
	for _, line := range lines {
		recipe, ok := byMenu[line.MenuItemID]
		if !ok {
			continue
		}
		var item database.InventoryItem
		if err := tx.WithContext(ctx).First(&item, recipe.InventoryItemID).Error; err != nil {
			return err
		}
		// Restock low items (except the media res de bife, SKU *-BEEF — its
		// out-of-stock state IS the demo's briefing narrative) so months of
		// daily simulation don't drift the showroom into absurd negative
		// quantities.
		if item.CurrentQuantity <= item.ReorderThreshold && !strings.HasSuffix(item.SKU, "-BEEF") {
			target := item.ReorderThreshold * 5
			restock := database.InventoryMovement{BusinessID: businessID, InventoryItemID: item.ID, MovementType: database.InventoryMovementTypeRestock, QuantityDelta: target - item.CurrentQuantity, QuantityBefore: item.CurrentQuantity, QuantityAfter: target, Reason: "Reposición programada (demo)", Actor: "demo-seed", CreatedAt: createdAt, UpdatedAt: createdAt}
			if err := tx.WithContext(ctx).Create(&restock).Error; err != nil {
				return err
			}
			item.CurrentQuantity = target
			if err := tx.WithContext(ctx).Model(&item).Update("current_quantity", target).Error; err != nil {
				return err
			}
		}
		delta := -recipe.QuantityRequired * float64(line.Quantity)
		if item.CurrentQuantity+delta < 0 {
			// Never consume below zero — an 86'd item stays at 0.00, not -54 kg.
			delta = -item.CurrentQuantity
		}
		if delta == 0 {
			continue
		}
		movement := database.InventoryMovement{BusinessID: businessID, InventoryItemID: item.ID, MovementType: database.InventoryMovementTypeOrderConsumption, QuantityDelta: delta, QuantityBefore: item.CurrentQuantity, QuantityAfter: item.CurrentQuantity + delta, Reason: "Consumo por pedido (demo)", Actor: "demo-seed", MenuItemID: line.MenuItemID, MenuItemName: line.Name, ReferenceOrderID: &orderID, ReferenceBillID: &bill.ID, CreatedAt: createdAt, UpdatedAt: createdAt}
		if err := tx.WithContext(ctx).Create(&movement).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Model(&item).Update("current_quantity", movement.QuantityAfter).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) createDelivery(ctx context.Context, tx *gorm.DB, businessID uint, bill *database.Bill, order *database.Order, customer *database.Customer, at time.Time) error {
	var zone database.DeliveryZone
	if err := tx.WithContext(ctx).Where("business_id = ?", businessID).First(&zone).Error; err != nil {
		return err
	}
	var driver database.DeliveryDriver
	if err := tx.WithContext(ctx).Where("business_id = ? AND is_active = ?", businessID, true).First(&driver).Error; err != nil {
		return err
	}
	rating := 5
	deliveryNumber, err := uniqueDeliveryNumberTx(ctx, tx)
	if err != nil {
		return err
	}
	delivery := database.DeliveryOrder{
		BusinessID: businessID, BillID: bill.ID, OrderID: &order.ID, CustomerID: &customer.ID, ZoneID: &zone.ID,
		DeliveryNumber: deliveryNumber, DeliveryType: database.DeliveryTypeInHouse, Status: database.DeliveryStatusDelivered, Priority: database.PriorityNormal,
		FulfillmentMode: "in_house", DriverID: &driver.ID, AssignedAt: ptrTime(at.Add(5 * time.Minute)),
		CustomerName: customer.Name, CustomerPhone: customer.Phone, CustomerEmail: customer.Email, CustomerLocale: "es",
		DeliveryAddress:     database.DeliveryAddress{Street: demoFakeStreet("Defensa", 148), City: "Buenos Aires", State: "CABA", PostalCode: "C1065", Country: "AR", FormattedAddress: demoFakeStreet("Defensa", 148) + ", San Telmo, Buenos Aires"},
		EstimatedPickupTime: ptrTime(at.Add(20 * time.Minute)), ActualPickupTime: ptrTime(at.Add(22 * time.Minute)), EstimatedDeliveryTime: ptrTime(at.Add(45 * time.Minute)), ActualDeliveryTime: ptrTime(at.Add(43 * time.Minute)),
		DeliveryFee: 290000, DriverTip: 200000, PlatformFee: 0, DeliveryInstructions: "Dejar con el encargado del edificio.", ContactlessDelivery: true, LeaveAtDoor: false, DeliveryCode: "1234", QuoteMetadata: database.JSONRawMessage(`{"demo":true}`), CustomerRating: &rating, DriverRating: &rating, CustomerFeedback: "Llegó rápido y todavía caliente.", CreatedAt: at, UpdatedAt: at.Add(43 * time.Minute),
	}
	if err := tx.WithContext(ctx).Create(&delivery).Error; err != nil {
		return err
	}
	history := []database.DeliveryStatusHistory{
		{DeliveryOrderID: delivery.ID, Status: database.DeliveryStatusConfirmed, Notes: "Pedido confirmado", ChangedBy: "demo", CreatedAt: at},
		{DeliveryOrderID: delivery.ID, Status: database.DeliveryStatusDelivered, Notes: "Pedido entregado", ChangedBy: "demo", CreatedAt: at.Add(43 * time.Minute)},
	}
	return tx.WithContext(ctx).Create(&history).Error
}

func (s *Service) createFiscalAndPrint(ctx context.Context, tx *gorm.DB, businessID uint, bill *database.Bill, payment *database.Payment, at time.Time) error {
	var settings database.BusinessFiscalSettings
	if err := tx.WithContext(ctx).Where("business_id = ?", businessID).First(&settings).Error; err != nil {
		return err
	}
	receiptNumber := fmt.Sprintf("DEMO-%d", bill.ID)
	auth := fmt.Sprintf("AUTH-%d", bill.ID)
	qr := fmt.Sprintf("https://payverge.local/fiscal/%d", bill.ID)
	// Cash/card bills carry no Payment row (the alt-payment rail settled them),
	// so the receipt and job reference a nil PaymentID — the fiscal chain still
	// issues, exactly like a real cash factura.
	var paymentID *uint
	if payment != nil {
		paymentID = &payment.ID
	}
	receipt := database.FiscalReceipt{BusinessID: businessID, SettingsID: settings.ID, BillID: bill.ID, PaymentID: paymentID, Country: settings.Country, Provider: settings.Provider, Action: fiscal.ActionIssueReceipt, ReceiptType: demoReceiptType(settings.Country, bill.ID), ReceiptNumber: &receiptNumber, ProviderReceiptID: &receiptNumber, AuthCode: &auth, QRPayload: &qr, TotalAmountCents: bill.TotalAmount, TipAmountCents: bill.TipAmount, Currency: "ARS", Status: database.FiscalStatusAuthorized, IssuedAt: &at, DeliveredAt: &at}
	if err := tx.WithContext(ctx).Create(&receipt).Error; err != nil {
		return err
	}
	job := database.FiscalJob{BusinessID: businessID, SettingsID: settings.ID, ReceiptID: &receipt.ID, BillID: bill.ID, PaymentID: paymentID, Action: fiscal.ActionIssueReceipt, IdempotencyKey: fmt.Sprintf("demo-fiscal-%d", bill.ID), Status: database.FiscalStatusAuthorized, Attempts: 1, MaxAttempts: 5, CreatedBy: "demo"}
	if err := tx.WithContext(ctx).Create(&job).Error; err != nil {
		return err
	}
	audit := database.FiscalAuditEvent{BusinessID: businessID, ReceiptID: &receipt.ID, JobID: &job.ID, Actor: "demo", EventType: "authorized", Message: "Comprobante fiscal autorizado (demo).", Metadata: map[string]interface{}{"bill_id": bill.ID}, CreatedAt: at}
	if err := tx.WithContext(ctx).Create(&audit).Error; err != nil {
		return err
	}
	var printer database.Printer
	if err := tx.WithContext(ctx).Where("business_id = ? AND role = ?", businessID, "bill").First(&printer).Error; err == nil {
		html := fmt.Sprintf("<html><body>Recibo demo %s</body></html>", bill.BillNumber)
		printed := at.Add(time.Minute)
		printJob := database.PrintJob{BusinessID: businessID, PrinterID: &printer.ID, Kind: database.PrintJobKindReceipt, SourceType: "bill", SourceID: bill.ID, Status: database.PrintJobStatusPrinted, PayloadHTML: &html, AttemptCount: 1, MaxAttempts: 6, Language: "es", CreatedBy: "demo", CreatedAt: at, UpdatedAt: printed, PrintedAt: &printed}
		if err := tx.WithContext(ctx).Create(&printJob).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) createBillHistoryAndAlert(ctx context.Context, tx *gorm.DB, businessID uint, bill *database.Bill, payment *database.Payment, staffID uint, at time.Time) error {
	event := database.BillHistoryEvent{BillID: bill.ID, BusinessID: businessID, EventType: database.BillHistoryEventBillCreated, Actor: "demo", Reason: "Ciclo de vida de cuenta (demo)", Details: map[string]interface{}{"bill_number": bill.BillNumber}, CreatedAt: at}
	if err := tx.WithContext(ctx).Create(&event).Error; err != nil {
		return err
	}
	// Cash/card bills settle through the alt-payment rail and carry no Payment
	// row — no payment-received alert to raise. The default Mercado Pago branch
	// fires ≥1×/day, so the verifier's one-alert-per-day floor still holds.
	if payment == nil {
		return nil
	}
	alert := database.OperationalAlert{BusinessID: businessID, AlertType: database.OperationalAlertTypePaymentReceived, ResourceType: database.OperationalAlertResourceTypePayment, ResourceID: int64(payment.ID), Status: database.OperationalAlertStatusResolved, Priority: database.OperationalAlertPriorityNormal, Title: "Pago recibido", Body: fmt.Sprintf("Se cobró la cuenta %s.", bill.BillNumber), ClaimedByStaffID: &staffID, ClaimedByName: "Encargada de turno", ClaimedAt: ptrTime(at.Add(2 * time.Minute)), ResolvedAt: ptrTime(at.Add(3 * time.Minute)), LastEventAt: at.Add(3 * time.Minute), Metadata: database.JSONRawMessage(`{"demo":true}`), CreatedAt: at, UpdatedAt: at.Add(3 * time.Minute)}
	if err := tx.WithContext(ctx).Create(&alert).Error; err != nil {
		return err
	}
	alertEvent := database.OperationalAlertEvent{AlertID: alert.ID, BusinessID: businessID, EventType: database.OperationalAlertEventTypeResolved, ActorStaffID: &staffID, ActorName: "Encargada de turno", Metadata: database.JSONRawMessage(`{"demo":true}`), CreatedAt: at.Add(3 * time.Minute)}
	return tx.WithContext(ctx).Create(&alertEvent).Error
}

func (s *Service) createTimeEntry(ctx context.Context, tx *gorm.DB, businessID, staffID uint, at time.Time) error {
	clockOut := at.Add(6 * time.Hour)
	entry := database.TimeEntry{BusinessID: businessID, StaffID: staffID, ClockInAt: at.Add(-2 * time.Hour), ClockOutAt: &clockOut, BreakMinutes: 30, Source: database.TimeEntrySourceManagerManual, Status: database.TimeEntryStatusApproved, ApprovedByStaffID: &staffID, Note: "Fichaje aprobado (demo)", CreatedAt: at, UpdatedAt: clockOut}
	return tx.WithContext(ctx).Create(&entry).Error
}

// demoReservationCodePrefix matches every confirmation code the day generator
// owns for a business. The language heal in ensureReservations reads it, so the
// two must never drift — a guest's own booking on a demo venue keeps whatever
// locale they chose.
func demoReservationCodePrefix(businessID uint) string {
	return fmt.Sprintf("RSV-%d-", businessID)
}

func demoReservationCode(businessID uint, day time.Time) string {
	return demoReservationCodePrefix(businessID) + day.Format("20060102")
}

func (s *Service) generateReservationsForDay(ctx context.Context, tx *gorm.DB, businessID uint, day time.Time, language string) error {
	tableID, err := nthTableID(ctx, tx, businessID, int(day.Day()))
	if err != nil {
		return err
	}
	// Dinner reservation at 21:00 — a 19:00 booking reads as a tourist trap in
	// Buenos Aires. Name/party rotate deterministically by calendar day.
	resTime := day.Add(21 * time.Hour)
	code := demoReservationCode(businessID, day)
	var existing database.TableReservation
	err = tx.WithContext(ctx).Where("business_id = ? AND confirmation_code = ?", businessID, code).First(&existing).Error
	if err == nil {
		return nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}
	resName := demoCustomerNames[int(day.Day())%len(demoCustomerNames)]
	res := database.TableReservation{BusinessID: businessID, TableID: &tableID, CustomerName: resName, CustomerPhone: demoFakeMobile(999), CustomerEmail: s.demoEmail(fmt.Sprintf("demo+reservation-%d-%s", businessID, day.Format("20060102"))), PartySize: 2 + int(day.Day())%5, ReservationTime: resTime, Language: language, Duration: 120, Status: "confirmed", Source: "customer", ConfirmationCode: code, SpecialRequests: "Mesa cerca de la ventana, si se puede", Notes: "Reserva confirmada (demo)", ReminderSent: true, CreatedBy: "customer", ConfirmedAt: ptrTime(resTime.Add(-24 * time.Hour)), AssignedAt: ptrTime(resTime.Add(-2 * time.Hour)), CreatedAt: resTime.Add(-48 * time.Hour), UpdatedAt: resTime.Add(-2 * time.Hour)}
	if err := tx.WithContext(ctx).Create(&res).Error; err != nil {
		return err
	}
	history := database.ReservationStatusHistory{ReservationID: res.ID, Status: "confirmed", TableID: &tableID, Notes: "Reserva confirmada", ChangedBy: "demo", CreatedAt: res.ConfirmedAt.UTC()}
	return tx.WithContext(ctx).Create(&history).Error
}

// minTime returns the earlier of two timestamps.
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func (s *Service) generateCashSessionForDay(ctx context.Context, tx *gorm.DB, businessID uint, day time.Time) error {
	// Jitter open/close so every session is not a carbon-copy 09:00–23:00 row
	// (the stride that made production cash history look generated).
	// Open window is 07:00–07:44 so early-morning showroom clocks still have
	// opened_at ≤ now. Always seed CLOSED sessions (never leave an OPEN row):
	// the one-open-per-business unique index would trip when the hourly append
	// advances the calendar, and the Demo Center cash history is a closed log.
	dayIndex := day.Unix() / 86400
	openJitter := time.Duration(demoHash(dayIndex, 11)%45) * time.Minute // 0–44 min
	closeJitter := time.Duration(demoHash(dayIndex, 12)%40) * time.Minute
	openedAt := day.Add(7*time.Hour + openJitter)
	closedAt := day.Add(22*time.Hour + closeJitter)
	if !closedAt.After(openedAt) {
		closedAt = openedAt.Add(8 * time.Hour)
	}

	now := s.now()
	// A shift must never close in the future. Clamp close to now and keep
	// opened_at STRICTLY before closed_at (never equal — that was the lie).
	if closedAt.After(now) {
		closedAt = now
	}
	if !closedAt.After(openedAt) {
		// now is at/before nominal open: pull open back so the session still
		// has a positive duration and coverage gets one row per baseline day.
		openedAt = closedAt.Add(-2 * time.Hour)
		if openedAt.Before(day) {
			openedAt = day
		}
		if !closedAt.After(openedAt) {
			// Degenerate: day == closedAt (clock at midnight). Nudge open.
			openedAt = closedAt.Add(-time.Minute)
		}
	}

	dayEnd := day.Add(24 * time.Hour)

	// #857: Caja's house rail auto-opens a live drawer the moment an operator
	// lands on the tab, so a second session can appear in the middle of a day
	// this generator already owns. The generated shift must HAND OVER at that
	// moment instead of running past it — live venue 142 had session 360
	// (07:19–16:00, AR$ 139.300) overlapping the still-open 362 by three hours,
	// so "¿cuánto hay en caja?" read zero on the drawer the floor was using
	// while a closed twin held the night's cash.
	handoverAt, err := foreignDrawerHandoverAt(ctx, tx, businessID, day, dayEnd)
	if err != nil {
		return err
	}
	if !handoverAt.IsZero() && closedAt.After(handoverAt) {
		closedAt = handoverAt
	}
	// The other drawer was already holding the till before this shift would
	// have started — either it opened earlier this morning, or it was carried
	// across midnight and never closed. Either way it owns the whole day and
	// no generated shift is minted beside it.
	ownsWholeDay := !handoverAt.IsZero() && !closedAt.After(openedAt)

	// One generated session per calendar day. The hourly append revisits today
	// after more bills (and their cash payments) have materialized, so an
	// existing session is UPDATED — never left frozen at its first write,
	// never duplicated (#796). The lookup is scoped to the generator's own
	// actor label so an operator's drawer is never adopted and rewritten.
	var existing database.CashRegisterSession
	err = tx.WithContext(ctx).
		Where("business_id = ? AND opened_at >= ? AND opened_at < ?", businessID, day, dayEnd).
		Where("status = ? AND opened_by_label = ?", database.CashRegisterSessionStatusClosed, demoCashActorLabel).
		Order("opened_at ASC").First(&existing).Error
	if err == nil {
		if closedAt.After(existing.OpenedAt) && (existing.ClosedAt == nil || !existing.ClosedAt.Equal(closedAt)) {
			if err := tx.WithContext(ctx).Model(&database.CashRegisterSession{}).
				Where("id = ?", existing.ID).
				Updates(map[string]interface{}{"closed_at": closedAt, "updated_at": closedAt}).Error; err != nil {
				return err
			}
		}
		// Cash figures are re-derived from the books per drawer below.
		return s.reattributeDayCash(ctx, tx, businessID, day)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if ownsWholeDay {
		return s.reattributeDayCash(ctx, tx, businessID, day)
	}

	cashSales, err := deriveCashSalesCents(ctx, tx, businessID, day)
	if err != nil {
		return err
	}
	// Vary the remaining shift figures deterministically per day so the
	// Caja/cash-register history reads like real shifts instead of identical
	// carbon-copy rows. Keyed off the absolute day index (day.Unix()/86400) —
	// never math/rand or time.Now — so reseeds stay idempotent and tests are
	// reproducible.
	amt := computeCashSessionAmounts(day, cashSales)
	session := database.CashRegisterSession{
		BusinessID: businessID, Status: database.CashRegisterSessionStatusClosed,
		OpeningFloatCents: amt.openingFloat, OpeningNote: "Apertura de caja", OpenedByLabel: demoCashActorLabel, OpenedAt: openedAt,
		CashSalesCents: amt.cashSales, CashRefundsCents: 0, CashInCents: amt.cashIn, CashOutCents: amt.cashOut,
		ExpectedCashCents: amt.expected, CountedCashCents: amt.counted, VarianceCents: amt.variance,
		ClosingNote: "Cierre de caja", ClosedByLabel: demoCashActorLabel, ClosedAt: &closedAt,
		CreatedAt: openedAt, UpdatedAt: closedAt,
	}
	if err := tx.WithContext(ctx).Create(&session).Error; err != nil {
		return err
	}
	// Keep movements inside the (possibly clamped) session window. Amounts
	// MUST match the session's cash-in / cash-out so totals reconcile.
	cashInAt := minTime(openedAt.Add(time.Hour), closedAt)
	cashOutAt := minTime(openedAt.Add(4*time.Hour), closedAt)
	if !cashInAt.After(openedAt) {
		cashInAt = openedAt.Add(time.Duration(math.Max(1, closedAt.Sub(openedAt).Minutes()/3)) * time.Minute)
		if cashInAt.After(closedAt) {
			cashInAt = closedAt
		}
	}
	movements := []database.CashRegisterMovement{
		{BusinessID: businessID, SessionID: session.ID, MovementType: database.CashRegisterMovementTypeCashIn, AmountCents: amt.cashIn, Reason: "cambio del banco", Note: "Ingreso de efectivo (demo)", ActorLabel: demoCashActorLabel, OccurredAt: cashInAt, CreatedAt: cashInAt},
		{BusinessID: businessID, SessionID: session.ID, MovementType: database.CashRegisterMovementTypeCashOut, AmountCents: amt.cashOut, Reason: "caja chica", Note: "Retiro de efectivo (demo)", ActorLabel: demoCashActorLabel, OccurredAt: cashOutAt, CreatedAt: cashOutAt},
	}
	if err := tx.WithContext(ctx).Create(&movements).Error; err != nil {
		return err
	}
	return s.reattributeDayCash(ctx, tx, businessID, day)
}

// demoCashActorLabel is the actor the generator signs its own Caja shifts
// with. It is the ownership predicate for every repair below: the house rail
// ("demo-dinner-drawer") and real operators sign their own drawers and are
// never adopted, rewritten or closed by the seeder.
const demoCashActorLabel = "demo-manager"

// foreignDrawerHandoverAt returns the moment during [day, dayEnd) at which a
// drawer this generator does NOT own — the house-rail drawer or an operator's
// — takes the till. Zero means the generator has the day to itself.
//
// A drawer that opened earlier and is STILL OPEN counts from midnight (#857):
// cashregister.ensureDemoHouseCashSessionTx only ever opens the house drawer,
// so nothing but a human close ever ends it, and a drawer armed at 12:57 is
// holding the till again tomorrow. Scoping this lookup to opened_at inside the
// day made that drawer invisible on D+1, and the generator answered by minting
// a fresh 07:xx→now CLOSED twin straddling it — the reported live shape.
func foreignDrawerHandoverAt(ctx context.Context, tx *gorm.DB, businessID uint, day, dayEnd time.Time) (time.Time, error) {
	var foreign database.CashRegisterSession
	err := tx.WithContext(ctx).
		Where("business_id = ? AND opened_at < ?", businessID, dayEnd).
		Where("opened_by_label <> ?", demoCashActorLabel).
		Where("(opened_at >= ? OR (status = ? AND closed_at IS NULL))",
			day, database.CashRegisterSessionStatusOpen).
		Order("opened_at ASC").Order("id ASC").
		First(&foreign).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	openedAt := foreign.OpenedAt.UTC()
	if openedAt.Before(day) {
		// Carried across midnight: it already holds the till at 00:00.
		return day, nil
	}
	return openedAt, nil
}

// daySessions returns every drawer that OVERLAPS [day, day+24h) — the ones
// opened during the day plus any carried across midnight that had not closed
// when the day began. The carried-over case is what #857 missed: the drawer the
// floor is actually holding is not "a session of this day" by opened_at, but it
// owns the day all the same.
func daySessions(ctx context.Context, tx *gorm.DB, businessID uint, day, dayEnd time.Time) ([]database.CashRegisterSession, error) {
	var sessions []database.CashRegisterSession
	if err := tx.WithContext(ctx).
		Where("business_id = ? AND opened_at < ?", businessID, dayEnd).
		Where("(opened_at >= ? OR closed_at IS NULL OR closed_at > ?)", day, day).
		Order("opened_at ASC").Order("id ASC").
		Find(&sessions).Error; err != nil {
		return nil, err
	}
	return sessions, nil
}

// cashSessionWindow is one drawer's slice of a demo day.
type cashSessionWindow struct {
	session database.CashRegisterSession
	from    time.Time
	to      time.Time
}

// partitionDayCash splits [day, day+24h) across every drawer opened that day:
// the first absorbs everything before it opened, each later drawer takes over
// at its own opened_at, and the last runs to midnight. A single drawer gets
// the whole day, which is exactly the pre-#857 behaviour.
func partitionDayCash(day time.Time, sessions []database.CashRegisterSession) []cashSessionWindow {
	dayEnd := day.Add(24 * time.Hour)
	windows := make([]cashSessionWindow, 0, len(sessions))
	for i, session := range sessions {
		from := day
		if i > 0 {
			from = session.OpenedAt.UTC()
		}
		to := dayEnd
		if i+1 < len(sessions) {
			to = sessions[i+1].OpenedAt.UTC()
		}
		if to.Before(from) {
			to = from
		}
		windows = append(windows, cashSessionWindow{session: session, from: from, to: to})
	}
	return windows
}

// reattributeDayCash hands every confirmed cash check of `day` to the drawer
// that was open when it was taken, and backs the figure with a real
// cash_sale movement per payment.
//
// Two live symptoms come from the same gap (#857): the demo wrote its cash
// tenders straight into the books without ever attaching them to a session, so
// Caja reported 107 payments / AR$ 11.97M of "efectivo sin asignar"; and the
// whole day's cash was stamped onto whichever drawer the generator happened to
// find, so the drawer the floor was actually using read zero.
//
// It never opens, closes, deletes or restatuses a session. A live drawer keeps
// its status and its count — it just stops lying about how much is in it.
func (s *Service) reattributeDayCash(ctx context.Context, tx *gorm.DB, businessID uint, day time.Time) error {
	dayEnd := day.Add(24 * time.Hour)
	sessions, err := daySessions(ctx, tx, businessID, day, dayEnd)
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		return nil
	}
	windows := partitionDayCash(day, sessions)

	var payments []database.AlternativePayment
	if err := tx.WithContext(ctx).Model(&database.AlternativePayment{}).
		Select("alternative_payments.*").
		Joins("JOIN bills ON bills.id = alternative_payments.bill_id").
		Where("bills.business_id = ? AND alternative_payments.payment_method = ? AND alternative_payments.status = ?",
			businessID, database.PaymentMethodCash, database.AltPaymentStatusConfirmed).
		Where("alternative_payments.confirmed_at >= ? AND alternative_payments.confirmed_at < ?", day, dayEnd).
		Order("alternative_payments.confirmed_at ASC").Order("alternative_payments.id ASC").
		Find(&payments).Error; err != nil {
		return err
	}

	existingMovements := map[uint]database.CashRegisterMovement{}
	if len(payments) > 0 {
		paymentIDs := make([]uint, 0, len(payments))
		for _, payment := range payments {
			paymentIDs = append(paymentIDs, payment.ID)
		}
		var movements []database.CashRegisterMovement
		if err := tx.WithContext(ctx).
			Where("movement_type = ? AND alternative_payment_id IN ?", database.CashRegisterMovementTypeCashSale, paymentIDs).
			Find(&movements).Error; err != nil {
			return err
		}
		for _, movement := range movements {
			if movement.AlternativePaymentID != nil {
				existingMovements[*movement.AlternativePaymentID] = movement
			}
		}
	}

	for _, payment := range payments {
		amount := payment.BillAmountCents + payment.TipAmountCents
		if amount <= 0 {
			amount = payment.Amount
		}
		if amount <= 0 {
			continue
		}
		idx := windowIndexFor(windows, payment.ConfirmedAt)
		if err := s.syncCashSaleMovement(ctx, tx, businessID, windows[idx], payment, amount, existingMovements[payment.ID]); err != nil {
			return err
		}
	}
	for _, window := range windows {
		if err := s.applyCashSales(ctx, tx, window.session); err != nil {
			return err
		}
	}
	return nil
}

// windowIndexFor picks the drawer that was open when a payment was confirmed:
// the last window whose start is at or before it. Anything before the first
// drawer opened belongs to that first drawer.
func windowIndexFor(windows []cashSessionWindow, confirmedAt *time.Time) int {
	if confirmedAt == nil {
		return 0
	}
	at := confirmedAt.UTC()
	idx := 0
	for i, window := range windows {
		if !at.Before(window.from) {
			idx = i
		}
	}
	return idx
}

// syncCashSaleMovement writes (or re-points) the cash_sale movement that backs
// one confirmed cash tender. Real cash goes through
// database.AttachCashRegisterMovementForAlternativePaymentTx; the demo writes
// history directly, so this is the equivalent door for seeded tenders.
func (s *Service) syncCashSaleMovement(ctx context.Context, tx *gorm.DB, businessID uint, window cashSessionWindow, payment database.AlternativePayment, amount int64, existing database.CashRegisterMovement) error {
	occurredAt := window.from
	if payment.ConfirmedAt != nil {
		occurredAt = payment.ConfirmedAt.UTC()
	}
	if existing.ID != 0 {
		// Never move a movement the demo did not write. Real cash the floor
		// rang up was attached to whichever drawer the cashier had open, and
		// its amount is already inside that session's declared total — dragging
		// it onto a generated shift would take money out of a live till.
		if existing.ActorLabel != demoCashActorLabel {
			return nil
		}
		if existing.SessionID == window.session.ID && existing.AmountCents == amount {
			return nil
		}
		return tx.WithContext(ctx).Model(&database.CashRegisterMovement{}).
			Where("id = ?", existing.ID).
			Updates(map[string]interface{}{
				"session_id":   window.session.ID,
				"business_id":  businessID,
				"amount_cents": amount,
				"occurred_at":  occurredAt,
			}).Error
	}
	paymentID := payment.ID
	billID := payment.BillID
	movement := database.CashRegisterMovement{
		BusinessID:           businessID,
		SessionID:            window.session.ID,
		MovementType:         database.CashRegisterMovementTypeCashSale,
		AmountCents:          amount,
		Reason:               "cash payment",
		AlternativePaymentID: &paymentID,
		BillID:               &billID,
		ActorLabel:           demoCashActorLabel,
		OccurredAt:           occurredAt,
		CreatedAt:            occurredAt,
	}
	return tx.WithContext(ctx).Create(&movement).Error
}

// sessionCashSaleTotal sums the cash_sale movements attached to one drawer.
//
// This is the same invariant the live rail maintains:
// database.AttachCashRegisterMovementForAlternativePaymentTx writes a cash_sale
// movement and then increments the open session by exactly that amount, so a
// session's cash_sales_cents IS the sum of its cash_sale movements. Deriving
// the figure from the ledger instead of from a day window is what makes the
// write safe for a drawer that outlives the day (#857): a still-open drawer
// carries every check attached to it since it opened — the demo's own tenders
// AND the real cash the floor rang up on later days — instead of being reset
// to the sum of whichever day is being reconciled.
func sessionCashSaleTotal(ctx context.Context, tx *gorm.DB, sessionID uint) (int64, error) {
	var total int64
	if err := tx.WithContext(ctx).Model(&database.CashRegisterMovement{}).
		Where("session_id = ? AND movement_type = ?", sessionID, database.CashRegisterMovementTypeCashSale).
		Select("COALESCE(SUM(amount_cents), 0)").
		Scan(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

// applyCashSales re-derives a drawer's cash sales from its own movement ledger
// and writes them back. A closed shift also gets expected/counted recomputed,
// preserving its recorded over/short — reconciling the books must never invent
// a different till count. A live drawer keeps expected/counted/variance
// untouched: those are close-time figures the operator has not declared yet.
//
// A session a HUMAN closed is left entirely alone. Its counted cash is a
// declaration somebody signed; the demo may re-derive its own seeded history,
// never an operator's closing count.
func (s *Service) applyCashSales(ctx context.Context, tx *gorm.DB, session database.CashRegisterSession) error {
	if session.ClosedByUserID != nil || session.ClosedByStaffID != nil {
		return nil
	}
	cashSales, err := sessionCashSaleTotal(ctx, tx, session.ID)
	if err != nil {
		return err
	}
	updates := map[string]interface{}{}
	if session.CashSalesCents != cashSales {
		updates["cash_sales_cents"] = cashSales
	}
	if session.Status == database.CashRegisterSessionStatusClosed {
		variance := session.CountedCashCents - session.ExpectedCashCents
		expected := session.OpeningFloatCents + cashSales - session.CashRefundsCents + session.CashInCents - session.CashOutCents
		counted := expected + variance
		if counted < 0 {
			counted = 0
			variance = counted - expected
		}
		if session.ExpectedCashCents != expected {
			updates["expected_cash_cents"] = expected
		}
		if session.CountedCashCents != counted {
			updates["counted_cash_cents"] = counted
		}
		if session.VarianceCents != variance {
			updates["variance_cents"] = variance
		}
	}
	if len(updates) == 0 {
		return nil
	}
	updates["updated_at"] = s.now().UTC()
	return tx.WithContext(ctx).Model(&database.CashRegisterSession{}).
		Where("id = ?", session.ID).Updates(updates).Error
}

// deriveCashSalesCents sums the confirmed CASH alternative payments a business
// took inside [day, day+24h). This is the ONLY source for a demo session's
// cash_sales, so the till always reconciles with the books: the old
// hash-fabricated band (60000–120000 cents) had zero cash payments behind it,
// so caja showed cash-heavy weeks while sales/accounting read $0 (#796).
func deriveCashSalesCents(ctx context.Context, tx *gorm.DB, businessID uint, day time.Time) (int64, error) {
	var cashSales int64
	if err := tx.WithContext(ctx).Table("alternative_payments").
		Joins("JOIN bills ON bills.id = alternative_payments.bill_id").
		Where("bills.business_id = ? AND alternative_payments.payment_method = ? AND alternative_payments.status = ? AND alternative_payments.confirmed_at >= ? AND alternative_payments.confirmed_at < ?",
			businessID, database.PaymentMethodCash, database.AltPaymentStatusConfirmed, day, day.Add(24*time.Hour)).
		Select("COALESCE(SUM(alternative_payments.amount), 0)").
		Scan(&cashSales).Error; err != nil {
		return 0, err
	}
	return cashSales, nil
}

// ReconcileCashSessions re-derives cash_sales for demo sessions that the
// forward-only append pointer will never revisit.
//
// generateCashSessionForDay only ever corrects the day it is generating, and
// the hourly append walks forward from last_simulated_business_date. Every
// session written before the derive-from-the-books fix therefore keeps its
// fabricated cash forever: live demo venue 86 was painting ~$4,907 of cash
// sales across an ISO week whose bills and payments summed to $0 (#796).
//
// Update-only and bounded to [start, end): it never creates a session, never
// touches status, and never closes an open drawer — today's live session is
// outside the window and is left exactly as the floor left it.
// Each day is re-derived per DRAWER (#857), so a day that carries a second
// session splits its cash at the handover instead of stamping the whole day
// onto whichever row was found first. Figures are recomputed from each row's
// OWN float and movements, never from computeCashSessionAmounts: the cash-in /
// cash-out movement rows are already written against those figures. The
// recorded over/short is preserved — reconciling the books must not invent a
// different till count.
func (s *Service) ReconcileCashSessions(ctx context.Context, tx *gorm.DB, businessID uint, start, end time.Time) error {
	for _, day := range enumerateDays(start, end.AddDate(0, 0, -1)) {
		if err := s.reattributeDayCash(ctx, tx, businessID, day); err != nil {
			return err
		}
	}
	return nil
}

// cashSessionAmounts holds the deterministically-varied cash-register figures
// for one demo day. The reconciliation invariant always holds:
//
//	expected = openingFloat + cashSales - cashRefunds + cashIn - cashOut
//	variance = counted - expected
type cashSessionAmounts struct {
	openingFloat int64
	cashSales    int64
	cashIn       int64
	cashOut      int64
	expected     int64
	counted      int64
	variance     int64
}

// computeCashSessionAmounts derives reproducible per-day session figures.
// cashSales comes from the caller — summed from the day's confirmed cash
// alternative payments, never fabricated (#796). The remaining figures key a
// tiny splitmix64-style hash off the absolute day index so the same day always
// yields the same numbers (idempotent reseeds, deterministic tests) while
// consecutive days differ.
func computeCashSessionAmounts(day time.Time, cashSales int64) cashSessionAmounts {
	dayIndex := day.Unix() / 86400
	// ARS cents, rounded to whole pesos (multiples of 100) for realism.
	openingFloat := 6000000 + int64(demoHash(dayIndex, 1)%2001)*1000 // AR$ 60.000–80.001
	cashIn := 1000000 + int64(demoHash(dayIndex, 3)%1501)*1000       // AR$ 10.000–25.010
	cashOut := 500000 + int64(demoHash(dayIndex, 4)%1001)*1000       // AR$ 5.000–15.010
	expected := openingFloat + cashSales + cashIn - cashOut
	counted := expected
	variance := int64(0)
	// ~1 in 4 days record a small over/short so the variance column tells a
	// story; the rest reconcile exactly to zero.
	if demoHash(dayIndex, 5)%4 == 0 {
		delta := 50000 + int64(demoHash(dayIndex, 6)%4501)*100 // AR$ 500–5.001
		if demoHash(dayIndex, 7)%2 == 0 {
			delta = -delta // came up short
		}
		counted = expected + delta
		variance = counted - expected
	}
	return cashSessionAmounts{openingFloat: openingFloat, cashSales: cashSales, cashIn: cashIn, cashOut: cashOut, expected: expected, counted: counted, variance: variance}
}

// demoHash is a small deterministic mixer (splitmix64 finalizer) over a day
// index and a stream selector, so different fields of the same day draw
// independent-looking-but-reproducible values without math/rand.
func demoHash(dayIndex int64, stream uint64) uint64 {
	x := uint64(dayIndex)*0x9E3779B97F4A7C15 + stream*0xBF58476D1CE4E5B9
	x ^= x >> 30
	x *= 0xBF58476D1CE4E5B9
	x ^= x >> 27
	x *= 0x94D049BB133111EB
	x ^= x >> 31
	return x
}

// demoReceiptType picks a country-appropriate receipt type for seeded demo
// invoices. US demo venues must NOT seed AFIP Factura A/B/C (issue #255).
func demoReceiptType(country string, billID uint) string {
	switch strings.ToUpper(strings.TrimSpace(country)) {
	case "AR":
		return demoAFIPReceiptType(billID)
	case "AE":
		return fiscal.ResolveIssuableReceiptTypeForCountry("AE", "", "", "", "")
	default:
		// US / other: alternate invoice (identified buyer) vs receipt (consumer).
		if billID%2 == 0 {
			return fiscal.ResolveIssuableReceiptTypeForCountry("US", "general", "", "EIN", "12-3456789")
		}
		return fiscal.ResolveIssuableReceiptTypeForCountry("US", "general", "", "", "")
	}
}

// demoCustomerNames is the fixed 12-slot CRM guest roster; slot assignment in
// ensureCustomer keys off index%12 so names, emails, and wallets stay aligned
// across reseeds.
var demoCustomerNames = [12]string{
	"Julieta Pereyra", "Matías Herrera", "Agustina Molina", "Franco Ledesma",
	"Rocío Fernández", "Gonzalo Ríos", "Carolina Vega", "Ezequiel Duarte",
	"Micaela Ponce", "Ramiro Salas", "Florencia Cabrera", "Federico Ibarra",
}

// demoValidCUIT builds a FAKE, checksum-valid CUIT (11 digits, no separators)
// from a type prefix (20/27/30) and a deterministic sequence number. The body
// is a demo placeholder, not a looked-up taxpayer. The fiscal
// receiver rejects bad check digits with the AFIP mod-11 rule, so seeded bill
// identities must pass it. When a candidate lands in the never-issued check-10
// residue class, the next sequence number is tried (still deterministic).
func demoValidCUIT(prefix, n int) string {
	weights := []int{5, 4, 3, 2, 7, 6, 5, 4, 3, 2}
	for {
		base := fmt.Sprintf("%02d%08d", prefix%100, (10000000+n)%100000000)
		sum := 0
		for i, w := range weights {
			sum += int(base[i]-'0') * w
		}
		check := 11 - sum%11
		if check == 11 {
			check = 0
		}
		if check != 10 {
			return fmt.Sprintf("%s%d", base, check)
		}
		n++
	}
}

// demoAFIPReceiptType picks a customer tax identity from billID and returns the
// AFIP receipt type the AR policy would issue (factura_a/b/c). The CUIT and
// DNI below are FAKE placeholders: only the shape matters to the policy.
func demoAFIPReceiptType(billID uint) string {
	switch billID % 3 {
	case 0:
		return fiscal.ResolveIssuableReceiptType("responsable_inscripto", "responsable_inscripto", "CUIT", "20111111112")
	case 1:
		return fiscal.ResolveIssuableReceiptType("responsable_inscripto", "consumidor_final", "DNI", "30111222")
	default:
		return fiscal.ResolveIssuableReceiptType("monotributo", "consumidor_final", "", "")
	}
}
