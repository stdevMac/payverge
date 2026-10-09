package server

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

// Host Live View floor actions (seat / clear / transfer / merge).
// Gated on tables:write so Host / Server / Manager can run dinner floor from
// the Tables tab without needing bills:create / bills:close / bills:refund.

type seatTableRequest struct {
	PartySize     int    `json:"party_size"`
	ReservationID *uint  `json:"reservation_id"`
	Notes         string `json:"notes"`
}

type transferTableRequest struct {
	TargetTableID uint `json:"target_table_id" binding:"required"`
}

type mergeTableRequest struct {
	TargetTableID uint `json:"target_table_id" binding:"required"`
}

// SeatTable seats a walk-in (empty open check) or checks in an assigned
// reservation from the Live View floor list.
func SeatTable(c *gin.Context) {
	business, table, ok := requireFloorTable(c)
	if !ok {
		return
	}

	var req seatTableRequest
	// Empty body is fine for a plain walk-in seat.
	_ = c.ShouldBindJSON(&req)

	// Prefer an explicit reservation, else the next assigned pending/confirmed
	// booking on this table — that's the host "they're here" path.
	reservationID := uint(0)
	if req.ReservationID != nil && *req.ReservationID > 0 {
		reservationID = *req.ReservationID
	} else if nextID, found := nextSeatableReservationID(business.ID, table.ID); found {
		reservationID = nextID
	}

	if reservationID > 0 {
		seatReservationFromFloor(c, business.ID, reservationID)
		return
	}

	bill, err := database.SeatWalkIn(database.SeatWalkInInput{
		BusinessID: business.ID,
		TableID:    table.ID,
		PartySize:  req.PartySize,
		Notes:      req.Notes,
		Actor:      getBusinessActionActor(c),
		StaffID:    staffIDFromContext(c),
	})
	if err != nil {
		respondFloorError(c, err)
		return
	}

	events.GetHub().PublishJSON(business.ID, "bill.created", gin.H{"bill_id": bill.ID, "table_id": table.ID})
	_ = events.NotifyTableBillChangedByBillID(database.GetDB(), bill.ID)
	invalidateOwnerHomeCaches(business.ID)
	invalidateReservationAvailability(business.ID)

	c.JSON(http.StatusCreated, gin.H{
		"bill":     bill,
		"table_id": table.ID,
		"action":   "seat_walk_in",
	})
}

// ClearTable frees a table by voiding its empty unpaid open check.
func ClearTable(c *gin.Context) {
	business, table, ok := requireFloorTable(c)
	if !ok {
		return
	}

	bill, err := database.ClearTable(database.ClearTableInput{
		BusinessID: business.ID,
		TableID:    table.ID,
		Actor:      getBusinessActionActor(c),
	})
	if err != nil {
		respondFloorError(c, err)
		return
	}

	events.GetHub().PublishJSON(business.ID, "bill.closed", gin.H{"bill_id": bill.ID, "table_id": table.ID})
	_ = events.NotifyTableBillChangedByBillID(database.GetDB(), bill.ID)
	invalidateOwnerHomeCaches(business.ID)
	invalidateReservationAvailability(business.ID)

	c.JSON(http.StatusOK, gin.H{
		"bill":     bill,
		"table_id": table.ID,
		"action":   "clear",
	})
}

// TransferTable moves the open check from this table onto another free table.
func TransferTable(c *gin.Context) {
	business, table, ok := requireFloorTable(c)
	if !ok {
		return
	}

	var req transferTableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	bill, err := database.TransferActiveBill(database.TransferBillInput{
		BusinessID:    business.ID,
		SourceTableID: table.ID,
		TargetTableID: req.TargetTableID,
		Actor:         getBusinessActionActor(c),
	})
	if err != nil {
		respondFloorError(c, err)
		return
	}

	events.GetHub().PublishJSON(business.ID, "bill.updated", gin.H{
		"bill_id":       bill.ID,
		"from_table_id": table.ID,
		"to_table_id":   req.TargetTableID,
		"action":        "transfer",
	})
	_ = events.NotifyTableBillChangedByBillID(database.GetDB(), bill.ID)
	invalidateOwnerHomeCaches(business.ID)
	invalidateReservationAvailability(business.ID)

	c.JSON(http.StatusOK, gin.H{
		"bill":          bill,
		"from_table_id": table.ID,
		"to_table_id":   req.TargetTableID,
		"action":        "transfer",
	})
}

