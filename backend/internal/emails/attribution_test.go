package emails

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// attributionTestDB adds the CRM tables the attribution projection writes to.
// They are created with explicit DDL rather than through the GORM migrator so
// the fixture does not drag the whole Business schema into sqlite.
func attributionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := suppressionTestDB(t)
	stmts := []string{
		`CREATE TABLE customers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT,
			name TEXT
		)`,
		`CREATE TABLE customer_businesses (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			customer_id INTEGER,
			business_id INTEGER
		)`,
		`CREATE TABLE customer_communications (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			business_id INTEGER NOT NULL,
			customer_business_id INTEGER NOT NULL,
			type TEXT NOT NULL,
			status TEXT,
			subject TEXT,
			content TEXT,
			scheduled_for DATETIME,
			sent_at DATETIME,
			delivered_at DATETIME,
			opened_at DATETIME,
			clicked_at DATETIME,
			error_message TEXT,
			campaign_id TEXT,
			created_at DATETIME,
			updated_at DATETIME
		)`,
	}
	for _, stmt := range stmts {
		require.NoError(t, db.Exec(stmt).Error)
	}
	return db
}

func seedKnownCustomer(t *testing.T, db *gorm.DB, businessID uint, email string) uint {
	t.Helper()
	require.NoError(t, db.Exec(`INSERT INTO customers (email, name) VALUES (?, ?)`, email, "Known Guest").Error)
	var customerID uint
	require.NoError(t, db.Raw(`SELECT id FROM customers WHERE email = ?`, email).Scan(&customerID).Error)
	require.NoError(t, db.Exec(`INSERT INTO customer_businesses (customer_id, business_id) VALUES (?, ?)`,
		customerID, businessID).Error)
	var linkID uint
	require.NoError(t, db.Raw(`SELECT id FROM customer_businesses WHERE customer_id = ? AND business_id = ?`,
		customerID, businessID).Scan(&linkID).Error)
	return linkID
}

// seedSentReservationEmail queues a reservation email through the real dispatch
// path and drives the worker, so the row under test is produced exactly the way
// production produces it.
func seedSentReservationEmail(t *testing.T, db *gorm.DB, businessID, reservationID uint, recipient string) EmailOutbox {
	t.Helper()
	provider := &outboxProviderStub{}
	server, store := newOutboxTestServer(t, db, provider)

	require.NoError(t, server.ForReservation(businessID, reservationID).SendReservationCancelledEmail(
		[]string{recipient}, "Known Guest", "Test Bistro", "Friday 20 June", "20:00", "+541100000000", "en"))

	worker := NewOutboxWorker(store, server)
	processed, err := worker.ProcessDue(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, processed)

	var row EmailOutbox
	require.NoError(t, db.Where("status = ?", OutboxStatusSent).First(&row).Error)
	require.NotEmpty(t, row.ProviderMessageID)
	return row
}

func TestOutboxCapturesOriginAtEnqueueTime(t *testing.T) {
	db := outboxTestDB(t)
	provider := &outboxProviderStub{}
	server, _ := newOutboxTestServer(t, db, provider)

	require.NoError(t, server.ForReservation(7, 4211).SendReservationCancelledEmail(
		[]string{"guest@example.test"}, "Guest", "Test Bistro", "Friday", "20:00", "+5411", "en"))
	// The shared singleton must not be mutated by a scoped copy.
	require.NoError(t, server.SendPasswordResetEmail([]string{"guest@example.test"}, "https://payverge.io/reset", "en"))

	var rows []EmailOutbox
	require.NoError(t, db.Order("id ASC").Find(&rows).Error)
	require.Len(t, rows, 2)

	require.NotNil(t, rows[0].BusinessID)
	require.EqualValues(t, 7, *rows[0].BusinessID)
	require.NotNil(t, rows[0].ReservationID)
	require.EqualValues(t, 4211, *rows[0].ReservationID)
	require.Equal(t, "reservation", rows[0].Origin().entityLabel())

	require.Nil(t, rows[1].BusinessID, "WithOrigin must not leak onto the shared server")
	require.Nil(t, rows[1].ReservationID)
	require.Equal(t, "unattributed", rows[1].Origin().entityLabel())
}

