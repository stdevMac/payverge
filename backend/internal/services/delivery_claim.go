package services

import (
	"errors"
	"fmt"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"gorm.io/gorm"
)

// deliveryClaimIdleTTL matches aiClaimIdleTTL (5 minutes): an abandoned claim
// becomes re-claimable without an explicit steal.
const deliveryClaimIdleTTL = 5 * time.Minute

// ClaimActor is the operator attempting a claim or mutation under claim gate.
type ClaimActor struct {
	StaffID  *uint
	Name     string
	Role     string
	CanSteal bool // owners + managers (dispatch:write override / ai_waiter:write)
}

// DeliveryClaimHeldError is returned when another operator holds a fresh claim.
// Handlers map it to HTTP 409 with the holder name so the blocked operator
// knows who to ask.
type DeliveryClaimHeldError struct {
	ClaimedByStaffID *uint
	ClaimedByName    string
	ClaimedByRole    string
}

func (e *DeliveryClaimHeldError) Error() string {
	if e.ClaimedByName != "" {
		return fmt.Sprintf("delivery claimed by %s", e.ClaimedByName)
	}
	return "delivery claimed by another operator"
}

// ErrDeliveryClaimStealRequired is returned when a steal-capable actor tries
// to take a held claim without the explicit steal flag.
var ErrDeliveryClaimStealRequired = errors.New("explicit steal required to take this claim")

// ErrDeliveryClaimForbidden is returned when a front-line actor cannot steal.
var ErrDeliveryClaimForbidden = errors.New("cannot take a claim held by another operator")

// SweepStaleDeliveryClaims clears claims whose claimed_at is past the idle TTL
// for one business. Called lazily from the dispatch list path (mirrors AI
// waiter GetAiConversations).
func (s *DeliveryService) SweepStaleDeliveryClaims(businessID uint) error {
	idleCutoff := time.Now().Add(-deliveryClaimIdleTTL)
	return s.db.Model(&database.DeliveryOrder{}).
		Where("business_id = ? AND claimed_at IS NOT NULL AND claimed_at < ?", businessID, idleCutoff).
		Updates(map[string]interface{}{
			"claimed_by_staff_id": nil,
			"claimed_by_name":     "",
			"claimed_by_role":     "",
			"claimed_at":          nil,
		}).Error
}

