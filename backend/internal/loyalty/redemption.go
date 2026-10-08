// Package loyalty provides standalone loyalty-point redemption logic that is
// free from import-cycle constraints (it depends only on database and gorm).
package loyalty

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"gorm.io/gorm"
)

// loadLoyaltyProgram returns the LoyaltyProgram for the given business.
// A missing table or record yields a disabled zero-point default so callers
// keep working on databases that haven't run the loyalty migration yet.
// Tiers are not preloaded: redeem/undo only read Enabled and
// RedemptionPointsPerDollar. Guest rate reads go through
// RedemptionRateForBusiness, which projects just those two columns.
func loadLoyaltyProgram(db *gorm.DB, businessID uint) (*database.LoyaltyProgram, error) {
	disabled := &database.LoyaltyProgram{Enabled: false, PointsPerDollar: 0, RedemptionPointsPerDollar: 0}
	var p database.LoyaltyProgram
	if err := db.Select("id", "business_id", "enabled", "points_per_dollar", "redemption_points_per_dollar").
		Where("business_id = ?", businessID).First(&p).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || strings.Contains(err.Error(), "no such table") {
			return disabled, nil
		}
		return nil, err
	}
	return &p, nil
}

// redemptionRate returns points required per $1 discount, or 0 when the program
// is disabled / has no redeem rate configured. This is independent of the earn
// rate (PointsPerDollar). There is no silent default: callers MUST treat 0 as
// "not configured" and refuse to redeem (a wrong rate silently moves money).
func redemptionRate(program *database.LoyaltyProgram) float64 {
	if program == nil || !program.Enabled || program.RedemptionPointsPerDollar <= 0 {
		return 0
	}
	return program.RedemptionPointsPerDollar
}

// RedemptionRateForBusiness returns the current points-per-dollar redemption
// rate for a business, or 0 when no rate is configured (disabled program, unset
// rate, or load error). Guest UI uses this to show an accurate discount estimate;
// a 0 means redemption is unavailable.
//
// Projects only enabled + redemption_points_per_dollar. Tiers and the rest of
// the loyalty_programs row are never read on this path (#560).
func RedemptionRateForBusiness(db *gorm.DB, businessID uint) float64 {
	if db == nil || businessID == 0 {
		return 0
	}
	var program database.LoyaltyProgram
	if err := db.Select("enabled", "redemption_points_per_dollar").
		Where("business_id = ?", businessID).
		Take(&program).Error; err != nil {
		return 0
	}
	return redemptionRate(&program)
}

