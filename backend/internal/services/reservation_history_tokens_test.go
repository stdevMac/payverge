package services

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func TestBuildReservationTransitionNote_Tokens(t *testing.T) {
	t.Parallel()
	res := &database.TableReservation{Status: "confirmed"}
	got := buildReservationTransitionNote("pending", res, "confirm", ReservationTransitionInput{})
	if got != "token:status_changed:pending:confirmed" {
		t.Fatalf("status change token = %q", got)
	}
	got = buildReservationTransitionNote("waitlist", res, "promote_waitlist", ReservationTransitionInput{})
	if got != reservationNotePromotedWaitlist {
		t.Fatalf("promote token = %q", got)
	}
	res.Table = &database.Table{Name: "Patio A"}
	got = buildReservationTransitionNote("pending", res, "assign_table", ReservationTransitionInput{})
	if got != "token:assigned_to:Patio A" {
		t.Fatalf("assign token = %q", got)
	}
}

func TestInitialReservationHistoryNote_Tokens(t *testing.T) {
	t.Parallel()
	if got := initialReservationHistoryNote(&database.TableReservation{Status: "waitlist"}); got != reservationNoteAddedWaitlist {
		t.Fatalf("waitlist = %q", got)
	}
	tid := uint(3)
	if got := initialReservationHistoryNote(&database.TableReservation{TableID: &tid}); got != reservationNoteCreatedWithTable {
		t.Fatalf("with table = %q", got)
	}
	if got := initialReservationHistoryNote(&database.TableReservation{}); got != reservationNoteCreated {
		t.Fatalf("created = %q", got)
	}
}

func TestCancelledByCustomerTokenConstant(t *testing.T) {
	t.Parallel()
	if reservationNoteCancelledByCustomer != "token:cancelled_by_customer" {
		t.Fatalf("unexpected cancelled token %q", reservationNoteCancelledByCustomer)
	}
	if reservationNoteAutoDeclinedNoResponse != "token:auto_declined_no_response" {
		t.Fatalf("unexpected auto-decline token %q", reservationNoteAutoDeclinedNoResponse)
	}
}

// The business-cancel reason is a machine code, so its history note must be a
// token the dashboard localizes, not the bare code. Free-text reasons stay.
func TestBuildReservationTransitionNote_BusinessCancelReasonIsToken(t *testing.T) {
	t.Parallel()
	res := &database.TableReservation{Status: "cancelled"}
	got := buildReservationTransitionNote("confirmed", res, "cancel", ReservationTransitionInput{Reason: database.ReservationReasonCancelledByBusiness})
	if got != "token:cancelled_by_business" {
		t.Fatalf("business cancel note = %q", got)
	}
	got = buildReservationTransitionNote("confirmed", res, "cancel", ReservationTransitionInput{Reason: "Kitchen fire"})
	if got != "Kitchen fire" {
		t.Fatalf("free-text reason = %q", got)
	}
}