// ClaimDeliveryOrder atomically assigns the delivery to the acting operator.
// Succeeds when unclaimed, already held by the caller, the claim is idle past
// the TTL, or steal=true with CanSteal. Steal records an RBAC audit row and
// returns the previous holder so the caller can notify them.
func (s *DeliveryService) ClaimDeliveryOrder(businessID, deliveryID uint, actor ClaimActor, steal bool) (order *database.DeliveryOrder, previousStaffID *uint, err error) {
	now := time.Now()
	idleCutoff := now.Add(-deliveryClaimIdleTTL)

	var prevStaffID *uint
	var prevName, prevRole string
	var prevFresh bool

	err = s.db.Transaction(func(tx *gorm.DB) error {
		var current database.DeliveryOrder
		if err := tx.Where("id = ? AND business_id = ?", deliveryID, businessID).
			Select("id", "business_id", "delivery_number", "claimed_by_staff_id", "claimed_by_name", "claimed_by_role", "claimed_at").
			First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("delivery order not found")
			}
			return err
		}

		prevFresh = claimIsFresh(current.ClaimedAt, now)
		if prevFresh {
			prevStaffID = current.ClaimedByStaffID
			prevName = current.ClaimedByName
			prevRole = current.ClaimedByRole
		}

		if actorHoldsDeliveryClaim(current, actor) {
			// Self re-claim: refresh claimed_at only. Do not audit/notify.
			prevStaffID, prevName, prevRole = nil, "", ""
			return tx.Model(&database.DeliveryOrder{}).
				Where("id = ? AND business_id = ?", deliveryID, businessID).
				Updates(map[string]interface{}{
					"claimed_by_staff_id": actor.StaffID,
					"claimed_by_name":     actor.Name,
					"claimed_by_role":     actor.Role,
					"claimed_at":          now,
				}).Error
		}

		if prevFresh {
			if !steal {
				if actor.CanSteal {
					return ErrDeliveryClaimStealRequired
				}
				return &DeliveryClaimHeldError{
					ClaimedByStaffID: current.ClaimedByStaffID,
					ClaimedByName:    current.ClaimedByName,
					ClaimedByRole:    current.ClaimedByRole,
				}
			}
			if !actor.CanSteal {
				return ErrDeliveryClaimForbidden
			}
		}

		// Atomic claim: only land if still free/idle/self or steal authorized.
		q := tx.Model(&database.DeliveryOrder{}).
			Where("id = ? AND business_id = ?", deliveryID, businessID)
		if !steal || !actor.CanSteal {
			q = applyClaimableWhere(q, idleCutoff, actor)
		}
		res := q.Updates(map[string]interface{}{
			"claimed_by_staff_id": actor.StaffID,
			"claimed_by_name":     actor.Name,
			"claimed_by_role":     actor.Role,
			"claimed_at":          now,
		})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			var held database.DeliveryOrder
			_ = tx.Select("claimed_by_staff_id", "claimed_by_name", "claimed_by_role").
				Where("id = ?", deliveryID).First(&held).Error
			return &DeliveryClaimHeldError{
				ClaimedByStaffID: held.ClaimedByStaffID,
				ClaimedByName:    held.ClaimedByName,
				ClaimedByRole:    held.ClaimedByRole,
			}
		}

		if steal && actor.CanSteal && prevFresh && (prevStaffID != nil || prevName != "") {
			// Audited steal — atomic with the claim write. Never staff_id=0 (FK).
			staffForAudit := resolveClaimAuditStaffID(actor.StaffID, prevStaffID)
			if staffForAudit != 0 {
				reason := fmt.Sprintf("delivery_order:%d prev=%s (%s) stealer=%s (%s)",
					deliveryID, prevName, prevRole, actor.Name, actor.Role)
				if err := tx.Create(&database.RBACAuditLog{
					StaffID:    staffForAudit,
					BusinessID: businessID,
					Action:     database.RBACActionClaimStolen,
					ChangedBy:  actor.Name,
					Reason:     reason,
					CreatedAt:  now.UTC(),
				}).Error; err != nil {
					return err
				}
			}
		} else {
			// Non-steal path: clear prev so caller does not notify.
			prevStaffID = nil
			prevName = ""
			prevRole = ""
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	var out database.DeliveryOrder
	if err := s.db.Where("id = ? AND business_id = ?", deliveryID, businessID).First(&out).Error; err != nil {
		return nil, nil, err
	}
	if prevStaffID != nil || prevName != "" {
		// Return previous holder only for steal notify path.
		return &out, prevStaffID, nil
	}
	return &out, nil, nil
}

