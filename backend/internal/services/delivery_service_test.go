package services

import (
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/notifications"
	"github.com/stdevmac/payverge/backend/internal/structs"
	"regexp"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// mockNotificationDispatcher implements structs.NotificationDispatcher for testing
type mockNotificationDispatcher struct {
	notifications []structs.Notification
	users         []structs.User
}

func (m *mockNotificationDispatcher) DispatchNotification(notification structs.Notification, user structs.User, carId string) {
	m.notifications = append(m.notifications, notification)
	m.users = append(m.users, user)
}

// deliveryTestOwnerEmail is the verified owner address venue notices use.
// restaurant@example.com stays on business.Email and must not receive them.
const deliveryTestOwnerEmail = "owner@venue.example"

func setupDeliveryTestDB(t *testing.T) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := gormDB.AutoMigrate(&database.User{}, &database.Business{}, &database.DeliveryOrder{}, &database.DeliveryDriver{}, &database.DeliveryStatusHistory{}); err != nil {
		t.Fatalf("failed to migrate test db: %v", err)
	}
	database.SetTestDB(gormDB)

	owner := database.User{Email: deliveryTestOwnerEmail, Name: "Venue Owner", EmailVerified: true}
	if err := gormDB.Create(&owner).Error; err != nil {
		t.Fatalf("failed to create owner: %v", err)
	}
	// Create the test business with ID 100 to match newTestDeliveryOrder().BusinessID.
	// business.Email is an unverified contact field and is not the notice recipient.
	biz := database.Business{
		Name:      "Test Restaurant",
		Email:     "restaurant@example.com",
		OwnerName: "Venue Owner",
		UserID:    &owner.ID,
	}
	biz.ID = 100
	if err := gormDB.Create(&biz).Error; err != nil {
		t.Fatalf("failed to create business: %v", err)
	}
}

func newTestDeliveryOrder() *database.DeliveryOrder {
	driverID := uint(10)
	driver := &database.DeliveryDriver{
		ID:    10,
		Name:  "Jane Driver",
		Phone: "+0987654321",
		Email: "jane@example.com",
	}
	return &database.DeliveryOrder{
		ID:             1,
		BusinessID:     100,
		DeliveryNumber: "DEL-123",
		CustomerName:   "John Doe",
		CustomerPhone:  "+1234567890",
		CustomerEmail:  "john@example.com",
		Status:         database.DeliveryStatusPending,
		DeliveryAddress: database.DeliveryAddress{
			Street:           "123 Main St",
			City:             "Springfield",
			State:            "IL",
			PostalCode:       "62701",
			FormattedAddress: "123 Main St, Springfield, IL 62701",
		},
		Business: database.Business{
			Name:  "Test Restaurant",
			Email: "restaurant@example.com",
		},
		DriverID: &driverID,
		Driver:   driver,
	}
}

func newTestDriver() *database.DeliveryDriver {
	return &database.DeliveryDriver{
		ID:    10,
		Name:  "Jane Driver",
		Phone: "+0987654321",
		Email: "jane@example.com",
	}
}

func newDeliveryServiceWithMockDispatchers(emailDispatcher *mockNotificationDispatcher) *DeliveryService {
	manager := notifications.NewNotificationManager(emailDispatcher)
	return &DeliveryService{
		db:                  nil,
		notificationManager: manager,
	}
}

// newTestNotificationManager returns a NotificationManager backed by the given
// email mock, for tests that need a real DeliveryService (with a DB) and
// notification capture.
func newTestNotificationManager(emailDispatcher *mockNotificationDispatcher) *notifications.NotificationManager {
	return notifications.NewNotificationManager(emailDispatcher)
}

func TestNotifyBusinessNewDelivery_NilManager(t *testing.T) {
	setupDeliveryTestDB(t)
	svc := &DeliveryService{
		db:                  nil,
		notificationManager: nil,
	}
	delivery := newTestDeliveryOrder()

	// Should not panic with nil notificationManager
	svc.notifyBusinessNewDelivery(delivery)
}

