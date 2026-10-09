package database

import (
	"fmt"

	"gorm.io/gorm"
)

// AIBusinessPurgeResult reports per-surface deletion counts for closure audits.
type AIBusinessPurgeResult struct {
	WaiterConversations int64
	WaiterMessages      int64
	OpsThreads          int64
	OpsMessages         int64
	OpsToolCalls        int64
	OpsRequests         int64
	DirectorThreads     int64
	DirectorMessages    int64
	DirectorToolCalls   int64
	DirectorProposals   int64
	DirectorAudits      int64
	WizardSessions      int64
	WizardMessages      int64
	ExtractionJobs      int64
	ExtractionImages    int64
	GeneratedImages     int64
}

// PurgeBusinessAIData removes tenant-scoped AI history for a business in
// FK-safe order. It is retry-safe and never deletes rows belonging to another
// business. Only owner/admin permanent closure paths call it; an admin
// suspension must not.
func PurgeBusinessAIData(businessID uint) (AIBusinessPurgeResult, error) {
	res := AIBusinessPurgeResult{}
	if businessID == 0 {
		return res, fmt.Errorf("business id is required")
	}
	if db == nil {
		return res, gorm.ErrInvalidDB
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		has := func(model any) bool {
			return tx.Migrator().HasTable(model)
		}
		// Waiter — skip surfaces whose tables are not migrated yet (minimal test DBs).
		var convIDs []uint
		if !has(&AiWaiterConversation{}) {
			// skip
		} else if err := tx.Model(&AiWaiterConversation{}).Where("business_id = ?", businessID).Pluck("id", &convIDs).Error; err != nil {
			if !isMissingTable(err) {
				return err
			}
		} else if len(convIDs) > 0 {
			r := tx.Where("conversation_id IN ?", convIDs).Delete(&AiWaiterMessage{})
			if r.Error != nil {
				return r.Error
			}
			res.WaiterMessages = r.RowsAffected
			r = tx.Where("id IN ?", convIDs).Delete(&AiWaiterConversation{})
			if r.Error != nil {
				return r.Error
			}
			res.WaiterConversations = r.RowsAffected
		}

		// Ops
		var opsIDs []uint
		if !has(&OpsAssistantThread{}) {
			// skip
		} else if err := tx.Model(&OpsAssistantThread{}).Where("business_id = ?", businessID).Pluck("id", &opsIDs).Error; err != nil {
			if !isMissingTable(err) {
				return err
			}
		} else if len(opsIDs) > 0 {
			r := tx.Where("thread_id IN ? AND business_id = ?", opsIDs, businessID).Delete(&OpsAssistantToolCall{})
			if r.Error != nil {
				return r.Error
			}
			res.OpsToolCalls = r.RowsAffected
			r = tx.Where("thread_id IN ? AND business_id = ?", opsIDs, businessID).Delete(&OpsAssistantMessage{})
			if r.Error != nil {
				return r.Error
			}
			res.OpsMessages = r.RowsAffected
			r = tx.Where("thread_id IN ? AND business_id = ?", opsIDs, businessID).Delete(&OpsAssistantRequest{})
			if r.Error != nil {
				return r.Error
			}
			res.OpsRequests = r.RowsAffected
			r = tx.Where("id IN ? AND business_id = ?", opsIDs, businessID).Delete(&OpsAssistantThread{})
			if r.Error != nil {
				return r.Error
			}
			res.OpsThreads = r.RowsAffected
		}

		// Director
		var dirIDs []uint
		if !has(&DirectorConsoleThread{}) {
			// skip
		} else if err := tx.Model(&DirectorConsoleThread{}).Where("business_id = ?", businessID).Pluck("id", &dirIDs).Error; err != nil {
			if !isMissingTable(err) {
				return err
			}
		} else if len(dirIDs) > 0 {
			r := tx.Where("thread_id IN ? AND business_id = ?", dirIDs, businessID).Delete(&DirectorActionAudit{})
			if r.Error != nil && !isMissingTable(r.Error) {
				return r.Error
			}
			res.DirectorAudits = r.RowsAffected
			r = tx.Where("thread_id IN ? AND business_id = ?", dirIDs, businessID).Delete(&DirectorProposedAction{})
			if r.Error != nil && !isMissingTable(r.Error) {
				return r.Error
			}
			res.DirectorProposals = r.RowsAffected
			r = tx.Where("thread_id IN ? AND business_id = ?", dirIDs, businessID).Delete(&DirectorToolCall{})
			if r.Error != nil {
				return r.Error
			}
			res.DirectorToolCalls = r.RowsAffected
			r = tx.Where("thread_id IN ? AND business_id = ?", dirIDs, businessID).Delete(&DirectorConsoleMessage{})
			if r.Error != nil {
				return r.Error
			}
			res.DirectorMessages = r.RowsAffected
			r = tx.Where("id IN ? AND business_id = ?", dirIDs, businessID).Delete(&DirectorConsoleThread{})
			if r.Error != nil {
				return r.Error
			}
			res.DirectorThreads = r.RowsAffected
		}

		// Menu wizard
		var wizIDs []uint
		if !has(&MenuWizardSession{}) {
			// skip
		} else if err := tx.Model(&MenuWizardSession{}).Where("business_id = ?", businessID).Pluck("id", &wizIDs).Error; err != nil {
			if !isMissingTable(err) {
				return err
			}
		} else if len(wizIDs) > 0 {
			r := tx.Where("session_id IN ?", wizIDs).Delete(&MenuWizardMessage{})
			if r.Error != nil {
				return r.Error
			}
			res.WizardMessages = r.RowsAffected
			r = tx.Where("id IN ?", wizIDs).Delete(&MenuWizardSession{})
			if r.Error != nil {
				return r.Error
			}
			res.WizardSessions = r.RowsAffected
		}

		// Extraction
		var jobIDs []uint
		if !has(&MenuExtractionJob{}) {
			// skip
		} else if err := tx.Model(&MenuExtractionJob{}).Where("business_id = ?", businessID).Pluck("id", &jobIDs).Error; err != nil {
			if !isMissingTable(err) {
				return err
			}
		} else if len(jobIDs) > 0 {
			r := tx.Where("job_id IN ?", jobIDs).Delete(&MenuExtractionImage{})
			if r.Error != nil {
				return r.Error
			}
			res.ExtractionImages = r.RowsAffected
			r = tx.Where("id IN ?", jobIDs).Delete(&MenuExtractionJob{})
			if r.Error != nil {
				return r.Error
			}
			res.ExtractionJobs = r.RowsAffected
		}

		// Image provenance
		if has(&AIGeneratedImage{}) {
			r := tx.Where("business_id = ?", businessID).Delete(&AIGeneratedImage{})
			if r.Error != nil && !isMissingTable(r.Error) {
				return r.Error
			}
			res.GeneratedImages = r.RowsAffected
		}

		return nil
	})
	return res, err
}
