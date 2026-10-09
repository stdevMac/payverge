package handlers

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	qrcode "github.com/skip2/go-qrcode"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/plugins"
	"github.com/stdevmac/payverge/backend/internal/plugins/mercadopago"
	"github.com/stdevmac/payverge/backend/internal/server"
)

const (
	mpPointPluginName      = "mercadopago"
	mpPointExpirationTime  = "PT30M"
	mpPointTerminalModePDV = "PDV"
	mpPointTerminalModeSA  = "STANDALONE"
)

// resolveMercadoPagoPlugin returns the registered Mercado Pago plugin, or nil if
// unavailable / wrong type. Overridable in tests.
var resolveMercadoPagoPlugin = func() (*mercadopago.MercadoPagoPlugin, error) {
	plugin, ok := plugins.GetPluginByName(mpPointPluginName)
	if !ok {
		return nil, errors.New("mercadopago plugin not registered")
	}
	mp, ok := plugin.(*mercadopago.MercadoPagoPlugin)
	if !ok || mp == nil {
		return nil, errors.New("mercadopago plugin has unexpected type")
	}
	return mp, nil
}

// ListMercadoPagoTerminals GET /inside/businesses/:id/mercadopago/terminals
func (ph *PluginHandlers) ListMercadoPagoTerminals(c *gin.Context) {
	businessID, ok := resolveBusinessIDParam(c)
	if !ok {
		return
	}

	mp, err := resolveMercadoPagoPlugin()
	if err != nil {
		server.RespondWithError(c, http.StatusServiceUnavailable, server.ErrCodeServiceUnavailable, "Mercado Pago is not available")
		return
	}

	terminals, err := mp.ListTerminals(businessID)
	if err != nil {
		log.Printf("mercadopago list terminals business=%d: %v", businessID, err)
		respondMercadoPagoPointAPIError(c, err)
		return
	}

	out := make([]gin.H, 0, len(terminals))
	for _, t := range terminals {
		out = append(out, gin.H{
			"id":              t.ID,
			"pos_id":          t.PosID,
			"store_id":        t.StoreID,
			"external_pos_id": t.ExternalPosID,
			"operating_mode":  t.OperatingMode,
		})
	}
	c.JSON(http.StatusOK, gin.H{"terminals": out})
}

