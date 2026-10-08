package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupCashRegisterHandlerDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsnName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	gormDB, err := gorm.Open(sqlite.Open("file:"+dsnName+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })

	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.User{},
		&database.Staff{},
		&database.Bill{},
		&database.AlternativePayment{},
		&database.CashRegisterSession{},
		&database.CashRegisterMovement{},
	))
	server.InitializeRBAC(database.GetDBWrapper())
	return gormDB
}

func createCashRegisterHandlerBusiness(t *testing.T, ownerAddress string) *database.Business {
	t.Helper()

	business := &database.Business{
		BusinessId:      fmt.Sprintf("cash-register-%s", strings.ToLower(ownerAddress)),
		Name:            "Caja Test Biz",
		OwnerAddress:    ownerAddress,
		SettlementAddr:  "0x1111111111111111111111111111111111111111",
		TippingAddr:     "0x2222222222222222222222222222222222222222",
		DefaultCurrency: "USD",
	}
	require.NoError(t, database.GetDB().Create(business).Error)
	return business
}

func createCashRegisterHandlerStaff(t *testing.T, businessID uint, role database.StaffRole) *database.Staff {
	t.Helper()

	staff := &database.Staff{
		BusinessID: businessID,
		Email:      fmt.Sprintf("%s-%d@example.com", role, businessID),
		Name:       string(role),
		Role:       role,
		IsActive:   true,
		InvitedBy:  "owner@example.com",
	}
	require.NoError(t, database.GetDB().Create(staff).Error)
	return staff
}

func cashRegisterHandlerRouter(t *testing.T, business *database.Business) *gin.Engine {
	t.Helper()
	return cashRegisterHandlerRouterWithContext(t, business, func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", business.OwnerAddress)
		c.Set("business_owner_address", business.OwnerAddress)
	})
}

func cashRegisterHandlerRouterWithContext(
	t *testing.T,
	business *database.Business,
	withContext func(c *gin.Context),
) *gin.Engine {
	t.Helper()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		withContext(c)
		c.Next()
	})

	handler := NewCashRegisterHandler(database.GetDB())
	group := router.Group("/inside/businesses/:id/cash-register")
	group.GET("/current", handler.GetCurrent)
	group.POST("/sessions", handler.OpenSession)
	group.GET("/sessions", handler.ListSessions)
	group.GET("/sessions/:sessionId", handler.GetSession)
	group.POST("/sessions/:sessionId/movements", handler.CreateMovement)
	group.POST("/sessions/:sessionId/close", handler.CloseSession)
	group.GET("/unassigned", handler.GetUnassigned)
	return router
}

func performCashRegisterRequest(t *testing.T, router *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var requestBody *bytes.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		require.NoError(t, err)
		requestBody = bytes.NewReader(payload)
	} else {
		requestBody = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, requestBody)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func decodeCashRegisterHandlerJSON(t *testing.T, body *bytes.Buffer) map[string]any {
	t.Helper()

	var payload map[string]any
	require.NoError(t, json.Unmarshal(body.Bytes(), &payload))
	return payload
}

func createCashRegisterHandlerOpenSession(t *testing.T, businessID uint, openingFloatCents int64) database.CashRegisterSession {
	t.Helper()

	session := database.CashRegisterSession{
		BusinessID:        businessID,
		Status:            database.CashRegisterSessionStatusOpen,
		OpeningFloatCents: openingFloatCents,
		OpenedByLabel:     "cashier",
		OpenedAt:          time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC),
	}
	require.NoError(t, database.GetDB().Create(&session).Error)
	return session
}

func createCashRegisterHandlerBill(t *testing.T, businessID uint, billNumber string) database.Bill {
	t.Helper()

	bill := database.Bill{
		BusinessID:     businessID,
		BillNumber:     billNumber,
		SettlementAddr: "settlement",
		TippingAddr:    "tipping",
		Status:         database.BillStatusPaid,
		TotalAmount:    10000,
		PaidAmount:     10000,
	}
	require.NoError(t, database.GetDB().Create(&bill).Error)
	return bill
}

