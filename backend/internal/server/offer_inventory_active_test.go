package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// operatorOfferPayload is the wire shape the Offers tab and the live verifier
// read. is_active is the EFFECTIVE answer (can this offer fire right now);
// manual_active is the operator's stored switch.
type operatorOfferPayload struct {
	ID               uint   `json:"id"`
	Name             string `json:"name"`
	IsActive         bool   `json:"is_active"`
	ManualActive     bool   `json:"manual_active"`
	InventoryBlocked bool   `json:"inventory_blocked"`
}

func (p operatorOfferPayload) String() string {
	return fmt.Sprintf("%s{is_active:%t manual_active:%t inventory_blocked:%t}",
		p.Name, p.IsActive, p.ManualActive, p.InventoryBlocked)
}

func findOfferPayload(t *testing.T, payloads []operatorOfferPayload, name string) operatorOfferPayload {
	t.Helper()
	for _, payload := range payloads {
		if payload.Name == name {
			return payload
		}
	}
	t.Fatalf("offer %q missing from operator payload %v", name, payloads)
	return operatorOfferPayload{}
}

// newParrillaOffersDB rebuilds venue 142 (Parrilla Quebracho Azul): the bife is
// inventory_out because the beef is at 0, the ensalada and the sorrentinos are
// stocked and stay sellable.
func newParrillaOffersDB(t *testing.T) (*gorm.DB, *database.Business) {
	t.Helper()
	db := setupInventoryAvailabilityTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.Offer{}))
	services.ResetPricingCache()

	business := &database.Business{
		BusinessId:      "demo-admin-8-ai-pro",
		Name:            "Parrilla Quebracho Azul",
		OwnerAddress:    "0xowner",
		DefaultLanguage: "es",
		DefaultCurrency: "ARS",
		Timezone:        "America/Buenos_Aires",
	}
	require.NoError(t, db.Create(business).Error)
	require.NoError(t, db.Create(&database.InventorySettings{
		BusinessID:           business.ID,
		InventoryEnabled:     true,
		AvailabilitySyncMode: database.InventoryAvailabilityModeHardBlock,
	}).Error)

	categories := []database.MenuCategory{{
		ID: "principales", Name: "Principales",
		Items: []database.MenuItem{
			{ID: "demo-bife", Name: "Bife de chorizo", Price: 24000, IsAvailable: true},
			{ID: "demo-ensalada", Name: "Ensalada mixta", Price: 6500, IsAvailable: true},
			{ID: "demo-sorrentinos", Name: "Sorrentinos", Price: 12000, IsAvailable: true},
		},
	}}
	raw, err := json.Marshal(categories)
	require.NoError(t, err)
	require.NoError(t, db.Create(&database.Menu{
		BusinessID: business.ID, Categories: string(raw), IsActive: true, Version: 1,
	}).Error)

	beef := seedInventoryItem(t, db, business.ID, "Bife", 0, true)
	greens := seedInventoryItem(t, db, business.ID, "Verduras", 18, true)
	pasta := seedInventoryItem(t, db, business.ID, "Masa", 9, true)
	seedRecipe(t, db, business.ID, "demo-bife", beef.ID, 0.35)
	seedRecipe(t, db, business.ID, "demo-ensalada", greens.ID, 0.2)
	seedRecipe(t, db, business.ID, "demo-sorrentinos", pasta.ID, 0.25)

	return db, business
}

func seedOffer(t *testing.T, db *gorm.DB, businessID uint, name, applicableTo string, targetID *string, isActive bool) *database.Offer {
	t.Helper()
	offer := &database.Offer{
		BusinessID: businessID, Name: name, DiscountType: "fixed", DiscountValue: 3000,
		IsActive: isActive, ApplicableTo: applicableTo, TargetID: targetID, WeekdayMask: 127,
	}
	require.NoError(t, db.Create(offer).Error)
	if !isActive {
		require.NoError(t, db.Exec("UPDATE offers SET is_active = 0 WHERE id = ?", offer.ID).Error)
	}
	return offer
}

func getOperatorOffers(t *testing.T, businessID uint) []operatorOfferPayload {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	path := fmt.Sprintf("/businesses/%d/offers", businessID)
	ctx.Request = httptest.NewRequest(http.MethodGet, path, nil)
	ctx.Params = gin.Params{{Key: "id", Value: fmt.Sprintf("%d", businessID)}}
	GetOffers(ctx)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var payloads []operatorOfferPayload
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &payloads))
	return payloads
}

