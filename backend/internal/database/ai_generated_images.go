package database

import (
	"errors"
	"time"
)

// AIGeneratedImage is the provenance record written on every successful AI
// menu-image generation/enhancement. It is the database half of the EU AI Act
// Art. 50 machine-readable marking requirement (the other half is the S3 object
// metadata x-amz-meta-ai-generated=true set at upload).
type AIGeneratedImage struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	BusinessID uint      `gorm:"index;not null" json:"business_id"`
	S3Key      string    `gorm:"type:text;not null" json:"s3_key"`
	Source     string    `gorm:"type:varchar(32);not null" json:"source"` // "generate" | "regenerate" | "enhance"
	Model      string    `gorm:"type:varchar(128)" json:"model"`
	CreatedAt  time.Time `gorm:"index" json:"created_at"`
}

func (AIGeneratedImage) TableName() string { return "ai_generated_images" }

// RecordAIGeneratedImage persists one provenance row. Best-effort at the call
// site (a failed record must not fail the user's generation), but the function
// itself returns the error for logging.
func RecordAIGeneratedImage(businessID uint, s3Key, source, model string) error {
	if businessID == 0 {
		return errors.New("business required")
	}
	return GetDB().Create(&AIGeneratedImage{
		BusinessID: businessID,
		S3Key:      s3Key,
		Source:     source,
		Model:      model,
		CreatedAt:  time.Now(),
	}).Error
}
