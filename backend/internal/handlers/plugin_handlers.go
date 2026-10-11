package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/locales"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/money"
	"github.com/stdevmac/payverge/backend/internal/plugins"
	mercadopago "github.com/stdevmac/payverge/backend/internal/plugins/mercadopago"
	"github.com/stdevmac/payverge/backend/internal/security"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services"
	"github.com/stdevmac/payverge/backend/internal/services/paidreceipt"
	"github.com/stdevmac/payverge/backend/internal/utils"
)

// PluginHandlers handles plugin-related HTTP requests
type PluginHandlers struct {
	pluginService   *services.PluginService
	reportScheduler *services.ReportScheduler
	// WebSocket removed - using polling instead
}

// verifiedWebhookPayloadEvidence is the only webhook body evidence persisted
// after signature verification. Provider retries and authoritative status
// reconciliation are the recovery mechanisms; raw bodies may contain payment
// and guest data and do not belong in the long-lived idempotency ledger.
func verifiedWebhookPayloadEvidence(payload []byte) string {
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:])
}

// resolveBusinessID resolves a business identifier (numeric ID or slug) to a numeric business ID.
func resolveBusinessID(identifier string) (uint, error) {
	business, err := database.GetBusinessByIdOrBusinessId(identifier)
	if err != nil {
		return 0, err
	}
	return business.ID, nil
}

var guestBillPaymentPluginSupported = func(pluginName string) bool {
	switch strings.ToLower(strings.TrimSpace(pluginName)) {
	case "stripe", "paypal", "mercadopago":
		return true
	default:
		return false
	}
}

var guestPaymentOptionVisible = func(pluginName string) bool {
	switch strings.ToLower(strings.TrimSpace(pluginName)) {
	case "stripe", "paypal", "mercadopago", "usdc_payment":
		return true
	case "cross_chain_payment":
		// Hidden while the rail cannot be payer-bound; see
		// guestCrossChainSettlementEnabled.
		return guestCrossChainSettlementEnabled
	default:
		return false
	}
}

// evalPaymentPluginConfigEnabled applies the guest payment-plugin "enabled"
// gate to an already-loaded config map: an explicit config["enabled"] (bool or
// parseable string) wins; when absent, every payment plugin defaults on. This
// is the single source of truth shared by the per-plugin
// guestPaymentPluginConfigured path and the batched guest listing loader, so
// both stay byte-identical.
func evalPaymentPluginConfigEnabled(pluginName string, config map[string]interface{}) bool {
	if !guestCardPluginHasCredentialShape(pluginName, config) {
		return false
	}
	if value, exists := config["enabled"]; exists {
		switch typed := value.(type) {
		case bool:
			return typed
		case string:
			if parsed, err := strconv.ParseBool(strings.TrimSpace(typed)); err == nil {
				return parsed
			}
		}
	}

	return true
}

// configFieldIsEncryptedSecret reports whether one of the charge credentials
// is already ciphertext. GetBusinessPaymentPluginConfigs does not decrypt, so
// guest listing sees v1: values instead of TEST-/sk_live_ prefixes. Do not
// decrypt here just to re-check those prefixes. Webhook/signing secrets are
// ignored — they cannot create a guest charge.
func configFieldIsEncryptedSecret(config map[string]interface{}, keys ...string) bool {
	for _, key := range keys {
		value := stringFromConfigMap(config, key)
		if strings.HasPrefix(value, "v1:") && len(value) > 3 {
			return true
		}
		alias := stringFromConfigMap(config, key+"_encrypted")
		if strings.HasPrefix(alias, "v1:") && len(alias) > 3 {
			return true
		}
	}
	return false
}

func guestCardPluginHasCredentialShape(pluginName string, config map[string]interface{}) bool {
	name := strings.ToLower(strings.TrimSpace(pluginName))
	switch name {
	case "mercadopago":
		if configFieldIsEncryptedSecret(config, "access_token") {
			return true
		}
		token := stringFromConfigMap(config, "access_token")
		return (strings.HasPrefix(token, "TEST-") || strings.HasPrefix(token, "APP_USR-")) && len(token) > 20
	case "paypal":
		if configFieldIsEncryptedSecret(config, "access_token", "client_secret") {
			return true
		}
		return stringFromConfigMap(config, "client_id") != "" ||
			len(stringFromConfigMap(config, "access_token")) > 10
	case "stripe":
		if configFieldIsEncryptedSecret(config, "access_token", "secret_key") {
			return true
		}
		token := stringFromConfigMap(config, "access_token")
		if token == "" {
			token = stringFromConfigMap(config, "secret_key")
		}
		return strings.HasPrefix(token, "sk_test_") || strings.HasPrefix(token, "sk_live_")
	default:
		return true
	}
}

func stringFromConfigMap(config map[string]interface{}, key string) string {
	if config == nil {
		return ""
	}
	value, ok := config[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func guestPaymentPluginConfigured(businessID uint, pluginName string) (bool, error) {
	enabled, err := database.IsPluginEnabledForBusiness(businessID, pluginName)
	if err != nil || !enabled {
		return false, err
	}

	config, err := database.GetBusinessPluginConfig(businessID, pluginName)
	if err != nil {
		if errors.Is(err, database.ErrBusinessPluginNotEnabled) {
			return false, nil
		}
		return false, err
	}

	return evalPaymentPluginConfigEnabled(pluginName, config), nil
}

// enqueueReceiptForPaidBill routes every settlement path through the paid-bill
// receipt domain operation. Failures are logged, never propagated: printing
// must not block payment settlement.
func enqueueReceiptForPaidBill(bill *database.Bill) {
	if bill == nil {
		return
	}
	db := database.GetDB()
	if db == nil {
		log.Printf("enqueue receipt print job for bill %d skipped: DB unavailable", bill.ID)
		return
	}
	if _, err := paidreceipt.NewService(db).HandleBillPaid(context.Background(), bill, "payment_settlement"); err != nil {
		log.Printf("enqueue receipt print job for bill %d failed: %v", bill.ID, err)
	}
}

var paymentWebhookProviderSupported = func(pluginName string) bool {
	if guestBillPaymentPluginSupported(pluginName) {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(pluginName)) {
	case "stripe", "paypal", "mercadopago":
		return true
	default:
		return false
	}
}

func truthyScanValue(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case []byte:
		normalized := strings.TrimSpace(strings.ToLower(string(typed)))
		return normalized == "1" || normalized == "true" || normalized == "t"
	case string:
		normalized := strings.TrimSpace(strings.ToLower(typed))
		return normalized == "1" || normalized == "true" || normalized == "t"
	default:
		return false
	}
}

func loadBusinessPluginRecord(businessID, pluginID uint) (*database.BusinessPlugin, error) {
	var businessPlugin database.BusinessPlugin
	if err := database.GetDB().
		Preload("Plugin").
		Where("business_id = ? AND plugin_id = ?", businessID, pluginID).
		First(&businessPlugin).Error; err != nil {
		return nil, err
	}
	return &businessPlugin, nil
}

func publicBusinessPluginRecord(pluginName string, businessPlugin *database.BusinessPlugin) *database.BusinessPlugin {
	if businessPlugin == nil {
		return nil
	}

	var config map[string]interface{}
	rawConfig := strings.TrimSpace(businessPlugin.Config)
	if rawConfig == "" {
		rawConfig = "{}"
	}
	if err := json.Unmarshal([]byte(rawConfig), &config); err != nil {
		// Unparseable config: fail closed rather than echoing the raw blob.
		clone := *businessPlugin
		clone.Config = "{}"
		return &clone
	}

	marshaled, err := json.Marshal(publicPluginConfig(pluginName, config))
	if err != nil {
		clone := *businessPlugin
		clone.Config = "{}"
		return &clone
	}

	clone := *businessPlugin
	clone.Config = string(marshaled)
	return &clone
}

// publicPluginConfig returns a browser-safe view of a plugin's config. A plugin
// that implements PublicConfigProvider controls its own masking; every other
// plugin gets the fail-closed generic mask, so payment-processor credentials
// (Stripe secret_key, PayPal/MercadoPago secrets, Telegram bot_token, webhook
// signing secrets) are never returned raw — even to owner staff. (SEC-1)
func publicPluginConfig(pluginName string, config map[string]interface{}) map[string]interface{} {
	if plugin, exists := plugins.GetPluginByName(pluginName); exists {
		if provider, ok := plugin.(plugins.PublicConfigProvider); ok {
			return provider.PublicConfig(config)
		}
	}
	return maskSecretConfig(config)
}

// pluginManagesOwnSecrets reports whether a plugin masks its own config (and so
// owns the save-side preservation of unchanged secrets). Only such plugins are
// exempt from the generic mask/restore.
func pluginManagesOwnSecrets(pluginName string) bool {
	if plugin, exists := plugins.GetPluginByName(pluginName); exists {
		if _, ok := plugin.(plugins.PublicConfigProvider); ok {
			return true
		}
	}
	return false
}

// maskedSecretValue is the placeholder returned in place of a stored secret.
// The save path treats an incoming value equal to this sentinel (or empty/
// missing) as "unchanged" and restores the stored credential.
const maskedSecretValue = "••••••••"

// maskSecretConfig returns a copy of config with every secret-bearing value
// replaced by maskedSecretValue. Non-secret keys pass through untouched.
// Uses security.IsSecretConfigKey so mask/restore stay aligned with encrypt-at-rest
// (including webhook_secret_previous / *_secret_previous rotation keys).
func maskSecretConfig(config map[string]interface{}) map[string]interface{} {
	if config == nil {
		return nil
	}
	out := make(map[string]interface{}, len(config))
	for k, v := range config {
		if !security.IsSecretConfigKey(k) {
			out[k] = v
			continue
		}
		if s, ok := v.(string); ok {
			if s == "" {
				out[k] = ""
			} else {
				out[k] = maskedSecretValue
			}
			continue
		}
		if v == nil {
			out[k] = nil
		} else {
			out[k] = maskedSecretValue
		}
	}
	return out
}

// restoreMaskedSecrets merges incoming config over existing, preserving each
// stored secret whenever the incoming value is missing, empty, or the mask
// sentinel — so a save that round-trips a masked secret can't wipe the real
// credential, while a genuinely new secret is still saved.
func restoreMaskedSecrets(incoming, existing map[string]interface{}) map[string]interface{} {
	if incoming == nil || existing == nil {
		return incoming
	}
	for k, ev := range existing {
		if !security.IsSecretConfigKey(k) {
			continue
		}
		iv, present := incoming[k]
		if !present {
			incoming[k] = ev
			continue
		}
		if s, ok := iv.(string); ok && (s == "" || s == maskedSecretValue) {
			incoming[k] = ev
		}
	}
	return incoming
}

func (ph *PluginHandlers) validatePluginConfiguration(pluginName string, config map[string]interface{}) error {
	if plugin, exists := plugins.GetPluginByName(pluginName); exists {
		return plugin.ValidateConfig(config)
	}
	if ph.pluginService == nil {
		return nil
	}
	return ph.pluginService.ValidatePluginConfig(pluginName, config)
}

func (ph *PluginHandlers) normalizePluginConfiguration(businessID uint, pluginName string, config map[string]interface{}) (map[string]interface{}, error) {
	plugin, exists := plugins.GetPluginByName(pluginName)
	if !exists {
		return config, nil
	}

	normalizer, ok := plugin.(plugins.ConfigNormalizer)
	if !ok {
		return config, nil
	}

	return normalizer.NormalizeConfig(businessID, config)
}

func (ph *PluginHandlers) initializePluginConfiguration(businessID uint, pluginName string, config map[string]interface{}) error {
	if err := ph.syncReportSchedule(businessID, pluginName, config); err != nil {
		return err
	}

	plugin, exists := plugins.GetPluginByName(pluginName)
	if !exists {
		return nil
	}
	return plugin.Initialize(businessID, config)
}

func (ph *PluginHandlers) cleanupPluginConfiguration(businessID uint, pluginName string) error {
	if err := ph.disableReportSchedule(businessID, pluginName); err != nil {
		return err
	}

	plugin, exists := plugins.GetPluginByName(pluginName)
	if !exists {
		return nil
	}
	return plugin.Cleanup(businessID)
}

func (ph *PluginHandlers) syncReportSchedule(businessID uint, pluginName string, config map[string]interface{}) error {
	if ph.reportScheduler == nil {
		if isReportPlugin(pluginName) {
			return fmt.Errorf("report scheduler unavailable")
		}
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(pluginName)) {
	case "weekly_email_report":
		return ph.reportScheduler.SyncWeeklySchedule(businessID, config)
	case "daily_email_report":
		return ph.reportScheduler.SyncDailySchedule(businessID, config)
	default:
		return nil
	}
}

func (ph *PluginHandlers) disableReportSchedule(businessID uint, pluginName string) error {
	if ph.reportScheduler == nil {
		if isReportPlugin(pluginName) {
			return fmt.Errorf("report scheduler unavailable")
		}
		return nil
	}

	switch strings.ToLower(strings.TrimSpace(pluginName)) {
	case "weekly_email_report":
		return ph.reportScheduler.DisableWeeklySchedule(businessID)
	case "daily_email_report":
		return ph.reportScheduler.DisableDailySchedule(businessID)
	default:
		return nil
	}
}

func isReportPlugin(pluginName string) bool {
	switch strings.ToLower(strings.TrimSpace(pluginName)) {
	case "weekly_email_report", "daily_email_report":
		return true
	default:
		return false
	}
}

func publicBillPaymentUnavailableMessage() string {
	return "Public bill payments are not available for this plugin"
}

var errPluginCheckoutBillFullyPaid = errors.New("bill is already fully paid")

func authoritativeGuestPluginChargeAmount(bill *database.Bill, tipAmountCents int64) (int64, error) {
	if tipAmountCents < 0 {
		return 0, database.ErrInvalidTipAmount
	}
	if bill.Status == database.BillStatusClosed || bill.Status == database.BillStatusPaid {
		return 0, database.ErrBillNotPayable
	}

	baseAmount := bill.TotalAmount - bill.PaidAmount
	if baseAmount <= 0 {
		return 0, errPluginCheckoutBillFullyPaid
	}

	total := baseAmount + tipAmountCents
	if total <= 0 {
		return 0, errPluginCheckoutBillFullyPaid
	}

	return total, nil
}

func splitShareIDFromMetadata(metadata map[string]interface{}) (uint, bool) {
	if metadata == nil {
		return 0, false
	}
	shareID, ok := uintFromAny(metadata["split_share_id"])
	return shareID, ok && shareID > 0
}

func pluginTrackerParticipantName(pluginName string, splitShareID *uint) string {
	if splitShareID == nil || *splitShareID == 0 {
		return pluginName
	}
	return fmt.Sprintf("%s|split_share_id=%d", pluginName, *splitShareID)
}

func splitShareIDFromPluginTrackerName(value string) (uint, bool) {
	for _, part := range strings.Split(value, "|") {
		key, raw, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found || key != "split_share_id" {
			continue
		}
		return uintFromAny(raw)
	}
	return 0, false
}

func pluginTrackerNameHasProviderTrackerID(value string, trackerID string) bool {
	trackerID = strings.TrimSpace(trackerID)
	if trackerID == "" {
		return false
	}
	for _, part := range strings.Split(value, "|") {
		key, raw, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found || key != "provider_tracker_id" {
			continue
		}
		if strings.TrimSpace(raw) == trackerID {
			return true
		}
	}
	return false
}

func pluginTrackerNameWithProviderTrackerID(value string, trackerID string) string {
	trackerID = strings.TrimSpace(trackerID)
	if trackerID == "" || pluginTrackerNameHasProviderTrackerID(value, trackerID) {
		return value
	}
	if strings.TrimSpace(value) == "" {
		return "provider_tracker_id=" + trackerID
	}
	return value + "|provider_tracker_id=" + trackerID
}

// currencyCanRepresentCents reports whether a stored-cents amount survives a
// round-trip through the provider minor-unit boundary for the given currency.
// For zero-decimal currencies (JPY, KRW, CLP, …) a non-whole-major amount is
// lossily rounded and cannot be charged/settled exactly; for 2- and 3-decimal
// currencies the round-trip is exact by construction. COP is ISO 2-decimal in
// storage but Mercado Pago Orders/Checkout Pro treat it as whole units, so it
// is treated as zero-decimal here. A zero amount (e.g. no tip) is always
// representable.
func currencyCanRepresentCents(cents int64, currency string) bool {
	if cents == 0 {
		return true
	}
	code := strings.ToUpper(strings.TrimSpace(currency))
	// MP zero-decimal (incl. COP): only whole major units are representable.
	if code == "COP" || money.DecimalDigits(code) == 0 {
		return cents%100 == 0
	}
	return money.FromMinorUnits(money.MinorUnits(cents, currency), currency) == cents
}

func authoritativeGuestPluginCurrency(business *database.Business) string {
	currency := strings.ToUpper(strings.TrimSpace(business.DefaultCurrency))
	if currency == "" {
		return "USD"
	}
	return currency
}

// NewPluginHandlers creates a new plugin handlers instance
func NewPluginHandlers(pluginService *services.PluginService, reportScheduler *services.ReportScheduler) *PluginHandlers {
	return &PluginHandlers{
		pluginService:   pluginService,
		reportScheduler: reportScheduler,
	}
}

// Admin Plugin Management Endpoints

// CreatePlugin creates a new plugin (admin only)
func (ph *PluginHandlers) CreatePlugin(c *gin.Context) {
	var plugin database.Plugin
	if err := c.ShouldBindJSON(&plugin); err != nil {
		server.RespondBindError(c, err)
		return
	}

	createdPlugin, err := database.CreatePlugin(plugin)
	if err != nil {
		log.Printf("Failed to create plugin: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Internal server error")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"plugin": createdPlugin})
}

// GetAllPlugins returns all plugins (admin only)
func (ph *PluginHandlers) GetAllPlugins(c *gin.Context) {
	activeOnly := c.Query("active_only") == "true"
	languageCode, ok := canonicalPluginLanguage(c.Query("lang"))
	if !ok {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Unsupported language")
		return
	}

	if languageCode != "" {
		// Get plugins with translations
		plugins, err := database.GetPluginsWithTranslations(activeOnly, languageCode)
		if err != nil {
			log.Printf("Failed to get plugins with translations: %v", err)
			server.RespondWithError(c, http.StatusInternalServerError, "", "Internal server error")
			return
		}
		c.JSON(http.StatusOK, gin.H{"plugins": plugins})
	} else {
		// Get plugins without translations
		plugins, err := database.GetAllPlugins(activeOnly)
		if err != nil {
			log.Printf("Failed to get all plugins: %v", err)
			server.RespondWithError(c, http.StatusInternalServerError, "", "Internal server error")
			return
		}
		c.JSON(http.StatusOK, gin.H{"plugins": plugins})
	}
}

func canonicalPluginLanguage(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", true
	}

	locale, ok := locales.Lookup(trimmed)
	if !ok {
		return "", false
	}

	return locale.Canonical, true
}

// GetPlugin returns a specific plugin by ID (admin only)
func (ph *PluginHandlers) GetPlugin(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid plugin ID")
		return
	}

	plugin, err := database.GetPluginByID(uint(id))
	if err != nil {
		log.Printf("Plugin not found (ID: %d): %v", id, err)
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Plugin not found")
		return
	}

	c.JSON(http.StatusOK, plugin)
}

// UpdatePlugin updates a plugin (admin only)
func (ph *PluginHandlers) UpdatePlugin(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid plugin ID")
		return
	}

	var updates database.Plugin
	var raw map[string]interface{}
	if err := c.ShouldBindJSON(&raw); err != nil {
		server.RespondBindError(c, err)
		return
	}
	rawJSON, err := json.Marshal(raw)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid request body")
		return
	}
	if err := json.Unmarshal(rawJSON, &updates); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid request body")
		return
	}

	updatedPlugin, err := database.UpdatePlugin(uint(id), updates)
	if err != nil {
		log.Printf("Failed to update plugin %d: %v", id, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Internal server error")
		return
	}

	if _, touched := raw["is_active"]; touched {
		if err := database.GetDB().Model(&database.Plugin{}).Where("id = ?", id).
			Update("admin_active_override", true).Error; err != nil {
			log.Printf("Failed to mark plugin %d admin override: %v", id, err)
		}
	}

	// If message was updated, automatically create/update translations
	if updates.Message != "" {
		ph.updatePluginTranslations(uint(id), updates.Message)
	}

	c.JSON(http.StatusOK, gin.H{"plugin": updatedPlugin})
}

// TogglePluginActive toggles the active status of a plugin (admin only)
func (ph *PluginHandlers) TogglePluginActive(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid plugin ID")
		return
	}

	updatedPlugin, err := database.TogglePluginActive(uint(id))
	if err != nil {
		log.Printf("Failed to toggle plugin active %d: %v", id, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Internal server error")
		return
	}

	c.JSON(http.StatusOK, gin.H{"plugin": updatedPlugin})
}

// DeactivatePlugin deactivates a plugin (admin only) - plugins cannot be deleted
func (ph *PluginHandlers) DeactivatePlugin(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid plugin ID")
		return
	}

	// Get the plugin first
	plugin, err := database.GetPluginByID(uint(id))
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Plugin not found")
		return
	}

	// Update the plugin to set is_active to false. UpdatePlugin uses GORM's
	// Updates(struct), which silently SKIPS is_active=false (a zero value), so the
	// old call was a no-op and the admin Deactivate button did nothing.
	// UpdatePluginForSync writes is_active via an explicit Select so false actually
	// persists (the other Selected columns are re-written with their current
	// values, an idempotent no-op).
	plugin.IsActive = false
	updatedPlugin, err := database.UpdatePluginForSync(uint(id), *plugin)
	if err != nil {
		log.Printf("Failed to deactivate plugin %d: %v", id, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Internal server error")
		return
	}
	if err := database.GetDB().Model(&database.Plugin{}).Where("id = ?", id).
		Update("admin_active_override", true).Error; err != nil {
		log.Printf("Failed to mark plugin %d admin override: %v", id, err)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Plugin deactivated successfully",
		"plugin":  updatedPlugin,
	})
}

// Business Plugin Management Endpoints

// GetBusinessPlugins returns all plugins for a business with their status
func (ph *PluginHandlers) GetBusinessPlugins(c *gin.Context) {
	businessID, err := resolveBusinessID(c.Param("id"))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Business not found")
			return
		}
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	plugins, err := database.GetBusinessPlugins(businessID)
	if err != nil {
		log.Printf("Failed to get business plugins for business %d: %v", businessID, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Internal server error")
		return
	}
	sortOperatorMarketplacePlugins(plugins)

	// #795: expose the enabled payment-rail count server-side so dashboards
	// derive payment-readiness from backend truth instead of hardcoding it.
	// Computed from the rows already in memory — no extra query.
	enabledPaymentCount := 0
	for _, row := range plugins {
		if fmt.Sprint(row["category"]) == database.PluginCategoryPayment &&
			database.BusinessPluginRowEnabled(row) {
			enabledPaymentCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"plugins":               plugins,
		"enabled_payment_count": enabledPaymentCount,
		"payments_ready":        enabledPaymentCount > 0,
	})
}

