package services

import (
	"errors"
	"fmt"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"gorm.io/gorm"
)

// reservationClaimIdleTTL matches delivery/AI claim idle (5 minutes).
const reservationClaimIdleTTL = 5 * time.Minute

// ReservationClaimHeldError is returned when another operator holds a fresh claim.
type ReservationClaimHeldError struct {
	ClaimedByStaffID *uint
	ClaimedByName    string
	ClaimedByRole    string
}

func (e *ReservationClaimHeldError) Error() string {
	if e.ClaimedByName != "" {
		return fmt.Sprintf("reservation claimed by %s", e.ClaimedByName)
	}
	return "reservation claimed by another operator"
}

// ErrReservationClaimStealRequired is returned when a steal-capable actor tries
// to take a held claim without the explicit steal flag.
var ErrReservationClaimStealRequired = errors.New("explicit steal required to take this claim")

// ErrReservationClaimForbidden is returned when a front-line actor cannot steal.
var ErrReservationClaimForbidden = errors.New("cannot take a claim held by another operator")

// ClaimReservation atomically assigns the reservation to the acting operator.
// Steal=true with CanSteal records claim_stolen audit and returns previous holder.
func (s *ReservationService) ClaimReservation(
	businessID, reservationID uint,
	actor ClaimActor,
	steal bool,
) (reservation *database.TableReservation, previousStaffID *uint, err error) {
	now := s.now()
	idleCutoff := now.Add(-reservationClaimIdleTTL)

	var prevStaffID *uint
	var prevName, prevRole string
	var prevFresh bool

	err = s.db.Transaction(func(tx *gorm.DB) error {
		var current database.TableReservation
		if err := tx.Where("id = ? AND business_id = ?", reservationID, businessID).
			Select("id", "business_id", "claimed_by_staff_id", "claimed_by_name", "claimed_by_role", "claimed_at").
			First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("reservation not found")
			}
			return err
		}

		prevFresh = reservationClaimIsFresh(current.ClaimedAt, now)
		if prevFresh {
			prevStaffID = current.ClaimedByStaffID
			prevName = current.ClaimedByName
			prevRole = current.ClaimedByRole
		}

		if actorHoldsReservationClaim(current, actor) {
			prevStaffID, prevName, prevRole = nil, "", ""
			return tx.Model(&database.TableReservation{}).
				Where("id = ? AND business_id = ?", reservationID, businessID).
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
					return ErrReservationClaimStealRequired
				}
				return &ReservationClaimHeldError{
					ClaimedByStaffID: current.ClaimedByStaffID,
					ClaimedByName:    current.ClaimedByName,
					ClaimedByRole:    current.ClaimedByRole,
				}
			}
			if !actor.CanSteal {
				return ErrReservationClaimForbidden
			}
		}

		q := tx.Model(&database.TableReservation{}).
			Where("id = ? AND business_id = ?", reservationID, businessID)
		if !steal || !actor.CanSteal {
			q = applyReservationClaimableWhere(q, idleCutoff, actor)
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
			var held database.TableReservation
			_ = tx.Select("claimed_by_staff_id", "claimed_by_name", "claimed_by_role").
				Where("id = ?", reservationID).First(&held).Error
			return &ReservationClaimHeldError{
				ClaimedByStaffID: held.ClaimedByStaffID,
				ClaimedByName:    held.ClaimedByName,
				ClaimedByRole:    held.ClaimedByRole,
			}
		}

		if steal && actor.CanSteal && prevFresh && (prevStaffID != nil || prevName != "") {
			staffForAudit := resolveClaimAuditStaffID(actor.StaffID, prevStaffID)
			if staffForAudit != 0 {
				reason := fmt.Sprintf("table_reservation:%d prev=%s (%s) stealer=%s (%s)",
					reservationID, prevName, prevRole, actor.Name, actor.Role)
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
			prevStaffID = nil
			prevName = ""
			prevRole = ""
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	var out database.TableReservation
	if err := s.db.Where("id = ? AND business_id = ?", reservationID, businessID).First(&out).Error; err != nil {
		return nil, nil, err
	}
	if prevStaffID != nil || prevName != "" {
		return &out, prevStaffID, nil
	}
	return &out, nil, nil
}

// ReleaseReservation clears the claim. Front-line may release only their own.
// CanSteal actors may force-release another's fresh claim only when selfOnly is
// false (explicit). Automatic post-mutate release must pass selfOnly=true so a
// mid-flight steal by another CanSteal actor is not undone with a phantom
// claim_force_released audit (L4-8).
func (s *ReservationService) ReleaseReservation(businessID, reservationID uint, actor ClaimActor, selfOnly bool) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var current database.TableReservation
		if err := tx.Where("id = ? AND business_id = ?", reservationID, businessID).
			Select("id", "claimed_by_staff_id", "claimed_by_name", "claimed_by_role", "claimed_at").
			First(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("reservation not found")
			}
			return err
		}

		holds := actorHoldsReservationClaim(current, actor)
		fresh := reservationClaimIsFresh(current.ClaimedAt, s.now())
		hasHolder := current.ClaimedByStaffID != nil || current.ClaimedByName != ""

		if holds {
			// Self-release of our own claim — clear below.
		} else if !fresh || !hasHolder {
			// Unclaimed or idle: clear leftover columns if any, no force audit.
		} else if selfOnly {
			// Auto-release after mutate: never force-clear a claim we no longer hold.
			return nil
		} else if !actor.CanSteal {
			return &ReservationClaimHeldError{
				ClaimedByStaffID: current.ClaimedByStaffID,
				ClaimedByName:    current.ClaimedByName,
				ClaimedByRole:    current.ClaimedByRole,
			}
		}

		forceRelease := !selfOnly &&
			actor.CanSteal &&
			!holds &&
			fresh &&
			hasHolder

		if err := tx.Model(&database.TableReservation{}).
			Where("id = ? AND business_id = ?", reservationID, businessID).
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
					"table_reservation:%d prev=%s (%s) releaser=%s (%s)",
					reservationID, current.ClaimedByName, current.ClaimedByRole, actor.Name, actor.Role,
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

