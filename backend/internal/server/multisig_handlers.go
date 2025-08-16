package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"web3-boilerplate/internal/database"
	"go.mongodb.org/mongo-driver/mongo"
)

// GetMultisigTx handles the GET request for multisig transaction information
func GetMultisigTx(c *gin.Context) {
	data, err := database.GetMultisigTx()
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{"error": "No transaction data found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve transaction data"})
		return
	}

	c.JSON(http.StatusOK, data)
}

// PutMultisigTx handles the PUT request to store multisig transaction data
func PutMultisigTx(c *gin.Context) {
	var data map[string]interface{}
	if err := c.ShouldBindJSON(&data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if err := database.StoreMultisigTx(data); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store transaction data"})
		return
	}

	c.JSON(http.StatusOK, data)
}

// DeleteMultisigTx handles the DELETE request for multisig transaction
func DeleteMultisigTx(c *gin.Context) {
	data, err := database.GetMultisigTx()
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{"error": "No transaction data found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve transaction data"})
		return
	}

	data["txId"] = "-1"

	if err := database.StoreMultisigTx(data); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update transaction data"})
		return
	}

	c.JSON(http.StatusOK, data)
}

type patchMultisigTxRequest struct {
	TxID string `json:"txId" binding:"required"`
}

// PatchMultisigTx handles the PATCH request to update the txId
func PatchMultisigTx(c *gin.Context) {
	var req patchMultisigTxRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	data, err := database.GetMultisigTx()
	if err != nil {
		if err == mongo.ErrNoDocuments {
			c.JSON(http.StatusNotFound, gin.H{"error": "No transaction data found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve transaction data"})
		return
	}

	data["txId"] = req.TxID

	if err := database.StoreMultisigTx(data); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update transaction data"})
		return
	}

	c.JSON(http.StatusOK, data)
}
