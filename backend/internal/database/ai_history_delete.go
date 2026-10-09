package database

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrAIHistoryNotFound is returned when a thread/session is missing or belongs
// to another business (tenant isolation).
var ErrAIHistoryNotFound = errors.New("ai history not found")

// PermanentlyDeleteDirectorConsoleThread removes a Director thread and children.
// Director remains owner-only at the handler layer; this is the data path only.
func PermanentlyDeleteDirectorConsoleThread(businessID, threadID uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var thread DirectorConsoleThread
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND business_id = ?", threadID, businessID).
			First(&thread).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAIHistoryNotFound
			}
			return err
		}
		// Child tables: tool calls then messages then thread. Tolerate minimal test
		// DBs that have not migrated optional child tables.
		if err := tx.Where("thread_id = ? AND business_id = ?", threadID, businessID).
			Delete(&DirectorToolCall{}).Error; err != nil && !isMissingTable(err) {
			return fmt.Errorf("delete director tool calls: %w", err)
		}
		if err := tx.Where("thread_id = ? AND business_id = ?", threadID, businessID).
			Delete(&DirectorConsoleMessage{}).Error; err != nil && !isMissingTable(err) {
			return fmt.Errorf("delete director messages: %w", err)
		}
		if err := tx.Where("thread_id = ? AND business_id = ?", threadID, businessID).
			Delete(&DirectorActionAudit{}).Error; err != nil && !isMissingTable(err) {
			return fmt.Errorf("delete director action audits: %w", err)
		}
		if err := tx.Where("thread_id = ? AND business_id = ?", threadID, businessID).
			Delete(&DirectorProposedAction{}).Error; err != nil && !isMissingTable(err) {
			return fmt.Errorf("delete director proposals: %w", err)
		}
		if err := tx.Where("id = ? AND business_id = ?", threadID, businessID).
			Delete(&DirectorConsoleThread{}).Error; err != nil {
			return fmt.Errorf("delete director thread: %w", err)
		}
		return nil
	})
}

func isMissingTable(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return containsAny(msg, "no such table", "does not exist")
}

func containsAny(s string, parts ...string) bool {
	for _, p := range parts {
		if len(p) > 0 && (len(s) >= len(p)) {
			for i := 0; i+len(p) <= len(s); i++ {
				if s[i:i+len(p)] == p {
					return true
				}
			}
		}
	}
	return false
}
