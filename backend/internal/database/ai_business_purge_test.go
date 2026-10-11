package database

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupPurgeDB(t *testing.T) *gorm.DB {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, gormDB.AutoMigrate(
		&AiWaiterConversation{},
		&AiWaiterMessage{},
		&OpsAssistantThread{},
		&OpsAssistantMessage{},
		&OpsAssistantToolCall{},
		&OpsAssistantRequest{},
		&DirectorConsoleThread{},
		&DirectorConsoleMessage{},
		&DirectorToolCall{},
		&DirectorProposedAction{},
		&DirectorActionAudit{},
		&MenuWizardSession{},
		&MenuWizardMessage{},
		&MenuExtractionJob{},
		&MenuExtractionImage{},
		&AIGeneratedImage{},
	))
	SetTestDB(gormDB)
	return gormDB
}

func TestPurgeBusinessAIData_TenantIsolation(t *testing.T) {
	setupPurgeDB(t)

	// Business 1 data
	conv := AiWaiterConversation{SessionID: "s1", BusinessID: 1, TableCode: "T1", Mode: "ordering", Status: "active"}
	require.NoError(t, db.Create(&conv).Error)
	require.NoError(t, db.Create(&AiWaiterMessage{ConversationID: conv.ID, Role: "user", Content: "hi"}).Error)

	// Business 2 data must survive
	conv2 := AiWaiterConversation{SessionID: "s2", BusinessID: 2, TableCode: "T2", Mode: "ordering", Status: "active"}
	require.NoError(t, db.Create(&conv2).Error)
	require.NoError(t, db.Create(&AiWaiterMessage{ConversationID: conv2.ID, Role: "user", Content: "keep"}).Error)

	ops := &OpsAssistantThread{BusinessID: 1, Title: "help", Locale: "en", LastMessageAt: time.Now()}
	require.NoError(t, db.Create(ops).Error)

	res, err := PurgeBusinessAIData(1)
	require.NoError(t, err)
	require.Equal(t, int64(1), res.WaiterConversations)
	require.Equal(t, int64(1), res.WaiterMessages)
	require.Equal(t, int64(1), res.OpsThreads)

	var remaining int64
	require.NoError(t, db.Model(&AiWaiterConversation{}).Where("business_id = ?", 2).Count(&remaining).Error)
	require.Equal(t, int64(1), remaining)
	require.NoError(t, db.Model(&AiWaiterMessage{}).Count(&remaining).Error)
	require.Equal(t, int64(1), remaining)
}

func TestPurgeBusinessAIData_RejectsZero(t *testing.T) {
	setupPurgeDB(t)
	_, err := PurgeBusinessAIData(0)
	require.Error(t, err)
}
