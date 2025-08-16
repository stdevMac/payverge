package services

import (
	"log"
	"time"

	"invoice-generator/database"
	"invoice-generator/models"

	"gorm.io/gorm"
)

type EventListenerService struct {
	db             *gorm.DB
	blockchain     *BlockchainService
	email          *EmailService
	eventChan      chan ContractEvent
	stopChan       chan bool
}

func NewEventListenerService(blockchain *BlockchainService, email *EmailService) *EventListenerService {
	return &EventListenerService{
		db:         database.GetDB(),
		blockchain: blockchain,
		email:      email,
		eventChan:  make(chan ContractEvent, 100),
		stopChan:   make(chan bool),
	}
}

func (els *EventListenerService) Start() error {
	log.Println("Starting event listener service...")

	// Subscribe to blockchain events
	err := els.blockchain.SubscribeToEvents(els.eventChan)
	if err != nil {
		return err
	}

	// Start event processing goroutine
	go els.processEvents()

	log.Println("Event listener service started successfully")
	return nil
}

func (els *EventListenerService) Stop() {
	log.Println("Stopping event listener service...")
	els.stopChan <- true
	close(els.eventChan)
}

func (els *EventListenerService) processEvents() {
	for {
		select {
		case event := <-els.eventChan:
			if err := els.handleEvent(event); err != nil {
				log.Printf("Error handling event: %v", err)
			}
		case <-els.stopChan:
			log.Println("Event processing stopped")
			return
		}
	}
}

func (els *EventListenerService) handleEvent(event ContractEvent) error {
	switch event.Type {
	case "InvoiceCreated":
		return els.handleInvoiceCreated(event)
	case "InvoicePaid":
		return els.handleInvoicePaid(event)
	default:
		log.Printf("Unknown event type: %s", event.Type)
		return nil
	}
}

func (els *EventListenerService) handleInvoiceCreated(event ContractEvent) error {
	log.Printf("Processing InvoiceCreated event for invoice %d", event.InvoiceID)

	// Find the invoice in our database
	var invoice models.Invoice
	result := els.db.Where("invoice_id = ?", event.InvoiceID).First(&invoice)
	if result.Error != nil {
		log.Printf("Invoice %d not found in database, skipping event", event.InvoiceID)
		return nil
	}

	// Update invoice with transaction hash
	invoice.TxHash = event.TxHash
	if err := els.db.Save(&invoice).Error; err != nil {
		return err
	}

	// Send initial invoice email if payer email is provided
	if invoice.PayerEmail != "" {
		if err := els.email.SendInvoiceEmail(&invoice); err != nil {
			log.Printf("Failed to send invoice email: %v", err)
			// Log the email attempt
			els.logEmailAttempt(invoice.InvoiceID, "initial", invoice.PayerEmail, "failed", err.Error())
		} else {
			log.Printf("Invoice email sent successfully for invoice %d", invoice.InvoiceID)
			els.logEmailAttempt(invoice.InvoiceID, "initial", invoice.PayerEmail, "sent", "")
		}
	}

	return nil
}

func (els *EventListenerService) handleInvoicePaid(event ContractEvent) error {
	log.Printf("Processing InvoicePaid event for invoice %d", event.InvoiceID)

	// Find the invoice in our database
	var invoice models.Invoice
	result := els.db.Where("invoice_id = ?", event.InvoiceID).First(&invoice)
	if result.Error != nil {
		log.Printf("Invoice %d not found in database", event.InvoiceID)
		return result.Error
	}

	// Create payment record
	payment := models.Payment{
		InvoiceID:   event.InvoiceID,
		Payer:       event.Payer,
		Amount:      event.Amount,
		Fee:         event.PlatformFee,
		TxHash:      event.TxHash,
		BlockNumber: event.BlockNumber,
	}

	if err := els.db.Create(&payment).Error; err != nil {
		return err
	}

	// Update invoice status and amount paid
	invoice.AmountPaid += event.Amount

	// Determine new status
	if invoice.AmountPaid >= invoice.Amount {
		invoice.Status = "paid"
	} else {
		invoice.Status = "partially_paid"
	}

	if err := els.db.Save(&invoice).Error; err != nil {
		return err
	}

	// Send payment confirmation email
	if invoice.PayerEmail != "" {
		if err := els.email.SendPaymentConfirmationEmail(&invoice, &payment); err != nil {
			log.Printf("Failed to send payment confirmation email: %v", err)
			els.logEmailAttempt(invoice.InvoiceID, "confirmation", invoice.PayerEmail, "failed", err.Error())
		} else {
			log.Printf("Payment confirmation email sent for invoice %d", invoice.InvoiceID)
			els.logEmailAttempt(invoice.InvoiceID, "confirmation", invoice.PayerEmail, "sent", "")
		}
	}

	log.Printf("Invoice %d payment processed successfully. Status: %s", event.InvoiceID, invoice.Status)
	return nil
}

func (els *EventListenerService) logEmailAttempt(invoiceID uint64, emailType, recipient, status, errorMsg string) {
	emailLog := models.EmailLog{
		InvoiceID: invoiceID,
		EmailType: emailType,
		Recipient: recipient,
		Status:    status,
		Error:     errorMsg,
	}

	if status == "sent" {
		now := time.Now()
		emailLog.SentAt = &now
	}

	if err := els.db.Create(&emailLog).Error; err != nil {
		log.Printf("Failed to log email attempt: %v", err)
	}
}