// AssertReservationClaimHeld gates mutating operator actions (update/cancel/
// seat/no-show/assign). Unclaimed or idle claims require Claim first.
func (s *ReservationService) AssertReservationClaimHeld(businessID, reservationID uint, actor ClaimActor) error {
	var current database.TableReservation
	if err := s.db.Where("id = ? AND business_id = ?", reservationID, businessID).
		Select("id", "claimed_by_staff_id", "claimed_by_name", "claimed_by_role", "claimed_at").
		First(&current).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("reservation not found")
		}
		return err
	}
	if !reservationClaimIsFresh(current.ClaimedAt, s.now()) {
		return &ReservationClaimHeldError{ClaimedByName: "", ClaimedByRole: ""}
	}
	if !actorHoldsReservationClaim(current, actor) {
		return &ReservationClaimHeldError{
			ClaimedByStaffID: current.ClaimedByStaffID,
			ClaimedByName:    current.ClaimedByName,
			ClaimedByRole:    current.ClaimedByRole,
		}
	}
	return nil
}

func reservationClaimIsFresh(claimedAt *time.Time, now time.Time) bool {
	if claimedAt == nil {
		return false
	}
	return claimedAt.After(now.Add(-reservationClaimIdleTTL))
}

func actorHoldsReservationClaim(r database.TableReservation, actor ClaimActor) bool {
	// Use wall clock for hold check; ReservationService methods use s.now() for
	// claim timestamps. Freshness here is intentionally independent of injectable
	// clock so assert paths without a service still work.
	if !reservationClaimIsFresh(r.ClaimedAt, time.Now()) {
		return false
	}
	if actor.StaffID != nil && r.ClaimedByStaffID != nil && *actor.StaffID == *r.ClaimedByStaffID {
		return true
	}
	if actor.StaffID == nil && r.ClaimedByStaffID == nil && r.ClaimedByRole == "owner" && actor.Role == "owner" {
		return true
	}
	return false
}

func applyReservationClaimableWhere(q *gorm.DB, idleCutoff time.Time, actor ClaimActor) *gorm.DB {
	if actor.StaffID != nil {
		return q.Where("claimed_at IS NULL OR claimed_at < ? OR claimed_by_staff_id = ?", idleCutoff, *actor.StaffID)
	}
	return q.Where(
		"claimed_at IS NULL OR claimed_at < ? OR (claimed_by_staff_id IS NULL AND claimed_by_role = ?)",
		idleCutoff, "owner",
	)
}
