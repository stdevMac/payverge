package handlers

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	qrcode "github.com/skip2/go-qrcode"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/plugins/mercadopago"
	"github.com/stdevmac/payverge/backend/internal/server"
)

const (
	mpQRPluginName     = "mercadopago"
	mpQRExpirationTime = "PT10M"
	mpQRExpiration     = 10 * time.Minute
	mpQRModeDynamic    = "dynamic"
	mpQRCodeSize       = 512
)

// mpQRCodeEncode is the QR PNG encoder; tests may swap it to force encode failures.
var mpQRCodeEncode = qrcode.Encode

// mercadoPagoBillDescription is the order description Mercado Pago shows the
// payer for a QR or Point charge, named after this instance (PRODUCT_NAME).
func mercadoPagoBillDescription(billID uint) string {
	return fmt.Sprintf("%s bill #%d", config.ProductName(), billID)
}

// ChargeMercadoPagoQR POST /inside/businesses/:id/bills/:bill_id/mercadopago/qr/charge
//
// Provisions store/POS on first use, creates a type:"qr" dynamic order with a
// 10-minute expiry, generates an on-screen PNG, and tracks the pending charge
// with ParticipantAddr = order id (status/cancel reuse Get/CancelMercadoPagoOrder).
func (ph *PluginHandlers) ChargeMercadoPagoQR(c *gin.Context) {
	businessID, ok := resolveBusinessIDParam(c)
	if !ok {
		return
	}
	billID, ok := parseUintParam(c, "bill_id")
	if !ok {
		return
	}

	var body struct {
		AmountCents *int64 `json:"amount_cents"`
	}
	// Empty body is allowed (defaults to outstanding balance).
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			server.RespondBindError(c, err)
			return
		}
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

	// Serialize pending-check + create + tracker insert per bill (F7). Pending
	// check runs first so 409 does not require store/POS provisioning.
	var (
		conflictOrderID string
		createdOrderID  string
		qrData          string
		createErr       error
		ensureErr       error
		missingPOS      bool
		missingQR       bool
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

		externalPOSID, err := mp.EnsureStoreAndPOS(businessID)
		if err != nil {
			ensureErr = err
			return nil
		}
		externalPOSID = strings.TrimSpace(externalPOSID)
		if externalPOSID == "" {
			missingPOS = true
			return nil
		}

		extRef := fmt.Sprintf("bill_%d_business_%d", bill.ID, businessID)
		amountStr := mercadopago.DecimalAmount(amountCents, currency)
		order, err := mp.CreateOrder(businessID, mercadopago.OrderRequest{
			Type:              "qr",
			ExternalReference: extRef,
			ExpirationTime:    mpQRExpirationTime,
			Description:       mercadoPagoBillDescription(bill.ID),
			Transactions: mercadopago.OrderTransactions{
				Payments: []mercadopago.OrderPayment{{Amount: amountStr}},
			},
			Config: mercadopago.OrderConfig{
				QR: &mercadopago.QRConfig{
					ExternalPOSID: externalPOSID,
					Mode:          mpQRModeDynamic,
				},
			},
		}, uuid.NewString())
		if err != nil {
			createErr = err
			return nil
		}
		createdOrderID = order.ID
		qrData = strings.TrimSpace(order.TypeResponse.QRData)
		if qrData == "" {
			missingQR = true
			if cancelErr := mp.CancelOrder(businessID, order.ID); cancelErr != nil {
				log.Printf("mercadopago qr rollback cancel order=%s: %v", order.ID, cancelErr)
			}
			return nil
		}

		if _, err := ph.storePluginPaymentRecord(
			bill.ID,
			businessID,
			mpQRPluginName,
			order.ID,
			amountCents,
			currency,
			amountCents,
			0,
			nil,
		); err != nil {
			log.Printf("mercadopago qr tracker insert business=%d bill=%d order=%s: %v", businessID, bill.ID, order.ID, err)
			if cancelErr := mp.CancelOrder(businessID, order.ID); cancelErr != nil {
				log.Printf("mercadopago qr rollback cancel order=%s: %v", order.ID, cancelErr)
			}
			trackFailed = true
			return nil
		}
		return nil
	})
	if lockErr != nil {
		log.Printf("mercadopago qr charge lock/check bill=%d: %v", bill.ID, lockErr)
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
	if ensureErr != nil {
		log.Printf("mercadopago qr ensureStoreAndPOS business=%d: %v", businessID, ensureErr)
		respondMercadoPagoPointAPIError(c, ensureErr)
		return
	}
	if missingPOS {
		server.RespondWithError(c, http.StatusBadGateway, server.ErrCodeServerError, "Mercado Pago POS is not available")
		return
	}
	if createErr != nil {
		log.Printf("mercadopago qr createOrder business=%d bill=%d: %v", businessID, bill.ID, createErr)
		respondMercadoPagoPointAPIError(c, createErr)
		return
	}
	if missingQR {
		log.Printf("mercadopago qr createOrder missing qr_data business=%d bill=%d order=%s", businessID, bill.ID, createdOrderID)
		server.RespondWithError(c, http.StatusBadGateway, server.ErrCodeServerError, "Mercado Pago did not return QR data")
		return
	}
	if trackFailed {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeServerError, "Failed to track QR charge")
		return
	}

	png, err := mpQRCodeEncode(qrData, qrcode.Medium, mpQRCodeSize)
	if err != nil {
		log.Printf("mercadopago qr encode business=%d order=%s: %v", businessID, createdOrderID, err)
		// Cancel MP order AND mark local tracker cancelled so the bill is not
		// stuck in pending forever (next charge would 409 on the orphan tracker).
		if cancelErr := mp.CancelOrder(businessID, createdOrderID); cancelErr != nil {
			log.Printf("mercadopago qr rollback cancel order=%s: %v", createdOrderID, cancelErr)
		}
		if markErr := markMercadoPagoTrackerCancelled(createdOrderID); markErr != nil {
			log.Printf("mercadopago qr encode mark tracker cancelled order=%s: %v", createdOrderID, markErr)
		}
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeServerError, "Failed to generate QR image")
		return
	}
	qrPNGBase64 := base64.StdEncoding.EncodeToString(png)
	expiresAt := time.Now().UTC().Add(mpQRExpiration)

	c.JSON(http.StatusCreated, gin.H{
		"order_id":      createdOrderID,
		"qr_data":       qrData,
		"qr_png_base64": qrPNGBase64,
		"expires_at":    expiresAt.Format(time.RFC3339),
		"status":        "pending",
	})
}
