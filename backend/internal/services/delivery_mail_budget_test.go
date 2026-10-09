package services

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/structs"
)

// mailOriginsTo returns the MailOrigin of every email notification sent to
// address, in dispatch order.
func mailOriginsTo(mock *mockNotificationDispatcher, address string) []structs.NotificationMailOrigin {
	var origins []structs.NotificationMailOrigin
	for i, user := range mock.users {
		if user.Email == address {
			origins = append(origins, mock.notifications[i].MailOrigin)
		}
	}
	return origins
}

// The order-received confirmation of a public checkout goes to an address an
// anonymous caller typed. It must be tenant mail in the guest lane; before,
// it was unstamped system mail, so the public checkout (30/min per IP per
// venue) was an unbudgeted relay to any address.
func TestGuestDeliveryCheckout_ReceivedMailIsInTheGuestLane(t *testing.T) {
	db := setupHospitalityServiceTestDB(t)
	emailMock := &mockNotificationDispatcher{}
	svc := NewDeliveryService(db, newTestNotificationManager(emailMock))
	business := setupTipValidationBusiness(t, db, "delivery-mail-lane", 12)
	require.NoError(t, db.Model(&database.Business{}).Where("id = ?", business.ID).
		Update("email", "venue-contact@example.test").Error)

	req := tipCheckoutRequest(0)
	result, err := svc.GuestDeliveryCheckout(business.ID, req)
	require.NoError(t, err)
	require.NotNil(t, result.DeliveryOrder)

	require.Equal(t, []structs.NotificationMailOrigin{{
		BusinessID:      business.ID,
		DeliveryOrderID: result.DeliveryOrder.ID,
		Purpose:         emails.MailPurposeGuestDeliveryRequest,
	}}, mailOriginsTo(emailMock, req.CustomerEmail))

	// business.Email is tenant-typed and unverified. With no verified owner,
	// the new-order notice is not sent at all.
	require.Empty(t, mailOriginsTo(emailMock, "venue-contact@example.test"))
}

// Mail a venue action sends to the order address is operator mail for that
// venue: stamped with the business and the delivery order, no lane purpose.
func TestDeliveryCustomerMailIsStampedForTheVenue(t *testing.T) {
	operator := structs.NotificationMailOrigin{BusinessID: 100, DeliveryOrderID: 1}

	cases := []struct {
		name string
		send func(svc *DeliveryService, delivery *database.DeliveryOrder)
	}{
		{"status change", func(svc *DeliveryService, d *database.DeliveryOrder) {
			svc.notifyStatusChange(d, database.DeliveryStatusPickedUp)
		}},
		{"accepted", func(svc *DeliveryService, d *database.DeliveryOrder) {
			svc.notifyDeliveryAccepted(d, database.DeliveryPaymentCashOnDelivery)
		}},
		{"expired", func(svc *DeliveryService, d *database.DeliveryOrder) {
			svc.notifyDeliveryTerminated(d, "expired")
		}},
		{"payment received", func(svc *DeliveryService, d *database.DeliveryOrder) {
			svc.notifyDeliveryPaymentReceived(d)
		}},
		{"cancelled", func(svc *DeliveryService, d *database.DeliveryOrder) {
			svc.notifyCancellation(d)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupDeliveryTestDB(t)
			emailMock := &mockNotificationDispatcher{}
			svc := newDeliveryServiceWithMockDispatchers(emailMock)
			delivery := newTestDeliveryOrder()

			tc.send(svc, delivery)

			require.Equal(t, []structs.NotificationMailOrigin{operator}, mailOriginsTo(emailMock, delivery.CustomerEmail))
			require.Empty(t, mailOriginsTo(emailMock, "restaurant@example.com"), "business.Email is not a venue-notice recipient")
			if tc.name == "cancelled" {
				require.Equal(t, []structs.NotificationMailOrigin{operator}, mailOriginsTo(emailMock, deliveryTestOwnerEmail))
			} else {
				require.Empty(t, mailOriginsTo(emailMock, deliveryTestOwnerEmail))
			}
		})
	}
}
