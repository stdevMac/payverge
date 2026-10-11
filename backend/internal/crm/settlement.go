package crm

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// RecordBillSettlementVisit records the CRM visit created by a paid bill settlement.
func (s *Service) RecordBillSettlementVisit(billID uint) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var bill database.Bill
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Preload("Business").
			Preload("ItemsRelation").
			First(&bill, billID).Error; err != nil {
			return fmt.Errorf("load bill for CRM settlement visit: %w", err)
		}

		if bill.Status != database.BillStatusPaid || bill.CRMCustomerID == nil || !bill.Business.CRMEnabled {
			return nil
		}

		var existingCount int64
		if err := tx.Model(&database.CustomerVisit{}).
			Where("bill_id = ?", bill.ID).
			Count(&existingCount).Error; err != nil {
			return fmt.Errorf("check existing CRM settlement visit: %w", err)
		}
		if existingCount > 0 {
			return nil
		}

		var customerBusiness database.CustomerBusiness
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("customer_id = ? AND business_id = ? AND is_active = ?", *bill.CRMCustomerID, bill.BusinessID, true).
			First(&customerBusiness).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return fmt.Errorf("load active customer-business connection: %w", err)
		}

		// PutLoyalty locks customer_businesses before the loyalty program. Keep
		// the same order here to prevent a save/settlement deadlock.
		program, err := s.loadLoyaltyProgramForSettlement(tx, bill.BusinessID)
		if err != nil {
			return fmt.Errorf("load loyalty program for CRM settlement: %w", err)
		}

		itemsPurchased, err := settlementItemsPurchasedJSON(bill)
		if err != nil {
			return err
		}

		amountSpent := float64(bill.TotalAmount) / 100
		pointsEarned := computePointsEarned(bill.TotalAmount, program)
		newLifetimeCents := lifetimeSpentCents(customerBusiness.TotalSpent) + bill.TotalAmount
		newTier := computeTier(newLifetimeCents, program)
		now := time.Now()
		visitBillID := bill.ID
		var tableID *uint
		if bill.TableID != 0 {
			tableIDValue := bill.TableID
			tableID = &tableIDValue
		}

		visit := &database.CustomerVisit{
			CustomerBusinessID: customerBusiness.ID,
			BillID:             &visitBillID,
			TableID:            tableID,
			AmountSpent:        amountSpent,
			PointsEarned:       pointsEarned,
			ItemsPurchased:     itemsPurchased,
			VisitDate:          now,
		}
		if err := tx.Create(visit).Error; err != nil {
			return fmt.Errorf("create CRM settlement visit: %w", err)
		}

		updates := map[string]interface{}{
			"total_spent":    gorm.Expr("total_spent + ?", amountSpent),
			"visit_count":    gorm.Expr("visit_count + 1"),
			"loyalty_points": gorm.Expr("loyalty_points + ?", pointsEarned),
			"last_visit_at":  now,
		}
		if newTier != "" {
			updates["loyalty_tier"] = newTier
		}
		if err := tx.Model(&database.CustomerBusiness{}).
			Where("id = ?", customerBusiness.ID).
			Updates(updates).Error; err != nil {
			return fmt.Errorf("update CRM customer-business settlement stats: %w", err)
		}

		return nil
	})
}

func settlementItemsPurchasedJSON(bill database.Bill) (string, error) {
	if len(bill.ItemsRelation) > 0 {
		items, err := json.Marshal(bill.ItemsRelation)
		if err != nil {
			return "", fmt.Errorf("marshal settlement bill items: %w", err)
		}
		return string(items), nil
	}

	if strings.TrimSpace(bill.Items) != "" {
		return bill.Items, nil
	}

	return "[]", nil
}
