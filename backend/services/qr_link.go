package services

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"image/color"
	"os"
	"path/filepath"

	"github.com/skip2/go-qrcode"
)

type QRLinkService struct {
	baseURL    string
	staticDir  string
	publicURL  string
}

func NewQRLinkService() *QRLinkService {
	return &QRLinkService{
		baseURL:   getEnvOrDefault("BASE_URL", "http://localhost:8080"),
		staticDir: getEnvOrDefault("STATIC_DIR", "./static"),
		publicURL: getEnvOrDefault("PUBLIC_URL", "http://localhost:8080/static"),
	}
}

func (qls *QRLinkService) GeneratePaymentLink(invoiceID uint64) string {
	return fmt.Sprintf("%s/pay/%d", qls.baseURL, invoiceID)
}

func (qls *QRLinkService) GenerateShortLink(invoiceID uint64) (string, error) {
	// Generate short code for the link (for future database storage)
	_ = qls.generateShortCode()

	// In a production environment, you would store this mapping in the database
	// For now, we'll use the invoice ID directly
	return fmt.Sprintf("%s/p/%d", qls.baseURL, invoiceID), nil
}

func (qls *QRLinkService) GenerateQRCode(paymentLink string, invoiceID uint64) (string, error) {
	// Ensure static directory exists
	if err := os.MkdirAll(qls.staticDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create static directory: %v", err)
	}

	// Generate QR code
	qrCode, err := qrcode.New(paymentLink, qrcode.Medium)
	if err != nil {
		return "", fmt.Errorf("failed to create QR code: %v", err)
	}

	// Set QR code styling
	qrCode.ForegroundColor = color.RGBA{0, 0, 0, 255}     // Black
	qrCode.BackgroundColor = color.RGBA{255, 255, 255, 255} // White background

	// Generate filename
	filename := fmt.Sprintf("qr_%d.png", invoiceID)
	filepath := filepath.Join(qls.staticDir, filename)

	// Save QR code as PNG
	err = qrCode.WriteFile(256, filepath)
	if err != nil {
		return "", fmt.Errorf("failed to save QR code: %v", err)
	}

	// Return public URL
	publicURL := fmt.Sprintf("%s/%s", qls.publicURL, filename)
	return publicURL, nil
}

func (qls *QRLinkService) generateShortCode() string {
	// Generate a random 6-character code
	bytes := make([]byte, 4)
	rand.Read(bytes)
	return base64.URLEncoding.EncodeToString(bytes)[:6]
}