func TestGenerateDeliveryNumber_UsesOpaqueToken(t *testing.T) {
	svc := &DeliveryService{}

	first := svc.generateDeliveryNumber()
	second := svc.generateDeliveryNumber()

	assertMatch := func(value string) {
		t.Helper()
		matched, err := regexp.MatchString(`^DEL-[A-Z0-9]{16}$`, value)
		if err != nil {
			t.Fatalf("failed to compile delivery number regex: %v", err)
		}
		if !matched {
			t.Fatalf("expected opaque delivery number, got %q", value)
		}
	}

	assertMatch(first)
	assertMatch(second)
	if first == second {
		t.Fatalf("expected unique delivery numbers, got %q twice", first)
	}
}

// TestGenerateDeliveryNumber_BulkUnique generates 10,000 numbers and asserts no duplicates
// and all match the expected pattern. This pins the crypto/rand contract.
func TestGenerateDeliveryNumber_BulkUnique(t *testing.T) {
	svc := &DeliveryService{}
	seen := make(map[string]struct{}, 10_000)
	pattern := regexp.MustCompile(`^DEL-[A-Z0-9]{16}$`)

	for i := 0; i < 10_000; i++ {
		n := svc.generateDeliveryNumber()
		if !pattern.MatchString(n) {
			t.Fatalf("iteration %d: delivery number %q does not match expected pattern", i, n)
		}
		if _, dup := seen[n]; dup {
			t.Fatalf("iteration %d: duplicate delivery number %q", i, n)
		}
		seen[n] = struct{}{}
	}
}

func TestNotifyBusinessNewDelivery_WithManager(t *testing.T) {
	setupDeliveryTestDB(t)
	emailDispatcher := &mockNotificationDispatcher{}

	svc := newDeliveryServiceWithMockDispatchers(emailDispatcher)
	delivery := newTestDeliveryOrder()

	svc.notifyBusinessNewDelivery(delivery)

	if len(emailDispatcher.users) != 1 {
		t.Fatalf("expected 1 venue notice, got %d", len(emailDispatcher.users))
	}
	if emailDispatcher.users[0].Email != deliveryTestOwnerEmail {
		t.Fatalf("venue notice recipient = %q, want verified owner", emailDispatcher.users[0].Email)
	}
	origin := emailDispatcher.notifications[0].MailOrigin
	if origin.BusinessID != delivery.BusinessID || origin.DeliveryOrderID != delivery.ID {
		t.Fatalf("venue notice origin = %+v, want business %d delivery %d", origin, delivery.BusinessID, delivery.ID)
	}
}

func TestNotifyStatusChange_NilManager(t *testing.T) {
	setupDeliveryTestDB(t)
	svc := &DeliveryService{
		db:                  nil,
		notificationManager: nil,
	}
	delivery := newTestDeliveryOrder()

	statuses := []database.DeliveryStatus{
		database.DeliveryStatusConfirmed,
		database.DeliveryStatusPreparing,
		database.DeliveryStatusPickedUp,
		database.DeliveryStatusInTransit,
		database.DeliveryStatusDelivered,
		database.DeliveryStatusCancelled,
	}

	for _, status := range statuses {
		svc.notifyStatusChange(delivery, status)
	}
}

func TestNotifyStatusChange_WithManager(t *testing.T) {
	setupDeliveryTestDB(t)
	emailDispatcher := &mockNotificationDispatcher{}

	svc := newDeliveryServiceWithMockDispatchers(emailDispatcher)
	delivery := newTestDeliveryOrder()

	// picked_up is a guest-visible transition — must fire.
	svc.notifyStatusChange(delivery, database.DeliveryStatusPickedUp)

	totalSent := len(emailDispatcher.notifications)
	if totalSent == 0 {
		t.Error("expected at least one notification to be dispatched for picked_up status change")
	}
}

func TestNotifyStatusChange_MultipleStatuses(t *testing.T) {
	setupDeliveryTestDB(t)
	// Only the guest-visible statuses should fire a notification.
	guestVisible := []database.DeliveryStatus{
		database.DeliveryStatusAssigned,
		database.DeliveryStatusPickedUp,
		database.DeliveryStatusNearby,
		database.DeliveryStatusDelivered,
		database.DeliveryStatusCancelled,
	}

	for _, status := range guestVisible {
		emailDispatcher := &mockNotificationDispatcher{}
		svc := newDeliveryServiceWithMockDispatchers(emailDispatcher)
		delivery := newTestDeliveryOrder()

		svc.notifyStatusChange(delivery, status)

		totalSent := len(emailDispatcher.notifications)
		if totalSent == 0 {
			t.Errorf("expected notification for guest-visible status %s, got none", status)
		}
	}
}