// EnableBusinessPlugin enables a plugin for a business
func (ph *PluginHandlers) EnableBusinessPlugin(c *gin.Context) {
	businessID, err := resolveBusinessID(c.Param("id"))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Business not found")
			return
		}
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	pluginIDStr := c.Param("plugin_id")
	pluginID, err := strconv.ParseUint(pluginIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid plugin ID")
		return
	}

	var request struct {
		Config map[string]interface{} `json:"config"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		server.RespondBindError(c, err)
		return
	}

	// Validate plugin configuration
	plugin, err := database.GetPluginByID(uint(pluginID))
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Plugin not found")
		return
	}

	// Preserve stored secrets when the client round-trips a masked/blank secret.
	// Enabling an already-configured plugin (e.g. re-enable after a disable, or a
	// save that re-submits the masked ••••••••  sentinel) must not overwrite the
	// real stored secret with the mask. This mirrors UpdateBusinessPluginConfig's
	// restore (SEC-1); without it the enable round-trip silently corrupts creds.
	if !pluginManagesOwnSecrets(plugin.Name) {
		if existing, exErr := database.GetBusinessPluginConfig(businessID, plugin.Name); exErr == nil {
			request.Config = restoreMaskedSecrets(request.Config, existing)
		}
	}

	normalizedConfig, err := ph.normalizePluginConfiguration(businessID, plugin.Name, request.Config)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}
	request.Config = normalizedConfig

	if err := ph.validatePluginConfiguration(plugin.Name, request.Config); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}
	if plugin.ComingSoon {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Plugin is coming soon")
		return
	}

	if err := ph.initializePluginConfiguration(businessID, plugin.Name, request.Config); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}

	if err := database.EnableBusinessPlugin(businessID, uint(pluginID), request.Config); err != nil {
		log.Printf("Failed to enable plugin %d for business %d: %v", pluginID, businessID, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Internal server error")
		return
	}
	// Guest pages read plugin state (Trustpilot) from a short-lived cache.
	services.InvalidatePublicGuestBusiness(businessID)

	businessPlugin, err := loadBusinessPluginRecord(businessID, uint(pluginID))
	if err != nil {
		log.Printf("Failed to load enabled plugin %d for business %d: %v", pluginID, businessID, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Internal server error")
		return
	}

	resp := gin.H{
		"message":         "Plugin enabled successfully",
		"business_plugin": publicBusinessPluginRecord(plugin.Name, businessPlugin),
	}
	if plugin.Name == "telegram" && !services.PluginDeliveryEnabled("telegram") {
		resp["delivery_available"] = false
		resp["warning"] = "Telegram notifications are unavailable because the bot token is not configured"
	}
	c.JSON(http.StatusOK, resp)
}

// DisableBusinessPlugin disables a plugin for a business
func (ph *PluginHandlers) DisableBusinessPlugin(c *gin.Context) {
	businessID, err := resolveBusinessID(c.Param("id"))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Business not found")
			return
		}
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	pluginIDStr := c.Param("plugin_id")
	pluginID, err := strconv.ParseUint(pluginIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid plugin ID")
		return
	}

	plugin, err := database.GetPluginByID(uint(pluginID))
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Plugin not found")
		return
	}

	if err := ph.cleanupPluginConfiguration(businessID, plugin.Name); err != nil {
		log.Printf("Plugin cleanup failed for business %d, plugin %s: %v", businessID, plugin.Name, err)
	}

	if err := database.DisableBusinessPlugin(businessID, uint(pluginID)); err != nil {
		log.Printf("Failed to disable plugin %d for business %d: %v", pluginID, businessID, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Internal server error")
		return
	}
	// Guest pages read plugin state (Trustpilot) from a short-lived cache.
	services.InvalidatePublicGuestBusiness(businessID)

	c.JSON(http.StatusOK, gin.H{"message": "Plugin disabled successfully"})
}

// UpdateBusinessPluginConfig updates the configuration for a business plugin
func (ph *PluginHandlers) UpdateBusinessPluginConfig(c *gin.Context) {
	businessID, err := resolveBusinessID(c.Param("id"))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Business not found")
			return
		}
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	pluginIDStr := c.Param("plugin_id")
	pluginID, err := strconv.ParseUint(pluginIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid plugin ID")
		return
	}

	var request struct {
		Config map[string]interface{} `json:"config"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		server.RespondBindError(c, err)
		return
	}

	// Get plugin name for validation
	plugin, err := database.GetPluginByID(uint(pluginID))
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Plugin not found")
		return
	}

	// Preserve stored secrets when the client round-trips a masked/blank secret
	// (the read paths return secrets masked for plugins without their own
	// masking, so the saved form re-submits the sentinel, not the real value). (SEC-1)
	if !pluginManagesOwnSecrets(plugin.Name) {
		if existing, exErr := database.GetBusinessPluginConfig(businessID, plugin.Name); exErr == nil {
			request.Config = restoreMaskedSecrets(request.Config, existing)
		}
	}

	normalizedConfig, err := ph.normalizePluginConfiguration(businessID, plugin.Name, request.Config)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}
	request.Config = normalizedConfig

	// Validate plugin configuration
	if err := ph.validatePluginConfiguration(plugin.Name, request.Config); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}

	if err := ph.initializePluginConfiguration(businessID, plugin.Name, request.Config); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}

	if err := database.UpdateBusinessPluginConfig(businessID, plugin.Name, request.Config); err != nil {
		log.Printf("Failed to update plugin config for business %d, plugin %s: %v", businessID, plugin.Name, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Internal server error")
		return
	}
	// Guest pages read plugin state (Trustpilot) from a short-lived cache.
	services.InvalidatePublicGuestBusiness(businessID)

	businessPlugin, err := loadBusinessPluginRecord(businessID, uint(pluginID))
	if err != nil {
		log.Printf("Failed to load updated plugin %d for business %d: %v", pluginID, businessID, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Internal server error")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "Plugin configuration updated successfully",
		"business_plugin": publicBusinessPluginRecord(plugin.Name, businessPlugin),
	})
}

// GetBusinessPluginConfig returns the configuration for a business plugin
func (ph *PluginHandlers) GetBusinessPluginConfig(c *gin.Context) {
	businessID, err := resolveBusinessID(c.Param("id"))
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Business not found")
			return
		}
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	pluginIDStr := c.Param("plugin_id")
	pluginID, err := strconv.ParseUint(pluginIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid plugin ID")
		return
	}

	// Get plugin name
	plugin, err := database.GetPluginByID(uint(pluginID))
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Plugin not found")
		return
	}

	config, err := database.GetBusinessPluginConfig(businessID, plugin.Name)
	if err != nil {
		if errors.Is(err, database.ErrBusinessPluginNotEnabled) {
			server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Plugin not enabled for this business")
			return
		}
		log.Printf("Failed to get plugin config for business %d, plugin %s: %v", businessID, plugin.Name, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Internal server error")
		return
	}

	// Mask secrets for any plugin that doesn't provide its own PublicConfig.
	// Fail-closed so payment-processor credentials never reach the browser. (SEC-1)
	config = publicPluginConfig(plugin.Name, config)

	c.JSON(http.StatusOK, gin.H{"config": config})
}

// Helper function to update plugin translations
func (ph *PluginHandlers) updatePluginTranslations(pluginID uint, message string) {
	// Create English translation (default)
	err := database.CreateOrUpdatePluginTranslation(pluginID, "en", "message", message)
	if err != nil {
		log.Printf("Failed to create English translation for plugin %d: %v", pluginID, err)
	}

	// Create Spanish translation using translation service
	if translationService := server.GetTranslationService(); translationService != nil && translationService.IsEnabled() {
		translations, err := translationService.TranslateText(message, []string{"es"})
		if err != nil {
			log.Printf("Failed to translate message for plugin %d: %v", pluginID, err)
			return
		}

		if spanishText, ok := translations["es"]; ok {
			err = database.CreateOrUpdatePluginTranslation(pluginID, "es", "message", spanishText)
			if err != nil {
				log.Printf("Failed to create Spanish translation for plugin %d: %v", pluginID, err)
			}
		}
	} else {
		// Fallback: use the same message for Spanish if translation service is not available
		err = database.CreateOrUpdatePluginTranslation(pluginID, "es", "message", message)
		if err != nil {
			log.Printf("Failed to create Spanish fallback translation for plugin %d: %v", pluginID, err)
		}
	}
}

// Guest Payment Plugin Endpoints