func TestBounceAttachesToOriginatingReservation(t *testing.T) {
	db := attributionTestDB(t)
	linkID := seedKnownCustomer(t, db, 7, "guest@example.test")
	row := seedSentReservationEmail(t, db, 7, 4211, "guest@example.test")

	now := time.Now().UTC()
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return applyOutboxDeliveryEvent(tx, "resend", row.ProviderMessageID,
			"email.bounced", "guest@example.test", "Permanent:General", now)
	}))

	updated := loadOutboxRow(t, db, row.ID)
	require.Equal(t, string(database.CommunicationStatusBounced), updated.DeliveryStatus)
	require.Equal(t, "Permanent:General", updated.DeliveryDetail)
	require.NotNil(t, updated.DeliveryEventAt)
	require.NotNil(t, updated.ReservationID)
	require.EqualValues(t, 4211, *updated.ReservationID)
	require.NotNil(t, updated.CustomerCommunicationID)

	var comm database.CustomerCommunication
	require.NoError(t, db.First(&comm, *updated.CustomerCommunicationID).Error)
	require.Equal(t, database.CommunicationStatusBounced, comm.Status,
		"CommunicationStatusBounced must finally have a real writer")
	require.EqualValues(t, 7, comm.BusinessID)
	require.EqualValues(t, linkID, comm.CustomerBusinessID)
	require.Equal(t, database.CommunicationTypeEmail, comm.Type)
	require.Equal(t, "Permanent:General", comm.ErrorMessage)
	require.NotEmpty(t, comm.Content, "the CRM row records which template failed")
}

func TestDeliveredThenComplainedReusesTheSameCommunicationRow(t *testing.T) {
	db := attributionTestDB(t)
	seedKnownCustomer(t, db, 7, "guest@example.test")
	row := seedSentReservationEmail(t, db, 7, 4211, "guest@example.test")

	now := time.Now().UTC()
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return applyOutboxDeliveryEvent(tx, "resend", row.ProviderMessageID,
			"email.delivered", "guest@example.test", "", now)
	}))

	delivered := loadOutboxRow(t, db, row.ID)
	require.Equal(t, string(database.CommunicationStatusDelivered), delivered.DeliveryStatus)
	require.NotNil(t, delivered.CustomerCommunicationID)
	firstID := *delivered.CustomerCommunicationID

	var comm database.CustomerCommunication
	require.NoError(t, db.First(&comm, firstID).Error)
	require.Equal(t, database.CommunicationStatusDelivered, comm.Status)
	require.NotNil(t, comm.DeliveredAt)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return applyOutboxDeliveryEvent(tx, "resend", row.ProviderMessageID,
			"email.bounced", "guest@example.test", "Permanent:Suppressed", now.Add(time.Minute))
	}))

	var count int64
	require.NoError(t, db.Model(&database.CustomerCommunication{}).Count(&count).Error)
	require.EqualValues(t, 1, count, "a second event must update the row, not duplicate the log")

	require.NoError(t, db.First(&comm, firstID).Error)
	require.Equal(t, database.CommunicationStatusBounced, comm.Status)
	require.Equal(t, "Permanent:Suppressed", comm.ErrorMessage)
}

