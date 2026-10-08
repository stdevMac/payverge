package agents

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/guardrails"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupOpsServiceTestDB(t *testing.T) {
	t.Helper()
	gormDB, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:ops-svc-%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(gormDB)
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.OpsAssistantThread{},
		&database.OpsAssistantMessage{},
		&database.OpsAssistantToolCall{},
	))
}

type blockingOffTopicClassifier struct{}

func (blockingOffTopicClassifier) Classify(context.Context, guardrails.ClassifyRequest) (guardrails.Verdict, error) {
	return guardrails.Verdict{Allowed: false, Category: guardrails.CategoryOffTopic, Reason: "test"}, nil
}

func TestOpsAsk_MenuFromBillsUsesDeterministicGuide(t *testing.T) {
	setupOpsServiceTestDB(t)
	biz := &database.Business{
		BusinessId: "ops-menu", Name: "Cafe", OwnerAddress: "0x", SettlementAddr: "0x1", TippingAddr: "0x2",
		IsActive: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, database.GetDB().Create(biz).Error)

	svc := NewOpsAssistantService(nil, NewRegistry(), database.GetDBWrapper(), nil)
	// No model required for deterministic path.
	result, err := svc.Ask(context.Background(), OpsAskRequest{
		BusinessID: biz.ID,
		Message:    "How do I add menu items?",
		Locale:     "en",
		ActiveTab:  "bills",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Contains(t, result.Response.Answer, "Menu")
	require.NotEmpty(t, result.Response.Steps)
	require.NotEmpty(t, result.Response.Actions)
	assert.Contains(t, result.Response.Actions[0].Href, "tab=menu")
	assert.Equal(t, "deterministic-guide", result.Usage.Model)
}

func TestOpsAsk_SupportIntentSpanish(t *testing.T) {
	setupOpsServiceTestDB(t)
	biz := &database.Business{
		BusinessId: "ops-sup", Name: "Cafe", OwnerAddress: "0x", SettlementAddr: "0x1", TippingAddr: "0x2",
		IsActive: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, database.GetDB().Create(biz).Error)

	svc := NewOpsAssistantService(nil, NewRegistry(), database.GetDBWrapper(), nil)
	result, err := svc.Ask(context.Background(), OpsAskRequest{
		BusinessID: biz.ID,
		Message:    "conectame con soporte",
		Locale:     "es",
		ActiveTab:  "bills",
	})
	require.NoError(t, err)
	assert.Equal(t, "deterministic-guide", result.Usage.Model)
	assert.Contains(t, stringsToLower(result.Response.Answer), "soporte")
}

func TestOpsAsk_GuestOrderDescribesDinerUINotSetup(t *testing.T) {
	setupOpsServiceTestDB(t)
	biz := &database.Business{
		BusinessId: "ops-guest-order", Name: "Cafe", OwnerAddress: "0x", SettlementAddr: "0x1", TippingAddr: "0x2",
		IsActive: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, database.GetDB().Create(biz).Error)

	svc := NewOpsAssistantService(nil, NewRegistry(), database.GetDBWrapper(), nil)
	result, err := svc.Ask(context.Background(), OpsAskRequest{
		BusinessID: biz.ID,
		Message:    "How do guests order?",
		Locale:     "en",
		ActiveTab:  "overview",
	})
	require.NoError(t, err)
	require.Equal(t, "deterministic-guide", result.Usage.Model)
	assert.Contains(t, stringsToLower(result.Response.Answer), "digital menu")
	assert.Contains(t, stringsToLower(result.Response.Answer), "scan")
	assert.NotContains(t, stringsToLower(result.Response.Answer), "setup progress")
	assert.NotContains(t, stringsToLower(result.Response.Answer), "incomplete setup cards")
}

func TestOpsAsk_TurnOnAIWaiterDescribesDinerUIOnce(t *testing.T) {
	setupOpsServiceTestDB(t)
	biz := &database.Business{
		BusinessId: "ops-diner-ui", Name: "Cafe", OwnerAddress: "0x", SettlementAddr: "0x1", TippingAddr: "0x2",
		IsActive: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, database.GetDB().Create(biz).Error)

	svc := NewOpsAssistantService(nil, NewRegistry(), database.GetDBWrapper(), nil)
	result, err := svc.Ask(context.Background(), OpsAskRequest{
		BusinessID: biz.ID,
		Message:    "How do I turn on the AI Waiter… what will diners see?",
		Locale:     "en",
		ActiveTab:  "overview",
	})
	require.NoError(t, err)
	require.Equal(t, "deterministic-guide", result.Usage.Model)
	lower := stringsToLower(result.Response.Answer + " " + result.ResponseV2.Answer.Content)
	assert.Contains(t, lower, "qr")
	assert.Contains(t, lower, "sage")
	assert.NotContains(t, lower, "setup progress")
	assert.NotContains(t, lower, "incomplete setup")
	assert.NotContains(t, lower, "set name and priority")
	assert.Zero(t, strings.Count(lower, "configure ai waiter name"),
		"configure-name-priority copy must not appear")
	// #874: a lone guide is the answer itself, so its identity rides on the
	// source/action instead of a section wrapping a copy of the same text.
	assert.Empty(t, result.ResponseV2.Sections)
	require.Len(t, result.ResponseV2.Sources, 1)
	assert.Equal(t, "guide:guest-order-experience", result.ResponseV2.Sources[0].ID)
}

func TestOpsAsk_OffTopicReturnsStructuredRecoveryNotError(t *testing.T) {
	setupOpsServiceTestDB(t)
	biz := &database.Business{
		BusinessId: "ops-off", Name: "Cafe", OwnerAddress: "0x", SettlementAddr: "0x1", TippingAddr: "0x2",
		IsActive: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	require.NoError(t, database.GetDB().Create(biz).Error)

	svc := NewOpsAssistantService(nil, NewRegistry(), database.GetDBWrapper(), nil).
		WithClassifier(blockingOffTopicClassifier{})
	result, err := svc.Ask(context.Background(), OpsAskRequest{
		BusinessID: biz.ID,
		Message:    "what is the capital of france?",
		Locale:     "en",
		ActiveTab:  "overview",
	})
	require.NoError(t, err, "off_topic must not become an internal error")
	require.NotNil(t, result)
	assert.NotEmpty(t, result.Response.Answer)
	assert.Equal(t, "scope-recovery", result.Usage.Model)
}

func stringsToLower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}