// SetMercadoPagoTerminalMode PATCH /inside/businesses/:id/mercadopago/terminals/:terminal_id/mode
func (ph *PluginHandlers) SetMercadoPagoTerminalMode(c *gin.Context) {
	businessID, ok := resolveBusinessIDParam(c)
	if !ok {
		return
	}
	terminalID := strings.TrimSpace(c.Param("terminal_id"))
	if terminalID == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "terminal_id is required")
		return
	}

	var body struct {
		Mode string `json:"mode" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		server.RespondBindError(c, err)
		return
	}
	mode := strings.ToUpper(strings.TrimSpace(body.Mode))
	if mode != mpPointTerminalModePDV && mode != mpPointTerminalModeSA {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "mode must be PDV or STANDALONE")
		return
	}

	mp, err := resolveMercadoPagoPlugin()
	if err != nil {
		server.RespondWithError(c, http.StatusServiceUnavailable, server.ErrCodeServiceUnavailable, "Mercado Pago is not available")
		return
	}

	if err := mp.SetTerminalMode(businessID, terminalID, mode); err != nil {
		log.Printf("mercadopago set terminal mode business=%d terminal=%s: %v", businessID, terminalID, err)
		respondMercadoPagoPointAPIError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"terminal_id":    terminalID,
		"operating_mode": mode,
	})
}

// ChargeMercadoPagoPoint POST /inside/businesses/:id/bills/:bill_id/mercadopago/point/charge
func (ph *PluginHandlers) ChargeMercadoPagoPoint(c *gin.Context) {
	businessID, ok := resolveBusinessIDParam(c)
	if !ok {
		return
	}
	billID, ok := parseUintParam(c, "bill_id")
	if !ok {
		return
	}

	var body struct {
		TerminalID  string `json:"terminal_id" binding:"required"`
		AmountCents *int64 `json:"amount_cents"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		server.RespondBindError(c, err)
		return
	}
	terminalID := strings.TrimSpace(body.TerminalID)
	if terminalID == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "terminal_id is required")
		return
	}

	bill, currency, ok := loadBillForMercadoPagoPointCharge(c, businessID, billID)
	if !ok {
		return
	}

	amountCents := bill.TotalAmount - bill.PaidAmount
	if body.AmountCents != nil && *body.AmountCents > 0 {
		amountCents = *body.AmountCents
	}
	if amountCents <= 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "charge amount must be positive")
		return
	}
	// Canonicalize once so the MP order amount and tracker cents match webhook
	// settlement (zero-decimal CLP/COP floor to whole major units, never overshoot).
	amountCents = mercadopago.CanonicalAmountCents(amountCents, currency)
	if amountCents <= 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "charge amount is below one major unit for this currency")
		return
	}

	mp, err := resolveMercadoPagoPlugin()
	if err != nil {
		server.RespondWithError(c, http.StatusServiceUnavailable, server.ErrCodeServiceUnavailable, "Mercado Pago is not available")
		return
	}

	// Serialize check + create + tracker insert per bill (F7 advisory lock).
	var (
		conflictOrderID string
		createdOrderID  string
		createErr       error
		trackFailed     bool
	)
	lockErr := withMercadoPagoBillChargeLock(bill.ID, func() error {
		// 409 if a pending Orders-API Point/QR charge already exists (F8: ignore mp_tracker_*).
		if existing, found, err := findPendingMercadoPagoTracker(bill.ID); err != nil {
			return err
		} else if found {
			conflictOrderID = existing.ParticipantAddr
			return nil
		}

		extRef := fmt.Sprintf("bill_%d_business_%d", bill.ID, businessID)
		amountStr := mercadopago.DecimalAmount(amountCents, currency)
		order, err := mp.CreateOrder(businessID, mercadopago.OrderRequest{
			Type:              "point",
			ExternalReference: extRef,
			ExpirationTime:    mpPointExpirationTime,
			Description:       mercadoPagoBillDescription(bill.ID),
			Transactions: mercadopago.OrderTransactions{
				Payments: []mercadopago.OrderPayment{{Amount: amountStr}},
			},
			Config: mercadopago.OrderConfig{
				Point: &mercadopago.PointConfig{TerminalID: terminalID},
			},
		}, uuid.NewString())
		if err != nil {
			createErr = err
			return nil
		}
		createdOrderID = order.ID

		// Tracker: ParticipantAddr = order ID so webhooks can resolve without business_id.
		if _, err := ph.storePluginPaymentRecord(
			bill.ID,
			businessID,
			mpPointPluginName,
			order.ID,
			amountCents,
			currency,
			amountCents,
			0,
			nil,
		); err != nil {
			log.Printf("mercadopago point tracker insert business=%d bill=%d order=%s: %v", businessID, bill.ID, order.ID, err)
			if cancelErr := mp.CancelOrder(businessID, order.ID); cancelErr != nil {
				log.Printf("mercadopago point rollback cancel order=%s: %v", order.ID, cancelErr)
			}
			trackFailed = true
			return nil
		}
		return nil
	})
	if lockErr != nil {
		log.Printf("mercadopago point charge lock/check bill=%d: %v", bill.ID, lockErr)
		if errors.Is(lockErr, errMercadoPagoBillChargeLockTimeout) {
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Another charge is in progress for this bill; retry shortly")
			return
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeServerError, "Failed to check existing charges")
		return
	}
	if conflictOrderID != "" {
		c.JSON(http.StatusConflict, gin.H{
			"error":    "A Mercado Pago charge is already pending for this bill",
			"code":     server.ErrCodeConflict,
			"order_id": conflictOrderID,
		})
		return
	}
	if createErr != nil {
		log.Printf("mercadopago point createOrder business=%d bill=%d: %v", businessID, bill.ID, createErr)
		respondMercadoPagoPointAPIError(c, createErr)
		return
	}
	if trackFailed {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeServerError, "Failed to track Point charge")
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"order_id": createdOrderID,
		"status":   "pending",
	})
}