func createCashRegisterHandlerCashPayment(t *testing.T, billID uint, amountCents int64) database.AlternativePayment {
	t.Helper()

	now := time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC)
	payment := database.AlternativePayment{
		BillID:          billID,
		ParticipantAddr: "cashier",
		ParticipantName: "Cashier",
		Amount:          amountCents,
		BillAmountCents: amountCents,
		PaymentMethod:   database.PaymentMethodCash,
		Status:          database.AltPaymentStatusConfirmed,
		ConfirmedBy:     "cashier",
		ConfirmedAt:     &now,
	}
	require.NoError(t, database.GetDB().Create(&payment).Error)
	return payment
}

func TestCashRegisterHandlersOpenCurrentManualCloseFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCashRegisterHandlerDB(t)
	business := createCashRegisterHandlerBusiness(t, "0xOwnerFlow")
	router := cashRegisterHandlerRouter(t, business)

	open := performCashRegisterRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/businesses/%d/cash-register/sessions", business.ID), map[string]any{
		"opening_float": 25.50,
		"opening_note":  "start shift",
	})
	require.Equal(t, http.StatusCreated, open.Code, open.Body.String())
	openPayload := decodeCashRegisterHandlerJSON(t, open.Body)
	require.Equal(t, "open", openPayload["status"])
	require.Equal(t, 25.50, openPayload["opening_float"])

	current := performCashRegisterRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/cash-register/current", business.ID), nil)
	require.Equal(t, http.StatusOK, current.Code, current.Body.String())
	currentPayload := decodeCashRegisterHandlerJSON(t, current.Body)
	require.NotNil(t, currentPayload["session"])
	require.Equal(t, float64(0), currentPayload["unassigned_cash_count"])
	require.Equal(t, float64(0), currentPayload["unassigned_cash_total"])
	require.Nil(t, currentPayload["suggested_opening_float"])

	sessionID := uint(openPayload["id"].(float64))
	movement := performCashRegisterRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/businesses/%d/cash-register/sessions/%d/movements", business.ID, sessionID), map[string]any{
		"movement_type": "cash_in",
		"amount":        10.25,
		"reason":        "drawer top-up",
		"note":          "manager approved",
	})
	require.Equal(t, http.StatusCreated, movement.Code, movement.Body.String())
	movementPayload := decodeCashRegisterHandlerJSON(t, movement.Body)
	require.Equal(t, "cash_in", movementPayload["movement"].(map[string]any)["movement_type"])
	require.Equal(t, 10.25, movementPayload["movement"].(map[string]any)["amount"])

	closeResp := performCashRegisterRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/businesses/%d/cash-register/sessions/%d/close", business.ID, sessionID), map[string]any{
		"counted_cash": 35.75,
		"closing_note": "balanced",
	})
	require.Equal(t, http.StatusOK, closeResp.Code, closeResp.Body.String())
	closePayload := decodeCashRegisterHandlerJSON(t, closeResp.Body)
	require.Equal(t, "closed", closePayload["status"])
	require.Equal(t, 35.75, closePayload["expected_cash"])
	require.Equal(t, 35.75, closePayload["counted_cash"])
	require.Equal(t, float64(0), closePayload["variance"])

	getSession := performCashRegisterRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/cash-register/sessions/%d", business.ID, sessionID), nil)
	require.Equal(t, http.StatusOK, getSession.Code, getSession.Body.String())
	sessionPayload := decodeCashRegisterHandlerJSON(t, getSession.Body)
	movements := sessionPayload["movements"].([]any)
	require.Len(t, movements, 1)
	require.Equal(t, "cash_in", movements[0].(map[string]any)["movement_type"])
}

func TestCashRegisterHandlersCurrentBlindCloseDoesNotReturnExpectedCash(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCashRegisterHandlerDB(t)
	business := createCashRegisterHandlerBusiness(t, "0xOwnerBlind")
	createCashRegisterHandlerOpenSession(t, business.ID, 5000)
	router := cashRegisterHandlerRouter(t, business)

	w := performCashRegisterRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/cash-register/current", business.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	payload := decodeCashRegisterHandlerJSON(t, w.Body)
	session := payload["session"].(map[string]any)
	require.NotContains(t, session, "expected_cash")
	require.NotContains(t, session, "counted_cash")
	require.NotContains(t, session, "variance")
}

