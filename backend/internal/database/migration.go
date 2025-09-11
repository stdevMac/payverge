package database

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
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

		if _, err := db.db.Exec(stmt); err != nil {
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
		if _, err := db.db.Exec(fmt.Sprintf("ANALYZE %s", table)); err != nil {
			fmt.Printf("Warning: Failed to analyze table %s: %v\n", table, err)
		}
	}

	return nil
}