// MergeTable folds this table's open check into another table's check (or
// transfers when the target is free).
func MergeTable(c *gin.Context) {
	business, table, ok := requireFloorTable(c)
	if !ok {
		return
	}

	var req mergeTableRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}

	targetBill, sourceBill, err := database.MergeTableChecks(database.MergeTablesInput{
		BusinessID:    business.ID,
		SourceTableID: table.ID,
		TargetTableID: req.TargetTableID,
		Actor:         getBusinessActionActor(c),
	})
	if err != nil {
		respondFloorError(c, err)
		return
	}

	payload := gin.H{
		"target_bill":   targetBill,
		"from_table_id": table.ID,
		"to_table_id":   req.TargetTableID,
		"action":        "merge",
	}
	if sourceBill != nil {
		payload["source_bill"] = sourceBill
	}
	events.GetHub().PublishJSON(business.ID, "bill.updated", payload)
	if targetBill != nil {
		_ = events.NotifyTableBillChangedByBillID(database.GetDB(), targetBill.ID)
	}
	if sourceBill != nil {
		_ = events.NotifyTableBillChangedByBillID(database.GetDB(), sourceBill.ID)
	}
	invalidateOwnerHomeCaches(business.ID)
	invalidateReservationAvailability(business.ID)

	c.JSON(http.StatusOK, payload)
}

func requireFloorTable(c *gin.Context) (*database.Business, *database.Table, bool) {
	business, ok := getTableRouteBusiness(c)
	if !ok {
		return nil, nil, false
	}
	if !checkTableBusinessOwnership(c, business) {
		c.JSON(http.StatusForbidden, gin.H{"error": "You don't own this business"})
		return nil, nil, false
	}

	tableID, err := strconv.ParseUint(c.Param("tableId"), 10, 32)
	if err != nil || tableID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid table ID", "code": "invalid_table"})
		return nil, nil, false
	}

	table, err := database.GetTableByID(uint(tableID))
	if err != nil || table == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Table not found", "code": "table_not_found"})
		return nil, nil, false
	}
	if table.BusinessID != business.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Table does not belong to this business", "code": "business_mismatch"})
		return nil, nil, false
	}
	return business, table, true
}

func nextSeatableReservationID(businessID, tableID uint) (uint, bool) {
	var reservation database.TableReservation
	err := database.GetDB().
		Where("business_id = ? AND table_id = ? AND status IN ?", businessID, tableID, []string{"pending", "confirmed"}).
		Order("reservation_time ASC").
		First(&reservation).Error
	if err != nil {
		return 0, false
	}
	return reservation.ID, true
}