func TestCashRegisterHandlersCurrentSuggestsDeclaredOpeningFloatNotCountedShortage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCashRegisterHandlerDB(t)
	business := createCashRegisterHandlerBusiness(t, "0xOwnerSuggestedFloat")
	router := cashRegisterHandlerRouter(t, business)

	olderClosedAt := time.Date(2026, 6, 26, 20, 0, 0, 0, time.UTC)
	require.NoError(t, database.GetDB().Create(&database.CashRegisterSession{
		BusinessID:        business.ID,
		Status:            database.CashRegisterSessionStatusClosed,
		OpeningFloatCents: 90000,
		ExpectedCashCents: 95000,
		CountedCashCents:  95000,
		OpenedByLabel:     "cashier",
		OpenedAt:          olderClosedAt.Add(-8 * time.Hour),
		ClosedByLabel:     "cashier",
		ClosedAt:          &olderClosedAt,
	}).Error)

	shortageClosedAt := time.Date(2026, 6, 27, 20, 0, 0, 0, time.UTC)
	require.NoError(t, database.GetDB().Create(&database.CashRegisterSession{
		BusinessID:        business.ID,
		Status:            database.CashRegisterSessionStatusClosed,
		OpeningFloatCents: 20000,
		ExpectedCashCents: 147490,
		CountedCashCents:  147307,
		VarianceCents:     -183,
		OpenedByLabel:     "cashier",
		OpenedAt:          shortageClosedAt.Add(-8 * time.Hour),
		ClosedByLabel:     "cashier",
		ClosedAt:          &shortageClosedAt,
	}).Error)

	w := performCashRegisterRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/cash-register/current", business.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	payload := decodeCashRegisterHandlerJSON(t, w.Body)
	require.Nil(t, payload["session"])
	require.Equal(t, 200.00, payload["suggested_opening_float"])
	require.NotEqual(t, 1473.07, payload["suggested_opening_float"])
	require.NotEqual(t, 1474.90, payload["suggested_opening_float"])
}

func TestCashRegisterHandlersCurrentSucceedsWhenSuggestedFloatLookupFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCashRegisterHandlerDB(t)
	business := createCashRegisterHandlerBusiness(t, "0xOwnerHintFailOpen")
	createCashRegisterHandlerClosedSession(t, business.ID, 20000)
	open := createCashRegisterHandlerOpenSession(t, business.ID, 12500)
	router := cashRegisterHandlerRouter(t, business)

	failErr := fmt.Errorf("forced last-declared-float lookup failure")
	const callback = "caja652:fail_hint"
	require.NoError(t, db.Callback().Query().Before("gorm:query").Register(callback, func(tx *gorm.DB) {
		if tx.Statement == nil {
			return
		}
		for _, sel := range tx.Statement.Selects {
			if strings.Contains(sel, "opening_float_cents") {
				_ = tx.AddError(failErr)
				return
			}
		}
	}))
	t.Cleanup(func() { _ = db.Callback().Query().Remove(callback) })

	w := performCashRegisterRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/cash-register/current", business.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	payload := decodeCashRegisterHandlerJSON(t, w.Body)
	require.NotEqual(t, http.StatusInternalServerError, w.Code)
	session, ok := payload["session"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(open.ID), session["id"])
	require.Nil(t, payload["suggested_opening_float"])
}

func TestCashRegisterHandlersOpenRejectsNegativeOpeningFloat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCashRegisterHandlerDB(t)
	business := createCashRegisterHandlerBusiness(t, "0xOwnerNegative")
	router := cashRegisterHandlerRouter(t, business)

	w := performCashRegisterRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/businesses/%d/cash-register/sessions", business.ID), map[string]any{
		"opening_float": -1,
	})
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestCashRegisterHandlersManualMovementRejectsSaleAndRefundTypes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCashRegisterHandlerDB(t)
	business := createCashRegisterHandlerBusiness(t, "0xOwnerMovementType")
	session := createCashRegisterHandlerOpenSession(t, business.ID, 0)
	router := cashRegisterHandlerRouter(t, business)

	for _, movementType := range []string{"cash_sale", "cash_refund"} {
		t.Run(movementType, func(t *testing.T) {
			w := performCashRegisterRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/businesses/%d/cash-register/sessions/%d/movements", business.ID, session.ID), map[string]any{
				"movement_type": movementType,
				"amount":        5,
				"reason":        "manual adjustment",
			})
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		})
	}
}

