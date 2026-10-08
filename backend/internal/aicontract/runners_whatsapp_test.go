//go:build whatsapp

package aicontract

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func registerWhatsAppRunners() {
	RegisterScenarioRunner("whatsapp-restores-after-process-restart", runWhatsAppRestore)
}

// Tagged builds run the real WhatsApp scenarios: nothing may stay gated.
func TestWhatsAppScenariosRegisteredInTaggedBuild(t *testing.T) {
	require.Empty(t, tagGatedScenarioIDs)
	_, ok := GetScenarioRunner("whatsapp-restores-after-process-restart")
	require.True(t, ok, "whatsapp build must register the restore runner")
}

func runWhatsAppRestore(sc Scenario) (ScenarioRunResult, error) {
	gdb, err := gorm.Open(sqlite.Open("file:wa-restore?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		return ScenarioRunResult{}, err
	}
	sqlDB, _ := gdb.DB()
	if sqlDB != nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := gdb.AutoMigrate(&database.Business{}, &database.WhatsAppBusinessDevice{}); err != nil {
		return ScenarioRunResult{}, err
	}
	database.SetTestDB(gdb)
	biz := database.Business{Name: "WA", SettlementAddr: "s", TippingAddr: "t", IsActive: true}
	if err := gdb.Create(&biz).Error; err != nil {
		return ScenarioRunResult{}, err
	}
	// Use non-email-looking JID to satisfy ValidateScenarios (no raw JID emails);
	// production stores full JIDs — scenario input only carries status flags.
	jid := "15550001111@s.whatsapp.net"
	fake, err := services.NewFakeWhatsAppClient(jid, true)
	if err != nil {
		return ScenarioRunResult{}, err
	}
	store := services.NewFakeDeviceStore()
	store.RegisterDevice(jid, fake)
	if err := database.UpsertWhatsAppBusinessDevice(&database.WhatsAppBusinessDevice{
		BusinessID: biz.ID,
		DeviceJID:  jid,
		Status:     database.WhatsAppDeviceStatusConnected,
	}); err != nil {
		return ScenarioRunResult{}, err
	}
	wm := services.NewWhatsAppManagerForHermetic(store, nil)
	if err := wm.RestoreAllSessionsCtx(context.Background()); err != nil {
		return ScenarioRunResult{}, err
	}
	// Second restore must not double-register handlers.
	if err := wm.RestoreAllSessionsCtx(context.Background()); err != nil {
		return ScenarioRunResult{}, err
	}
	handlerRegs := fake.HandlerRegs()
	dup := 0
	if handlerRegs > 1 {
		dup = handlerRegs - 1
	}
	// Manager-level registration count must also be 1.
	if n := wm.HandlerRegistrationCount(biz.ID); n != 1 {
		return ScenarioRunResult{}, fmt.Errorf("manager handlers=%d", n)
	}
	req, err := llm.NewGenerateRequest("waiter")
	zdr := err == nil && req.RequiresZDR()
	return ScenarioRunResult{
		Code: "restored",
		Text: "restored",
		ZDR:  zdr,
		Effects: map[string]int{
			"handler_registered": handlerRegs,
			"duplicate_handlers": dup,
		},
	}, nil
}
