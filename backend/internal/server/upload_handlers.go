package server

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"payverge/internal/s3"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"crypto/rand"
	"encoding/hex"
)

// UploadFile handles file uploads to S3
func UploadFile(c *gin.Context) {
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
	location, err := s3.UploadFile(file, fileName, folderPath)
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

// generateUniqueFilename creates a unique filename with timestamp and random string
func generateUniqueFilename(originalFilename string) string {
	ext := filepath.Ext(originalFilename)
	name := strings.TrimSuffix(originalFilename, ext)
	
	// Generate random bytes
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	randomString := hex.EncodeToString(randomBytes)
	
	// Create timestamp
	timestamp := time.Now().Format("20060102_150405")
	
	return fmt.Sprintf("%s_%s_%s%s", name, timestamp, randomString, ext)
}

// UploadFileLocal handles file uploads to local filesystem
func UploadFileLocal(c *gin.Context) {
	// Get the file from form data
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file provided"})
		return
	}

	// Validate file size (max 10MB)
	const maxFileSize = 10 << 20 // 10MB
	if file.Size > maxFileSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File size too large. Maximum 10MB allowed"})
		return
	}

	// Validate file type (images only for now)
	allowedTypes := map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".gif":  true,
		".webp": true,
		".svg":  true,
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !allowedTypes[ext] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file type. Only images are allowed"})
		return
	}

	// Generate unique filename
	fileName := generateUniqueFilename(file.Filename)

	// Create upload directory if it doesn't exist (same location as database)
	uploadDir := "./uploads"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create upload directory"})
		return
	}

	// Full file path
	filePath := filepath.Join(uploadDir, fileName)

	// Save the file
	if err := c.SaveUploadedFile(file, filePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save file"})
		return
	}

	// Generate URL for accessing the file
	fileURL := fmt.Sprintf("/api/v1/files/%s", fileName)

	c.JSON(http.StatusOK, gin.H{
		"url":      fileURL,
		"filename": fileName,
		"size":     file.Size,
		"type":     ext,
	})
}

// ServeUploadedFile serves files from the local uploads directory
func ServeUploadedFile(c *gin.Context) {
	filename := c.Param("filename")

	// Clean filename to prevent directory traversal
	filename = filepath.Clean(filename)
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file path"})
		return
	}

	// Construct file path
	filePath := filepath.Join("./uploads", filename)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": "File not found"})
		return
	}

	// Serve the file
	c.File(filePath)
}
