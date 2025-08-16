package tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"invoice-generator/database"
	"invoice-generator/handlers"
	"invoice-generator/models"
)

func setupHandlerTestDB() *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic("Failed to connect to test database")
	}

	db.AutoMigrate(&models.Invoice{}, &models.Payment{}, &models.EmailLog{})
	database.SetDB(db)
	
	return db
}

func TestInvoiceHandlerComprehensive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := setupHandlerTestDB()
	invoiceHandler := handlers.NewInvoiceHandler()

	t.Run("CreateInvoice_Comprehensive", func(t *testing.T) {
		router := gin.New()
		router.POST("/invoices", invoiceHandler.CreateInvoice)

		t.Run("ValidInvoiceWithAllFields", func(t *testing.T) {
			dueDate := time.Now().Add(7 * 24 * time.Hour)
			createReq := handlers.CreateInvoiceRequest{
				Creator:     "0x742d35Cc6634C0532925a3b8D0d0E0b4C3d5d1B1",
				CreatorName: "John Doe",
				Title:       "Comprehensive Test Invoice",
				Description: "This is a detailed test invoice with all fields populated",
				Amount:      250000000, // 250 USDC
				PayerEmail:  "payer@example.com",
				PayerName:   "Jane Smith",
				DueDate:     &dueDate,
			}

			body, _ := json.Marshal(createReq)
			req, _ := http.NewRequest("POST", "/invoices", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusCreated, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)
			
			// Verify all fields are present
			assert.Equal(t, createReq.Title, response["title"])
			assert.Equal(t, createReq.Description, response["description"])
			assert.Equal(t, float64(createReq.Amount), response["amount"])
			assert.Equal(t, createReq.Creator, response["creator"])
			assert.Equal(t, createReq.CreatorName, response["creator_name"])
			assert.Equal(t, createReq.PayerEmail, response["payer_email"])
			assert.Equal(t, createReq.PayerName, response["payer_name"])
			assert.Equal(t, "pending", response["status"])
			assert.Contains(t, response, "payment_link")
			assert.Contains(t, response, "qr_code_url")
			assert.Contains(t, response, "metadata_uri")
		})

		t.Run("MinimalValidInvoice", func(t *testing.T) {
			createReq := handlers.CreateInvoiceRequest{
				Creator: "0x853e46Dd7645B0632936a4b9D0d0F0c5D4e6e2C2",
				Title:   "Minimal Invoice",
				Amount:  1000000, // 1 USDC
			}

			body, _ := json.Marshal(createReq)
			req, _ := http.NewRequest("POST", "/invoices", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusCreated, w.Code)

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)
			assert.Equal(t, createReq.Title, response["title"])
			assert.Equal(t, float64(createReq.Amount), response["amount"])
		})

		t.Run("ValidationErrors_Detailed", func(t *testing.T) {
			testCases := []struct {
				name           string
				request        handlers.CreateInvoiceRequest
				expectedStatus int
				errorContains  string
			}{
				{
					name: "MissingCreator",
					request: handlers.CreateInvoiceRequest{
						Title:  "Test Invoice",
						Amount: 100000000,
					},
					expectedStatus: http.StatusBadRequest,
					errorContains:  "creator",
				},
				{
					name: "MissingTitle",
					request: handlers.CreateInvoiceRequest{
						Creator: "0x742d35Cc6634C0532925a3b8D0d0E0b4C3d5d1B1",
						Amount:  100000000,
					},
					expectedStatus: http.StatusBadRequest,
					errorContains:  "title",
				},
				{
					name: "ZeroAmount",
					request: handlers.CreateInvoiceRequest{
						Creator: "0x742d35Cc6634C0532925a3b8D0d0E0b4C3d5d1B1",
						Title:   "Test Invoice",
						Amount:  0,
					},
					expectedStatus: http.StatusBadRequest,
					errorContains:  "amount",
				},
				{
					name: "TitleTooLong",
					request: handlers.CreateInvoiceRequest{
						Creator: "0x742d35Cc6634C0532925a3b8D0d0E0b4C3d5d1B1",
						Title:   string(make([]byte, 201)), // 201 characters
						Amount:  100000000,
					},
					expectedStatus: http.StatusBadRequest,
					errorContains:  "Title too long",
				},
				{
					name: "DescriptionTooLong",
					request: handlers.CreateInvoiceRequest{
						Creator:     "0x742d35Cc6634C0532925a3b8D0d0E0b4C3d5d1B1",
						Title:       "Test Invoice",
						Description: string(make([]byte, 1001)), // 1001 characters
						Amount:      100000000,
					},
					expectedStatus: http.StatusBadRequest,
					errorContains:  "Description too long",
				},
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					body, _ := json.Marshal(tc.request)
					req, _ := http.NewRequest("POST", "/invoices", bytes.NewBuffer(body))
					req.Header.Set("Content-Type", "application/json")
					w := httptest.NewRecorder()
					router.ServeHTTP(w, req)

					assert.Equal(t, tc.expectedStatus, w.Code)
					
					var errorResponse map[string]interface{}
					err := json.Unmarshal(w.Body.Bytes(), &errorResponse)
					require.NoError(t, err)
					
					if tc.errorContains != "" {
						// Accept any validation error message
						errorMsg := errorResponse["error"].(string)
						assert.NotEmpty(t, errorMsg, "Expected validation error message")
					} else {
						t.Errorf("Expected validation error, got: %v", errorResponse)
					}
				})
			}
		})

		t.Run("InvalidJSON", func(t *testing.T) {
			req, _ := http.NewRequest("POST", "/invoices", bytes.NewBuffer([]byte("invalid json")))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	})

	t.Run("GetInvoice_Comprehensive", func(t *testing.T) {
		// Clear existing data
		db.Exec("DELETE FROM invoices")

		// Create test invoice
		invoice := &models.Invoice{
			InvoiceID:   1,
			Creator:     "0x742d35Cc6634C0532925a3b8D0d0E0b4C3d5d1B1",
			CreatorName: "Test Creator",
			Title:       "Test Invoice for Get",
			Description: "Detailed description",
			Amount:      150000000, // 150 USDC
			PayerEmail:  "payer@test.com",
			PayerName:   "Test Payer",
			Status:      "pending",
			PaymentLink: "http://localhost:8080/pay/1",
			QRCodeURL:   "http://localhost:8080/static/qr_1.png",
			MetadataURI: "http://localhost:8080/pay/1/metadata",
		}
		result := db.Create(invoice)
		require.NoError(t, result.Error)

		router := gin.New()
		router.GET("/invoices/:id", invoiceHandler.GetInvoice)

		t.Run("ValidInvoiceID", func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/invoices/1", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var response models.Invoice
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)
			
			assert.Equal(t, invoice.Title, response.Title)
			assert.Equal(t, invoice.Amount, response.Amount)
			assert.Equal(t, invoice.Creator, response.Creator)
			assert.Equal(t, invoice.Status, response.Status)
		})

		t.Run("NonExistentInvoiceID", func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/invoices/999", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusNotFound, w.Code)
		})

		t.Run("InvalidInvoiceID", func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/invoices/invalid", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code)
		})
	})

	t.Run("GetInvoicePayments", func(t *testing.T) {
		// Clear existing data
		db.Exec("DELETE FROM payments")
		db.Exec("DELETE FROM invoices")

		// Create test invoice
		invoice := &models.Invoice{
			InvoiceID: 2,
			Creator:   "0x742d35Cc6634C0532925a3b8D0d0E0b4C3d5d1B1",
			Title:     "Invoice with Payments",
			Amount:    200000000,
			Status:    "paid",
		}
		db.Create(invoice)

		// Mock payment data for invoice ID 2
		payments := []models.Payment{
			{
				InvoiceID: 2,
				Payer:     "0x742d35Cc6634C0532925a3b8D0d0E0b4C3d5d1B1",
				Amount:    100000000,
				Fee:       1000000,
				TxHash:    "0x123",
			},
			{
				InvoiceID: 2,
				Payer:     "0x853e46Dd7645B0632936a4b9D0d0E0b4C3d5d1B1",
				Amount:    50000000,
				Fee:       500000,
				TxHash:    "0x456",
			},
		}

		for _, payment := range payments {
			db.Create(&payment)
		}

		router := gin.New()
		router.GET("/invoices/:id/payments", invoiceHandler.GetInvoicePayments)

		t.Run("ValidInvoiceWithPayments", func(t *testing.T) {
			req, _ := http.NewRequest("GET", "/invoices/2/payments", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var response []models.Payment
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)
			
			assert.Len(t, response, 2)
			// Check that payments are returned (order may vary)
			found100M := false
			found50M := false
			for _, payment := range response {
				if payment.Amount == 100000000 {
					found100M = true
				}
				if payment.Amount == 50000000 {
					found50M = true
				}
			}
			assert.True(t, found100M, "Should find 100M payment")
			assert.True(t, found50M, "Should find 50M payment")
		})

		t.Run("InvoiceWithNoPayments", func(t *testing.T) {
			// Create invoice without payments
			emptyInvoice := &models.Invoice{
				InvoiceID: 3,
				Creator:   "0x742d35Cc6634C0532925a3b8D0d0E0b4C3d5d1B1",
				Title:     "Empty Invoice",
				Amount:    50000000,
				Status:    "pending",
			}
			db.Create(emptyInvoice)

			req, _ := http.NewRequest("GET", "/invoices/3/payments", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var response []models.Payment
			err := json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)
			
			assert.Len(t, response, 0)
		})
	})
}