// GetMercadoPagoOrder GET /inside/businesses/:id/mercadopago/orders/:order_id
func (ph *PluginHandlers) GetMercadoPagoOrder(c *gin.Context) {
	businessID, ok := resolveBusinessIDParam(c)
	if !ok {
		return
	}
	orderID := strings.TrimSpace(c.Param("order_id"))
	if orderID == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "order_id is required")
		return
	}

	if ok := ensureMercadoPagoOrderBelongsToBusiness(c, businessID, orderID); !ok {
		return
	}

	mp, err := resolveMercadoPagoPlugin()
	if err != nil {
		server.RespondWithError(c, http.StatusServiceUnavailable, server.ErrCodeServiceUnavailable, "Mercado Pago is not available")
		return
	}

	order, err := mp.GetOrder(businessID, orderID)
	if err != nil {
		log.Printf("mercadopago getOrder business=%d order=%s: %v", businessID, orderID, err)
		respondMercadoPagoPointAPIError(c, err)
		return
	}

	resp := gin.H{
		"order_id": order.ID,
		"status":   mercadopago.OrderStatus(order.Status),
	}
	// P6d: surface QR payload on status poll so a 409 recovery client can render
	// the scannable code when MP still provides it (encode PNG server-side).
	if qr := strings.TrimSpace(order.TypeResponse.QRData); qr != "" {
		resp["qr_data"] = qr
		if png, encErr := mpQRCodeEncode(qr, qrcode.Medium, mpQRCodeSize); encErr == nil {
			resp["qr_png_base64"] = base64.StdEncoding.EncodeToString(png)
		}
	}
	c.JSON(http.StatusOK, resp)
}

// CancelMercadoPagoOrder POST /inside/businesses/:id/mercadopago/orders/:order_id/cancel
func (ph *PluginHandlers) CancelMercadoPagoOrder(c *gin.Context) {
	businessID, ok := resolveBusinessIDParam(c)
	if !ok {
		return
	}
	orderID := strings.TrimSpace(c.Param("order_id"))
	if orderID == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "order_id is required")
		return
	}

	tracker, ok := loadMercadoPagoTrackerForBusiness(c, businessID, orderID)
	if !ok {
		return
	}

	mp, err := resolveMercadoPagoPlugin()
	if err != nil {
		server.RespondWithError(c, http.StatusServiceUnavailable, server.ErrCodeServiceUnavailable, "Mercado Pago is not available")
		return
	}

	if err := mp.CancelOrder(businessID, orderID); err != nil {
		log.Printf("mercadopago cancelOrder business=%d order=%s: %v", businessID, orderID, err)
		// If MP already cancelled the order, still mark the local tracker so the
		// bill is not stuck pending forever (409 on next charge).
		if !mercadopagoOrderAlreadyCancelled(mp, businessID, orderID) {
			respondMercadoPagoPointAPIError(c, err)
			return
		}
	}

	if err := markMercadoPagoTrackerCancelledByID(tracker.ID); err != nil {
		log.Printf("mercadopago cancel tracker order=%s: %v", orderID, err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeServerError, "Order cancelled but tracker update failed")
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "cancelled"})
}

// markMercadoPagoTrackerCancelled marks a pending tracker cancelled by order id
// (ParticipantAddr). Used when encode/create rollbacks cancel an MP order.
func markMercadoPagoTrackerCancelled(orderID string) error {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return nil
	}
	now := time.Now()
	return database.GetDB().Model(&database.AlternativePayment{}).
		Where("participant_addr = ? AND payment_method = ? AND status = ?",
			orderID, database.AlternativePaymentMethod("mercadopago"), database.AltPaymentStatusPending).
		Updates(map[string]interface{}{
			"status":     database.AltPaymentStatusCancelled,
			"updated_at": now,
		}).Error
}

func markMercadoPagoTrackerCancelledByID(trackerID uint) error {
	now := time.Now()
	return database.GetDB().Model(&database.AlternativePayment{}).
		Where("id = ? AND status = ?", trackerID, database.AltPaymentStatusPending).
		Updates(map[string]interface{}{
			"status":     database.AltPaymentStatusCancelled,
			"updated_at": now,
		}).Error
}

// mercadopagoOrderAlreadyCancelled reports whether GET order shows a cancelled
// terminal status (so a failed cancel can still free the local tracker).
func mercadopagoOrderAlreadyCancelled(mp *mercadopago.MercadoPagoPlugin, businessID uint, orderID string) bool {
	if mp == nil {
		return false
	}
	order, err := mp.GetOrder(businessID, orderID)
	if err != nil || order == nil {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(order.Status))
	return status == "canceled" || status == "cancelled" || status == "expired"
}

