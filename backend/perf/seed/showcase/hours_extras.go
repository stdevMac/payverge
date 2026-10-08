package main

import (
	"context"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// seedOperatingHours wires a typical week: closed Monday, lunch + dinner
// Tuesday-Saturday, brunch + dinner Sunday. Idempotent — checks existing
// rows per (business_id, day_of_week) before inserting.
func seedOperatingHours(ctx context.Context, db *gorm.DB, bizID uint) error {
	type row struct {
		Day   int
		Open  string
		Close string
		Off   bool
	}
	week := []row{
		{Day: 0, Open: "10:00", Close: "21:00"}, // Sunday
		{Day: 1, Off: true},                     // Monday — closed
		{Day: 2, Open: "11:30", Close: "22:00"}, // Tuesday
		{Day: 3, Open: "11:30", Close: "22:00"}, // Wednesday
		{Day: 4, Open: "11:30", Close: "22:30"}, // Thursday
		{Day: 5, Open: "11:30", Close: "23:00"}, // Friday
		{Day: 6, Open: "10:00", Close: "23:00"}, // Saturday
	}

	for _, r := range week {
		var existing database.BusinessOperatingHours
		err := db.WithContext(ctx).
			Where("business_id = ? AND day_of_week = ?", bizID, r.Day).
			First(&existing).Error
		if err == nil {
			continue
		}
		if err != gorm.ErrRecordNotFound {
			return err
		}
		rec := database.BusinessOperatingHours{
			BusinessID: bizID,
			DayOfWeek:  r.Day,
			OpenTime:   r.Open,
			CloseTime:  r.Close,
			IsClosed:   r.Off,
		}
		if err := db.WithContext(ctx).Create(&rec).Error; err != nil {
			return err
		}
	}
	return nil
}

// seedGallery inserts a small set of curated gallery placeholders. The
// frontend reads `image_url` directly, so the URLs here can be replaced by
// the operator from the dashboard once real photos are uploaded. Idempotent
// on (business_id, image_url).
func seedGallery(ctx context.Context, db *gorm.DB, bizID uint) error {
	images := []struct {
		URL     string
		Caption string
	}{
		{"https://images.unsplash.com/photo-1551183053-bf91a1d81141?w=1200", "Wood-fired oven at the back of the dining room"},
		{"https://images.unsplash.com/photo-1414235077428-338989a2e8c0?w=1200", "Patio seating with bay views"},
		{"https://images.unsplash.com/photo-1555396273-367ea4eb4db5?w=1200", "House-made tagliatelle resting before service"},
		{"https://images.unsplash.com/photo-1601315379734-425765a06fc4?w=1200", "The bar at golden hour"},
	}
	for i, img := range images {
		var existing database.BusinessGalleryImage
		err := db.WithContext(ctx).
			Where("business_id = ? AND image_url = ?", bizID, img.URL).
			First(&existing).Error
		if err == nil {
			continue
		}
		if err != gorm.ErrRecordNotFound {
			return err
		}
		rec := database.BusinessGalleryImage{
			BusinessID:   bizID,
			ImageURL:     img.URL,
			Caption:      img.Caption,
			DisplayOrder: i,
			IsActive:     true,
		}
		if err := db.WithContext(ctx).Create(&rec).Error; err != nil {
			return err
		}
	}
	return nil
}

// seedSpecialFeatures adds the "amenities" tags that show on the public
// business page. Idempotent on (business_id, title).
func seedSpecialFeatures(ctx context.Context, db *gorm.DB, bizID uint) error {
	features := []struct {
		Title       string
		Description string
		Icon        string
	}{
		{"Outdoor seating", "Heated patio with bay views, dog-friendly", "Sun"},
		{"Private dining", "Up to 18 guests in our wine cellar room", "Users"},
		{"Wine pairing", "200+ Italian labels, sommelier on Fri/Sat", "Wine"},
		{"Family-style menu", "Sunday Tuscan supper, second seating 8pm", "Soup"},
		{"Gluten-free pasta", "House-made daily, ask your server", "Wheat"},
	}
	for i, f := range features {
		var existing database.BusinessSpecialFeature
		err := db.WithContext(ctx).
			Where("business_id = ? AND title = ?", bizID, f.Title).
			First(&existing).Error
		if err == nil {
			continue
		}
		if err != gorm.ErrRecordNotFound {
			return err
		}
		rec := database.BusinessSpecialFeature{
			BusinessID:   bizID,
			Title:        f.Title,
			Description:  f.Description,
			Icon:         f.Icon,
			DisplayOrder: i,
			IsActive:     true,
		}
		if err := db.WithContext(ctx).Create(&rec).Error; err != nil {
			return err
		}
	}
	return nil
}