// RedeemPoints deducts loyalty points from the customer-business relationship
// and applies a discount to the bill. It returns the discount in cents and the
// remaining loyalty point balance.
//
// The business must have an active loyalty program; if redemption is disabled
// (no program, disabled program, or zero rate) an error is returned.
//
// The update is atomic: both the point deduction and the bill update happen
// inside a single database transaction.
func RedeemPoints(db *gorm.DB, customerID uint, billID uint, businessID uint, points int) (discountCents int64, pointsDeducted int, remainingPoints int, err error) {
	if points <= 0 {
		return 0, 0, 0, ErrPointsMustBePositive
	}

	program, err := loadLoyaltyProgram(db, businessID)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to load loyalty program: %w", err)
	}

	if !program.Enabled {
		return 0, 0, 0, ErrProgramNotEnabled
	}

	// No silent default: an enabled program with no configured rate is a
	// misconfiguration, not "redeem at 100 pts/$". Refuse rather than silently
	// move money at a wrong rate.
	rate := redemptionRate(program)
	if rate <= 0 {
		return 0, 0, 0, ErrRateNotConfigured
	}

	// discountDollars = points / rate. This is the customer's requested discount;
	// it is clamped to the bill's gross total inside the transaction so surplus
	// points are never consumed (see the clamp below).
	requestedDiscountCents := int64(math.Round(float64(points) / rate * 100))
	if requestedDiscountCents <= 0 {
		return 0, 0, 0, ErrPointsTooSmall
	}

	var remaining int
	var appliedPoints int
	var appliedDiscountCents int64

	txErr := db.Transaction(func(tx *gorm.DB) error {
		// Load the bill FIRST so we can clamp the redemption to what the bill
		// actually owes before deducting any points. Apply the discount only while
		// the bill is still open or partially paid. TotalAmount is the single
		// source of truth every owed-amount calculation reads (guest "remaining",
		// the plugin charge amount, the fully-paid check, printers, accounting), so
		// recording the discount without reducing the total leaves the redemption
		// inert: the guest spends points and still pays full price.
		var bill database.Bill
		if err := tx.Select("id", "business_id", "counter_id", "subtotal", "tax_amount", "service_fee_amount", "status", "loyalty_discount_cents", "paid_amount", "closed_at", "settled_at").
			Where("id = ? AND status IN ('open', 'partial')", billID).
			First(&bill).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrBillNotOpen
			}
			return fmt.Errorf("failed to load bill: %w", err)
		}

		// Invariant: at most one active loyalty redemption per bill. A restaurant
		// table shares ONE open bill across every authenticated guest seated there,
		// so an unconditional discount write would let a second guest's redemption
		// overwrite the first guest's redeemer record and shrink the discount —
		// the first guest's points were already deducted but the bill would record
		// only the second. Voiding then refunds only the second guest, silently
		// losing the first's points. The legitimate same-customer "redeem more"
		// flow is undo-then-redeem (the guest UI hides "redeem" once applied), so
		// requiring an explicit undo first is both safe and the existing behavior.
		if bill.LoyaltyDiscountCents > 0 {
			return ErrDiscountAlreadyApplied
		}

		// Recompute from the stored gross components (subtotal + tax + fee) rather
		// than the live TotalAmount, so the math is independent of any prior state.
		grossTotal := bill.Subtotal + bill.TaxAmount + bill.ServiceFeeAmount

		// Only the unpaid remainder can be discounted. Clamping against the gross
		// total would let total_amount drop below paid_amount (negative balance
		// owed / stuck bill). A fully-covered bill has nothing left to discount.
		remainingCents := grossTotal - bill.PaidAmount
		if remainingCents <= 0 {
			return fmt.Errorf("%w: bill %d", ErrBillFullyCovered, billID)
		}

		// Clamp the discount to the unpaid remainder. Without this, a guest with
		// (say) 10,000 points against a $20 bill would have all 10,000 points
		// deducted while NetBillTotalCents floors the applied discount at $20 —
		// silently destroying the surplus point value. Instead we cap the discount
		// at what is still owed and consume only the points that discount is worth,
		// leaving the rest of the balance intact.
		discountCents := requestedDiscountCents
		pointsToDeduct := points
		if discountCents > remainingCents {
			discountCents = remainingCents
			// Points worth exactly this capped discount, rounded up so the discount
			// is fully covered. dc>remaining implies points > remaining*rate ≥ pointsToDeduct,
			// so this never exceeds the requested (already-verified-available) points.
			pointsToDeduct = int(math.Ceil(float64(remainingCents) / 100 * rate))
			if pointsToDeduct > points {
				pointsToDeduct = points
			}
		}

		// Deduct the (clamped) points from customer_businesses row — atomically
		// guarded by loyalty_points >= ? so we never go negative.
		result := tx.Model(&database.CustomerBusiness{}).
			Where("customer_id = ? AND business_id = ? AND loyalty_points >= ? AND is_active = ?",
				customerID, businessID, pointsToDeduct, true).
			Updates(map[string]interface{}{
				"loyalty_points": gorm.Expr("loyalty_points - ?", pointsToDeduct),
			})
		if result.Error != nil {
			return fmt.Errorf("failed to deduct points: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			// Either customer has no connection to this business or insufficient points.
			// Distinguish the two for a useful error message.
			var cb database.CustomerBusiness
			lookupErr := tx.Select("loyalty_points").
				Where("customer_id = ? AND business_id = ? AND is_active = ?", customerID, businessID, true).
				First(&cb).Error
			if errors.Is(lookupErr, gorm.ErrRecordNotFound) {
				return ErrNotConnected
			}
			return fmt.Errorf("%w: have %d, need %d", ErrInsufficientPoints, cb.LoyaltyPoints, pointsToDeduct)
		}

		netTotal := database.NetBillTotalCents(grossTotal, discountCents)
		// Race-safety: the read above and this write are not atomic on their own —
		// two guests can both read loyalty_discount_cents = 0 and both proceed.
		// Gate the write on loyalty_discount_cents = 0 in SQL so only one wins; if
		// a concurrent tx already applied a discount, RowsAffected == 0 and we
		// return the same error, rolling back this tx (including the point
		// deduction above). paid_amount in the WHERE is a CAS guard: if a payment
		// lands between our read and this write, the clamp math is stale and the
		// redemption fails instead of underwriting collected money.
		updates := map[string]interface{}{
			"loyalty_discount_cents":          discountCents,
			"total_amount":                    netTotal,
			"loyalty_points_redeemed":         pointsToDeduct,
			"loyalty_redeemed_by_customer_id": customerID,
		}
		if bill.PaidAmount >= netTotal {
			// Payments already cover the discounted total. The shared direct-paid
			// finalizer below owns closure/settlement timestamps, occupancy release,
			// and the paid hook.
			updates["status"] = database.BillStatusPaid
			bill.Status = database.BillStatusPaid
		}
		result = tx.Model(&database.Bill{}).
			Where("id = ? AND loyalty_discount_cents = 0 AND paid_amount = ?", bill.ID, bill.PaidAmount).
			Updates(updates)
		if result.Error != nil {
			return fmt.Errorf("failed to apply discount to bill: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			// Bill is still open (we loaded it) but a discount got applied between
			// our read and this write — a concurrent redemption won the race.
			// (Or paid_amount moved: CAS guard rejected a stale clamp.)
			return ErrDiscountAlreadyApplied
		}
		bill.TotalAmount = netTotal
		if bill.Status == database.BillStatusPaid {
			if err := database.FinalizeDirectBillPaidTransitionTx(tx, &bill, time.Time{}); err != nil {
				return fmt.Errorf("failed to finalize loyalty-settled bill: %w", err)
			}
		}
		appliedPoints = pointsToDeduct
		appliedDiscountCents = discountCents

		// Read back the remaining balance inside the transaction for a consistent result.
		var cb database.CustomerBusiness
		if err := tx.Select("loyalty_points").
			Where("customer_id = ? AND business_id = ?", customerID, businessID).
			First(&cb).Error; err != nil {
			return fmt.Errorf("failed to read updated loyalty points: %w", err)
		}
		remaining = cb.LoyaltyPoints

		return nil
	})
	if txErr != nil {
		return 0, 0, 0, txErr
	}

	return appliedDiscountCents, appliedPoints, remaining, nil
}

