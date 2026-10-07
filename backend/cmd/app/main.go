package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/stdevmac/payverge/backend/internal/accounting"
	"github.com/stdevmac/payverge/backend/internal/activation"
	"github.com/stdevmac/payverge/backend/internal/adminruntime"
	"github.com/stdevmac/payverge/backend/internal/agents"
	"github.com/stdevmac/payverge/backend/internal/agents/ops_tools"
	"github.com/stdevmac/payverge/backend/internal/analytics"
	"github.com/stdevmac/payverge/backend/internal/auth"
	"github.com/stdevmac/payverge/backend/internal/auththrottle"
	"github.com/stdevmac/payverge/backend/internal/blockchain"
	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/crm"
	"github.com/stdevmac/payverge/backend/internal/demo"
	"github.com/stdevmac/payverge/backend/internal/demomode"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/guardrails"
	"github.com/stdevmac/payverge/backend/internal/handlers"
	"github.com/stdevmac/payverge/backend/internal/health"
	"github.com/stdevmac/payverge/backend/internal/jobs"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/llm/openrouter"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/middleware"
	"github.com/stdevmac/payverge/backend/internal/notifications"
	"github.com/stdevmac/payverge/backend/internal/observability"
	"github.com/stdevmac/payverge/backend/internal/s3"
	"github.com/stdevmac/payverge/backend/internal/session"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/escalation"
	"github.com/stdevmac/payverge/backend/internal/fiscal"
	fiscaldelivery "github.com/stdevmac/payverge/backend/internal/fiscal/delivery"
	"github.com/stdevmac/payverge/backend/internal/fiscal/providers"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/runtimecontrol"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/services"
	"github.com/stdevmac/payverge/backend/internal/services/cryptorefund"
	"github.com/stdevmac/payverge/backend/internal/services/director_tools"
	"github.com/stdevmac/payverge/backend/internal/services/foodcost"
	"github.com/stdevmac/payverge/backend/internal/services/marketing"
	"github.com/stdevmac/payverge/backend/internal/services/menuengineering"
	operational_alerts "github.com/stdevmac/payverge/backend/internal/services/operational_alerts"
	printsvc "github.com/stdevmac/payverge/backend/internal/services/print"
	"github.com/stdevmac/payverge/backend/internal/services/reorgwatch"
	"github.com/stdevmac/payverge/backend/internal/spaces/scan"
	"github.com/stdevmac/payverge/backend/internal/structs"
	"github.com/stdevmac/payverge/backend/internal/telegram"
	"github.com/stdevmac/payverge/backend/internal/utils"

	// Import plugin packages to trigger their init() functions
	"github.com/stdevmac/payverge/backend/internal/plugins"
	mercadopago "github.com/stdevmac/payverge/backend/internal/plugins/mercadopago"
	_ "github.com/stdevmac/payverge/backend/internal/plugins/paypal"
	stripeplugin "github.com/stdevmac/payverge/backend/internal/plugins/stripe"
	telegramPlugin "github.com/stdevmac/payverge/backend/internal/plugins/telegram"
	_ "github.com/stdevmac/payverge/backend/internal/plugins/trustpilot"

	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/robfig/cron/v3"
)

