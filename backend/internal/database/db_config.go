package database

import (
	"log"
	"os"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type DbConfig struct {
	DatabasePath string `json:"database_path"`
}

func NewConfig(databasePath string) *DbConfig {
	return &DbConfig{
		DatabasePath: databasePath,
	}
}

var (
	db *gorm.DB
)

func InitDB(config *DbConfig) {
	// Ensure the directory exists
	dir := filepath.Dir(config.DatabasePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatal("Failed to create database directory:", err)
	}

	// Open SQLite database
	database, err := gorm.Open(sqlite.Open(config.DatabasePath), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	db = database

	// Auto-migrate all models
	if err := autoMigrate(); err != nil {
		log.Fatal("Failed to migrate database:", err)
	}

	log.Println("Database connected and migrated successfully")
}

// GetDB returns the GORM database instance
func GetDB() *gorm.DB {
	return db
}

// autoMigrate creates all necessary tables
func autoMigrate() error {
	return db.AutoMigrate(
		&User{},
		&Code{},
		&ErrorLog{},
		&FaucetTransaction{},
		&Subscriber{},
		&MultisigTx{},
	)
}
