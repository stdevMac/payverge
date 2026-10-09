package demo

import (
	"context"
	"fmt"
	"math"
	"strings"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func (s *Service) coverageChecks(ctx context.Context, businessIDs []uint) []CoverageCheck {
	checks := []struct {
		key     string
		model   interface{}
		min     int64
		message string
	}{
		{"business_profile", &database.Business{}, int64(len(businessIDs)), "demo businesses missing"},
		{"menu_images", &database.Menu{}, int64(len(businessIDs)), "menus with real images missing"},
		{"tables", &database.Table{}, int64(len(businessIDs) * 4), "tables missing"},
		{"bills", &database.Bill{}, int64(len(businessIDs) * s.baselineDays), "30-day bill history missing"},
		{"bill_items", &database.BillItem{}, int64(len(businessIDs) * s.baselineDays), "bill item rows missing"},
		{"payments", &database.Payment{}, int64(len(businessIDs) * s.baselineDays), "confirmed payments missing"},
		{"alternative_payments", &database.AlternativePayment{}, int64(len(businessIDs) * 2), "alternative tender rows missing"},
		{"orders", &database.Order{}, int64(len(businessIDs) * s.baselineDays), "orders/kitchen rows missing"},
		{"crm", &database.CustomerBusiness{}, int64(len(businessIDs)), "CRM customer links missing"},
		{"customer_addresses", &database.CustomerAddress{}, int64(len(businessIDs)), "customer saved delivery addresses missing"},
		{"inventory", &database.InventoryMovement{}, int64(len(businessIDs) * s.baselineDays), "inventory movements missing"},
		{"reservations", &database.TableReservation{}, int64(len(businessIDs) * s.baselineDays), "reservation rows missing"},
		{"delivery", &database.DeliveryOrder{}, int64(len(businessIDs)), "delivery orders missing"},
		{"staff", &database.Staff{}, int64(len(businessIDs) * 4), "staff rows missing"},
		{"staff_invitations", &database.StaffInvitation{}, int64(len(businessIDs)), "pending staff invitations missing"},
		{"staff_notifications", &database.StaffNotification{}, int64(len(businessIDs)), "staff notification inbox rows missing"},
		{"scheduling", &database.Shift{}, int64(len(businessIDs)), "schedule shifts missing"},
		{"time_off", &database.TimeOffRequest{}, int64(len(businessIDs)), "time-off workflow rows missing"},
		{"coverage_claims", &database.OpenShiftClaim{}, int64(len(businessIDs)), "open-shift claim rows missing"},
		{"coverage_swaps", &database.ShiftSwapRequest{}, int64(len(businessIDs)), "shift swap/give-up rows missing"},
		{"shift_logbook", &database.ShiftNote{}, int64(len(businessIDs)), "shift logbook rows missing"},
		{"chat", &database.ChatMessage{}, int64(len(businessIDs)), "chat messages missing"},
		{"engagement", &database.ChecklistTemplate{}, int64(len(businessIDs)), "engagement checklist missing"},
		{"fiscal", &database.FiscalReceipt{}, int64(len(businessIDs) * s.baselineDays), "fiscal receipts missing"},
		{"printers", &database.Printer{}, int64(len(businessIDs)), "printers missing"},
		{"cash_register", &database.CashRegisterSession{}, int64(len(businessIDs) * s.baselineDays), "cash register sessions missing"},
		{"manual_ledger", &database.ManualLedgerEntry{}, int64(len(businessIDs) * 2), "manual ledger rows missing"},
		{"payroll", &database.PayrollRun{}, int64(len(businessIDs)), "payroll run rows missing"},
		{"loyalty", &database.LoyaltyProgram{}, int64(len(businessIDs)), "loyalty program rows missing"},
		{"plugin_configs", &database.BusinessPlugin{}, int64(len(businessIDs) * 2), "business plugin config rows missing"},
		{"operational_alerts", &database.OperationalAlert{}, int64(len(businessIDs) * s.baselineDays), "operational alerts missing"},
		{"ai", &database.AIGeneratedImage{}, int64(len(businessIDs)), "AI demo records missing"},
	}
	out := make([]CoverageCheck, 0, len(checks))
	for _, check := range checks {
		count, err := s.countCoverage(ctx, businessIDs, check.model)
		status := CoveragePassed
		message := ""
		if err != nil {
			status = CoverageFailed
			message = err.Error()
		} else if count < check.min {
			status = CoverageFailed
			message = fmt.Sprintf("%s: got %d want >= %d", check.message, count, check.min)
		}
		out = append(out, CoverageCheck{Key: check.key, Status: status, Count: count, Message: message})
	}
	if !s.menusHaveRealImages(ctx, businessIDs) {
		for i := range out {
			if out[i].Key == "menu_images" {
				out[i].Status = CoverageFailed
				out[i].Message = "menu images must be real https URLs and not dummy placeholders"
			}
		}
	}
	count, err := s.countDeliveryPaymentFixtures(ctx, businessIDs)
	deliveryPayment := CoverageCheck{Key: "delivery_payment", Status: CoveragePassed, Count: count}
	if err != nil {
		deliveryPayment.Status = CoverageFailed
		deliveryPayment.Message = err.Error()
	} else if count < int64(len(businessIDs)) {
		deliveryPayment.Status = CoverageFailed
		deliveryPayment.Message = fmt.Sprintf("awaiting-payment delivery rows missing: got %d want >= %d", count, len(businessIDs))
	}
	out = append(out, deliveryPayment)
	return out
}

func (s *Service) countCoverage(ctx context.Context, businessIDs []uint, model interface{}) (int64, error) {
	if len(businessIDs) == 0 {
		return 0, nil
	}
	var count int64
	var err error
	switch model.(type) {
	case *database.Business:
		err = s.db.WithContext(ctx).Model(&database.Business{}).Where("id IN ? AND is_demo = ?", businessIDs, true).Count(&count).Error
	case *database.BillItem:
		err = s.db.WithContext(ctx).Model(&database.BillItem{}).
			Joins("JOIN bills ON bills.id = bill_items.bill_id").
			Where("bills.business_id IN ?", businessIDs).
			Count(&count).Error
	case *database.Payment:
		err = s.db.WithContext(ctx).Model(&database.Payment{}).
			Joins("JOIN bills ON bills.id = payments.bill_id").
			Where("bills.business_id IN ?", businessIDs).
			Count(&count).Error
	case *database.AlternativePayment:
		err = s.db.WithContext(ctx).Model(&database.AlternativePayment{}).
			Joins("JOIN bills ON bills.id = alternative_payments.bill_id").
			Where("bills.business_id IN ?", businessIDs).
			Count(&count).Error
	default:
		err = s.db.WithContext(ctx).Model(model).Where("business_id IN ?", businessIDs).Count(&count).Error
	}
	return count, err
}

func (s *Service) menusHaveRealImages(ctx context.Context, businessIDs []uint) bool {
	var menus []database.Menu
	if err := s.db.WithContext(ctx).Where("business_id IN ?", businessIDs).Find(&menus).Error; err != nil || len(menus) == 0 {
		return false
	}
	for _, menu := range menus {
		if !strings.Contains(menu.Categories, "/"+seedAssetPrefix+"/") || strings.Contains(menu.Categories, "dummyimage") {
			return false
		}
	}
	return true
}

func (s *Service) countDeliveryPaymentFixtures(ctx context.Context, businessIDs []uint) (int64, error) {
	if len(businessIDs) == 0 {
		return 0, nil
	}
	var count int64
	err := s.db.WithContext(ctx).Model(&database.DeliveryOrder{}).
		Joins("JOIN bills ON bills.id = delivery_orders.bill_id").
		Where("delivery_orders.business_id IN ?", businessIDs).
		Where("delivery_orders.status = ?", database.DeliveryStatusConfirmed).
		Where("delivery_orders.payment_expires_at > ?", s.now().UTC()).
		Where("bills.status <> ?", database.BillStatusPaid).
		Count(&count).Error
	return count, err
}

func (s *Service) verifyMoney(ctx context.Context, businessIDs []uint, result *VerificationResult) error {
	if len(businessIDs) == 0 {
		return nil
	}
	var bills []database.Bill
	if err := s.db.WithContext(ctx).Where("business_id IN ?", businessIDs).Find(&bills).Error; err != nil {
		return err
	}
	for _, bill := range bills {
		var items []database.BillItem
		if err := s.db.WithContext(ctx).Where("bill_id = ?", bill.ID).Find(&items).Error; err != nil {
			return err
		}
		var subtotal int64
		for _, item := range items {
			subtotal += int64(math.Round(item.Subtotal * 100))
		}
		if subtotal != bill.Subtotal {
			result.Errors = append(result.Errors, fmt.Sprintf("bill %s subtotal mismatch: items=%d bill=%d", bill.BillNumber, subtotal, bill.Subtotal))
		}
		var paid int64
		if err := s.db.WithContext(ctx).Model(&database.Payment{}).
			Where("bill_id = ? AND status = ?", bill.ID, database.PaymentStatusConfirmed).
			Select("COALESCE(SUM(amount), 0)").Scan(&paid).Error; err != nil {
			return err
		}
		var altPaid int64
		if err := s.db.WithContext(ctx).Model(&database.AlternativePayment{}).
			Where("bill_id = ? AND status = ?", bill.ID, database.AltPaymentStatusConfirmed).
			Select("COALESCE(SUM(amount), 0)").Scan(&altPaid).Error; err != nil {
			return err
		}
		recognizedPaid := paid + altPaid
		if recognizedPaid != bill.PaidAmount {
			result.Errors = append(result.Errors, fmt.Sprintf("bill %s paid amount mismatch: recognized=%d bill_paid=%d", bill.BillNumber, recognizedPaid, bill.PaidAmount))
		}
		if bill.Status == database.BillStatusPaid && recognizedPaid != bill.TotalAmount {
			result.Errors = append(result.Errors, fmt.Sprintf("bill %s paid mismatch: recognized=%d total=%d", bill.BillNumber, recognizedPaid, bill.TotalAmount))
		}
	}
	return nil
}

// demoWipeChildSpec is one grandchild cleanup in deleteOwnedDemoData: rows that
// hang off another demo row rather than off the business itself, through a
// foreign key the genesis schema declares NO ACTION. Postgres aborts the whole
// transaction if even one of these outlives its parent, and the forced reseed
// rolls back with it — which is how a demo instance stays frozen on an old
// seed version through every deploy, still advertising last season's menu and
// last week's open tickets (#921, #904, #905).
type demoWipeChildSpec struct {
	// parent names the id set that feeds this cleanup.
	parent string
	column string
	model  interface{}
}

// demoWipeChildSpecs is delete-ordered: children strictly before their parents.
// TestDemoWipeClearsEveryTableThatBlocksABusinessDelete reads the genesis
// schema and fails if a new blocking foreign key lands without an entry here.
var demoWipeChildSpecs = []demoWipeChildSpec{
	{"delivery_orders", "delivery_order_id", &database.DeliveryStatusHistory{}},
	{"table_reservations", "reservation_id", &database.ReservationStatusHistory{}},
	{"customer_businesses", "customer_business_id", &database.CustomerVisit{}},
	{"customers", "customer_id", &database.CustomerPreferences{}},
	{"loyalty_programs", "loyalty_program_id", &database.LoyaltyTier{}},
	{"ai_waiter_conversations", "conversation_id", &database.AiWaiterMessage{}},
	{"menu_wizard_sessions", "session_id", &database.MenuWizardMessage{}},
	// Extraction images FK-reference their job (fk_menu_extraction_jobs_images);
	// deleting jobs while page images survive aborts the whole wipe on Postgres,
	// which is exactly how legacy demo instances stayed bricked on old seed
	// versions — every forced reseed rolled back at this point.
	{"menu_extraction_jobs", "job_id", &database.MenuExtractionImage{}},
	// Refund evidence hangs off the payment, not the bill.
	{"payments", "payment_id", &database.PaymentRefundDestination{}},
	{"bills", "bill_id", &database.BillItem{}},
	{"bills", "bill_id", &database.Payment{}},
	{"bills", "bill_id", &database.AlternativePayment{}},
	{"bills", "bill_id", &database.BillSplitShare{}},
	// Every outbound plugin notification keeps its per-attempt log.
	{"plugin_notification_deliveries", "delivery_id", &database.PluginNotificationDeliveryAttempt{}},
	// Staff login codes accrue from real logins on the demo dashboards.
	{"staff", "staff_id", &database.StaffLoginCode{}},
}

// demoWipeBusinessModels lists, in delete order, everything the wipe clears by
// business_id. Children accrued OUTSIDE the seeder belong here too — startup
// backfills (business_revenue_aggregates exists for every business with bills)
// and live operator/guest usage (offers, AI-waiter chats, director tool calls,
// languages). Production Postgres enforces their FKs to businesses/bills, so
// missing any of these aborts the wipe and the forced reseed never lands.
func demoWipeBusinessModels() []interface{} {
	return []interface{}{
		&database.BusinessRevenueAggregate{},
		&database.BusinessMilestoneEvent{},
		&database.Offer{},
		&database.Bundle{},
		&database.BusinessCurrency{},
		&database.BusinessLanguage{},
		&database.Translation{},
		&database.AiWaiterConversation{},
		&database.MenuWizardSession{},
		&database.MenuExtractionJob{},
		&database.PluginNotificationDelivery{},
		&database.TelegramConnectionToken{},
		&database.TelegramUpdateReceipt{},
		&database.RBACAuditLog{},
		&database.CompVoidAudit{},
		&database.WithdrawalHistory{},
		&database.ReportSchedule{},
		&database.PushSubscription{},
		&database.InventoryAlertLog{},
		&database.DirectorActionAudit{},
		&database.DirectorToolCall{},
		&database.OperationalAlertEvent{},
		&database.OperationalAlert{},
		&database.BusinessPlugin{},
		&database.PayrollLineItem{},
		&database.PayrollRun{},
		&database.ManualLedgerEntry{},
		&database.CashRegisterMovement{},
		&database.CashRegisterSession{},
		&database.PrintJob{},
		&database.Printer{},
		&database.FiscalAuditEvent{},
		&database.FiscalJob{},
		&database.FiscalReceipt{},
		&database.BusinessFiscalSettings{},
		&database.PollVote{},
		&database.PollOption{},
		&database.Poll{},
		&database.Shoutout{},
		&database.DocumentAck{},
		&database.Document{},
		&database.ChecklistItemCompletion{},
		&database.ChecklistRun{},
		&database.ChecklistItem{},
		&database.ChecklistTemplate{},
		&database.StaffNotification{},
		&database.ShiftNote{},
		&database.AnnouncementAck{},
		&database.Announcement{},
		&database.ChatRead{},
		&database.ChatMessage{},
		&database.ChatChannelMember{},
		&database.ChatChannel{},
		&database.TimeEntry{},
		&database.TimeOffRequest{},
		&database.StaffAvailability{},
		&database.ShiftSwapRequest{},
		&database.OpenShiftClaim{},
		&database.Shift{},
		&database.Schedule{},
		&database.BusinessScheduleSettings{},
		&database.StaffPosition{},
		&database.Position{},
		&database.DeliveryOrder{},
		&database.DeliveryDriver{},
		&database.DeliveryZone{},
		&database.DeliverySettings{},
		&database.CustomerCommunication{},
		&database.CustomerAddress{},
		&database.CustomerBusiness{},
		&database.InventoryMovement{},
		&database.InventoryRecipe{},
		&database.InventoryItem{},
		&database.InventorySettings{},
		&database.LoyaltyProgram{},
		&database.TableReservation{},
		&database.ReservationSettings{},
		&database.BusinessSpecialFeature{},
		&database.BusinessOperatingHours{},
		&database.BusinessGalleryImage{},
		&database.Order{},
		&database.BillHistoryEvent{},
		&database.Bill{},
		&database.Menu{},
		&database.Counter{},
		&database.Table{},
		&database.AIGeneratedImage{},
		&database.AIImageUsage{},
		&database.DirectorProposedAction{},
		&database.DirectorConsoleMessage{},
		&database.DirectorConsoleThread{},
		&database.BusinessAlertSettings{},
		&database.StaffInvitation{},
		&database.Staff{},
	}
}

func (s *Service) deleteOwnedDemoData(ctx context.Context, tx *gorm.DB, adminUserID uint) error {
	var businesses []database.Business
	if err := tx.WithContext(ctx).
		Where("is_demo = ? AND demo_owner_user_id = ?", true, adminUserID).
		Find(&businesses).Error; err != nil {
		return err
	}
	if len(businesses) == 0 {
		return nil
	}
	businessIDs := make([]uint, 0, len(businesses))
	for _, business := range businesses {
		businessIDs = append(businessIDs, business.ID)
	}

	var billIDs []uint
	if err := tx.WithContext(ctx).Model(&database.Bill{}).Where("business_id IN ?", businessIDs).Pluck("id", &billIDs).Error; err != nil {
		return err
	}
	var deliveryIDs []uint
	if err := tx.WithContext(ctx).Model(&database.DeliveryOrder{}).Where("business_id IN ?", businessIDs).Pluck("id", &deliveryIDs).Error; err != nil {
		return err
	}
	var reservationIDs []uint
	if err := tx.WithContext(ctx).Model(&database.TableReservation{}).Where("business_id IN ?", businessIDs).Pluck("id", &reservationIDs).Error; err != nil {
		return err
	}
	var customerBusinessIDs []uint
	if err := tx.WithContext(ctx).Model(&database.CustomerBusiness{}).Where("business_id IN ?", businessIDs).Pluck("id", &customerBusinessIDs).Error; err != nil {
		return err
	}
	var customerIDs []uint
	if err := tx.WithContext(ctx).Model(&database.CustomerBusiness{}).Where("business_id IN ?", businessIDs).Pluck("customer_id", &customerIDs).Error; err != nil {
		return err
	}
	var loyaltyProgramIDs []uint
	if err := tx.WithContext(ctx).Model(&database.LoyaltyProgram{}).Where("business_id IN ?", businessIDs).Pluck("id", &loyaltyProgramIDs).Error; err != nil {
		return err
	}
	var aiWaiterConversationIDs []uint
	if err := tx.WithContext(ctx).Model(&database.AiWaiterConversation{}).Where("business_id IN ?", businessIDs).Pluck("id", &aiWaiterConversationIDs).Error; err != nil {
		return err
	}
	var menuWizardSessionIDs []uint
	if err := tx.WithContext(ctx).Model(&database.MenuWizardSession{}).Where("business_id IN ?", businessIDs).Pluck("id", &menuWizardSessionIDs).Error; err != nil {
		return err
	}
	var menuExtractionJobIDs []uint
	if err := tx.WithContext(ctx).Model(&database.MenuExtractionJob{}).Where("business_id IN ?", businessIDs).Pluck("id", &menuExtractionJobIDs).Error; err != nil {
		return err
	}
	var staffIDs []uint
	if err := tx.WithContext(ctx).Model(&database.Staff{}).Where("business_id IN ?", businessIDs).Pluck("id", &staffIDs).Error; err != nil {
		return err
	}
	var pluginDeliveryIDs []uint
	if err := tx.WithContext(ctx).Model(&database.PluginNotificationDelivery{}).Where("business_id IN ?", businessIDs).Pluck("id", &pluginDeliveryIDs).Error; err != nil {
		return err
	}
	var paymentIDs []uint
	if len(billIDs) > 0 {
		if err := tx.WithContext(ctx).Model(&database.Payment{}).Where("bill_id IN ?", billIDs).Pluck("id", &paymentIDs).Error; err != nil {
			return err
		}
	}

	childIDs := map[string][]uint{
		"bills":                          billIDs,
		"payments":                       paymentIDs,
		"delivery_orders":                deliveryIDs,
		"table_reservations":             reservationIDs,
		"customer_businesses":            customerBusinessIDs,
		"customers":                      customerIDs,
		"loyalty_programs":               loyaltyProgramIDs,
		"ai_waiter_conversations":        aiWaiterConversationIDs,
		"menu_wizard_sessions":           menuWizardSessionIDs,
		"menu_extraction_jobs":           menuExtractionJobIDs,
		"plugin_notification_deliveries": pluginDeliveryIDs,
		"staff":                          staffIDs,
	}
	for _, spec := range demoWipeChildSpecs {
		ids := childIDs[spec.parent]
		if len(ids) == 0 {
			continue
		}
		if err := tx.WithContext(ctx).Where(spec.column+" IN ?", ids).Delete(spec.model).Error; err != nil {
			return err
		}
	}

	deleteModels := demoWipeBusinessModels()
	for _, model := range deleteModels {
		q := tx.WithContext(ctx)
		// Soft-deleting models keep the row (and its FK to the business);
		// the wipe needs hard deletes or the business delete still fails.
		switch model.(type) {
		case *database.ChatMessage, *database.DirectorToolCall:
			q = q.Unscoped()
		}
		if err := q.Where("business_id IN ?", businessIDs).Delete(model).Error; err != nil {
			return err
		}
	}
	if len(customerIDs) > 0 {
		if err := tx.WithContext(ctx).
			Where("id IN ? AND email LIKE ?", customerIDs, fmt.Sprintf("demo+admin%d-%%", adminUserID)).
			Delete(&database.Customer{}).Error; err != nil {
			return err
		}
	}
	// The instance row still points at the businesses about to be deleted.
	// The SQL migration's FK is ON DELETE SET NULL, but GORM's auto-migrate
	// safety net adds its own NO ACTION constraint alongside it — clear the
	// pointers first so neither can block the delete.
	if err := tx.WithContext(ctx).Model(&database.DemoInstance{}).
		Where("admin_user_id = ?", adminUserID).
		Updates(map[string]interface{}{"primary_business_id": nil, "secondary_business_id": nil}).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Where("id IN ? AND is_demo = ? AND demo_owner_user_id = ?", businessIDs, true, adminUserID).Delete(&database.Business{}).Error
}
