package services

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func setupWebPushFanoutDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:webpush-fanout-%s?mode=memory&cache=shared", t.Name())
	g, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := g.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, g.AutoMigrate(
		&database.User{},
		&database.Business{},
		&database.Staff{},
		&database.PushSubscription{},
	))
	database.SetTestDB(g)
	return g
}

// Fanout must never deliver to a deactivated staff member's subscription, even
// when a stale push_subscriptions row still points at the business_id.
func TestSendLocalizedPushToBusiness_SkipsDeactivatedStaffPrincipal(t *testing.T) {
	g := setupWebPushFanoutDB(t)

	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	vapidPriv, vapidPub, err := webpush.GenerateVAPIDKeys()
	require.NoError(t, err)
	svc := NewWebPushService(g, vapidPub, vapidPriv, "mailto:test@payverge.io")
	svc.validateEndpoint = func(string) error { return nil } // httptest endpoints are plain http

	ownerID := uint(9)
	biz := &database.Business{
		BusinessId:     "fanout-biz",
		Name:           "Fanout Biz",
		OwnerAddress:   "0xowner",
		SettlementAddr: "0x1111111111111111111111111111111111111111",
		TippingAddr:    "0x2222222222222222222222222222222222222222",
		UserID:         &ownerID,
		IsActive:       true,
	}
	require.NoError(t, g.Create(biz).Error)

	active := &database.Staff{
		BusinessID: biz.ID, Email: "active@example.com", Name: "Active",
		Role: database.StaffRoleServer, IsActive: true, InvitedBy: "0xowner", AuthzVersion: 1,
	}
	deact := &database.Staff{
		BusinessID: biz.ID, Email: "deact@example.com", Name: "Deact",
		Role: database.StaffRoleServer, IsActive: true, InvitedBy: "0xowner", AuthzVersion: 2,
	}
	require.NoError(t, g.Create(active).Error)
	require.NoError(t, g.Create(deact).Error)
	// GORM omits bool zero values on Create when default:true — force inactive.
	require.NoError(t, g.Model(deact).Update("is_active", false).Error)

	p256dh, auth := browserSubscriptionKeys(t)
	require.NoError(t, g.Create(&database.PushSubscription{
		UserID: 0, BusinessID: biz.ID,
		PrincipalType: database.PushPrincipalStaff, PrincipalID: active.ID,
		Endpoint: server.URL + "/active", P256dhKey: p256dh, AuthKey: auth,
	}).Error)
	require.NoError(t, g.Create(&database.PushSubscription{
		UserID: 0, BusinessID: biz.ID,
		PrincipalType: database.PushPrincipalStaff, PrincipalID: deact.ID,
		Endpoint: server.URL + "/deact", P256dhKey: p256dh, AuthKey: auth,
	}).Error)
	// Owner principal must still receive fanout.
	require.NoError(t, g.Create(&database.PushSubscription{
		UserID: ownerID, BusinessID: biz.ID,
		PrincipalType: database.PushPrincipalOwnerUser, PrincipalID: ownerID,
		Endpoint: server.URL + "/owner", P256dhKey: p256dh, AuthKey: auth,
	}).Error)

	require.NoError(t, svc.SendLocalizedPushToBusiness(biz.ID, PushKeyNewOrder, PushArgs{}, "/url"))

	// Active staff + owner only — not the deactivated staff.
	assert.Equal(t, int32(2), atomic.LoadInt32(&hits), "must deliver only to active members (active staff + owner)")
}