// ReleaseDeliveryOrder clears the claim. Front-line may release only their own;
// CanSteal actors may release any. Force-release of another operator's fresh
// claim is audited (L4-8 — force-release must not be silent).
func (s *DeliveryService) ReleaseDeliveryOrder(businessID, deliveryID uint, actor ClaimActor) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var current database.DeliveryOrder
		if err := tx.Where("id = ? AND business_id = ?", deliveryID, businessID).
			Select("id", "claimed_by_staff_id", "claimed_by_name", "claimed_by_role", "claimed_at").
			First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("delivery order not found")
			}
			return err
		}

		if !actor.CanSteal && !actorHoldsDeliveryClaim(current, actor) {
			return &DeliveryClaimHeldError{
				ClaimedByStaffID: current.ClaimedByStaffID,
				ClaimedByName:    current.ClaimedByName,
				ClaimedByRole:    current.ClaimedByRole,
			}
		}

		// L4-8: manager/owner releasing someone else's still-fresh claim is a
		// force-release and must land an audit row (steal already audits).
		forceRelease := actor.CanSteal &&
			!actorHoldsDeliveryClaim(current, actor) &&
			claimIsFresh(current.ClaimedAt, time.Now()) &&
			(current.ClaimedByStaffID != nil || current.ClaimedByName != "")

		if err := tx.Model(&database.DeliveryOrder{}).
			Where("id = ? AND business_id = ?", deliveryID, businessID).
			Updates(map[string]interface{}{
				"claimed_by_staff_id": nil,
				"claimed_by_name":     "",
				"claimed_by_role":     "",
				"claimed_at":          nil,
			}).Error; err != nil {
			return err
		}

		if forceRelease {
			staffForAudit := resolveClaimAuditStaffID(actor.StaffID, current.ClaimedByStaffID)
			if staffForAudit != 0 {
				reason := fmt.Sprintf(
					"delivery_order:%d prev=%s (%s) releaser=%s (%s)",
					deliveryID, current.ClaimedByName, current.ClaimedByRole, actor.Name, actor.Role,
				)
				if err := tx.Create(&database.RBACAuditLog{
					StaffID:    staffForAudit,
					BusinessID: businessID,
					Action:     database.RBACActionClaimForceReleased,
					ChangedBy:  actor.Name,
					Reason:     reason,
					CreatedAt:  time.Now().UTC(),
				}).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// AssertDeliveryClaimHeld returns a DeliveryClaimHeldError when the actor does
// not hold a fresh claim on the delivery. Used to gate patch/status/assign/cancel.
func (s *DeliveryService) AssertDeliveryClaimHeld(businessID, deliveryID uint, actor ClaimActor) error {
	var current database.DeliveryOrder
	if err := s.db.Where("id = ? AND business_id = ?", deliveryID, businessID).
		Select("id", "claimed_by_staff_id", "claimed_by_name", "claimed_by_role", "claimed_at").
		First(&current).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("delivery order not found")
		}
		return err
	}
	if !claimIsFresh(current.ClaimedAt, time.Now()) {
		return &DeliveryClaimHeldError{
			ClaimedByName: "",
			ClaimedByRole: "",
		}
	}
	if !actorHoldsDeliveryClaim(current, actor) {
		return &DeliveryClaimHeldError{
			ClaimedByStaffID: current.ClaimedByStaffID,
			ClaimedByName:    current.ClaimedByName,
			ClaimedByRole:    current.ClaimedByRole,
		}
	}
	return nil
}

// TouchDeliveryClaim refreshes claimed_at so an active dispatcher does not
// idle-expire mid-work.
func (s *DeliveryService) TouchDeliveryClaim(businessID, deliveryID uint) error {
	return s.db.Model(&database.DeliveryOrder{}).
		Where("id = ? AND business_id = ? AND claimed_at IS NOT NULL", deliveryID, businessID).
		Update("claimed_at", time.Now()).Error
}

func claimIsFresh(claimedAt *time.Time, now time.Time) bool {
	if claimedAt == nil {
		return false
	}
	return claimedAt.After(now.Add(-deliveryClaimIdleTTL))
}

func actorHoldsDeliveryClaim(order database.DeliveryOrder, actor ClaimActor) bool {
	if !claimIsFresh(order.ClaimedAt, time.Now()) {
		return false
	}
	if actor.StaffID != nil && order.ClaimedByStaffID != nil && *actor.StaffID == *order.ClaimedByStaffID {
		return true
	}
	// Owner principal: NULL staff id + role "owner".
	if actor.StaffID == nil && order.ClaimedByStaffID == nil && order.ClaimedByRole == "owner" && actor.Role == "owner" {
		return true
	}
	return false
}

// applyClaimableWhere restricts an UPDATE to rows that are unclaimed, idle, or
// held by the actor (self re-claim).
func applyClaimableWhere(q *gorm.DB, idleCutoff time.Time, actor ClaimActor) *gorm.DB {
	if actor.StaffID != nil {
		return q.Where("claimed_at IS NULL OR claimed_at < ? OR claimed_by_staff_id = ?", idleCutoff, *actor.StaffID)
	}
	// Owner: free/idle, or already held by an owner principal.
	return q.Where(
		"claimed_at IS NULL OR claimed_at < ? OR (claimed_by_staff_id IS NULL AND claimed_by_role = ?)",
		idleCutoff, "owner",
	)
}
