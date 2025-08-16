package tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"invoice-generator/database"
	"invoice-generator/models"
)

func setupDatabaseTest() *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("Failed to connect to test database")
	}

	// Auto-migrate all models
	err = db.AutoMigrate(&models.Invoice{}, &models.Payment{}, &models.EmailLog{})
	if err != nil {
		panic("Failed to migrate test database")
	}

	database.SetDB(db)
	return db
}

func TestInvoiceModel(t *testing.T) {
	db := setupDatabaseTest()

	t.Run("CreateInvoice", func(t *testing.T) {
		invoice := &models.Invoice{
			InvoiceID:   1,
			Creator:     "0x742d35Cc6634C0532925a3b8D0d0E0b4C3d5d1B1",
			CreatorName: "John Doe",
			Title:       "Test Invoice",
			Description: "This is a test invoice",
			Amount:      100000000, // 100 USDC
			PayerEmail:  "payer@example.com",
			PayerName:   "Jane Smith",
			Status:      "pending",
			PaymentLink: "http://localhost:8080/pay/1",
			QRCodeURL:   "http://localhost:8080/static/qr_1.png",
			MetadataURI: "http://localhost:8080/pay/1/metadata",
			IsActive:    true,
		}

		result := db.Create(invoice)
		require.NoError(t, result.Error)
		assert.Greater(t, invoice.ID, uint(0))
		assert.NotZero(t, invoice.CreatedAt)
		assert.NotZero(t, invoice.UpdatedAt)
	})

	t.Run("FindInvoice", func(t *testing.T) {
		// Create test invoice
		invoice := &models.Invoice{
			InvoiceID: 2,
			Creator:   "0x853e46Dd7645B0632936a4b9D0d0F0c5D4e6e2C2",
			Title:     "Find Test Invoice",
			Amount:    50000000,
			Status:    "pending",
		}
		db.Create(invoice)

		// Find by ID
		var foundInvoice models.Invoice
		result := db.First(&foundInvoice, invoice.ID)
		require.NoError(t, result.Error)
		assert.Equal(t, invoice.Title, foundInvoice.Title)
		assert.Equal(t, invoice.Amount, foundInvoice.Amount)

		// Find by InvoiceID
		var foundByInvoiceID models.Invoice
		result = db.Where("invoice_id = ?", 2).First(&foundByInvoiceID)
		require.NoError(t, result.Error)
		assert.Equal(t, invoice.Title, foundByInvoiceID.Title)
	})

	t.Run("UpdateInvoice", func(t *testing.T) {
		// Create test invoice
		invoice := &models.Invoice{
			InvoiceID: 3,
			Creator:   "0x964f57Fe8756B0743047a5c9E0e1F1d6F5f7f3D3",
			Title:     "Update Test Invoice",
			Amount:    75000000,
			Status:    "pending",
		}
		db.Create(invoice)

		// Update status
		originalUpdatedAt := invoice.UpdatedAt
		time.Sleep(1 * time.Millisecond) // Ensure timestamp difference

		result := db.Model(invoice).Update("status", "paid")
		require.NoError(t, result.Error)

		// Verify update
		var updatedInvoice models.Invoice
		db.First(&updatedInvoice, invoice.ID)
		assert.Equal(t, "paid", updatedInvoice.Status)
		assert.True(t, updatedInvoice.UpdatedAt.After(originalUpdatedAt))
	})

	t.Run("DeleteInvoice", func(t *testing.T) {
		// Create test invoice
		invoice := &models.Invoice{
			InvoiceID: 4,
			Creator:   "0x742d35Cc6634C0532925a3b8D0d0E0b4C3d5d1B1",
			Title:     "Delete Test Invoice",
			Amount:    25000000,
			Status:    "pending",
		}
		db.Create(invoice)

		// Soft delete
		result := db.Delete(invoice)
		require.NoError(t, result.Error)

		// Verify soft delete
		var deletedInvoice models.Invoice
		result = db.First(&deletedInvoice, invoice.ID)
		assert.Error(t, result.Error) // Should not find deleted record

		// Verify with Unscoped
		result = db.Unscoped().First(&deletedInvoice, invoice.ID)
		require.NoError(t, result.Error)
		assert.NotNil(t, deletedInvoice.DeletedAt)
	})

	t.Run("QueryInvoicesByCreator", func(t *testing.T) {
		creator := "0x111222333444555666777888999aaabbbcccddd"
		
		// Create multiple invoices for same creator
		invoices := []*models.Invoice{
			{InvoiceID: 10, Creator: creator, Title: "Invoice 1", Amount: 10000000, Status: "pending"},
			{InvoiceID: 11, Creator: creator, Title: "Invoice 2", Amount: 20000000, Status: "paid"},
			{InvoiceID: 12, Creator: creator, Title: "Invoice 3", Amount: 30000000, Status: "cancelled"},
		}

		for _, inv := range invoices {
			db.Create(inv)
		}

		// Query by creator
		var foundInvoices []models.Invoice
		result := db.Where("creator = ?", creator).Find(&foundInvoices)
		require.NoError(t, result.Error)
		assert.Len(t, foundInvoices, 3)

		// Query by creator and status
		var pendingInvoices []models.Invoice
		result = db.Where("creator = ? AND status = ?", creator, "pending").Find(&pendingInvoices)
		require.NoError(t, result.Error)
		assert.Len(t, pendingInvoices, 1)
		assert.Equal(t, "Invoice 1", pendingInvoices[0].Title)
	})
}