func intEnv(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func installAssistantTelemetrySink() {
	llm.SetTelemetrySink(func(event llm.AITelemetryEvent) {
		_ = metrics.RecordAssistantTerminal(event)
	})
}

// reorgAlerter adapts the reorg reconciler to the operational-alerts system: a
// suspected-reorg signal becomes a high-priority operator alert (which also
// publishes the permission-mapped alert.created SSE frame). Best-effort — a
// failed alert is logged, never fatal to the sweep.
type reorgAlerter struct{}

func (reorgAlerter) ReorgSuspected(ctx context.Context, ev reorgwatch.ReorgAlert) {
	meta := map[string]any{
		"tx_hash":           ev.TxHash,
		"bill_id":           ev.BillID,
		"payment_id":        ev.PaymentID,
		"record_id":         ev.RecordID,
		"stored_block_hash": ev.StoredBlockHash,
		"detail":            ev.Detail,
	}
	if err := operational_alerts.NewService(database.GetDB()).CreateReorgSuspectedAlert(
		ctx, ev.BusinessID, ev.RecordID, string(ev.Kind), meta); err != nil {
		logger.Logger.Warnf("reorgwatch: create alert for %s %d: %v", ev.Kind, ev.RecordID, err)
	}
}

// readinessComponentNames is the fixed set of production-preflight components
// surfaced on /health/ready. Keep in sync with config.ValidateProduction
// component tags (jwt, plugin, network, email, storage, proxy, ai_budget).
// recurringEntriesScheduler is package-scoped so the graceful-shutdown block
// can Stop() it; nil when it never started (route wiring disabled or Start failed).
var recurringEntriesScheduler *services.RecurringEntriesScheduler

var readinessComponentNames = []string{
	"jwt",
	"plugin",
	"network",
	"email",
	"storage",
	"proxy",
	"ai_budget",
}

// readinessComponentsFromPreflight maps a non-secret config.Report into
// health.ComponentResult entries. Mapping lives in main (not health/config)
// to avoid an import cycle. A component is "failed" with the first matching
// issue Code if the report has an issue for that Component; otherwise "ok".
func readinessComponentsFromPreflight(report config.Report) []health.ComponentResult {
	firstCode := make(map[string]string, len(readinessComponentNames))
	for _, issue := range report.Issues() {
		if _, seen := firstCode[issue.Component]; !seen {
			firstCode[issue.Component] = issue.Code
		}
	}
	out := make([]health.ComponentResult, 0, len(readinessComponentNames))
	for _, name := range readinessComponentNames {
		if code, failed := firstCode[name]; failed {
			out = append(out, health.ComponentResult{
				Component: name,
				Status:    "failed",
				Code:      code,
				Source:    "config",
			})
			continue
		}
		out = append(out, health.ComponentResult{
			Component: name,
			Status:    "ok",
			Source:    "config",
		})
	}
	return out
}

// envOrDefault returns the trimmed env value, or fallback when it is unset or
// blank. Server --db-* flags default to their DB_* env vars, so a plain
// `server` with the documented env reaches the same database the CLI
// subcommands resolve (an explicit flag still wins).
func envOrDefault(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func main() {
	// Maintenance subcommands (admin create/reset-password, demo seed/reset)
	// run before flag parsing and exit without starting the server.
	if handled, code := runCLI(os.Args[1:], os.Stdin, os.Stdout, os.Stderr); handled {
		os.Exit(code)
	}

	// Get flags and initialize the database
	telegramTokenDefault := strings.TrimSpace(os.Getenv("TELEGRAM_TOKEN"))
	if telegramTokenDefault == "" {
		telegramTokenDefault = strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	}

	var (
		dbHost                    = flag.String("db-host", envOrDefault("DB_HOST", "localhost"), "PostgreSQL database host")
		dbPort                    = flag.String("db-port", envOrDefault("DB_PORT", "5432"), "PostgreSQL database port")
		dbUser                    = flag.String("db-user", envOrDefault("DB_USER", "payverge"), "PostgreSQL database user")
		dbPassword                = flag.String("db-password", "", "PostgreSQL database password")
		dbName                    = flag.String("db-name", envOrDefault("DB_NAME", "payverge"), "PostgreSQL database name")
		dbSSLMode                 = flag.String("db-sslmode", envOrDefault("DB_SSLMODE", "require"), "PostgreSQL SSL mode")
		production                = flag.Bool("production", false, "Production mode")
		rpcUrl                    = flag.String("rpc-url", "", "RPC URL for the Ethereum node")
		telegramToken             = flag.String("telegram-token", telegramTokenDefault, "Telegram token")
		telegramWebhookSecret     = flag.String("telegram-webhook-secret", strings.TrimSpace(os.Getenv("TELEGRAM_WEBHOOK_SECRET")), "Telegram webhook secret token")
		telegramWorkerConcurrency = flag.Int("telegram-notification-worker-concurrency", intEnv("TELEGRAM_NOTIFICATION_WORKER_CONCURRENCY", 2), "Telegram notification delivery worker concurrency")
		s3Bucket                  = flag.String("s3-bucket", "", "AWS S3 bucket name")
		awsAccessKey              = flag.String("aws-access-key", "", "AWS Access Key ID")
		awsSecretKey              = flag.String("aws-secret-key", "", "AWS Secret Access Key")
		awsRegion                 = flag.String("aws-region", "us-east-1", "AWS Region")
		s3EndpointURL             = flag.String("s3-endpoint", "", "S3 Endpoint URL (optional)")
		s3PublicBaseURL           = flag.String("s3-public-base-url", "", "Public base URL for stored objects, e.g. https://media.example.com (optional; needed when the read host differs from the S3 write endpoint, as with Cloudflare R2 custom domains)")
		s3ProtectedBucket         = flag.String("s3-protected-bucket", "", "AWS S3 bucket name")
		awsProtectedAccessKey     = flag.String("aws-protected-access-key", "", "AWS Access Key ID")
		awsProtectedSecretKey     = flag.String("aws-protected-secret-key", "", "AWS Secret Access Key")
		s3ProtectedEndpointURL    = flag.String("s3-protected-endpoint", "", "S3 Endpoint URL (optional)")
		s3ProtectedBaseURL        = flag.String("s3-protected-base-url", "", "Base URL override for protected object location strings (optional; does not make objects public)")
		fromEmail                 = flag.String("from-email", "", "From email")
		fromEmailUpdates          = flag.String("from-email-updates", "", "From email updates")
		emailProvider             = flag.String("email-provider", "", "Email provider: log|smtp|resend|postmark (env: EMAIL_PROVIDER; unset = resend/postmark when their API key is present, else log)")
		emailAPIKey               = flag.String("email-api-key", "", "Email provider API key (env: EMAIL_API_KEY)")
		googleTranslateAPIKey     = flag.String("google-translate-api-key", "", "Google Translate API Key")
		templatesDir              = flag.String("templates-dir", "email/templates", "Path to email templates directory")
		vapidPublicKey            = flag.String("vapid-public-key", strings.TrimSpace(os.Getenv("VAPID_PUBLIC_KEY")), "VAPID public key for Web Push")
		vapidPrivateKey           = flag.String("vapid-private-key", strings.TrimSpace(os.Getenv("VAPID_PRIVATE_KEY")), "VAPID private key for Web Push")
		vapidSubject              = flag.String("vapid-subject", strings.TrimSpace(os.Getenv("VAPID_SUBJECT")), "VAPID subject (mailto: or https: URL) for Web Push")
	)
	flag.Parse()

	// Telegram enablement is derived from credential presence: the worker runs
	// whenever a bot token is configured, and the webhook additionally requires
	// its secret. There is no separate on/off flag.
	telegramWorkerEnabled := strings.TrimSpace(*telegramToken) != ""
	telegramWebhookEnabled := telegramWorkerEnabled && strings.TrimSpace(*telegramWebhookSecret) != ""

	// Initialize structured logging first, before any logger.Logger.* calls
	logger.InitLogger()

	// Initialize Sentry error tracking with the observability hardening layer
	// (scrubbers, request-context tagging, log hook). No-op unless SENTRY_DSN is
	// set and the environment resolves to staging/production (or SENTRY_ENABLED=true).
	sentryClient := observability.DisabledClient()
	if initialized, err := observability.Init(observability.LoadConfigFromEnv("backend"), logger.Logger); err != nil {
		logger.Logger.Warnf("Sentry init failed: %v", err)
	} else {
		sentryClient = initialized
	}
	defer sentryClient.Flush(2 * time.Second)

	if sentryClient.Enabled() {
		logger.Logger.Info("Sentry initialized")
		logger.SetPanicReporter(func(name string, r interface{}, stack []byte) {
			hub := sentry.CurrentHub().Clone()
			scope := hub.Scope()
			scope.SetTag("goroutine", name)
			scope.AddAttachment(&sentry.Attachment{
				Filename:    "goroutine-stack.txt",
				ContentType: "text/plain",
				Payload:     stack,
			})
			if err, ok := r.(error); ok {
				hub.CaptureException(err)
			} else {
				hub.CaptureException(fmt.Errorf("panic in %s: %v", name, r))
			}
		})
	}

	productionMode := config.IsProductionMode(*production)
	config.SetProductionModeOverride(productionMode)
	utils.SetProductionOverride(productionMode)
	applyEmailFlagEnv(*emailProvider, *emailAPIKey)

	// Resolve production preflight inputs once before any external clients or
	// migrations. Values mirror the same flags/env main already uses later.
	emailProviderResolved := strings.TrimSpace(*emailProvider)
	if emailProviderResolved == "" {
		emailProviderResolved = strings.TrimSpace(os.Getenv("EMAIL_PROVIDER"))
	}
	emailAPIKeyResolved := strings.TrimSpace(*emailAPIKey)
	if emailAPIKeyResolved == "" {
		emailAPIKeyResolved = strings.TrimSpace(os.Getenv("EMAIL_API_KEY"))
	}
	fromEmailResolved := strings.TrimSpace(*fromEmail)
	if fromEmailResolved == "" {
		fromEmailResolved = strings.TrimSpace(os.Getenv("FROM_EMAIL"))
	}
	fromEmailUpdatesResolved := strings.TrimSpace(*fromEmailUpdates)
	if fromEmailUpdatesResolved == "" {
		fromEmailUpdatesResolved = strings.TrimSpace(os.Getenv("FROM_EMAIL_UPDATES"))
	}
	s3BucketResolved := strings.TrimSpace(*s3Bucket)
	if s3BucketResolved == "" {
		s3BucketResolved = strings.TrimSpace(os.Getenv("S3_BUCKET"))
	}
	awsAccessKeyResolved := strings.TrimSpace(*awsAccessKey)
	if awsAccessKeyResolved == "" {
		awsAccessKeyResolved = strings.TrimSpace(os.Getenv("AWS_ACCESS_KEY"))
	}
	awsSecretKeyResolved := strings.TrimSpace(*awsSecretKey)
	if awsSecretKeyResolved == "" {
		awsSecretKeyResolved = strings.TrimSpace(os.Getenv("AWS_SECRET_KEY"))
	}
	s3EndpointResolved := strings.TrimSpace(*s3EndpointURL)
	if s3EndpointResolved == "" {
		s3EndpointResolved = strings.TrimSpace(os.Getenv("S3_ENDPOINT"))
	}
	s3PublicBaseURLResolved := strings.TrimSpace(*s3PublicBaseURL)
	if s3PublicBaseURLResolved == "" {
		s3PublicBaseURLResolved = strings.TrimSpace(os.Getenv("S3_PUBLIC_BASE_URL"))
	}
	s3ProtectedBucketResolved := strings.TrimSpace(*s3ProtectedBucket)
	if s3ProtectedBucketResolved == "" {
		s3ProtectedBucketResolved = strings.TrimSpace(os.Getenv("S3_PROTECTED_BUCKET"))
	}
	awsProtectedAccessKeyResolved := strings.TrimSpace(*awsProtectedAccessKey)
	if awsProtectedAccessKeyResolved == "" {
		awsProtectedAccessKeyResolved = strings.TrimSpace(os.Getenv("AWS_PROTECTED_ACCESS_KEY"))
	}
	awsProtectedSecretKeyResolved := strings.TrimSpace(*awsProtectedSecretKey)
	if awsProtectedSecretKeyResolved == "" {
		awsProtectedSecretKeyResolved = strings.TrimSpace(os.Getenv("AWS_PROTECTED_SECRET_KEY"))
	}
	s3ProtectedEndpointResolved := strings.TrimSpace(*s3ProtectedEndpointURL)
	if s3ProtectedEndpointResolved == "" {
		s3ProtectedEndpointResolved = strings.TrimSpace(os.Getenv("S3_PROTECTED_ENDPOINT"))
	}
	rpcURLResolved := strings.TrimSpace(*rpcUrl)
	if rpcURLResolved == "" {
		rpcURLResolved = strings.TrimSpace(os.Getenv("RPC_URL"))
	}
	// SEC-1: these two were the only secrets with no env fallback, which forced
	// Compose to pass them as argv — where they are readable from
	// /proc/<pid>/cmdline and echoed by `docker inspect`. Same flag-then-env
	// precedence as every resolution above, so an explicit --db-password still
	// wins for local/CLI use.
	dbPasswordResolved := strings.TrimSpace(*dbPassword)
	if dbPasswordResolved == "" {
		dbPasswordResolved = strings.TrimSpace(os.Getenv("DB_PASSWORD"))
	}
	googleTranslateAPIKeyResolved := strings.TrimSpace(*googleTranslateAPIKey)
	if googleTranslateAPIKeyResolved == "" {
		googleTranslateAPIKeyResolved = strings.TrimSpace(os.Getenv("GOOGLE_TRANSLATE_API_KEY"))
	}
	emailLogContent, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("EMAIL_LOG_CONTENT")))
	emailLogAllowProduction, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("EMAIL_PROVIDER_LOG_ALLOW_PRODUCTION")))

	preflightInputs := config.ProductionInputs{
		Production: productionMode,
		DBPassword: dbPasswordResolved,

		JWTSecretKey:    strings.TrimSpace(os.Getenv("JWT_SECRET_KEY")),
		PluginSecretKey: strings.TrimSpace(os.Getenv("PLUGIN_SECRET_KEY")),
		RPCURL:          rpcURLResolved,

		EmailProvider:           emailProviderResolved,
		EmailAPIKey:             emailAPIKeyResolved,
		FromEmail:               fromEmailResolved,
		FromEmailUpdates:        fromEmailUpdatesResolved,
		EmailAllowedFromDomains: strings.TrimSpace(os.Getenv("EMAIL_ALLOWED_FROM_DOMAINS")),
		EmailWebhookSecret:      strings.TrimSpace(os.Getenv("RESEND_WEBHOOK_SECRET")),
		EmailSPFDomain:          strings.TrimSpace(os.Getenv("EMAIL_SPF_DOMAIN")),
		EmailDKIMDomain:         strings.TrimSpace(os.Getenv("EMAIL_DKIM_DOMAIN")),
		EmailReturnPathDomain:   strings.TrimSpace(os.Getenv("EMAIL_RETURN_PATH_DOMAIN")),
		EmailDMARCPolicy:        strings.TrimSpace(os.Getenv("EMAIL_DMARC_POLICY")),
		EmailLogContent:         emailLogContent,
		EmailLogAllowProduction: emailLogAllowProduction,

		S3Bucket:        s3BucketResolved,
		AWSAccessKey:    awsAccessKeyResolved,
		AWSSecretKey:    awsSecretKeyResolved,
		S3Endpoint:      s3EndpointResolved,
		S3PublicBaseURL: s3PublicBaseURLResolved,

		S3ProtectedBucket:     s3ProtectedBucketResolved,
		AWSProtectedAccessKey: awsProtectedAccessKeyResolved,
		AWSProtectedSecretKey: awsProtectedSecretKeyResolved,
		S3ProtectedEndpoint:   s3ProtectedEndpointResolved,

		AllowedOrigins: strings.TrimSpace(os.Getenv("ALLOWED_ORIGINS")),
		CookieDomain:   strings.TrimSpace(os.Getenv("COOKIE_DOMAIN")),

		TrustedPlatform: strings.TrimSpace(os.Getenv("TRUSTED_PLATFORM")),
		TrustedProxies:  strings.TrimSpace(os.Getenv("TRUSTED_PROXIES")),

		AIDailyBudgetUSD:     strings.TrimSpace(os.Getenv("AI_DAILY_BUDGET_USD")),
		AIBudgetGlobalUSD:    strings.TrimSpace(os.Getenv("AI_BUDGET_GLOBAL_USD_DAY")),
		AIBudgetGuestPoolUSD: strings.TrimSpace(os.Getenv("AI_BUDGET_GUEST_POOL_USD_DAY")),
		OpenRouterAPIKey:     config.LLMAPIKey(),
		OpenRouterZDRMode:    llmPreflightOpenRouterZDRMode(strings.TrimSpace(os.Getenv("OPENROUTER_ZDR_MODE"))),
		PaymentSecrets: map[string]string{
			"STRIPE_PLUGIN_WEBHOOK_SECRET": strings.TrimSpace(os.Getenv("STRIPE_PLUGIN_WEBHOOK_SECRET")),
			"STRIPE_WEBHOOK_SECRET":        strings.TrimSpace(os.Getenv("STRIPE_WEBHOOK_SECRET")),
			"PAYPAL_WEBHOOK_SECRET":        strings.TrimSpace(os.Getenv("PAYPAL_WEBHOOK_SECRET")),
			"MERCADOPAGO_WEBHOOK_SECRET":   strings.TrimSpace(os.Getenv("MERCADOPAGO_WEBHOOK_SECRET")),
		},
		FiscalWSAAURL: strings.TrimSpace(os.Getenv("FISCAL_WSAA_URL")),
		FiscalWSFEURL: strings.TrimSpace(os.Getenv("FISCAL_WSFE_URL")),

		TelegramWebhookEnabled: telegramWebhookEnabled,
		TelegramWebhookSecret:  strings.TrimSpace(*telegramWebhookSecret),

		// Wave 4 fiscal delivery worker — 0 means default; negative fails preflight.
		FiscalDeliveryWorkerIntervalSeconds: intEnv("FISCAL_DELIVERY_WORKER_INTERVAL_SECONDS", 0),
		FiscalDeliveryWorkerConcurrency:     intEnv("FISCAL_DELIVERY_WORKER_CONCURRENCY", 0),
	}
	trustedProxiesOperatorSet := server.TrustedProxiesConfigured(os.Getenv("TRUSTED_PROXIES"))
	applyOSSPreflightInputs(&preflightInputs, *emailProvider, *emailAPIKey) // see oss_preflight.go
	preflight := config.ValidateProduction(preflightInputs)
	if !preflight.OK() {
		for _, issue := range preflight.Issues() {
			logger.Logger.Errorf("PRODUCTION PREFLIGHT — %s [%s]: %s", issue.Code, issue.Component, issue.Message)
		}
		logger.Logger.Fatal("Production preflight failed: unsafe or incomplete configuration (see PRODUCTION PREFLIGHT errors above)")
	}
	finishOSSPreflight(productionMode, preflight)

	// ValidateConfig still enforces JWT/plugin/DB field checks (including dev
	// DB-required fields). Production secret/storage/proxy issues are owned by
	// ValidateProduction above so we do not double-Fatal with contradictory
	// messaging for the same production gap.
	if errs := config.ValidateConfig(*dbHost, *dbUser, dbPasswordResolved, *dbName, productionMode); len(errs) > 0 {
		logger.Logger.Fatal(config.FormatErrors(errs))
	}
	runInstanceStartupChecks(productionMode)
	for _, w := range config.ProductionWarnings(productionMode, *emailProvider, *emailAPIKey) {
		logger.Logger.Warnf("PRODUCTION CONFIG WARNING — %s: %s", w.Field, w.Message)
	}

	// Initialize JWT secret at startup to catch missing config immediately
	_ = structs.GetSecretKey()

	if productionMode {
		// Set Gin to production mode
		gin.SetMode(gin.ReleaseMode)
	}
	if telegramWorkerEnabled && !telegramWebhookEnabled {
		logger.Logger.Warn("TELEGRAM_TOKEN is set but TELEGRAM_WEBHOOK_SECRET is not; Telegram notifications will run without the inbound webhook")
	}
	if !telegramWorkerEnabled && strings.TrimSpace(*telegramWebhookSecret) != "" {
		logger.Logger.Warn("TELEGRAM_WEBHOOK_SECRET is set but TELEGRAM_TOKEN is not; the Telegram webhook will not be registered and notifications are disabled")
	}
	warnOperatorRoutingGaps(telegramWorkerEnabled)

	rpcURLResolved = resolveSettlementRPCURL(rpcURLResolved)

	// Initialize the email server (EMAIL_PROVIDER=log|smtp|resend|postmark;
	// see cmd/app/oss_email.go and docs/self-hosting/email.md).
	emailServer, emailProviderName := buildEmailServer(productionMode, fromEmailResolved, fromEmailUpdatesResolved, *templatesDir)
	server.SetInstanceRuntime(instanceRuntimeFromBoot(rpcURLResolved, telegramWorkerEnabled, emailProviderName))

	// Initialize notification dispatchers
	emailDispatcher := emails.NewEmailServerDispatcher(emailServer)

	// Initialize the Telegram bot for the business-plugin webhook.
	// The user-level notification dispatcher is gone; this bot only registers
	// the inbound webhook when a token and webhook secret are configured.
	var bot *tgbotapi.BotAPI
	if token := strings.TrimSpace(*telegramToken); token != "" {
		var err error
		bot, err = tgbotapi.NewBotAPIWithClient(token, tgbotapi.APIEndpoint, telegram.BoundedTelegramClient())
		if err != nil {
			logger.Logger.Warnf("Failed to create Telegram bot (notifications will be unavailable): %v", err)
		}
	}
	// Webhook registration degrades like bot creation does: Telegram being
	// unreachable at boot must not crash the whole backend. The nil check must
	// stay at this call site — passing a nil *tgbotapi.BotAPI into the
	// telegramWebhookRegistrar interface would defeat the helper's nil guard
	// (typed-nil interface) and panic inside MakeRequest.
	if telegramWebhookEnabled {
		if bot == nil {
			logger.Logger.Warn("Skipping Telegram webhook registration: bot initialization failed, inbound updates will be unavailable")
		} else if webhookURL, err := configureTelegramWebhook(bot, config.APIBaseURL(), *telegramWebhookSecret); err != nil {
			logger.Logger.Warnf("Failed to configure Telegram webhook (inbound updates will be unavailable): %v", err)
		} else {
			logger.Logger.Infof("Telegram webhook configured: %s", webhookURL)
		}
	}
	// Initialize the notification manager (email only; consumed by the delivery service).
	notificationManager := notifications.NewNotificationManager(emailDispatcher)

	// Create images directory if it doesn't exist
	if err := os.MkdirAll("images", 0755); err != nil {
		logger.Logger.Fatalf("Error creating images directory: %v", err)
	}

	// Initialize the database
	dbConfig := database.NewConfig(*dbHost, *dbPort, *dbUser, dbPasswordResolved, *dbName, *dbSSLMode)
	database.InitDB(dbConfig)

	if warnDBSSLDisabled(productionMode, *dbSSLMode, *dbHost) {
		logger.Logger.Warn("DB_SSLMODE=disable in production - ensure database is on a trusted network")
	}

	// Initialize database wrapper and blockchain service
	db := database.GetDBWrapper()
	blockchainService, err := blockchain.NewBlockchainService(rpcURLResolved)
	if err != nil {
		logger.Logger.Warnf("Failed to initialize blockchain service: %v (blockchain features will be unavailable)", err)
	}
	enforceProductionSettlementChain(productionMode, blockchainService)

	// Initialize RBAC system
	server.InitializeRBAC(db)
	logger.Logger.Info("RBAC system initialized")

	// Wire the canonical RBAC resolver into the events package so the SSE stream
	// can scope realtime delivery per event type (financial/PII frames never
	// reach a role denied the matching permission). events cannot import server
	// (import cycle), so the resolver is injected here.
	events.SetPermissionResolver(server.ResolveContextEventPermissions)

	// Genesis baseline: only a truly empty public schema loads the deterministic
	// schema dump (txn + advisory xact lock). Established/versioned DBs skip it
	// and apply only pending numbered migrations. Production schema ownership is
	// deliberately limited to this baseline plus backend/migrations/*.sql.
	// A failed baseline rolls back inside BootstrapGenesisSchema — Fatalf leaves
	// the DB empty rather than marking a partial schema current.
	dbClass, classErr := database.ClassifyDatabase(database.GetDB())
	if classErr != nil {
		logger.Logger.Fatalf("Failed to classify database for genesis bootstrap: %v", classErr)
	}
	switch dbClass {
	case database.DBEmpty:
		logger.Logger.Info("Empty database detected; applying genesis schema baseline")
		if err := database.BootstrapGenesisSchema(database.GetDB()); err != nil {
			logger.Logger.Fatalf("Genesis schema bootstrap failed: %v", err)
		}
		logger.Logger.Infof("Genesis schema baseline applied (migration head %d)", database.GenesisMigrationHead())
	case database.DBVersioned:
		logger.Logger.Info("Versioned database detected; applying pending migrations")
	case database.DBLegacy:
		logger.Logger.Fatalf("Database has application tables but no usable migration ledger; refusing startup instead of force-baselining an unverifiable schema")
	default:
		logger.Logger.Fatalf("Unknown database classification %d; refusing startup", dbClass)
	}

	// Run versioned SQL migrations via golang-migrate.
	// This ensures the schema exactly matches the versioned migration files.
	if err := database.RunMigrations(*dbHost, *dbPort, *dbUser, dbPasswordResolved, *dbName, *dbSSLMode, "migrations"); err != nil {
		logger.Logger.Fatalf("Database migration failed: %v", err)
	}

	// Schema verification: refuse to continue if schema_migrations is dirty,
	// multi-row, or not at the latest migration this binary ships — a
	// failed/partial baseline (or an incompletely-applied migration set) must
	// never be treated as current. Verify against the latest migration version,
	// not the frozen genesis head, so a bootstrapped DB that then applied
	// 000136+ still passes.
	latestVer, latestErr := database.LatestMigrationVersion("migrations")
	if latestErr != nil {
		logger.Logger.Fatalf("Failed to resolve latest migration version for schema verification: %v", latestErr)
	}
	if err := database.VerifySchemaAtVersion(database.GetDB(), latestVer); err != nil {
		logger.Logger.Fatalf("Schema verification failed: %v", err)
	}
	// Schema drift: the Postgres major and the live schema fingerprint must
	// match the genesis baseline this binary embeds.
	if fpSHA, err := database.VerifySchemaFingerprint(database.GetDB(), latestVer); err != nil {
		logger.Logger.Fatalf("Schema verification failed: %v", err)
	} else if fpSHA != "" {
		logger.Logger.Infof("Schema fingerprint matches the genesis baseline (SHA-256 %s)", fpSHA)
	}
	// The suppression table is versioned schema. Attach it only after
	// migration verification so every application email, including the explicit
	// Postmark rollback transport, fails closed for bounced/complained addresses.
	emailServer.SetSuppressionStore(emails.NewGormSuppressionStore(database.GetDB()))
	emailServer.SetOutboundStore(emails.NewGormOutboundStore(database.GetDB()))
	wireTenantMailBudget(emailServer, database.GetDB()) // tenant outbound email budget (oss_sec_mail.go)

	// Durable transactional email outbox. Attached after migration
	// verification, like the suppression store: every transactional send is
	// persisted first and delivered by the worker with bounded retries,
	// exponential backoff and a terminal dead-letter state — the same contract
	// the Telegram notification outbox already has. EMAIL_OUTBOX_ENABLED=false
	// falls back to the legacy in-process retry (at-most-once on a crash).
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("EMAIL_OUTBOX_ENABLED")), "false") {
		emailOutboxStore := emails.NewGormOutboxStore(database.GetDB())
		emailServer.SetOutboxStore(emailOutboxStore)
		emailOutboxWorker := emails.NewOutboxWorker(emailOutboxStore, emailServer)
		emails.StartEmailOutboxWorker(context.Background(), emailOutboxWorker, 5*time.Second)
		emailOutboxTTL := time.Duration(intEnv("EMAIL_OUTBOX_TTL_HOURS", 24)) * time.Hour
		emailOutboxRetention := time.Duration(intEnv("EMAIL_OUTBOX_RETENTION_DAYS", 14)) * 24 * time.Hour
		emails.StartEmailOutboxJanitor(context.Background(), emailOutboxStore, emailOutboxTTL, emailOutboxRetention, 30*time.Minute)
		logger.Logger.Infof("Email outbox worker started (ttl=%s, retention=%s)", emailOutboxTTL, emailOutboxRetention)
	} else {
		logger.Logger.Warn("Email outbox DISABLED (EMAIL_OUTBOX_ENABLED=false): transactional email falls back to in-process retry and a crash mid-send loses the message")
	}

	// Wallet sign-ins create users rows without a wallet user_auths row; this
	// reconciles them at boot. It is not a one-off backfill (see backlog).
	if err := auth.MigrateExistingUsers(database.GetDB()); err != nil {
		logger.Logger.Fatalf("Auth user migration failed: %v", err)
	}

	// Initialize session store for token revocation. Ping immediately so a
	// broken DB handle or missing migration surfaces here instead of causing
	// handlers to silently fall through the "if session.GlobalStore != nil"
	// nil-checks at request time.
	session.GlobalStore = session.NewStore(database.GetDB())
	if err := session.GlobalStore.Ping(); err != nil {
		logger.Logger.Fatalf("Session store ping failed — refusing to start: %v", err)
	}
	logger.Logger.Info("Session store initialized for token revocation")

	demoShowroomOwnerID := prepareDemoMode(database.GetDB()) // 0 unless DEMO_MODE (public demo)
	adminDemoService := demo.NewService(database.GetDB(), demo.Options{
		EmailDomain:         os.Getenv("ADMIN_DEMO_EMAIL_DOMAIN"),
		ShowroomOwnerUserID: demoShowroomOwnerID,
		// Demo gallery photos get copied into our own public bucket at seed
		// time: the stock CDN is outside our control, and AI photo cleanup
		// refuses stock hosts, so those rows were permanently un-improvable.
		AssetRehoster: demo.NewS3AssetRehoster(nil),
	})
	demoPlan := demoStartupPlanFromEnv() // off unless ADMIN_DEMO_AUTOMATION_ENABLED or DEMO_DATA
	adminDemoAutomationEnabled := demoPlan.Append
	// The demo startup ensure goroutine is launched further down, after
	// initObjectStorage: EnsureSeedAssets needs the public store, and launching
	// here would race initialization (the ensure would see "storage not
	// initialized" and skip the photography self-heal).

	// Start hourly cleanup of expired sessions
	logger.SafeGo(func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := session.GlobalStore.CleanExpired(); err != nil {
				logger.Logger.Warnf("Failed to clean expired sessions: %v", err)
			}
		}
	})

	// Start hourly cleanup of stale AI wizard sessions and AI waiter conversations
	logger.SafeGo(func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			db := database.GetDB()

			// Clean stale wizard sessions (30 min inactivity)
			wizardResult := db.Model(&database.MenuWizardSession{}).
				Where("status = ? AND updated_at < ?", "in_progress", time.Now().Add(-30*time.Minute)).
				Update("status", "abandoned")
			if wizardResult.Error != nil {
				logger.Logger.Warnf("Failed to clean stale AI wizard sessions: %v", wizardResult.Error)
			} else if wizardResult.RowsAffected > 0 {
				logger.Logger.Infof("Cleaned %d stale AI wizard sessions", wizardResult.RowsAffected)
			}

			// Clean stale AI waiter conversations (24h inactivity)
			waiterResult := db.Model(&database.AiWaiterConversation{}).
				Where("status = ? AND updated_at < ?", "active", time.Now().Add(-24*time.Hour)).
				Update("status", "closed")
			if waiterResult.Error != nil {
				logger.Logger.Warnf("Failed to clean stale AI waiter conversations: %v", waiterResult.Error)
			} else if waiterResult.RowsAffected > 0 {
				logger.Logger.Infof("Cleaned %d stale AI waiter conversations", waiterResult.RowsAffected)
			}
		}
	})

	// First platform admin: ADMIN_EMAIL/ADMIN_PASSWORD. See oss_admin.go.
	bootstrapAdminID := bootstrapPlatformAdmins(context.Background(), database.GetDB())

	// Initialize exchange rate and translation services
	exchangeRateService := services.NewExchangeRateService(db)
	translationService := services.NewTranslationService(db, googleTranslateAPIKeyResolved)
	logger.Logger.Info("Translation service initialized")

	// Initialize Google Places service. The integration is non-fatal: if no
	// API key is supplied we still register the client (its methods will
	// simply return errors) and the public diner-page handlers fail-soft.
	googlePlacesAPIKey := strings.TrimSpace(os.Getenv("GOOGLE_PLACES_API_KEY"))
	if googlePlacesAPIKey == "" {
		log.Println("[startup] google places integration disabled: missing GOOGLE_PLACES_API_KEY")
	}
	googlePlacesService := services.NewGooglePlacesService(googlePlacesAPIKey)
	logger.Logger.Info("Google Places service initialized")

	// Initialize Plugin service
	pluginService := services.NewPluginService(db)
	logger.Logger.Info("Plugin service initialized")
	telegramPluginInstance := telegramPlugin.NewTelegramPlugin(pluginService, bot)
	telegramWorkerStop := make(chan struct{})
	telegramWorkerStarted := false
	if telegramWorkerEnabled && bot != nil {
		telegramWorkerStarted = true
		workerConcurrency := *telegramWorkerConcurrency
		if workerConcurrency < 1 {
			workerConcurrency = 1
		}
		for workerID := 1; workerID <= workerConcurrency; workerID++ {
			telegramWorker := services.NewPluginNotificationWorker("telegram", telegramPluginInstance)
			go func(workerID int, telegramWorker *services.PluginNotificationWorker) {
				ticker := time.NewTicker(5 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-ticker.C:
						// SafeTick: a panic while delivering one notification
						// (plugin HTTP/JSON work) must not unwind this worker
						// goroutine and crash the whole process — log and keep
						// the loop alive for the next tick.
						logger.SafeTick("telegram-notification-worker", func() {
							if processed, err := telegramWorker.ProcessDue(context.Background()); err != nil {
								logger.Logger.Warnf("Telegram notification worker %d failed: %v", workerID, err)
							} else if processed > 0 {
								logger.Logger.Debugf("Telegram notification worker %d processed %d deliveries", workerID, processed)
							}
						})
					case <-telegramWorkerStop:
						return
					}
				}
			}(workerID, telegramWorker)
		}
		logger.Logger.Infof("Telegram notification worker started with concurrency=%d", workerConcurrency)
	}

	// Record whether Telegram delivery is live so EnqueuePluginNotification does
	// not accumulate dead outbox rows when no worker will ever consume them, and
	// warn loudly so a nil-bot / disabled worker is never silent.
	services.SetPluginDeliveryEnabled("telegram", telegramWorkerStarted)
	if !telegramWorkerStarted {
		logger.Logger.Warn("Telegram notification worker NOT started (TELEGRAM_TOKEN missing, or bot initialization failed): Telegram notifications will be skipped at enqueue time and any existing backlog will be expired by the outbox janitor.")
	}

	// Start the outbox janitor regardless of worker state: it expires stale
	// pending/retry deliveries (TTL) so the outbox self-heals and stays bounded,
	// and warns when a backlog exists with no running worker.
	pluginNotificationTTL := time.Duration(intEnv("PLUGIN_NOTIFICATION_TTL_HOURS", 24)) * time.Hour
	services.StartPluginNotificationJanitor(context.Background(), []string{"telegram"}, pluginNotificationTTL, 30*time.Minute)
	logger.Logger.Infof("Plugin notification outbox janitor started (ttl=%s)", pluginNotificationTTL)

	// Start the AI transcript retention janitor: enforces the GDPR storage-
	// limitation window for guest AI waiter transcripts (see
	// docs/policies/ai-data-retention.md). 0 disables (retain indefinitely).
	aiRetentionDays := intEnv("AI_TRANSCRIPT_RETENTION_DAYS", services.DefaultAiTranscriptRetentionDays)
	aiRetentionCtx, aiRetentionCancel := context.WithCancel(context.Background())
	services.StartAiTranscriptRetentionJanitor(aiRetentionCtx, aiRetentionDays, time.Hour)
	logger.Logger.Infof("AI transcript retention janitor started (retention_days=%d)", aiRetentionDays)

	// DB-backed loops registered here are cancelled and drained during
	// shutdown, before the connection pool is closed.
	var dbWorkers dbWorkerGroup

	// Account erasure janitor: anonymizes accounts whose 30-day deletion
	// window has passed (users + user_auths identifiers, live sessions
	// revoked). Owned businesses are kept for accounting retention.
	accountErasureCtx, accountErasureCancel := context.WithCancel(context.Background())
	accountErasureDone := services.StartAccountErasureJanitor(accountErasureCtx, database.GetDB(), session.GlobalStore, time.Hour)
	dbWorkers.Track("account_erasure", accountErasureCancel, accountErasureDone)
	logger.Logger.Info("Account erasure janitor started (hourly)")

	// Start the webhook_events retention janitor: removes old processed payloads
	// to bound table growth (failed rows retained for forensics). Default 60d.
	// Set WEBHOOK_EVENT_RETENTION_DAYS=0 to disable (retain indefinitely).
	webhookRetentionDays := intEnv("WEBHOOK_EVENT_RETENTION_DAYS", services.DefaultWebhookEventRetentionDays)
	services.StartWebhookEventRetentionJanitor(aiRetentionCtx, webhookRetentionDays, time.Hour)
	logger.Logger.Infof("webhook_events retention janitor started (retention_days=%d)", webhookRetentionDays)

	// Early refund/dispute webhooks parked in retry_pending (decision D3) are
	// expired with a payment-review alert once the provider stops redelivering.
	capturePendingCtx, capturePendingCancel := context.WithCancel(context.Background())
	capturePendingDone := handlers.StartPluginCapturePendingSweeper(capturePendingCtx, 15*time.Minute)
	dbWorkers.Track("plugin_capture_pending_sweeper", capturePendingCancel, capturePendingDone)
	logger.Logger.Info("Capture-pending webhook sweeper started (every 15m)")

	// Ops Assistant threads with no new message for 30 days are purged
	// (Wave 5 privacy contract). There is no archive step.
	// OPS_ASSISTANT_RETENTION_DAYS=0 disables the janitor in every mode; threads
	// are then kept until the business is deleted.
	opsRetentionDays := intEnv("OPS_ASSISTANT_RETENTION_DAYS", services.DefaultOpsAssistantRetentionDays)
	services.StartOpsAssistantRetentionJanitor(aiRetentionCtx, opsRetentionDays, time.Hour)
	logger.Logger.Infof("ops assistant retention janitor started (retention_days=%d)", opsRetentionDays)

	// Spaces & Tables: phone-scan processing worker + raw upload retention.
	// Artifacts use protected S3 when configured; local dev falls back to
	// .local/space-scans/ (see SPACE_SCAN_* env vars in .env.example).
	spaceScanStore := scan.DefaultArtifactStore("")
	server.SetSpaceScanArtifactStore(spaceScanStore)
	spaceScanWorkerCtx, spaceScanWorkerCancel := context.WithCancel(context.Background())
	// fiscalWorkerCtx stops the fiscal job and delivery workers on shutdown.
	fiscalWorkerCtx, fiscalWorkerCancel := context.WithCancel(context.Background())
	spaceScanWorker := services.StartSpaceScanWorker(spaceScanWorkerCtx, spaceScanStore)
	logger.Logger.Info("space scan worker started")
	spaceScanRetentionDays := intEnv("SPACE_SCAN_RAW_RETENTION_DAYS", services.DefaultSpaceScanRawRetentionDays)
	services.StartSpaceScanRetentionJanitor(aiRetentionCtx, spaceScanRetentionDays, spaceScanStore, time.Hour)
	logger.Logger.Infof("space scan retention janitor started (retention_days=%d)", spaceScanRetentionDays)

	// Start the verified-token cache sweeper: evicts expired JWT cache entries
	// every minute so never-re-presented tokens don't accumulate for the
	// process lifetime (P6.8).
	stopVerifiedTokenCacheSweeper := server.StartVerifiedTokenCacheSweeper()
	logger.Logger.Info("Verified token cache sweeper started (interval=1m)")

	// Initialize Plugin Registry
	plugins.InitializePluginRegistry(pluginService)
	plugins.GlobalRegistry.RegisterPlugin(telegramPluginInstance)
	logger.Logger.Info("Plugin registry initialized")

	// Initialize Analytics Service for report scheduler
	analyticsService := analytics.NewAnalyticsService(db)
	logger.Logger.Info("Analytics service initialized")

	// Initialize Report Scheduler
	reportScheduler := services.NewReportScheduler(db, emailServer, analyticsService)
	if err := reportScheduler.Start(); err != nil {
		logger.Logger.Warnf("Failed to start report scheduler: %v", err)
	} else {
		logger.Logger.Info("Report scheduler started successfully")
	}

	// Workers stopped explicitly on shutdown so in-flight work drains before the DB closes.
	var menuExtractionWorker *services.MenuExtractionWorker

	// Initialize Lifecycle Scheduler (getting-started and setup-nudge
	// onboarding emails).
	lifecycleScheduler := services.NewLifecycleScheduler(db)
	if err := lifecycleScheduler.Start(); err != nil {
		logger.Logger.Warnf("Failed to start lifecycle scheduler: %v", err)
	} else {
		logger.Logger.Info("Lifecycle scheduler started successfully")
	}

	// Initialize Milestone Scheduler
	milestoneScheduler := services.NewMilestoneScheduler(db)
	if err := milestoneScheduler.Start(); err != nil {
		logger.Logger.Warnf("Failed to start milestone scheduler: %v", err)
	} else {
		logger.Logger.Info("Milestone scheduler started successfully")
	}

	activationScheduler := activation.NewScheduler(db.GetGorm())
	if err := activationScheduler.Start(); err != nil {
		logger.Logger.Warnf("Failed to start activation analytics dispatcher: %v", err)
	} else {
		logger.Logger.Info("Activation analytics dispatcher started successfully")
	}

	// Initialize Guest Feedback Scheduler
	guestFeedbackScheduler := services.NewGuestFeedbackScheduler(db)
	if err := guestFeedbackScheduler.Start(); err != nil {
		logger.Logger.Warnf("Failed to start guest feedback scheduler: %v", err)
	} else {
		logger.Logger.Info("Guest feedback scheduler started successfully")
	}

	// Initialize Inventory Alert Scheduler
	inventoryAlertScheduler := services.NewInventoryAlertScheduler(db.GetDB(), emailServer)
	if err := inventoryAlertScheduler.Start(); err != nil {
		logger.Logger.Warnf("Failed to start inventory alert scheduler: %v", err)
		adminruntime.SetWorker("inventory_alerts", "stopped", err.Error(), false)
	} else {
		logger.Logger.Info("Inventory alert scheduler started successfully")
		adminruntime.SetWorker("inventory_alerts", "running", "low-stock Telegram alerts", true)
	}

	// Initialize Director Digest Scheduler
	directorDigestScheduler := services.NewDirectorDigestScheduler(db.GetDB(), emailServer, server.BuildDigestInsights)
	if err := directorDigestScheduler.Start(); err != nil {
		logger.Logger.Warnf("Failed to start director digest scheduler: %v", err)
		adminruntime.SetWorker("director_digest", "stopped", err.Error(), false)
	} else {
		logger.Logger.Info("Director digest scheduler started successfully")
		adminruntime.SetWorker("director_digest", "running", "weekly owner digests", true)
	}

	adminDemoScheduler := cron.New(cron.WithChain(cron.Recover(cron.DefaultLogger)))
	adminDemoSchedulerStarted := false
	if adminDemoAutomationEnabled {
		// Hourly, not daily: today's demo bills only materialize once their
		// closed_at passes (the pacing-honesty guard in the day generator),
		// so the append must revisit the in-progress day through the day.
		// Re-runs are idempotent (deterministic rng + bill_number skip).
		if _, err := adminDemoScheduler.AddFunc("0 * * * *", func() {
			if err := adminDemoService.AppendDueDays(context.Background()); err != nil {
				logger.Logger.Warnf("Admin demo daily append failed: %v", err)
			}
		}); err != nil {
			logger.Logger.Warnf("Failed to schedule admin demo daily append: %v", err)
			adminruntime.SetWorker("admin_demo_append", "stopped", err.Error(), false)
		} else {
			adminDemoScheduler.Start()
			adminDemoSchedulerStarted = true
			logger.Logger.Info("Admin demo daily append scheduler started successfully")
			adminruntime.SetWorker("admin_demo_append", "running", "hourly demo data append", true)
		}
	} else {
		adminruntime.SetWorker("admin_demo_append", "disabled", "ADMIN_DEMO_AUTOMATION_ENABLED=false", false)
	}

	// Initialize Delivery Service
	deliveryService := services.NewDeliveryService(db.GetDB(), notificationManager)
	server.SetDeliveryService(deliveryService)
	logger.Logger.Info("Delivery service initialized")

	// Initialize AI Service (OpenRouter provider behind the neutral llm interface)
	var aiService *services.AIService
	// Shared guardrails input classifier; stays nil when AI is unavailable so
	// downstream consumers keep their fail-open AllowAll default.
	var aiInputClassifier guardrails.InputClassifier
	// Shared per-business daily USD ceiling gate; nil when AI is unavailable (no-op / uncapped).
	var aiCostGate *llm.AICostGate
	var guestAICostGate *llm.AICostGate // guest lanes (AI waiter web + WhatsApp)
	if reason := llmDisabledReason(); reason != "" {
		logger.Logger.Warn(reason)
	} else {
		// Durable, replica-safe spend ledger (micro-USD). Authorization never
		// uses process-local memory; DailyCostRollup is metrics-only.
		budgetStore := llm.NewBudgetStore(db.GetDB())
		costRollup := llm.NewDailyCostRollup()
		// Mandatory owner/guest/global daily USD ceilings (oss_sec_ai.go).
		aiBudgets := wireAIBudgets(budgetStore)
		aiCostGate, guestAICostGate = aiBudgets.Owner, aiBudgets.Guest
		callBudget := aiBudgets.CallBudget

		llmObserver := func(ci llm.CallInfo) {
			class := "ok"
			switch {
			case ci.Err == nil:
				// keep "ok"
			case errors.Is(ci.Err, llm.ErrBudgetExceeded):
				class = "over_budget"
			case errors.Is(ci.Err, llm.ErrUnpricedModel):
				class = "unpriced_model"
			case errors.Is(ci.Err, llm.ErrAuth):
				class = "auth"
			case errors.Is(ci.Err, llm.ErrRateLimited):
				class = "rate_limited"
			case errors.Is(ci.Err, llm.ErrUpstream):
				class = "upstream"
			case errors.Is(ci.Err, llm.ErrMalformedResponse):
				class = "malformed"
			default:
				class = "error"
			}
			slog.Info("llm_call",
				"feature", ci.Feature,
				"model", ci.Model,
				"served_model", ci.ServedModel,
				"input_tokens", ci.InputTokens,
				"output_tokens", ci.OutputTokens,
				"latency_ms", ci.Latency.Milliseconds(),
				"error_class", class,
				"business_id", ci.BusinessID,
				"estimated_cost_usd", ci.EstimatedCostUSD,
			)
			// Metrics only — does not authorize spend.
			costRollup.Add(ci.BusinessID, ci.Feature, ci.EstimatedCostUSD)
		}
		provider, perr := openrouter.New(llmProviderConfig(), openrouter.WithObserver(llmObserver), openrouter.WithBudget(callBudget))
		if perr != nil {
			logger.Logger.Warnf("Failed to initialize OpenRouter provider: %v", perr)
		} else {
			models := llm.LoadModelConfig()
			// Pricing + ZDR checks; OpenRouter-only rules relax for LLM_BASE_URL.
			runLLMStartupChecks(models, productionMode)
			svc, aerr := services.NewAIService(provider, models)
			if aerr != nil {
				logger.Logger.Warnf("Failed to initialize AI service: %v", aerr)
			} else {
				aiService = svc
				server.SetAIService(aiService)
				logger.Logger.Info("AI service initialized (OpenRouter)")

				menuAIService := services.NewMenuAIService(provider, models)
				server.SetMenuAIService(menuAIService)
				logger.Logger.Info("Menu AI service initialized (OpenRouter)")

				// Durable menu extraction worker: atomic DB claims + protected
				// object inputs. Enqueue wakes only; restart recovery re-lists
				// pending/stale jobs so a deploy cannot strand in-flight work.
				menuExtractionWorker = services.NewMenuExtractionWorker(
					menuAIService,
					server.LoadMenuExtractionInputsForWorker,
					server.CleanupMenuExtractionAssetsForWorker,
				)
				services.SetMenuExtractionWorker(menuExtractionWorker)
				menuExtractionWorker.Start(context.Background())
				if n, rerr := menuExtractionWorker.RestorePending(100); rerr != nil {
					logger.Logger.Warnf("Menu extraction restore pending failed: %v", rerr)
				} else if n > 0 {
					logger.Logger.Infof("Menu extraction worker restored %d pending/stale jobs", n)
				}
				logger.Logger.Info("Menu extraction worker started")

				aiInputClassifier = guardrails.NewGeminiClassifierStrict(
					provider,
					models.Guardrail,
					models.GuardrailFallbacks,
					resolveGuardrailStrict(os.Getenv("GUARDRAIL_STRICT"), productionMode),
				)
				server.SetAIWaiterClassifier(aiInputClassifier)
				server.SetImagePromptClassifier(aiInputClassifier)
				logger.Logger.Info("Guardrails classifier initialized (waiter, image prompts, director)")

				// UTC-date metrics from the durable store (not "24h since process start").
				// Also reconcile stale reservations fail-closed: consumed calls use
				// recorded actual spend, while ambiguous crashed calls retain their
				// full conservative charge rather than silently restoring headroom.
				logger.SafeGo(func() {
					ticker := time.NewTicker(1 * time.Hour)
					defer ticker.Stop()
					emit := func() {
						now := time.Now().UTC()
						if n, err := budgetStore.ExpireStale(context.Background(), now); err != nil {
							slog.Warn("llm_budget_reconcile_stale_failed", "error", err.Error())
						} else if n > 0 {
							slog.Info("llm_budget_reconcile_stale", "reconciled", n)
						}
						metrics, err := budgetStore.MetricsForUTCDate(context.Background(), now)
						if err != nil {
							slog.Warn("llm_cost_daily_metrics_failed", "error", err.Error())
							return
						}
						for _, line := range metrics {
							slog.Info("llm_cost_daily",
								"business_id", line.BusinessID,
								"feature_scope", line.FeatureScope,
								"usage_date", llm.UTCDate(line.UsageDate).Format("2006-01-02"),
								"finalized_usd", llm.MicroUSDToDollars(line.FinalizedMicroUSD),
								"reserved_usd", llm.MicroUSDToDollars(line.ReservedMicroUSD),
								"finalized_micro_usd", line.FinalizedMicroUSD,
								"reserved_micro_usd", line.ReservedMicroUSD,
							)
						}
					}
					emit()
					for range ticker.C {
						emit()
					}
				})
			}
		}
	}

	// Initialize Director Console Service (works with AI or deterministic fallback when AI is unavailable)
	directorToolRegistry := director_tools.NewRegistry()
	directorToolRegistry.Register(&director_tools.BusinessProfileTool{})
	directorToolRegistry.Register(&director_tools.RevenueSummaryTool{})
	directorToolRegistry.Register(&director_tools.OrderFunnelTool{})
	directorToolRegistry.Register(&director_tools.MenuTopItemsTool{})
	directorToolRegistry.Register(&director_tools.MenuUnderperformersTool{})
	directorToolRegistry.Register(&director_tools.SlowDaypartsTool{})
	directorToolRegistry.Register(&director_tools.CRMSegmentTool{})
	directorToolRegistry.Register(&director_tools.ReservationLoadTool{})
	directorToolRegistry.Register(&director_tools.AIWaiterPerformanceTool{})
	directorToolRegistry.Register(&director_tools.PluginStatusTool{})
	// Pillar 2 write-proposal tools (dry-run only; they stage a pending proposal
	// the operator applies via /ai/director/actions/apply — they never mutate menu data).
	directorToolRegistry.Register(&director_tools.ProposePriceChangeTool{})
	directorToolRegistry.Register(&director_tools.ProposeAvailabilityChangeTool{})
	directorToolRegistry.Register(&director_tools.ProposeContentEditTool{})
	directorToolRegistry.Register(&director_tools.FoodCostAnalysisTool{})
	directorToolRegistry.Register(&director_tools.PreviewMarginChangeTool{})
	directorToolRegistry.Register(&director_tools.MenuEngineeringTool{})
	directorToolRegistry.Register(&director_tools.WasteVarianceTool{})
	directorToolRegistry.Register(&director_tools.LaborCostTool{})
	directorToolRegistry.Register(&director_tools.LiveFloorTool{})
	directorToolRegistry.Register(&director_tools.KitchenStatusTool{})
	directorToolRegistry.Register(&director_tools.PromosTool{})

	directorConsoleService := services.NewDirectorConsoleService(db, analyticsService, aiService, directorToolRegistry).
		WithClassifier(aiInputClassifier)
	// Apply the same per-business daily USD ceiling the AI Waiter uses,
	// so the flagship Director surface can't run up unmetered LLM spend. Only
	// wire a real gate — a typed-nil would make budgetExceeded panic.
	if aiCostGate != nil {
		directorConsoleService = directorConsoleService.WithCostGate(aiCostGate)
	}
	server.SetDirectorConsoleService(directorConsoleService)

	// The Ops Assistant requires a live LLM provider: constructing it with a nil
	// aiService (OPENROUTER_API_KEY absent) panics at boot. Only wire it when
	// AI is available; the handler-level nil guard (ensureOpsAssistantEnabled)
	// then correctly returns 503.
	// Deterministic escalation sink: persists escalation rows and fans out to a
	// DEDICATED support-escalation Telegram bot (TELEGRAM_ESCALATION_*) + email.
	// Silent no-op for Telegram when its env vars are unset (feature dark).
	escalationSvc := escalation.NewService(emailServer)

	if aiService != nil {
		opsRegistry := ops_tools.NewRegistry()
		opsAssistantSvc := agents.NewOpsAssistantService(aiService, opsRegistry, db, emailServer).
			WithClassifier(aiInputClassifier).
			WithEscalator(escalationSvc)
		server.SetOpsAssistantService(opsAssistantSvc)
		logger.Logger.Info("Ops Assistant service initialized")
	} else {
		logger.Logger.Warn("AI unavailable; Ops Assistant disabled (routes return 503)")
	}

	// Director action service commits/undoes human-applied proposals (no AI client;
	// the AI only proposes, a human applies via the apply/undo endpoints).
	directorActionService := services.NewDirectorActionService(db)
	server.SetDirectorActionService(directorActionService)
	logger.Logger.Info("Director console service initialized")

	// Marketing suggestion engine — reuses analytics + the same menu-engineering
	// read the Director console uses (food-cost report → quadrant classification).
	marketingMenuEng := func(businessID uint, period string, loc *time.Location) (menuengineering.Report, error) {
		report, err := foodcost.NewCalculator(db, analyticsService).Analyze(businessID, period, loc)
		if err != nil {
			return menuengineering.Report{}, err
		}
		return menuengineering.Classify(report), nil
	}
	server.SetMarketingEngine(marketing.NewEngine(db, analyticsService, marketingMenuEng))
	logger.Logger.Info("Marketing suggestion engine initialized")

	// Initialize Reservation Scheduler
	reservationScheduler := jobs.NewReservationScheduler()
	if err := reservationScheduler.Start(); err != nil {
		logger.Logger.Warnf("Failed to start reservation scheduler: %v", err)
	} else {
		logger.Logger.Info("Reservation scheduler started successfully")
	}

	// Initialize CRM settlement reconciliation scheduler.
	// WithChain(Recover(...)) so a panic in an AddFunc job is recovered and
	// logged instead of unwinding the cron goroutine and crashing the process.
	crmSettlementScheduler := cron.New(cron.WithChain(cron.Recover(cron.DefaultLogger)))
	crmSettlementSchedulerStarted := false
	if _, err := crmSettlementScheduler.AddFunc("@every 15m", func() {
		jobs.ReconcileCRMSettlementVisits(db.GetDB(), 100)
	}); err != nil {
		logger.Logger.Warnf("Failed to schedule CRM settlement reconciliation: %v", err)
	} else {
		crmSettlementScheduler.Start()
		crmSettlementSchedulerStarted = true
		logger.Logger.Info("CRM settlement reconciliation scheduler started successfully")
	}

	// IMP-05: stuck-bill watchdog. Every 5 minutes, scans open bills past
	// the 2h staleness threshold and emits a bill.stuck SSE event per
	// affected business so Today's Briefings refreshes in realtime instead
	// of waiting on its 60s poll cadence. Doesn't mutate the bill status.
	stuckBillWatchdog := jobs.NewStuckBillWatchdog(database.GetDB(), jobs.StuckBillWatchdogConfig{})
	stuckBillWatchdog.Start()
	logger.Logger.Info("Stuck-bill watchdog started (IMP-05)")

	// Waiter-call seating SLA (#729). Resolves open service_call alerts
	// older than 90 minutes so Tables Live View, the sidebar badge, and
	// guest QR chrome drop leftover water / empty-table check-please.
	// Unpaid same-seating check-please and claimed calls stay. Read paths
	// hide lapsed calls immediately; this persists the resolve.
	serviceCallTTLJanitor := jobs.NewServiceCallTTLJanitor(database.GetDB(), 2*time.Minute)
	serviceCallTTLJanitor.Start()
	logger.Logger.Info("Service-call TTL janitor started (90m seating SLA)")

	// Task 16: bill lifecycle sweeper. Marks open/partial bills past the
	// abandon threshold (default 24h, business-overridable) as abandoned so
	// they leave the active set. Never touches a bill with a payment in flight.
	// Surfacing (2h, stuck-bill watchdog above) and abandon (24h) are separate.
	billLifecycleSweeper := services.NewBillLifecycleSweeper(database.GetDB(), services.BillLifecycleConfig{})
	billLifecycleSweeper.Start()
	logger.Logger.Info("Bill lifecycle sweeper started (Task 16)")

	// Delivery payment-expiry job: cancels prepay delivery orders whose
	// 15-minute payment window lapsed without payment (spec §3.1).
	deliveryPaymentExpiry := jobs.NewDeliveryPaymentExpiry(deliveryService, time.Minute)
	deliveryPaymentExpiry.Start()
	logger.Logger.Info("Delivery payment-expiry job started")

	// Initialize WhatsApp Manager — gated by WHATSAPP_ENABLED=true (dark by default).
	// The manager was UI/marketing-hidden but previously ran unconditionally, mounting
	// routes and restoring sessions on every boot. Now it only starts when the flag is
	// explicitly set, so the dark state is enforced server-side.
	var whatsAppManager *services.WhatsAppManager
	if services.WhatsAppAvailable() {
		wam, err := services.NewWhatsAppManager(db, aiService)
		if err != nil {
			logger.Logger.Warnf("Failed to initialize WhatsApp manager: %v", err)
		} else {
			whatsAppManager = wam
			server.SetWhatsAppManager(whatsAppManager)
			whatsAppManager.WithClassifier(aiInputClassifier).WithCostGate(guestAICostGate)
			logger.Logger.Info("WhatsApp manager initialized")

			// Restore sessions in background
			logger.SafeGo(func() {
				if err := whatsAppManager.RestoreAllSessions(); err != nil {
					logger.Logger.Warnf("Failed to restore WhatsApp sessions: %v", err)
				}
			})
		}
	} else {
		logWhatsAppDisabled()
	}

	// Make services available to the server package
	server.SetTranslationService(translationService)
	server.SetBatchTranslationService(translationService)
	server.SetGooglePlacesService(googlePlacesService)

	// Initialize default currencies and languages
	if err := exchangeRateService.InitializeDefaultCurrencies(); err != nil {
		logger.Logger.Warnf("Failed to initialize default currencies: %v", err)
	}
	if err := translationService.InitializeDefaultLanguages(); err != nil {
		logger.Logger.Warnf("Failed to initialize default languages: %v", err)
	}

	// Initialize default plugins
	if err := services.InitializeDefaultPlugins(); err != nil {
		logger.Logger.Warnf("Failed to initialize default plugins: %v", err)
	}

	// Start periodic exchange rate fetching. The worker group cancels this
	// loop on shutdown and drains it before the pool closes.
	dbWorkers.Go("exchange_rate", exchangeRateService.RunPeriodicFetch)

	// Initialize Web Push service (non-fatal: no-op when VAPID keys are absent)
	webPushService := services.NewWebPushService(database.GetDB(), *vapidPublicKey, *vapidPrivateKey, *vapidSubject)
	if *vapidPublicKey != "" {
		logger.Logger.Info("Web Push service initialized")
	} else {
		logger.Logger.Info("[startup] Web Push disabled: VAPID_PUBLIC_KEY not set")
	}
	server.SetWebPushService(webPushService)

	// Initialize Shift Reminder Scheduler (staff scheduling): reminds staff before
	// a published shift starts (tz-aware + quiet-hours-guarded, deduped via the
	// atomic shifts.reminded_at claim). Constructed here — after the web-push
	// service exists — so the reminder's staff-facing last-mile (inbox + SSE +
	// push) can reach subscribed phones.
	shiftReminderScheduler := services.NewShiftReminderScheduler(db, webPushService)
	if err := shiftReminderScheduler.Start(); err != nil {
		logger.Logger.Warnf("Failed to start shift reminder scheduler: %v", err)
	} else {
		logger.Logger.Info("Shift reminder scheduler started successfully")
	}

	// Initialize CRM service and handler (used in multiple route groups)
	crmService := crm.NewService(database.GetDB())
	crmHandler := crm.NewHandler(crmService)

	// Initialize payment handler
	paymentHandler := handlers.NewPaymentHandler(db, blockchainService, nil, exchangeRateService)

	// Initialize delivery handlers
	deliveryHandler := handlers.NewDeliveryHandler(deliveryService)
	handlers.SetSharedDeliveryService(deliveryService)

	// Initialize thermal printer handlers (Sprint 1: browser print + CloudPRNT enqueue)
	// CachingFactory wraps CredentialAwareFactory so the same WSAAClient/token is
	// reused across fiscal jobs for the same business. AFIP rejects a fresh login
	// while a prior TA is still valid; caching avoids that rejection and the
	// cost of a cert-CMS sign + SOAP round-trip on every job. Token refresh is
	// handled internally by the WSAAClient inside each cached Provider.
	cachedFiscalFactory := providers.NewCachingFactory(providers.NewCredentialAwareFactory())
	fiscalService := fiscal.NewService(database.GetDB(), fiscal.NewProviderRegistry()).
		WithProviderFactory(cachedFiscalFactory)
	fiscalHandlers := handlers.NewFiscalHandlers(fiscalService)

	// Wave 4 fiscal outbox: enqueue the issue job INSIDE the payment
	// settlement transaction. database cannot import fiscal, so the hook is
	// injected here. The post-commit enqueueFiscalJobForPaidBill calls in
	// handlers remain as idempotent belt-and-braces (same idempotency key).
	database.SetBillPaidInTxHook(func(tx *gorm.DB, bill *database.Bill, paymentID, altPaymentID *uint) error {
		return fiscal.EnqueueIssueJobInTx(tx, bill, paymentID, altPaymentID, "system")
	})

	// Fiscal job worker (AFIP/ARCA): always on. Businesses without fiscal
	// credentials produce no jobs, so the worker idles for them. A production
	// deploy without PLUGIN_SECRET_KEY now fails the production preflight above
	// (plugin_secret.missing) and Fatals before reaching this point, so no
	// warn-only fallback for that misconfiguration is needed here.
	{
		fiscalWorker := fiscal.NewFiscalWorker(database.GetDB(), cachedFiscalFactory)
		// Give each process a UNIQUE worker id so the reclaim CAS (MarkJobResult's
		// `WHERE locked_by = ?`) is a real per-owner guard. With the default
		// constant id, two replicas both stamp "fiscal-worker" and a stalled
		// replica could clobber a job another replica already re-claimed. hostname
		// +pid is stable within a process and distinct across replicas/restarts.
		hostname, _ := os.Hostname()
		if hostname == "" {
			hostname = "fiscal-worker"
		}
		fiscalWorker.WorkerID = fmt.Sprintf("%s-%d", hostname, os.Getpid())
		// Wire real receipt delivery (protected-S3 upload + customer email + best-
		// effort receipt print). Delivery is best-effort and never fails the fiscal
		// job; print enqueue leaves the job pending when no printer is configured.
		fiscalDispatcher := fiscaldelivery.New(printsvc.NewService(database.GetDB()))
		fiscalWorker.WithDelivery(fiscalDispatcher)
		fiscalWorkerInterval := time.Duration(intEnv("FISCAL_WORKER_INTERVAL_SECONDS", 10)) * time.Second
		// StartFiscalWorker logs its own "Fiscal worker started" line.
		go fiscal.StartFiscalWorker(fiscalWorkerCtx, fiscalWorker, fiscalWorkerInterval)
		adminruntime.SetWorker("fiscal_compliance", "running", fiscalWorkerInterval.String()+" interval", true)

		// Wave 4: durable per-channel fiscal receipt delivery worker. Authorization
		// only enqueues tasks; this worker executes artifact/email/print with leases
		// and retries. Separate from the issue-job worker so delivery backlog never
		// blocks AFIP authorization throughput.
		{
			deliveryIntervalSec := intEnv("FISCAL_DELIVERY_WORKER_INTERVAL_SECONDS", 10)
			deliveryConcurrency := intEnv("FISCAL_DELIVERY_WORKER_CONCURRENCY", 1)
			if deliveryIntervalSec <= 0 {
				deliveryIntervalSec = 10
			}
			if deliveryConcurrency <= 0 {
				deliveryConcurrency = 1
			}
			deliveryWorker := fiscal.NewDeliveryWorker(database.GetDB(), fiscalService.WithDelivery(fiscalDispatcher), fiscalDispatcher)
			hostnameD, _ := os.Hostname()
			if hostnameD == "" {
				hostnameD = "fiscal-delivery"
			}
			deliveryWorker.WorkerID = fmt.Sprintf("%s-delivery-%d", hostnameD, os.Getpid())
			deliveryWorker.Concurrency = deliveryConcurrency
			deliveryInterval := time.Duration(deliveryIntervalSec) * time.Second
			// StartDeliveryWorker logs its own start line (interval + concurrency).
			go fiscal.StartDeliveryWorker(fiscalWorkerCtx, deliveryWorker, deliveryInterval)
			adminruntime.SetWorker("fiscal_delivery", "running",
				fmt.Sprintf("%s interval concurrency=%d", deliveryInterval, deliveryConcurrency), true)
		}
	}

	// Wave 4 REFUND track: noncustodial crypto refund confirmation worker.
	// Backend NEVER custodies a treasury key — only verifies submitted hashes.
	// Mainnet confirmation is OFF by default (CRYPTO_REFUND_MAINNET_ENABLED).
	{
		cryptoRefundCfg := cryptorefund.DefaultConfig()
		hostname, _ := os.Hostname()
		if hostname == "" {
			hostname = "crypto-refund"
		}
		cryptoRefundCfg.WorkerID = fmt.Sprintf("%s-%d", hostname, os.Getpid())
		var outboundVerifier cryptorefund.OutboundVerifier
		if blockchainService != nil {
			outboundVerifier = blockchainService
		}
		cryptoRefundWorker := cryptorefund.NewWorker(
			database.GetDB(),
			outboundVerifier,
			handlers.EnqueueFiscalCreditNoteForRefund,
			cryptoRefundCfg,
		)
		dbWorkers.Go("crypto_refunds", cryptoRefundWorker.Run)
		mainnetLabel := "mainnet=off"
		if cryptoRefundCfg.MainnetEnabled {
			mainnetLabel = "mainnet=on"
		}
		logger.Logger.Infof("Crypto refund worker started (interval=%s %s)", cryptoRefundCfg.Interval, mainnetLabel)
		adminruntime.SetWorker("crypto_refunds", "running", cryptoRefundCfg.Interval.String()+" "+mainnetLabel, true)
	}

	// Reorg reconciliation sweep (P5 Gap 4). The one-shot confirmation gate never
	// re-reads the chain, so a reorg deeper than that depth can orphan a confirmed
	// USDC payment (bill stays paid) or an outbound crypto refund (ledger stays
	// reversed) undetected. This sweep re-verifies recently-confirmed records
	// (within a finality window) against the canonical chain and raises a
	// high-priority operator alert on a suspected reorg — it deliberately does NOT
	// auto-reverse the ledger (a flaky RPC must never flip a paid bill to unpaid).
	// Disabled with no RPC verifier dialed, or via REORG_WATCH_ENABLED=false.
	if blockchainService != nil && strings.TrimSpace(os.Getenv("REORG_WATCH_ENABLED")) != "false" {
		reorgCfg := reorgwatch.Config{
			FinalityWindow: time.Duration(intEnv("REORG_FINALITY_WINDOW_MIN", 30)) * time.Minute,
			Cooldown:       time.Duration(intEnv("REORG_WATCH_COOLDOWN_SEC", 120)) * time.Second,
			SuspicionGrace: time.Duration(intEnv("REORG_SUSPICION_GRACE_SEC", 300)) * time.Second,
			BatchSize:      intEnv("REORG_WATCH_BATCH", 100),
		}
		reorgInterval := time.Duration(intEnv("REORG_WATCH_INTERVAL_SEC", 60)) * time.Second
		reconciler := reorgwatch.New(database.GetDB(), blockchainService, reorgAlerter{}, reorgCfg)
		dbWorkers.Go("reorg_watch", func(ctx context.Context) { reconciler.Run(ctx, reorgInterval) })
		logger.Logger.Infof("Reorg reconciliation sweep started (interval=%s window=%s)", reorgInterval, reorgCfg.FinalityWindow)
		adminruntime.SetWorker("reorg_watch", "running", reorgInterval.String()+" interval", true)
	} else {
		adminruntime.SetWorker("reorg_watch", "disabled", "no RPC verifier or REORG_WATCH_ENABLED=false", false)
	}

	operationalAlertHandlers := handlers.NewOperationalAlertHandlers(database.GetDB())
	printerHandlers := handlers.NewPrinterHandlers(database.GetDB())
	printJobHandlers := handlers.NewPrintJobHandlers(database.GetDB())
	// X-2: order approval enqueues kitchen tickets through the same shared
	// print service instance as the HTTP print handlers.
	handlers.SetSharedPrintService(printJobHandlers.Service())

	// IMP-02 print job retry worker. Drains `failed_retryable` and stale rows
	// on the schedule defined in printsvc.defaultBackoff (5s → 30m, terminal
	// at attempt #6). The send hook is nil for now — the cloudprnt/browser
	// transports register themselves via the print handlers, and the worker
	// returns the job to the queue with a soft "no transport configured"
	// error if the print endpoint is offline at sweep time. Always on:
	// without it, pending/failed_retryable print jobs never self-heal.
	printRetryWorker := printsvc.NewRetryWorker(database.GetDB(), nil, printsvc.RetryWorkerConfig{})
	printRetryWorker.Start(context.Background())
	logger.Logger.Info("Print retry worker started (IMP-02)")

	printOrphanSweep := printsvc.NewOrphanSweepWorker(database.GetDB(), printJobHandlers.Service(), 0, 0)
	printOrphanSweep.Start(context.Background())
	logger.Logger.Info("Print orphan sweep worker started (IMP-04)")

	// Initialize new auth handler (email, Google + wallet linking)
	authHandler := auth.NewAuthHandler(database.GetDB(), emailServer)
	runtimeControlService := runtimecontrol.New(database.GetDB())
	server.SetLaunchIdentityAdmission(func(ctx context.Context, identity, code string, create func(*gorm.DB) error) error {
		err := runtimeControlService.Admit(ctx, identity, code, create)
		switch {
		case errors.Is(err, runtimecontrol.ErrRegistrationClosed):
			return server.ErrRegistrationClosed
		case errors.Is(err, runtimecontrol.ErrInviteRequired):
			return server.ErrLaunchInviteRequired
		case errors.Is(err, runtimecontrol.ErrInviteExpired):
			return server.ErrLaunchInviteExpired
		case errors.Is(err, runtimecontrol.ErrCohortFull):
			return server.ErrLaunchCohortFull
		case errors.Is(err, runtimecontrol.ErrInviteAlreadyClaimed):
			return server.ErrLaunchInviteAlreadyClaimed
		default:
			return err
		}
	})
	authHandler.SetRegistrationAdmission(runtimeControlService)
	authHandler.SetRegistrationRateLimit(intEnv("REGISTRATION_RATE_LIMIT_PER_HOUR", 5))
	authHandler.SetLoginThrottle(auththrottle.New(database.GetDB(), auththrottle.Config{
		MaxAttempts: 5, Window: 15 * time.Minute, BaseLockout: time.Minute, MaxLockout: time.Hour,
	}))

	// Auth service for admin user management (created here before `auth` is shadowed by router group)
	adminAuthService := auth.NewAuthService(database.GetDB())

	// Object storage (STORAGE_DRIVER=local|s3, default local; see oss_storage.go).
	// Env-resolved values: compose does not pass secrets on argv (SEC-1).
	initObjectStorage(productionMode, s3.S3Settings{Bucket: s3BucketResolved, AccessKey: awsAccessKeyResolved, SecretKey: awsSecretKeyResolved, Region: *awsRegion, Endpoint: s3EndpointResolved, PublicBaseURL: s3PublicBaseURLResolved, ProtectedBucket: s3ProtectedBucketResolved, ProtectedAccessKey: awsProtectedAccessKeyResolved, ProtectedSecretKey: awsProtectedSecretKeyResolved, ProtectedEndpoint: s3ProtectedEndpointResolved, ProtectedBaseURL: *s3ProtectedBaseURL})
	startDemoStartupEnsure(adminDemoService, demoPlan, bootstrapAdminID)
	// Initialize the metrics
	metrics.Init()
	installAssistantTelemetrySink()
	defer llm.SetTelemetrySink(nil)
	if err := metrics.RefreshLaunchReadinessState(database.GetDB()); err != nil {
		logger.Logger.Warnf("Failed to initialize launch-readiness metrics: %v", err)
	}
	if err := metrics.RefreshAlternativePaymentRequestState(database.GetDB(), time.Now()); err != nil {
		logger.Logger.Warnf("Failed to initialize alternative-payment request metrics: %v", err)
	}
	logger.SafeGo(func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if err := metrics.RefreshLaunchReadinessState(database.GetDB()); err != nil {
				logger.Logger.Warnf("Failed to refresh launch-readiness metrics: %v", err)
			}
			if err := metrics.RefreshAlternativePaymentRequestState(database.GetDB(), time.Now()); err != nil {
				logger.Logger.Warnf("Failed to refresh alternative-payment request metrics: %v", err)
			}
		}
	})

	// Initialize PostHog with environment variable
	posthogAPIKey := os.Getenv("POSTHOG_API_KEY")
	posthogHost := os.Getenv("POSTHOG_HOST")
	if posthogAPIKey != "" && posthogHost != "" {
		metrics.InitPostHogClient(posthogAPIKey, posthogHost)
		defer metrics.ClosePostHogClient()
	}

	// Create rate limiter with different limits for production vs development.
	// Use context.Background() — limiter lives for the process lifetime.
	var rateLimiter *middleware.SimpleRateLimiter
	if productionMode {
		rateLimiter = middleware.NewSimpleRateLimiter(
			context.Background(),
			intEnv("GLOBAL_RATE_LIMIT_REQUESTS_PER_MINUTE", 300),
		) // production default: 300/min (5/s)
	} else {
		rateLimiter = middleware.NewSimpleRateLimiter(
			context.Background(),
			intEnv("GLOBAL_RATE_LIMIT_REQUESTS_PER_MINUTE", 1200),
		) // development default: 1200/min (20/s)
	}

	// Parse allowed CORS origins from environment
	allowedOrigins := resolveAllowedOrigins(os.Getenv("ALLOWED_ORIGINS"))
	// M-siwe: SIWE domains are PUBLIC_URL plus explicitly configured origins only, never the built-in CORS defaults.
	server.SetSIWETrustedOrigins(strings.Split(os.Getenv("ALLOWED_ORIGINS"), ","))

	r := gin.New()
	r.Use(middleware.AccessLogger())
	if err := server.ConfigureTrustedProxies(r, os.Getenv("TRUSTED_PROXIES")); err != nil {
		logger.Logger.Fatalf("Invalid TRUSTED_PROXIES configuration: %v", err)
	}
	if err := server.ConfigureTrustedPlatform(r, os.Getenv("TRUSTED_PLATFORM")); err != nil {
		logger.Logger.Fatalf("Invalid TRUSTED_PLATFORM configuration: %v", err)
	}
	if !trustedProxiesOperatorSet {
		// FBE-2: the loopback-only default is silent for a proxy on a
		// container network; flag the first forwarded request from one.
		// (TRUSTED_PROXIES is never empty here: applyOSSPreflightInputs
		// fills in the loopback default, so this warning replaces the old
		// "TRUSTED_PROXIES is empty" boot log, which could no longer fire.)
		r.Use(server.WarnOnUntrustedForwardingPeer(logger.Logger.Errorf))
	}
	r.Use(gin.Recovery())
	r.Use(observability.GinMiddlewares(sentryClient)...)
	r.Use(middleware.ErrorSanitizer())
	r.Use(middleware.RequestID())
	r.Use(middleware.CORS(allowedOrigins))
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.InputValidation())
	r.Use(middleware.JSONSizeLimit(10 << 20)) // 10MB limit
	r.Use(rateLimiter.RateLimit())
	r.Use(server.PrometheusMiddleware())
	r.Use(middleware.FlowMetrics())
	// Durable launch controls execute before route handlers. Health, provider
	// reconciliation callbacks, and the authenticated admin recovery surface are
	// explicit exemptions inside the middleware.
	r.Use(runtimecontrol.Middleware(runtimeControlService))
	// DEMO_MODE (public demo): refuse outbound/account/upload actions, noindex,
	// per-IP write budget. No-op otherwise (internal/demomode).
	r.Use(demomode.Guard())

	// Configure Gin to handle larger file uploads
	r.MaxMultipartMemory = 100 << 20 // 100 MiB
	// GET/HEAD /media/*key — public objects (oss_storage.go).
	registerMediaRoutes(r)

	// Metrics endpoint. Supports graceful token rotation: operators can set
	// METRICS_TOKENS to a comma-separated list and rotate by prepending a new
	// token, waiting for scrapers to pick it up, then removing the old. Legacy
	// single-token METRICS_TOKEN is still honored.
	//
	// In production, refuse to register the endpoint when no metrics tokens
	// are configured — an unauthenticated /metrics leaks internal state.
	metricsTokens := parseMetricsTokens(os.Getenv("METRICS_TOKENS"), os.Getenv("METRICS_TOKEN"))
	if productionMode && len(metricsTokens) == 0 {
		logger.Logger.Warn("/metrics endpoint not registered: no METRICS_TOKEN(S) configured in production")
	} else {
		r.GET("/metrics", func(c *gin.Context) {
			acceptedTokens := parseMetricsTokens(os.Getenv("METRICS_TOKENS"), os.Getenv("METRICS_TOKEN"))
			if len(acceptedTokens) > 0 {
				auth := c.GetHeader("Authorization")
				provided, ok := strings.CutPrefix(auth, "Bearer ")
				if !ok {
					c.AbortWithStatus(http.StatusUnauthorized)
					return
				}
				provided = strings.TrimSpace(provided)
				matched := false
				for _, t := range acceptedTokens {
					if subtle.ConstantTimeCompare([]byte(provided), []byte(t)) == 1 {
						matched = true
						break
					}
				}
				if !matched {
					c.AbortWithStatus(http.StatusUnauthorized)
					return
				}
			}
			promhttp.Handler().ServeHTTP(c.Writer, c.Request)
		})
	}

	// Token-gated pprof. PPROF_TOKEN gates both /debug/pprof/* and the
	// snapshot endpoint. Unset → both routes are not registered.
	pprofToken := os.Getenv("PPROF_TOKEN")
	handlers.RegisterPprofSnapshot(r, pprofToken)

	// Manager-PIN brute-force lockout (P2). DB-backed so it survives restart.
	managerPinThrottle := auththrottle.New(database.GetDB(), auththrottle.Config{
		MaxAttempts: 5, Window: 15 * time.Minute, BaseLockout: time.Minute, MaxLockout: time.Hour,
	})
	server.SetManagerPinThrottle(managerPinThrottle)
	// Dedicated limiter for the PIN-gated correction routes (void/refund and the
	// crypto-refund lifecycle). It must NOT be PaymentRateLimit: /bills/:bill_id/void
	// has no business_id/id param, so the payment key collapsed to a single
	// per-IP 1-per-10s token shared by every correction at a venue — one 404
	// probe starved the next real void with a 429 that talked about payments
	// (issue 909). ManagerActionRateLimit keeps its own bucket, scopes per
	// (actor, route, target), and still fails closed.
	managerPinLimiter := middleware.ManagerActionRateLimit()

	// Rate limiter for auth endpoints (login, register, password reset).
	// Defaults are production-strict; CI release-smoke can raise these without
	// changing the published image or weakening production deployments.
	authLimiter := middleware.AuthRateLimiter(
		intEnv("AUTH_RATE_LIMIT_REQUESTS_PER_MINUTE", 10),
		intEnv("AUTH_RATE_LIMIT_BURST", 3),
	)
	// Stricter limiter for unauthenticated public form endpoints (email
	// opt-out). Keeps abuse from burning through quota without friction for
	// normal visitors.
	publicFormLimiter := middleware.AuthRateLimiter(
		intEnv("PUBLIC_FORM_RATE_LIMIT_REQUESTS_PER_MINUTE", 5),
		intEnv("PUBLIC_FORM_RATE_LIMIT_BURST", 2),
	)
	// Unauthenticated reservation mutations (create/cancel) trigger
	// emails, Telegram pushes and dashboard alerts per row, so they need a
	// per-IP throttle too — slightly looser than the public-form limiter because a
	// legit guest may retry after "slot no longer available".
	publicReservationLimiter := middleware.AuthRateLimiter(10, 5)

	// Auth routes: wallet (SIWE), email/password, and OAuth.
	// Block cross-origin mutations (login/refresh/signout/…) from any host
	// not in the allowlist. GETs (session checks, OAuth redirects from
	// Google) are not affected by the Origin middleware.
	auth := r.Group("/api/v1/auth")
	auth.Use(middleware.RequireTrustedOriginForMutations(allowedOrigins))
	{
		// SIWE sign-in touches an in-memory challenge store and issues session
		// cookies — group it behind authLimiter.
		siweGroup := auth.Group("")
		siweGroup.Use(authLimiter)
		{
			siweGroup.POST("/challenge", server.GenerateChallenge)
			siweGroup.POST("/signin", server.SignIn)
		}
		auth.GET("/session", server.GetSession)
		auth.POST("/signout", server.SignOut)

		// New authentication routes (email, Google)
		auth.POST("/register", authLimiter, authHandler.Register)
		auth.POST("/login", authLimiter, authHandler.Login)
		// Public demo one-click sign-in; 404 unless DEMO_MODE.
		auth.POST("/demo/login", authLimiter, authHandler.DemoLogin)
		auth.POST("/logout", authHandler.Logout)
		auth.GET("/me", server.AuthenticationMiddleware(), authHandler.GetCurrentUser)

		// OAuth routes
		auth.GET("/google", authHandler.GoogleAuthURL)
		auth.GET("/google/callback", authHandler.GoogleCallback)
		auth.GET("/google/staff", authHandler.GoogleStaffAuthURL)
		auth.GET("/google/staff/callback", authHandler.GoogleStaffCallback)
		// Wallet linking (for authenticated users)
		auth.POST("/wallet/link", server.AuthenticationMiddleware(), authHandler.LinkWallet)
		auth.DELETE("/wallet/unlink", server.AuthenticationMiddleware(), authHandler.UnlinkWallet)
		auth.GET("/methods", server.AuthenticationMiddleware(), authHandler.GetAuthMethods)

		// Session info (reads httpOnly cookies server-side; no middleware needed)
		auth.GET("/session-info", server.GetSessionInfo)

		// Password reset
		auth.POST("/password/reset-request", authHandler.RejectEmptyPasswordResetEmail(), authLimiter, authHandler.RequestPasswordReset)
		auth.POST("/password/reset", authLimiter, authHandler.ResetPassword)

		// Email verification
		auth.POST("/email/verify", authLimiter, authHandler.VerifyEmail)
		auth.POST("/email/resend-verification", authLimiter, authHandler.ResendVerification)

		// Token refresh
		auth.POST("/refresh", authLimiter, server.RefreshToken)
	}

	// Public routes (do not require authentication)
	publicRoutes := r.Group("/api/v1/")
	// B9: Shared per-IP limiter (middleware.GuestOrderWriteRequestsPerMinute)
	// for unauthenticated guest create/pay routes. Declared outside the
	// publicRoutes block so crypto payment wiring (registered later in main)
	// can reuse the same budget.
	guestOrderRateLimiter := middleware.NewSimpleRateLimiter(context.Background(),
		intEnv("GUEST_ORDER_RATE_LIMIT_REQUESTS_PER_MINUTE", middleware.GuestOrderWriteRequestsPerMinute))
	{
		// Health check endpoints (#286). /health/live stays a minimal public
		// probe for Caddy/Docker. /health and /health/ready return status-only
		// without HEALTH_DETAIL_TOKEN; the detailed payloads (latency, uptime,
		// goroutines, component matrix) require that bearer/query token.
		publicRoutes.GET("/health", health.PublicHandler(database.GetDB(), health.Handler(database.GetDB(), "1.0.0")))
		readinessComponents := readinessComponentsFromPreflight(preflight)
		// Safe AI privacy readiness (no secrets/prompts — mode + status only).
		// Degraded (audit mode gaps) is non-blocking so local/dev stays ready;
		// failed (enforce violations) blocks readiness.
		if st := llm.PrivacyReadiness(); st.Status != "" {
			aiPrivStatus := "ok"
			aiPrivCode := ""
			switch st.Status {
			case "failed":
				aiPrivStatus = "failed"
				aiPrivCode = "ai_privacy.failed"
			case "degraded":
				aiPrivCode = "ai_privacy.degraded"
			}
			readinessComponents = append(readinessComponents, health.ComponentResult{
				Component: "ai_privacy",
				Status:    aiPrivStatus,
				Code:      aiPrivCode,
				Source:    "config",
			})
		}
		// Built once: the cached probe closure must not construct a store per request.
		emailOutboxStore := emails.NewGormOutboxStore(database.GetDB())
		readinessState := health.ReadinessState{
			DB:         database.GetDB(),
			Components: readinessComponents,
			ExtraComponents: newCachedLiveReadiness(readinessLiveCacheTTL, time.Now, func() []health.ComponentResult {
				return []health.ComponentResult{emailOutboxReadiness(context.Background(), emailOutboxStore, time.Now())}
			}),
		}
		publicRoutes.GET("/health/ready", health.PublicReadinessHandler(readinessState))
		publicRoutes.GET("/health/live", health.LivenessHandler())
		publicRoutes.GET("/instance", server.GetInstanceInfo)
		publicRoutes.GET("/demo/tables", server.GetDemoTables) // 404 unless DEMO_MODE

		// P2-10: tokenized marketing opt-out — POST is the RFC-8058 one-click
		// target, GET is the /unsubscribe page's status probe. Public, rate-limited.
		publicRoutes.GET("/email/unsubscribe", publicFormLimiter, server.GetUnsubscribeStatus)
		publicRoutes.POST("/email/unsubscribe", publicFormLimiter, server.UnsubscribeMarketingEmail)

		// General Routes

		// Error logging endpoint (stricter rate limit: 30 req/min per IP)
		errorLogRateLimiter := middleware.NewSimpleRateLimiter(context.Background(), 30)
		errorLogHandler := handlers.NewErrorLogHandler()
		publicRoutes.POST("/logs/error", errorLogRateLimiter.RateLimit(), middleware.JSONSizeLimit(handlers.MaxErrorLogBodyBytes), errorLogHandler.IngestError)

		// Page Analytics Tracking Routes (public - no auth required).
		// One shared per-IP limiter (120 req/min) fronts all four ingest
		// endpoints so an attacker can't flood analytics / stress the DB by
		// fanning bogus events across the three routes to triple their budget.
		// The frontend dedups interactions per section and fires page-views per
		// navigation, so legitimate clients sit well under the cap.
		pageAnalyticsService := analytics.NewPageAnalyticsService(db)
		pageAnalyticsHandler := handlers.NewPageAnalyticsHandler(pageAnalyticsService)
		activationEventHandler := handlers.NewActivationEventHandler(database.GetDBWrapper())
		pageAnalyticsRateLimiter := middleware.NewSimpleRateLimiter(context.Background(), 120)
		publicRoutes.POST("/analytics/page-view", pageAnalyticsRateLimiter.RateLimit(), pageAnalyticsHandler.TrackPageView)
		publicRoutes.POST("/analytics/interaction", pageAnalyticsRateLimiter.RateLimit(), pageAnalyticsHandler.TrackInteraction)
		publicRoutes.POST("/analytics/conversion", pageAnalyticsRateLimiter.RateLimit(), pageAnalyticsHandler.TrackConversion)
		publicRoutes.POST("/analytics/activation-event", pageAnalyticsRateLimiter.RateLimit(), activationEventHandler.RecordClientEvent)

		// IMP-31: sendBeacon ingest for translation-key misses. Rate-limited
		// per IP at 60 req/min — the frontend already dedups per session, so
		// healthy clients sit well below the cap and bots get throttled
		// without breaking real telemetry.
		missingTranslationRateLimiter := middleware.NewSimpleRateLimiter(context.Background(), 60)
		missingTranslationHandler := handlers.NewMissingTranslationHandler()
		publicRoutes.POST("/analytics/missing_translation", missingTranslationRateLimiter.RateLimit(), missingTranslationHandler.IngestMissing)

		// Payverge public routes (for guests). Share the guest-table read
		// budget so /table/:code cannot bypass /guest/table/:code probing limits (#293).
		// Budget lives in middleware.GuestTableReadRequestsPerMinute and must
		// cover a full table of diners behind a venue NAT (#814).
		guestTableReadLimiter := middleware.NewSimpleRateLimiter(context.Background(),
			intEnv("GUEST_TABLE_READ_RATE_LIMIT_REQUESTS_PER_MINUTE", middleware.GuestTableReadRequestsPerMinute))

		// Spaces phone-scan token routes (opaque token, rate-limited, no broad account access).
		// Token is crypto/rand base64url; only SHA-256 is stored. Pair code or
		// authenticated business access required on /connect.
		spaceScanRateLimiter := middleware.NewSimpleRateLimiter(context.Background(), 60)
		publicRoutes.GET("/space-scan/:token", spaceScanRateLimiter.RateLimit(), server.PublicSpaceScanSessionMeta)
		publicRoutes.POST("/space-scan/:token/connect", spaceScanRateLimiter.RateLimit(), server.PublicSpaceScanConnect)
		publicRoutes.POST("/space-scan/:token/status", spaceScanRateLimiter.RateLimit(), server.PublicSpaceScanStatus)
		publicRoutes.POST("/space-scan/:token/uploads", spaceScanRateLimiter.RateLimit(), server.PublicSpaceScanUpload)
		publicRoutes.POST("/space-scan/:token/complete-upload", spaceScanRateLimiter.RateLimit(), server.PublicSpaceScanCompleteUpload)
		publicRoutes.GET("/space-scan/:token/result", spaceScanRateLimiter.RateLimit(), server.PublicSpaceScanResult)
		publicRoutes.POST("/space-scan/:token/apply-review", spaceScanRateLimiter.RateLimit(), server.PublicSpaceScanApplyReview)

		// Phase 3: Public guest table API endpoints.
		// GET probing of table codes reuses guestTableReadLimiter (declared
		// above) so enumerable/demo codes cannot be brute-forced cheaply (#293).
		publicRoutes.GET("/guest/table/:code", guestTableReadLimiter.RateLimit(), server.GetTableByCodePublic)
		publicRoutes.GET("/guest/table/:code/bill", guestTableReadLimiter.RateLimit(), server.GetOpenBillByTableCode)

		// B9: The public guest endpoints that CREATE an order or a bill have no
		// auth, so without a limiter an attacker can flood them. guestOrderRateLimiter
		// (declared above publicRoutes) fronts creation routes. The budget is
		// intentionally generous so legitimate diners sharing one restaurant
		// NAT/WiFi IP are not throttled, while bot floods still get capped.
		publicRoutes.POST("/guest/table/:code/bill", guestOrderRateLimiter.RateLimit(), server.CreateBillByTableCode)
		publicRoutes.POST("/guest/table/:code/order", guestOrderRateLimiter.RateLimit(), server.CreateGuestOrder)
		publicRoutes.POST("/guest/table/:code/order/quote", guestOrderRateLimiter.RateLimit(), server.QuoteGuestOrder)
		publicRoutes.POST("/guest/table/:code/orders/:orderId/cancel", guestOrderRateLimiter.RateLimit(), server.GuestCancelOrder)
		publicRoutes.GET("/guest/table/:code/business", guestTableReadLimiter.RateLimit(), server.GetBusinessByTableCode)
		publicRoutes.GET("/guest/table/:code/menu", guestTableReadLimiter.RateLimit(), server.GetMenuByTableCode)
		publicRoutes.GET("/guest/table/:code/events", guestTableReadLimiter.RateLimit(), server.GuestTableEvents)
		// Guest service call ("call waiter"): POST raises/refreshes, GET polls.
		// Reasons are a closed enum (water|order|check) — never guest free text.
		publicRoutes.POST("/guest/table/:code/service-call", guestOrderRateLimiter.RateLimit(), server.CreateServiceCallByTableCode)
		publicRoutes.GET("/guest/table/:code/service-call", guestTableReadLimiter.RateLimit(), server.GetServiceCallStatusByTableCode)
		publicRoutes.POST("/guest/table/:code/validate-promo", guestOrderRateLimiter.RateLimit(), server.ValidatePromoCode)
		publicRoutes.GET("/guest/table/:code/loyalty-rate", guestTableReadLimiter.RateLimit(), server.GetLoyaltyRate)
		// Cookie-authenticated guest loyalty MUTATIONS: layer the Origin-allowlist
		// guard (as on the customer/* groups) on top of customer auth. SameSite=Lax
		// alone doesn't stop a same-registrable-domain pivot, and these mutate
		// persistent loyalty-point state. The rest of publicRoutes is read-only or
		// webhook/guest-create, so the guard is applied per-route here.
		publicRoutes.POST("/guest/table/:code/redeem-points", middleware.RequireTrustedOriginForMutations(allowedOrigins), server.CustomerAuthenticationMiddleware(), server.RedeemLoyaltyPoints)
		publicRoutes.POST("/guest/table/:code/undo-redemption", middleware.RequireTrustedOriginForMutations(allowedOrigins), server.CustomerAuthenticationMiddleware(), server.UndoLoyaltyRedemption)
		publicRoutes.GET("/guest/table/:code/points-earned", server.CustomerAuthenticationMiddleware(), server.GetLoyaltyPointsEarned)
		publicRoutes.GET("/guest/bill/:bill_token", server.GetBillByNumberPublic)
		// Fiscal customer identity (T22): guests set their own recipient fiscal
		// data at checkout (before payment), keyed by the opaque bill number.
		publicRoutes.POST("/guest/bill/:bill_token/fiscal-customer", guestOrderRateLimiter.RateLimit(), server.SetBillFiscalCustomerByNumber)
		// Guest factura access (Wave A): status JSON + PDF stream, keyed by the
		// same opaque bill token. Rate-limit both (JSON enumeration + S3 PDF).
		publicRoutes.GET("/guest/bill/:bill_token/fiscal-receipt", guestOrderRateLimiter.RateLimit(), server.GetGuestFiscalReceipt)
		publicRoutes.GET("/guest/bill/:bill_token/fiscal-receipt/pdf", guestOrderRateLimiter.RateLimit(), server.GetGuestFiscalReceiptPDF)

		// Public business page API endpoints
		publicRoutes.GET("/business/:customUrl/menu", guestTableReadLimiter.RateLimit(), server.GetMenuByBusinessCustomUrl)

		// Phase 4: Payment processing endpoints (public guest access via opaque bill numbers)

		// Phase 5: Bill Splitting routes (public for guests via opaque bill numbers)
		splittingHandler := handlers.NewSplittingHandler(database.GetDBWrapper())
		publicRoutes.GET("/guest/bill/:bill_token/split/options", splittingHandler.GetBillSplitOptions)
		publicRoutes.GET("/guest/bill/:bill_token/split/state", splittingHandler.GetSplitState)
		publicRoutes.GET("/guest/bill/:bill_token/split/events", splittingHandler.StreamSplitEvents)
		publicRoutes.GET("/guest/bill/:bill_token/split/my-shares", splittingHandler.GetMySplitShares)
		publicRoutes.GET("/guest/bill/:bill_token/split/shares/:share_id/receipt", splittingHandler.GetSplitShareReceipt)
		publicRoutes.POST("/guest/bill/:bill_token/split/holds", guestOrderRateLimiter.RateLimit(), splittingHandler.CreateSplitHold)
		publicRoutes.POST("/guest/bill/:bill_token/split/shares/:share_id/release", guestOrderRateLimiter.RateLimit(), splittingHandler.ReleaseSplitShare)
		publicRoutes.POST("/guest/bill/:bill_token/split/equal", splittingHandler.CalculateEqualSplit)
		publicRoutes.POST("/guest/bill/:bill_token/split/custom", splittingHandler.CalculateCustomSplit)
		publicRoutes.POST("/guest/bill/:bill_token/split/items", splittingHandler.CalculateItemSplit)
		publicRoutes.POST("/guest/bill/:bill_token/split/validate", splittingHandler.ValidateSplit)

		publicRoutes.POST("/guest/bill/:bill_token/split/execute", guestOrderRateLimiter.RateLimit(), splittingHandler.ExecuteSplitPayment)

		// WebSocket endpoint removed - using polling instead

		// Alternative Payment routes (public for guests via opaque bill numbers)
		publicRoutes.POST("/guest/bill/:bill_token/request-alternative-payment", guestOrderRateLimiter.RateLimit(), paymentHandler.RequestAlternativePayment)
		publicRoutes.GET("/guest/bill/:bill_token/alternative-payments", guestOrderRateLimiter.RateLimit(), paymentHandler.GetBillAlternativePayments)
		publicRoutes.GET("/guest/bill/:bill_token/payment-breakdown", paymentHandler.GetBillPaymentBreakdown)
		// Guest "email my receipt": settled bills only, guest-typed recipient.
		publicRoutes.POST("/guest/bill/:bill_token/email-receipt", guestOrderRateLimiter.RateLimit(), paymentHandler.EmailBillReceipt)

		// Guest Order routes (public for guests)
		publicRoutes.GET("/guest/bill/:bill_token/orders", guestTableReadLimiter.RateLimit(), handlers.GetGuestOrdersByBillNumber)

		// Staff Authentication routes (public - no auth required)
		staffAuthLimiter := middleware.AuthRateLimiter(
			intEnv("STAFF_AUTH_RATE_LIMIT_REQUESTS_PER_MINUTE", 5),
			intEnv("STAFF_AUTH_RATE_LIMIT_BURST", 2),
		)
		publicRoutes.GET("/staff/invitation-preview", staffAuthLimiter, server.GetStaffInvitationPreview)
		publicRoutes.POST("/staff/accept-invitation", staffAuthLimiter, server.AcceptInvitation)
		publicRoutes.POST("/staff/request-login-code", staffAuthLimiter, server.RequestLoginCode)
		publicRoutes.POST("/staff/verify-login-code", staffAuthLimiter, server.VerifyLoginCode)

		// Staff Profile routes (require staff authentication)
		staffRoutes := r.Group("/api/v1/staff")
		staffRoutes.Use(server.StaffAuthenticationMiddleware())
		// Guard the cookie-authenticated staff mutation (POST /logout) with the
		// Origin-allowlist, matching every other authenticated mutation group. The
		// guard passes GET through, so /profile is unaffected.
		staffRoutes.Use(middleware.RequireTrustedOriginForMutations(allowedOrigins))
		staffRoutes.GET("/profile", server.GetStaffProfile)
		staffRoutes.POST("/logout", server.StaffLogout)

		// Multi-currency routes (public - no auth required for exchange rates)
		currencyHandler := handlers.NewCurrencyHandler(database.GetDBWrapper(), exchangeRateService, translationService)
		publicRoutes.GET("/currencies", guestTableReadLimiter.RateLimit(), currencyHandler.GetSupportedCurrencies)
		publicRoutes.GET("/languages", guestTableReadLimiter.RateLimit(), currencyHandler.GetSupportedLanguages)
		publicRoutes.GET("/exchange-rate", guestTableReadLimiter.RateLimit(), currencyHandler.GetExchangeRate)
		publicRoutes.GET("/convert", guestTableReadLimiter.RateLimit(), currencyHandler.ConvertAmount)

		// Business landing page routes (public - no auth required)
		// Static segment registered BEFORE the :customUrl wildcard so Gin's tree
		// router resolves /business/storefronts (public sitemap slug list, SEO-2)
		// ahead of the param route — no collision.
		publicRoutes.GET("/business/storefronts", server.ListPublishedStorefronts)
		// What the instance root ("/") serves: PRIMARY_VENUE, the single
		// published venue, a directory, or nothing (-> /dashboard).
		publicRoutes.GET("/home", guestTableReadLimiter.RateLimit(), server.GetHome)
		publicRoutes.GET("/business/:customUrl", guestTableReadLimiter.RateLimit(), server.GetBusinessByCustomURL)
		publicRoutes.GET("/business/:customUrl/google/reviews", guestTableReadLimiter.RateLimit(), server.GetPublicBusinessGoogleReviews)
		publicRoutes.GET("/business/:customUrl/google/details", guestTableReadLimiter.RateLimit(), server.GetPublicBusinessGoogleDetails)

		publicRoutes.GET("/platform/registration-mode", server.GetPlatformRegistrationMode)

		// CRM Customer Authentication routes (public - no auth required for registration/login).
		// Login runs bcrypt (CPU-costly) — gate behind authLimiter to blunt credential-stuffing.
		crmAuthRoutes := publicRoutes.Group("/crm")
		crmAuthRoutes.Use(middleware.RequireTrustedOriginForMutations(allowedOrigins))
		{
			crmAuthRoutes.POST("/register", authLimiter, crmHandler.RegisterCustomer)
			crmAuthRoutes.POST("/login", authLimiter, crmHandler.LoginCustomer)
		}

		// Public Reservation routes (for guests to create reservations)
		publicRoutes.GET("/business/:customUrl/reservations/settings", server.GetPublicReservationSettings)
		publicRoutes.POST("/business/:customUrl/reservations", publicReservationLimiter, server.CreatePublicReservation)
		publicRoutes.GET("/business/:customUrl/reservations/availability", guestTableReadLimiter.RateLimit(), server.GetReservationAvailability)
		publicRoutes.GET("/reservations/:confirmationCode", publicReservationLimiter, server.GetPublicReservationByCode)
		publicRoutes.POST("/reservations/:confirmationCode/cancel", publicReservationLimiter, server.CancelPublicReservation)

		// burst 3: a guest legitimately fires a quick follow-up (or double-taps
		// send) without tripping the limiter; sustained rate stays 20/min.
		publicRoutes.POST("/ai-waiter/:businessId", middleware.BusinessRateLimitWithBurst(20, 3), server.HandleAIWaiter)
		publicRoutes.POST("/ai-waiter/:businessId/session", middleware.BusinessRateLimit(20), server.CreateAIWaiterSession)
		publicRoutes.GET("/ai-waiter/:businessId/messages", middleware.BusinessRateLimit(30), server.GetAiWaiterMessages)
		publicRoutes.GET("/ai-waiter/:businessId/stream", middleware.BusinessRateLimit(30), server.HandleAIWaiterStream)

	}

	// Public customer session routes. These intentionally avoid
	// CustomerAuthenticationMiddleware so expired access tokens can refresh and
	// session-info can report unauthenticated instead of 401.
	customerSessionRoutes := r.Group("/api/v1/customer")
	customerSessionRoutes.Use(middleware.RequireTrustedOriginForMutations(allowedOrigins))
	{
		customerSessionRoutes.GET("/session-info", crmHandler.GetCustomerSessionInfo)
		customerSessionRoutes.POST("/refresh", authLimiter, crmHandler.RefreshCustomerToken)
		customerSessionRoutes.POST("/logout", crmHandler.LogoutCustomer)
	}

	// Protected Customer routes (require customer authentication via JWT/session)
	// These routes allow customers to manage their own data
	customerRoutes := r.Group("/api/v1/customer")
	customerRoutes.Use(
		middleware.RequireTrustedOriginForMutations(allowedOrigins),
		server.CustomerAuthenticationMiddleware(),
	)
	{
		// Customer self-service routes (authenticated customers only)
		customerRoutes.POST("/connect-business", crmHandler.ConnectCustomerToBusiness)
		customerRoutes.POST("/table/:code/check-in", server.CheckInCustomerToTable)
		customerRoutes.GET("/profile", crmHandler.GetCustomerProfile)
		customerRoutes.GET("/businesses", crmHandler.GetCustomerBusinesses)
		customerRoutes.PUT("/profile", crmHandler.UpdateCustomerProfile)
		customerRoutes.PUT("/preferences", crmHandler.UpdateCustomerPreferences)
		customerRoutes.POST("/link-wallet", crmHandler.LinkWallet)
		customerRoutes.DELETE("/account", crmHandler.DeleteCustomerAccount)
	}

	// Protected routes (require Web3 or Staff authentication). Mutations on
	// this group also require a trusted Origin/Referer — SameSite=Lax alone
	// doesn't stop a same-registrable-domain pivot.
	protectedRoutes := r.Group("/api/v1/inside")
	protectedRoutes.Use(
		middleware.RequireTrustedOriginForMutations(allowedOrigins),
		server.HybridAuthenticationMiddleware(),
	)
	{
		// Tenant-scoped file mutations. The URL business is canonical; handlers
		// never infer ownership from contact email or caller-supplied object paths.
		// Deliberately no RequireOperationalBusiness here: owners must be able to
		// upload setup assets at any time; tenant + file RBAC remain mandatory for
		// every mutation.
		fileMutationRoutes := protectedRoutes.Group("/businesses/:id/uploads")
		fileMutationRoutes.POST("", server.RoleBasedAccessMiddleware("files:upload"), server.UploadFile)
		fileMutationRoutes.POST("/protected", server.RoleBasedAccessMiddleware("files:upload"), server.UploadFileProtected)
		fileMutationRoutes.POST("/logo", server.RoleBasedAccessMiddleware("files:upload"), server.UploadBusinessLogo)
		fileMutationRoutes.DELETE("", server.RoleBasedAccessMiddleware("files:delete"), server.DeleteUploadedFile)

		// User routes
		protectedRoutes.GET("/get_user/:address", server.GetUser)
		protectedRoutes.PUT("/update_user", server.UpdateUser)
		protectedRoutes.PUT("/set_language", server.SetLanguage)

		// IMP-20: GDPR / UAE PDPL self-service — data export + soft-delete.
		// Both are POSTs because they're side-effecting (export writes an
		// audit-log line + sets cache-busting headers, delete flips a flag).
		protectedRoutes.POST("/account/export", server.ExportAccountData)
		protectedRoutes.POST("/account/delete", server.RequestAccountDeletion)
		// User settings routes
		protectedRoutes.PUT("/settings/notifications", server.UpdateNotificationPreferences)
		protectedRoutes.GET("/settings/notifications", server.GetNotificationPreferences)

		// Payverge Business routes
		protectedRoutes.POST("/businesses/generate-id", server.GenerateBusinessId)
		protectedRoutes.POST("/businesses", server.CreateBusiness)
		protectedRoutes.GET("/businesses", server.GetMyBusinesses)
		protectedRoutes.GET("/businesses/:id", server.GetBusiness)
		protectedRoutes.PUT("/businesses/:id", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("business:settings"), server.UpdateBusiness)
		protectedRoutes.DELETE("/businesses/:id", server.RoleBasedAccessMiddleware("business:settings"), server.DeleteBusiness)
		protectedRoutes.GET("/businesses/check-url", server.CheckCustomURLAvailability)

		protectedRoutes.GET("/businesses/:id/events", server.RoleBasedAccessMiddleware("overview:read"), events.SSEHandler) // SSE real-time events (per-permission topic-scoped in the handler)
		protectedRoutes.GET("/businesses/:id/alerts", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("alerts:read"), operationalAlertHandlers.List)
		protectedRoutes.GET("/businesses/:id/alerts/settings", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("alerts:read"), operationalAlertHandlers.GetSettings)
		protectedRoutes.PUT("/businesses/:id/alerts/settings", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("alerts:settings"), operationalAlertHandlers.UpdateSettings)
		protectedRoutes.POST("/businesses/:id/alerts/:alertId/claim", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("alerts:claim"), operationalAlertHandlers.Claim)
		protectedRoutes.POST("/businesses/:id/alerts/:alertId/resolve", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("alerts:resolve"), operationalAlertHandlers.Resolve)
		protectedRoutes.POST("/businesses/:id/alerts/:alertId/snooze", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("alerts:resolve"), operationalAlertHandlers.Snooze)
		protectedRoutes.GET("/businesses/:id/alerts/:alertId/events", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("alerts:read"), operationalAlertHandlers.Events)

		// Kitchen and Orders toggle routes (paid tier only)
		protectedRoutes.GET("/businesses/:id/kitchen-orders-status", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("settings:read"), server.GetKitchenOrdersStatus)   // Get kitchen/orders status
		protectedRoutes.POST("/businesses/:id/toggle-kitchen-orders", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("settings:write"), server.ToggleKitchenAndOrders) // Toggle kitchen/orders features

		// IMP-06: first-run setup wizard. The state blob is opaque to the
		// backend; the wizard owns the shape. Completion is a separate
		// idempotent endpoint so "Skip for now" and "All six done" share the
		// same dismissal path. requireBusinessAccess gates reads/writes to
		// members of the business.
		protectedRoutes.POST("/businesses/:id/onboarding-state/complete", server.CompleteOnboarding)
		protectedRoutes.GET("/businesses/:id/setup-status", server.GetSetupStatus)

		// Google Places API routes
		protectedRoutes.POST("/google/businesses/search", server.SearchGoogleBusinesses)
		protectedRoutes.PUT("/businesses/:id/google", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("settings:google"), server.UpdateBusinessGoogleInfo)
		protectedRoutes.DELETE("/businesses/:id/google", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("settings:google"), server.RemoveBusinessGoogleInfo)

		// Counter management routes (RBAC Protected, operational business required)
		protectedRoutes.GET("/businesses/:id/counters", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("counter:read"), server.GetBusinessCounters)            // All staff can read
		protectedRoutes.GET("/businesses/:id/counters/available", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("counter:read"), server.GetAvailableCounters) // All staff can read
		protectedRoutes.PUT("/businesses/:id/counters/settings", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("counter:settings"), server.UpdateCounterSettings)

		// Menu routes (RBAC Protected). GET is open to all staff; menu
		// mutations require an operational business so suspended/closed
		// businesses are read-only (matches offers/bundles/inventory below).
		protectedRoutes.GET("/businesses/:id/menu", server.RoleBasedAccessMiddleware("menu:read"), server.GetMenu) // All staff can read
		protectedRoutes.POST("/businesses/:id/menu", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("menu:write"), server.CreateMenu)
		// Granular reorder (§3.7 fix 4): a single category/item move under CAS,
		// so a drag no longer re-uploads the whole menu document.
		protectedRoutes.POST("/businesses/:id/menu/reorder", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("menu:write"), server.ReorderMenu)
		protectedRoutes.POST("/businesses/:id/menu/translate", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("menu:translate"), server.TranslateMenu)

		// Phase 2: Enhanced Menu Management routes (RBAC Protected) - legacy index-based
		protectedRoutes.POST("/businesses/:id/menu/categories", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("menu:categories"), server.AddMenuCategory)
		protectedRoutes.PUT("/businesses/:id/menu/categories/:category_index", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("menu:categories"), server.UpdateMenuCategory)
		protectedRoutes.DELETE("/businesses/:id/menu/categories/:category_index", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("menu:categories"), server.DeleteMenuCategory)
		protectedRoutes.POST("/businesses/:id/menu/items", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("menu:items"), server.AddMenuItem)
		protectedRoutes.PUT("/businesses/:id/menu/items", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("menu:items"), server.UpdateMenuItem)
		protectedRoutes.DELETE("/businesses/:id/menu/categories/:category_index/items/:item_index", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("menu:items"), server.DeleteMenuItem)

		// Phase 3: ID-based menu routes with optimistic concurrency control
		protectedRoutes.PUT("/businesses/:id/menu/category/:category_id", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("menu:categories"), server.UpdateMenuCategory)
		protectedRoutes.DELETE("/businesses/:id/menu/category/:category_id", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("menu:categories"), server.DeleteMenuCategory)
		protectedRoutes.DELETE("/businesses/:id/menu/category/:category_id/item/:item_id", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("menu:items"), server.DeleteMenuItem)

		// Offers & Bundles (operational business required)
		protectedRoutes.POST("/businesses/:id/offers", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("menu:write"), server.CreateOffer)
		protectedRoutes.GET("/businesses/:id/offers", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("menu:read"), server.GetOffers)
		protectedRoutes.PUT("/businesses/:id/offers/:offerId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("menu:write"), server.UpdateOffer)
		protectedRoutes.DELETE("/businesses/:id/offers/:offerId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("menu:write"), server.DeleteOffer)

		protectedRoutes.POST("/businesses/:id/bundles", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("menu:write"), server.CreateBundle)
		protectedRoutes.GET("/businesses/:id/bundles", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("menu:read"), server.GetBundles)
		protectedRoutes.PUT("/businesses/:id/bundles/:bundleId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("menu:write"), server.UpdateBundle)
		protectedRoutes.DELETE("/businesses/:id/bundles/:bundleId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("menu:write"), server.DeleteBundle)

		// Inventory (operational business required)
		protectedRoutes.GET("/businesses/:id/inventory/settings", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("inventory:read"), server.GetInventorySettings)
		protectedRoutes.PUT("/businesses/:id/inventory/settings", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("inventory:write"), server.UpdateInventorySettings)
		protectedRoutes.GET("/businesses/:id/inventory/items", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("inventory:read"), server.ListInventoryItems)
		protectedRoutes.POST("/businesses/:id/inventory/items", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("inventory:write"), server.CreateInventoryItem)
		protectedRoutes.PUT("/businesses/:id/inventory/items/:itemId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("inventory:write"), server.UpdateInventoryItem)
		protectedRoutes.DELETE("/businesses/:id/inventory/items/:itemId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("inventory:write"), server.DeleteInventoryItem)
		protectedRoutes.GET("/businesses/:id/inventory/recipes", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("inventory:recipes"), server.ListInventoryRecipes)
		protectedRoutes.PUT("/businesses/:id/inventory/recipes/:menuItemId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("inventory:recipes"), server.ReplaceInventoryRecipe)
		protectedRoutes.GET("/businesses/:id/inventory/movements", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("inventory:read"), server.ListInventoryMovements)
		protectedRoutes.POST("/businesses/:id/inventory/adjustments", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("inventory:adjust"), server.CreateInventoryAdjustment)
		protectedRoutes.GET("/businesses/:id/inventory/summary", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("inventory:read"), server.GetInventorySummary)

		// AI Menu Features (PDF Digitization, AI Wizard, Image Generation) — operational business required
		aiMenuRoutes := protectedRoutes.Group("/businesses/:id", server.RequireOperationalBusiness())
		{
			aiMenuRoutes.POST("/ai/extract-menu/start", server.RoleBasedAccessMiddleware("menu:write"), server.StartMenuExtraction)
			aiMenuRoutes.POST("/ai/extract-menu/upload/:jobId", server.RoleBasedAccessMiddleware("menu:write"), server.UploadMenuPage)
			aiMenuRoutes.POST("/ai/extract-menu/process/:jobId", server.RoleBasedAccessMiddleware("menu:write"), server.ProcessMenuExtraction)
			aiMenuRoutes.GET("/ai/extract-menu/:jobId", server.RoleBasedAccessMiddleware("menu:read"), server.GetMenuExtractionJob)
			aiMenuRoutes.POST("/ai/import-extracted-menu", server.RoleBasedAccessMiddleware("menu:write"), server.ImportExtractedMenu)
			aiMenuRoutes.POST("/ai/wizard/start", server.RoleBasedAccessMiddleware("menu:write"), server.StartWizardSession)
			aiMenuRoutes.POST("/ai/wizard/:sessionId/message", server.RoleBasedAccessMiddleware("menu:write"), server.SendWizardMessage)
			aiMenuRoutes.GET("/ai/wizard/:sessionId", server.RoleBasedAccessMiddleware("menu:read"), server.GetWizardSession)
			aiMenuRoutes.POST("/ai/wizard/:sessionId/generate", server.RoleBasedAccessMiddleware("menu:write"), server.GenerateMenuFromWizard)
			aiMenuRoutes.POST("/ai/wizard/:sessionId/import", server.RoleBasedAccessMiddleware("menu:write"), server.ImportWizardMenu)
			aiMenuRoutes.POST("/ai/regenerate-image", server.RoleBasedAccessMiddleware("menu:write"), server.RegenerateMenuItemImage)
			aiMenuRoutes.POST("/ai/enhance-image", server.RoleBasedAccessMiddleware("menu:write"), server.EnhanceMenuItemImage)
			aiMenuRoutes.POST("/generate-menu-image", server.RoleBasedAccessMiddleware("menu:write"), server.GenerateMenuImage)
			aiMenuRoutes.GET("/marketing/suggestions", server.RoleBasedAccessMiddleware("marketing:read"), server.GetMarketingSuggestions)
			aiMenuRoutes.POST("/marketing/image", server.RoleBasedAccessMiddleware("marketing:write"), server.GenerateMarketingImage)
			aiMenuRoutes.POST("/marketing/image/cleanup", server.RoleBasedAccessMiddleware("marketing:write"), server.CleanUpMarketingImage)
			aiMenuRoutes.POST("/marketing/caption", server.RoleBasedAccessMiddleware("marketing:write"), server.GenerateMarketingCaption)
			aiMenuRoutes.GET("/marketing/activity", server.RoleBasedAccessMiddleware("marketing:read"), server.GetMarketingActivity)
			aiMenuRoutes.POST("/marketing/activity", server.RoleBasedAccessMiddleware("marketing:write"), server.RecordMarketingActivityHandler)
			aiMenuRoutes.GET("/marketing/settings", server.RoleBasedAccessMiddleware("marketing:read"), server.GetMarketingSettingsHandler)
			aiMenuRoutes.PUT("/marketing/settings", server.RoleBasedAccessMiddleware("marketing:write"), server.PutMarketingSettingsHandler)
		}

		// Table routes (Phase 2: Enhanced Table Management - RBAC Protected, operational business required)
		protectedRoutes.GET("/businesses/:id/tables", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:read"), server.GetBusinessTables)          // All staff can read
		protectedRoutes.GET("/businesses/:id/tables/status", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:read"), server.GetTablesWithStatus) // Get tables with status (occupied, available, reserved)
		protectedRoutes.GET("/businesses/:id/tables/:tableId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:read"), server.GetTable)          // All staff can read
		protectedRoutes.POST("/businesses/:id/tables", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:create"), server.CreateTableWithQR)
		// First-value milestone: intentionally outside RequireOperationalBusiness.
		// ProtectedRoutes supplies canonical authentication/tenant
		// context; ownership is rechecked by the handler and RBAC stays enforced.
		protectedRoutes.POST("/businesses/:id/onboarding/qr-preview", server.RoleBasedAccessMiddleware("tables:read"), server.MarkQRPreviewed)
		protectedRoutes.PUT("/businesses/:id/tables/:tableId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:write"), server.UpdateTable)
		// Host Live View floor actions (seat / clear / transfer / merge). tables:write
		// so Host can run the dinner floor without bills:create / bills:close.
		protectedRoutes.POST("/businesses/:id/tables/:tableId/seat", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:write"), server.SeatTable)
		protectedRoutes.POST("/businesses/:id/tables/:tableId/clear", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:write"), server.ClearTable)
		protectedRoutes.POST("/businesses/:id/tables/:tableId/transfer", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:write"), server.TransferTable)
		protectedRoutes.POST("/businesses/:id/tables/:tableId/merge", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:write"), server.MergeTable)
		protectedRoutes.POST("/businesses/:id/tables/qr-branding", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:write"), server.ApplyQRBrandingToAllTables) // Bulk "Apply to All" QR branding (transactional; replaces the per-table PUT fan-out)
		protectedRoutes.PUT("/tables/:id", server.RequireTableBusinessAccess(), server.RoleBasedAccessMiddleware("tables:write"), server.UpdateTableDetails)
		protectedRoutes.DELETE("/tables/:id", server.RequireTableBusinessAccess(), server.RoleBasedAccessMiddleware("tables:delete"), server.DeleteTableSoft)

		// Spaces & Tables floor-plan routes (reuse tables:* RBAC; operational business required)
		// Static paths (summary, reorder) before :spaceId to avoid Gin param capture.
		protectedRoutes.GET("/businesses/:id/spaces", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:read"), server.ListSpaces)
		protectedRoutes.GET("/businesses/:id/spaces/summary", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:read"), server.GetSpacesSummary)
		protectedRoutes.POST("/businesses/:id/spaces", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:create"), server.CreateSpace)
		protectedRoutes.POST("/businesses/:id/spaces/reorder", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:write"), server.ReorderSpaces)
		protectedRoutes.GET("/businesses/:id/spaces/:spaceId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:read"), server.GetSpace)
		protectedRoutes.PATCH("/businesses/:id/spaces/:spaceId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:write"), server.PatchSpace)
		protectedRoutes.POST("/businesses/:id/spaces/:spaceId/duplicate", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:create"), server.DuplicateSpace)
		protectedRoutes.POST("/businesses/:id/spaces/:spaceId/archive", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:write"), server.ArchiveSpace)
		protectedRoutes.DELETE("/businesses/:id/spaces/:spaceId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:delete"), server.DeleteSpace)
		protectedRoutes.GET("/businesses/:id/spaces/:spaceId/layout/draft", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:read"), server.GetSpaceLayoutDraft)
		protectedRoutes.PUT("/businesses/:id/spaces/:spaceId/layout/draft", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:write"), server.PutSpaceLayoutDraft)
		protectedRoutes.POST("/businesses/:id/spaces/:spaceId/layout/validate", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:read"), server.ValidateSpaceLayout)
		protectedRoutes.POST("/businesses/:id/spaces/:spaceId/layout/publish", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:write"), server.PublishSpaceLayout)
		protectedRoutes.POST("/businesses/:id/spaces/:spaceId/layout/discard", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:write"), server.DiscardSpaceLayout)
		protectedRoutes.GET("/businesses/:id/spaces/:spaceId/layout/published", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:read"), server.GetSpaceLayoutPublished)
		protectedRoutes.POST("/businesses/:id/spaces/:spaceId/tables/assign", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:write"), server.AssignTablesToSpaceHandler)
		protectedRoutes.POST("/businesses/:id/spaces/:spaceId/scan-sessions", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:write"), server.CreateSpaceScanSession)
		protectedRoutes.GET("/businesses/:id/scan-sessions/:sessionId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:read"), server.GetSpaceScanSession)
		protectedRoutes.POST("/businesses/:id/scan-sessions/:sessionId/cancel", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:write"), server.CancelSpaceScanSession)
		protectedRoutes.POST("/businesses/:id/scan-sessions/:sessionId/complete", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:write"), server.CompleteSpaceScanSession)
		protectedRoutes.POST("/businesses/:id/scan-sessions/:sessionId/retry-process", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("tables:write"), server.RetryProcessSpaceScanSession)

		// Reservation Management routes (RBAC Protected, operational business required)
		protectedRoutes.GET("/businesses/:id/reservations/settings", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("reservations:read"), server.GetReservationSettings)
		protectedRoutes.PUT("/businesses/:id/reservations/settings", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("reservations:settings"), server.UpdateReservationSettings)
		protectedRoutes.GET("/businesses/:id/reservations/table-options", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("reservations:read"), server.GetReservationTableOptions)
		protectedRoutes.GET("/businesses/:id/reservations", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("reservations:read"), server.GetReservations)
		protectedRoutes.GET("/businesses/:id/reservations/upcoming", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("reservations:read"), server.GetUpcomingReservations)
		protectedRoutes.GET("/businesses/:id/reservations/stats", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("reservations:read"), server.GetReservationStats)
		protectedRoutes.GET("/businesses/:id/reservations/:reservationId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("reservations:read"), server.GetReservation)
		protectedRoutes.POST("/businesses/:id/reservations", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("reservations:create"), server.CreateReservation)
		protectedRoutes.PUT("/businesses/:id/reservations/:reservationId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("reservations:write"), server.UpdateReservation)
		protectedRoutes.DELETE("/businesses/:id/reservations/:reservationId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("reservations:delete"), server.CancelReservation)
		protectedRoutes.POST("/businesses/:id/reservations/:reservationId/assign-table", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("reservations:write"), server.AssignReservationTable)
		protectedRoutes.POST("/businesses/:id/reservations/:reservationId/check-in", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("reservations:write"), server.CheckInReservation)
		protectedRoutes.POST("/businesses/:id/reservations/:reservationId/no-show", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("reservations:write"), server.MarkReservationNoShow)
		protectedRoutes.POST("/businesses/:id/reservations/:reservationId/promote-waitlist", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("reservations:write"), server.PromoteWaitlistReservation)
		// L4-8: multi-operator claim lock (claim before mutate; steal for manager/owner).
		protectedRoutes.POST("/businesses/:id/reservations/:reservationId/claim", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("reservations:write"), server.ClaimReservation)
		protectedRoutes.POST("/businesses/:id/reservations/:reservationId/release", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("reservations:write"), server.ReleaseReservation)

		// Phase 3: Bill Management routes (RBAC Protected + operational business required)
		protectedRoutes.GET("/businesses/:id/bills", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("bills:read"), server.GetBusinessBills)          // All staff can read
		protectedRoutes.GET("/businesses/:id/bills/open", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("bills:read"), server.GetOpenBusinessBills) // All staff can read
		protectedRoutes.GET("/bills/:bill_id", server.RequireBillBusinessAccess(), server.RoleBasedAccessMiddleware("bills:read"), server.GetBill)                          // All staff can read
		protectedRoutes.POST("/businesses/:id/bills", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("bills:create"), server.CreateBill)
		protectedRoutes.PUT("/bills/:bill_id", server.RequireBillBusinessAccess(), server.RoleBasedAccessMiddleware("bills:write"), server.UpdateBill)
		// Fiscal customer identity (T22): set the recipient doc type/number, tax
		// condition, and name on a bill so the fiscal worker emits the correct
		// factura (A for IVA-registered, B/C otherwise). Same auth as the sibling
		// bill mutation route above.
		protectedRoutes.PUT("/bills/:bill_id/fiscal-customer", server.RequireBillBusinessAccess(), server.RoleBasedAccessMiddleware("bills:write"), server.SetBillFiscalCustomer)
		protectedRoutes.POST("/bills/:bill_id/items", server.RequireBillBusinessAccess(), server.RoleBasedAccessMiddleware("bills:items"), server.AddBillItem)
		// IMP-14: voids on bill items require a manager PIN + audit log. The
		// middleware lets the call through when the actor has no PIN yet
		// (preserves day-1 onboarding) but always records the action.
		protectedRoutes.PATCH("/bills/:bill_id/items/:item_id", server.RequireBillBusinessAccess(), server.RoleBasedAccessMiddleware("bills:items"), server.RequireManagerPIN("bill_item", "void"), server.AdjustBillItem)
		protectedRoutes.POST("/bills/:bill_id/close",
			server.RequireBillBusinessAccess(),
			server.RoleBasedAccessMiddleware("bills:close"),
			middleware.Idempotency(database.GetDB(), "POST /bills/:bill_id/close"), // IMP-03
			server.CloseBill)

		// IMP-15: void + refund flow. Gated on bills:refund, which is owner-only
		// (no staff role holds it) to match the handler's existing owner-only
		// authorization (authorizeBillManagement) — the route RBAC now reflects
		// reality so it can't drift to silently grant staff refund power.
		// RequireManagerPIN gates the action (same pre-write audit semantics as
		// the bill_item void in IMP-14); middleware.Idempotency guarantees a
		// retried network call from the operator UI doesn't refund twice. The
		// handlers live in internal/handlers/bill_void_refund.go alongside
		// PaymentHandler so they can reuse h.authorizeBillManagement.
		protectedRoutes.POST("/bills/:bill_id/void",
			managerPinLimiter,
			server.RequireBillBusinessAccess(),
			server.RoleBasedAccessMiddleware("bills:refund"),
			middleware.Idempotency(database.GetDB(), "POST /bills/:bill_id/void"),
			server.RequireManagerPIN("bill", "void"),
			paymentHandler.VoidBill)
		protectedRoutes.POST("/bills/:bill_id/refund",
			managerPinLimiter,
			server.RequireBillBusinessAccess(),
			server.RoleBasedAccessMiddleware("bills:refund"),
			middleware.Idempotency(database.GetDB(), "POST /bills/:bill_id/refund"),
			server.RequireManagerPIN("bill", "refund"),
			paymentHandler.RefundBillPayment)
		protectedRoutes.GET("/bills/:bill_id/audit",
			server.RequireBillBusinessAccess(),
			server.RoleBasedAccessMiddleware("bills:read"),
			paymentHandler.GetBillAuditLog)

		// Wave 4 REFUND track — durable noncustodial crypto refunds.
		// Owner-only by default (refunds:crypto:* absent from staff roles).
		// Manager PIN step-up + idempotency on request/approve/submit.
		protectedRoutes.GET("/businesses/:id/crypto-refunds",
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("refunds:crypto:request"),
			paymentHandler.ListCryptoRefunds)
		protectedRoutes.POST("/businesses/:id/crypto-refunds",
			managerPinLimiter,
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("refunds:crypto:request"),
			middleware.Idempotency(database.GetDB(), "POST /businesses/:id/crypto-refunds"),
			server.RequireManagerPIN("crypto_refund", "request"),
			paymentHandler.RequestCryptoRefund)
		protectedRoutes.POST("/businesses/:id/crypto-refunds/:refund_id/approve",
			managerPinLimiter,
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("refunds:crypto:approve"),
			middleware.Idempotency(database.GetDB(), "POST /businesses/:id/crypto-refunds/:refund_id/approve"),
			server.RequireManagerPIN("crypto_refund", "approve"),
			paymentHandler.ApproveCryptoRefund)
		protectedRoutes.POST("/businesses/:id/crypto-refunds/:refund_id/reject",
			managerPinLimiter,
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("refunds:crypto:approve"),
			middleware.Idempotency(database.GetDB(), "POST /businesses/:id/crypto-refunds/:refund_id/reject"),
			server.RequireManagerPIN("crypto_refund", "reject"),
			paymentHandler.RejectCryptoRefund)
		protectedRoutes.GET("/businesses/:id/crypto-refunds/:refund_id/unsigned-request",
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("refunds:crypto:approve"),
			paymentHandler.GetCryptoRefundUnsignedRequest)
		protectedRoutes.POST("/businesses/:id/crypto-refunds/:refund_id/submit-tx",
			managerPinLimiter,
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("refunds:crypto:approve"),
			middleware.Idempotency(database.GetDB(), "POST /businesses/:id/crypto-refunds/:refund_id/submit-tx"),
			server.RequireManagerPIN("crypto_refund", "submit"),
			paymentHandler.SubmitCryptoRefundTx)
		protectedRoutes.GET("/businesses/:id/payments/:payment_id/refund-destination",
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("refunds:crypto:request"),
			paymentHandler.GetPaymentRefundDestination)

		protectedRoutes.GET("/businesses/:id/fiscal/settings",
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("fiscal:read"),
			fiscalHandlers.GetSettings)
		protectedRoutes.PUT("/businesses/:id/fiscal/settings",
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("fiscal:write"),
			fiscalHandlers.UpdateSettings)
		protectedRoutes.POST("/businesses/:id/fiscal/validate",
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("fiscal:write"),
			fiscalHandlers.ValidateSettings)
		protectedRoutes.GET("/businesses/:id/fiscal/receipts",
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("fiscal:read"),
			fiscalHandlers.ListReceipts)
		protectedRoutes.GET("/businesses/:id/fiscal/issuable-bills",
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("fiscal:issue"),
			fiscalHandlers.ListIssuableBills)
		protectedRoutes.POST("/businesses/:id/fiscal/receipts/issue",
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("fiscal:issue"),
			fiscalHandlers.IssueReceipt)
		// Letter preview for IssueInvoiceDrawer — single source of truth (no FE rule dupe).
		protectedRoutes.GET("/businesses/:id/fiscal/resolve-type",
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("fiscal:issue"),
			fiscalHandlers.ResolveReceiptType)
		protectedRoutes.POST("/businesses/:id/fiscal/receipts/:receiptId/retry",
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("fiscal:retry"),
			fiscalHandlers.RetryReceipt)
		protectedRoutes.POST("/businesses/:id/fiscal/receipts/:receiptId/credit",
			server.RequireOperationalBusiness(),
			// Credit notes reverse tax/VAT — owner-only (fiscal:credit), not granted to
			// any staff role. Settings update / validate / resend stay on fiscal:write.
			server.RoleBasedAccessMiddleware("fiscal:credit"),
			fiscalHandlers.CreditNote)
		protectedRoutes.POST("/businesses/:id/fiscal/receipts/:receiptId/resend",
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("fiscal:write"),
			fiscalHandlers.ResendReceipt)
		// Wave 4: durable per-channel delivery status + operator requeue.
		protectedRoutes.GET("/businesses/:id/fiscal/receipts/:receiptId/delivery",
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("fiscal:read"),
			fiscalHandlers.ListReceiptDelivery)
		protectedRoutes.POST("/businesses/:id/fiscal/delivery-tasks/:taskId/retry",
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("fiscal:retry"),
			fiscalHandlers.RetryDeliveryTask)
		protectedRoutes.POST("/businesses/:id/fiscal/credentials",
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("fiscal:credentials"),
			fiscalHandlers.UploadCredentials)

		// Sprint 1: Printer Management routes (RBAC Protected + operational business required)
		protectedRoutes.GET("/businesses/:id/printers",
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("printers:read"),
			printerHandlers.ListPrinters)
		protectedRoutes.POST("/businesses/:id/printers",
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("printers:write"),
			printerHandlers.CreatePrinter)
		protectedRoutes.PATCH("/businesses/:id/printers/:printerId",
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("printers:write"),
			printerHandlers.UpdatePrinter)
		protectedRoutes.DELETE("/businesses/:id/printers/:printerId",
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("printers:write"),
			printerHandlers.DeletePrinter)
		protectedRoutes.POST("/businesses/:id/printers/:printerId/test",
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("printers:write"),
			printerHandlers.TestPrint)

		// Sprint 1: Print Job routes (RBAC Protected + operational business required)
		printAgentPermissions := []string{"print:bill", "print:receipt", "orders:kitchen"}
		protectedRoutes.GET("/businesses/:id/print/stations",
			server.RequireOperationalBusiness(),
			server.RoleBasedAnyAccessMiddleware(printAgentPermissions...),
			printJobHandlers.ListBrowserStations)
		protectedRoutes.GET("/businesses/:id/print/jobs/pending-count",
			server.RequireOperationalBusiness(),
			server.RoleBasedAnyAccessMiddleware(printAgentPermissions...),
			printJobHandlers.PendingBrowserJobCount)
		protectedRoutes.GET("/businesses/:id/print/jobs",
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("printers:read"),
			printJobHandlers.ListJobs)
		protectedRoutes.POST("/businesses/:id/print/jobs",
			server.RequireOperationalBusiness(),
			server.RoleBasedAnyAccessMiddleware(printAgentPermissions...),
			printJobHandlers.Create)
		// Wave 4 PRINT — browser-agent lease endpoints (claim/renew vs present/confirm/retry)
		protectedRoutes.POST("/businesses/:id/print/jobs/claim",
			server.RequireOperationalBusiness(),
			server.RoleBasedAnyAccessMiddleware(printAgentPermissions...),
			printJobHandlers.ClaimBrowserJob)
		protectedRoutes.POST("/businesses/:id/print/jobs/:jobId/renew",
			server.RequireOperationalBusiness(),
			server.RoleBasedAnyAccessMiddleware(printAgentPermissions...),
			printJobHandlers.RenewBrowserLease)
		protectedRoutes.POST("/businesses/:id/print/jobs/:jobId/presented",
			server.RequireOperationalBusiness(),
			server.RoleBasedAnyAccessMiddleware(printAgentPermissions...),
			printJobHandlers.MarkBrowserPresented)
		protectedRoutes.POST("/businesses/:id/print/jobs/:jobId/confirm",
			server.RequireOperationalBusiness(),
			server.RoleBasedAnyAccessMiddleware(printAgentPermissions...),
			printJobHandlers.ConfirmBrowserPrinted)
		protectedRoutes.POST("/businesses/:id/print/jobs/:jobId/retry",
			server.RequireOperationalBusiness(),
			server.RoleBasedAnyAccessMiddleware(printAgentPermissions...),
			printJobHandlers.RetryBrowserJob)
		protectedRoutes.POST("/businesses/:id/print/jobs/:jobId/agent-cancel",
			server.RequireOperationalBusiness(),
			server.RoleBasedAnyAccessMiddleware(printAgentPermissions...),
			printJobHandlers.CancelBrowserJob)
		protectedRoutes.POST("/businesses/:id/print/jobs/:jobId/mark-printed",
			server.RequireOperationalBusiness(),
			server.RoleBasedAnyAccessMiddleware(printAgentPermissions...),
			printJobHandlers.MarkPrinted)
		protectedRoutes.POST("/businesses/:id/print/jobs/:jobId/cancel",
			server.RequireOperationalBusiness(),
			server.RoleBasedAccessMiddleware("printers:write"),
			printJobHandlers.Cancel)
		protectedRoutes.POST("/businesses/:id/print/jobs/:jobId/reroute",
			server.RequireOperationalBusiness(),
			server.RoleBasedAnyAccessMiddleware(printAgentPermissions...),
			printJobHandlers.Reroute)
		protectedRoutes.POST("/businesses/:id/print/jobs/:jobId/reprint",
			server.RequireOperationalBusiness(),
			server.RoleBasedAnyAccessMiddleware(printAgentPermissions...),
			printJobHandlers.Reprint)

		// Crypto Payment routes (public for guests)
		publicRoutes.POST("/guest/bill/:bill_token/crypto-payment", guestOrderRateLimiter.RateLimit(), paymentHandler.ProcessCryptoPayment)
		publicRoutes.POST("/guest/bill/:bill_token/crypto-quote", guestOrderRateLimiter.RateLimit(), paymentHandler.IssueCryptoQuote)
		publicRoutes.POST("/guest/bill/:bill_token/cross-chain-payment", guestOrderRateLimiter.RateLimit(), paymentHandler.ProcessCrossChainPayment)

		// Public Delivery Routes (for guests)
		// 30/min sustained, burst 5 — the checkout flow fires quote +
		// re-quote-at-checkout + delivery/orders in close succession.
		deliveryQuoteRateLimit := middleware.BusinessRateLimitWithBurst(30, 5)
		// Rate-limit sequential business-ID probing on the public settings GET (#292).
		publicRoutes.GET("/businesses/:business_id/delivery-settings", deliveryQuoteRateLimit, deliveryHandler.GetDeliverySettings)
		publicRoutes.POST("/businesses/:business_id/delivery/quote", deliveryQuoteRateLimit, deliveryHandler.QuoteDelivery)
		publicRoutes.POST("/businesses/:business_id/delivery/orders", deliveryQuoteRateLimit, deliveryHandler.GuestDeliveryCheckout)
		// M-track: per-client tracking limiter (IPv6 per /64); polled every 15 s
		// by the guest track and pay pages, so sized above two polling tabs.
		deliveryTrackRateLimit := middleware.AuthRateLimiter(120, 20)
		publicRoutes.GET("/delivery/:delivery_number/track", deliveryTrackRateLimit, deliveryHandler.TrackDelivery)

		// Delivery: dispatch (orders)
		protectedRoutes.GET("/businesses/:id/deliveries", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("delivery:dispatch:read"), deliveryHandler.GetBusinessDeliveries)
		protectedRoutes.GET("/businesses/:id/deliveries/:delivery_id", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("delivery:dispatch:read"), deliveryHandler.GetDeliveryOrder)
		protectedRoutes.PATCH("/businesses/:id/deliveries/:delivery_id", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("delivery:dispatch:write"), deliveryHandler.PatchDeliveryOrder)
		protectedRoutes.POST("/businesses/:id/delivery-orders", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("delivery:dispatch:write"), deliveryHandler.CreateDeliveryOrder)
		protectedRoutes.PUT("/businesses/:id/deliveries/:delivery_id/status", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("delivery:dispatch:write"), deliveryHandler.UpdateDeliveryStatus)
		protectedRoutes.POST("/businesses/:id/deliveries/:delivery_id/assign", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("delivery:dispatch:write"), deliveryHandler.AssignDriver)
		protectedRoutes.POST("/businesses/:id/deliveries/:delivery_id/cancel", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("delivery:dispatch:write"), deliveryHandler.CancelDeliveryOrder)
		protectedRoutes.POST("/businesses/:id/deliveries/:delivery_id/claim", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("delivery:dispatch:write"), deliveryHandler.ClaimDeliveryOrder)
		protectedRoutes.POST("/businesses/:id/deliveries/:delivery_id/release", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("delivery:dispatch:write"), deliveryHandler.ReleaseDeliveryOrder)

		// Delivery: drivers
		protectedRoutes.GET("/businesses/:id/drivers", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("delivery:drivers:read"), deliveryHandler.GetBusinessDrivers)
		protectedRoutes.GET("/businesses/:id/drivers/available", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("delivery:drivers:read"), deliveryHandler.GetAvailableDrivers)
		protectedRoutes.GET("/businesses/:id/drivers/performance", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("delivery:drivers:read"), deliveryHandler.ListDriverPerformance)
		protectedRoutes.GET("/businesses/:id/drivers/:driver_id/performance", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("delivery:drivers:read"), deliveryHandler.GetDriverPerformance)
		protectedRoutes.POST("/businesses/:id/drivers", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("delivery:drivers:write"), deliveryHandler.CreateDriver)
		protectedRoutes.PUT("/businesses/:id/drivers/:driver_id", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("delivery:drivers:write"), deliveryHandler.UpdateDriver)
		protectedRoutes.DELETE("/businesses/:id/drivers/:driver_id", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("delivery:drivers:write"), deliveryHandler.DeleteDriver)

		// Delivery: settings
		protectedRoutes.GET("/businesses/:id/delivery-settings", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("delivery:settings:read"), deliveryHandler.GetDeliverySettings)
		protectedRoutes.PUT("/businesses/:id/delivery-settings", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("delivery:settings:write"), deliveryHandler.UpdateDeliverySettings)

		// WhatsApp Integration Routes (RBAC Protected).
		// Status is always mounted so AI Waiter can soft-degrade when WA is dark
		// (WHATSAPP_ENABLED != true) instead of 404ing the channel card.
		protectedRoutes.GET("/businesses/:id/whatsapp/status", server.RoleBasedAccessMiddleware("settings:read"), server.GetWhatsAppStatus)
		if services.WhatsAppAvailable() {
			protectedRoutes.POST("/businesses/:id/whatsapp/connect", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("settings:write"), server.ConnectWhatsApp)
			protectedRoutes.POST("/businesses/:id/whatsapp/disconnect", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("settings:write"), server.DisconnectWhatsApp)
			protectedRoutes.GET("/businesses/:id/whatsapp/qr", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("settings:write"), server.GetWhatsAppQR)
		}

		// Phase 6: Analytics and Dashboard routes (RBAC Protected - Manager+ only, operational business required)
		analyticsHandler := handlers.NewAnalyticsHandler(database.GetDBWrapper())
		protectedRoutes.GET("/businesses/:id/analytics/sales", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("analytics:sales"), analyticsHandler.GetSalesAnalytics)
		protectedRoutes.GET("/businesses/:id/analytics/tips", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("analytics:tips"), analyticsHandler.GetTipAnalytics)
		protectedRoutes.GET("/businesses/:id/analytics/tips-by-staff", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("analytics:tips"), analyticsHandler.GetTipsByStaff)
		protectedRoutes.GET("/businesses/:id/analytics/items", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("analytics:items"), analyticsHandler.GetItemAnalytics)
		protectedRoutes.GET("/businesses/:id/analytics/dashboard", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("overview:kpi"), analyticsHandler.GetDashboardSummary)
		protectedRoutes.POST("/analytics/dashboard-summaries", server.AllowUserLevelPermissions(), server.RoleBasedAccessMiddleware("overview:kpi"), analyticsHandler.GetDashboardSummaries)
		protectedRoutes.GET("/businesses/:id/analytics/timeseries", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("analytics:sales"), analyticsHandler.GetTimeseries)

		// Phase 7: Order Management routes (operational business required)
		orderHandler := handlers.NewOrderHandler(nil)
		protectedRoutes.POST("/businesses/:id/orders", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("orders:create"), orderHandler.CreateOrder)
		protectedRoutes.POST("/businesses/:id/orders/quote", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("orders:create"), server.QuoteBusinessOrder)
		protectedRoutes.GET("/businesses/:id/orders", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("orders:read"), handlers.GetOrders)
		protectedRoutes.GET("/businesses/:id/orders/:orderId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("orders:read"), handlers.GetOrder)
		protectedRoutes.PUT("/businesses/:id/orders/:orderId/status", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("orders:status"), handlers.UpdateOrderStatus)
		protectedRoutes.PATCH("/businesses/:id/orders/:orderId/cancel", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("orders:status"), server.CancelOrder)
		protectedRoutes.GET("/businesses/:id/analytics/live-bills", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("overview:kpi"), analyticsHandler.GetLiveBills)
		protectedRoutes.GET("/businesses/:id/reports/export", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("reports:export"), analyticsHandler.ExportSalesData)
		protectedRoutes.GET("/businesses/:id/payments/history", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:read"), server.GetPaymentHistory)
		protectedRoutes.GET("/businesses/:id/payments/export", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("reports:export"), server.ExportPaymentHistory)

		// Cash Register / Caja routes (front-of-house cash reconciliation)
		cashRegisterHandler := handlers.NewCashRegisterHandler(database.GetDB())
		cashRegisterRoutes := protectedRoutes.Group("/businesses/:id/cash-register")
		cashRegisterRoutes.Use(server.RequireOperationalBusiness())
		{
			cashRegisterRoutes.GET("/current", server.RoleBasedAccessMiddleware(string(server.PermCashRegisterRead)), cashRegisterHandler.GetCurrent)
			cashRegisterRoutes.POST("/sessions", server.RoleBasedAccessMiddleware(string(server.PermCashRegisterOperate)), cashRegisterHandler.OpenSession)
			cashRegisterRoutes.GET("/sessions", server.RoleBasedAccessMiddleware(string(server.PermCashRegisterRead)), cashRegisterHandler.ListSessions)
			cashRegisterRoutes.GET("/sessions/:sessionId", server.RoleBasedAccessMiddleware(string(server.PermCashRegisterRead)), cashRegisterHandler.GetSession)
			cashRegisterRoutes.POST("/sessions/:sessionId/movements", server.RoleBasedAccessMiddleware(string(server.PermCashRegisterOperate)), cashRegisterHandler.CreateMovement)
			cashRegisterRoutes.POST("/sessions/:sessionId/close", server.RoleBasedAccessMiddleware(string(server.PermCashRegisterOperate)), cashRegisterHandler.CloseSession)
			cashRegisterRoutes.GET("/unassigned", server.RoleBasedAccessMiddleware(string(server.PermCashRegisterRead)), cashRegisterHandler.GetUnassigned)
			cashRegisterRoutes.GET("/unassigned/list", server.RoleBasedAccessMiddleware(string(server.PermCashRegisterRead)), cashRegisterHandler.ListUnassigned)
		}

		// Accounting routes (owner + manager access, operational business required)
		accountingHandler := handlers.NewAccountingHandler(database.GetDBWrapper())
		// Recurring ledger entries — non-fatal hourly generator.
		recurringEntriesScheduler = services.NewRecurringEntriesScheduler(database.GetDBWrapper()).WithPeriodLockChecker(func(db *gorm.DB, businessID uint, day time.Time) (bool, error) {
			err := accounting.CheckDateUnlocked(db, businessID, day)
			if errors.Is(err, accounting.ErrPeriodLocked) {
				return true, nil
			}
			return false, err
		})
		if err := recurringEntriesScheduler.Start(); err != nil {
			logger.Logger.Warnf("recurring entries scheduler failed to start: %v", err)
			recurringEntriesScheduler = nil
		}
		staffCompHandler := handlers.NewStaffCompensationHandler(database.GetDBWrapper())
		positionHandler := handlers.NewPositionHandler(database.GetDBWrapper())
		protectedRoutes.GET("/businesses/:id/accounting/summary", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:read"), accountingHandler.GetSummary)
		protectedRoutes.GET("/businesses/:id/accounting/profit-loss", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:read"), accountingHandler.GetProfitLoss)
		protectedRoutes.GET("/businesses/:id/accounting/profit-loss/export.csv", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:read"), accountingHandler.ExportProfitLossCSV)
		protectedRoutes.GET("/businesses/:id/accounting/recurring-templates", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:read"), accountingHandler.ListRecurringTemplates)
		protectedRoutes.POST("/businesses/:id/accounting/recurring-templates", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:write"), accountingHandler.CreateRecurringTemplate)
		protectedRoutes.PATCH("/businesses/:id/accounting/recurring-templates/:templateId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:write"), accountingHandler.UpdateRecurringTemplate)
		protectedRoutes.DELETE("/businesses/:id/accounting/recurring-templates/:templateId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:write"), accountingHandler.DeleteRecurringTemplate)
		protectedRoutes.GET("/businesses/:id/accounting/period-lock", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:read"), accountingHandler.GetPeriodLock)
		protectedRoutes.POST("/businesses/:id/accounting/period-lock", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:write"), accountingHandler.PostPeriodLock)
		protectedRoutes.GET("/businesses/:id/accounting/unpaid-bills", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:read"), accountingHandler.GetUnpaidBills)
		protectedRoutes.GET("/businesses/:id/accounting/categories", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:read"), accountingHandler.ListAccountingCategories)
		protectedRoutes.POST("/businesses/:id/accounting/categories", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:write"), accountingHandler.CreateAccountingCategory)
		protectedRoutes.PATCH("/businesses/:id/accounting/categories/:categoryId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:write"), accountingHandler.UpdateAccountingCategory)
		protectedRoutes.GET("/businesses/:id/accounting/entries/:entryId/attachments", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:read"), accountingHandler.ListEntryAttachments)
		protectedRoutes.POST("/businesses/:id/accounting/entries/:entryId/attachments", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:write"), accountingHandler.UploadEntryAttachment)
		protectedRoutes.GET("/businesses/:id/accounting/entries/:entryId/attachments/:attachmentId/download", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:read"), accountingHandler.DownloadEntryAttachment)
		protectedRoutes.DELETE("/businesses/:id/accounting/entries/:entryId/attachments/:attachmentId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:write"), accountingHandler.DeleteEntryAttachment)

		protectedRoutes.GET("/businesses/:id/accounting/timeseries", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:read"), accountingHandler.GetTimeseries)
		protectedRoutes.GET("/businesses/:id/accounting/food-cost", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:read"), accountingHandler.GetFoodCost)
		protectedRoutes.GET("/businesses/:id/accounting/menu-engineering", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:read"), accountingHandler.GetMenuEngineering)
		protectedRoutes.GET("/businesses/:id/accounting/waste-variance", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:read"), accountingHandler.GetWasteVariance)
		protectedRoutes.GET("/businesses/:id/accounting/labor-cost", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:read"), accountingHandler.GetLaborCost)
		protectedRoutes.GET("/businesses/:id/accounting/entries", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:read"), accountingHandler.ListEntries)
		protectedRoutes.GET("/businesses/:id/accounting/entries/export.csv", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:read"), accountingHandler.ExportEntriesCSV)
		protectedRoutes.POST("/businesses/:id/accounting/entries", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:write"), accountingHandler.CreateManualEntry)
		protectedRoutes.POST("/businesses/:id/accounting/entries/:entryId/void", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:write"), accountingHandler.VoidManualEntry)
		protectedRoutes.GET("/businesses/:id/accounting/payroll-runs", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:read"), accountingHandler.ListPayrollRuns)
		protectedRoutes.GET("/businesses/:id/accounting/payroll-runs/export.csv", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:read"), accountingHandler.ExportPayrollRunsCSV)
		protectedRoutes.POST("/businesses/:id/accounting/payroll-runs", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("payroll:write"), accountingHandler.CreatePayrollRun)
		protectedRoutes.GET("/businesses/:id/accounting/payroll-runs/:runId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("financial:read"), accountingHandler.GetPayrollRun)
		protectedRoutes.POST("/businesses/:id/accounting/payroll-runs/:runId/mark-paid", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("payroll:write"), accountingHandler.MarkPayrollRunPaid)
		protectedRoutes.DELETE("/businesses/:id/accounting/payroll-runs/:runId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("payroll:write"), accountingHandler.DeletePayrollRun)
		protectedRoutes.POST("/businesses/:id/accounting/payroll-runs/:runId/void", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("payroll:write"), accountingHandler.VoidPayrollRun)
		// Alternative Payment routes (in-person tender: cash, card, venmo, other)
		protectedRoutes.POST("/bills/:bill_id/alternative-payment",
			server.RequireBillBusinessAccess(),
			server.RoleBasedAccessMiddleware(string(server.PermBillsPayment)),
			middleware.Idempotency(database.GetDB(), "POST /bills/:bill_id/alternative-payment"), // IMP-03
			paymentHandler.MarkAlternativePayment)
		protectedRoutes.GET("/bills/:bill_id/pending-alternative-payments",
			server.RequireBillBusinessAccess(),
			server.RoleBasedAccessMiddleware(string(server.PermBillsRead)),
			paymentHandler.GetPendingAlternativePayments)
		protectedRoutes.POST("/bills/:bill_id/pending-alternative-payments/:request_id/cancel",
			server.RequireBillBusinessAccess(),
			server.RoleBasedAccessMiddleware(string(server.PermBillsPayment)),
			paymentHandler.CancelPendingAlternativePayment)
		protectedRoutes.POST("/bills/:bill_id/pending-alternative-payments/:request_id/reject",
			server.RequireBillBusinessAccess(),
			server.RoleBasedAccessMiddleware(string(server.PermBillsPayment)),
			paymentHandler.RejectPendingAlternativePayment)
		protectedRoutes.GET("/bills/:bill_id/payment-breakdown",
			server.RequireBillBusinessAccess(),
			server.RoleBasedAccessMiddleware(string(server.PermBillsRead)),
			paymentHandler.GetBillPaymentBreakdown)

		// Staff Management routes (RBAC Protected, operational business required)
		rbacHandlers := handlers.NewRBACHandlers(db)
		protectedRoutes.GET("/businesses/:id/staff", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("staff:read"), server.GetBusinessStaff) // All staff can view
		// Compensation is sensitive (individual wages): a separate owner-only sub-resource, never on the staff list payload.
		protectedRoutes.GET("/businesses/:id/staff/:staffId/compensation", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("payroll:write"), staffCompHandler.GetCompensation)
		protectedRoutes.PUT("/businesses/:id/staff/:staffId/compensation", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("payroll:write"), staffCompHandler.UpdateCompensation)

		// Positions / roles (Slice 0). Catalog + assignment gated on schedule:write;
		// reads on schedule:read. Pay rate is a SEPARATE owner-only sub-resource on
		// payroll:write — pay never rides the assignment payload (StaffPosition.PayRateCents is json:"-").
		protectedRoutes.GET("/businesses/:id/positions", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:read"), positionHandler.List)
		protectedRoutes.POST("/businesses/:id/positions", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:write"), positionHandler.Create)
		protectedRoutes.PATCH("/businesses/:id/positions/:positionId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:write"), positionHandler.Update)
		protectedRoutes.DELETE("/businesses/:id/positions/:positionId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:write"), positionHandler.Delete)
		protectedRoutes.GET("/businesses/:id/staff/:staffId/positions", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:read"), positionHandler.ListForStaff)
		protectedRoutes.POST("/businesses/:id/staff/:staffId/positions", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:write"), positionHandler.Assign)
		protectedRoutes.DELETE("/businesses/:id/staff/:staffId/positions/:positionId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:write"), positionHandler.Unassign)
		protectedRoutes.GET("/businesses/:id/staff/:staffId/positions/:positionId/rate", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("payroll:write"), positionHandler.GetRate)
		protectedRoutes.PUT("/businesses/:id/staff/:staffId/positions/:positionId/rate", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("payroll:write"), positionHandler.SetRate)

		// Per-business schedule settings (Slice 1 foundation; consumed by Slice 2 builder).
		// GET on schedule:read (all staff); PUT on schedule:write (manager/owner). No money fields.
		scheduleSettingsHandler := handlers.NewScheduleSettingsHandler(database.GetDBWrapper())
		protectedRoutes.GET("/businesses/:id/schedule/settings", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:read"), scheduleSettingsHandler.Get)
		protectedRoutes.PUT("/businesses/:id/schedule/settings", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:write"), scheduleSettingsHandler.Put)

		// Logbook / shift-handover notes (Slice 8). Both gated schedule:read — list
		// reads, and per contracts §5 ANY staff may log, so create is schedule:read
		// (not schedule:write). No money fields; author taken from the staff session.
		logbookHandler := handlers.NewLogbookHandler(database.GetDBWrapper())
		protectedRoutes.GET("/businesses/:id/shift-notes", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:read"), logbookHandler.List)
		// Any staff with schedule:read may create shift notes (handover log); write is
		// reserved for schedule structure mutations (positions, publish, settings).
		protectedRoutes.POST("/businesses/:id/shift-notes", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:read"), logbookHandler.Create)

		// Scheduling core (Slice 2). GET is row-scoped in the handler (managers see
		// draft+all; plain staff see published + own + open). Writes on
		// schedule:write, publish on schedule:publish — perms minted in Slice 0.
		scheduleHandler := handlers.NewScheduleHandler(database.GetDBWrapper())
		protectedRoutes.GET("/businesses/:id/schedule", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:read"), scheduleHandler.Get)
		protectedRoutes.POST("/businesses/:id/schedule", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:write"), scheduleHandler.CreateDraft)
		// L5-26 / decision #10: attempt-scoped Idempotency-Key on shift create so a
		// lost-response retry (and multi-day fan-out per-day keys) replays the
		// cached 201 instead of double-creating. Same middleware as bill close /
		// alt-payment — no new mechanism.
		protectedRoutes.POST("/businesses/:id/shifts", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:write"), middleware.Idempotency(database.GetDB(), "POST /businesses/:id/shifts"), scheduleHandler.CreateShift)
		protectedRoutes.PATCH("/businesses/:id/shifts/:shiftId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:write"), scheduleHandler.UpdateShift)
		protectedRoutes.DELETE("/businesses/:id/shifts/:shiftId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:write"), scheduleHandler.DeleteShift)
		protectedRoutes.POST("/businesses/:id/schedule/:scheduleId/publish", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:publish"), scheduleHandler.Publish)
		// Transactional copy-week: {from_week,to_week,only_empty_days} copies all
		// source shifts into the destination draft in one call (replaces the client's
		// N serial per-shift POSTs). schedule:write.
		protectedRoutes.POST("/businesses/:id/schedule/copy-week", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:write"), scheduleHandler.CopyWeek)
		protectedRoutes.GET("/businesses/:id/schedule/:scheduleId/labor-preview", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:write"), scheduleHandler.LaborPreview)

		// Availability & time-off (Slice 3). Availability is self-service
		// (schedule:self, always scoped to the caller's own staff_id). Time-off:
		// any staff files a request (schedule:self); the list is route-gated on
		// schedule:read but the SERVICE row-scopes by role (approvers see all,
		// plain staff see own); decisions need schedule:approve. No money fields.
		availabilityHandler := handlers.NewAvailabilityHandler(database.GetDBWrapper())
		protectedRoutes.GET("/businesses/:id/me/availability", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:self"), availabilityHandler.GetMyAvailability)
		protectedRoutes.PUT("/businesses/:id/me/availability", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:self"), availabilityHandler.PutMyAvailability)
		// Team availability overlay for the manager's schedule builder: every
		// staff member's recurring windows grouped by staff_id, one bounded query.
		// Manager-gated on schedule:write (full-team PII; servers only hold schedule:read).
		protectedRoutes.GET("/businesses/:id/team-availability", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:write"), availabilityHandler.GetTeamAvailability)
		protectedRoutes.POST("/businesses/:id/me/time-off", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:self"), availabilityHandler.CreateMyTimeOff)
		protectedRoutes.GET("/businesses/:id/time-off", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:read"), availabilityHandler.ListTimeOff)
		protectedRoutes.POST("/businesses/:id/time-off/:reqId/decision", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:approve"), availabilityHandler.DecideTimeOff)

		// Time clock (Slice 4). Punch endpoints (clock-in/out/break + own
		// timesheet) are self-service (timeclock:punch, always scoped to the
		// caller's own staff_id; hours/minutes only, never dollars). The review
		// queue, approvals, and manual entries are manager-gated (timeclock:manage;
		// owner bypasses). Any labor-$ derived from approved entries lives only in
		// the owner/financial:read-gated accounting labor endpoint, never here.
		timeclockHandler := handlers.NewTimeclockHandler(database.GetDBWrapper())
		protectedRoutes.POST("/businesses/:id/me/clock-in", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("timeclock:punch"), timeclockHandler.ClockIn)
		protectedRoutes.POST("/businesses/:id/me/clock-out", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("timeclock:punch"), timeclockHandler.ClockOut)
		protectedRoutes.POST("/businesses/:id/me/break", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("timeclock:punch"), timeclockHandler.Break)
		protectedRoutes.GET("/businesses/:id/me/timesheet", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("timeclock:punch"), timeclockHandler.GetMyTimesheet)
		protectedRoutes.GET("/businesses/:id/timesheets", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("timeclock:manage"), timeclockHandler.ListTimesheets)
		protectedRoutes.POST("/businesses/:id/timesheets/:entryId/approve", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("timeclock:manage"), timeclockHandler.ApproveTimesheet)
		// Time-entry corrections (timeclock:manage): reject (returns the entry to the
		// staffer with a reason) + manager edit (adjust clock-in/out; original values
		// kept in the RBAC audit trail). Approved entries are locked.
		protectedRoutes.POST("/businesses/:id/timesheets/:entryId/reject", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("timeclock:manage"), timeclockHandler.RejectTimesheet)
		protectedRoutes.PATCH("/businesses/:id/timesheets/:entryId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("timeclock:manage"), timeclockHandler.EditTimesheet)
		protectedRoutes.POST("/businesses/:id/time-entries", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("timeclock:manage"), timeclockHandler.CreateManualEntry)
		// Live floor (Phase 3): manager board of who's on the clock / scheduled /
		// late / no-show for the business day. Read-only, timeclock:manage-gated,
		// money-free (hours-context only, no dollars).
		protectedRoutes.GET("/businesses/:id/live-floor", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("timeclock:manage"), timeclockHandler.LiveFloor)

		// Kiosk clock-in (Phase 4b): a shared terminal runs under a
		// manager/owner session (timeclock:manage) and lets staff toggle their
		// OWN punch with a PIN. No independent staff session is minted — the PIN
		// only attributes the punch. Roster + punch are both money-free; the
		// staffer is tenant-scoped in the DB before any PIN check.
		kioskHandler := handlers.NewKioskHandler(database.GetDBWrapper())
		protectedRoutes.GET("/businesses/:id/kiosk/roster", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("timeclock:manage"), kioskHandler.Roster)
		protectedRoutes.POST("/businesses/:id/kiosk/punch", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("timeclock:manage"), kioskHandler.Punch)

		// Staff notifications (Slice 9). Self-scoped inbox: list/unread-count/read.
		// Every staff member gets these — no extra RBAC permission; the handler
		// self-route guard (staffIDFromContext) rejects staff_id==0 with 403.
		// Inbox rows are money-free.
		notificationsHandler := handlers.NewNotificationsHandler(database.GetDBWrapper())
		protectedRoutes.GET("/businesses/:id/me/notifications", server.RequireOperationalBusiness(), notificationsHandler.ListMine)
		protectedRoutes.GET("/businesses/:id/me/notifications/unread-count", server.RequireOperationalBusiness(), notificationsHandler.UnreadCount)
		protectedRoutes.POST("/businesses/:id/me/notifications/read", server.RequireOperationalBusiness(), notificationsHandler.MarkRead)

		// Engagement badges — the caller's own "needs attention" counts (unacked
		// require_ack announcements + not-yet-complete checklist runs) so the staff
		// nav can badge buried comms/checklists. A /me self route: self-scoped by
		// context staff_id (owner → 403), no RBAC middleware needed. Money-free.
		engagementBadgesHandler := handlers.NewEngagementBadgesHandler(database.GetDBWrapper())
		protectedRoutes.GET("/businesses/:id/me/engagement-badges", server.RequireOperationalBusiness(), engagementBadgesHandler.Badges)

		// Coverage (Slice 5). Staff claim open shifts / request swaps / accept a
		// coworker's swap (schedule:self, always scoped to the caller's own staff_id;
		// eligibility = holding the shift's position). Managers approve/deny via the
		// single polymorphic decision route (schedule:approve; body kind=swap|open_claim).
		// Coverage lists are schedule:read (role-branched: approvers also get the
		// pending-approval queue) / schedule:self (the caller's own claims+swaps).
		coverageHandler := handlers.NewCoverageHandler(database.GetDBWrapper())
		protectedRoutes.POST("/businesses/:id/shifts/:shiftId/claim", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:self"), coverageHandler.ClaimOpenShift)
		protectedRoutes.POST("/businesses/:id/shifts/:shiftId/swap", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:self"), coverageHandler.RequestSwap)
		protectedRoutes.POST("/businesses/:id/swaps/:swapId/accept", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:self"), coverageHandler.AcceptSwap)
		protectedRoutes.POST("/businesses/:id/swaps/:swapId/decision", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:approve"), coverageHandler.Decide)
		// Staff retract their OWN request (swap/giveup/open_claim) while it's still
		// non-terminal — schedule:self, ownership enforced in the service layer.
		protectedRoutes.POST("/businesses/:id/coverage/:requestId/cancel", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:self"), coverageHandler.Cancel)
		protectedRoutes.GET("/businesses/:id/coverage/open", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:read"), coverageHandler.ListOpen)
		protectedRoutes.GET("/businesses/:id/coverage/mine", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:self"), coverageHandler.ListMine)
		protectedRoutes.GET("/businesses/:id/coverage/history", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("schedule:read"), coverageHandler.History)

		// Staff chat (Slice 7). Every staff role holds chat:read+chat:send; only
		// manager/owner hold chat:announce+chat:moderate. The handler enforces the
		// per-channel privacy gate (CanReadChannel -> 403) on top of these route
		// permissions; SSE frames published on post/announce are content-free (ids
		// only — see internal/handlers/chat.go + events/sse_permissions.go).
		chatHandler := handlers.NewChatHandler(database.GetDBWrapper())
		protectedRoutes.GET("/businesses/:id/chat/channels", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("chat:read"), chatHandler.GetChannels)
		protectedRoutes.GET("/businesses/:id/chat/channels/:channelId/messages", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("chat:read"), chatHandler.GetMessages)
		protectedRoutes.POST("/businesses/:id/chat/channels/:channelId/messages", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("chat:send"), chatHandler.PostMessage)
		protectedRoutes.POST("/businesses/:id/chat/channels/:channelId/read", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("chat:read"), chatHandler.MarkRead)
		protectedRoutes.POST("/businesses/:id/chat/dm/:staffId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("chat:send"), chatHandler.PostDM)
		protectedRoutes.DELETE("/businesses/:id/chat/messages/:messageId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("chat:moderate"), chatHandler.DeleteMessage)
		// Announcements (broadcast + ack roster). Create + the acks roster are
		// announce-only (manager/owner); read + ack are chat:read (all staff).
		protectedRoutes.POST("/businesses/:id/announcements", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("chat:announce"), chatHandler.CreateAnnouncement)
		protectedRoutes.GET("/businesses/:id/announcements", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("chat:read"), chatHandler.ListAnnouncements)
		protectedRoutes.POST("/businesses/:id/announcements/:announcementId/ack", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("chat:read"), chatHandler.AckAnnouncement)
		protectedRoutes.GET("/businesses/:id/announcements/:announcementId/acks", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("chat:announce"), chatHandler.GetAnnouncementAcks)
		// Batch ack-summary (kills the per-announcement AckRoster poll N+1): ?ids=1,2,3
		// returns {acked,total_eligible,first_acker_names} per id in one call. Same
		// chat:announce gate as the roster read (only announce-holders see counts).
		protectedRoutes.GET("/businesses/:id/announcements/ack-summaries", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("chat:announce"), chatHandler.GetAnnouncementAckSummaries)
		// Announcement lifecycle (edit in place / hard delete + audit). chat:announce
		// (manager/owner). Delete stops ack tracking; there is no soft-delete column.
		protectedRoutes.PUT("/businesses/:id/announcements/:announcementId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("chat:announce"), chatHandler.UpdateAnnouncement)
		protectedRoutes.DELETE("/businesses/:id/announcements/:announcementId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("chat:announce"), chatHandler.DeleteAnnouncement)

		// Staff Engagement (Slice 9): checklists. Template authoring + run
		// creation are checklist:manage (manager+owner); listing own runs +
		// ticking items are checklist:complete (all staff, own-run scoped in
		// the handler).
		checklistHandler := handlers.NewChecklistHandler(database.GetDBWrapper())
		protectedRoutes.GET("/businesses/:id/checklists/templates", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("checklist:manage"), checklistHandler.ListTemplates)
		protectedRoutes.POST("/businesses/:id/checklists/templates", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("checklist:manage"), checklistHandler.CreateTemplate)
		protectedRoutes.POST("/businesses/:id/checklists/runs", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("checklist:manage"), checklistHandler.CreateRun)
		protectedRoutes.GET("/businesses/:id/checklists/runs", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("checklist:complete"), checklistHandler.ListRuns)
		protectedRoutes.GET("/businesses/:id/checklists/runs/:runId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("checklist:complete"), checklistHandler.RunDetail)
		protectedRoutes.POST("/businesses/:id/checklists/runs/:runId/items/:itemId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("checklist:complete"), checklistHandler.TickItem)

		// Staff Engagement (Slice 9): versioned documents. Read + ack are
		// doc:read (all staff); authoring is doc:manage (manager+owner). Ack
		// records the document's CURRENT version, so a version bump re-opens it.
		documentHandler := handlers.NewDocumentHandler(database.GetDBWrapper())
		protectedRoutes.GET("/businesses/:id/documents", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("doc:read"), documentHandler.List)
		protectedRoutes.POST("/businesses/:id/documents", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("doc:manage"), documentHandler.Create)
		protectedRoutes.PUT("/businesses/:id/documents/:docId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("doc:manage"), documentHandler.Update)
		protectedRoutes.POST("/businesses/:id/documents/:docId/ack", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("doc:read"), documentHandler.Ack)

		// Staff Engagement (Slice 9): recognition/shoutouts. Both list + send are
		// recognition:send (all-staff). GET is intentionally gated on
		// recognition:send rather than chat:read so this slice stays independently
		// shippable (no Slice-7 dependency) — both are all-staff, identical audience.
		recognitionHandler := handlers.NewRecognitionHandler(database.GetDBWrapper())
		protectedRoutes.GET("/businesses/:id/shoutouts", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("recognition:send"), recognitionHandler.List)
		protectedRoutes.POST("/businesses/:id/shoutouts", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("recognition:send"), recognitionHandler.Create)

		// Staff Engagement (Slice 9): polls. List + vote are poll:vote (all-staff);
		// create + close are poll:manage (manager+owner). One-vote-per-staff is a DB
		// constraint (idx_poll_one_vote) + OnConflict, and anonymous polls never
		// project voter ids in results.
		pollHandler := handlers.NewPollHandler(database.GetDBWrapper())
		protectedRoutes.GET("/businesses/:id/polls", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("poll:vote"), pollHandler.List)
		protectedRoutes.POST("/businesses/:id/polls", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("poll:manage"), pollHandler.Create)
		protectedRoutes.GET("/businesses/:id/polls/:pollId/results", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("poll:vote"), pollHandler.Results)
		protectedRoutes.POST("/businesses/:id/polls/:pollId/vote", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("poll:vote"), pollHandler.Vote)
		protectedRoutes.POST("/businesses/:id/polls/:pollId/close", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("poll:manage"), pollHandler.Close)

		protectedRoutes.POST("/businesses/:id/staff/invite", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("staff:invite"), server.InviteStaff)
		protectedRoutes.POST("/businesses/:id/staff/invitations/:invitationId/resend", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("staff:invite"), server.ResendInvitation)
		// Re-materialize accept URL for authorized inviters only (copy-link).
		// Tokens are intentionally omitted from GET .../staff list responses.
		protectedRoutes.GET("/businesses/:id/staff/invitations/:invitationId/link", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("staff:invite"), server.GetInvitationLink)
		// Revoke an invitation: invalidates its token (the original link dies) and
		// marks it revoked. staff:invite (same gate as invite/resend).
		protectedRoutes.POST("/businesses/:id/staff/invitations/:invitationId/revoke", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("staff:invite"), server.RevokeInvitation)
		protectedRoutes.DELETE("/businesses/:id/staff/:staffId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("staff:delete"), server.RemoveStaff)

		// IMP-14: manager PIN enrollment / rotation. Owners can set any
		// staff's PIN; staff can rotate their own. The handler enforces both
		// rules, so we don't add an RBAC-permission gate here (which would
		// reject self-rotation for non-manager roles).
		protectedRoutes.POST("/staff/:staff_id/pin", server.RequireOperationalBusiness(), server.SetStaffPin)

		// RBAC-specific staff management routes
		protectedRoutes.PUT("/businesses/:id/staff/:staffId/role", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("staff:role"), rbacHandlers.ChangeStaffRole)
		protectedRoutes.GET("/businesses/:id/staff/:staffId/permissions", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("staff:read"), rbacHandlers.GetStaffPermissions)
		protectedRoutes.POST("/businesses/:id/staff/:staffId/permissions", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("staff:permissions"), rbacHandlers.GrantCustomPermission)
		protectedRoutes.DELETE("/businesses/:id/staff/:staffId/permissions", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("staff:permissions"), rbacHandlers.RevokeCustomPermission)
		protectedRoutes.POST("/businesses/:id/staff/:staffId/permission-denies", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("staff:permissions"), rbacHandlers.DenyPermission)
		protectedRoutes.DELETE("/businesses/:id/staff/:staffId/permission-denies", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("staff:permissions"), rbacHandlers.RemovePermissionDeny)
		protectedRoutes.POST("/businesses/:id/staff/:staffId/deactivate", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("staff:deactivate"), rbacHandlers.DeactivateStaff)
		protectedRoutes.POST("/businesses/:id/staff/:staffId/reactivate", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("staff:reactivate"), rbacHandlers.ReactivateStaff)

		// RBAC audit routes
		protectedRoutes.GET("/businesses/:id/staff/:staffId/audit", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("staff:audit"), rbacHandlers.GetStaffAuditLog)

		// Multi-currency and multilingual routes (protected - require authentication)
		currencyHandler := handlers.NewCurrencyHandler(database.GetDBWrapper(), exchangeRateService, translationService)
		protectedRoutes.GET("/businesses/:id/currencies", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("currencies:read"), currencyHandler.GetBusinessCurrencies)
		protectedRoutes.PUT("/businesses/:id/currencies", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("currencies:write"), currencyHandler.UpdateBusinessCurrencies)
		protectedRoutes.GET("/businesses/:id/languages", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("languages:read"), currencyHandler.GetBusinessLanguages)
		protectedRoutes.PUT("/businesses/:id/languages", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("languages:write"), currencyHandler.UpdateBusinessLanguages)
		protectedRoutes.GET("/translations", currencyHandler.GetTranslatedContent)
		protectedRoutes.PUT("/translations", currencyHandler.UpdateTranslation)
		protectedRoutes.GET("/menu-translations", currencyHandler.GetMenuTranslations)
		// Public guest-accessible variant: resolves business via table code so
		// unauthenticated diners can render translated menus without a 401.
		guestCatalogReadLimiter := newGuestCatalogReadLimiter() // sec-med: public catalog reads are per-client throttled
		publicRoutes.GET("/guest/table/:code/menu-translations", guestCatalogReadLimiter, currencyHandler.GetGuestMenuTranslations)

		// Hospitality Features routes (RBAC Protected - Manager+ only; write paths require an operational business)
		protectedRoutes.GET("/businesses/:id/gallery-images", server.RoleBasedAccessMiddleware("settings:read"), server.GetBusinessGalleryImages)   // All staff can read
		protectedRoutes.GET("/businesses/:id/operating-hours", server.RoleBasedAccessMiddleware("settings:read"), server.GetBusinessOperatingHours) // All staff can read
		protectedRoutes.GET("/businesses/:id/operating-exceptions", server.RoleBasedAccessMiddleware("settings:read"), server.GetBusinessOperatingExceptions)
		protectedRoutes.GET("/businesses/:id/special-features", server.RoleBasedAccessMiddleware("settings:read"), server.GetBusinessSpecialFeatures) // All staff can read
		protectedRoutes.PUT("/businesses/:id/design-settings", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("settings:design"), server.UpdateBusinessDesignSettings)
		protectedRoutes.PUT("/businesses/:id/hospitality-settings", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("settings:write"), server.UpdateBusinessHospitalitySettings)
		protectedRoutes.PUT("/businesses/:id/gallery-images", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("settings:write"), server.UpdateBusinessGalleryImages)
		protectedRoutes.PUT("/businesses/:id/operating-hours", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("settings:write"), server.UpdateBusinessOperatingHours)
		protectedRoutes.PUT("/businesses/:id/operating-exceptions", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("settings:write"), server.UpdateBusinessOperatingExceptions)
		protectedRoutes.PUT("/businesses/:id/special-features", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("settings:write"), server.UpdateBusinessSpecialFeatures)

		// Phase 4: AI Waiter Dashboard Routes (RBAC protected + operational business required)
		aiWaiterDashboard := protectedRoutes.Group("/businesses/:id", server.RequireOperationalBusiness())
		{
			aiWaiterDashboard.GET("/ai/conversations", server.RoleBasedAccessMiddleware("ai_waiter:read"), server.GetAiConversations)
			aiWaiterDashboard.GET("/ai/conversations/:convId/messages", server.RoleBasedAccessMiddleware("ai_waiter:read"), server.GetAiConversationMessages)
			aiWaiterDashboard.GET("/ai/insights", server.RoleBasedAccessMiddleware("ai_waiter:insights"), server.GetAiInsights)
			aiWaiterDashboard.POST("/ai/conversations/:convId/claim", server.RoleBasedAccessMiddleware("ai_waiter:reply"), server.ClaimAiConversation)
			aiWaiterDashboard.POST("/ai/conversations/:convId/release", server.RoleBasedAccessMiddleware("ai_waiter:reply"), server.ReleaseAiConversation)
			aiWaiterDashboard.POST("/ai/conversations/:convId/pause", server.RoleBasedAccessMiddleware("ai_waiter:reply"), server.ToggleAiPause)
			aiWaiterDashboard.POST("/ai/conversations/:convId/reply", server.RoleBasedAccessMiddleware("ai_waiter:reply"), server.PostAiReply)
			aiWaiterDashboard.POST("/ai/conversations/:convId/close", server.RoleBasedAccessMiddleware("ai_waiter:write"), server.CloseAiConversation)
			// Operator sandbox test chat: authenticated, no guest cookie side
			// effects, isolated from Live Monitor / guest quota.
			aiWaiterDashboard.POST("/ai/test-chat/session", server.RoleBasedAccessMiddleware("ai_waiter:write"), server.CreateAIWaiterTestSession)
			aiWaiterDashboard.POST("/ai/test-chat", server.RoleBasedAccessMiddleware("ai_waiter:write"), server.HandleAIWaiterTestChat)
		}

		// Director Console routes (owner-only; staff denied by permission + handler checks).
		// A suspended or closed business is refused at the route level (403
		// business_suspended / business_closed) even if RBAC is misconfigured.
		directorConsole := protectedRoutes.Group("/businesses/:id", server.RequireOperationalBusiness())
		{
			directorConsole.POST("/ai/director/ask", server.RoleBasedAccessMiddleware("director:write"), server.AskDirector)
			directorConsole.POST("/ai/director/ask/stream", server.RoleBasedAccessMiddleware("director:write"), server.AskDirectorStream)
			directorConsole.GET("/ai/director/threads", server.RoleBasedAccessMiddleware("director:read"), server.ListDirectorThreads)
			directorConsole.GET("/ai/director/threads/:threadId/messages", server.RoleBasedAccessMiddleware("director:read"), server.GetDirectorThreadMessages)
			directorConsole.PATCH("/ai/director/threads/:threadId", server.RoleBasedAccessMiddleware("director:write"), server.PatchDirectorThread)
			directorConsole.PATCH("/ai/director/threads/:threadId/archive", server.RoleBasedAccessMiddleware("director:write"), server.ArchiveDirectorThread)
			directorConsole.POST("/ai/director/threads/:threadId/restore", server.RoleBasedAccessMiddleware("director:write"), server.RestoreDirectorThread)
			directorConsole.DELETE("/ai/director/threads/:threadId", server.RoleBasedAccessMiddleware("director:write"), server.DeleteDirectorThread)
			directorConsole.POST("/ai/director/threads/:threadId/pin", server.RoleBasedAccessMiddleware("director:write"), server.PinDirectorThread)
			directorConsole.POST("/ai/director/threads/:threadId/unpin", server.RoleBasedAccessMiddleware("director:write"), server.UnpinDirectorThread)
			directorConsole.GET("/ai/director/threads/:threadId/export", server.RoleBasedAccessMiddleware("director:read"), server.ExportDirectorThread)
			directorConsole.POST("/ai/director/messages/:messageId/feedback", server.RoleBasedAccessMiddleware("director:write"), server.SubmitDirectorFeedback)
			directorConsole.GET("/director-console/proactive-insights", server.RoleBasedAccessMiddleware("director:read"), server.GetDirectorProactiveInsights)
			directorConsole.GET("/director-console/briefing", server.RoleBasedAccessMiddleware("director:read"), server.GetDirectorBriefing)
			// Pillar 2 write apply/undo — human-initiated. Two stacked middlewares
			// guarantee director:write AND menu:write (AND semantics regardless of
			// RequirePermissions internals); the service re-validates server-side.
			directorConsole.POST("/ai/director/actions/apply", server.RoleBasedAccessMiddleware("director:write"), server.RoleBasedAccessMiddleware("menu:write"), server.ApplyDirectorAction)
			directorConsole.POST("/ai/director/actions/undo", server.RoleBasedAccessMiddleware("director:write"), server.RoleBasedAccessMiddleware("menu:write"), server.UndoDirectorAction)
			// Applied-actions history: newest N committed actions with server-truth
			// undo eligibility, so inspect/undo survive a refresh (read-only).
			directorConsole.GET("/ai/director/actions/applied", server.RoleBasedAccessMiddleware("director:read"), server.ListAppliedDirectorActions)
			// Non-AI propose entry point (menu-engineering matrix "Propose new price"):
			// stages a single-item price-change proposal onto the SAME governed
			// apply/undo rail. Same director:write AND menu:write gate as apply/undo.
			directorConsole.POST("/ai/director/actions/propose-price-change", server.RoleBasedAccessMiddleware("director:write"), server.RoleBasedAccessMiddleware("menu:write"), server.ProposeDirectorPriceChange)
		}

		opsAssistant := protectedRoutes.Group("/businesses/:id", server.RequireOperationalBusiness())
		{
			opsAssistant.POST("/assistant/ask", server.RoleBasedAccessMiddleware("assistant:read"), server.AskOpsAssistant)
			opsAssistant.GET("/assistant/threads/:threadId/messages", server.RoleBasedAccessMiddleware("assistant:read"), server.GetOpsAssistantThreadMessages)
			opsAssistant.POST("/assistant/messages/:messageId/feedback", server.RoleBasedAccessMiddleware("assistant:write"), server.SubmitOpsAssistantFeedback)
		}

		// Batch translation routes (RBAC Protected - Manager+ only)
		protectedRoutes.POST("/businesses/:id/translate", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("menu:translate"), server.TranslateEntireMenu)
		protectedRoutes.GET("/translation-jobs/:jobId/status", server.GetTranslationStatus) // All staff can check status

		// Business Plugin Management Routes (RBAC Protected, operational business required)
		pluginHandlers := handlers.NewPluginHandlers(pluginService, reportScheduler)
		protectedRoutes.GET("/plugins", pluginHandlers.GetAllPlugins)                                                                                                            // All staff can view available plugins (no business scope)
		protectedRoutes.GET("/businesses/:id/plugins", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("plugins:read"), pluginHandlers.GetBusinessPlugins) // Manager-only
		protectedRoutes.GET("/businesses/:id/plugins/:plugin_id/config", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("plugins:config"), pluginHandlers.GetBusinessPluginConfig)
		protectedRoutes.POST("/businesses/:id/plugins/:plugin_id/enable", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("plugins:write"), pluginHandlers.EnableBusinessPlugin)
		protectedRoutes.POST("/businesses/:id/plugins/:plugin_id/disable", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("plugins:write"), pluginHandlers.DisableBusinessPlugin)
		protectedRoutes.PUT("/businesses/:id/plugins/:plugin_id/config", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("plugins:config"), pluginHandlers.UpdateBusinessPluginConfig)
		protectedRoutes.POST("/businesses/:id/plugins/:plugin_id/test", middleware.BusinessRateLimitWithBurst(10, 3), server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("plugins:config"), pluginHandlers.TestPluginConnection)

		// Mercado Pago Point terminals + in-store charge (Orders API)
		protectedRoutes.GET("/businesses/:id/mercadopago/terminals", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("plugins:payments"), pluginHandlers.ListMercadoPagoTerminals)
		protectedRoutes.PATCH("/businesses/:id/mercadopago/terminals/:terminal_id/mode", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("plugins:payments"), pluginHandlers.SetMercadoPagoTerminalMode)
		protectedRoutes.POST("/businesses/:id/bills/:bill_id/mercadopago/point/charge", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("plugins:payments"), pluginHandlers.ChargeMercadoPagoPoint)
		protectedRoutes.POST("/businesses/:id/bills/:bill_id/mercadopago/qr/charge", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("plugins:payments"), pluginHandlers.ChargeMercadoPagoQR)
		protectedRoutes.GET("/businesses/:id/mercadopago/orders/:order_id", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("plugins:payments"), pluginHandlers.GetMercadoPagoOrder)
		protectedRoutes.POST("/businesses/:id/mercadopago/orders/:order_id/cancel", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("plugins:payments"), pluginHandlers.CancelMercadoPagoOrder)

		// Mercado Pago OAuth connect (platform client credentials from env)
		mpOAuthClient, mpOAuthErr := mercadopago.NewOAuthClientFromEnv()
		if mpOAuthErr != nil {
			logger.Logger.Warnf("Mercado Pago OAuth not configured: %v", mpOAuthErr)
		}
		mpOAuthHandlers := handlers.NewMercadoPagoOAuthHandlers(pluginService, mpOAuthClient)
		protectedRoutes.POST("/businesses/:id/plugins/mercadopago/oauth/start", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("plugins:config"), mpOAuthHandlers.HandleOAuthStart)
		// Static OAuth redirect URI (public): /api/v1/mercadopago/oauth/callback
		mpOAuthPublic := r.Group("/api/v1/mercadopago")
		mpOAuthPublic.GET("/oauth/callback", middleware.PaymentRateLimit(), mpOAuthHandlers.HandleOAuthCallback)

		// Stripe Connect OAuth (Standard accounts; platform client from env)
		stripeOAuthClient, stripeOAuthErr := stripeplugin.NewOAuthClientFromEnv()
		if stripeOAuthErr != nil {
			logger.Logger.Warnf("Stripe Connect OAuth not configured: %v", stripeOAuthErr)
		}
		stripeOAuthHandlers := handlers.NewStripeOAuthHandlers(pluginService, stripeOAuthClient)
		protectedRoutes.POST("/businesses/:id/plugins/stripe/oauth/start", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("plugins:config"), stripeOAuthHandlers.HandleOAuthStart)
		// Static OAuth redirect URI (public): /api/v1/stripe/oauth/callback
		stripeOAuthPublic := r.Group("/api/v1/stripe")
		stripeOAuthPublic.GET("/oauth/callback", middleware.PaymentRateLimit(), stripeOAuthHandlers.HandleOAuthCallback)

		// Guest Payment Plugin Routes (public - for bill payments)
		publicRoutes.GET("/businesses/:business_id/payment-plugins", guestCatalogReadLimiter, pluginHandlers.GetBusinessPaymentPlugins)
		publicRoutes.POST("/guest/bill/:bill_token/plugin-payment", middleware.PaymentRateLimit(), pluginHandlers.CreatePluginPayment)
		publicRoutes.GET("/guest/bill/:bill_token/plugin-payment/:payment_id/status", middleware.PaymentRateLimit(), pluginHandlers.GetPluginPaymentStatus)

		// Telegram Plugin Specific Routes
		telegramHandlers := handlers.NewTelegramPluginHandlers(pluginService, telegramPluginInstance)
		protectedRoutes.GET("/businesses/:id/plugins/telegram/status", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("plugins:read"), telegramHandlers.GetConnectionStatus)
		protectedRoutes.POST("/businesses/:id/plugins/telegram/test", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware(string(server.PermPluginsWrite)), telegramHandlers.SendTestNotification)
		protectedRoutes.POST("/businesses/:id/plugins/telegram/generate-token", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware(string(server.PermPluginsWrite)), telegramHandlers.GenerateConnectionToken)
		protectedRoutes.POST("/businesses/:id/plugins/telegram/revoke-token", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware(string(server.PermPluginsWrite)), telegramHandlers.RevokeConnectionToken)
		protectedRoutes.POST("/businesses/:id/plugins/telegram/disconnect", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware(string(server.PermPluginsWrite)), telegramHandlers.DisconnectTelegram)

		// CRM Routes (protected - require authentication)
		crmService := crm.NewService(database.GetDB())
		crmHandler := crm.NewHandler(crmService)

		// CRM status and toggle (RBAC Protected - Manager+ only; operational business required)
		protectedRoutes.GET("/businesses/:id/crm/status", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("crm:read"), crmHandler.GetCRMStatus)
		protectedRoutes.POST("/businesses/:id/crm/toggle", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("settings:write"), crmHandler.ToggleCRM)

		// Business CRM Management (RBAC Protected - Manager+ can view/manage customers, operational business required)
		protectedRoutes.GET("/businesses/:id/crm/customers", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("crm:read"), crmHandler.GetBusinessCustomers)
		protectedRoutes.GET("/businesses/:id/crm/customers/:customerBusinessId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("crm:read"), crmHandler.GetCustomerDetails)
		protectedRoutes.PUT("/businesses/:id/crm/customers/:customerBusinessId/notes", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("crm:write"), crmHandler.UpdateCustomerNotes)
		protectedRoutes.PUT("/businesses/:id/crm/customers/:customerBusinessId/allergies", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("crm:write"), crmHandler.UpdateCustomerAllergies)
		protectedRoutes.PUT("/businesses/:id/crm/customers/:customerBusinessId/loyalty-points", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("crm:write"), crmHandler.AdjustCustomerLoyaltyPoints)
		protectedRoutes.PUT("/businesses/:id/crm/customers/:customerBusinessId/tags", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("crm:write"), crmHandler.UpdateCustomerTags)
		// Soft-unlink from this business only (not global hard delete) — L5-9.
		protectedRoutes.DELETE("/businesses/:id/crm/customers/:customerBusinessId", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("crm:write"), crmHandler.UnlinkCustomer)
		protectedRoutes.GET("/businesses/:id/crm/export", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("crm:export"), crmHandler.ExportCustomers)
		protectedRoutes.GET("/businesses/:id/crm/segments", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("crm:read"), crmHandler.GetSegments)
		protectedRoutes.POST("/businesses/:id/crm/customers", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("crm:write"), crmHandler.AddCustomer)

		// Loyalty program editor (Manager+ for writes; reads use crm:read so the
		// CRM tab can render the editor in read-only mode for support staff).
		protectedRoutes.GET("/businesses/:id/crm/loyalty", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("crm:read"), crmHandler.GetLoyalty)
		protectedRoutes.PUT("/businesses/:id/crm/loyalty", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("crm:write"), crmHandler.PutLoyalty)
		protectedRoutes.POST("/businesses/:id/crm/loyalty/preview", server.RequireOperationalBusiness(), server.RoleBasedAccessMiddleware("crm:read"), crmHandler.PreviewLoyalty)

		// Web Push subscription management
		protectedRoutes.POST("/push-subscriptions", server.CreatePushSubscription)
		protectedRoutes.GET("/push-subscriptions", server.ListPushSubscriptions)
		protectedRoutes.DELETE("/push-subscriptions/:subscriptionId", server.DeletePushSubscription)
	}

	// Initialize plugin handlers (used by both admin and webhook routes)
	pluginHandlers := handlers.NewPluginHandlers(pluginService, reportScheduler)

	// Generic plugin payment reconciliation sweep: age-gated poll of pending
	// plugin trackers via each plugin's GetPaymentStatus, settling completed
	// captures and expiring failed ones so a lost webhook can't strand guest
	// money forever. Always on.
	// A slow provider sweep must not stack runs every minute.
	pluginReconciliationScheduler := cron.New(cron.WithChain(cron.Recover(cron.DefaultLogger), cron.SkipIfStillRunning(cron.DefaultLogger)))
	pluginReconciliationSchedulerStarted := false
	if _, err := pluginReconciliationScheduler.AddFunc("@every 1m", func() {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		pluginHandlers.ReconcilePendingPluginPayments(ctx, 50)
	}); err != nil {
		logger.Logger.Warnf("Failed to schedule plugin payment reconciliation: %v", err)
	} else {
		pluginReconciliationScheduler.Start()
		pluginReconciliationSchedulerStarted = true
		logger.Logger.Info("Plugin payment reconciliation scheduler started successfully")
	}

	// Mercado Pago OAuth token refresh: proactively renew access tokens that
	// expire within 30 days so merchant connections stay live. Gated on
	// platform client id (full OAuth client also needs secret). Logs counts only.
	mpTokenRefreshScheduler := cron.New(cron.WithChain(cron.Recover(cron.DefaultLogger)))
	mpTokenRefreshSchedulerStarted := false
	if strings.TrimSpace(os.Getenv("MERCADOPAGO_CLIENT_ID")) != "" {
		mpRefreshClient, mpRefreshErr := mercadopago.NewOAuthClientFromEnv()
		if mpRefreshErr != nil {
			logger.Logger.Warnf("Mercado Pago token refresh not scheduled: %v", mpRefreshErr)
		} else if _, err := mpTokenRefreshScheduler.AddFunc("@every 12h", func() {
			stats := handlers.RefreshMercadoPagoTokens(database.GetDBWrapper(), mpRefreshClient, time.Now().UTC())
			logger.Logger.Infof("MercadoPago token refresh: refreshed=%d skipped=%d failed=%d",
				stats.Refreshed, stats.Skipped, stats.Failed)
		}); err != nil {
			logger.Logger.Warnf("Failed to schedule Mercado Pago token refresh: %v", err)
		} else {
			mpTokenRefreshScheduler.Start()
			mpTokenRefreshSchedulerStarted = true
			logger.Logger.Info("Mercado Pago OAuth token refresh scheduler started successfully (@every 12h)")
		}
	}

	// Admin routes (require admin authentication). Rate-limit per-IP to cap
	// abuse on heavy endpoints like /stats even from authenticated admins
	// whose tokens leak.
	adminRateLimiter := middleware.AuthRateLimiter(60, 20)
	adminRoutes := r.Group("/api/v1/admin")
	adminRoutes.Use(
		middleware.RequireTrustedOriginForMutations(allowedOrigins),
		server.AuthenticationAdminMiddleware(),
		adminRateLimiter,
	)
	{
		runtimeControlHandler := runtimecontrol.NewHandler(runtimeControlService)
		adminRoutes.GET("/runtime-controls", runtimeControlHandler.List)
		adminRoutes.PUT("/runtime-controls/:key", runtimeControlHandler.Update)
		adminRoutes.POST("/runtime-controls/invite-batches", runtimeControlHandler.CreateInviteBatch)
		adminRoutes.GET("/runtime-controls/audit", runtimeControlHandler.Audit)

		// Admin Dashboard Statistics
		adminRoutes.GET("/stats", server.GetAdminStats)
		adminRoutes.GET("/system/health", server.GetAdminSystemHealth)
		adminRoutes.GET("/businesses", server.GetBusinessList)
		adminRoutes.GET("/users", server.GetUserList)

		// Admin Operations
		adminDemoHandler := handlers.NewAdminDemoHandler(adminDemoService)
		adminRoutes.GET("/demo", adminDemoHandler.Get)
		adminRoutes.POST("/demo/ensure", adminDemoHandler.Ensure)
		adminRoutes.POST("/demo/reset", adminDemoHandler.Reset)
		adminRoutes.POST("/demo/append-day", adminDemoHandler.AppendDay)
		adminRoutes.POST("/demo/verify", adminDemoHandler.Verify)

		// Ops-assistant escalations (newest first, paginated).
		adminRoutes.GET("/escalations", server.GetAdminEscalations)
		adminRoutes.GET("/escalations/:id", server.GetAdminEscalationDetail)
		adminRoutes.PATCH("/escalations/:id", server.PatchAdminEscalation)
		adminRoutes.GET("/ops-threads/:business_id/:thread_id/messages", server.GetAdminOpsThreadMessages)

		// Platform fiscal compliance oversight
		adminRoutes.GET("/fiscal/summary", server.GetAdminFiscalSummary)
		adminRoutes.GET("/fiscal/jobs", server.GetAdminFiscalJobs)
		adminRoutes.GET("/fiscal/receipts", server.GetAdminFiscalReceipts)
		adminRoutes.POST("/fiscal/jobs/:id/requeue", server.PostAdminRequeueFiscalJob)

		// Plugin Management Routes (admin only)
		adminRoutes.POST("/plugins", pluginHandlers.CreatePlugin)
		adminRoutes.GET("/plugins", pluginHandlers.GetAllPlugins)
		adminRoutes.GET("/plugins/:id", pluginHandlers.GetPlugin)
		adminRoutes.PUT("/plugins/:id", pluginHandlers.UpdatePlugin)
		adminRoutes.POST("/plugins/:id/toggle-active", pluginHandlers.TogglePluginActive)
		adminRoutes.POST("/plugins/:id/deactivate", pluginHandlers.DeactivatePlugin)

		// Failed webhook review (every provider's webhook_events rows).
		webhookReviewHandler := handlers.NewWebhookReviewHandler(db)
		adminRoutes.GET("/webhooks/failed", webhookReviewHandler.AdminGetAllFailedWebhooks)
		adminRoutes.POST("/webhooks/:id/acknowledge", webhookReviewHandler.AdminAcknowledgeFailedWebhook)

		// Email Broadcast Management (admin only)
		adminRoutes.POST("/emails/operational-update", server.SendOperationalUpdate)
		adminRoutes.POST("/emails/platform-update", server.SendPlatformUpdate)
		adminRoutes.GET("/emails/business-list", server.GetBusinessEmails)

		// Page Analytics Routes (admin only)
		adminPageAnalyticsService := analytics.NewPageAnalyticsService(db)
		adminPageAnalyticsHandler := handlers.NewPageAnalyticsHandler(adminPageAnalyticsService)
		adminRoutes.GET("/analytics/summary", adminPageAnalyticsHandler.GetAnalyticsSummary)
		adminRoutes.GET("/analytics/sessions", adminPageAnalyticsHandler.GetRecentSessions)

		// IMP-31: read side of the missing-translation telemetry sink.
		adminMissingTranslationHandler := handlers.NewMissingTranslationHandler()
		adminRoutes.GET("/analytics/missing-translations", adminMissingTranslationHandler.ListMissing)
		adminRoutes.PATCH("/analytics/missing-translations/:id/status", adminMissingTranslationHandler.UpdateStatus)

		// Error log routes
		adminErrorLogHandler := handlers.NewErrorLogHandler()
		adminRoutes.GET("/errors", adminErrorLogHandler.ListErrors)

		// Admin User Management
		adminUserHandler := handlers.NewAdminUserHandler(adminAuthService)
		adminRoutes.GET("/users/:id/detail", adminUserHandler.AdminGetUserDetail)
		adminRoutes.POST("/users/:id/close", adminUserHandler.AdminCloseAccount)
		adminRoutes.POST("/users/:id/reset-password", adminUserHandler.AdminResetPassword)

		// Admin Business Management
		adminBusinessHandler := handlers.NewAdminBusinessHandler(database.GetDB())
		adminRoutes.GET("/businesses/:id/detail", adminBusinessHandler.GetBusinessDetail)
		adminRoutes.POST("/businesses/:id/suspend", adminBusinessHandler.SuspendBusiness)
		adminRoutes.POST("/businesses/:id/reactivate", adminBusinessHandler.ReactivateBusiness)
	}

	// Payment Plugin Webhook Routes (public - accessible by payment providers)
	webhookRoutes := r.Group("/api/v1/webhooks")
	{
		registerEmailWebhookRoutes(webhookRoutes, emailProviderName)

		// Always register the route so a missing token/secret is a refused
		// update (503) rather than a silent 404 that looks like the path
		// does not exist.
		telegramUpdateProcessor := &server.TelegramUpdateProcessor{ReplySender: telegramPluginInstance}
		var telegramWebhookHandler *handlers.TelegramWebhookHandler
		if telegramWebhookEnabled {
			telegramWebhookHandler = handlers.NewTelegramWebhookHandler(*telegramWebhookSecret, telegramUpdateProcessor)
		} else {
			telegramWebhookHandler = handlers.NewUnavailableTelegramWebhookHandler()
		}
		webhookRoutes.POST("/telegram", telegramWebhookHandler.HandleTelegramWebhook)

		// PayPal webhooks
		webhookRoutes.POST("/paypal", pluginHandlers.HandlePayPalWebhook)
		webhookRoutes.GET("/paypal/return", middleware.PaymentRateLimit(), pluginHandlers.HandlePayPalReturn)
		webhookRoutes.GET("/paypal/cancel", middleware.PaymentRateLimit(), pluginHandlers.HandlePayPalCancel)

		// Stripe webhooks (for guest bill payments via plugin)
		webhookRoutes.POST("/stripe", pluginHandlers.HandleStripeWebhook)

		// MercadoPago webhooks
		webhookRoutes.POST("/mercadopago", pluginHandlers.HandleMercadoPagoWebhook)
		webhookRoutes.GET("/mercadopago/return", middleware.PaymentRateLimit(), pluginHandlers.HandleMercadoPagoReturn)
	}

	// Background GC for redeemed wallet challenges. Correctness does not
	// depend on the cadence: Verify and Redeem enforce the nonce's own expiry,
	// and Redeem purges inline when the redeemed set reaches its cap. This
	// just keeps the set small between sign-ins.
	logger.SafeGo(func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			server.ChallengeStore.Cleanup()
		}
	})

	// Background release for AI takeover claims idle past their TTL. The
	// conversation list poll still sweeps its own venue lazily, but with this
	// ticker it normally finds nothing to write, and venues nobody is watching
	// still hand abandoned chats back to the AI.
	logger.SafeGo(func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if _, err := server.ReleaseStaleAiClaims(context.Background(), 0, time.Now()); err != nil {
				logger.Logger.Warnf("ai stale claim sweep failed: %v", err)
			}
		}
	})

	// Background release for abandoned guest split holds. Correctness is still
	// enforced on every read/write path; this keeps other phones updated even
	// when the holder walks away and nobody actively refreshes the bill.
	logger.SafeGo(func() {
		ticker := time.NewTicker(1 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			_, states, err := database.ReleaseExpiredBillSplitSharesWithStates(time.Now().UTC())
			if err != nil {
				logger.Logger.Warnf("split hold expiry sweep failed: %v", err)
				continue
			}
			for _, state := range states {
				handlers.PublishGuestSplitState(state)
			}
		}
	})

	listenPort := strings.TrimSpace(os.Getenv("PORT"))
	if listenPort == "" {
		listenPort = "8080"
	}
	srv := &http.Server{
		Addr:    ":" + listenPort,
		Handler: r,
		// ReadHeaderTimeout bounds the time allowed to read request headers,
		// closing the Slowloris hole (gosec G112) as defense-in-depth behind
		// Caddy. IdleTimeout reaps idle keep-alive connections. We deliberately
		// DO NOT set ReadTimeout/WriteTimeout: this server streams SSE
		// (text/event-stream, long-lived) and a whole-request read/write
		// deadline would tear those connections down — the same reason the
		// Caddy read_timeout has to be tuned for SSE (see SSE reliability notes).
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	// Initializing the server in a goroutine so that
	// it won't block the graceful shutdown handling below
	go func() {
		logger.Logger.Infof("Starting server on port: %s", listenPort)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Logger.Fatalf("listen: %s", err)
		}
	}()

	// Wait for interrupt signal to gracefully shut down the server with
	// a timeout of 5 seconds.
	quit := make(chan os.Signal, 1)
	// kill (no param) default send syscall.SIGTERM
	// kill -2 is syscall.SIGINT
	// kill -9 is syscall.SIGKILL but can't be caught, so don't need to add it
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Logger.Info("Shutting down server...")

	// Stop report scheduler
	if err := reportScheduler.Stop(); err != nil {
		logger.Logger.Warnf("Error stopping report scheduler: %v", err)
	}

	// Stop lifecycle scheduler
	if lifecycleScheduler != nil {
		if err := lifecycleScheduler.Stop(); err != nil {
			logger.Logger.Warnf("Error stopping lifecycle scheduler: %v", err)
		}
	}

	// Stop milestone scheduler
	if err := milestoneScheduler.Stop(); err != nil {
		logger.Logger.Warnf("Error stopping milestone scheduler: %v", err)
	}
	activationScheduler.Stop()

	// Stop guest feedback scheduler
	if err := guestFeedbackScheduler.Stop(); err != nil {
		logger.Logger.Warnf("Error stopping guest feedback scheduler: %v", err)
	}

	// Stop inventory alert scheduler
	inventoryAlertScheduler.Stop()

	// Stop recurring entries scheduler
	if recurringEntriesScheduler != nil {
		if err := recurringEntriesScheduler.Stop(); err != nil {
			logger.Logger.Warnf("Error stopping recurring entries scheduler: %v", err)
		}
	}

	// Stop director digest scheduler
	directorDigestScheduler.Stop()

	if adminDemoSchedulerStarted {
		ctx := adminDemoScheduler.Stop()
		<-ctx.Done()
	}

	// Stop shift reminder scheduler
	shiftReminderScheduler.Stop()

	// Stop verified token cache sweeper (P6.8).
	stopVerifiedTokenCacheSweeper()

	// Stop AI transcript retention janitor
	aiRetentionCancel()
	spaceScanWorkerCancel()
	spaceScanWorker.Stop()
	fiscalWorkerCancel()

	// Stop the menu extraction worker; it waits for its in-flight pass
	// before returning.
	if menuExtractionWorker != nil {
		menuExtractionWorker.Stop()
	}

	// Stop reservation scheduler
	reservationScheduler.Stop()

	// Stop IMP-02 print workers.
	if printRetryWorker != nil {
		printRetryWorker.Stop()
	}
	if printOrphanSweep != nil {
		printOrphanSweep.Stop()
	}

	// Stop IMP-05 stuck-bill watchdog.
	stuckBillWatchdog.Stop()

	// Stop Task 16 bill lifecycle sweeper.
	billLifecycleSweeper.Stop()

	// Stop delivery payment-expiry job.
	deliveryPaymentExpiry.Stop()

	// Stop CRM settlement reconciliation scheduler
	if crmSettlementSchedulerStarted {
		ctx := crmSettlementScheduler.Stop()
		<-ctx.Done()
	}
	if pluginReconciliationSchedulerStarted {
		ctx := pluginReconciliationScheduler.Stop()
		<-ctx.Done()
	}
	if mpTokenRefreshSchedulerStarted {
		ctx := mpTokenRefreshScheduler.Stop()
		<-ctx.Done()
	}

	// Stop WhatsApp manager: exit the sweep goroutine and disconnect all
	// active clients so shutdown leaves no leaked goroutine or open socket (B4).
	if whatsAppManager != nil {
		whatsAppManager.Stop()
	}

	// Payment monitor removed - using polling instead
	if telegramWorkerStarted {
		close(telegramWorkerStop)
	}

	// The context is used to inform the server it has 30 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Logger.Warnf("Server shutdown timed out after 30s: %v", err)
	} else {
		logger.Logger.Info("HTTP server stopped gracefully")
	}

	// Stop the DB-backed workers (crypto refunds, reorg sweep, account
	// erasure) and wait up to 10s for their current pass, so none of them
	// queries a closed pool.
	dbWorkers.Shutdown(dbWorkerShutdownTimeout)

	// Close database connections
	sqlDB, err := database.GetDB().DB()
	if err == nil {
		if err := sqlDB.Close(); err != nil {
			logger.Logger.Warnf("Error closing database: %v", err)
		} else {
			logger.Logger.Info("Database connections closed")
		}
	}

	sentryClient.Flush(2 * time.Second)
	logger.Logger.Info("Server exiting")
}

// parseMetricsTokens returns the de-duplicated list of bearer tokens that are
// currently accepted on /metrics. METRICS_TOKENS (comma-separated) enables
// graceful rotation: add the new token, wait for scrapers to roll, then drop
// the old. METRICS_TOKEN remains as a single-token shortcut for simple setups.
// An empty result means /metrics is open (no auth enforced) — the caller is
// responsible for deciding whether that's acceptable.
//
// Placeholder values (e.g. the .env.example value "replace_with_metrics_bearer_token")
// are silently filtered so a copy-paste deploy does not register /metrics behind
// a publicly-known bearer token. (CFG-002)
func parseMetricsTokens(multi, single string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 2)
	for _, raw := range strings.Split(multi, ",") {
		t := strings.TrimSpace(raw)
		if t == "" || isPlaceholderMetricsToken(t) {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	if single = strings.TrimSpace(single); single != "" && !isPlaceholderMetricsToken(single) {
		if _, ok := seen[single]; !ok {
			out = append(out, single)
		}
	}
	return out
}

// isPlaceholderMetricsToken returns true when the token is a known
// example/placeholder value that appears in published .env.example files
// and must never be treated as a real configuration value. (CFG-002)
func isPlaceholderMetricsToken(token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	if token == "replace_with_metrics_bearer_token" {
		return true
	}
	lower := strings.ToLower(token)
	return strings.HasPrefix(lower, "changeme") || strings.HasPrefix(lower, "replace_with")
}

// resolveAllowedOrigins merges env-provided origins with the built-in defaults.
// The instance's own PUBLIC_URL origin (exactly that origin, no
// www./apex twin) is always included when configured. Localhost
// defaults are only added outside production — otherwise a malicious local service on port 3000/3001 can make
// credentialed cross-origin requests to the API (CORS allows credentials; see
// internal/middleware/security.go). Duplicates are deduped on the normalized
// value. Whitespace-only entries in the env var are dropped. Env entries are
// validated; invalid ones are dropped. Defaults are not validated. A single
// trailing slash is trimmed before compare and append.
func resolveAllowedOrigins(env string) []string {
	defaults := config.PublicOrigins()
	production := utils.IsProduction()
	if !production {
		// B12: localhost origins are only safe in dev. Warn loudly so that a
		// deployed server that is accidentally NOT in production mode (missing
		// --production / ENV=production) is noticed before it ships a CORS
		// allow-list that trusts localhost.
		logger.Logger.Warn("CORS: non-production mode — adding localhost origins to the allow-list (http://localhost:3000, http://localhost:3001). If this is a deployed server, your production flag is misconfigured.")
		defaults = append(defaults,
			"http://localhost:3000",
			"http://localhost:3001",
		)
	}
	seen := make(map[string]struct{}, len(defaults))
	out := make([]string, 0, len(defaults))
	for _, o := range defaults {
		seen[o] = struct{}{}
		out = append(out, o)
	}
	for _, raw := range strings.Split(env, ",") {
		o := strings.TrimSpace(raw)
		if o == "" {
			continue
		}
		if err := validateAllowedOrigin(o, production); err != nil {
			if logger.Logger != nil {
				logger.Logger.Warnf("CORS: dropping invalid ALLOWED_ORIGINS entry %q: %s", o, err.Error())
			}
			continue
		}
		o = strings.TrimSuffix(o, "/")
		if _, dup := seen[o]; dup {
			continue
		}
		seen[o] = struct{}{}
		out = append(out, o)
	}
	return out
}

// validateAllowedOrigin rejects origins that must not appear in a credentialed
// CORS allow-list. A single trailing slash is ignored so https://ok.example/
// and https://ok.example are the same origin. production requires https.
func validateAllowedOrigin(o string, production bool) error {
	o = strings.TrimSuffix(strings.TrimSpace(o), "/")
	if strings.EqualFold(o, "null") {
		return fmt.Errorf("literal null is not allowed")
	}
	u, err := url.Parse(o)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("scheme %q is not http or https", u.Scheme)
	}
	if production && scheme != "https" {
		return fmt.Errorf("production origins must use https")
	}
	if u.Host == "" {
		return fmt.Errorf("empty host")
	}
	if u.User != nil {
		return fmt.Errorf("userinfo is not allowed")
	}
	if u.ForceQuery || u.RawQuery != "" {
		return fmt.Errorf("query is not allowed")
	}
	if u.Fragment != "" || u.RawFragment != "" {
		return fmt.Errorf("fragment is not allowed")
	}
	if u.Path != "" {
		return fmt.Errorf("path %q is not allowed", u.Path)
	}
	return nil
}
