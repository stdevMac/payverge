package database

import (
	"embed"
	"fmt"
	"strings"
)

//go:embed *.sql
var migrationFiles embed.FS

// ApplyIndexes applies database indexes for performance optimization
func (db *DB) ApplyIndexes() error {
	indexSQL, err := migrationFiles.ReadFile("indexes.sql")
	if err != nil {
		return fmt.Errorf("failed to read indexes.sql: %w", err)
	}

	// Split SQL statements by semicolon and execute each
	statements := strings.Split(string(indexSQL), ";")
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}

		if err := db.conn.Exec(stmt).Error; err != nil {
			// Log warning but don't fail if index already exists
			fmt.Printf("Warning: Failed to create index: %v\n", err)
		}
	}

	return nil
}

// OptimizeDatabase performs database optimization tasks
func (db *DB) OptimizeDatabase() error {
	// Apply indexes
	if err := db.ApplyIndexes(); err != nil {
		return fmt.Errorf("failed to apply indexes: %w", err)
	}

	// Analyze tables for query optimization
	tables := []string{"bills", "payments", "businesses", "tables"}
	for _, table := range tables {
		if err := db.conn.Exec(fmt.Sprintf("ANALYZE %s", table)).Error; err != nil {
			fmt.Printf("Warning: Failed to analyze table %s: %v\n", table, err)
		}
	}

	return nil
}