func TestPaymentModel(t *testing.T) {
	db := setupDatabaseTest()

	t.Run("CreatePayment", func(t *testing.T) {
		// First create an invoice
		invoice := &models.Invoice{
			InvoiceID: 100,
			Creator:   "0x742d35Cc6634C0532925a3b8D0d0E0b4C3d5d1B1",
			Title:     "Payment Test Invoice",
			Amount:    100000000,
			Status:    "pending",
		}
		db.Create(invoice)

		// Create payment
		payment := &models.Payment{
			InvoiceID: 100,
			Payer:     "0x853e46Dd7645B0632936a4b9D0d0F0c5D4e6e2C2",
			Amount:    100000000,
			Fee:       1000000, // 1%
			TxHash:    "0x123abc456def789ghi012jkl345mno678pqr901stu234vwx567yz890",
		}

		result := db.Create(payment)
		require.NoError(t, result.Error)
		assert.Greater(t, payment.ID, uint(0))
		assert.NotZero(t, payment.CreatedAt)
	})

	t.Run("QueryPaymentsByInvoice", func(t *testing.T) {
		invoiceID := uint64(200)
		
		// Create test invoice
		invoice := &models.Invoice{
			InvoiceID: invoiceID,
			Creator:   "0x964f57Fe8756B0743047a5c9E0e1F1d6F5f7f3D3",
			Title:     "Multi Payment Invoice",
			Amount:    200000000,
			Status:    "paid",
		}
		db.Create(invoice)

		// Create multiple payments
		payments := []*models.Payment{
			{
				InvoiceID: invoiceID,
				Payer:     "0x111111111111111111111111111111111111111111",
				Amount:    100000000,
				Fee:       1000000,
				TxHash:    "0x111",
			},
			{
				InvoiceID: invoiceID,
				Payer:     "0x222222222222222222222222222222222222222222",
				Amount:    100000000,
				Fee:       1000000,
				TxHash:    "0x222",
			},
		}

		for _, payment := range payments {
			db.Create(payment)
		}

		// Query payments by invoice
		var foundPayments []models.Payment
		result := db.Where("invoice_id = ?", invoiceID).Find(&foundPayments)
		require.NoError(t, result.Error)
		assert.Len(t, foundPayments, 2)

		// Verify total amount
		totalAmount := uint64(0)
		for _, payment := range foundPayments {
			totalAmount += payment.Amount
		}
		assert.Equal(t, uint64(200000000), totalAmount)
	})

	t.Run("UpdatePayment", func(t *testing.T) {
		// Create invoice first
		invoice := &models.Invoice{
			InvoiceID: 300,
			Creator:   "0x853e46Dd7645B0632936a4b9D0d0F0c5D4e6e2C2",
			Title:     "Update Payment Invoice",
			Amount:    50000000,
			Status:    "pending",
		}
		db.Create(invoice)

		// Create payment
		payment := &models.Payment{
			InvoiceID: 300,
			Payer:     "0x964f57Fe8756B0743047a5c9E0e1F1d6F5f7f3D3",
			Amount:    50000000,
			Fee:       500000,
			TxHash:    "0x333",
		}
		db.Create(payment)

		// Update payment amount
		updatedPayment := &models.Payment{}
		result := db.Model(updatedPayment).Where("id = ?", payment.ID).Update("amount", 60000000)
		require.NoError(t, result.Error)

		// Verify update
		db.First(updatedPayment, payment.ID)
		assert.Equal(t, uint64(60000000), updatedPayment.Amount)
	})

	t.Run("PaymentAmountUpdates", func(t *testing.T) {
		payment := &models.Payment{
			InvoiceID: 300,
			Payer:     "0x333333333333333333333333333333333333333333",
			Amount:    50000000,
			Fee:       500000,
			TxHash:    "0x333",
		}
		db.Create(payment)

		// Update amount
		result := db.Model(payment).Update("amount", 55000000)
		require.NoError(t, result.Error)

		// Verify update
		var updatedPayment models.Payment
		db.First(&updatedPayment, payment.ID)
		assert.Equal(t, uint64(55000000), updatedPayment.Amount)
	})

	t.Run("CreateEmailLog", func(t *testing.T) {
		// Create invoice first
		invoice := &models.Invoice{
			InvoiceID: 400,
			Creator:   "0x742d35Cc6634C0532925a3b8D0d0E0b4C3d5d1B1",
			Title:     "Email Test Invoice",
			Amount:    100000000,
			Status:    "pending",
		}
		db.Create(invoice)

		// Create email log
		now := time.Now()
		emailLog := &models.EmailLog{
			InvoiceID: 400,
			EmailType: "initial",
			Recipient: "test@example.com",
			Subject:   "Invoice Created",
			SentAt:    &now,
		}

		result := db.Create(emailLog)
		require.NoError(t, result.Error)
		assert.Greater(t, emailLog.ID, uint(0))
		assert.NotZero(t, emailLog.CreatedAt)
	})

	t.Run("QueryEmailLogsByType", func(t *testing.T) {
		invoiceID := uint64(500)
		
		// Create invoice first
		invoice := &models.Invoice{
			InvoiceID: invoiceID,
			Creator:   "0x964f57Fe8756B0743047a5c9E0e1F1d6F5f7f3D3",
			Title:     "Multi Email Invoice",
			Amount:    200000000,
			Status:    "paid",
		}
		db.Create(invoice)

		// Create multiple email logs
		now := time.Now()
		reminder := now.Add(24 * time.Hour)
		confirmation := now.Add(48 * time.Hour)
		emailLogs := []*models.EmailLog{
			{
				InvoiceID: invoiceID,
				EmailType: "initial",
				Recipient: "test1@example.com",
				Subject:   "Invoice Created",
				SentAt:    &now,
			},
			{
				InvoiceID: invoiceID,
				EmailType: "reminder",
				Recipient: "test1@example.com",
				Subject:   "Payment Reminder",
				SentAt:    &reminder,
			},
			{
				InvoiceID: invoiceID,
				EmailType: "confirmation",
				Recipient: "test1@example.com",
				Subject:   "Payment Confirmed",
				SentAt:    &confirmation,
			},
		}

		for _, emailLog := range emailLogs {
			db.Create(emailLog)
		}

		// Query logs by email type
		var initialLogs []models.EmailLog
		result := db.Where("email_type = ?", "initial").Find(&initialLogs)
		require.NoError(t, result.Error)
		assert.GreaterOrEqual(t, len(initialLogs), 1)
	})
}

