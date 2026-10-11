package demo

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// demoGhostTicketGrace is how long an unfinished ticket may keep occupying a
// demo table after its check went terminal.
//
// GetTablesWithStatus deliberately keeps a table occupied while an order is
// still pending or in the kitchen even though the check is closed — expo may
// be plating after a $0 close (#704). That grace is measured in minutes of a
// real service. Six hours is past the end of any dinner, so anything older is
// not "still cooking", it is abandoned.
const demoGhostTicketGrace = 6 * time.Hour

const (
	demoGhostTicketActor  = "demo-floor-selfheal"
	demoGhostTicketReason = "Demo showroom self-heal: ticket left unfinished on a check that was already closed."
)

// retireGhostFloorTickets closes out unfinished tickets whose check is already
// terminal, so the showroom floor stops reading occupied for tables nobody is
// sitting at.
//
// #904: US Core Table 1 showed occupied for NINE days with a pending ticket and
// bills=0, and venue 2 carried a 46-day-old pending ticket plus a 71h in_kitchen
// one. The demo storefront is public, so any visitor who abandons a guest
// checkout — or any QA pass that leaves one behind — strands a ticket that
// occupies a demo table until the next full reseed. The generator cannot stop
// producing those (it does not produce them: guest checkout does), so the
// showroom repairs them instead, on every ensure and every hourly append.
//
// The ownership predicate is the CHECK, not the ticket:
//   - the bill must be terminal (not open/partial) — an unpaid walk-out check is
//     still a live check, and the QA leftover fixtures on the AR venues have
//     exactly that shape, so they and their tables are never touched;
//   - the ticket must have sat untouched past the grace window, so a real close
//     with expo still cooking behaves the way #704 intended;
//   - a ticket on a PAID check is marked delivered, not cancelled: the guest paid
//     for that food, and cancelling it would rewrite the venue's own history.
func (s *Service) retireGhostFloorTickets(ctx context.Context, tx *gorm.DB, businessID uint) error {
	now := s.now().UTC()
	unfinished := append(database.KitchenLiveOrderStatuses(), database.OrderStatusPending)

	type ghostRow struct {
		ID         uint                `gorm:"column:id"`
		BillStatus database.BillStatus `gorm:"column:bill_status"`
	}
	var ghosts []ghostRow
	if err := tx.WithContext(ctx).Model(&database.Order{}).
		Select("orders.id AS id, bills.status AS bill_status").
		Joins("JOIN bills ON bills.id = orders.bill_id").
		Where("orders.business_id = ?", businessID).
		Where("orders.status IN ?", unfinished).
		Where("orders.updated_at < ?", now.Add(-demoGhostTicketGrace)).
		Where("bills.status NOT IN ?", database.ActiveBillStatuses()).
		Scan(&ghosts).Error; err != nil {
		return err
	}
	if len(ghosts) == 0 {
		return nil
	}

	served := make([]uint, 0, len(ghosts))
	abandoned := make([]uint, 0, len(ghosts))
	for _, ghost := range ghosts {
		if ghost.BillStatus == database.BillStatusPaid {
			served = append(served, ghost.ID)
			continue
		}
		abandoned = append(abandoned, ghost.ID)
	}

	if len(served) > 0 {
		if err := tx.WithContext(ctx).Model(&database.Order{}).Where("id IN ?", served).
			Updates(map[string]interface{}{
				"status":     database.OrderStatusOrderDelivered,
				"updated_at": now,
			}).Error; err != nil {
			return err
		}
	}
	if len(abandoned) > 0 {
		if err := tx.WithContext(ctx).Model(&database.Order{}).Where("id IN ?", abandoned).
			Updates(map[string]interface{}{
				"status":        database.OrderStatusOrderCancelled,
				"cancelled_by":  demoGhostTicketActor,
				"cancel_reason": demoGhostTicketReason,
				"cancelled_at":  now,
				"updated_at":    now,
			}).Error; err != nil {
			return err
		}
	}
	return nil
}