// ── helpers ──────────────────────────────────────────────────────────────────

func resolveBusinessIDParam(c *gin.Context) (uint, bool) {
	businessID, err := resolveBusinessID(c.Param("id"))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Business not found")
			return 0, false
		}
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return 0, false
	}
	return businessID, true
}

func parseUintParam(c *gin.Context, name string) (uint, bool) {
	raw := strings.TrimSpace(c.Param(name))
	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, fmt.Sprintf("Invalid %s", name))
		return 0, false
	}
	return uint(id), true
}

func loadBillForMercadoPagoPointCharge(c *gin.Context, businessID, billID uint) (*database.Bill, string, bool) {
	var bill database.Bill
	err := database.GetDB().
		Select("id", "business_id", "total_amount", "paid_amount", "status", "bill_number").
		Where("id = ?", billID).
		First(&bill).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Bill not found")
			return nil, "", false
		}
		log.Printf("mercadopago point load bill=%d: %v", billID, err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeServerError, "Failed to load bill")
		return nil, "", false
	}
	// Ownership: wrong-business bill → 404 (do not leak existence).
	if bill.BusinessID != businessID {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Bill not found")
		return nil, "", false
	}

	currency := "USD"
	var biz database.Business
	if err := database.GetDB().Select("default_currency").Where("id = ?", businessID).First(&biz).Error; err == nil {
		if strings.TrimSpace(biz.DefaultCurrency) != "" {
			currency = strings.ToUpper(strings.TrimSpace(biz.DefaultCurrency))
		}
	}
	return &bill, currency, true
}

// isMercadoPagoOrdersAPITracker reports whether participant_addr is an Orders
// API order id (ORD…) used by Point/QR staff charges — not a guest Checkout Pro
// placeholder (mp_tracker_…).
func isMercadoPagoOrdersAPITracker(participantAddr string) bool {
	addr := strings.TrimSpace(participantAddr)
	if addr == "" {
		return false
	}
	lower := strings.ToLower(addr)
	if strings.HasPrefix(lower, "mp_tracker_") {
		return false
	}
	return strings.HasPrefix(strings.ToUpper(addr), "ORD")
}

// findPendingMercadoPagoTracker returns a pending Orders-API (Point/QR) tracker
// for the bill. Guest Checkout Pro placeholders (mp_tracker_*) are ignored so
// abandoned guest checkouts do not block staff charges (F8). Expired pending
// rows are swept then ignored (P4) so they neither 409 a new charge nor settle.
func findPendingMercadoPagoTracker(billID uint) (*database.AlternativePayment, bool, error) {
	if err := expireStalePendingPluginTrackers(billID); err != nil {
		return nil, false, err
	}
	now := time.Now().UTC()
	var rows []database.AlternativePayment
	err := database.GetDB().
		Where("bill_id = ? AND payment_method = ? AND status = ?",
			billID,
			database.AlternativePaymentMethod(mpPointPluginName),
			database.AltPaymentStatusPending,
		).
		Order("id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, false, err
	}
	for i := range rows {
		if !pluginTrackerIsActivelyPending(&rows[i], now) {
			continue
		}
		if isMercadoPagoOrdersAPITracker(rows[i].ParticipantAddr) {
			return &rows[i], true, nil
		}
	}
	return nil, false, nil
}

// mercadoPagoBillChargeLockNamespace namespaces pg_advisory_xact_lock keys for
// Point/QR charge serialization (must not collide with genesis bootstrap lock).
const mercadoPagoBillChargeLockNamespace int64 = 0x4d504348 // "MPCH"

func mercadoPagoBillChargeLockKey(billID uint) int64 {
	return (mercadoPagoBillChargeLockNamespace << 32) | int64(billID&0xffffffff)
}

// acquireMercadoPagoBillChargeLock takes a transaction-scoped Postgres advisory
// lock for the bill. No-op on non-Postgres (SQLite tests) where the surrounding
// transaction + MaxOpenConns=1 still serializes.
func acquireMercadoPagoBillChargeLock(tx *gorm.DB, billID uint) error {
	if tx == nil || tx.Dialector == nil || tx.Dialector.Name() != "postgres" {
		return nil
	}
	return tx.Exec("SELECT pg_advisory_xact_lock(?)", mercadoPagoBillChargeLockKey(billID)).Error
}