// GetBusinessPaymentPlugins returns active payment plugins for a business (public)
func (ph *PluginHandlers) GetBusinessPaymentPlugins(c *gin.Context) {
	businessIDStr := c.Param("business_id")
	if businessIDStr == "" {
		businessIDStr = c.Param("id")
	}
	businessID, err := strconv.ParseUint(businessIDStr, 10, 32)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	// Get enabled payment plugins for this business
	businessPlugins, err := database.GetBusinessPlugins(uint(businessID))
	if err != nil {
		log.Printf("Failed to get business payment plugins for business %d: %v", businessID, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Internal server error")
		return
	}

	// Collect guest-visible payment-plugin candidates (already filtered to
	// enabled + active + payment + guest-visible). GetBusinessPlugins already
	// constrained p.is_active=true, and the is_enabled check below covers
	// bp.is_enabled — so IsPluginEnabledForBusiness is fully redundant here and
	// only the config["enabled"] gate genuinely needs the config blob.
	var candidates []map[string]interface{}
	candidateNames := make([]string, 0, len(businessPlugins))
	for _, bp := range businessPlugins {
		if !truthyScanValue(bp["is_enabled"]) {
			continue
		}
		pluginName := stringFromAny(bp["name"])
		if category, ok := bp["category"].(string); ok && category == "payment" && guestPaymentOptionVisible(pluginName) {
			candidates = append(candidates, bp)
			candidateNames = append(candidateNames, pluginName)
		}
	}

	// One batched config load replaces the per-plugin IsPluginEnabledForBusiness
	// + GetBusinessPluginConfig pair. A DB fault here must surface as 500,
	// consistent with the GetBusinessPlugins error handling above — never show
	// zero payment options on a transient error.
	configs, err := database.GetBusinessPaymentPluginConfigs(uint(businessID), candidateNames)
	if err != nil {
		log.Printf("Failed to load payment plugin configs for business %d: %v", businessID, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Internal server error")
		return
	}

	// Filter for active payment plugins only. Preallocate an empty (non-nil)
	// slice so a venue with no processor rails serializes as `"plugins": []`
	// and never as `"plugins": null` — this list is advertised as a list and
	// clients iterate it directly.
	paymentPlugins := make([]map[string]interface{}, 0, len(candidates))
	for _, bp := range candidates {
		pluginName := stringFromAny(bp["name"])
		// Absent from configs => not enabled/active or malformed config; skip
		// it (matches the old "not configured / config error → skip" path).
		config, ok := configs[pluginName]
		if !ok || !evalPaymentPluginConfigEnabled(pluginName, config) {
			continue
		}

		// This endpoint is public (unauthenticated guest storefront). Project only
		// the guest-safe catalog fields — never the operator-only health columns
		// (last_status/last_error/last_error_at/last_success_at), which would leak
		// internal webhook failure text and operational state to anonymous users.
		paymentPlugins = append(paymentPlugins, guestSafePaymentPluginView(bp))
	}

	sortGuestPaymentPlugins(paymentPlugins)

	// Optional guest locale: merge catalog translations into display_name /
	// description. Unsupported or empty lang degrades to English catalog names
	// (never 400 — this is a public payment-method picker).
	if lang := strings.TrimSpace(c.Query("lang")); lang != "" {
		languageCode, ok := canonicalPluginLanguage(lang)
		if ok && languageCode != "" && languageCode != "en" {
			applyGuestPaymentPluginTranslations(paymentPlugins, languageCode)
		}
	}

	// House cash rail: report an already-open drawer so "pay at counter"
	// is settleable. Never Create() a session from this public guest GET —
	// heal only on operator Current / cash-record (#769 review).
	// A Caja lookup must never 500 this public picker.
	counterReady := false
	if gormDB := database.GetDB(); gormDB != nil {
		session, err := database.FindOpenCashRegisterSessionForBusinessTx(gormDB, uint(businessID))
		if err != nil {
			log.Printf("dinner tender lookup failed for business %d: %v", businessID, err)
		} else {
			counterReady = session != nil
		}
	}

	// A venue with no processor credentials still has one live, settleable
	// rail when the drawer is open. Reporting it only through the boolean left
	// this endpoint's rail list empty, so a client reading `plugins` concluded
	// there was nothing to pay with while cash was being taken at the counter
	// (#894). List it alongside the processor rails; counter_settlement_ready
	// stays for the shipped clients that already gate on it.
	if counterReady {
		paymentPlugins = append(paymentPlugins, guestHouseCounterRailView())
	}

	c.JSON(http.StatusOK, gin.H{
		"plugins":                  paymentPlugins,
		"counter_settlement_ready": counterReady,
	})
}

// guestHouseCounterRailName identifies the venue's own counter/cash rail in the
// guest payment-rail list. It is deliberately not a plugin catalog name: no
// plugin implements it and it must never be routed to the plugin-payment
// endpoint.
const guestHouseCounterRailName = "counter_cash"

// guestHouseCounterRailView describes the house counter rail. It carries no
// plugin_id and no credential material of any kind — the venue settles it at
// its own register. `settlement: "counter"` is the marker clients switch on to
// render it with their own localized cashier affordance instead of a processor
// checkout; there is deliberately no display_name, since this endpoint's
// catalog translations only cover real plugins.
func guestHouseCounterRailView() map[string]interface{} {
	return map[string]interface{}{
		"name":       guestHouseCounterRailName,
		"category":   "payment",
		"is_enabled": true,
		"settlement": "counter",
	}
}

// guestSafePaymentPluginView returns the subset of a business-plugin projection
// that is safe to expose to unauthenticated guests: catalog display fields and
// the enabled flag, but none of the operator health/error columns.
func guestSafePaymentPluginView(bp map[string]interface{}) map[string]interface{} {
	view := map[string]interface{}{
		"is_enabled": true,
	}
	for _, key := range []string{"plugin_id", "name", "display_name", "description", "image", "category", "version"} {
		if value, ok := bp[key]; ok {
			view[key] = value
		}
	}
	return view
}

// guestPaymentPluginRank puts card/local processors first and crypto last so
// guest checkout never door-leads with USDC (#202).
func guestPaymentPluginRank(name string) int {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "mercadopago":
		return 0
	case "paypal":
		return 1
	case "stripe":
		return 2
	case "usdc_payment":
		return 3
	case "cross_chain_payment":
		return 4
	default:
		return 50
	}
}

func sortGuestPaymentPlugins(plugins []map[string]interface{}) {
	sort.SliceStable(plugins, func(i, j int) bool {
		ni := stringFromAny(plugins[i]["name"])
		nj := stringFromAny(plugins[j]["name"])
		ri := guestPaymentPluginRank(ni)
		rj := guestPaymentPluginRank(nj)
		if ri != rj {
			return ri < rj
		}
		return ni < nj
	})
}

// sortOperatorMarketplacePlugins matches the FE MP-first payment order so
// clients that trust API array order never door-lead with crypto (#711).
func sortOperatorMarketplacePlugins(plugins []map[string]interface{}) {
	sort.SliceStable(plugins, func(i, j int) bool {
		ci := strings.ToLower(stringFromAny(plugins[i]["category"]))
		cj := strings.ToLower(stringFromAny(plugins[j]["category"]))
		if ci != cj {
			return ci < cj
		}
		if ci == "payment" && cj == "payment" {
			ri := guestPaymentPluginRank(stringFromAny(plugins[i]["name"]))
			rj := guestPaymentPluginRank(stringFromAny(plugins[j]["name"]))
			if ri != rj {
				return ri < rj
			}
		}
		di := stringFromAny(plugins[i]["display_name"])
		dj := stringFromAny(plugins[j]["display_name"])
		if di != dj {
			return di < dj
		}
		return stringFromAny(plugins[i]["name"]) < stringFromAny(plugins[j]["name"])
	})
}

// applyGuestPaymentPluginTranslations overwrites guest-safe display_name /
// description from PluginTranslation rows for languageCode, with a light base-
// language fallback (es-AR → es) when the regional rows are missing a field.
// English base catalog fields stay when no translation is present. Failures log
// and leave the English projection intact so the payment page still works.
func applyGuestPaymentPluginTranslations(paymentPlugins []map[string]interface{}, languageCode string) {
	if len(paymentPlugins) == 0 || languageCode == "" || languageCode == "en" {
		return
	}

	// Prefer exact locale, then base (es-AR → es). Exact wins per field.
	codes := []string{languageCode}
	if i := strings.IndexByte(languageCode, '-'); i > 0 {
		base := languageCode[:i]
		if base != "" && base != "en" && base != languageCode {
			codes = append(codes, base)
		}
	}

	// pluginID → field_name → content
	byPluginID := make(map[uint]map[string]string)
	// name → field_name → content (fallback when plugin_id scan is awkward)
	byName := make(map[string]map[string]string)

	for _, code := range codes {
		translated, err := database.GetPluginsWithTranslations(true, code)
		if err != nil {
			log.Printf("Failed to load plugin translations for lang %q: %v", code, err)
			continue
		}
		for _, p := range translated {
			idFields := byPluginID[p.ID]
			if idFields == nil {
				idFields = make(map[string]string)
				byPluginID[p.ID] = idFields
			}
			nameFields := byName[p.Name]
			if nameFields == nil {
				nameFields = make(map[string]string)
				byName[p.Name] = nameFields
			}
			for _, tr := range p.Translations {
				if strings.TrimSpace(tr.Content) == "" {
					continue
				}
				// First code (exact locale) wins; later base fills gaps only.
				if _, exists := idFields[tr.FieldName]; !exists {
					idFields[tr.FieldName] = tr.Content
				}
				if _, exists := nameFields[tr.FieldName]; !exists {
					nameFields[tr.FieldName] = tr.Content
				}
			}
		}
	}

	for _, view := range paymentPlugins {
		var fields map[string]string
		if id, ok := uintFromAny(view["plugin_id"]); ok {
			fields = byPluginID[id]
		}
		if fields == nil {
			if name := stringFromAny(view["name"]); name != "" {
				fields = byName[name]
			}
		}
		if fields == nil {
			continue
		}
		if v, ok := fields["display_name"]; ok && v != "" {
			view["display_name"] = v
		}
		if v, ok := fields["description"]; ok && v != "" {
			view["description"] = v
		}
	}
}

// CreatePluginPayment initiates a payment through a plugin (public)
func (ph *PluginHandlers) CreatePluginPayment(c *gin.Context) {
	billNumber := strings.TrimSpace(c.Param("bill_token"))
	if billNumber == "" {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Bill not found")
		return
	}

	var request struct {
		PluginID  string                 `json:"plugin_id" binding:"required"`
		Amount    float64                `json:"amount" binding:"required"`
		Currency  string                 `json:"currency" binding:"required"`
		TipAmount float64                `json:"tip_amount"`
		Metadata  map[string]interface{} `json:"metadata"`
		ReturnURL string                 `json:"return_url"`
		CancelURL string                 `json:"cancel_url"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		server.RespondBindError(c, err)
		return
	}

	if request.TipAmount < 0 {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "tip amount cannot be negative")
		return
	}

	if !guestBillPaymentPluginSupported(request.PluginID) {
		server.RespondWithError(c, http.StatusServiceUnavailable, "", publicBillPaymentUnavailableMessage())
		return
	}

	// Get bill from database by public_token capability (route param still named billNumber locally).
	bill, table, err := database.GetGuestPluginCheckoutBillByToken(billNumber)
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Bill not found")
		return
	}
	if table != nil {
		bill.Table = *table
	}

	// Get business information
	business, err := database.GetGuestPluginCheckoutBusiness(bill.BusinessID)
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Business not found")
		return
	}

	// Admin-lifecycle gate: mirror the native crypto quote path
	// (IssueCryptoQuote), which refuses guest payments for a suspended or
	// closed business. Without this, the two settlement families disagreed.
	// is_active/closed_at ride along on the single
	// GetGuestPluginCheckoutBusiness load above, so this adds no extra query.
	if !database.IsBusinessOperational(business) {
		c.JSON(http.StatusForbidden, gin.H{
			"error": "This business is not currently accepting payments",
			"code":  services.OrderErrCodeBusinessUnavailable,
		})
		return
	}

	pluginRecord, err := database.GetPluginByName(request.PluginID)
	if err != nil || !pluginRecord.IsActive || pluginRecord.Category != database.PluginCategoryPayment {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Payment plugin not found")
		return
	}

	tipAmountCents := dollarsToCents(request.TipAmount)
	var splitShareID *uint
	totalCharge, err := authoritativeGuestPluginChargeAmount(bill, tipAmountCents)
	if parsedShareID, ok := splitShareIDFromMetadata(request.Metadata); ok {
		shareID := parsedShareID
		share, ok := reserveGuestPaymentSplitShare(c, bill, &shareID)
		if !ok {
			return
		}
		splitShareID = &shareID
		totalCharge = share.AmountCents + tipAmountCents
		err = nil
	}
	if err != nil {
		switch {
		case errors.Is(err, database.ErrInvalidTipAmount):
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		case errors.Is(err, database.ErrBillNotPayable):
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "bill is not open")
		case errors.Is(err, errPluginCheckoutBillFullyPaid):
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, err.Error())
		default:
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		}
		return
	}

	// FIX A (zero-decimal representability guard): reject amounts that the
	// business currency cannot charge exactly BEFORE any provider call, so no
	// money ever moves for a charge we could never settle. Zero-decimal
	// currencies (JPY, KRW, CLP, …) have no sub-unit: a stored-cents amount that
	// is not a whole major unit (e.g. a percentage tip, or a split share of
	// ¥333.33 = 33333 cents) is lossily rounded at the provider boundary
	// (money.MinorUnits), so the inbound webhook reconciles to a DIFFERENT amount
	// and settlement fails forever with the guest already charged. USD/EUR (2
	// decimals) and BHD (3 decimals) round-trip exactly and are never rejected.
	guardCurrency := authoritativeGuestPluginCurrency(business)
	billPortionCents := totalCharge - tipAmountCents
	if !currencyCanRepresentCents(billPortionCents, guardCurrency) || !currencyCanRepresentCents(tipAmountCents, guardCurrency) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":    fmt.Sprintf("%s has no sub-unit; amounts must be whole %s", guardCurrency, guardCurrency),
			"code":     "amount_not_representable",
			"currency": guardCurrency,
		})
		return
	}

	// H3: reject a second live plugin intent for the same split share. Two
	// concurrent intents (double-tap, or two providers) that both settle would
	// double-pay the share; the settlement side auto-refunds the loser, but
	// blocking the duplicate intent up front avoids charging the guest twice in
	// the first place. Best-effort pre-check; the settlement-side guard is the
	// race-proof backstop.
	if splitShareID != nil {
		if hasPending, checkErr := hasPendingPluginTrackerForSplitShare(bill.ID, *splitShareID); checkErr != nil {
			log.Printf("Failed to check for existing plugin intent on split share %d (bill %d): %v", *splitShareID, bill.ID, checkErr)
		} else if hasPending {
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "A payment is already in progress for this split share")
			return
		}
	}

	// Get the payment plugin from registry
	plugin, exists := plugins.GetPluginByName(request.PluginID)
	if !exists {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Payment plugin not found")
		return
	}

	paymentPlugin, ok := plugin.(plugins.PaymentPlugin)
	if !ok {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Plugin is not a payment plugin")
		return
	}

	// Check if business has this plugin enabled
	_, err = database.GetBusinessPluginConfig(business.ID, request.PluginID)
	if err != nil {
		if errors.Is(err, database.ErrBusinessPluginNotEnabled) {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Payment plugin not enabled for this business")
			return
		}
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Payment plugin not enabled for this business")
		return
	}

	production := config.IsProductionMode(false)
	successPath := fmt.Sprintf("/t/%s/bill?payment=success&method=%s&bill_number=%s", url.PathEscape(bill.Table.TableCode), url.QueryEscape(request.PluginID), url.QueryEscape(bill.BillNumber))
	cancelPath := fmt.Sprintf("/t/%s/bill?payment=cancelled&method=%s&bill_number=%s", url.PathEscape(bill.Table.TableCode), url.QueryEscape(request.PluginID), url.QueryEscape(bill.BillNumber))
	returnURL, err := config.AbsoluteRedirectURL(successPath, production)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid return URL")
		return
	}
	cancelURL, err := config.AbsoluteRedirectURL(cancelPath, production)
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid cancel URL")
		return
	}
	if strings.TrimSpace(request.ReturnURL) != "" {
		normalized, err := config.AbsoluteRedirectURL(request.ReturnURL, production)
		if err != nil {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid return URL")
			return
		}
		returnURL = normalized
	}
	if strings.TrimSpace(request.CancelURL) != "" {
		normalized, err := config.AbsoluteRedirectURL(request.CancelURL, production)
		if err != nil {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid cancel URL")
			return
		}
		cancelURL = normalized
	}

	// Prepare metadata with bill and business information
	metadata := request.Metadata
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["bill_id"] = bill.ID
	metadata["bill_number"] = bill.BillNumber
	metadata["business_name"] = business.Name
	metadata["return_url"] = returnURL
	metadata["cancel_url"] = cancelURL
	// Public metadata values are in DOLLARS to match the JSON wire contract (see models_json.go).
	// The *_cents keys preserve exact integer values for backend/PSP reconciliation.
	if splitShareID != nil {
		metadata["split_share_id"] = *splitShareID
	}
	metadata["tip_amount"] = request.TipAmount
	metadata["amount"] = centsToDollars(totalCharge)
	metadata["bill_amount"] = centsToDollars(totalCharge - dollarsToCents(request.TipAmount))
	metadata["amount_cents"] = totalCharge
	metadata["tip_amount_cents"] = tipAmountCents
	metadata["bill_amount_cents"] = totalCharge - tipAmountCents

	// Create payment through plugin using CreateBillPayment method
	currency := authoritativeGuestPluginCurrency(business)

	response, err := paymentPlugin.CreateBillPayment(business.ID, bill.ID, totalCharge, currency, metadata)
	if err != nil {
		log.Printf("Failed to create plugin payment for bill %d: %v", bill.ID, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Payment processing failed")
		return
	}

	tracker, hasTracker := paymentPlugin.(plugins.AlternativePaymentTracker)
	trackedAmountCents := totalCharge
	if hasTracker {
		trackedAmountCents, err = pluginPaymentGrossAmountCents(totalCharge, response.Metadata)
		if err != nil {
			log.Printf("Failed to validate provider gross amount for %s payment %s: %v", request.PluginID, response.PaymentID, err)
			server.RespondWithError(c, http.StatusInternalServerError, "", "Payment processing failed")
			return
		}
	}

	// Store payment record in database for tracking
	billAmountCents := totalCharge - tipAmountCents
	trackedPayment, err := ph.storePluginPaymentRecordForPayer(bill.ID, business.ID, request.PluginID, response.PaymentID, trackedAmountCents, currency, billAmountCents, tipAmountCents, splitShareID, guestPayerSession(c))
	if err != nil {
		log.Printf("Failed to store payment record: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Payment processing failed")
		return
	}

	if hasTracker {
		if err := tracker.AttachAlternativePayment(business.ID, response.PaymentID, trackedPayment.ID); err != nil {
			log.Printf("Failed to attach plugin payment tracker for %s payment %s: %v", request.PluginID, response.PaymentID, err)
			if markErr := ph.markPluginPaymentRecordFailed(trackedPayment.ID); markErr != nil {
				log.Printf("Failed to mark plugin payment tracker row %d failed: %v", trackedPayment.ID, markErr)
			}
			server.RespondWithError(c, http.StatusInternalServerError, "", "Payment processing failed")
			return
		}
	}

	if response.Metadata == nil {
		response.Metadata = map[string]interface{}{}
	}
	// Match the JSON wire contract: emit dollars for client-facing fields,
	// preserve cents in *_cents for server-side reconciliation.
	if _, exists := response.Metadata["amount"]; !exists {
		response.Metadata["amount"] = centsToDollars(trackedAmountCents)
	}
	if _, exists := response.Metadata["amount_cents"]; !exists {
		response.Metadata["amount_cents"] = trackedAmountCents
	}
	response.Metadata["tip_amount"] = request.TipAmount
	response.Metadata["bill_amount"] = centsToDollars(totalCharge - tipAmountCents)
	response.Metadata["requested_amount"] = centsToDollars(totalCharge)
	response.Metadata["requested_amount_cents"] = totalCharge
	response.Metadata["tip_amount_cents"] = tipAmountCents
	response.Metadata["bill_amount_cents"] = totalCharge - tipAmountCents
	if splitShareID != nil {
		response.Metadata["split_share_id"] = *splitShareID
	}

	c.JSON(http.StatusCreated, gin.H{
		"payment_id":   response.PaymentID,
		"status":       response.Status,
		"payment_url":  response.PaymentURL,
		"redirect_url": response.RedirectURL,
		"qr_code":      response.QRCode,
		"expires_at":   response.ExpiresAt,
		"metadata":     response.Metadata,
	})
}

// GetPluginPaymentStatus returns the status of a plugin payment (public)
func (ph *PluginHandlers) GetPluginPaymentStatus(c *gin.Context) {
	billNumber := strings.TrimSpace(c.Param("bill_token"))
	if billNumber == "" {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Bill not found")
		return
	}

	paymentID := c.Param("payment_id")
	pluginName := c.Query("plugin")

	if pluginName == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Plugin name is required")
		return
	}

	if !guestBillPaymentPluginSupported(pluginName) {
		server.RespondWithError(c, http.StatusServiceUnavailable, "", publicBillPaymentUnavailableMessage())
		return
	}

	// Get the payment plugin from registry
	plugin, exists := plugins.GetPluginByName(pluginName)
	if !exists {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Payment plugin not found")
		return
	}

	paymentPlugin, ok := plugin.(plugins.PaymentPlugin)
	if !ok {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Plugin is not a payment plugin")
		return
	}

	// Resolve only the bill id + owning business id; this status read never
	// touches bill relations, items, or amounts. (JSON-01/PRELOAD-01/OF-03)
	billID, businessID, err := database.GetGuestBillScopeByToken(billNumber)
	if err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Bill not found")
		return
	}

	storedPayments, err := database.GetDBWrapper().AlternativePaymentService.GetByBillID(billID)
	if err != nil {
		log.Printf("Failed to load stored plugin payments for bill %d: %v", billID, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Payment processing failed")
		return
	}

	var matchedPayment *database.AlternativePayment
	for _, payment := range storedPayments {
		if string(payment.PaymentMethod) != pluginName {
			continue
		}
		if payment.ParticipantAddr == paymentID || pluginTrackerNameHasProviderTrackerID(payment.ParticipantName, paymentID) {
			paymentCopy := payment
			matchedPayment = &paymentCopy
			break
		}
	}
	if matchedPayment == nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Payment not found for bill")
		return
	}

	if _, err := database.GetBusinessPluginConfig(businessID, pluginName); err != nil {
		if errors.Is(err, database.ErrBusinessPluginNotEnabled) {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Payment plugin not enabled for this business")
			return
		}
		log.Printf("Failed to load plugin config for bill %d, plugin %s: %v", billID, pluginName, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Payment processing failed")
		return
	}

	statusPaymentID := paymentID
	if matchedPayment.ParticipantAddr != "" {
		statusPaymentID = matchedPayment.ParticipantAddr
	}

	var settledPayment *database.Payment
	if paymentRecord, paymentErr := database.GetPaymentByTxHash(fmt.Sprintf("plugin_%s", statusPaymentID)); paymentErr == nil {
		settledPayment = paymentRecord
	}
	if statusPaymentID != paymentID && pluginPaymentLocalSettlementRecorded(matchedPayment, settledPayment) {
		c.JSON(http.StatusOK, gin.H{
			"status": "completed",
			"metadata": pluginPaymentMetadataWithTracker(gin.H{
				"amount":       float64(settledPayment.Amount) / 100.0,
				"tip_amount":   float64(settledPayment.TipAmount) / 100.0,
				"total_amount": float64(settledPayment.Amount+settledPayment.TipAmount) / 100.0,
			}, matchedPayment),
		})
		return
	}
	if strings.HasPrefix(statusPaymentID, "mp_tracker_") && matchedPayment.Status == database.AltPaymentStatusPending {
		c.JSON(http.StatusOK, gin.H{
			"status":   "pending",
			"metadata": pluginPaymentProcessingMetadata(matchedPayment),
		})
		return
	}

	if detailed, ok := paymentPlugin.(plugins.DetailedPaymentStatusProvider); ok {
		details, err := detailed.GetPaymentStatusDetails(businessID, statusPaymentID)
		if err != nil {
			log.Printf("Failed to get detailed payment status for bill %d, payment %s: %v", billID, statusPaymentID, err)
			server.RespondWithError(c, http.StatusInternalServerError, "", "Payment processing failed")
			return
		}
		if pluginPaymentTerminalStatus(details.Status) && !pluginPaymentLocalSettlementRecorded(matchedPayment, settledPayment) {
			c.JSON(http.StatusOK, gin.H{
				"status":   "processing",
				"metadata": pluginPaymentProcessingMetadata(matchedPayment),
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status":   details.Status,
			"metadata": pluginPaymentMetadataWithTracker(details.Metadata, matchedPayment),
		})
		return
	}

	// Get payment status from plugin (simplified call)
	status, err := paymentPlugin.GetPaymentStatus(businessID, statusPaymentID)
	if err != nil {
		// Some plugins (e.g. Stripe Checkout) have no queryable payment status and
		// settle only via webhook — their GetPaymentStatus always errors. Surfacing
		// that as a 500 made the guest poller show "Payment failed" for a card
		// payment that in fact succeeded once the webhook landed. Instead: if the
		// webhook already recorded a local settlement, report completed; otherwise
		// the payment is still in flight → pending, and the poller keeps going until
		// the webhook lands (or the client-side window elapses).
		if settledPayment != nil {
			c.JSON(http.StatusOK, gin.H{
				"status": "completed",
				"metadata": pluginPaymentMetadataWithTracker(gin.H{
					"amount":       float64(settledPayment.Amount) / 100.0,
					"tip_amount":   float64(settledPayment.TipAmount) / 100.0,
					"total_amount": float64(settledPayment.Amount+settledPayment.TipAmount) / 100.0,
				}, matchedPayment),
			})
			return
		}
		log.Printf("Payment status unavailable for bill %d, payment %s (plugin settles via webhook): %v", billID, statusPaymentID, err)
		c.JSON(http.StatusOK, gin.H{
			"status":   "pending",
			"metadata": pluginPaymentProcessingMetadata(matchedPayment),
		})
		return
	}

	if pluginPaymentTerminalStatus(status) {
		if !pluginPaymentLocalSettlementRecorded(matchedPayment, settledPayment) {
			c.JSON(http.StatusOK, gin.H{
				"status":   "processing",
				"metadata": pluginPaymentProcessingMetadata(matchedPayment),
			})
			return
		}
	}

	var responseMetadata interface{}
	if settledPayment != nil {
		responseMetadata = pluginPaymentMetadataWithTracker(gin.H{
			"amount":       float64(settledPayment.Amount) / 100.0,
			"tip_amount":   float64(settledPayment.TipAmount) / 100.0,
			"total_amount": float64(settledPayment.Amount+settledPayment.TipAmount) / 100.0,
		}, matchedPayment)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   status,
		"metadata": responseMetadata,
	})
}

// storePluginPaymentRecord logs the initial payment creation for traceability.
// The actual Payment record is created atomically when the webhook confirms
// the payment (see updateBillPaymentStatus).
func (ph *PluginHandlers) storePluginPaymentRecord(billID, businessID uint, pluginName, paymentID string, amountCents int64, currency string, billAmountCents, tipAmountCents int64, splitShareID *uint) (*database.AlternativePayment, error) {
	return ph.storePluginPaymentRecordForPayer(billID, businessID, pluginName, paymentID, amountCents, currency, billAmountCents, tipAmountCents, splitShareID, nil)
}

// storePluginPaymentRecordForPayer is storePluginPaymentRecord for a
// guest-initiated checkout: payerGuestSession (guestsession.Fingerprint of the
// guest's session, nil for operator-initiated QR/Point charges) is stamped on
// a newly created tracker row as payment proof for the guest fiscal identity
// binding (M-545). An existing tracker for the same provider payment id keeps
// its original attribution.
func (ph *PluginHandlers) storePluginPaymentRecordForPayer(billID, businessID uint, pluginName, paymentID string, amountCents int64, currency string, billAmountCents, tipAmountCents int64, splitShareID *uint, payerGuestSession *string) (*database.AlternativePayment, error) {
	log.Printf("Plugin payment initiated - Bill: %d, Business: %d, Plugin: %s, PaymentID: %s, Amount: %d cents %s",
		billID, businessID, pluginName, paymentID, amountCents, currency)

	dbWrapper := database.GetDBWrapper()
	var existing database.AlternativePayment
	err := dbWrapper.GetGorm().
		Where("bill_id = ? AND participant_addr = ? AND payment_method = ? AND status = ?",
			billID,
			paymentID,
			database.AlternativePaymentMethod(pluginName),
			database.AltPaymentStatusPending,
		).
		First(&existing).Error
	if err == nil {
		if existing.Amount != amountCents {
			return nil, fmt.Errorf("existing plugin payment record amount %d does not match provider amount %d", existing.Amount, amountCents)
		}
		// Backfill breakdown on the existing row if it was created before PAY-2.
		if existing.BillAmountCents == 0 && existing.TipAmountCents == 0 && (billAmountCents > 0 || tipAmountCents > 0) {
			if err := dbWrapper.GetGorm().Model(&database.AlternativePayment{}).
				Where("id = ?", existing.ID).
				Updates(map[string]interface{}{
					"bill_amount_cents": billAmountCents,
					"tip_amount_cents":  tipAmountCents,
				}).Error; err != nil {
				log.Printf("Failed to backfill breakdown on existing plugin payment %d: %v", existing.ID, err)
			} else {
				existing.BillAmountCents = billAmountCents
				existing.TipAmountCents = tipAmountCents
			}
		}
		return &existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to inspect existing plugin payment record: %w", err)
	}

	// F5: a deterministic idempotency key (bill + provider payment id) makes the
	// insert race-safe via the partial unique index
	// idx_alt_payments_bill_idem (bill_id, idempotency_key) WHERE idempotency_key <> ''.
	// The prior SELECT-then-INSERT could create duplicate pending rows under
	// concurrency (double-tap / webhook racing the create); the key collapses
	// them and we recover the existing row on conflict.
	// Q3: ExpiresAt only for MercadoPago trackers (ORD 24h / guest 7d). Other
	// plugins keep NULL until each gets a deliberate lifecycle decision — the
	// prior default 24h silently re-opened Stripe/PayPal split-shares.
	_ = expireStalePendingPluginTrackers(billID)
	expiresAt := pluginTrackerExpiresAt(pluginName, paymentID)
	idempotencyKey := pluginTrackerIdempotencyKey(pluginName, paymentID)
	payment := &database.AlternativePayment{
		BillID:            billID,
		ParticipantAddr:   paymentID,
		ParticipantName:   pluginTrackerParticipantName(pluginName, splitShareID),
		Amount:            amountCents,
		BillAmountCents:   billAmountCents,
		TipAmountCents:    tipAmountCents,
		PaymentMethod:     database.AlternativePaymentMethod(pluginName),
		Status:            database.AltPaymentStatusPending,
		IdempotencyKey:    idempotencyKey,
		ExpiresAt:         expiresAt,
		PayerGuestSession: payerGuestSession,
	}

	if err := dbWrapper.AlternativePaymentService.Create(payment); err != nil {
		if database.IsUniqueConstraintError(err) {
			// A concurrent request already inserted this tracker — fetch and reuse
			// it instead of creating a duplicate pending row.
			var existingRow database.AlternativePayment
			if lookupErr := dbWrapper.GetGorm().
				Where("bill_id = ? AND idempotency_key = ?", billID, idempotencyKey).
				First(&existingRow).Error; lookupErr == nil {
				if existingRow.Amount != amountCents {
					return nil, fmt.Errorf("existing plugin payment record amount %d does not match provider amount %d", existingRow.Amount, amountCents)
				}
				return &existingRow, nil
			}
		}
		return nil, fmt.Errorf("failed to store plugin payment record: %w", err)
	}

	return payment, nil
}

// pluginTrackerIdempotencyKey builds the deterministic dedup key for a plugin
// payment tracker. The provider payment id is globally unique per capture, so
// scoping by plugin name + payment id gives one stable key per attempt.
func pluginTrackerIdempotencyKey(pluginName, paymentID string) string {
	return fmt.Sprintf("plugin:%s:%s", strings.TrimSpace(pluginName), strings.TrimSpace(paymentID))
}

// Plugin tracker TTL policy (P4 + Q3):
//   - MercadoPago ORD… (Point/QR Orders API): 24h — MP orders expire in minutes;
//     after a day a still-pending tracker is garbage and must not 409 a new charge.
//   - MercadoPago mp_tracker_* (Checkout Pro guest): 7d — offline methods
//     (Rapipago, Pago Fácil) can remain pending for multiple days.
//   - Other MercadoPago payment ids: 24h default.
//   - Non-MercadoPago plugins: no ExpiresAt (NULL) — keep pre-P4 non-expiring
//     lifecycle until each provider gets a deliberate decision.
const (
	pluginTrackerORDExpiry           = 24 * time.Hour
	pluginTrackerGuestCheckoutExpiry = 7 * 24 * time.Hour
	pluginTrackerDefaultExpiry       = 24 * time.Hour
)

// pluginTrackerExpiresAt returns the ExpiresAt pointer for a new plugin tracker,
// or nil when the plugin must not auto-expire (Stripe/PayPal/Telegram/…).
func pluginTrackerExpiresAt(pluginName, paymentID string) *time.Time {
	if !strings.EqualFold(strings.TrimSpace(pluginName), "mercadopago") {
		return nil
	}
	now := time.Now().UTC()
	id := strings.TrimSpace(paymentID)
	var exp time.Time
	switch {
	case strings.HasPrefix(strings.ToUpper(id), "ORD"):
		exp = now.Add(pluginTrackerORDExpiry)
	case strings.HasPrefix(strings.ToLower(id), "mp_tracker_"):
		exp = now.Add(pluginTrackerGuestCheckoutExpiry)
	default:
		exp = now.Add(pluginTrackerDefaultExpiry)
	}
	return &exp
}

// expireStalePendingPluginTrackers marks pending alternative_payments with a
// past ExpiresAt as expired. Called before conflict checks and tracker insert.
// Logs ORD expirations so ops can correlate with the 48h reconciler self-heal.
func expireStalePendingPluginTrackers(billID uint) error {
	if billID == 0 {
		return nil
	}
	now := time.Now().UTC()
	var stale []database.AlternativePayment
	if err := database.GetDB().
		Where("bill_id = ? AND status = ? AND expires_at IS NOT NULL AND expires_at <= ?",
			billID, database.AltPaymentStatusPending, now).
		Find(&stale).Error; err != nil {
		return err
	}
	if len(stale) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(stale))
	for _, row := range stale {
		ids = append(ids, row.ID)
		if strings.EqualFold(string(row.PaymentMethod), "mercadopago") &&
			strings.HasPrefix(strings.ToUpper(strings.TrimSpace(row.ParticipantAddr)), "ORD") {
			log.Printf("MercadoPago ORD tracker expired without settle bill=%d tracker=%d order=%s expires_at=%s (reconciler will poll for %s)",
				billID, row.ID, row.ParticipantAddr, row.ExpiresAt.UTC().Format(time.RFC3339), pluginReconciliationExpiredORDLookback)
			metrics.PaymentReconciliationOutcomes.WithLabelValues("mercadopago", "ord_tracker_expired").Inc()
		}
	}
	return database.GetDB().Model(&database.AlternativePayment{}).
		Where("id IN ?", ids).
		Updates(map[string]interface{}{
			"status":     database.AltPaymentStatusExpired,
			"updated_at": now,
		}).Error
}

// pluginTrackerIsActivelyPending reports whether a tracker should still block
// new charges / participate in settlement (pending and not past ExpiresAt).
func pluginTrackerIsActivelyPending(tracker *database.AlternativePayment, now time.Time) bool {
	if tracker == nil || tracker.Status != database.AltPaymentStatusPending {
		return false
	}
	if tracker.ExpiresAt != nil && !tracker.ExpiresAt.After(now) {
		return false
	}
	return true
}

// hasPendingPluginTrackerForSplitShare reports whether a still-pending plugin
// payment tracker already exists for the given split share on this bill. Trackers
// encode the share id in ParticipantName (see pluginTrackerParticipantName), so
// we scan the bill's pending trackers (a small, bounded set) and match the id.
func hasPendingPluginTrackerForSplitShare(billID, shareID uint) (bool, error) {
	if shareID == 0 {
		return false, nil
	}
	_ = expireStalePendingPluginTrackers(billID)
	now := time.Now().UTC()
	var trackers []database.AlternativePayment
	if err := database.GetDB().
		Select("participant_name", "status", "expires_at").
		Where("bill_id = ? AND status = ?", billID, database.AltPaymentStatusPending).
		Find(&trackers).Error; err != nil {
		return false, err
	}
	for _, tracker := range trackers {
		if !pluginTrackerIsActivelyPending(&tracker, now) {
			continue
		}
		if id, ok := splitShareIDFromPluginTrackerName(tracker.ParticipantName); ok && id == shareID {
			return true, nil
		}
	}
	return false, nil
}

// lookupPluginPaymentBreakdown reads the locally persisted bill/tip breakdown from
// the alternative_payments row. Returns (0, 0, false) when the row is missing or
// both breakdown fields are zero.
func lookupPluginPaymentBreakdown(billID uint, paymentID string) (int64, int64, bool) {
	var row database.AlternativePayment
	if err := database.GetDBWrapper().GetGorm().
		Select("bill_amount_cents", "tip_amount_cents").
		Where("bill_id = ? AND participant_addr = ?", billID, paymentID).
		First(&row).Error; err != nil {
		return 0, 0, false
	}
	if row.BillAmountCents == 0 && row.TipAmountCents == 0 {
		return 0, 0, false
	}
	return row.BillAmountCents, row.TipAmountCents, true
}

func (ph *PluginHandlers) markPluginPaymentRecordFailed(alternativePaymentID uint) error {
	now := time.Now()
	return database.GetDBWrapper().GetGorm().
		Model(&database.AlternativePayment{}).
		Where("id = ?", alternativePaymentID).
		Updates(map[string]interface{}{
			"status":     database.AltPaymentStatusFailed,
			"updated_at": now,
		}).Error
}

func pluginPaymentGrossAmountCents(defaultAmountCents int64, metadata map[string]interface{}) (int64, error) {
	if amountCents, ok, err := metadataStrictInt64(metadata["amount_cents"]); err != nil {
		return 0, fmt.Errorf("invalid provider amount_cents: %w", err)
	} else if ok {
		if providerGrossAmountHasDifferentCurrency(metadata) {
			return validatePositiveProviderGrossAmountCents(amountCents)
		}
		return validateProviderGrossAmountCents(defaultAmountCents, amountCents)
	}

	if amountCents, ok, err := metadataStrictInt64(metadata["gross_due_cents"]); err != nil {
		return 0, fmt.Errorf("invalid provider gross_due_cents: %w", err)
	} else if ok {
		if providerGrossAmountHasDifferentCurrency(metadata) {
			return validatePositiveProviderGrossAmountCents(amountCents)
		}
		return validateProviderGrossAmountCents(defaultAmountCents, amountCents)
	}

	feeBreakdown, ok := metadataMap(metadata["fee_breakdown"])
	if !ok {
		return defaultAmountCents, nil
	}
	if amountCents, ok, err := metadataStrictInt64(feeBreakdown["gross_due_cents"]); err != nil {
		return 0, fmt.Errorf("invalid provider fee_breakdown.gross_due_cents: %w", err)
	} else if ok {
		if providerGrossAmountHasDifferentCurrency(metadata) {
			return validatePositiveProviderGrossAmountCents(amountCents)
		}
		return validateProviderGrossAmountCents(defaultAmountCents, amountCents)
	}
	return defaultAmountCents, nil
}

func providerGrossAmountHasDifferentCurrency(metadata map[string]interface{}) bool {
	amountCurrency := strings.ToUpper(strings.TrimSpace(stringFromAny(metadata["amount_currency"])))
	localCurrency := strings.ToUpper(strings.TrimSpace(stringFromAny(metadata["local_currency"])))
	return amountCurrency != "" && localCurrency != "" && amountCurrency != localCurrency
}

func validatePositiveProviderGrossAmountCents(amountCents int64) (int64, error) {
	if amountCents <= 0 {
		return 0, errors.New("provider gross amount must be positive")
	}
	return amountCents, nil
}

func validateProviderGrossAmountCents(defaultAmountCents, amountCents int64) (int64, error) {
	if amountCents <= 0 {
		return 0, errors.New("provider gross amount must be positive")
	}
	if amountCents < defaultAmountCents {
		return 0, fmt.Errorf("provider gross amount %d is below authoritative amount %d", amountCents, defaultAmountCents)
	}
	return amountCents, nil
}

func metadataMap(value interface{}) (map[string]interface{}, bool) {
	switch typed := value.(type) {
	case map[string]interface{}:
		return typed, true
	case gin.H:
		return map[string]interface{}(typed), true
	default:
		return nil, false
	}
}

func metadataStrictInt64(value interface{}) (int64, bool, error) {
	switch typed := value.(type) {
	case nil:
		return 0, false, nil
	case int:
		return int64(typed), true, nil
	case int8:
		return int64(typed), true, nil
	case int16:
		return int64(typed), true, nil
	case int32:
		return int64(typed), true, nil
	case int64:
		return typed, true, nil
	case uint:
		if uint64(typed) > math.MaxInt64 {
			return 0, true, errors.New("integer overflows int64")
		}
		return int64(typed), true, nil
	case uint8:
		return int64(typed), true, nil
	case uint16:
		return int64(typed), true, nil
	case uint32:
		return int64(typed), true, nil
	case uint64:
		if typed > math.MaxInt64 {
			return 0, true, errors.New("integer overflows int64")
		}
		return int64(typed), true, nil
	case float32:
		floatValue := float64(typed)
		if math.IsNaN(floatValue) || math.IsInf(floatValue, 0) || floatValue > float64(math.MaxInt64) || floatValue < float64(math.MinInt64) {
			return 0, true, errors.New("number is outside int64 range")
		}
		if floatValue != math.Trunc(floatValue) {
			return 0, true, errors.New("must be integer cents")
		}
		return int64(floatValue), true, nil
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || typed > float64(math.MaxInt64) || typed < float64(math.MinInt64) {
			return 0, true, errors.New("number is outside int64 range")
		}
		if typed != math.Trunc(typed) {
			return 0, true, errors.New("must be integer cents")
		}
		return int64(typed), true, nil
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return parsed, true, nil
		}
		return 0, true, errors.New("must be integer cents")
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err == nil {
			return parsed, true, nil
		}
		return 0, true, errors.New("must be integer cents")
	}
	return 0, false, nil
}

func pluginPaymentTerminalStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "success", "paid", "confirmed":
		return true
	default:
		return false
	}
}

func pluginPaymentLocalSettlementRecorded(matchedPayment *database.AlternativePayment, settledPayment *database.Payment) bool {
	return (matchedPayment != nil && matchedPayment.Status == database.AltPaymentStatusConfirmed) || settledPayment != nil
}

func pluginPaymentProcessingMetadata(matchedPayment *database.AlternativePayment) gin.H {
	amount := float64(0)
	if matchedPayment != nil {
		amount = float64(matchedPayment.Amount) / 100.0
	}
	return pluginPaymentMetadataWithTracker(gin.H{
		"amount":              amount,
		"awaiting_settlement": true,
	}, matchedPayment)
}

func pluginPaymentMetadataWithTracker(metadata map[string]interface{}, matchedPayment *database.AlternativePayment) gin.H {
	response := gin.H{}
	for key, value := range metadata {
		response[key] = value
	}
	if matchedPayment != nil {
		if shareID, ok := splitShareIDFromPluginTrackerName(matchedPayment.ParticipantName); ok {
			response["split_share_id"] = shareID
		}
	}
	if len(response) == 0 {
		return nil
	}
	return response
}

// Payment Plugin Webhook Handlers

// HandlePayPalWebhook handles PayPal webhook callbacks
func (ph *PluginHandlers) HandlePayPalWebhook(c *gin.Context) {
	ph.handlePaymentWebhook(c, "paypal")
}

// HandlePayPalReturn handles PayPal return URL callbacks
func (ph *PluginHandlers) HandlePayPalReturn(c *gin.Context) {
	billID := c.Query("bill_id")
	paymentToken := firstNonEmptyString(
		strings.TrimSpace(c.Query("token")),
		strings.TrimSpace(c.Query("paymentId")),
	)
	paymentID := c.Query("PayerID")

	log.Printf("PayPal return callback - Bill ID: %s, Payment ID: %s", billID, paymentID)

	redirectPaymentID := paymentToken
	capturedPaymentID, failed := ph.capturePayPalReturnPayment(c, billID, paymentToken)
	if failed {
		c.Redirect(http.StatusFound, buildGuestBillRedirectURLForProvider("failed", "paypal", billID, ""))
		return
	}
	if capturedPaymentID != "" {
		redirectPaymentID = capturedPaymentID
	}

	c.Redirect(
		http.StatusFound,
		buildGuestBillRedirectURLForProvider("success", "paypal", billID, redirectPaymentID),
	)
}

func (ph *PluginHandlers) capturePayPalReturnPayment(c *gin.Context, billIDRaw string, orderID string) (string, bool) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return "", false
	}
	parsedBillID, err := strconv.ParseUint(strings.TrimSpace(billIDRaw), 10, 32)
	if err != nil {
		log.Printf("PayPal return capture skipped: invalid bill ID %q", billIDRaw)
		return "", false
	}
	bill, _, err := database.GetBillByID(uint(parsedBillID))
	if err != nil || bill == nil {
		log.Printf("PayPal return capture skipped: bill %s not found: %v", billIDRaw, err)
		return "", false
	}
	if _, err := database.GetBusinessPluginConfig(bill.BusinessID, "paypal"); err != nil {
		log.Printf("PayPal return capture skipped: paypal config unavailable for business %d: %v", bill.BusinessID, err)
		return "", false
	}
	// Only capture orders that THIS server legitimately initiated: require a
	// local pending PayPal tracker for (bill, orderID). Without this an
	// unauthenticated caller could drive outbound PayPal capture calls for
	// arbitrary (bill, token) pairs (amplification / PayPal rate-limit burn).
	if !hasPendingPayPalTracker(bill.ID, orderID) {
		log.Printf("PayPal return capture skipped: no local pending tracker for bill %d order %s", bill.ID, orderID)
		return "", false
	}
	// PayPal orders are only authorized until captured here: never capture
	// funds onto a bill that can no longer accept them.
	if bill.Status != database.BillStatusOpen && bill.Status != database.BillStatusPartial {
		log.Printf("PayPal return capture refused: bill %d is %s", bill.ID, bill.Status)
		return "", true
	}
	plugin, exists := plugins.GetPluginByName("paypal")
	if !exists {
		log.Printf("PayPal return capture skipped: paypal plugin not registered")
		return "", false
	}
	capturer, ok := plugin.(plugins.PaymentReturnCapturer)
	if !ok {
		log.Printf("PayPal return capture skipped: paypal plugin cannot capture returns")
		return "", false
	}
	response, err := capturer.CapturePaymentReturn(bill.BusinessID, bill.ID, orderID)
	if err != nil {
		log.Printf("PayPal return capture failed for bill %d order %s: %v", bill.ID, orderID, err)
		return "", false
	}
	if response == nil || !response.Success {
		message := ""
		if response != nil {
			message = response.Message
		}
		log.Printf("PayPal return capture did not complete for bill %d order %s: %s", bill.ID, orderID, message)
		return "", false
	}
	effectiveBillID := response.BillID
	if effectiveBillID == 0 {
		effectiveBillID = bill.ID
	}
	if effectiveBillID != bill.ID {
		log.Printf("PayPal return capture bill mismatch: callback bill %d response bill %d", bill.ID, effectiveBillID)
		return "", false
	}
	if strings.ToLower(strings.TrimSpace(response.Status)) != "completed" {
		return response.PaymentID, false
	}
	if err := rekeyTrackedPluginPayment(bill.ID, orderID, response.PaymentID); err != nil {
		log.Printf("PayPal return capture failed to re-key tracker for bill %d order %s capture %s: %v", bill.ID, orderID, response.PaymentID, err)
		return response.PaymentID, false
	}
	localBill, localTip, hasLocal := lookupPluginPaymentBreakdown(bill.ID, response.PaymentID)
	if ph.settleReturnCapture(c, plugin, "paypal", bill, response, localBill, localTip, hasLocal) {
		return response.PaymentID, true
	}
	return response.PaymentID, false
}

func rekeyTrackedPluginPayment(billID uint, oldPaymentID string, newPaymentID string) error {
	oldPaymentID = strings.TrimSpace(oldPaymentID)
	newPaymentID = strings.TrimSpace(newPaymentID)
	if billID == 0 || oldPaymentID == "" || newPaymentID == "" || oldPaymentID == newPaymentID {
		return nil
	}
	if strings.HasPrefix(oldPaymentID, "mp_tracker_") {
		var tracker database.AlternativePayment
		if err := database.GetDB().
			Select("id", "participant_name").
			Where("bill_id = ? AND participant_addr = ?", billID, oldPaymentID).
			First(&tracker).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		return database.GetDB().
			Model(&database.AlternativePayment{}).
			Where("id = ?", tracker.ID).
			Updates(map[string]interface{}{
				"participant_addr": newPaymentID,
				"participant_name": pluginTrackerNameWithProviderTrackerID(tracker.ParticipantName, oldPaymentID),
			}).Error
	}
	return database.GetDB().
		Model(&database.AlternativePayment{}).
		Where("bill_id = ? AND participant_addr = ?", billID, oldPaymentID).
		Update("participant_addr", newPaymentID).Error
}

// hasPendingPayPalTracker returns true when a locally-initiated, not-yet-settled
// PayPal AlternativePayment row exists for (billID, orderID). This is used as a
// precondition before making an outbound capture call: only orders that this server
// created (and therefore has a tracker for) should ever drive a capture attempt.
func hasPendingPayPalTracker(billID uint, orderID string) bool {
	orderID = strings.TrimSpace(orderID)
	if billID == 0 || orderID == "" {
		return false
	}
	var count int64
	database.GetDB().
		Model(&database.AlternativePayment{}).
		Where(
			"bill_id = ? AND participant_addr = ? AND payment_method = ? AND status = ?",
			billID, orderID, database.AlternativePaymentMethod("paypal"), database.AltPaymentStatusPending,
		).
		Count(&count)
	return count > 0
}

func pluginTrackerIDFromWebhookMetadata(metadata map[string]interface{}) string {
	for _, key := range []string{"payverge_tracker_id", "provider_tracker_id", "preference_id"} {
		if value, ok := metadata[key]; ok {
			trackerID := strings.TrimSpace(fmt.Sprint(value))
			if trackerID != "" && trackerID != "<nil>" {
				return trackerID
			}
		}
	}
	return ""
}

// HandleMercadoPagoReturn handles Checkout Pro back_url redirects and captures
// payment status immediately (PayPal-style return path).
func (ph *PluginHandlers) HandleMercadoPagoReturn(c *gin.Context) {
	billID := firstNonEmptyString(
		strings.TrimSpace(c.Query("bill_id")),
		strings.TrimSpace(c.Query("bill_token")),
	)
	paymentID := firstNonEmptyString(
		strings.TrimSpace(c.Query("payment_id")),
		strings.TrimSpace(c.Query("collection_id")),
	)
	// Prefer external_reference for bill resolution when bill_id is missing.
	if billID == "" {
		if ref := strings.TrimSpace(c.Query("external_reference")); ref != "" {
			if parsedBill, _, ok := parseMercadoPagoExternalReferenceForReturn(ref); ok {
				billID = strconv.FormatUint(uint64(parsedBill), 10)
			}
		}
	}

	log.Printf("MercadoPago return callback - Bill ID: %s, Payment ID: %s", billID, paymentID)

	redirectPaymentID := paymentID
	capturedPaymentID, failed := ph.captureMercadoPagoReturnPayment(c, billID, paymentID)
	if failed {
		c.Redirect(http.StatusFound, buildGuestBillRedirectURLForProvider("failed", "mercadopago", billID, ""))
		return
	}
	if capturedPaymentID != "" {
		redirectPaymentID = capturedPaymentID
	}

	c.Redirect(
		http.StatusFound,
		buildGuestBillRedirectURLForProvider("success", "mercadopago", billID, redirectPaymentID),
	)
}

func parseMercadoPagoExternalReferenceForReturn(reference string) (uint, uint, bool) {
	reference = strings.TrimSpace(reference)
	parts := strings.Split(reference, "_")
	if len(parts) < 4 || parts[0] != "bill" || parts[2] != "business" {
		return 0, 0, false
	}
	billID, err1 := strconv.ParseUint(parts[1], 10, 32)
	businessID, err2 := strconv.ParseUint(parts[3], 10, 32)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return uint(billID), uint(businessID), true
}

func (ph *PluginHandlers) captureMercadoPagoReturnPayment(c *gin.Context, billIDRaw string, paymentID string) (string, bool) {
	paymentID = strings.TrimSpace(paymentID)
	if paymentID == "" {
		return "", false
	}
	parsedBillID, err := strconv.ParseUint(strings.TrimSpace(billIDRaw), 10, 32)
	if err != nil {
		log.Printf("MercadoPago return capture skipped: invalid bill ID %q", billIDRaw)
		return "", false
	}
	bill, _, err := database.GetBillByID(uint(parsedBillID))
	if err != nil || bill == nil {
		log.Printf("MercadoPago return capture skipped: bill %s not found: %v", billIDRaw, err)
		return "", false
	}
	if _, err := database.GetBusinessPluginConfig(bill.BusinessID, "mercadopago"); err != nil {
		log.Printf("MercadoPago return capture skipped: mercadopago config unavailable for business %d: %v", bill.BusinessID, err)
		return "", false
	}
	// Only capture when this server has a pending mercadopago tracker for the bill
	// (amplification guard, same rationale as PayPal).
	if !hasPendingMercadoPagoTracker(bill.ID) {
		log.Printf("MercadoPago return capture skipped: no local pending tracker for bill %d", bill.ID)
		return "", false
	}
	plugin, exists := plugins.GetPluginByName("mercadopago")
	if !exists {
		log.Printf("MercadoPago return capture skipped: mercadopago plugin not registered")
		return "", false
	}
	capturer, ok := plugin.(plugins.PaymentReturnCapturer)
	if !ok {
		log.Printf("MercadoPago return capture skipped: mercadopago plugin cannot capture returns")
		return "", false
	}
	response, err := capturer.CapturePaymentReturn(bill.BusinessID, bill.ID, paymentID)
	if err != nil {
		log.Printf("MercadoPago return capture failed for bill %d payment %s: %v", bill.ID, paymentID, err)
		return "", false
	}
	if response == nil || !response.Success {
		message := ""
		if response != nil {
			message = response.Message
		}
		log.Printf("MercadoPago return capture did not complete for bill %d payment %s: %s", bill.ID, paymentID, message)
		return "", false
	}
	effectiveBillID := response.BillID
	if effectiveBillID == 0 {
		effectiveBillID = bill.ID
	}
	if effectiveBillID != bill.ID {
		log.Printf("MercadoPago return capture bill mismatch: callback bill %d response bill %d", bill.ID, effectiveBillID)
		return "", false
	}
	if strings.ToLower(strings.TrimSpace(response.Status)) != "completed" {
		return response.PaymentID, false
	}
	// Re-key mp_tracker_* placeholder to the real MercadoPago payment id.
	if trackerID := pluginTrackerIDFromWebhookMetadata(response.Metadata); trackerID != "" {
		if err := rekeyTrackedPluginPayment(bill.ID, trackerID, response.PaymentID); err != nil {
			log.Printf("MercadoPago return capture failed to re-key tracker for bill %d tracker %s payment %s: %v", bill.ID, trackerID, response.PaymentID, err)
			return response.PaymentID, false
		}
	}
	localBill, localTip, hasLocal := lookupPluginPaymentBreakdown(bill.ID, response.PaymentID)
	if !hasLocal {
		// Also try rekeyed tracker path for breakdown lookup by scanning pending.
		localBill, localTip, hasLocal = lookupPluginPaymentBreakdown(bill.ID, pluginTrackerIDFromWebhookMetadata(response.Metadata))
	}
	if ph.settleReturnCapture(c, plugin, "mercadopago", bill, response, localBill, localTip, hasLocal) {
		return response.PaymentID, true
	}
	return response.PaymentID, false
}

func hasPendingMercadoPagoTracker(billID uint) bool {
	if billID == 0 {
		return false
	}
	var count int64
	database.GetDB().
		Model(&database.AlternativePayment{}).
		Where(
			"bill_id = ? AND payment_method = ? AND status = ?",
			billID, database.AlternativePaymentMethod("mercadopago"), database.AltPaymentStatusPending,
		).
		Count(&count)
	return count > 0
}

// HandlePayPalCancel handles PayPal cancel URL callbacks
func (ph *PluginHandlers) HandlePayPalCancel(c *gin.Context) {
	billID := c.Query("bill_id")

	log.Printf("PayPal cancel callback - Bill ID: %s", billID)

	c.Redirect(
		http.StatusFound,
		buildGuestBillRedirectURLForProvider("cancelled", "paypal", billID, ""),
	)
}

func buildGuestBillRedirectURLForProvider(paymentStatus, paymentMethod, billID, paymentID string) string {
	params := url.Values{}
	params.Set("payment", paymentStatus)
	params.Set("method", paymentMethod)
	if strings.TrimSpace(paymentID) != "" {
		params.Set("payment_id", paymentID)
		params.Set("payment_method", paymentMethod)
	}

	parsedBillID, err := strconv.ParseUint(strings.TrimSpace(billID), 10, 32)
	if err != nil {
		return "/?" + params.Encode()
	}

	bill, _, err := database.GetBillByID(uint(parsedBillID))
	if err != nil {
		return "/?" + params.Encode()
	}
	if strings.TrimSpace(bill.BillNumber) != "" {
		params.Set("bill_number", bill.BillNumber)
	}

	if bill.TableID != 0 {
		table, err := database.GetTableByID(bill.TableID)
		if err == nil && strings.TrimSpace(table.TableCode) != "" {
			return fmt.Sprintf("/t/%s/bill?%s", url.PathEscape(table.TableCode), params.Encode())
		}
	}

	return "/?" + params.Encode()
}

// HandleStripeWebhook handles Stripe webhook callbacks
func (ph *PluginHandlers) HandleStripeWebhook(c *gin.Context) {
	ph.handlePaymentWebhook(c, "stripe")
}

// HandleMercadoPagoWebhook handles MercadoPago webhook callbacks
func (ph *PluginHandlers) HandleMercadoPagoWebhook(c *gin.Context) {
	ph.handlePaymentWebhook(c, "mercadopago")
}

// Generic webhook handler for payment plugins
func (ph *PluginHandlers) handlePaymentWebhook(c *gin.Context, pluginName string) {
	if !paymentWebhookProviderSupported(pluginName) {
		server.RespondWithError(c, http.StatusServiceUnavailable, "", "Secure webhook verification unavailable for this payment provider")
		return
	}

	// Read the webhook payload
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("Failed to read webhook payload for %s: %v", pluginName, err)
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Failed to read payload")
		return
	}

	// Get headers for signature verification
	headers := make(map[string]string)
	for key, values := range c.Request.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	var payloadData map[string]interface{}
	if err := json.Unmarshal(payload, &payloadData); err != nil {
		log.Printf("Failed to parse webhook payload for %s: %v", pluginName, err)
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid webhook payload")
		return
	}

	// Get the payment plugin from registry
	plugin, exists := plugins.GetPluginByName(pluginName)
	if !exists {
		log.Printf("Payment plugin %s not found", pluginName)
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeBusinessNotFound, "Plugin not found")
		return
	}

	paymentPlugin, ok := plugin.(plugins.PaymentPlugin)
	if !ok {
		log.Printf("Plugin %s is not a payment plugin", pluginName)
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Not a payment plugin")
		return
	}

	query := c.Request.URL.Query()
	eventType := extractPluginWebhookEventType(pluginName, payloadData)
	billIDFromPayload := extractPluginWebhookBillID(pluginName, payloadData, query)

	isStripe := strings.EqualFold(strings.TrimSpace(pluginName), "stripe")
	isStripeDeauth := isStripe && eventType == "account.application.deauthorized"
	var stripeTenant stripeWebhookTenant
	var businessID uint
	if isStripe {
		var resolveErr error
		stripeTenant, resolveErr = resolveStripeWebhookTenant(payloadData)
		if resolveErr != nil {
			log.Printf("Failed to resolve Stripe webhook tenant: %v", resolveErr)
			server.RespondWithError(c, http.StatusInternalServerError, "", "Webhook processing failed")
			return
		}
		// A forged metadata.business_id is the whole cross-tenant attack: every
		// Connect delivery is signed with the same platform secret, so a valid
		// signature proves only "some connected merchant". Reject rather than
		// downgrade to unresolved — a 4xx is visible in the sender's Stripe
		// dashboard and the delivery never touches any tenant.
		if stripeTenant.metadataMismatch {
			metrics.PaymentWebhookUnsupportedActions.WithLabelValues(pluginName, "connect.account_business_mismatch").Inc()
			log.Printf("Rejecting Stripe webhook: payload business does not own connected account %s", stripeTenant.account)
			server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Webhook does not belong to this business")
			return
		}
		// Deauthorization acts on every business holding the account, so it does
		// not need a single unambiguous tenant. Anything that moves money does.
		if stripeTenant.ambiguous && !isStripeDeauth {
			metrics.PaymentWebhookUnsupportedActions.WithLabelValues(pluginName, "connect.account_ambiguous").Inc()
			log.Printf("Rejecting Stripe webhook: connected account %s maps to %d businesses", stripeTenant.account, len(stripeTenant.accountBusinessIDs))
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Connected account maps to multiple businesses")
			return
		}
		businessID = stripeTenant.businessID
	} else {
		businessID = extractPluginWebhookBusinessID(pluginName, payloadData, query)
	}
	// Backfilling the tenant from the referenced bill is only safe when the
	// payload's routing identity is not cross-tenant reachable. On a Connect
	// delivery the account IS the identity; if it resolved to no business,
	// adopting the named bill's owner would let any connected merchant settle a
	// victim's bill just by naming its bill_id.
	if billIDFromPayload > 0 && businessID == 0 && stripeTenant.account == "" {
		if billBusinessID, err := database.GetBillBusinessIDByBillID(billIDFromPayload); err == nil && billBusinessID > 0 {
			businessID = billBusinessID
		}
	}

	// Verify signature BEFORE creating the idempotency row so that forged
	// webhooks never touch the database. Resolve businessID first because
	// the verification function needs it to look up the plugin secret.
	// Connect deauthorization is exempt: the account may already be unknown to
	// us (nothing left to disconnect), and it still verifies against the
	// platform Connect secret below before it is allowed to ACK.
	if businessID == 0 && !isStripeDeauth && paymentWebhookRequiresResolvedBusiness(pluginName, paymentPlugin) {
		rejectPluginWebhookBeforeClaim(c, pluginName, metrics.WebhookFailureBusinessUnresolved, http.StatusBadRequest, server.ErrCodeInvalidInput, "Unable to resolve business for payment webhook")
		return
	}
	// MercadoPago orders-topic notifications may omit business_id and can race
	// the tracker insert (MP creates the order before our AlternativePayment row
	// exists). When signature verifies but no tracker resolves business:
	//   - money-state actions (processed/refunded/canceled/…) → 500 so MP retries
	//   - informational actions (created/updated) → 200 ACK + counter (P5)
	// Foreign/unknown settle-class orders exhaust MP retries harmlessly.
	if businessID == 0 && strings.EqualFold(pluginName, "mercadopago") {
		action := extractPluginWebhookEventType(pluginName, payloadData)
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(action)), "order.") {
			if err := verifyPluginWebhookSignature(paymentPlugin, pluginName, 0, payload, headers, query); err != nil {
				if errors.Is(err, errPluginWebhookSecretMissing) {
					rejectPluginWebhookBeforeClaim(c, pluginName, metrics.WebhookFailureSecretMissing, http.StatusServiceUnavailable, "", "Webhook configuration unavailable")
					return
				}
				rejectPluginWebhookBeforeClaim(c, pluginName, metrics.WebhookFailureSignatureInvalid, http.StatusUnauthorized, server.ErrCodeTokenInvalid, "Invalid webhook signature")
				return
			}
			// Distinct labels so foreign/untracked settle-class 500s are not
			// counted as generic app failures in error-rate alerts (Q4b).
			if mercadoPagoOrderActionNeedsTrackerRetry(action) {
				metrics.PaymentWebhookUnsupportedActions.WithLabelValues(pluginName, "order.untracked_settle_retry").Inc()
				log.Printf("mp_order_untracked_settle_retry action=%s (by-design 500 for MP retry; not an app failure)", action)
				server.RespondWithError(c, http.StatusInternalServerError, "", "Order tracker not ready")
				return
			}
			metrics.PaymentWebhookUnsupportedActions.WithLabelValues(pluginName, "order.untracked_informational").Inc()
			log.Printf("mp_order_untracked_informational action=%s (ACK without local tracker)", action)
			c.JSON(http.StatusOK, gin.H{"message": "Order notification acknowledged without local tracker"})
			return
		}
		rejectPluginWebhookBeforeClaim(c, pluginName, metrics.WebhookFailureBusinessUnresolved, http.StatusBadRequest, server.ErrCodeInvalidInput, "Unable to resolve business for payment webhook")
		return
	}
	if err := verifyPluginWebhookSignature(paymentPlugin, pluginName, businessID, payload, headers, query); err != nil {
		if errors.Is(err, errPluginWebhookSecretMissing) {
			rejectPluginWebhookBeforeClaim(c, pluginName, metrics.WebhookFailureSecretMissing, http.StatusServiceUnavailable, "", "Webhook configuration unavailable")
			return
		}
		rejectPluginWebhookBeforeClaim(c, pluginName, metrics.WebhookFailureSignatureInvalid, http.StatusUnauthorized, server.ErrCodeTokenInvalid, "Invalid webhook signature")
		return
	}

	providerKey := "plugin_" + pluginName
	webhookID := pluginFirstNonEmpty(
		getHeaderValue(headers, "webhook-id"),
		getHeaderValue(headers, "x-webhook-id"),
		extractPluginWebhookID(pluginName, payloadData),
	)
	if webhookID == "" && firstPartyRequiresWebhookID(pluginName) {
		log.Printf("Rejecting %s webhook with no extractable id (replay dedup would be unprotected)", pluginName)
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Webhook missing id")
		return
	}
	// ownsWebhookRow is true once this request holds the webhook_events row
	// (fresh insert or a won retry claim). From then on every non-2xx exit must
	// leave the row failed: a row left in "processing" turns the provider's
	// redelivery into a no-op for StaleWebhookProcessingAfter (OBS-001). The
	// deferred guard covers every exit, including ones added later.
	ownsWebhookRow := false
	defer func() {
		guardPluginWebhookRow(c, ownsWebhookRow, pluginName, providerKey, webhookID, recover())
	}()
	if webhookID != "" {
		eventRecord := &database.WebhookEvent{
			Provider:   providerKey,
			WebhookID:  webhookID,
			EventType:  eventType,
			Payload:    verifiedWebhookPayloadEvidence(payload),
			Status:     "processing",
			ReceivedAt: time.Now(),
		}
		duplicate, err := database.GetDBWrapper().CreateWebhookEventIfNotExists(eventRecord)
		if err != nil {
			log.Printf("Failed to register webhook event for %s: %v", pluginName, err)
			server.RespondWithError(c, http.StatusInternalServerError, "", "Webhook processing failed")
			return
		}
		if !duplicate {
			ownsWebhookRow = true
		}
		if duplicate {
			existingEvent, getErr := database.GetDBWrapper().GetWebhookEvent(providerKey, webhookID)
			if getErr != nil {
				log.Printf("Failed to inspect duplicate webhook for %s: %v", pluginName, getErr)
				server.RespondWithError(c, http.StatusInternalServerError, "", "Webhook processing failed")
				return
			}
			if existingEvent != nil && database.WebhookEventReprocessable(existingEvent, time.Now()) {
				// Exclusive CAS claim: concurrent Stripe/provider retries after
				// a failed/stale event must not both re-enter settlement.
				// Settlement remains independently idempotent as belt-and-braces.
				claimed, claimErr := database.GetDBWrapper().ClaimWebhookEventForProcessing(existingEvent.ID, time.Now())
				if claimErr != nil {
					log.Printf("Failed to claim webhook for retry %s: %v", pluginName, claimErr)
					server.RespondWithError(c, http.StatusInternalServerError, "", "Webhook processing failed")
					return
				}
				if !claimed {
					// Another delivery won the claim and is processing right now. A
					// 2xx here would let the provider drop this delivery even if that
					// worker then fails, so ask for a later retry instead.
					log.Printf("Duplicate webhook claim lost for %s: %s", pluginName, webhookID)
					server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Webhook is still being processed")
					return
				}
				ownsWebhookRow = true
			} else if pluginName == "mercadopago" && existingEvent != nil && existingEvent.Status == "processed" {
				// MercadoPago's stable notification identity is action:data.id, but
				// it can reuse payment.updated for successive states of the same
				// payment (for example pending followed by approved). Re-fetch the
				// authoritative provider state after a completed delivery instead of
				// swallowing that transition as a replay. Settlement and reversal
				// remain independently idempotent, so an identical redelivery is a
				// harmless revalidation. An in-flight duplicate is still rejected by
				// the processing-state branch below.
				log.Printf("Revalidating MercadoPago payment update %s", webhookID)
				ownsWebhookRow = true
			} else if existingEvent != nil && existingEvent.Status == "processed" {
				log.Printf("Duplicate webhook ignored for %s: %s", pluginName, webhookID)
				c.JSON(http.StatusOK, gin.H{"message": "Webhook already processed"})
				return
			} else {
				// A fresh in-flight row (or an unknown state): only a processed row
				// may be acknowledged. 409 makes the provider redeliver after the
				// current attempt finishes or goes stale.
				log.Printf("Duplicate webhook still in flight for %s: %s", pluginName, webhookID)
				server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Webhook is still being processed")
				return
			}
		}
	}

	// Stripe Connect deauthorization. Deliberately placed AFTER signature
	// verification and AFTER the webhook_events dedup claim: the mutation is
	// destructive and not idempotent in effect. A stale Stripe retry landing
	// after the merchant reconnected would disconnect them a second time, so the
	// dedup row is what turns a redelivery into a no-op ACK. The account, not
	// the payload's metadata, decides which businesses are affected.
	if isStripeDeauth {
		updated, markErr := database.MarkStripeOAuthDeauthorized(stripeTenant.account, stripeTenant.accountBusinessIDs...)
		if markErr != nil {
			if webhookID != "" {
				_ = database.GetDBWrapper().MarkWebhookEventFailed(providerKey, webhookID, truncateWebhookError(markErr.Error(), 1000))
			}
			log.Printf("Stripe deauth mark failed account=%s: %v", stripeTenant.account, markErr)
			server.RespondWithError(c, http.StatusInternalServerError, "", "Webhook processing failed")
			return
		}
		if webhookID != "" {
			if err := database.GetDBWrapper().MarkWebhookEventProcessed(providerKey, webhookID); err != nil {
				log.Printf("Failed to mark webhook as processed for %s: %v", pluginName, err)
			}
		}
		log.Printf("Stripe account.application.deauthorized account=%s businesses_updated=%d", stripeTenant.account, updated)
		c.JSON(http.StatusOK, gin.H{"message": "Deauthorization processed", "updated": updated})
		return
	}

	// Handle the webhook
	response, err := paymentPlugin.HandleWebhook(businessID, payload, headers)
	if err != nil {
		if webhookID != "" {
			_ = database.GetDBWrapper().MarkWebhookEventFailed(providerKey, webhookID, truncateWebhookError(err.Error(), 1000))
		}
		recordPluginWebhookFailureHealth(businessID, pluginName, err.Error())
		log.Printf("Failed to handle %s webhook: %v", pluginName, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Webhook processing failed")
		return
	}

	if !response.Success {
		if webhookID != "" {
			_ = database.GetDBWrapper().MarkWebhookEventFailed(providerKey, webhookID, truncateWebhookError(response.Message, 1000))
		}
		recordPluginWebhookFailureHealth(businessID, pluginName, response.Message)
		log.Printf("Webhook processing failed for %s: %s", pluginName, response.Message)
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, response.Message)
		return
	}

	effectiveBillID := response.BillID
	if effectiveBillID == 0 {
		effectiveBillID = billIDFromPayload
	}

	// PAY-1: verify the settled bill belongs to the business whose secret
	// verified the webhook signature. businessID is the verified-owner
	// identity (resolved from metadata/query, possibly backfilled from the
	// bill — but in the cross-business attack the attacker supplies a
	// different business_id in metadata, so the backfill path is NOT
	// triggered). Comparing bill.BusinessID against businessID catches
	// the attack where an operator holding business A's webhook secret
	// crafts a signed payload referencing business B's bill_id.
	//
	// Guard-removal proof: temporarily deleting this block causes
	// TestHandlePaymentWebhook_RejectsCrossBusinessSettlement to FAIL
	// (victim bill settles, PaidAmount>0, Payment row appears).
	if effectiveBillID > 0 && businessID > 0 {
		victimBusinessID, loadErr := database.GetBillBusinessIDByBillID(effectiveBillID)
		if loadErr != nil {
			// Fail closed: never settle when ownership cannot be verified.
			if webhookID != "" {
				_ = database.GetDBWrapper().MarkWebhookEventFailed(providerKey, webhookID, truncateWebhookError(loadErr.Error(), 1000))
			}
			log.Printf("Failed to load bill %d ownership for webhook %s: %v", effectiveBillID, pluginName, loadErr)
			server.RespondWithError(c, http.StatusInternalServerError, "", "Webhook processing failed")
			return
		}
		if victimBusinessID != businessID {
			server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden,
				"Bill does not belong to this business")
			return
		}
	}

	if response.Status == "completed" && businessID > 0 {
		defaultCurrency, _, loadErr := database.GetBusinessDefaults(businessID)
		if loadErr != nil {
			if webhookID != "" {
				_ = database.GetDBWrapper().MarkWebhookEventFailed(providerKey, webhookID, truncateWebhookError(loadErr.Error(), 1000))
			}
			log.Printf("Failed to load business %d currency for webhook %s: %v", businessID, pluginName, loadErr)
			server.RespondWithError(c, http.StatusInternalServerError, "", "Webhook processing failed")
			return
		}
		expectedCurrency := authoritativeGuestPluginCurrency(&database.Business{DefaultCurrency: defaultCurrency})
		actualCurrency := strings.ToUpper(strings.TrimSpace(response.Currency))
		if actualCurrency != expectedCurrency {
			currencyErr := fmt.Errorf("%w: provider currency %q does not match business currency %q", errPluginPaymentCurrencyMismatch, actualCurrency, expectedCurrency)
			// The provider has already captured the guest's funds. Treat a currency
			// mismatch like every other unsettleable capture: refund it immediately,
			// acknowledge only after refund success, and leave a failed event/non-2xx
			// response when the provider refund must be retried.
			if ph.autoRefundUnsettleablePluginCapture(c, paymentPlugin, pluginName, businessID, effectiveBillID, response, providerKey, webhookID, currencyErr) {
				return
			}
			notePluginWebhookSettlementFailure(c, businessID, pluginName, "provider currency does not match business currency")
			if webhookID != "" {
				_ = database.GetDBWrapper().MarkWebhookEventFailed(providerKey, webhookID, "provider currency does not match business currency")
			}
			log.Printf("Rejecting %s payment %s for business %d: provider currency %q does not match %q", pluginName, response.PaymentID, businessID, actualCurrency, expectedCurrency)
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Payment currency does not match the business currency")
			return
		}
	}

	// D3: a refund, dispute or reversal for a capture that is not in the ledger
	// yet is asked to retry (503 + retry_pending row) instead of being acked and
	// lost; after 72h it is acked with an operator alert. effectiveBillID was
	// verified above to belong to businessID.
	if ph.deferPluginWebhookUntilCaptureRecorded(c, pluginName, businessID, effectiveBillID, payloadData, response, providerKey, webhookID) {
		return
	}

	// A dispute is an alert state, not a reversal. The provider still considers
	// the capture settled until a later loss/refund event, so preserve the local
	// payment and bill while surfacing a durable, idempotent review alert.
	if response.Status == "disputed" {
		txHash := fmt.Sprintf("plugin_%s", response.PaymentID)
		disputeBillID := effectiveBillID
		if disputeBillID == 0 {
			if payment, paymentErr := database.GetPaymentByTxHash(txHash); paymentErr == nil && payment != nil {
				disputeBillID = payment.BillID
			}
		}
		// A provider dispute object may omit the original bill reference. When
		// the local Payment supplies it, repeat the ownership check that normally
		// runs above so business A's signed webhook cannot create an alert against
		// business B by naming B's provider payment ID.
		if disputeBillID > 0 && businessID > 0 {
			disputeBusinessID, loadErr := database.GetBillBusinessIDByBillID(disputeBillID)
			if loadErr != nil {
				if webhookID != "" {
					_ = database.GetDBWrapper().MarkWebhookEventFailed(providerKey, webhookID, truncateWebhookError(loadErr.Error(), 1000))
				}
				server.RespondWithError(c, http.StatusInternalServerError, "", "Webhook processing failed")
				return
			}
			if disputeBusinessID != businessID {
				server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Bill does not belong to this business")
				return
			}
		}
		if disputeBillID > 0 {
			if bill, _, billErr := database.GetBillByIDLean(disputeBillID); billErr != nil {
				if webhookID != "" {
					_ = database.GetDBWrapper().MarkWebhookEventFailed(providerKey, webhookID, truncateWebhookError(billErr.Error(), 1000))
				}
				server.RespondWithError(c, http.StatusInternalServerError, "", "Webhook processing failed")
				return
			} else if bill != nil {
				createPaymentRefundReviewOperationalAlert(context.Background(), bill, txHash, map[string]any{
					"provider_payment_id": response.PaymentID,
					"settlement_source":   "plugin_webhook",
					"provider_status":     response.Status,
					"reconciliation_note": "provider dispute opened; captured money remains settled pending outcome",
				})
			}
		}
	}

	// Cancellation, expiry, and terminal failure close only the matching pending
	// attempt. A late provider event must never regress a confirmed tracker or a
	// captured Payment. ResolvePendingPluginPayment also releases any linked held
	// split share in the same transaction.
	if response.Status == "cancelled" || response.Status == "expired" || response.Status == "failed" {
		target := database.AltPaymentStatusFailed
		switch response.Status {
		case "cancelled":
			target = database.AltPaymentStatusCancelled
		case "expired":
			target = database.AltPaymentStatusExpired
		}
		if _, resolveErr := database.ResolvePendingPluginPayment(effectiveBillID, pluginName, response.PaymentID, target, time.Now()); resolveErr != nil {
			if webhookID != "" {
				_ = database.GetDBWrapper().MarkWebhookEventFailed(providerKey, webhookID, truncateWebhookError(resolveErr.Error(), 1000))
			}
			log.Printf("Failed to resolve pending %s payment %s on bill %d: %v", pluginName, response.PaymentID, effectiveBillID, resolveErr)
			server.RespondWithError(c, http.StatusInternalServerError, "", "Webhook processing failed")
			return
		}
	}

	// Refunds and provider-confirmed reversals withdraw captured money and must
	// reverse the local settlement. Ordinary failures are handled above and only
	// close pending attempts.
	//
	// We distinguish FULL from PARTIAL provider refunds. ReversePluginPayment
	// always reverses the WHOLE local payment, which is correct for a full
	// refund but WRONG for an out-of-band partial
	// refund (e.g. a merchant refunds $10 of a $50 PayPal capture from the PSP
	// dashboard): it would flip the entire $50 payment to reversed and
	// over-reverse the books. The full-vs-partial decision reads response.Amount,
	// whose meaning is PROVIDER-SPECIFIC:
	//   - PayPal's capture-refund webhook sets response.Amount to the REFUNDED
	//     amount in cents, so a partial refund yields 0 < refunded < captured.
	//   - MercadoPago only maps a "refunded" status for FULL refunds (partial MP
	//     refunds keep an "approved"/completed status) and sets response.Amount to
	//     the original captured amount, so refunded == captured and it
	//     classifies as full — which is correct.
	// The captured baseline is the FULL capture (bill + tip), NOT the bill-only
	// payment.Amount — the capture always included any tip, so measuring against
	// bill-only would misclassify a tip-inclusive refund (H9).
	// CONTRACT: any provider routed through here that can emit a "refunded" status
	// for a PARTIAL refund MUST set response.Amount to the refunded portion (not
	// the original capture); otherwise a partial would misclassify as full and be
	// over-reversed. When Amount is unknown/zero, or covers the whole capture, we
	// do the existing full reversal. When it is a strict partial
	// (0 < refunded < captured), apply the proportional ledger reduction and
	// surface a review alert for any fiscal follow-up.
	//
	// If the reversal DB write fails we must NOT 200 — the PSP would skip its
	// retry and our ledger would silently retain the refunded amount as
	// still-paid. Mark the event failed so the provider retries, and respond
	// 500 for the same reason.
	//
	// Providers that report CUMULATIVE totals (RefundedCumulativeCents,
	// DisputedCents) are applied as deltas against the payment's recorded
	// provider totals, so a replay or a later larger total never reverses the
	// same money twice, and a dispute withdraws only the disputed amount and
	// is restored on "dispute_reinstated".
	if response.Status == "refunded" || response.Status == "reversed" || response.Status == "dispute_reinstated" {
		txHash := fmt.Sprintf("plugin_%s", response.PaymentID)

		// The reversal is keyed on txHash and does NOT need the bill id. A provider
		// may drop the bill reference on the refund object, leaving effectiveBillID
		// == 0; gating reversal on effectiveBillID>0 (the old bug) then 200-acked a
		// real refund without reversing, leaving the bill silently 'paid'. Resolve
		// the bill from the local Payment row when the webhook does not carry it,
		// so reversal always applies whenever the refund is attributable to a
		// recorded payment.
		reversalBillID := effectiveBillID
		reversalPayment, reversalPaymentErr := database.GetPaymentByTxHash(txHash)
		if reversalPaymentErr != nil && !errors.Is(reversalPaymentErr, database.ErrPaymentNotFound) {
			// A failed lookup cannot prove the payment belongs to this business,
			// and the reversal below is keyed on tx_hash alone: fail closed so the
			// provider retries instead of mutating an unverified payment.
			if webhookID != "" {
				_ = database.GetDBWrapper().MarkWebhookEventFailed(providerKey, webhookID, truncateWebhookError(reversalPaymentErr.Error(), 1000))
			}
			log.Printf("Failed to load recorded payment %s for %s reversal: %v", response.PaymentID, pluginName, reversalPaymentErr)
			server.RespondWithError(c, http.StatusInternalServerError, "", "Webhook processing failed")
			return
		}
		if reversalPaymentErr == nil && reversalPayment != nil {
			// The ledger is keyed on tx_hash alone, so the webhook's own bill
			// reference cannot be trusted to scope the reversal: business A
			// signing an event with its own bill_id but business B's payment id
			// would pass the ownership check on A's bill and then reverse B's
			// payment. The recorded payment's bill is authoritative.
			if reversalBillID != 0 && reversalPayment.BillID != reversalBillID {
				log.Printf("Rejecting %s reversal for payment %s: webhook bill %d does not match recorded bill %d", pluginName, response.PaymentID, reversalBillID, reversalPayment.BillID)
				server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden,
					"Bill does not belong to this business")
				return
			}
			// Bind the provider too: a payment recorded under another known
			// provider is never reversible by this provider's signed event.
			if recorded := strings.ToLower(strings.TrimSpace(string(reversalPayment.PaymentMethod))); recorded != pluginName &&
				(recorded == "stripe" || recorded == "paypal" || recorded == "mercadopago") {
				log.Printf("Rejecting %s reversal for payment %s: recorded under provider %s", pluginName, response.PaymentID, recorded)
				server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden,
					"Payment does not belong to this provider")
				return
			}
			reversalBillID = reversalPayment.BillID
		}

		// PAY-1 (bill-ref-dropped path): the top guard only ran for
		// effectiveBillID>0. When we recovered the bill from the payment row,
		// re-run the cross-business check so an operator holding business A's
		// webhook secret cannot reverse business B's payment.
		if businessID > 0 && reversalBillID > 0 {
			victimBusinessID, loadErr := database.GetBillBusinessIDByBillID(reversalBillID)
			if loadErr != nil {
				if webhookID != "" {
					_ = database.GetDBWrapper().MarkWebhookEventFailed(providerKey, webhookID, truncateWebhookError(loadErr.Error(), 1000))
				}
				log.Printf("Failed to load bill %d ownership for refund webhook %s: %v", reversalBillID, pluginName, loadErr)
				server.RespondWithError(c, http.StatusInternalServerError, "", "Webhook processing failed")
				return
			}
			if victimBusinessID != businessID {
				server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden,
					"Bill does not belong to this business")
				return
			}
		}

		switch {
		case reversalBillID == 0:
			// No bill from the webhook AND no matching local payment: there is
			// genuinely nothing to reverse (the payment was never recorded here), so
			// a reversal would be a no-op. Ack and move on rather than retry-looping
			// the PSP on an event we cannot attribute.
			log.Printf("Refund/reversal %s webhook for payment %s has no resolvable bill or local payment — nothing to reverse", pluginName, response.PaymentID)
		case response.DisputedCents != nil:
			change, err := database.SetPluginDisputedCents(txHash, *response.DisputedCents)
			if err != nil {
				log.Printf("Failed to apply %s dispute amount for payment %s: %v", pluginName, response.PaymentID, err)
				server.RespondWithError(c, http.StatusInternalServerError, "", "Webhook processing failed")
				return
			}
			if change != 0 {
				ph.alertPluginProviderAdjustment(reversalBillID, txHash, response, map[string]any{
					"disputed_cents":      *response.DisputedCents,
					"ledger_change_cents": change,
					"reconciliation_note": "provider dispute withdrawal/reinstatement applied to ledger",
				})
			}
		case response.Status == "dispute_reinstated":
			// A reinstatement without an amount cannot be applied safely; the
			// plugin contract is to always send DisputedCents with it.
			log.Printf("%s dispute_reinstated for payment %s carried no disputed amount; ignored", pluginName, response.PaymentID)
		case response.Status == "refunded" && response.RefundedCumulativeCents != nil:
			if !ph.applyPluginCumulativeRefund(c, pluginName, reversalBillID, txHash, response) {
				return
			}
		case ph.isPartialPluginRefund(response, txHash):
			// Partial refund: apply a proportional ledger reduction so PaidAmount
			// and revenue aggregates stay truthful, then surface a review alert
			// for operator awareness (fiscal credit notes still need human/outbox
			// follow-up). Prefer plugin-declared RefundedAmountCents when set.
			refundedCents := response.Amount
			if response.RefundedAmountCents != nil {
				refundedCents = *response.RefundedAmountCents
			}
			if err := database.PartialReversePluginPayment(txHash, refundedCents); err != nil {
				if webhookID != "" {
					_ = database.GetDBWrapper().MarkWebhookEventFailed(providerKey, webhookID, truncateWebhookError(err.Error(), 1000))
				}
				log.Printf("Failed partial reverse for %s payment %s: %v", pluginName, response.PaymentID, err)
				server.RespondWithError(c, http.StatusInternalServerError, "", "Webhook processing failed")
				return
			}
			log.Printf("Partial refund applied for %s payment %s on bill %d: refunded %d cents",
				pluginName, response.PaymentID, reversalBillID, refundedCents)
			if bill, _, billErr := database.GetBillByIDLean(reversalBillID); billErr == nil && bill != nil {
				createPaymentRefundReviewOperationalAlert(context.Background(), bill, txHash, map[string]any{
					"provider_payment_id": response.PaymentID,
					"refunded_cents":      refundedCents,
					"original_payment_id": response.PaymentID,
					"settlement_source":   "plugin_webhook",
					"reconciliation_note": "partial refund applied to ledger; review fiscal credit note if required",
					"ledger_applied":      true,
				})
			} else if billErr != nil {
				log.Printf("Failed to load bill %d for partial-refund alert: %v", reversalBillID, billErr)
			}
			// Fall through to the shared MarkWebhookEventProcessed + 200 ack below.
		default:
			if err := database.ReversePluginPayment(txHash); err != nil {
				if webhookID != "" {
					_ = database.GetDBWrapper().MarkWebhookEventFailed(providerKey, webhookID, truncateWebhookError(err.Error(), 1000))
				}
				log.Printf("Failed to reverse payment %s: %v", response.PaymentID, err)
				server.RespondWithError(c, http.StatusInternalServerError, "", "Webhook processing failed")
				return
			}
		}
	}

	// If payment was successful, update the bill status
	if effectiveBillID > 0 && response.Status == "completed" {
		// Rekey FIRST: providers that confirm under a different id than the
		// tracker row (PayPal order → capture id, MP preference → payment id)
		// must have the row renamed before the breakdown lookup keyed by
		// response.PaymentID runs, or the guest's bill/tip split is lost.
		if trackerID := pluginTrackerIDFromWebhookMetadata(response.Metadata); trackerID != "" {
			if err := rekeyTrackedPluginPayment(effectiveBillID, trackerID, response.PaymentID); err != nil {
				notePluginWebhookSettlementFailure(c, businessID, pluginName, "settlement rekey failed")
				if webhookID != "" {
					_ = database.GetDBWrapper().MarkWebhookEventFailed(providerKey, webhookID, truncateWebhookError(err.Error(), 1000))
				}
				log.Printf("Failed to re-key tracked plugin payment for %s bill %d tracker %s payment %s: %v", pluginName, effectiveBillID, trackerID, response.PaymentID, err)
				server.RespondWithError(c, http.StatusInternalServerError, "", "Webhook processing failed")
				return
			}
		}

		localBill, localTip, hasLocal := lookupPluginPaymentBreakdown(effectiveBillID, response.PaymentID)
		paymentAmountCents, tipAmountCents, breakdownErr := pluginWebhookSettlementBreakdownCents(response, localBill, localTip, hasLocal)
		if breakdownErr != nil {
			// The guest was already charged at the PSP but the capture's bill/tip
			// breakdown cannot be reconciled against the webhook amount — e.g. a
			// zero-decimal currency lossily rounded at the provider boundary, or an
			// in-flight intent that predates the FIX A representability guard. Keeping
			// the funds would leave the guest charged with the bill unpaid forever, so
			// auto-refund + alert exactly like the other unsettleable captures. A
			// successful refund acks 200; a failed one keeps a non-2xx so the PSP
			// retries and the operator alert drives manual action.
			if unsettleablePluginCaptureError(breakdownErr) && businessID > 0 {
				if ph.autoRefundUnsettleablePluginCapture(c, paymentPlugin, pluginName, businessID, effectiveBillID, response, providerKey, webhookID, breakdownErr) {
					return
				}
			}
			notePluginWebhookSettlementFailure(c, businessID, pluginName, "settlement breakdown failed")
			if webhookID != "" {
				_ = database.GetDBWrapper().MarkWebhookEventFailed(providerKey, webhookID, truncateWebhookError(breakdownErr.Error(), 1000))
			}
			if respondWithPluginSettlementError(c, breakdownErr) {
				return
			}
			log.Printf("Failed to determine payment breakdown for %s webhook: %v", pluginName, breakdownErr)
			server.RespondWithError(c, http.StatusInternalServerError, "", "Webhook processing failed")
			return
		}

		// Webhooks usually run without staff context (provider callbacks), so
		// this evaluates to nil and leaves ClosedByStaffID unset.
		closingStaffID := server.ExtractStaffIDFromContext(c)

		_, _, settlementErr := ph.updateBillPaymentStatus(effectiveBillID, response.PaymentID, paymentAmountCents, tipAmountCents, response.Currency, pluginName, closingStaffID)
		err = settlementErr
		if err != nil {
			// The guest's money is already captured at the PSP but it cannot be
			// applied to this bill — the bill went terminal (paid by cash
			// meanwhile), the payment exceeds the remaining balance, or a racing
			// payment already settled this split share. In every case keeping the
			// captured funds silently double-charges the guest, so we auto-refund
			// and alert. A SUCCESSFUL auto-refund acks 200: a 4xx would make the
			// PSP retry the webhook and re-attempt the refund (F7). A failed or
			// impossible refund keeps the non-2xx response so the PSP retries and
			// the operator alert drives manual action.
			if unsettleablePluginCaptureError(err) && businessID > 0 {
				if ph.autoRefundUnsettleablePluginCapture(c, paymentPlugin, pluginName, businessID, effectiveBillID, response, providerKey, webhookID, err) {
					return
				}
			}
			notePluginWebhookSettlementFailure(c, businessID, pluginName, "bill settlement failed")
			if webhookID != "" {
				_ = database.GetDBWrapper().MarkWebhookEventFailed(providerKey, webhookID, truncateWebhookError(err.Error(), 1000))
			}
			if respondWithPluginSettlementError(c, err) {
				return
			}
			log.Printf("Failed to update bill payment status: %v", err)
			server.RespondWithError(c, http.StatusInternalServerError, "", "Webhook processing failed")
			return
		}

		// A capture that the provider already partially refunded (MercadoPago
		// keeps status approved + partially_refunded) carries the cumulative
		// refunded total; apply the delta now that the payment row exists.
		if response.RefundedCumulativeCents != nil && *response.RefundedCumulativeCents > 0 {
			if !ph.applyPluginCumulativeRefund(c, pluginName, effectiveBillID, fmt.Sprintf("plugin_%s", response.PaymentID), response) {
				return
			}
		}
	}

	if webhookID != "" {
		if err := database.GetDBWrapper().MarkWebhookEventProcessed(providerKey, webhookID); err != nil {
			log.Printf("Failed to mark webhook as processed for %s: %v", pluginName, err)
		}
	}

	recordPluginWebhookSuccessHealth(businessID, pluginName)
	log.Printf("Successfully processed %s webhook for bill %d", pluginName, effectiveBillID)
	c.JSON(http.StatusOK, gin.H{"message": "Webhook processed successfully"})
}

// applyPluginCumulativeRefund reverses the delta between the provider's
// cumulative refunded total and what the payment already absorbed. It responds
// 500 and returns false when the ledger write fails so the provider retries.
func (ph *PluginHandlers) applyPluginCumulativeRefund(c *gin.Context, pluginName string, billID uint, txHash string, response *plugins.WebhookResponse) bool {
	cumulative := *response.RefundedCumulativeCents
	applied, err := database.ApplyPluginRefundCumulative(txHash, cumulative)
	if err != nil {
		log.Printf("Failed to apply %s cumulative refund for payment %s: %v", pluginName, response.PaymentID, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Webhook processing failed")
		return false
	}
	if applied > 0 {
		log.Printf("Refund applied for %s payment %s on bill %d: %d cents (provider cumulative %d)",
			pluginName, response.PaymentID, billID, applied, cumulative)
		ph.alertPluginProviderAdjustment(billID, txHash, response, map[string]any{
			"refunded_cents":            applied,
			"provider_cumulative_cents": cumulative,
			"original_payment_id":       response.PaymentID,
			"reconciliation_note":       "provider refund applied to ledger; review fiscal credit note if required",
		})
	}
	return true
}

// alertPluginProviderAdjustment raises the refund-review alert for a provider
// refund or dispute already applied to the ledger. Alert failures never fail
// the webhook.
func (ph *PluginHandlers) alertPluginProviderAdjustment(billID uint, txHash string, response *plugins.WebhookResponse, details map[string]any) {
	if billID == 0 {
		return
	}
	bill, _, err := database.GetBillByIDLean(billID)
	if err != nil || bill == nil {
		if err != nil {
			log.Printf("Failed to load bill %d for provider adjustment alert: %v", billID, err)
		}
		return
	}
	details["provider_payment_id"] = response.PaymentID
	details["provider_status"] = response.Status
	details["settlement_source"] = "plugin_webhook"
	details["ledger_applied"] = true
	createPaymentRefundReviewOperationalAlert(context.Background(), bill, txHash, details)
}

// pluginWebhookFailureReasonKey carries a closed failure reason for
// failPluginWebhook. Settlement paths set it so a 4xx/5xx status is not
// collapsed into rejected/internal_error. Pre-claim exits count themselves
// and return before the webhook row is claimed, so they must not also flow
// through failPluginWebhook.
const pluginWebhookFailureReasonKey = "plugin_webhook_failure_reason"

func rejectPluginWebhookBeforeClaim(c *gin.Context, pluginName, reason string, status int, code, message string) {
	metrics.RecordPaymentWebhookFailure(pluginName, reason)
	slog.Warn("payment webhook rejected",
		"plugin", pluginName,
		"reason", reason,
		"status", status,
	)
	server.RespondWithError(c, status, code, message)
}

func notePluginWebhookSettlementFailure(c *gin.Context, businessID uint, pluginName, msg string) {
	c.Set(pluginWebhookFailureReasonKey, metrics.WebhookFailureSettlementFailed)
	recordPluginWebhookFailureHealth(businessID, pluginName, msg)
}

// failPluginWebhookIfUnacknowledged runs after handlePaymentWebhook returns
// for a request that owns its webhook_events row. When the response is non-2xx
// it marks the row failed (unless an exit already did, keeping that more
// specific error), logs a structured line, and counts the failure. A failed
// row is reprocessable, so the provider's redelivery runs the event again
// instead of being acknowledged as a duplicate.
func failPluginWebhook(c *gin.Context, pluginName, providerKey, webhookID string) {
	status := c.Writer.Status()
	reason := metrics.WebhookFailureRejected
	if status >= 500 {
		reason = metrics.WebhookFailureInternal
	}
	if raw, ok := c.Get(pluginWebhookFailureReasonKey); ok {
		if set, ok := raw.(string); ok && strings.TrimSpace(set) != "" {
			reason = strings.TrimSpace(set)
		}
	}
	metrics.RecordPaymentWebhookFailure(pluginName, reason)
	slog.Warn("payment webhook failed",
		"plugin", pluginName,
		"webhook_id", webhookID,
		"status", status,
		"reason", reason,
	)
	if webhookID == "" {
		return
	}
	db := database.GetDBWrapper()
	existing, err := db.GetWebhookEvent(providerKey, webhookID)
	if err != nil {
		slog.Error("payment webhook failure marking lookup failed", "plugin", pluginName, "webhook_id", webhookID, "error", err)
		return
	}
	if existing != nil && (existing.Status == "failed" || existing.Status == database.WebhookEventStatusRetryPending) {
		return
	}
	if err := db.MarkWebhookEventFailed(providerKey, webhookID, reason); err != nil {
		slog.Error("payment webhook failure marking failed", "plugin", pluginName, "webhook_id", webhookID, "error", err)
	}
}

// guardPluginWebhookRow is the deferred exit guard for a webhook request that
// may own its webhook_events row. recovered is the value of recover() in the
// caller's defer. A panic runs this before gin's Recovery writes the 500, so
// the writer still reports the default 200: the row is marked failed (reason
// internal_error) here and the panic is re-raised for Recovery. Otherwise any
// non-2xx exit marks the owned row failed.
func guardPluginWebhookRow(c *gin.Context, owns bool, pluginName, providerKey, webhookID string, recovered any) {
	if recovered != nil {
		if owns {
			c.Set(pluginWebhookFailureReasonKey, metrics.WebhookFailureInternal)
			if !c.Writer.Written() {
				c.Status(http.StatusInternalServerError)
			}
			failPluginWebhook(c, pluginName, providerKey, webhookID)
		}
		panic(recovered)
	}
	if owns {
		failPluginWebhookIfUnacknowledged(c, pluginName, providerKey, webhookID)
	}
}

func failPluginWebhookIfUnacknowledged(c *gin.Context, pluginName, providerKey, webhookID string) {
	if status := c.Writer.Status(); status >= 200 && status < 300 {
		return
	}
	failPluginWebhook(c, pluginName, providerKey, webhookID)
}

// recordPluginWebhookFailureHealth and recordPluginWebhookSuccessHealth record
// per-business plugin health for the dashboard. They run only for authenticated
// webhooks (after signature verification) so a forged webhook can never write
// the health field, and are best-effort: a health-write error is logged and
// never fails webhook processing. businessID == 0 (unresolved) is skipped.
func recordPluginWebhookFailureHealth(businessID uint, pluginName, msg string) {
	if businessID == 0 {
		return
	}
	if err := database.RecordBusinessPluginWebhookFailure(businessID, pluginName, truncateWebhookError(msg, 500)); err != nil {
		log.Printf("Failed to record webhook failure health (business %d, %s): %v", businessID, pluginName, err)
	}
}

func recordPluginWebhookSuccessHealth(businessID uint, pluginName string) {
	if businessID == 0 {
		return
	}
	if err := database.RecordBusinessPluginWebhookSuccess(businessID, pluginName); err != nil {
		log.Printf("Failed to record webhook success health (business %d, %s): %v", businessID, pluginName, err)
	}
}

// isPartialPluginRefund reports whether a refund webhook represents a strict
// PARTIAL refund of the original local payment — i.e. the provider refunded
// less than the full captured amount. It only applies to the "refunded" status:
// a "failed"/voided payment is always a full reversal regardless of any amount
// the webhook carries.
//
// response.Amount's meaning is provider-specific (see the dispatch-site comment
// in handlePaymentWebhook): PayPal sets it to the refunded amount; MercadoPago
// only reports "refunded" for full refunds and sets it to the original capture.
// Any provider that can report "refunded" for a partial MUST populate it with
// the refunded portion, or partials misclassify as full. The decision is conservative:
//   - status != "refunded"            -> full reversal (false)
//   - refunded amount unknown/<=0      -> full reversal (false) [preserve safe default]
//   - original payment not found       -> full reversal (false) [keep today's idempotent no-op path]
//   - refunded >= full capture (bill+tip) -> full reversal (false)
//   - 0 < refunded < full capture (bill+tip) -> PARTIAL (true)
func (ph *PluginHandlers) isPartialPluginRefund(response *plugins.WebhookResponse, txHash string) bool {
	if response == nil || response.Status != "refunded" {
		return false
	}
	payment, err := database.GetPaymentByTxHash(txHash)
	if err != nil || payment == nil {
		// Payment not found (or lookup error): let ReversePluginPayment handle it
		// (it is an idempotent no-op when the payment is missing).
		return false
	}
	// H9: the capture was for bill + tip, so the baseline a provider refund is
	// measured against is the FULL captured amount (payment.Amount is the bill
	// portion only; payment.TipAmount is the tip). Comparing against the bill-only
	// amount misclassified a tip-inclusive refund as a strict partial (or a full
	// bill refund as partial), leaving money captured that was actually returned.
	capturedCents := payment.Amount + payment.TipAmount
	// Prefer the plugin-declared refunded amount when present (explicit contract).
	if response.RefundedAmountCents != nil {
		r := *response.RefundedAmountCents
		return r > 0 && r < capturedCents
	}
	// Legacy fallback: infer from the (per-provider) Amount field.
	refundedCents := response.Amount
	if refundedCents <= 0 {
		// Unknown/zero refund amount: default to the existing full reversal so we
		// never silently skip reversing a refund.
		return false
	}
	return refundedCents < capturedCents
}

var errPluginWebhookSecretMissing = errors.New("plugin webhook secret not configured")

// errPluginWebhookConfigUnknown is returned by resolvePluginWebhookSignature
// when the plugin has no entry in pluginWebhookConfigs. Callers may choose to
// skip verification rather than hard-fail for unregistered plugins.
var errPluginWebhookConfigUnknown = errors.New("no webhook signature config for plugin")

// pluginWebhookConfig holds per-plugin webhook configuration.
type pluginWebhookConfig struct {
	// signatureHeader is the HTTP header that carries the webhook signature.
	signatureHeader string
	// envVars is an ordered list of environment variable names to try when
	// the business plugin config does not supply a webhook_secret.
	envVars []string
}

var pluginWebhookConfigs = map[string]pluginWebhookConfig{
	"stripe": {
		signatureHeader: "Stripe-Signature",
		// STRIPE_CONNECT_WEBHOOK_SECRET first: platform Connect endpoint for
		// OAuth-connected accounts. Per-business webhook_secret (manual) still
		// takes priority when present on the business config row.
		envVars: []string{"STRIPE_CONNECT_WEBHOOK_SECRET", "STRIPE_PLUGIN_WEBHOOK_SECRET", "STRIPE_WEBHOOK_SECRET"},
	},
	"paypal": {
		signatureHeader: "Paypal-Transmission-Sig",
		envVars:         []string{"PAYPAL_WEBHOOK_SECRET"},
	},
	"mercadopago": {
		signatureHeader: "x-signature",
		envVars:         []string{"MERCADOPAGO_WEBHOOK_SECRET", "MERCADOPAGO_WEBHOOK_SECRET_PREVIOUS"},
	},
}

// resolvePluginWebhookSignature reads the signature from request headers and
// loads the webhook secret from the business plugin config (falling back to
// environment variables). It returns both values so the caller can hand them
// to the plugin's VerifyWebhookSignature method.
func resolvePluginWebhookSignature(pluginName string, businessID uint, headers map[string]string) (signature string, secret string, err error) {
	signature, secrets, err := resolvePluginWebhookSignatures(pluginName, businessID, headers)
	if err != nil {
		return "", "", err
	}
	return signature, secrets[0], nil
}

func resolvePluginWebhookSignatures(pluginName string, businessID uint, headers map[string]string) (signature string, secrets []string, err error) {
	cfg, ok := pluginWebhookConfigs[pluginName]
	if !ok {
		return "", nil, fmt.Errorf("%w: %q", errPluginWebhookConfigUnknown, pluginName)
	}

	// Load the webhook secret: business config takes priority over env vars for
	// manual merchants. When businessID is 0 (e.g. PayPal/MercadoPago before the
	// business is resolved) skip the DB lookup and fall straight through to env.
	//
	// Stripe OAuth (connection_mode=oauth) is special: Connect platform events
	// are signed with STRIPE_CONNECT_WEBHOOK_SECRET. Even if a leftover
	// per-business webhook_secret remains (manual→OAuth migration), we MUST
	// still try platform Connect env secrets — verifyWebhookSignatureWithSecrets
	// accepts any matching secret in the list.
	oauthConnect := false
	if businessID > 0 {
		pluginConfig, dbErr := database.GetBusinessPluginConfig(businessID, pluginName)
		if dbErr != nil {
			return "", nil, fmt.Errorf("failed to load %s plugin configuration: %w", pluginName, dbErr)
		}
		mode := strings.ToLower(strings.TrimSpace(stringFromAny(pluginConfig["connection_mode"])))
		oauthConnect = strings.EqualFold(pluginName, "stripe") && mode == "oauth"
		if oauthConnect {
			// Prefer platform Connect secrets for OAuth businesses.
			for _, envVar := range cfg.envVars {
				if secret := strings.TrimSpace(os.Getenv(envVar)); secret != "" {
					secrets = append(secrets, secret)
				}
			}
			for _, envVar := range cfg.envVars {
				if secret := strings.TrimSpace(os.Getenv(envVar + "_PREVIOUS")); secret != "" {
					secrets = append(secrets, secret)
				}
			}
			// Optional per-business secret still accepted if operator set one.
			for _, key := range []string{"webhook_secret", "webhook_secret_previous"} {
				if secret := strings.TrimSpace(stringFromAny(pluginConfig[key])); secret != "" {
					secrets = append(secrets, secret)
				}
			}
		} else {
			for _, key := range []string{"webhook_secret", "webhook_secret_previous"} {
				if secret := strings.TrimSpace(stringFromAny(pluginConfig[key])); secret != "" {
					secrets = append(secrets, secret)
				}
			}
		}
	}
	if len(secrets) == 0 {
		for _, envVar := range cfg.envVars {
			if secret := strings.TrimSpace(os.Getenv(envVar)); secret != "" {
				secrets = append(secrets, secret)
			}
		}
		for _, envVar := range cfg.envVars {
			if secret := strings.TrimSpace(os.Getenv(envVar + "_PREVIOUS")); secret != "" {
				secrets = append(secrets, secret)
			}
		}
	}
	if len(secrets) == 0 {
		return "", nil, errPluginWebhookSecretMissing
	}

	// Deduplicate while preserving order (oauth path may already include env).
	secrets = uniqueNonEmptySecrets(secrets)

	signature = getHeaderValue(headers, cfg.signatureHeader)
	if signature == "" {
		return "", nil, fmt.Errorf("missing %s header", cfg.signatureHeader)
	}

	return signature, secrets, nil
}

func uniqueNonEmptySecrets(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func verifyWebhookSignatureWithSecrets(paymentPlugin plugins.PaymentPlugin, payload []byte, signature string, secrets []string) error {
	for _, secret := range secrets {
		if paymentPlugin.VerifyWebhookSignature(payload, signature, secret) {
			return nil
		}
	}
	return errors.New("signature verification failed")
}

// verifyPluginWebhookSignature resolves the signature and secret for the given
// plugin and verifies the webhook payload against them.
// If a FIRST-PARTY payment provider (stripe, paypal, mercadopago —
// firstPartyRequiresWebhookID) implements neither WebhookRequestVerifier nor has
// an entry in pluginWebhookConfigs, verification FAILS CLOSED (the webhook is
// rejected): it settles real money, so accepting it unverified would let a
// forged payload settle bills. Non-first-party ad-hoc/test plugins keep the
// tolerant (skip-verification) path.
func verifyPluginWebhookSignature(paymentPlugin plugins.PaymentPlugin, pluginName string, businessID uint, payload []byte, headers map[string]string, query url.Values) error {
	if requestVerifier, ok := paymentPlugin.(plugins.WebhookRequestVerifier); ok {
		pluginConfig := map[string]interface{}{}
		if businessID > 0 {
			cfg, err := database.GetBusinessPluginConfig(businessID, pluginName)
			if err != nil {
				return fmt.Errorf("failed to load %s plugin configuration: %w", pluginName, err)
			}
			pluginConfig = cfg
		} else {
			// Unresolved business (e.g. MercadoPago order.* with no local tracker):
			// fall back to env webhook secrets so we can still verify signatures.
			_, secrets, err := resolvePluginWebhookSignatures(pluginName, 0, headers)
			if err != nil {
				return err
			}
			if len(secrets) > 0 {
				pluginConfig["webhook_secret"] = secrets[0]
			}
			if len(secrets) > 1 {
				pluginConfig["webhook_secret_previous"] = secrets[1]
			}
		}
		return requestVerifier.VerifyWebhookRequest(plugins.WebhookVerificationRequest{
			Payload:    payload,
			Headers:    headers,
			Query:      query,
			BusinessID: businessID,
			Config:     pluginConfig,
		})
	}

	signature, secrets, err := resolvePluginWebhookSignatures(pluginName, businessID, headers)
	if err != nil {
		if errors.Is(err, errPluginWebhookConfigUnknown) {
			// Fail closed for any first-party payment provider: it settles real
			// money, so a missing signature config must REJECT the webhook rather
			// than accept it unverified off a forgeable payload. Also fail closed
			// for any non-test plugin that can settle bills in production — only
			// explicitly non-production ad-hoc plugins may skip verification.
			if firstPartyRequiresWebhookID(pluginName) || utils.IsProduction() {
				log.Printf("Rejecting %q webhook: no signature verification config registered", pluginName)
				return fmt.Errorf("%w: %q", errPluginWebhookConfigUnknown, pluginName)
			}
			log.Printf("WARNING: no webhook signature config for plugin %q — skipping verification (non-production only)", pluginName)
			return nil
		}
		return err
	}
	return verifyWebhookSignatureWithSecrets(paymentPlugin, payload, signature, secrets)
}

func paymentWebhookRequiresResolvedBusiness(pluginName string, paymentPlugin plugins.PaymentPlugin) bool {
	// MercadoPago no longer hard-requires a business_id query param: orders-topic
	// notifications resolve business via AlternativePayment tracker (participant_addr
	// = order id). payment.updated without a resolvable business is rejected later
	// in handlePaymentWebhook after the orders-specific accept-ignore branch.
	if strings.EqualFold(strings.TrimSpace(pluginName), "mercadopago") {
		return false
	}
	if _, ok := paymentPlugin.(plugins.WebhookRequestVerifier); ok {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(pluginName)) {
	case "stripe":
		return true
	default:
		return false
	}
}

// firstPartyRequiresWebhookID reports whether a provider MUST present a parsable
// webhook id for replay dedup. The three first-party PSPs always carry one;
// processing without it would silently disable replay protection.
func firstPartyRequiresWebhookID(pluginName string) bool {
	switch pluginName {
	case "stripe", "paypal", "mercadopago":
		return true
	default:
		return false
	}
}

func extractPluginWebhookEventType(pluginName string, payload map[string]interface{}) string {
	switch pluginName {
	case "stripe":
		return stringFromAny(payload["type"])
	case "paypal":
		return stringFromAny(payload["event_type"])
	case "mercadopago":
		return stringFromAny(payload["action"])
	default:
		return stringFromAny(payload["type"])
	}
}

// mercadoPagoOrderActionNeedsTrackerRetry reports order.* actions that settle or
// change money state. Untracked informational notifications (created/updated)
// ACK 200; settle-class actions return 500 so MP retries until the tracker lands.
func mercadoPagoOrderActionNeedsTrackerRetry(action string) bool {
	a := strings.ToLower(strings.TrimSpace(action))
	// Strip optional "order." prefix for matching the status leaf.
	leaf := strings.TrimPrefix(a, "order.")
	switch leaf {
	case "processed", "refunded", "canceled", "cancelled", "expired", "failed",
		"partially_refunded", "charged_back", "chargeback":
		return true
	case "created", "updated", "action_required":
		return false
	default:
		// Unknown order.* action: fail closed (retry) so we never silently drop
		// a future money-state event we do not yet classify.
		return strings.HasPrefix(a, "order.")
	}
}

func extractPluginWebhookID(pluginName string, payload map[string]interface{}) string {
	switch pluginName {
	case "stripe":
		return pluginFirstNonEmpty(
			stringFromAny(payload["id"]),
			stringFromAny(payload["event_id"]),
		)
	case "paypal":
		return pluginFirstNonEmpty(
			stringFromAny(payload["id"]),
			stringFromAny(payload["event_id"]),
		)
	case "mercadopago":
		// MercadoPago redelivers the SAME payment event under DIFFERENT
		// top-level notification ids (payload["id"]/resource_id vary per
		// delivery), so keying dedup on them never matches a replay. The stable
		// identifier is the payment id at payload["data"]["id"] — the same value
		// the plugin reads to load the payment. Fall back to the delivery-scoped
		// ids only when data.id is absent.
		dataID := ""
		if data, ok := payload["data"].(map[string]interface{}); ok {
			dataID = stringFromAny(data["id"])
		}
		// MP reuses data.id across the payment lifecycle (payment.created,
		// payment.updated, payment.refunded, chargebacks…). Dedupe per
		// action so a later lifecycle event is not swallowed as a replay of
		// the first one.
		if action := stringFromAny(payload["action"]); action != "" && dataID != "" {
			return action + ":" + dataID
		}
		return pluginFirstNonEmpty(
			dataID,
			stringFromAny(payload["id"]),
			stringFromAny(payload["resource_id"]),
		)
	default:
		return stringFromAny(payload["id"])
	}
}

// stripeWebhookTenant is the resolved tenant identity of a Stripe webhook.
type stripeWebhookTenant struct {
	// businessID is the single business the event is allowed to act on, or 0
	// when it cannot be determined unambiguously.
	businessID uint
	// account is the connected account (acct_…) Stripe stamped on the delivery,
	// empty for a platform-direct delivery to a manual merchant's endpoint.
	account string
	// accountBusinessIDs is every business currently connected to account.
	accountBusinessIDs []uint
	// metadataMismatch reports that the payload named a business_id that does
	// not own account — i.e. a forged cross-tenant event.
	metadataMismatch bool
	// ambiguous reports that account maps to more than one business and the
	// payload carries no metadata to disambiguate.
	ambiguous bool
}

// stripeWebhookConnectedAccount returns the connected account the event belongs
// to. Only the top-level "account" field is authoritative; data.object.id is
// accepted solely when it is itself an acct_ id (some Connect shapes nest it),
// never the ca_ application id that account.application.deauthorized carries.
func stripeWebhookConnectedAccount(payload map[string]interface{}) string {
	if acct := strings.TrimSpace(stringFromAny(payload["account"])); acct != "" {
		return acct
	}
	if data, ok := payload["data"].(map[string]interface{}); ok {
		if object, ok := data["object"].(map[string]interface{}); ok {
			if id := strings.TrimSpace(stringFromAny(object["id"])); strings.HasPrefix(id, "acct_") {
				return id
			}
		}
	}
	return ""
}

// stripeWebhookMetadataBusinessID reads the business id our own Checkout
// Sessions stamp into metadata. For a Connect delivery this value is written by
// whoever created the session, so it is attacker-controlled and is only ever
// used as a disambiguator that must agree with the connected account.
func stripeWebhookMetadataBusinessID(payload map[string]interface{}) uint {
	data, ok := payload["data"].(map[string]interface{})
	if !ok {
		return 0
	}
	object, ok := data["object"].(map[string]interface{})
	if !ok {
		return 0
	}
	metadata, ok := object["metadata"].(map[string]interface{})
	if !ok {
		return 0
	}
	businessID, ok := uintFromAny(metadata["business_id"])
	if !ok {
		return 0
	}
	return businessID
}

// resolveStripeWebhookTenant maps a Stripe webhook to the business that owns it.
// It is the tenant-isolation boundary for Connect.
//
// Every OAuth-connected merchant's events arrive at the SAME platform endpoint
// signed with the SAME STRIPE_CONNECT_WEBHOOK_SECRET, so a valid signature only
// proves "some connected merchant sent this" — never which one. The connected
// account id is the only tenant identity Stripe itself asserts;
// metadata.business_id is written by whoever created the Checkout Session.
// Trusting metadata alone let any connected merchant settle another merchant's
// bill by paying themselves with a forged business_id.
//
//   - No account field → platform-direct delivery to a manual merchant's own
//     endpoint, verified with that merchant's own webhook_secret. Metadata is
//     the only routing key available there and is not cross-tenant reachable.
//   - Account present → the tenant comes from the account. metadata.business_id
//     is honoured only when it names one of the account's businesses; anything
//     else is flagged as a mismatch for the caller to reject.
//
// The error return is reserved for lookup failures; a payload that simply does
// not resolve yields a zero businessID, never a fallback guess.
func resolveStripeWebhookTenant(payload map[string]interface{}) (stripeWebhookTenant, error) {
	tenant := stripeWebhookTenant{account: stripeWebhookConnectedAccount(payload)}
	metadataBusinessID := stripeWebhookMetadataBusinessID(payload)
	if tenant.account == "" {
		tenant.businessID = metadataBusinessID
		return tenant, nil
	}

	ids, err := database.FindBusinessIDsByStripeUserID(tenant.account)
	if err != nil {
		return tenant, err
	}
	tenant.accountBusinessIDs = ids
	if len(ids) == 0 {
		// Unknown or already-disconnected account. Never fall back to metadata:
		// with a shared Connect secret that would be a free cross-tenant write.
		return tenant, nil
	}
	if metadataBusinessID > 0 {
		for _, id := range ids {
			if id == metadataBusinessID {
				tenant.businessID = id
				return tenant, nil
			}
		}
		tenant.metadataMismatch = true
		return tenant, nil
	}
	if len(ids) > 1 {
		tenant.ambiguous = true
		return tenant, nil
	}
	tenant.businessID = ids[0]
	return tenant, nil
}

func extractPluginWebhookBusinessID(pluginName string, payload map[string]interface{}, query url.Values) uint {
	switch pluginName {
	case "stripe":
		// Tenant identity is decided by resolveStripeWebhookTenant, which binds
		// the event to its connected account instead of to attacker-controlled
		// metadata. A mismatch or ambiguity is "no business" here;
		// handlePaymentWebhook rejects those deliveries outright.
		tenant, err := resolveStripeWebhookTenant(payload)
		if err != nil {
			return 0
		}
		return tenant.businessID
	case "paypal":
		if _, businessID, ok := extractPayPalBillBusinessReferenceFromPayload(payload); ok {
			return businessID
		}
	case "mercadopago":
		if businessID, ok := uintFromAny(query.Get("business_id")); ok {
			return businessID
		}
		// Orders topic (Point/QR): application webhooks omit business_id. Resolve
		// via tracker ParticipantAddr = data.id + payment_method = mercadopago.
		// This is pre-verification routing only; signature is verified before any
		// settlement write using that business's secrets (+ env fallback).
		action := strings.ToLower(strings.TrimSpace(stringFromAny(payload["action"])))
		if strings.HasPrefix(action, "order.") {
			dataID := ""
			if data, ok := payload["data"].(map[string]interface{}); ok {
				dataID = stringFromAny(data["id"])
			}
			if dataID != "" {
				if bizID, err := database.GetBusinessIDByPluginPaymentTracker(dataID, "mercadopago"); err == nil && bizID > 0 {
					return bizID
				}
			}
		}
	}

	return 0
}

func extractPluginWebhookBillID(pluginName string, payload map[string]interface{}, query url.Values) uint {
	switch pluginName {
	case "stripe":
		data, ok := payload["data"].(map[string]interface{})
		if !ok {
			return 0
		}
		object, ok := data["object"].(map[string]interface{})
		if !ok {
			return 0
		}
		metadata, ok := object["metadata"].(map[string]interface{})
		if !ok {
			return 0
		}
		if billID, ok := uintFromAny(metadata["bill_id"]); ok {
			return billID
		}
	case "paypal":
		if billID, _, ok := extractPayPalBillBusinessReferenceFromPayload(payload); ok {
			return billID
		}
	case "mercadopago":
		if billID, ok := uintFromAny(query.Get("bill_id")); ok {
			return billID
		}
	}

	return 0
}

func extractPayPalBillBusinessReferenceFromPayload(payload map[string]interface{}) (uint, uint, bool) {
	resource, ok := payload["resource"].(map[string]interface{})
	if !ok {
		return 0, 0, false
	}
	candidates := []string{
		stringFromAny(resource["custom_id"]),
		stringFromAny(resource["custom"]),
		stringFromAny(resource["invoice_id"]),
		stringFromAny(resource["reference_id"]),
	}
	if units, ok := resource["purchase_units"].([]interface{}); ok {
		for _, unitValue := range units {
			unit, ok := unitValue.(map[string]interface{})
			if !ok {
				continue
			}
			candidates = append(candidates,
				stringFromAny(unit["custom_id"]),
				stringFromAny(unit["invoice_id"]),
				stringFromAny(unit["reference_id"]),
			)
		}
	}
	// A dispute resource carries no custom_id of its own: the purchase unit's
	// custom_id is echoed as "custom" on each disputed transaction.
	if transactions, ok := resource["disputed_transactions"].([]interface{}); ok {
		for _, value := range transactions {
			transaction, ok := value.(map[string]interface{})
			if !ok {
				continue
			}
			candidates = append(candidates,
				stringFromAny(transaction["custom"]),
				stringFromAny(transaction["invoice_number"]),
			)
		}
	}
	for _, candidate := range candidates {
		billID, businessID, ok := parsePluginBillBusinessReference(candidate)
		if ok {
			return billID, businessID, true
		}
	}
	return 0, 0, false
}

func parsePluginBillBusinessReference(reference string) (uint, uint, bool) {
	parts := strings.Split(strings.TrimSpace(reference), "_")
	if len(parts) < 2 || parts[0] != "bill" {
		return 0, 0, false
	}
	billID, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return 0, 0, false
	}
	var businessID uint64
	if len(parts) >= 4 && parts[2] == "business" {
		if parsedBusinessID, err := strconv.ParseUint(parts[3], 10, 32); err == nil {
			businessID = parsedBusinessID
		}
	}
	return uint(billID), uint(businessID), true
}

func uintFromAny(v interface{}) (uint, bool) {
	switch value := v.(type) {
	case float64:
		if value >= 0 {
			return uint(value), true
		}
	case float32:
		if value >= 0 {
			return uint(value), true
		}
	case int:
		if value >= 0 {
			return uint(value), true
		}
	case int64:
		if value >= 0 {
			return uint(value), true
		}
	case uint:
		return value, true
	case uint64:
		return uint(value), true
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return 0, false
		}
		parsed, err := strconv.ParseUint(trimmed, 10, 32)
		if err == nil {
			return uint(parsed), true
		}
	}
	return 0, false
}

func int64FromAny(v interface{}) (int64, bool) {
	switch value := v.(type) {
	case int64:
		return value, true
	case int:
		return int64(value), true
	case int32:
		return int64(value), true
	case uint:
		return int64(value), true
	case uint64:
		return int64(value), true
	case float64:
		return int64(value), true
	case float32:
		return int64(value), true
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return 0, false
		}
		parsed, err := strconv.ParseInt(trimmed, 10, 64)
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func stringFromAny(v interface{}) string {
	if v == nil {
		return ""
	}
	switch value := v.(type) {
	case string:
		return strings.TrimSpace(value)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

func getHeaderValue(headers map[string]string, key string) string {
	if value, ok := headers[key]; ok && strings.TrimSpace(value) != "" {
		return value
	}
	lower := strings.ToLower(key)
	for k, v := range headers {
		if strings.ToLower(k) == lower && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func pluginWebhookSettlementBreakdownCents(response *plugins.WebhookResponse, localBillCents, localTipCents int64, hasLocal bool) (int64, int64, error) {
	if response == nil {
		return 0, 0, errors.New("missing webhook response")
	}

	paymentAmountCents := response.Amount
	tipAmountCents := int64(0)

	// When metadata is present (provider supplied cents keys), honor it with sum validation.
	metadataHasCents := false
	if response.Metadata != nil {
		if _, ok := int64FromAny(response.Metadata["tip_amount_cents"]); ok {
			metadataHasCents = true
		}
		if _, ok := int64FromAny(response.Metadata["bill_amount_cents"]); ok {
			metadataHasCents = true
		}
	}

	if response.Metadata != nil && metadataHasCents {
		if value, ok := int64FromAny(response.Metadata["tip_amount_cents"]); ok {
			tipAmountCents = value
		}
		if value, ok := int64FromAny(response.Metadata["bill_amount_cents"]); ok {
			paymentAmountCents = value
		} else {
			paymentAmountCents = response.Amount - tipAmountCents
		}

		// If local row disagrees with provider metadata, log and keep local.
		if hasLocal && (localBillCents != paymentAmountCents || localTipCents != tipAmountCents) {
			log.Printf("plugin webhook breakdown: provider metadata (%d,%d) disagrees with local row (%d,%d); keeping local",
				paymentAmountCents, tipAmountCents, localBillCents, localTipCents)
			paymentAmountCents = localBillCents
			tipAmountCents = localTipCents
		}
	} else if hasLocal && localBillCents >= 0 && localTipCents >= 0 {
		// No provider cents metadata: the locally initiated tracker remains the
		// authoritative amount binding. A provider capture that differs cannot be
		// reinterpreted as a valid partial payment.
		if localBillCents+localTipCents != response.Amount {
			return 0, 0, errPluginPaymentBreakdownMismatch
		}
		paymentAmountCents = localBillCents
		tipAmountCents = localTipCents
	}

	if tipAmountCents < 0 {
		return 0, 0, database.ErrInvalidTipAmount
	}
	if paymentAmountCents < 0 {
		return 0, 0, database.ErrInvalidPaymentAmount
	}
	if response.Amount > 0 && paymentAmountCents+tipAmountCents != response.Amount {
		return 0, 0, errPluginPaymentBreakdownMismatch
	}

	return paymentAmountCents, tipAmountCents, nil
}

var errPluginPaymentBreakdownMismatch = errors.New("payment breakdown does not match webhook amount")
var errPluginPaymentCurrencyMismatch = errors.New("payment currency does not match business currency")

// unsettleablePluginCaptureError reports whether a settlement error means the
// PSP-captured funds cannot be applied to the bill and must be refunded rather
// than kept. It covers the terminal-bill case (H2), the exceeds-remaining case,
// the double-paid split share (H3), and the breakdown-mismatch case where a
// capture's bill/tip split cannot be reconciled against the webhook amount
// (e.g. a zero-decimal currency lossily rounded at the provider boundary, or an
// in-flight/legacy intent that predates the FIX A representability guard). In
// every case the guest was charged but the funds can never settle, so the
// capture is auto-refunded rather than left stranded.
func unsettleablePluginCaptureError(err error) bool {
	return errors.Is(err, database.ErrPaymentExceedsRemaining) ||
		errors.Is(err, database.ErrBillNotPayable) ||
		errors.Is(err, errPluginSplitShareDoublePaid) ||
		errors.Is(err, errPluginPaymentBreakdownMismatch) ||
		errors.Is(err, errPluginPaymentCurrencyMismatch)
}

// refundUnsettleablePluginCapture refunds a captured plugin payment that could
// not settle against the bill and raises an operator alert. It is transport
// free so both the webhook and the guest return paths share it; source is the
// settlement_source recorded on the alert. It returns the provider refund error
// (nil when the refund succeeded).
func refundUnsettleablePluginCapture(
	paymentPlugin plugins.PaymentPlugin,
	pluginName string,
	businessID uint,
	billID uint,
	response *plugins.WebhookResponse,
	source string,
	settlementErr error,
) error {
	log.Printf("Attempting auto-refund for unsettleable %s capture %s (business %d, bill %d, source %s): %v",
		pluginName, response.PaymentID, businessID, billID, source, settlementErr)

	txHash := fmt.Sprintf("plugin_%s", response.PaymentID)
	refundErr := paymentPlugin.RefundPayment(businessID, response.PaymentID, 0)

	// Best-effort operator alert regardless of refund outcome. The alert is keyed
	// off the capture's synthetic txHash so it points at the local Payment row
	// when one exists (e.g. the reversed double-pay).
	if bill, _, billErr := database.GetBillByIDLean(billID); billErr == nil && bill != nil {
		note := "captured plugin payment could not settle against the bill — auto-refunded at the provider"
		if refundErr != nil {
			note = "captured plugin payment could not settle against the bill AND auto-refund failed — MANUAL REFUND REQUIRED"
		}
		createPaymentRefundReviewOperationalAlert(context.Background(), bill, txHash, map[string]any{
			"provider_payment_id": response.PaymentID,
			"amount_cents":        response.Amount,
			"currency":            response.Currency,
			"settlement_source":   source,
			"settlement_error":    settlementErr.Error(),
			"auto_refunded":       refundErr == nil,
			"reconciliation_note": note,
		})
	} else if billErr != nil {
		log.Printf("Failed to load bill %d for unsettleable-capture alert: %v", billID, billErr)
	}

	if refundErr != nil {
		log.Printf("Auto-refund failed for %s capture %s: %v — manual refund required", pluginName, response.PaymentID, refundErr)
		return refundErr
	}
	log.Printf("Auto-refund succeeded for unsettleable %s capture %s", pluginName, response.PaymentID)
	recordPluginCaptureAutoRefunded(pluginName, response.PaymentID)
	return nil
}

// autoRefundUnsettleablePluginCapture is the webhook wrapper around
// refundUnsettleablePluginCapture. On a successful refund it marks the webhook
// processed, clears the plugin health error, and acks 200 (returning true so
// the caller stops). On a failed or unsupported refund it returns false so the
// caller responds non-2xx and the PSP retries.
func (ph *PluginHandlers) autoRefundUnsettleablePluginCapture(
	c *gin.Context,
	paymentPlugin plugins.PaymentPlugin,
	pluginName string,
	businessID uint,
	billID uint,
	response *plugins.WebhookResponse,
	providerKey string,
	webhookID string,
	settlementErr error,
) bool {
	if refundUnsettleablePluginCapture(paymentPlugin, pluginName, businessID, billID, response, "plugin_webhook", settlementErr) != nil {
		return false
	}
	if webhookID != "" {
		if err := database.GetDBWrapper().MarkWebhookEventProcessed(providerKey, webhookID); err != nil {
			log.Printf("Failed to mark webhook processed after auto-refund for %s: %v", pluginName, err)
		}
	}
	recordPluginWebhookSuccessHealth(businessID, pluginName)
	c.JSON(http.StatusOK, gin.H{"message": "Capture auto-refunded; bill unchanged"})
	return true
}

// returnCaptureCurrencyError reports a mismatch between the provider capture
// currency and the business currency, mirroring the webhook guard.
func returnCaptureCurrencyError(businessID uint, response *plugins.WebhookResponse) error {
	defaultCurrency, _, err := database.GetBusinessDefaults(businessID)
	if err != nil {
		return err
	}
	expected := authoritativeGuestPluginCurrency(&database.Business{DefaultCurrency: defaultCurrency})
	actual := strings.ToUpper(strings.TrimSpace(response.Currency))
	if actual != expected {
		return fmt.Errorf("%w: provider currency %q does not match business currency %q", errPluginPaymentCurrencyMismatch, actual, expected)
	}
	return nil
}

// settleReturnCapture applies a completed return-path capture to the bill. When
// the capture cannot settle (terminal bill, overpay, breakdown or currency
// mismatch) it is refunded at the provider and failed=true is returned so the
// guest is redirected to a failed state instead of "success".
func (ph *PluginHandlers) settleReturnCapture(
	c *gin.Context,
	plugin plugins.Plugin,
	pluginName string,
	bill *database.Bill,
	response *plugins.WebhookResponse,
	localBill, localTip int64,
	hasLocal bool,
) (failed bool) {
	refund := func(settlementErr error) bool {
		// The webhook may have settled this same capture before the return path
		// reached any unsettleable branch. A ledger row for the capture on this
		// bill means the funds landed: never refund it and do not fail the
		// guest. A lookup failure cannot prove the capture is unsettled, so it
		// is left for the webhook rather than refunded.
		existing, lookupErr := database.GetPaymentByTxHash(fmt.Sprintf("plugin_%s", response.PaymentID))
		if lookupErr == nil && existing != nil && existing.BillID == bill.ID {
			return false
		}
		if lookupErr != nil && !errors.Is(lookupErr, database.ErrPaymentNotFound) {
			log.Printf("%s return capture %s ledger check failed for bill %d; not refunding: %v", pluginName, response.PaymentID, bill.ID, lookupErr)
			return false
		}
		paymentPlugin, ok := plugin.(plugins.PaymentPlugin)
		if !ok {
			log.Printf("%s return capture %s unsettleable for bill %d and plugin cannot refund — MANUAL REFUND REQUIRED: %v", pluginName, response.PaymentID, bill.ID, settlementErr)
			return true
		}
		if err := refundUnsettleablePluginCapture(paymentPlugin, pluginName, bill.BusinessID, bill.ID, response, "plugin_return", settlementErr); err != nil {
			log.Printf("%s return capture %s left unsettled and unrefunded for bill %d: %v", pluginName, response.PaymentID, bill.ID, err)
		}
		return true
	}
	if err := returnCaptureCurrencyError(bill.BusinessID, response); err != nil {
		if errors.Is(err, errPluginPaymentCurrencyMismatch) {
			return refund(err)
		}
		// Transient lookup failure: leave the capture for the webhook to settle.
		log.Printf("%s return capture currency check failed for bill %d: %v", pluginName, bill.ID, err)
		return false
	}
	paymentAmountCents, tipAmountCents, breakdownErr := pluginWebhookSettlementBreakdownCents(response, localBill, localTip, hasLocal)
	if breakdownErr != nil {
		log.Printf("%s return capture breakdown rejected for bill %d: %v", pluginName, bill.ID, breakdownErr)
		if unsettleablePluginCaptureError(breakdownErr) {
			return refund(breakdownErr)
		}
		return false
	}
	closingStaffID := server.ExtractStaffIDFromContext(c)
	if _, _, err := ph.updateBillPaymentStatus(bill.ID, response.PaymentID, paymentAmountCents, tipAmountCents, response.Currency, pluginName, closingStaffID); err != nil {
		log.Printf("%s return capture settlement failed for bill %d payment %s: %v", pluginName, bill.ID, response.PaymentID, err)
		if unsettleablePluginCaptureError(err) {
			return refund(err)
		}
	}
	return false
}

func respondWithPluginSettlementError(c *gin.Context, err error) bool {
	switch {
	case errors.Is(err, errPluginPaymentBreakdownMismatch):
		log.Printf("Plugin payment breakdown rejected: %v", err)
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Payment breakdown does not match webhook amount")
	case errors.Is(err, database.ErrPaymentExceedsRemaining):
		log.Printf("Plugin payment exceeds remaining balance: %v", err)
		server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Payment exceeds remaining bill balance")
	case errors.Is(err, database.ErrPaymentTxHashConflict):
		log.Printf("Plugin payment transaction conflict: %v", err)
		server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Payment transaction already recorded")
	case errors.Is(err, database.ErrBillNotPayable):
		log.Printf("Plugin payment rejected for terminal bill: %v", err)
		server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Bill is already paid or closed")
	case errors.Is(err, errPluginSplitShareDoublePaid):
		log.Printf("Plugin payment rejected — split share already settled by another payment: %v", err)
		server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Split share already settled by another payment")
	case errors.Is(err, database.ErrInvalidPaymentAmount):
		log.Printf("Plugin payment amount rejected: %v", err)
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Payment amount must be greater than zero")
	case errors.Is(err, database.ErrInvalidTipAmount):
		log.Printf("Plugin payment tip rejected: %v", err)
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Tip amount cannot be negative")
	default:
		return false
	}
	return true
}

func pluginFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func truncateWebhookError(message string, max int) string {
	if len(message) <= max || max <= 0 {
		return message
	}
	return message[:max]
}

// updateBillPaymentStatus records the confirmed plugin payment and updates the
// bill amounts atomically. It uses ApplyConfirmedPayment so that every
// plugin payment produces a Payment record (audit trail) alongside the bill
// amount update in a single transaction.
//
// pluginName is stored on Payment.PaymentMethod (e.g. "stripe", "paypal") so
// analytics and refunds can attribute the tender correctly. currency is only
// used for logging / notification copy — the wire amount is already cents.
func (ph *PluginHandlers) updateBillPaymentStatus(billID uint, paymentID string, paymentAmountCents, tipAmountCents int64, currency, pluginName string, closingStaffID *uint) (*database.Bill, bool, error) {
	log.Printf("Payment completed - Bill ID: %d, Payment ID: %s, Payment Amount: %d %s, Tip Amount: %d %s, Plugin: %s",
		billID, paymentID, paymentAmountCents, currency, tipAmountCents, currency, pluginName)

	method := strings.TrimSpace(pluginName)
	if method == "" {
		method = "plugin"
	}

	txHash := fmt.Sprintf("plugin_%s", paymentID)
	paymentInput := database.ConfirmedPaymentInput{
		BillID:        billID,
		PayerAddr:     "plugin",
		Amount:        paymentAmountCents,
		TipAmount:     tipAmountCents,
		TxHash:        txHash,
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: method,
	}
	if existingPayment, err := database.GetPaymentByTxHash(txHash); err == nil {
		// Match amounts/status; tolerate legacy rows that stored currency codes
		// in PaymentMethod (e.g. "usd") instead of the plugin name by comparing
		// a method-normalized clone of the existing row.
		matchInput := paymentInput
		if existingPayment.PaymentMethod != "" && existingPayment.PaymentMethod != paymentInput.PaymentMethod {
			// Prefer existing method for the equality check so replays of
			// pre-fix rows still idempotently short-circuit.
			matchInput.PaymentMethod = existingPayment.PaymentMethod
		}
		// A row the provider has since partially refunded or disputed no longer
		// matches the original capture amounts by design; a replayed or
		// refund-carrying "completed" notification is still the same payment.
		providerAdjusted := existingPayment.ProviderRefundedCents > 0 || existingPayment.ProviderDisputedCents > 0
		if existingPayment.BillID != billID || (!providerAdjusted && !database.PaymentMatchesConfirmedInput(existingPayment, matchInput)) {
			return nil, false, fmt.Errorf("%w: %s", database.ErrPaymentTxHashConflict, txHash)
		}
		if err := markTrackedPluginPaymentConfirmed(billID, paymentID); err != nil {
			log.Printf("Failed to sync tracked plugin payment %s for bill %d: %v", paymentID, billID, err)
			return nil, false, fmt.Errorf("failed to confirm tracked plugin payment: %w", err)
		}
		var bill *database.Bill
		if bill, _, billErr := database.GetBillByIDLean(billID); billErr == nil {
			flushPluginPendingMilestones(bill.BusinessID)
			if splitErr := settleTrackedPluginSplitShare(bill, paymentID, txHash, tipAmountCents, method); splitErr != nil && !errors.Is(splitErr, errPluginSplitShareDoublePaid) {
				return bill, false, splitErr
			}
		} else {
			log.Printf("Failed to reload already-recorded plugin bill %d: %v", billID, billErr)
		}
		log.Printf("Plugin payment %s for bill %d was already recorded, skipping", paymentID, billID)
		return bill, false, nil
	} else if !errors.Is(err, database.ErrPaymentNotFound) {
		return nil, false, fmt.Errorf("failed to check plugin payment: %w", err)
	}

	// Get the bill from database — lean scalar load only; ApplyConfirmedPayment
	// re-reads under FOR UPDATE inside the transaction.
	bill, _, err := database.GetBillByIDLean(billID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get bill: %w", err)
	}

	// Closed or already paid bills should not be updated again.
	if bill.Status == database.BillStatusClosed || bill.Status == database.BillStatusPaid {
		return nil, false, fmt.Errorf("%w: bill %d is %s", database.ErrBillNotPayable, billID, bill.Status)
	}

	// Plugin webhooks already provide amounts in cents (see WebhookResponse.Amount).
	// Create a Payment record and update bill amounts atomically.
	updatedBill, applied, err := database.ApplyConfirmedPayment(paymentInput, closingStaffID)
	if err != nil {
		return nil, false, fmt.Errorf("failed to apply plugin payment: %w", err)
	}

	if !applied {
		if err := markTrackedPluginPaymentConfirmed(billID, paymentID); err != nil {
			log.Printf("Failed to sync tracked plugin payment %s for bill %d: %v", paymentID, billID, err)
			return nil, false, fmt.Errorf("failed to confirm tracked plugin payment: %w", err)
		}
		if splitErr := settleTrackedPluginSplitShare(updatedBill, paymentID, txHash, tipAmountCents, method); splitErr != nil && !errors.Is(splitErr, errPluginSplitShareDoublePaid) {
			return updatedBill, false, splitErr
		}
		flushPluginPendingMilestones(updatedBill.BusinessID)
		log.Printf("Plugin payment %s for bill %d was already recorded, skipping", paymentID, billID)
		return updatedBill, false, nil
	}

	if err := markTrackedPluginPaymentConfirmed(billID, paymentID); err != nil {
		log.Printf("Failed to confirm tracked plugin payment %s for bill %d: %v", paymentID, billID, err)
		// Payment applied; leave tracker pending so webhook retry / recon can sync.
		// Still attempt split settle below so guests are not stuck.
	}
	if splitErr := settleTrackedPluginSplitShare(updatedBill, paymentID, txHash, tipAmountCents, method); splitErr != nil {
		if errors.Is(splitErr, errPluginSplitShareDoublePaid) {
			// H3: this payment double-paid a share another payment already settled.
			// Undo the aggregate we just applied so the surplus does not silently
			// pay down other guests' still-owed shares, then surface the sentinel so
			// the webhook handler auto-refunds the capture at the PSP.
			if reverseErr := database.ReversePluginPayment(txHash); reverseErr != nil {
				log.Printf("Failed to reverse double-paid split-share plugin payment %s on bill %d: %v", paymentID, billID, reverseErr)
				return nil, false, fmt.Errorf("failed to reverse double-paid split-share payment: %w", reverseErr)
			}
			if markErr := markTrackedPluginPaymentDoublePaid(billID, paymentID); markErr != nil {
				log.Printf("Failed to mark double-paid plugin tracker %s on bill %d failed: %v", paymentID, billID, markErr)
			}
			return updatedBill, false, errPluginSplitShareDoublePaid
		}
		// Unexpected settle failure after money applied: reverse local payment so
		// webhook can retry cleanly rather than leaving share open + bill advanced.
		if reverseErr := database.ReversePluginPayment(txHash); reverseErr != nil {
			log.Printf("Failed to reverse after split-settle error for %s on bill %d: %v (settle: %v)", paymentID, billID, reverseErr, splitErr)
			return nil, false, fmt.Errorf("split settle failed and reverse failed: %w", splitErr)
		}
		return nil, false, splitErr
	}

	flushPluginPendingMilestones(updatedBill.BusinessID)

	// P2: after an MP settlement on a zero-decimal currency, floor rounding can
	// leave 1–99 platform cents that no longer chargeable at MP. Absorb that
	// dust as an explicit accounting adjustment so the bill can close paid.
	if strings.EqualFold(method, "mercadopago") {
		if absorbed, absErr := absorbMercadoPagoZeroDecimalDust(updatedBill, currency, paymentID); absErr != nil {
			log.Printf("MP zero-decimal dust absorb failed for bill %d payment %s: %v", billID, paymentID, absErr)
		} else if absorbed != nil {
			updatedBill = absorbed
		}
	}

	enqueueTelegramPaymentReceivedNotification(updatedBill, paymentID, paymentAmountCents, tipAmountCents, currency)
	createPaymentReceivedOperationalAlertForTxHash(context.Background(), updatedBill, txHash, map[string]any{
		"provider_payment_id": paymentID,
		"amount_cents":        paymentAmountCents,
		"tip_cents":           tipAmountCents,
		"currency":            currency,
		"method":              "plugin",
		"payment_status":      string(database.PaymentStatusConfirmed),
		"settlement_source":   "plugin_webhook",
	})

	// Every recorded plugin payment (partial or final) announces
	// payment.received plus a bill.updated that embeds the bill row, whose
	// MarshalJSON emits amounts as dollars (money wire contract) — dashboards
	// need the amounts, not just the status flip, and partial settlements were
	// previously invisible until the next poll.
	publishRecordedPaymentSSE(updatedBill, paymentAmountCents, tipAmountCents, "plugin")

	if updatedBill.Status == database.BillStatusPaid {
		log.Printf("Bill %d marked as paid via %s payment %s", billID, currency, paymentID)
		recordCRMSettlementVisitForPaidBill(updatedBill, "plugin")
		enqueueReceiptForPaidBill(updatedBill)
		enqueueFiscalJobForPaidBill(updatedBill, nil, "plugin")
	} else {
		log.Printf("Partial payment received for bill %d: %.2f %s of %.2f total",
			billID, float64(paymentAmountCents)/100.0, currency, float64(bill.TotalAmount)/100.0)
	}

	if sharedDeliveryService != nil {
		if err := sharedDeliveryService.HandleDeliveryBillPaid(updatedBill.ID); err != nil {
			log.Printf("delivery payment hook failed for bill %d: %v", updatedBill.ID, err)
		}
	}

	return updatedBill, true, nil
}

func enqueueTelegramPaymentReceivedNotification(bill *database.Bill, paymentID string, paymentAmountCents, tipAmountCents int64, currency string) {
	if bill == nil {
		return
	}
	// Same eligibility gate as orders / manual payments / inventory /
	// reservations: businesses without a connected Telegram config (or with
	// the event disabled) must not get outbox rows enqueued into the void.
	if !services.ShouldEnqueueTelegramNotification(bill.BusinessID, services.PluginEventPaymentReceived) {
		return
	}
	payload := map[string]interface{}{
		"payment_id":     paymentID,
		"bill_id":        bill.ID,
		"bill_number":    bill.BillNumber,
		"amount_cents":   paymentAmountCents,
		"tip_cents":      tipAmountCents,
		"currency":       currency,
		"payment_method": "plugin",
		"provider":       "plugin",
		"status":         string(database.PaymentStatusConfirmed),
		"confirmed_at":   time.Now().UTC(),
	}
	if _, _, err := services.EnqueuePluginNotification(services.PluginNotificationEvent{
		BusinessID: bill.BusinessID,
		EventType:  services.PluginEventPaymentReceived,
		EventID:    fmt.Sprintf("payment:%s", paymentID),
		Payload:    payload,
		CreatedAt:  time.Now().UTC(),
	}, "telegram"); err != nil {
		log.Printf("Failed to enqueue Telegram payment notification for business_id=%d bill_id=%d payment_id=%s: %v", bill.BusinessID, bill.ID, paymentID, err)
	}
}

func flushPluginPendingMilestones(businessID uint) {
	milestoneTracker := services.NewMilestoneTracker(database.GetDBWrapper())
	if err := milestoneTracker.ProcessPendingMilestones(businessID); err != nil {
		log.Printf("Failed to flush plugin pending milestones: %v", err)
	}
}

func markTrackedPluginPaymentConfirmed(billID uint, paymentID string) error {
	now := time.Now()
	result := database.GetDB().Model(&database.AlternativePayment{}).
		Where("bill_id = ? AND participant_addr = ?", billID, paymentID).
		Updates(map[string]interface{}{
			"status":       database.AltPaymentStatusConfirmed,
			"confirmed_by": "plugin_webhook",
			"confirmed_at": &now,
			"updated_at":   now,
		})
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// mpDustLedgerCreate inserts the ManualLedgerEntry for a zero-decimal dust
// write-off. Overridable in tests to force insert failure (atomic absorb path).
var mpDustLedgerCreate = func(tx *gorm.DB, entry *database.ManualLedgerEntry) error {
	return tx.Create(entry).Error
}

// absorbMercadoPagoZeroDecimalDust closes a bill when MP floor-rounding left a
// sub-major-unit remainder (1–99 platform cents) on a zero-decimal currency.
// Creates a synthetic Payment (method mercadopago_rounding) plus a ManualLedgerEntry
// expense in ONE transaction so a failed ledger insert never leaves a closed-paid
// bill without books. If either step fails, neither persists; the prior MP
// settlement still succeeds and the bill stays partial (recoverable).
// Returns the refreshed bill when dust was absorbed, (nil, nil) when no-op.
func absorbMercadoPagoZeroDecimalDust(bill *database.Bill, currency, paymentID string) (*database.Bill, error) {
	if bill == nil {
		return nil, nil
	}
	remaining := bill.TotalAmount - bill.PaidAmount
	if !mercadopago.ShouldAbsorbSettlementDust(remaining, currency) {
		return nil, nil
	}
	if bill.Status == database.BillStatusPaid || bill.Status == database.BillStatusClosed {
		return nil, nil
	}

	now := time.Now().UTC()
	dustTxHash := fmt.Sprintf("plugin_mp_dust_%s", strings.TrimSpace(paymentID))
	dustInput := database.ConfirmedPaymentInput{
		BillID:        bill.ID,
		PayerAddr:     "system",
		Amount:        remaining,
		TipAmount:     0,
		TxHash:        dustTxHash,
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "mercadopago_rounding",
	}
	// Accounting visibility: expense write-off of uncollectible sub-unit remainder.
	ref := fmt.Sprintf("mp_dust:bill:%d:pay:%s", bill.ID, strings.TrimSpace(paymentID))
	var updated *database.Bill

	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		billAfter, _, applyErr := database.ApplyConfirmedPaymentInTx(tx, dustInput, nil)
		if applyErr != nil {
			return fmt.Errorf("apply MP rounding payment: %w", applyErr)
		}
		if billAfter != nil {
			updated = billAfter
		} else {
			updated = bill
		}

		var existing int64
		if err := tx.Model(&database.ManualLedgerEntry{}).
			Where("business_id = ? AND reference = ?", bill.BusinessID, ref).
			Count(&existing).Error; err != nil {
			return fmt.Errorf("check MP dust ledger entry: %w", err)
		}
		if existing == 0 {
			entry := database.ManualLedgerEntry{
				BusinessID:  bill.BusinessID,
				EntryType:   database.AccountingEntryTypeExpense,
				Category:    "rounding",
				Amount:      remaining,
				Currency:    strings.ToUpper(strings.TrimSpace(currency)),
				OccurredAt:  now,
				Description: "Mercado Pago zero-decimal currency rounding adjustment",
				Notes: fmt.Sprintf(
					"Absorbed %d cents remaining after MP settlement on bill %d (payment %s); not collectable as a whole major unit",
					remaining, bill.ID, paymentID,
				),
				Reference: ref,
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := mpDustLedgerCreate(tx, &entry); err != nil {
				return fmt.Errorf("create MP dust ledger entry: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		// Neither dust payment nor ledger persisted; bill remains partial.
		return nil, err
	}

	log.Printf("Absorbed MP zero-decimal dust of %d cents on bill %d (payment %s, currency %s)",
		remaining, bill.ID, paymentID, currency)
	return updated, nil
}

// errPluginSplitShareDoublePaid signals that a confirmed plugin payment targeted
// a split share that a DIFFERENT payment already settled — a double-pay. The
// caller (updateBillPaymentStatus) reverses the just-applied local payment and
// the webhook handler auto-refunds the surplus at the PSP so it never bleeds
// into the other guests' still-owed shares.
var errPluginSplitShareDoublePaid = errors.New("plugin split share already settled by another payment")

// settleTrackedPluginSplitShare links a confirmed plugin payment to its split
// share. It returns nil for regular (non-split) plugin payments and for the
// idempotent re-settle of the SAME payment. It returns errPluginSplitShareDoublePaid
// when the share was already finalized by a DIFFERENT payment (H3), so the
// caller can undo the double charge.
// markTrackedPluginPaymentDoublePaid flips a tracker row to failed after its
// capture was reversed as a double-pay, so reconciliation and the operator UI
// do not treat it as an outstanding pending payment.
func markTrackedPluginPaymentDoublePaid(billID uint, paymentID string) error {
	now := time.Now()
	return database.GetDB().Model(&database.AlternativePayment{}).
		Where("bill_id = ? AND participant_addr = ?", billID, paymentID).
		Updates(map[string]interface{}{
			"status":     database.AltPaymentStatusFailed,
			"updated_at": now,
		}).Error
}

func settleTrackedPluginSplitShare(bill *database.Bill, paymentID string, txHash string, tipAmountCents int64, tender string) error {
	if bill == nil || strings.TrimSpace(paymentID) == "" {
		return nil
	}
	var tracker database.AlternativePayment
	if err := database.GetDB().
		Select("id", "bill_id", "participant_addr", "participant_name").
		Where("bill_id = ? AND participant_addr = ?", bill.ID, paymentID).
		First(&tracker).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("Failed to load plugin split tracker for bill %d payment %s: %v", bill.ID, paymentID, err)
		}
		return nil
	}
	shareID, ok := splitShareIDFromPluginTrackerName(tracker.ParticipantName)
	if !ok {
		return nil
	}
	payment, err := database.GetPaymentByTxHash(txHash)
	if err != nil {
		log.Printf("Failed to load plugin split payment row for bill %d payment %s: %v", bill.ID, paymentID, err)
		return nil
	}
	if _, err := database.MarkBillSplitShareSettledByPayment(database.MarkBillSplitShareSettledByPaymentInput{
		ShareID:        shareID,
		BillID:         bill.ID,
		PaymentID:      payment.ID,
		TipCents:       tipAmountCents,
		Tender:         tender,
		IdempotencyKey: paymentID,
		Now:            time.Now().UTC(),
	}); err != nil {
		// The share was already finalized by a DIFFERENT payment (concurrent
		// double-intent), or the payment's amount does not match the share
		// (F6 guard): this is a double-pay, not a benign replay. Signal it so the
		// caller reverses the local payment and auto-refunds the surplus.
		if errors.Is(err, database.ErrSplitShareAlreadyFinal) ||
			errors.Is(err, database.ErrPaymentAlreadySettledForBill) ||
			errors.Is(err, database.ErrSplitAmountUnavailable) {
			log.Printf("Plugin payment %s double-pays already-settled split share %d on bill %d: %v", paymentID, shareID, bill.ID, err)
			return errPluginSplitShareDoublePaid
		}
		log.Printf("Failed to settle split share %d for plugin payment %s: %v", shareID, paymentID, err)
		// Fail closed: the payment row was applied but the share is still open.
		// Returning an error lets the webhook path reverse/auto-refund instead of
		// leaving guests looking unpaid while the bill already advanced.
		return fmt.Errorf("settle split share %d for payment %s: %w", shareID, paymentID, err)
	}
	if state, err := database.GetBillSplitStateByBillID(bill.ID, time.Now().UTC()); err == nil {
		PublishGuestSplitState(state)
	}
	return nil
}
