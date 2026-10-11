package database

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// WhatsApp device lifecycle statuses.
const (
	WhatsAppDeviceStatusPairing      = "pairing"
	WhatsAppDeviceStatusConnecting   = "connecting"
	WhatsAppDeviceStatusConnected    = "connected"
	WhatsAppDeviceStatusDegraded     = "degraded"
	WhatsAppDeviceStatusDisconnected = "disconnected"
)

// WhatsAppBusinessDevice maps one business to one WhatsApp device JID with
// restart-safe lifecycle metadata.
type WhatsAppBusinessDevice struct {
	ID                uint       `gorm:"primaryKey" json:"id"`
	BusinessID        uint       `gorm:"column:business_id;uniqueIndex;not null" json:"business_id"`
	DeviceJID         string     `gorm:"column:device_jid;type:text;uniqueIndex;not null" json:"device_jid"`
	Status            string     `gorm:"column:status;size:24;not null;default:'disconnected'" json:"status"`
	LastErrorCode     string     `gorm:"column:last_error_code;size:64" json:"last_error_code,omitempty"`
	LastConnectedAt   *time.Time `gorm:"column:last_connected_at" json:"last_connected_at,omitempty"`
	ReconnectAttempts int64      `gorm:"column:reconnect_attempts" json:"reconnect_attempts"`
	NextRetryAt       *time.Time `gorm:"column:next_retry_at" json:"next_retry_at,omitempty"`
	CreatedAt         time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt         time.Time  `gorm:"column:updated_at" json:"updated_at"`
}

func (WhatsAppBusinessDevice) TableName() string { return "whatsapp_business_devices" }

var validWhatsAppDeviceStatuses = map[string]struct{}{
	WhatsAppDeviceStatusPairing:      {},
	WhatsAppDeviceStatusConnecting:   {},
	WhatsAppDeviceStatusConnected:    {},
	WhatsAppDeviceStatusDegraded:     {},
	WhatsAppDeviceStatusDisconnected: {},
}

// UpsertWhatsAppBusinessDevice creates or updates the business's device mapping.
// DeviceJID must be non-empty; blank JIDs are rejected so provisional pairings
// never persist. Implemented as find-then-create/update so SQLite unit tests and
// Postgres production share one path (avoids dialect-specific ON CONFLICT quirks).
func UpsertWhatsAppBusinessDevice(device *WhatsAppBusinessDevice) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	if device == nil || device.BusinessID == 0 {
		return fmt.Errorf("business_id is required")
	}
	if device.DeviceJID == "" {
		return fmt.Errorf("device_jid is required")
	}
	if device.Status == "" {
		device.Status = WhatsAppDeviceStatusDisconnected
	}
	if _, ok := validWhatsAppDeviceStatuses[device.Status]; !ok {
		return fmt.Errorf("invalid device status %q", device.Status)
	}
	now := time.Now().UTC()
	device.UpdatedAt = now

	var existing WhatsAppBusinessDevice
	err := db.Where("business_id = ?", device.BusinessID).First(&existing).Error
	if err == nil {
		device.ID = existing.ID
		device.CreatedAt = existing.CreatedAt
		return db.Model(&existing).Updates(map[string]interface{}{
			"device_jid":         device.DeviceJID,
			"status":             device.Status,
			"last_error_code":    device.LastErrorCode,
			"last_connected_at":  device.LastConnectedAt,
			"reconnect_attempts": device.ReconnectAttempts,
			"next_retry_at":      device.NextRetryAt,
			"updated_at":         now,
		}).Error
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}
	device.CreatedAt = now
	return db.Create(device).Error
}

// GetWhatsAppBusinessDevice loads the mapping for a business.
func GetWhatsAppBusinessDevice(businessID uint) (*WhatsAppBusinessDevice, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	var device WhatsAppBusinessDevice
	if err := db.Where("business_id = ?", businessID).First(&device).Error; err != nil {
		return nil, err
	}
	return &device, nil
}

// ListWhatsAppBusinessDevicesForRestore returns mappings that should be restored
// or retried: connected/connecting/degraded/pairing, or disconnected with a due
// next_retry_at.
func ListWhatsAppBusinessDevicesForRestore(now time.Time, limit int) ([]WhatsAppBusinessDevice, error) {
	if db == nil {
		return nil, gorm.ErrInvalidDB
	}
	if limit <= 0 {
		limit = 100
	}
	var devices []WhatsAppBusinessDevice
	err := db.Where(
		`status IN ? OR (status = ? AND next_retry_at IS NOT NULL AND next_retry_at <= ?)`,
		[]string{
			WhatsAppDeviceStatusPairing,
			WhatsAppDeviceStatusConnecting,
			WhatsAppDeviceStatusConnected,
			WhatsAppDeviceStatusDegraded,
		},
		WhatsAppDeviceStatusDisconnected,
		now,
	).Order("id ASC").Limit(limit).Find(&devices).Error
	return devices, err
}

// MarkWhatsAppDeviceStatus updates lifecycle fields for a business-scoped device.
func MarkWhatsAppDeviceStatus(businessID uint, status, errorCode string, connectedAt, nextRetry *time.Time, attempts *int64) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	if _, ok := validWhatsAppDeviceStatuses[status]; !ok {
		return fmt.Errorf("invalid device status %q", status)
	}
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": time.Now().UTC(),
	}
	if errorCode != "" || status == WhatsAppDeviceStatusConnected {
		updates["last_error_code"] = errorCode
	}
	if connectedAt != nil {
		updates["last_connected_at"] = connectedAt
	}
	if nextRetry != nil {
		updates["next_retry_at"] = nextRetry
	} else if status == WhatsAppDeviceStatusConnected {
		updates["next_retry_at"] = nil
	}
	if attempts != nil {
		updates["reconnect_attempts"] = *attempts
	}
	result := db.Model(&WhatsAppBusinessDevice{}).
		Where("business_id = ?", businessID).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// DeleteWhatsAppBusinessDevice removes the mapping for a business.
func DeleteWhatsAppBusinessDevice(businessID uint) error {
	if db == nil {
		return gorm.ErrInvalidDB
	}
	return db.Where("business_id = ?", businessID).Delete(&WhatsAppBusinessDevice{}).Error
}