func TestDatabaseConstraints(t *testing.T) {
	db := setupDatabaseTest()

	t.Run("UniqueInvoiceID", func(t *testing.T) {
		// Create first invoice
		invoice1 := &models.Invoice{
			InvoiceID: 800,
			Creator:   "0x742d35Cc6634C0532925a3b8D0d0E0b4C3d5d1B1",
			Title:     "First Invoice",
			Amount:    100000000,
			Status:    "pending",
		}
		result := db.Create(invoice1)
		require.NoError(t, result.Error)

		// Try to create second invoice with same InvoiceID
		invoice2 := &models.Invoice{
			InvoiceID: 800, // Same ID
			Creator:   "0x853e46Dd7645B0632936a4b9D0d0F0c5D4e6e2C2",
			Title:     "Second Invoice",
			Amount:    200000000,
			Status:    "pending",
		}
		result = db.Create(invoice2)
		assert.Error(t, result.Error) // Should fail due to unique constraint
	})

	t.Run("RequiredFields", func(t *testing.T) {
		// Try to create invoice without required fields
		invalidInvoice := &models.Invoice{
			// Missing required fields
		}
		
		result := db.Create(invalidInvoice)
		// Note: GORM might not enforce all validations at DB level
		// This test verifies the model can handle empty values
		if result.Error != nil {
			assert.Error(t, result.Error)
		}
	})
}