func TestCashRegisterHandlersManualMovementRequiresReason(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCashRegisterHandlerDB(t)
	business := createCashRegisterHandlerBusiness(t, "0xOwnerReason")
	session := createCashRegisterHandlerOpenSession(t, business.ID, 0)
	router := cashRegisterHandlerRouter(t, business)

	w := performCashRegisterRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/businesses/%d/cash-register/sessions/%d/movements", business.ID, session.ID), map[string]any{
		"movement_type": "cash_out",
		"amount":        5,
		"reason":        "  ",
	})
	require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestCashRegisterHandlersCloseRejectsDoubleClose(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCashRegisterHandlerDB(t)
	business := createCashRegisterHandlerBusiness(t, "0xOwnerDoubleClose")
	session := createCashRegisterHandlerOpenSession(t, business.ID, 1000)
	router := cashRegisterHandlerRouter(t, business)

	first := performCashRegisterRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/businesses/%d/cash-register/sessions/%d/close", business.ID, session.ID), map[string]any{
		"counted_cash": 10,
	})
	require.Equal(t, http.StatusOK, first.Code, first.Body.String())

	second := performCashRegisterRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/businesses/%d/cash-register/sessions/%d/close", business.ID, session.ID), map[string]any{
		"counted_cash": 10,
	})
	require.Equal(t, http.StatusConflict, second.Code, second.Body.String())
}

func createCashRegisterHandlerClosedSession(t *testing.T, businessID uint, openingFloatCents int64) database.CashRegisterSession {
	t.Helper()

	closedAt := time.Date(2026, 6, 27, 18, 0, 0, 0, time.UTC)
	session := database.CashRegisterSession{
		BusinessID:        businessID,
		Status:            database.CashRegisterSessionStatusClosed,
		OpeningFloatCents: openingFloatCents,
		OpenedByLabel:     "cashier",
		OpenedAt:          time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC),
		ClosedByLabel:     "cashier",
		ClosedAt:          &closedAt,
	}
	require.NoError(t, database.GetDB().Create(&session).Error)
	return session
}

// The sessions list is the closed-shifts history (L2-29): it must scope to the
// business AND exclude the currently open session.
func TestCashRegisterHandlersListSessionsScopesBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCashRegisterHandlerDB(t)
	business := createCashRegisterHandlerBusiness(t, "0xOwnerScopeA")
	other := createCashRegisterHandlerBusiness(t, "0xOwnerScopeB")
	createCashRegisterHandlerClosedSession(t, business.ID, 1000)
	createCashRegisterHandlerOpenSession(t, business.ID, 1500)
	createCashRegisterHandlerClosedSession(t, other.ID, 2000)
	router := cashRegisterHandlerRouter(t, business)

	w := performCashRegisterRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/cash-register/sessions", business.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	payload := decodeCashRegisterHandlerJSON(t, w.Body)
	require.Equal(t, float64(1), payload["total"])
	sessions := payload["sessions"].([]any)
	require.Len(t, sessions, 1)
	first := sessions[0].(map[string]any)
	require.Equal(t, float64(business.ID), first["business_id"])
	require.Equal(t, string(database.CashRegisterSessionStatusClosed), first["status"])
}

func TestCashRegisterHandlersBusinessLookupErrorReturnsInternalServerError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupCashRegisterHandlerDB(t)
	require.NoError(t, db.Migrator().DropTable(&database.Business{}))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xOwner")
		c.Set("business_owner_address", "0xOwner")
		c.Next()
	})
	handler := NewCashRegisterHandler(database.GetDB())
	router.GET("/inside/businesses/:id/cash-register/current", handler.GetCurrent)

	w := performCashRegisterRequest(t, router, http.MethodGet, "/inside/businesses/1/cash-register/current", nil)
	require.Equal(t, http.StatusInternalServerError, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), server.ErrCodeInternal)
}