func TestNotifyDriverAssignment_NilManager(t *testing.T) {
	setupDeliveryTestDB(t)
	svc := &DeliveryService{
		db:                  nil,
		notificationManager: nil,
	}
	delivery := newTestDeliveryOrder()
	driver := newTestDriver()

	svc.notifyDriverAssignment(delivery, driver)
}

func TestNotifyDriverAssignment_WithManager(t *testing.T) {
	setupDeliveryTestDB(t)
	emailDispatcher := &mockNotificationDispatcher{}

	svc := newDeliveryServiceWithMockDispatchers(emailDispatcher)
	delivery := newTestDeliveryOrder()
	driver := newTestDriver()

	svc.notifyDriverAssignment(delivery, driver)

	// Driver-assignment is operator-facing only; the customer-side email is
	// dispatched via notifyStatusChange(assigned) at the call site to avoid
	// double-firing. The recipient is the verified owner, not business.Email.
	if len(emailDispatcher.users) != 1 {
		t.Fatalf("expected exactly 1 ops-side notification, got %d", len(emailDispatcher.users))
	}
	if emailDispatcher.users[0].Email != deliveryTestOwnerEmail {
		t.Fatalf("assignment notice recipient = %q, want verified owner", emailDispatcher.users[0].Email)
	}
	origin := emailDispatcher.notifications[0].MailOrigin
	if origin.BusinessID != delivery.BusinessID || origin.DeliveryOrderID != delivery.ID {
		t.Fatalf("assignment notice origin = %+v", origin)
	}
}

func TestNotifyCancellation_NilManager(t *testing.T) {
	setupDeliveryTestDB(t)
	svc := &DeliveryService{
		db:                  nil,
		notificationManager: nil,
	}
	delivery := newTestDeliveryOrder()

	svc.notifyCancellation(delivery)
}

func TestNotifyCancellation_WithManager(t *testing.T) {
	setupDeliveryTestDB(t)
	emailDispatcher := &mockNotificationDispatcher{}

	svc := newDeliveryServiceWithMockDispatchers(emailDispatcher)
	delivery := newTestDeliveryOrder()
	delivery.CancellationReason = "Customer requested"

	svc.notifyCancellation(delivery)

	// Customer plus the verified owner. business.Email is not a recipient,
	// and the owner notice is stamped for the tenant mail budget.
	if len(emailDispatcher.users) != 2 {
		t.Fatalf("expected customer and owner notices, got %d", len(emailDispatcher.users))
	}
	seen := map[string]structs.NotificationMailOrigin{}
	for i, user := range emailDispatcher.users {
		seen[user.Email] = emailDispatcher.notifications[i].MailOrigin
	}
	if _, ok := seen[delivery.CustomerEmail]; !ok {
		t.Fatal("customer cancellation notice missing")
	}
	ownerOrigin, ok := seen[deliveryTestOwnerEmail]
	if !ok {
		t.Fatal("owner cancellation notice missing")
	}
	if ownerOrigin.BusinessID != delivery.BusinessID || ownerOrigin.DeliveryOrderID != delivery.ID {
		t.Fatalf("owner cancellation origin = %+v", ownerOrigin)
	}
	if _, ok := seen["restaurant@example.com"]; ok {
		t.Fatal("business.Email must not receive venue notices")
	}
}

func TestNotifyCancellation_NoDriver(t *testing.T) {
	setupDeliveryTestDB(t)
	emailDispatcher := &mockNotificationDispatcher{}

	svc := newDeliveryServiceWithMockDispatchers(emailDispatcher)
	delivery := newTestDeliveryOrder()
	delivery.DriverID = nil
	delivery.CancellationReason = "Out of stock"

	svc.notifyCancellation(delivery)

	// No driver: customer plus the verified owner. business.Email is not a recipient.
	if len(emailDispatcher.users) != 2 {
		t.Fatalf("expected customer and owner notices, got %d", len(emailDispatcher.users))
	}
	seen := map[string]bool{}
	for _, user := range emailDispatcher.users {
		seen[user.Email] = true
	}
	if !seen[delivery.CustomerEmail] || !seen[deliveryTestOwnerEmail] {
		t.Fatalf("notices went to %v, want customer and verified owner", seen)
	}
	if seen["restaurant@example.com"] {
		t.Fatal("business.Email must not receive venue notices")
	}
}

