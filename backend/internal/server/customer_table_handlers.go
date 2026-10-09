package server

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func CheckInCustomerToTable(c *gin.Context) {
	customerIDRaw, exists := c.Get("customer_id")
	if !exists {
		RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenMissing, "Customer authentication required")
		return
	}
	customerID, ok := contextUintFromValue(customerIDRaw)
	if !ok || customerID == 0 {
		RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "Invalid customer session")
		return
	}

	tableCode := c.Param("code")
	table, business, err := database.GetActiveTableWithBusinessByCode(tableCode)
	if err != nil {
		RespondWithError(c, http.StatusNotFound, ErrCodeBusinessNotFound, "Business not found")
		return
	}

	billAttached := false
	var billID *uint
	bill, err := database.GetOpenBillSummaryByTableID(table.ID)
	if err != nil && !errors.Is(err, database.ErrNoActiveBill) {
		RespondWithError(c, http.StatusInternalServerError, "", "Failed to load table bill")
		return
	}
	if bill != nil {
		attached, attachErr := database.AttachCRMCustomerIDToBillIfEmpty(bill.ID, customerID)
		if attachErr != nil {
			if errors.Is(attachErr, database.ErrBillCRMCustomerConflict) {
				RespondWithError(c, http.StatusConflict, "bill_customer_conflict", "This bill is already linked to another customer")
				return
			}
			if errors.Is(attachErr, database.ErrBillNotOpen) {
				RespondWithError(c, http.StatusConflict, "bill_not_open", "This bill is no longer open")
				return
			}
			RespondWithError(c, http.StatusInternalServerError, "", "Failed to attach customer to bill")
			return
		}
		billAttached = attached.CRMCustomerID != nil && *attached.CRMCustomerID == customerID
		billID = &bill.ID
	}

	connection, err := connectCustomerToActiveBusinessForCheckIn(customerID, business.ID, true)
	if err != nil {
		respondCheckInConnectionError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"customer_business": connection,
		"bill_attached":     billAttached,
		"bill_id":           billID,
	})
}

var (
	errCheckInCustomerNotFound  = errors.New("customer not found")
	errCheckInCustomerInactive  = errors.New("customer account is inactive")
	errCheckInBusinessNotFound  = errors.New("business not found")
	errCheckInBusinessInactive  = errors.New("business is inactive")
	errCheckInConnectionStorage = errors.New("customer business connection storage error")
)

func respondCheckInConnectionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errCheckInCustomerNotFound):
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "Customer not found")
	case errors.Is(err, errCheckInCustomerInactive):
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "Customer account is inactive")
	case errors.Is(err, errCheckInBusinessNotFound), errors.Is(err, errCheckInBusinessInactive):
		RespondWithError(c, http.StatusNotFound, ErrCodeBusinessNotFound, "Business not found")
	default:
		RespondWithError(c, http.StatusInternalServerError, "", "Failed to connect customer to business")
	}
}

func connectCustomerToActiveBusinessForCheckIn(customerID, businessID uint, optInMarketing bool) (*database.CustomerBusiness, error) {
	db := database.GetDB()

	if err := validateCustomerForCheckIn(db, customerID); err != nil {
		return nil, err
	}

	connection, err := database.EnsureActiveCustomerBusinessConnection(db, customerID, businessID, optInMarketing)
	if err != nil {
		return nil, errCheckInConnectionStorage
	}

	return connection, nil
}

func validateCustomerForCheckIn(db *gorm.DB, customerID uint) error {
	var customer database.Customer
	if err := db.Select("id", "is_active").First(&customer, customerID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errCheckInCustomerNotFound
		}
		return errCheckInConnectionStorage
	}
	if !customer.IsActive {
		return errCheckInCustomerInactive
	}
	return nil
}

func contextUintFromValue(v interface{}) (uint, bool) {
	switch typed := v.(type) {
	case uint:
		return typed, true
	case float64:
		if typed <= 0 {
			return 0, false
		}
		return uint(typed), true
	default:
		return 0, false
	}
}