// #835: live 142 returned "AR$ 3.000 menos en el bife" with is_active:true AND
// inventory_blocked:true while the guest surfaces had already dropped it.
func TestGetOffers_InventoryBlockedOfferIsNotActive(t *testing.T) {
	db, business := newParrillaOffersDB(t)
	bife := "demo-bife"
	ensalada := "demo-ensalada"
	sorrentinos := "demo-sorrentinos"
	seedOffer(t, db, business.ID, "AR$ 3.000 menos en el bife", "item", &bife, true)
	seedOffer(t, db, business.ID, "AR$ 1.000 menos en la ensalada", "item", &ensalada, true)
	seedOffer(t, db, business.ID, "Sorrentinos pausados", "item", &sorrentinos, false)
	seedOffer(t, db, business.ID, "15% en toda la carta", "all", nil, true)

	payloads := getOperatorOffers(t, business.ID)
	require.Len(t, payloads, 4)

	blocked := findOfferPayload(t, payloads, "AR$ 3.000 menos en el bife")
	require.True(t, blocked.InventoryBlocked, "bife offer targets an inventory_out dish")
	require.False(t, blocked.IsActive, "an offer that cannot fire must not report is_active:true (#835)")
	require.True(t, blocked.ManualActive, "the operator never switched this offer off")

	sellable := findOfferPayload(t, payloads, "AR$ 1.000 menos en la ensalada")
	require.False(t, sellable.InventoryBlocked)
	require.True(t, sellable.IsActive, "a stocked target stays active")
	require.True(t, sellable.ManualActive)

	operatorOff := findOfferPayload(t, payloads, "Sorrentinos pausados")
	require.False(t, operatorOff.InventoryBlocked, "sorrentinos are stocked")
	require.False(t, operatorOff.IsActive)
	require.False(t, operatorOff.ManualActive, "manual_active must carry the stored off switch")

	venueWide := findOfferPayload(t, payloads, "15% en toda la carta")
	require.False(t, venueWide.InventoryBlocked)
	require.True(t, venueWide.IsActive)
	require.True(t, venueWide.ManualActive)

	// The stored column is untouched: the block is a serve-time overlay, so the
	// offer fires again on its own once the beef is restocked.
	var stored database.Offer
	require.NoError(t, db.Where("name = ?", "AR$ 3.000 menos en el bife").First(&stored).Error)
	require.True(t, stored.IsActive, "annotation must never persist the block")
}

// A restock must be enough to bring the offer back — nothing to re-enable.
func TestGetOffers_BlockedOfferReturnsAfterRestock(t *testing.T) {
	db, business := newParrillaOffersDB(t)
	bife := "demo-bife"
	seedOffer(t, db, business.ID, "AR$ 3.000 menos en el bife", "item", &bife, true)

	blocked := findOfferPayload(t, getOperatorOffers(t, business.ID), "AR$ 3.000 menos en el bife")
	require.False(t, blocked.IsActive)
	require.True(t, blocked.InventoryBlocked)

	require.NoError(t, db.Model(&database.InventoryItem{}).
		Where("business_id = ? AND name = ?", business.ID, "Bife").
		Update("current_quantity", 12).Error)
	services.ResetPricingCache()

	restocked := findOfferPayload(t, getOperatorOffers(t, business.ID), "AR$ 3.000 menos en el bife")
	require.True(t, restocked.IsActive, "restocking the beef re-arms the offer with no operator action")
	require.False(t, restocked.InventoryBlocked)
	require.True(t, restocked.ManualActive)
}

// Non-item offers never consult the menu, so a venue with no menu at all still
// gets a well-formed manual_active mirror.
func TestAnnotateOffersWithInventory_VenueWideOffersSkipMenuLookup(t *testing.T) {
	views := annotateOffersWithInventory(4242, []database.Offer{
		{ID: 1, Name: "15% en toda la carta", ApplicableTo: "all", IsActive: true},
		{ID: 2, Name: "Postres pausados", ApplicableTo: "category", TargetID: strptr("postres"), IsActive: false},
	})
	require.Len(t, views, 2)
	require.True(t, views[0].IsActive)
	require.True(t, views[0].ManualActive)
	require.False(t, views[0].InventoryBlocked)
	require.False(t, views[1].IsActive)
	require.False(t, views[1].ManualActive)
	require.False(t, views[1].InventoryBlocked)
}

// The create/update echoes must agree with the list a second later, or a client
// that trusts the response body paints Active until the next refetch.
func TestUpdateOffer_EchoesEffectiveActiveForBlockedTarget(t *testing.T) {
	db, business := newParrillaOffersDB(t)
	bife := "demo-bife"
	offer := seedOffer(t, db, business.ID, "AR$ 3.000 menos en el bife", "item", &bife, true)

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	body := `{"name":"AR$ 3.000 menos en el bife","discount_type":"fixed","discount_value":3000,` +
		`"applicable_to":"item","target_id":"demo-bife","is_active":true}`
	path := fmt.Sprintf("/businesses/%d/offers/%d", business.ID, offer.ID)
	ctx.Request = httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{
		{Key: "id", Value: fmt.Sprintf("%d", business.ID)},
		{Key: "offerId", Value: fmt.Sprintf("%d", offer.ID)},
	}
	UpdateOffer(ctx)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var echoed operatorOfferPayload
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &echoed))
	require.False(t, echoed.IsActive, "the update echo must not paint Active on a blocked offer (#835)")
	require.True(t, echoed.InventoryBlocked)
	require.True(t, echoed.ManualActive, "the operator asked for on, and that is what was stored")

	var stored database.Offer
	require.NoError(t, db.First(&stored, offer.ID).Error)
	require.True(t, stored.IsActive, "the stored switch keeps the operator's answer")
}