// TestGuestVisibleMessage pins which transitions produce customer-facing notifications.
func TestGuestVisibleMessage_GuestVisibleTransitions(t *testing.T) {
	guestVisible := []database.DeliveryStatus{
		database.DeliveryStatusAssigned,
		database.DeliveryStatusPickedUp,
		database.DeliveryStatusNearby,
		database.DeliveryStatusDelivered,
		database.DeliveryStatusCancelled,
	}
	for _, status := range guestVisible {
		title, body, visible := localizedGuestMessage(status, "", LocaleEN)
		if !visible {
			t.Errorf("status %s should be guest-visible but got visible=false", status)
		}
		if title == "" || body == "" {
			t.Errorf("status %s: expected non-empty title and body, got title=%q body=%q", status, title, body)
		}
	}
}

func TestGuestVisibleMessage_OperationalTransitions(t *testing.T) {
	operational := []database.DeliveryStatus{
		database.DeliveryStatusPending,
		database.DeliveryStatusConfirmed,
		database.DeliveryStatusPreparing,
		database.DeliveryStatusReady,
		database.DeliveryStatusInTransit,
	}
	for _, status := range operational {
		_, _, visible := localizedGuestMessage(status, "", LocaleEN)
		if visible {
			t.Errorf("status %s should NOT be guest-visible but got visible=true", status)
		}
	}
}

func TestGuestVisibleMessage_CancelledWithReason(t *testing.T) {
	_, body, visible := localizedGuestMessage(database.DeliveryStatusCancelled, "out of stock", LocaleEN)
	if !visible {
		t.Fatal("cancelled should be guest-visible")
	}
	if !strings.Contains(body, "out of stock") {
		t.Errorf("cancellation body should contain reason, got: %q", body)
	}
}

func TestNotifyStatusChange_OnlyFiresForGuestVisibleStatuses(t *testing.T) {
	setupDeliveryTestDB(t)
	type testCase struct {
		status       database.DeliveryStatus
		expectNotify bool
	}
	cases := []testCase{
		{database.DeliveryStatusAssigned, true},
		{database.DeliveryStatusPickedUp, true},
		{database.DeliveryStatusNearby, true},
		{database.DeliveryStatusDelivered, true},
		{database.DeliveryStatusCancelled, true},
		// operational — must NOT fire
		{database.DeliveryStatusPending, false},
		{database.DeliveryStatusConfirmed, false},
		{database.DeliveryStatusPreparing, false},
		{database.DeliveryStatusReady, false},
		{database.DeliveryStatusInTransit, false},
	}
	for _, tc := range cases {
		emailDispatcher := &mockNotificationDispatcher{}
		svc := newDeliveryServiceWithMockDispatchers(emailDispatcher)
		delivery := newTestDeliveryOrder() // has CustomerEmail set

		svc.notifyStatusChange(delivery, tc.status)

		total := len(emailDispatcher.notifications)
		if tc.expectNotify && total == 0 {
			t.Errorf("status %s: expected notification but none sent", tc.status)
		}
		if !tc.expectNotify && total > 0 {
			t.Errorf("status %s: expected NO notification but %d sent", tc.status, total)
		}
	}
}

// TestSendDeliveryNotificationToEmail_SetsLanguageSelected asserts that the
// locale is threaded through to User.LanguageSelected so the email dispatcher
// selects the correct template chrome. Without the fix, LanguageSelected was
// always empty, locking every delivery email into the English chrome regardless
// of the resolved guest locale.
func TestSendDeliveryNotificationToEmail_SetsLanguageSelected(t *testing.T) {
	for _, tc := range []struct {
		locale NotificationLocale
		want   string
	}{
		{LocaleES, "es"},
		{LocaleEN, "en"},
	} {
		emailDispatcher := &mockNotificationDispatcher{}
		svc := newDeliveryServiceWithMockDispatchers(emailDispatcher)

		svc.sendDeliveryNotificationToEmail(structs.NotificationMailOrigin{}, "guest@example.com", "Guest", "title", "body", tc.locale)

		if len(emailDispatcher.users) == 0 {
			t.Fatalf("locale %s: no notification dispatched", tc.locale)
		}
		got := emailDispatcher.users[0].LanguageSelected
		if got != tc.want {
			t.Errorf("locale %s: LanguageSelected = %q, want %q", tc.locale, got, tc.want)
		}
	}
}
