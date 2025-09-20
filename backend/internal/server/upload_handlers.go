package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"path/filepath"
	"payverge/internal/database"
	"payverge/internal/s3"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// UploadFile handles file uploads to S3
func UploadFile(c *gin.Context) {
	// Get the file from form data
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file provided"})
		return
	}

	// Get user address from JWT token (set by authentication middleware)
	userAddress, exists := c.Get("address")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Get all businesses owned by the user
	businesses, err := database.GetBusinessByOwnerAddress(userAddress.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve user businesses"})
		return
	}

	if len(businesses) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "No businesses found for this user"})
		return
	}

	var business *database.Business

	// Try to get business ID from various sources
	businessID := c.Param("id") // URL parameter (if route is /business/:id/upload)
	if businessID == "" {
		businessID = c.Query("business_id") // Query parameter
	}
	if businessID == "" {
		businessID = c.PostForm("business_id") // Form data
	}

	if businessID != "" {
		// Business ID provided, find and verify it
		businessIDUint, err := strconv.ParseUint(businessID, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid business ID"})
			return
		}

		// Find the business in user's businesses
		for i := range businesses {
			if businesses[i].ID == uint(businessIDUint) {
				business = &businesses[i]
				break
			}
		}

		if business == nil {
			c.JSON(http.StatusForbidden, gin.H{"error": "Business not found or you don't have permission"})
			return
		}
	} else {
		// No business ID provided, use the first (and possibly only) business
		if len(businesses) > 1 {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Multiple businesses found. Please specify business_id parameter",
				"businesses": businesses,
			})
			return
		}
		business = &businesses[0]
	}

	// Generate random filename to prevent conflicts
	fileName := generateUniqueFilename(file.Filename)

	// Create folder path with business ID
	folderPath := fmt.Sprintf("businesses/%d", business.ID)

	// Allow custom subfolder within business folder
	customFolder := c.PostForm("folder")
	if customFolder != "" {
		// Clean the custom folder path
		customFolder = strings.Trim(customFolder, "/")
		if customFolder != "" {
			folderPath = fmt.Sprintf("%s/%s", folderPath, customFolder)
		}
	}

	// Upload file to S3
	location, err := s3.UploadFile(file, fileName, folderPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"location":    location,
		"filename":    fileName,
		"folder":      folderPath,
		"business_id": business.ID,
	})
}

// UploadFileProtected handles file uploads to S3
func UploadFileProtected(c *gin.Context) {
	// Get the file from form data
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file provided"})
		return
	}

	// Get the folder path from form data (optional)
	folderPath := c.PostForm("folder")

	// Get or generate the file name
	fileName := c.PostForm("name")
	if fileName == "" {
		// If no name provided, use the original filename
		fileName = file.Filename
	}

	// Clean the filename to prevent directory traversal and ensure safe characters
	fileName = filepath.Base(fileName)
	if fileName == "." || fileName == "" || strings.Contains(fileName, "/") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid filename"})
		return
	}

	// Add file extension if not present
	if !strings.Contains(fileName, ".") {
		originalExt := filepath.Ext(file.Filename)
		if originalExt != "" {
			fileName = fmt.Sprintf("%s%s", fileName, originalExt)
		}
	}

	// Upload file to S3
	location, err := s3.UploadFileProtected(file, fileName, folderPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"location": location,
		"filename": fileName,
		"folder":   folderPath,
	})
}

// generateUniqueFilename creates a unique filename with random string and timestamp
func generateUniqueFilename(originalFilename string) string {
	ext := filepath.Ext(originalFilename)

	// Generate random bytes for unique identifier
	randomBytes := make([]byte, 8) // 16 character hex string
	rand.Read(randomBytes)
	randomString := hex.EncodeToString(randomBytes)

	// Create timestamp
	timestamp := time.Now().Format("20060102_150405")

	// Create filename: random_timestamp.ext
	return fmt.Sprintf("%s_%s%s", randomString, timestamp, ext)
}