func seatReservationFromFloor(c *gin.Context, businessID, reservationID uint) {
	actor := reservationClaimActor(c)
	svc := getReservationService()

	// Claim first so check-in satisfies the exclusive-operator lock. Steal when
	// the actor is allowed (manager/owner) so a stuck claim cannot block seating.
	if _, _, err := svc.ClaimReservation(businessID, reservationID, actor, actor.CanSteal); err != nil {
		var held *services.ReservationClaimHeldError
		if errors.As(err, &held) {
			c.JSON(http.StatusConflict, gin.H{
				"error":           "Another teammate is working this reservation",
				"code":            "reservation_claimed",
				"claimed_by_name": held.ClaimedByName,
				"claimed_by_role": held.ClaimedByRole,
			})
			return
		}
		if errors.Is(err, services.ErrReservationClaimStealRequired) {
			c.JSON(http.StatusConflict, gin.H{
				"error": "This reservation is claimed by another teammate",
				"code":  "reservation_claimed",
			})
			return
		}
		if errors.Is(err, services.ErrReservationClaimForbidden) {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Not allowed to take over this reservation claim",
				"code":  "reservation_claim_forbidden",
			})
			return
		}
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "Reservation not found", "code": "reservation_not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to claim reservation"})
		return
	}

	reservation, err := svc.TransitionReservation(
		businessID,
		reservationID,
		"check_in",
		services.ReservationTransitionInput{},
		getReservationActor(c),
	)
	if err != nil {
		respondReservationWriteError(c, err)
		return
	}

	// Best-effort release so the board doesn't stay locked after seating.
	_ = svc.ReleaseReservation(businessID, reservationID, actor, false)
	invalidateReservationAvailability(businessID)

	events.GetHub().PublishJSON(reservation.BusinessID, "reservation.updated", reservationEventPayload(reservation))
	resolveReservationOperationalAlert(c, reservation)
	invalidateOwnerHomeCaches(businessID)

	c.JSON(http.StatusOK, gin.H{
		"reservation": reservation,
		"table_id":    reservation.TableID,
		"action":      "seat_reservation",
	})
}

func respondFloorError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, database.ErrFloorTableNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "Table not found", "code": "table_not_found"})
	case errors.Is(err, database.ErrFloorTableInactive):
		c.JSON(http.StatusBadRequest, gin.H{"error": "Table is inactive", "code": "table_inactive"})
	case errors.Is(err, database.ErrFloorTableOccupied):
		c.JSON(http.StatusConflict, gin.H{"error": "Table already has an open check", "code": "table_occupied"})
	case errors.Is(err, database.ErrFloorNoActiveBill):
		c.JSON(http.StatusConflict, gin.H{"error": "Table has no open check to clear or move", "code": "no_active_bill"})
	case errors.Is(err, database.ErrFloorSettleRequired):
		c.JSON(http.StatusConflict, gin.H{
			"error": "Settle or void the open check before clearing this table",
			"code":  "settle_required",
		})
	case errors.Is(err, database.ErrFloorLiveKitchenTickets):
		c.JSON(http.StatusConflict, gin.H{
			"error": "The kitchen is still working this table — bump or cancel the open tickets first",
			"code":  "kitchen_tickets_live",
		})
	case errors.Is(err, database.ErrFloorPendingOrders):
		c.JSON(http.StatusConflict, gin.H{
			"error": "A guest order is still waiting for approval — approve or reject it before freeing this table",
			"code":  "orders_pending_approval",
		})
	case errors.Is(err, database.ErrFloorTargetOccupied):
		c.JSON(http.StatusConflict, gin.H{"error": "Target table already has an open check", "code": "target_occupied"})
	case errors.Is(err, database.ErrFloorTargetSame):
		c.JSON(http.StatusBadRequest, gin.H{"error": "Pick a different table", "code": "same_table"})
	case errors.Is(err, database.ErrFloorBusinessMismatch):
		c.JSON(http.StatusBadRequest, gin.H{"error": "Table does not belong to this business", "code": "business_mismatch"})
	case errors.Is(err, database.ErrFloorMergePaymentsBlock):
		c.JSON(http.StatusConflict, gin.H{
			"error": "Cannot merge checks that already have payments — settle them first",
			"code":  "merge_payments_block",
		})
	case errors.Is(err, database.ErrFloorMergeLoyaltyBlock):
		c.JSON(http.StatusConflict, gin.H{
			"error": "Both checks have a loyalty redemption — remove one before merging",
			"code":  "merge_loyalty_block",
		})
	case errors.Is(err, database.ErrFloorBillNotTransferable):
		c.JSON(http.StatusConflict, gin.H{"error": "Only open checks can be moved", "code": "not_transferable"})
	default:
		RespondWithError(c, http.StatusInternalServerError, ErrCodeInternal, "Floor action failed")
	}
}
