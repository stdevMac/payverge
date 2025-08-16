package server

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/stdevMac/shares/internal/s3"
	"net/http"
	"path/filepath"
	"strings"
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