func TestUnknownProviderMessageIDIsProcessedSafely(t *testing.T) {
	db := attributionTestDB(t)
	seedKnownCustomer(t, db, 7, "guest@example.test")
	row := seedSentReservationEmail(t, db, 7, 4211, "guest@example.test")

	now := time.Now().UTC()
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		// Mail sent before the outbox shipped, or by another sender on the domain.
		return applyOutboxDeliveryEvent(tx, "resend", "re_never_seen_by_us",
			"email.bounced", "guest@example.test", "Permanent:General", now)
	}))

	untouched := loadOutboxRow(t, db, row.ID)
	require.Empty(t, untouched.DeliveryStatus)
	require.Nil(t, untouched.CustomerCommunicationID)

	var count int64
	require.NoError(t, db.Model(&database.CustomerCommunication{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestBounceForUnregisteredGuestStillAttributesToTheEntity(t *testing.T) {
	db := attributionTestDB(t)
	// No customers row: the usual case for a guest reservation booked with a
	// bare email address.
	row := seedSentReservationEmail(t, db, 7, 4211, "walkin@example.test")

	now := time.Now().UTC()
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return applyOutboxDeliveryEvent(tx, "resend", row.ProviderMessageID,
			"email.bounced", "walkin@example.test", "Permanent:General", now)
	}))

	updated := loadOutboxRow(t, db, row.ID)
	require.Equal(t, string(database.CommunicationStatusBounced), updated.DeliveryStatus,
		"the entity-level attribution must not depend on the guest having an account")
	require.NotNil(t, updated.ReservationID)
	require.EqualValues(t, 4211, *updated.ReservationID)
	require.Nil(t, updated.CustomerCommunicationID)

	var count int64
	require.NoError(t, db.Model(&database.CustomerCommunication{}).Count(&count).Error)
	require.Zero(t, count, "there is no CRM row to hang the communication off")
}

func TestResendWebhookAttributesBounceEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := attributionTestDB(t)
	seedKnownCustomer(t, db, 7, "guest@example.test")
	row := seedSentReservationEmail(t, db, 7, 4211, "guest@example.test")

	secret := "whsec_" + base64.StdEncoding.EncodeToString([]byte("webhook-secret"))
	handler := NewResendWebhookHandler(db, secret)

	payload, err := json.Marshal(map[string]interface{}{
		"type":       "email.bounced",
		"created_at": time.Now().UTC().Format(time.RFC3339Nano),
		"data": map[string]interface{}{
			"email_id": row.ProviderMessageID,
			"to":       []string{"guest@example.test"},
			"bounce":   map[string]interface{}{"type": "Permanent", "subType": "General"},
		},
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = signedResendWebhookRequest(t, secret, "evt-attributed-bounce", payload)
	handler.Handle(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	updated := loadOutboxRow(t, db, row.ID)
	require.Equal(t, string(database.CommunicationStatusBounced), updated.DeliveryStatus)
	require.Equal(t, "Permanent:General", updated.DeliveryDetail)
	require.NotNil(t, updated.CustomerCommunicationID)

	// The pre-existing bookkeeping still happens: suppression + delivery event.
	var suppressed int64
	require.NoError(t, db.Model(&EmailSuppression{}).
		Where("email = ?", "guest@example.test").Count(&suppressed).Error)
	require.EqualValues(t, 1, suppressed)

	var events int64
	require.NoError(t, db.Model(&EmailDeliveryEvent{}).Count(&events).Error)
	require.EqualValues(t, 1, events)
}

func TestResendWebhookForUnqueuedMailStillSucceeds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := attributionTestDB(t)

	secret := "whsec_" + base64.StdEncoding.EncodeToString([]byte("webhook-secret"))
	handler := NewResendWebhookHandler(db, secret)

	payload, err := json.Marshal(map[string]interface{}{
		"type":       "email.bounced",
		"created_at": time.Now().UTC().Format(time.RFC3339Nano),
		"data": map[string]interface{}{
			"email_id": "re_from_another_system",
			"to":       []string{"stranger@example.test"},
			"bounce":   map[string]interface{}{"type": "Permanent"},
		},
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = signedResendWebhookRequest(t, secret, "evt-unknown-id", payload)
	handler.Handle(ctx)
	require.Equal(t, http.StatusOK, recorder.Code)

	var suppressed int64
	require.NoError(t, db.Model(&EmailSuppression{}).
		Where("email = ?", "stranger@example.test").Count(&suppressed).Error)
	require.EqualValues(t, 1, suppressed, "suppression must not depend on attribution")
}