// mercadoPagoBillChargeLocalMu serializes Point/QR charge creation per bill in
// non-Postgres environments (SQLite tests use MaxOpenConns=1 — a transaction
// held across storePluginPaymentRecord would deadlock on a second connection).
var mercadoPagoBillChargeLocalMu sync.Map // billID -> *sync.Mutex

// mercadoPagoBillChargeLockTimeout bounds how long a waiter blocks on the
// Postgres advisory lock. Without this, a stuck holder (long MP HTTP while
// holding the xact lock) can exhaust the connection pool.
const mercadoPagoBillChargeLockTimeout = "3s"

// errMercadoPagoBillChargeLockTimeout is returned when SET LOCAL lock_timeout
// fires while waiting for the charge advisory lock.
var errMercadoPagoBillChargeLockTimeout = errors.New("mercadopago: bill charge lock timeout")

// withMercadoPagoBillChargeLock runs fn while holding a per-bill lock so
// concurrent Point/QR charge requests serialize check + create + tracker insert.
// On Postgres: transaction-scoped pg_advisory_xact_lock with lock_timeout so
// waiters get a clean error instead of hanging forever. Elsewhere: process mutex.
func withMercadoPagoBillChargeLock(billID uint, fn func() error) error {
	db := database.GetDB()
	if db != nil && db.Dialector != nil && db.Dialector.Name() == "postgres" {
		return db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec("SET LOCAL lock_timeout = '" + mercadoPagoBillChargeLockTimeout + "'").Error; err != nil {
				return err
			}
			if err := acquireMercadoPagoBillChargeLock(tx, billID); err != nil {
				// Postgres lock_timeout → SQLSTATE 55P03 (lock_not_available).
				msg := err.Error()
				if strings.Contains(msg, "55P03") || strings.Contains(strings.ToLower(msg), "lock timeout") ||
					strings.Contains(strings.ToLower(msg), "canceling statement due to lock timeout") {
					return fmt.Errorf("%w: bill %d", errMercadoPagoBillChargeLockTimeout, billID)
				}
				return err
			}
			return fn()
		})
	}
	muIface, _ := mercadoPagoBillChargeLocalMu.LoadOrStore(billID, &sync.Mutex{})
	mu := muIface.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	return fn()
}

func loadMercadoPagoTrackerForBusiness(c *gin.Context, businessID uint, orderID string) (*database.AlternativePayment, bool) {
	var tracker database.AlternativePayment
	err := database.GetDB().
		Where("participant_addr = ? AND payment_method = ?",
			orderID,
			database.AlternativePaymentMethod(mpPointPluginName),
		).
		First(&tracker).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Order not found")
			return nil, false
		}
		log.Printf("mercadopago load tracker order=%s: %v", orderID, err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeServerError, "Failed to load order")
		return nil, false
	}

	var billBizID uint
	if err := database.GetDB().Model(&database.Bill{}).
		Select("business_id").
		Where("id = ?", tracker.BillID).
		Scan(&billBizID).Error; err != nil || billBizID == 0 {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Order not found")
		return nil, false
	}
	if billBizID != businessID {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeNotFound, "Order not found")
		return nil, false
	}
	return &tracker, true
}

func ensureMercadoPagoOrderBelongsToBusiness(c *gin.Context, businessID uint, orderID string) bool {
	_, ok := loadMercadoPagoTrackerForBusiness(c, businessID, orderID)
	return ok
}

func respondMercadoPagoPointAPIError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	if errors.Is(err, mercadopago.ErrTerminalBusy) {
		server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Terminal is busy with another order")
		return
	}
	if errors.Is(err, mercadopago.ErrForbidden) {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Mercado Pago rejected the request")
		return
	}
	if errors.Is(err, mercadopago.ErrUsersMeFailed) {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput,
			"Could not resolve Mercado Pago user id from the access token; reconnect or re-enter credentials")
		return
	}
	if errors.Is(err, mercadopago.ErrInsufficientTokenScope) {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput,
			"Mercado Pago token lacks store/POS permissions; reconnect OAuth with offline_access or enable in-store scopes")
		return
	}
	server.RespondWithError(c, http.StatusBadGateway, server.ErrCodeServerError, "Mercado Pago request failed")
}