// UndoRedemption reverses a previously applied loyalty redemption: the points
// are re-credited to the customer and the bill discount is zeroed out.
//
// The bill must still be open or partially paid; redemptions on closed bills
// cannot be undone via this path.
func UndoRedemption(db *gorm.DB, customerID uint, billID uint, businessID uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// Read the current discount, gross components, redeemer, and the points
		// recorded at redeem time on the bill.
		var bill database.Bill
		if err := tx.Select("id", "status", "loyalty_discount_cents", "subtotal", "tax_amount", "service_fee_amount", "loyalty_redeemed_by_customer_id", "loyalty_points_redeemed").
			Where("id = ? AND business_id = ? AND status IN ('open', 'partial')", billID, businessID).
			First(&bill).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrBillNotOpen
			}
			return fmt.Errorf("failed to load bill: %w", err)
		}

		if bill.LoyaltyDiscountCents <= 0 {
			return ErrNoDiscountToUndo
		}

		// Ownership guard: only the guest who applied the discount may undo it.
		// A table shares one open bill across every authenticated guest, so without
		// this any guest could undo another guest's redemption and pocket the
		// re-credited points. RedeemPoints always records the redeemer, so a
		// discount without one is not undoable by any guest.
		if bill.LoyaltyRedeemedByCustomerID == nil || *bill.LoyaltyRedeemedByCustomerID != customerID {
			return ErrUndoNotOwner
		}

		// Return the exact points recorded at redeem time (rate-independent), so
		// an undo is correct even if the rate changed or was cleared since.
		pointsToReturn := bill.LoyaltyPointsRedeemed
		if pointsToReturn <= 0 {
			return ErrCannotDetermineRefund
		}

		// Race-safety: clear the bill discount FIRST, gated in SQL on the discount
		// still being present (loyalty_discount_cents > 0). Two concurrent undos can
		// both read a non-zero discount; only the tx whose conditional UPDATE wins
		// (RowsAffected == 1) proceeds to credit the points — the loser sees
		// RowsAffected == 0 and rolls back, so points are credited at most once.
		// Performing the clear before the point credit means a rollback here leaves
		// no stray credit behind.
		grossTotal := bill.Subtotal + bill.TaxAmount + bill.ServiceFeeAmount
		clear := tx.Model(&database.Bill{}).
			Where("id = ? AND loyalty_discount_cents > ?", billID, 0).
			Updates(map[string]interface{}{
				"loyalty_discount_cents": 0,
				"total_amount":           grossTotal,
				// Clear the redeemer linkage so a later void can't double-refund the
				// points this undo already returned.
				"loyalty_points_redeemed":         0,
				"loyalty_redeemed_by_customer_id": nil,
			})
		if clear.Error != nil {
			return fmt.Errorf("failed to clear bill discount: %w", clear.Error)
		}
		if clear.RowsAffected == 0 {
			// A concurrent undo already cleared the discount; nothing to reverse.
			return ErrNoDiscountToUndo
		}

		// Re-add points to the customer-business record.
		result := tx.Model(&database.CustomerBusiness{}).
			Where("customer_id = ? AND business_id = ? AND is_active = ?", customerID, businessID, true).
			Updates(map[string]interface{}{
				"loyalty_points": gorm.Expr("loyalty_points + ?", pointsToReturn),
			})
		if result.Error != nil {
			return fmt.Errorf("failed to restore points: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return ErrNotConnected
		}

		return nil
	})
}
