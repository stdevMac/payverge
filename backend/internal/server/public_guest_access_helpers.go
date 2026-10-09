package server

import (
	"errors"
	"net/http"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/gin-gonic/gin"
)

var errPublicGuestTableNotFound = errors.New("public guest table not found")
var errPublicGuestBillNotFound = errors.New("public guest bill not found")

func loadPublicGuestTableContext(code string) (*database.Table, *database.Business, error) {
	return services.PublicGuestTableContextByCode(code, loadPublicGuestTableContextUncached)
}

func loadPublicGuestTableContextUncached(code string) (*database.Table, *database.Business, error) {
	table, business, err := database.GetActiveTableWithBusinessByCode(code)
	if err != nil {
		// Only a genuine not-found maps to a 404. A DB/infra error must
		// propagate so respondPublicGuestTableLookupError returns 500 instead
		// of telling the guest their valid table doesn't exist.
		if errors.Is(err, database.ErrTableNotFound) {
			return nil, nil, errPublicGuestTableNotFound
		}
		return nil, nil, err
	}

	return table, business, nil
}

func loadPublicGuestBillByToken(token string) (*database.Bill, []database.BillItem, error) {
	bill, items, err := database.GetPublicBillByToken(token)
	if err != nil {
		if errors.Is(err, database.ErrPublicGuestBillNotFound) {
			return nil, nil, errPublicGuestBillNotFound
		}
		return nil, nil, err
	}

	return bill, items, nil
}

func respondPublicGuestTableLookupError(c *gin.Context, err error) {
	if errors.Is(err, errPublicGuestTableNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Table not found"})
		return
	}

	c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load table"})
}

func respondPublicGuestBillLookupError(c *gin.Context, err error) {
	if errors.Is(err, errPublicGuestBillNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Bill not found"})
		return
	}

	c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load bill"})
}
