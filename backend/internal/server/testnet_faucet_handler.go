package server

import (
	"net/http"

	"payverge/internal/faucet"

	"github.com/gin-gonic/gin"
)

// TestnetFaucetCheck checks if an address is eligible for testnet faucet
func TestnetFaucetCheck(c *gin.Context) {
	// Only allow on testnets
	if !isTestnet() {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Testnet faucet is only available on test networks",
		})
		return
	}

	address := c.Param("address")
	if address == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Address parameter is required",
		})
		return
	}

	response, err := faucet.CheckTestnetEligibility(address)
	if err != nil {
		c.JSON(http.StatusInternalServerError, response)
		return
	}

	c.JSON(http.StatusOK, response)
}

// TestnetFaucetTopUp sends testnet ETH and USDC to an address
func TestnetFaucetTopUp(c *gin.Context) {
	// Only allow on testnets
	if !isTestnet() {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "Testnet faucet is only available on test networks",
		})
		return
	}

	var input struct {
		Address string `json:"address" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request format",
		})
		return
	}

	response, err := faucet.TestnetFaucetTopUp(input.Address)
	if err != nil {
		// The error is already formatted in the response
		c.JSON(http.StatusInternalServerError, response)
		return
	}

	c.JSON(http.StatusOK, response)
}

// isTestnet checks if we're running on a testnet based on environment
func isTestnet() bool {
	// chainID := os.Getenv("CHAIN_ID")
	// Base Sepolia = 84532, Ethereum Sepolia = 11155111
	// return chainID == "84532" || chainID == "11155111"
	// TODO: update this to check for testnet
	return true
}
