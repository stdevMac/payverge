package websocket

import (
	"encoding/json"
	"log"
	"strconv"
	"time"

	"payverge/internal/blockchain"
	"payverge/internal/database"
)

// PaymentMonitor handles real-time payment monitoring and notifications
type PaymentMonitor struct {
	hub        *Hub
	db         *database.DB
	blockchain *blockchain.BlockchainService
	stopCh     chan struct{}
}

// PaymentNotification represents a payment notification message
type PaymentNotification struct {
	Type            string    `json:"type"`
	BillID          uint      `json:"bill_id"`
	TransactionHash string    `json:"transaction_hash"`
	Amount          int64     `json:"amount"`
	TipAmount       int64     `json:"tip_amount"`
	PayerAddress    string    `json:"payer_address"`
	BusinessID      uint      `json:"business_id"`
	Status          string    `json:"status"`
	Timestamp       time.Time `json:"timestamp"`
}

// BillUpdateNotification represents a bill status update
type BillUpdateNotification struct {
	Type       string `json:"type"`
	BillID     uint   `json:"bill_id"`
	BusinessID uint   `json:"business_id"`
	TableID    uint   `json:"table_id"`
	Status     string `json:"status"`
	PaidAmount int64  `json:"paid_amount"`
	TipAmount  int64  `json:"tip_amount"`
	Total      int64  `json:"total"`
	Remaining  int64  `json:"remaining"`
	Timestamp  time.Time `json:"timestamp"`
}

// NewPaymentMonitor creates a new payment monitor
func NewPaymentMonitor(hub *Hub, db *database.DB, blockchain *blockchain.BlockchainService) *PaymentMonitor {
	return &PaymentMonitor{
		hub:        hub,
		db:         db,
		blockchain: blockchain,
		stopCh:     make(chan struct{}),
	}
}

// Start begins monitoring for payment events
func (pm *PaymentMonitor) Start() error {
	log.Println("Starting payment monitor...")

	// Start blockchain event monitoring
	err := pm.blockchain.MonitorPayments(pm.handlePaymentEvent)
	if err != nil {
		return err
	}

	// Start periodic bill status checks
	go pm.startPeriodicChecks()

	log.Println("Payment monitor started successfully")
	return nil
}

// Stop stops the payment monitor
func (pm *PaymentMonitor) Stop() {
	log.Println("Stopping payment monitor...")
	close(pm.stopCh)
}

// handlePaymentEvent processes incoming payment events from the blockchain
func (pm *PaymentMonitor) handlePaymentEvent(payment blockchain.Payment) {
	log.Printf("Received payment event: %+v", payment)

	// Parse bill ID
	billID, err := strconv.ParseUint(payment.BillID, 10, 32)
	if err != nil {
		log.Printf("Invalid bill ID in payment event: %s", payment.BillID)
		return
	}

	// Get bill from database
	bill, err := pm.db.GetBill(uint(billID))
	if err != nil {
		log.Printf("Failed to get bill %d: %v", billID, err)
		return
	}

	// Update bill with payment information
	bill.PaidAmount += float64(payment.Amount)
	bill.TipAmount += float64(payment.TipAmount)

	// Update bill status
	if bill.PaidAmount >= bill.TotalAmount {
		bill.Status = database.BillStatusPaid
	} else if bill.PaidAmount > 0 {
		bill.Status = "partial"
	}

	// Save updated bill
	err = pm.db.UpdateBill(bill)
	if err != nil {
		log.Printf("Failed to update bill %d: %v", billID, err)
		return
	}

	// Send payment notification to business dashboard
	pm.sendPaymentNotification(payment, bill)

	// Send bill update notification to guests
	pm.sendBillUpdateNotification(bill)
}

// sendPaymentNotification sends a payment notification to the business dashboard
func (pm *PaymentMonitor) sendPaymentNotification(payment blockchain.Payment, bill *database.Bill) {
	notification := PaymentNotification{
		Type:            "payment_received",
		BillID:          bill.ID,
		TransactionHash: payment.TransactionHash,
		Amount:          payment.Amount,
		TipAmount:       payment.TipAmount,
		PayerAddress:    payment.Payer,
		BusinessID:      bill.BusinessID,
		Status:          "confirmed",
		Timestamp:       payment.Timestamp,
	}

	// Send to business dashboard room
	businessRoom := "business_" + strconv.FormatUint(uint64(bill.BusinessID), 10)
	pm.sendToRoom(businessRoom, notification)

	log.Printf("Sent payment notification to business %d", bill.BusinessID)
}