func TestCashRegisterHandlersUnassignedCashReturnsSummary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCashRegisterHandlerDB(t)
	business := createCashRegisterHandlerBusiness(t, "0xOwnerUnassigned")
	bill := createCashRegisterHandlerBill(t, business.ID, "BILL-1")
	createCashRegisterHandlerCashPayment(t, bill.ID, 1250)
	createCashRegisterHandlerCashPayment(t, bill.ID, 775)
	router := cashRegisterHandlerRouter(t, business)

	w := performCashRegisterRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/cash-register/unassigned", business.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	payload := decodeCashRegisterHandlerJSON(t, w.Body)
	require.Equal(t, float64(2), payload["count"])
	require.Equal(t, 20.25, payload["total"])
}

func TestCashRegisterActorLabelUsesHumanUserName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCashRegisterHandlerDB(t)
	business := createCashRegisterHandlerBusiness(t, "0xOwnerHumanLabel")

	user := database.User{
		Email: "owner-label@example.com",
		Name:  "Local Admin",
		Role:  "user",
	}
	require.NoError(t, database.GetDB().Create(&user).Error)
	require.NoError(t, database.GetDB().Model(business).Update("user_id", user.ID).Error)
	business.UserID = &user.ID

	router := cashRegisterHandlerRouterWithContext(t, business, func(c *gin.Context) {
		c.Set("token_type", "user")
		c.Set("user_id", user.ID)
		c.Set("email", user.Email)
	})

	open := performCashRegisterRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/businesses/%d/cash-register/sessions", business.ID), map[string]any{
		"opening_float": 10.00,
		"opening_note":  "human label",
	})
	require.Equal(t, http.StatusCreated, open.Code, open.Body.String())
	payload := decodeCashRegisterHandlerJSON(t, open.Body)
	require.Equal(t, "Local Admin", payload["opened_by_label"])
	require.NotContains(t, fmt.Sprint(payload["opened_by_label"]), "user:")
}

func TestCashRegisterActorLabelUsesStaffNameFromContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCashRegisterHandlerDB(t)
	business := createCashRegisterHandlerBusiness(t, "0xOwnerStaffLabel")
	staff := createCashRegisterHandlerStaff(t, business.ID, database.StaffRoleManager)

	router := cashRegisterHandlerRouterWithContext(t, business, func(c *gin.Context) {
		c.Set("token_type", "staff")
		c.Set("staff_id", staff.ID)
		c.Set("staff_name", "Maria Lopez")
		c.Set("staff_email", staff.Email)
		c.Set("staff_business_id", business.ID)
	})

	open := performCashRegisterRequest(t, router, http.MethodPost, fmt.Sprintf("/inside/businesses/%d/cash-register/sessions", business.ID), map[string]any{
		"opening_float": 12.00,
	})
	require.Equal(t, http.StatusCreated, open.Code, open.Body.String())
	payload := decodeCashRegisterHandlerJSON(t, open.Body)
	require.Equal(t, "Maria Lopez", payload["opened_by_label"])
}

func TestCashRegisterHandlersCurrentOpensDemoHouseRailAfterSeededClose(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCashRegisterHandlerDB(t)
	business := createCashRegisterHandlerBusiness(t, "0xDemoDinner86")
	require.NoError(t, database.GetDB().Model(business).Updates(map[string]any{
		"is_demo": true,
		"kind":    database.BusinessKindDemo,
	}).Error)
	router := cashRegisterHandlerRouter(t, business)

	closedAt := time.Date(2026, 8, 21, 4, 0, 0, 0, time.UTC)
	require.NoError(t, database.GetDB().Create(&database.CashRegisterSession{
		BusinessID:        business.ID,
		Status:            database.CashRegisterSessionStatusClosed,
		OpeningFloatCents: 20000,
		CashSalesCents:    118230,
		OpenedByLabel:     "demo-manager",
		OpenedAt:          closedAt.Add(-2 * time.Hour),
		ClosedByLabel:     "demo-manager",
		ClosedAt:          &closedAt,
	}).Error)

	w := performCashRegisterRequest(t, router, http.MethodGet, fmt.Sprintf("/inside/businesses/%d/cash-register/current", business.ID), nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	payload := decodeCashRegisterHandlerJSON(t, w.Body)
	session, ok := payload["session"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "open", session["status"])
	require.Equal(t, "cash", payload["house_rail"])
	require.Equal(t, 200.00, payload["suggested_opening_float"])
}
