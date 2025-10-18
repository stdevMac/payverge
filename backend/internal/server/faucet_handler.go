package server

import (
	"net/http"

	"payverge/internal/faucet"

	"github.com/gin-gonic/gin"
)

// CheckAndTopUp handles requests to check an Ethereum address balance and top it up if needed
// It verifies the address is valid, checks its current balance, and sends ETH if:
// 1. The balance is below the minimum threshold
// 2. The address hasn't received ETH in the last 3 days (cooldown period)
func CheckAndTopUp(c *gin.Context) {
	var input struct {
		Address string `json:"address"`
	}

	// Bind JSON request body to input struct
	if err := c.BindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Invalid request format",
		})
		return
	}

	// Check if address is empty
	if input.Address == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Address is required",
		})
		return
	}

	// Call the faucet service to check balance and top up if needed
	response, err := faucet.CheckAndTopUp(input.Address)
	if err != nil {
		// The error is already formatted in the response
		c.JSON(http.StatusInternalServerError, response)
		return
	}

	// Return appropriate status code based on the response status
	statusCode := http.StatusOK
	if response.Status == "error" {
		statusCode = http.StatusInternalServerError
	} else if response.Status == "cooldown_period" {
		statusCode = http.StatusTooManyRequests
	}

	c.JSON(statusCode, response)
}

// CheckFaucetAvailability handles requests to check if an Ethereum address is eligible for a faucet top-up
// without actually sending any ETH
func CheckFaucetAvailability(c *gin.Context) {
	// Get address from URL path parameter
	address := c.Param("address")

	// Check if address is empty
	if address == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  "Address is required in the URL path",
		})
		return
	}

	// Call the faucet service to check availability
	response, err := faucet.CheckFaucetAvailability(address)
	if err != nil {
		// The error is already formatted in the response
		c.JSON(http.StatusInternalServerError, response)
		return
	}

	// Return appropriate status code based on the response status
	statusCode := http.StatusOK
	if response.Status == "error" {
		statusCode = http.StatusInternalServerError
	} else if response.Status == "cooldown_period" {
		statusCode = http.StatusTooManyRequests
	}

	c.JSON(statusCode, response)
}