// sendBillUpdateNotification sends a bill update notification to guests
func (pm *PaymentMonitor) sendBillUpdateNotification(bill *database.Bill) {
	notification := BillUpdateNotification{
		Type:       "bill_update",
		BillID:     bill.ID,
		BusinessID: bill.BusinessID,
		TableID:    bill.TableID,
		Status:     string(bill.Status),
		PaidAmount: int64(bill.PaidAmount),
		TipAmount:  int64(bill.TipAmount),
		Total:      int64(bill.TotalAmount),
		Remaining:  int64(bill.TotalAmount - bill.PaidAmount),
		Timestamp:  time.Now(),
	}

	// Send to table room (for guests)
	table, err := pm.db.GetTable(bill.TableID)
	if err != nil {
		log.Printf("Failed to get table %d: %v", bill.TableID, err)
		return
	}

	tableRoom := "table_" + table.TableCode
	pm.sendToRoom(tableRoom, notification)

	// Also send to bill-specific room
	billRoom := "bill_" + strconv.FormatUint(uint64(bill.ID), 10)
	pm.sendToRoom(billRoom, notification)

	log.Printf("Sent bill update notification for bill %d", bill.ID)
}

// sendToRoom sends a message to all clients in a specific room
func (pm *PaymentMonitor) sendToRoom(room string, data interface{}) {
	message, err := json.Marshal(data)
	if err != nil {
		log.Printf("Failed to marshal notification: %v", err)
		return
	}

	pm.hub.BroadcastToRoom(room, message)
}

// startPeriodicChecks starts periodic checks for payment confirmations
func (pm *PaymentMonitor) startPeriodicChecks() {
	ticker := time.NewTicker(30 * time.Second) // Check every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pm.checkPendingPayments()
		case <-pm.stopCh:
			return
		}
	}
}

// checkPendingPayments checks for pending payments that may need status updates
func (pm *PaymentMonitor) checkPendingPayments() {
	// Get bills with partial or open status
	bills, err := pm.db.GetBillsByStatus(database.BillStatusOpen)
	if err != nil {
		log.Printf("Failed to get pending bills: %v", err)
		return
	}

	for _, bill := range bills {
		// Check blockchain for latest payment status
		totalPaid, err := pm.blockchain.GetBillTotalPaid(strconv.FormatUint(uint64(bill.ID), 10))
		if err != nil {
			log.Printf("Failed to get total paid for bill %d: %v", bill.ID, err)
			continue
		}

		// Update if there's a discrepancy
		if float64(totalPaid) != bill.PaidAmount {
			bill.PaidAmount = float64(totalPaid)

			// Update status
			if bill.PaidAmount >= bill.TotalAmount {
				bill.Status = database.BillStatusPaid
			} else if bill.PaidAmount > 0 {
				bill.Status = "partial"
			}

			if err := pm.db.UpdateBill(bill); err != nil {
				log.Printf("Failed to update bill %d: %v", bill.ID, err)
				continue
			}

			// Send notification
			pm.sendBillUpdateNotification(bill)
		}
	}
}

// NotifyPaymentConfirmation manually triggers a payment confirmation notification
func (pm *PaymentMonitor) NotifyPaymentConfirmation(billID uint, txHash string) {
	bill, err := pm.db.GetBill(billID)
	if err != nil {
		log.Printf("Failed to get bill %d for confirmation: %v", billID, err)
		return
	}

	// Create a mock payment for notification
	payment := blockchain.Payment{
		BillID:          strconv.FormatUint(uint64(billID), 10),
		TransactionHash: txHash,
		Timestamp:       time.Now(),
	}

	pm.sendPaymentNotification(payment, bill)
	pm.sendBillUpdateNotification(bill)
}

// GetPaymentStats returns payment statistics for monitoring
func (pm *PaymentMonitor) GetPaymentStats() map[string]interface{} {
	// This could be expanded to include various payment statistics
	return map[string]interface{}{
		"monitor_status": "active",
		"last_check":     time.Now(),
	}
}
