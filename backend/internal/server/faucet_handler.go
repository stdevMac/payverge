package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/stdevMac/shares/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stdevMac/shares/internal/faucet"
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

func CreateCode(c *gin.Context) {
	var code database.Codes

	if err := c.ShouldBindJSON(&code); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data"})
		return
	}

	generatedCode := faucet.GenerateCode()

	code.Code = generatedCode

	// Store on Database
	err := database.SaveCode(&code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error creating code"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Code created successfully"})
}

func GetCode(c *gin.Context) {
	var input struct {
		Code string `json:"code"`
	}

	if err := c.BindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// Call the faucet service to check balance and top up if needed
	response, err := database.GetCode(input.Code)
	if err != nil {
		// The error is already formatted in the response
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error getting code"})
		return
	}

	c.JSON(http.StatusOK, response)
}

func UpdateCode(c *gin.Context) {
	var code database.Codes

	if err := c.BindJSON(&code); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// Call the faucet service to check balance and top up if needed
	err := database.UpdateCode(&code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error updating code"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Code updated successfully"})
}

// DeleteCode deletes a code from the database
func DeleteCode(c *gin.Context) {
	var input struct {
		Code string `json:"code"`
	}

	if err := c.BindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// Call the faucet service to check balance and top up if needed
	err := database.DeleteCode(input.Code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error deleting code"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Code deleted successfully"})

}

// GetAllCodes retrieves all codes from the database
func GetAllCodes(c *gin.Context) {
	codes, err := database.GetAllCodes()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error retrieving codes"})
		return
	}

	c.JSON(http.StatusOK, codes)
}

// CheckCode verifies if a code is valid and available for use without actually redeeming it
func CheckCode(c *gin.Context) {
	var input struct {
		Code string `json:"code"`
	}

	if err := c.BindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// Check if code is provided
	if input.Code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Code is required"})
		return
	}

	// Get code from database
	codeObj, err := database.GetCode(input.Code)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Code not found or invalid"})
		return
	}

	// Check if code is already used
	if codeObj.Used {
		c.JSON(http.StatusBadRequest, gin.H{"status": "unavailable", "error": "Code has already been used"})
		return
	}

	// Check if code is expired
	if codeObj.Expiry != "" {
		expiry, err := time.Parse(time.RFC3339, codeObj.Expiry)
		if err == nil && time.Now().After(expiry) {
			c.JSON(http.StatusBadRequest, gin.H{"status": "unavailable", "error": "Code has expired"})
			return
		}
	}

	// Code is valid and available
	c.JSON(http.StatusOK, gin.H{
		"status": "available",
		"code":   input.Code,
		"amount": codeObj.Amount,
		"expiry": codeObj.Expiry,
	})
}

// UseCode redeems a code for the user
func UseCode(c *gin.Context) {
	var input struct {
		Code    string `json:"code"`
		Address string `json:"address"`
	}

	if err := c.BindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// Check if code and address are provided
	if input.Code == "" || input.Address == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Code and address are required"})
		return
	}

	// Get code from database
	codeObj, err := database.GetCode(input.Code)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Code not found or invalid"})
		return
	}

	// Check if code is already used
	if codeObj.Used {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Code has already been used"})
		return
	}

	// Check if code is expired
	if codeObj.Expiry != "" {
		expiry, err := time.Parse(time.RFC3339, codeObj.Expiry)
		if err == nil && time.Now().After(expiry) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Code has expired"})
			return
		}
	}

	// Send USDC to the address
	response, err := faucet.SendUSDCWithCode(input.Address, codeObj.Amount, input.Code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": response.Error})
		return
	}

	// Update code status in database
	codeObj.Used = true
	codeObj.Address = input.Address
	codeObj.ClaimedAt = time.Now()

	err = database.UpdateCode(codeObj)
	if err != nil {
		fmt.Printf("Warning: Transaction sent but failed to update code status: %v\n", err)
	}

	// Save transaction to database
	err = database.SaveFaucetTransaction(input.Address, codeObj.Amount, response.TxHash, "USDC")
	if err != nil {
		// Log the error but don't fail the request
		fmt.Printf("Warning: Transaction sent but failed to save to database: %v\n", err)
	}

	c.JSON(http.StatusOK, response)
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
