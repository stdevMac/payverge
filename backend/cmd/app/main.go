package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"payverge/internal/blockchain"
	"payverge/internal/faucet"
	"payverge/internal/handlers"
	"payverge/internal/health"
	"payverge/internal/logger"
	"payverge/internal/middleware"
	"payverge/internal/s3"

	"payverge/internal/database"
	"payverge/internal/emails"
	"payverge/internal/metrics"
	"payverge/internal/notifications"
	"payverge/internal/server"
	"payverge/internal/services"
	"payverge/internal/telegram"
	"payverge/internal/websocket"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	// Get flags and initialize the database
	var (
		databasePath           = flag.String("database-path", "./data/app.db", "SQLite database file path")
		production             = flag.Bool("production", false, "Production mode")
		faucetPrivateKey       = flag.String("faucet-private-key", "", "Private key for Faucet Address")
		rpcUrl                 = flag.String("rpc-url", "", "RPC URL for the Ethereum node")
		chainId                = flag.Int64("chain-id", 1, "Chain ID for the Ethereum network")
		usdcContractAddress    = flag.String("usdc-contract", "", "USDC token contract address")
		payvergeContractAddr   = flag.String("payverge-contract", "", "Payverge smart contract address")
		telegramToken          = flag.String("telegram-token", "", "Telegram token")
		s3Bucket               = flag.String("s3-bucket", "", "AWS S3 bucket name")
		awsAccessKey           = flag.String("aws-access-key", "", "AWS Access Key ID")
		awsSecretKey           = flag.String("aws-secret-key", "", "AWS Secret Access Key")
		awsRegion              = flag.String("aws-region", "us-east-1", "AWS Region")
		s3EndpointURL          = flag.String("s3-endpoint", "", "S3 Endpoint URL (optional)")
		s3ProtectedBucket      = flag.String("s3-protected-bucket", "", "AWS S3 bucket name")
		awsProtectedAccessKey  = flag.String("aws-protected-access-key", "", "AWS Access Key ID")
		awsProtectedSecretKey  = flag.String("aws-protected-secret-key", "", "AWS Secret Access Key")
		s3ProtectedEndpointURL = flag.String("s3-protected-endpoint", "", "S3 Endpoint URL (optional)")
		postmarkServerToken    = flag.String("postmark-server-token", "", "Postmark server token")
		fromEmail              = flag.String("from-email", "", "From email")
		fromEmailNews          = flag.String("from-email-news", "", "From email news")
		fromEmailUpdates       = flag.String("from-email-updates", "", "From email updates")
		googleTranslateAPIKey  = flag.String("google-translate-api-key", "", "Google Translate API Key")
	)
	flag.Parse()
	if *production {
		// Set Gin to production mode
		gin.SetMode(gin.ReleaseMode)
	}

	if *faucetPrivateKey == "" || *rpcUrl == "" {
		log.Fatal("Faucet private key and RPC URL are required")
	} else {
		// Initialize faucet service
		faucet.InitFaucet(*rpcUrl, *faucetPrivateKey, *chainId)

		// Initialize USDC transfer functionality
		faucet.InitUSDCTransfer(*usdcContractAddress, *rpcUrl, *faucetPrivateKey, *chainId)
	}

	server.SetChainId(*chainId)

	// Initialize the email server
	emailServer := emails.NewEmailServer(
		*fromEmail,
		*fromEmailNews,
		*fromEmailUpdates,
		*postmarkServerToken,
	)

	// Initialize notification dispatchers
	emailDispatcher := emails.NewEmailServerDispatcher(emailServer)

	// Initialize the Telegram bot
	bot := server.InitializeTelegramBot(*telegramToken)
	telegramDispatcher := telegram.NewTelegramNotificationDispatcher(bot)

	// Initialize the notification manager
	notificationManager := notifications.NewNotificationManager(emailDispatcher, telegramDispatcher)

	// Make notification manager available to the server package
	server.SetNotificationManager(notificationManager)

	// Create images directory if it doesn't exist
	if err := os.MkdirAll("images", 0755); err != nil {
		log.Fatalf("Error creating images directory: %v", err)
	}

	// Initialize the database
	config := database.NewConfig(*databasePath)
	database.InitDB(config)

	// Initialize database wrapper and blockchain service
	db := database.GetDBWrapper()
	blockchainService, err := blockchain.NewBlockchainService(*rpcUrl, *payvergeContractAddr, *faucetPrivateKey)
	if err != nil {
		log.Fatalf("Failed to initialize blockchain service: %v", err)
	}

	// Initialize exchange rate and translation services
	exchangeRateService := services.NewExchangeRateService(db)
	translationService := services.NewTranslationService(db, *googleTranslateAPIKey)
	log.Println("Translation service initialized")
	log.Println("Api key Google: ", *googleTranslateAPIKey)
	
	// Make translation service available to the server package
	server.SetTranslationService(translationService)

	// Initialize default currencies and languages
	if err := exchangeRateService.InitializeDefaultCurrencies(); err != nil {
		log.Printf("Failed to initialize default currencies: %v", err)
	}
	if err := translationService.InitializeDefaultLanguages(); err != nil {
		log.Printf("Failed to initialize default languages: %v", err)
	}

	// Start periodic exchange rate fetching
	exchangeRateService.StartPeriodicFetch()

	// Initialize WebSocket Hub
	wsHub := websocket.NewHub()
	go wsHub.Run()

	// Initialize Payment Monitor
	paymentMonitor := websocket.NewPaymentMonitor(wsHub, db, blockchainService)
	if err := paymentMonitor.Start(); err != nil {
		log.Printf("Warning: Failed to start payment monitor: %v", err)
	}

	// Initialize payment handler
	paymentHandler := handlers.NewPaymentHandler(db, blockchainService)

	// Initialize S3
	if err := s3.InitS3(*s3Bucket, *awsAccessKey, *awsSecretKey, *awsRegion, *s3EndpointURL); err != nil {
		log.Fatalf("Failed to initialize S3: %v", err)
	}
	// Initialize S3 Protected
	if err := s3.InitS3Protected(*s3ProtectedBucket, *awsProtectedAccessKey, *awsProtectedSecretKey, *awsRegion, *s3ProtectedEndpointURL); err != nil {
		log.Fatalf("Failed to initialize S3: %v", err)
	}
	// Initialize structured logging
	logger.InitLogger()

	// Initialize the metrics
	metrics.Init()

	// Initialize PostHog with environment variable
	posthogAPIKey := os.Getenv("POSTHOG_API_KEY")
	posthogHost := os.Getenv("POSTHOG_HOST")
	if posthogAPIKey != "" && posthogHost != "" {
		metrics.InitPostHogClient(posthogAPIKey, posthogHost)
		defer metrics.ClosePostHogClient()
	}

	// Create rate limiter (60 requests per minute)
	rateLimiter := middleware.NewSimpleRateLimiter(60)

	r := gin.Default()
	r.Use(gin.Recovery())
	r.Use(middleware.CORS())
	r.Use(middleware.SecurityHeaders())
	r.Use(middleware.InputValidation())
	r.Use(middleware.JSONSizeLimit(10 << 20)) // 10MB limit
	r.Use(rateLimiter.RateLimit())
	r.Use(server.PrometheusMiddleware())

	// Configure Gin to handle larger file uploads
	r.MaxMultipartMemory = 100 << 20 // 100 MiB

	// Metrics endpoint
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Auth routes
	auth := r.Group("/api/v1/auth")
	{
		auth.POST("/challenge", server.GenerateChallenge)
		auth.GET("/session", server.GetSession)
		auth.POST("/signin", server.SignIn)
		auth.POST("/signout", server.SignOut)
	}

	// Public routes (do not require authentication)
	publicRoutes := r.Group("/api/v1/")
	{
		// Health check endpoints
		publicRoutes.GET("/health", health.Handler(database.GetDB(), "1.0.0"))
		publicRoutes.GET("/health/ready", health.ReadinessHandler(database.GetDB()))
		publicRoutes.GET("/health/live", health.LivenessHandler())

		// Telegram webhook
		publicRoutes.POST("/telegram/webhook", server.WebhookHandler)

		// Serve static files from a directory
		publicRoutes.POST("/subscribe", server.Subscribe)
		publicRoutes.GET("/unsubscribe", server.UnSubscribe)

		// General Routes
		// TODO: implement

		// Error logging endpoint
		publicRoutes.POST("/logs/error", server.LogError)

		// Payverge public routes (for guests)
		publicRoutes.GET("/table/:code", server.GetTableByCode)

		// Phase 3: Public guest table API endpoints
		publicRoutes.GET("/guest/table/:code", server.GetTableByCodePublic)
		publicRoutes.GET("/guest/table/:code/bill", server.GetOpenBillByTableCode)
		publicRoutes.POST("/guest/table/:code/bill", server.CreateBillByTableCode)
		publicRoutes.POST("/guest/table/:code/order", server.CreateGuestOrder)
		publicRoutes.GET("/guest/table/:code/business", server.GetBusinessByTableCode)
		publicRoutes.GET("/guest/table/:code/menu", server.GetMenuByTableCode)
		publicRoutes.GET("/guest/table/:code/status", server.GetTableStatusByCode)
		publicRoutes.GET("/guest/bill/:bill_number", server.GetBillByNumberPublic)

		// Phase 4: Payment processing endpoints
		publicRoutes.POST("/payments/process", paymentHandler.CreateBillPayment)
		publicRoutes.GET("/payments/history/:bill_id", paymentHandler.GetBillPayments)
		publicRoutes.GET("/payments/:payment_id", paymentHandler.GetPaymentDetails)
		publicRoutes.GET("/payments/total/:bill_id", paymentHandler.GetBillTotalPaid)

		// Phase 5: Bill Splitting routes (public for guests)
		splittingHandler := handlers.NewSplittingHandler(database.GetDBWrapper(), blockchainService)
		publicRoutes.GET("/bills/:bill_id/split/options", splittingHandler.GetBillSplitOptions)
		publicRoutes.POST("/bills/:bill_id/split/equal", splittingHandler.CalculateEqualSplit)
		publicRoutes.POST("/bills/:bill_id/split/custom", splittingHandler.CalculateCustomSplit)
		publicRoutes.POST("/bills/:bill_id/split/items", splittingHandler.CalculateItemSplit)
		publicRoutes.POST("/bills/:bill_id/split/validate", splittingHandler.ValidateSplit)

		// Blockchain integration routes for split payments (public for guests)
		publicRoutes.GET("/bills/:bill_id/participants", splittingHandler.GetBillParticipants)
		publicRoutes.GET("/bills/:bill_id/participants/:address", splittingHandler.GetParticipantInfo)
		publicRoutes.GET("/bills/:bill_id/summary", splittingHandler.GetBillSummaryWithParticipants)
		publicRoutes.POST("/bills/:bill_id/split/execute", splittingHandler.ExecuteSplitPayment)
		publicRoutes.POST("/payments/webhook", paymentHandler.WebhookPaymentConfirmation)

		// WebSocket endpoint for real-time updates
		publicRoutes.GET("/ws", gin.WrapH(http.HandlerFunc(wsHub.ServeWS)))

		// Alternative Payment routes (public for guests)
		publicRoutes.POST("/bills/:bill_id/request-alternative-payment", paymentHandler.RequestAlternativePayment)
		publicRoutes.GET("/bills/:bill_id/alternative-payments", paymentHandler.GetBillAlternativePayments)
		publicRoutes.GET("/bills/:bill_id/payment-breakdown", paymentHandler.GetBillPaymentBreakdown)

		// Staff Authentication routes (public - no auth required)
		publicRoutes.POST("/staff/accept-invitation", server.AcceptInvitation)
		publicRoutes.POST("/staff/request-login-code", server.RequestLoginCode)
		publicRoutes.POST("/staff/verify-login-code", server.VerifyLoginCode)

		// Referral system routes (public - no auth required for checking availability)
		publicRoutes.POST("/referrals/check-code", server.CheckReferralCodeAvailability)
		publicRoutes.GET("/referrals/stats", server.GetReferralStats)
		publicRoutes.GET("/referrals/referrer/code/:referral_code", server.GetReferrerByCode)

		// Multi-currency routes (public - no auth required for exchange rates)
		currencyHandler := handlers.NewCurrencyHandler(database.GetDBWrapper(), exchangeRateService, translationService)
		publicRoutes.GET("/currencies", currencyHandler.GetSupportedCurrencies)
		publicRoutes.GET("/languages", currencyHandler.GetSupportedLanguages)
		publicRoutes.GET("/exchange-rate", currencyHandler.GetExchangeRate)
		publicRoutes.GET("/convert", currencyHandler.ConvertAmount)
	}

	// Protected routes (require authentication)
	protectedRoutes := r.Group("/api/v1/inside")
	protectedRoutes.Use(server.AuthenticationMiddleware())
	{
		// Faucet endpoint
		protectedRoutes.POST("/faucet", server.CheckAndTopUp)
		protectedRoutes.GET("/faucet/check/:address", server.CheckFaucetAvailability)

		// File upload endpoints
		protectedRoutes.POST("/upload", server.UploadFile) // S3 upload - commented for now
		// protectedRoutes.POST("/upload_protected", server.UploadFileProtected) // S3 upload - commented for now
		// protectedRoutes.POST("/upload", server.UploadFileLocal)

		// User routes
		protectedRoutes.GET("/get_user/:address", server.GetUser)
		protectedRoutes.PUT("/update_notification_preference", server.UpdateNotificationPreference)
		protectedRoutes.PUT("/update_user", server.UpdateUser)
		protectedRoutes.PUT("/set_language", server.SetLanguage)
		protectedRoutes.POST("/set_referrer", server.SetReferrer)

		// User settings routes
		protectedRoutes.PUT("/settings/notifications", server.UpdateNotificationPreferences)
		protectedRoutes.GET("/settings/notifications", server.GetNotificationPreferences)

		// Payverge Business routes
		protectedRoutes.POST("/businesses", server.CreateBusiness)
		protectedRoutes.GET("/businesses", server.GetMyBusinesses)
		protectedRoutes.GET("/businesses/:id", server.GetBusiness)
		protectedRoutes.PUT("/businesses/:id", server.UpdateBusiness)
		protectedRoutes.DELETE("/businesses/:id", server.DeleteBusiness)
		protectedRoutes.GET("/businesses/check-url", server.CheckCustomURLAvailability)

		// Counter management routes
		protectedRoutes.PUT("/businesses/:id/counters/settings", server.UpdateCounterSettings)
		protectedRoutes.GET("/businesses/:id/counters", server.GetBusinessCounters)
		protectedRoutes.GET("/businesses/:id/counters/available", server.GetAvailableCounters)

		// Menu routes
		protectedRoutes.POST("/businesses/:id/menu", server.CreateMenu)
		protectedRoutes.GET("/businesses/:id/menu", server.GetMenu)
		protectedRoutes.POST("/businesses/:id/menu/translate", server.TranslateMenu)

		// Phase 2: Enhanced Menu Management routes
		protectedRoutes.POST("/businesses/:id/menu/categories", server.AddMenuCategory)
		protectedRoutes.PUT("/businesses/:id/menu/categories/:category_index", server.UpdateMenuCategory)
		protectedRoutes.DELETE("/businesses/:id/menu/categories/:category_index", server.DeleteMenuCategory)
		protectedRoutes.POST("/businesses/:id/menu/items", server.AddMenuItem)
		protectedRoutes.PUT("/businesses/:id/menu/items", server.UpdateMenuItem)
		protectedRoutes.DELETE("/businesses/:id/menu/categories/:category_index/items/:item_index", server.DeleteMenuItem)

		// Table routes (Phase 2: Enhanced Table Management)
		protectedRoutes.POST("/businesses/:id/tables", server.CreateTableWithQR)
		protectedRoutes.GET("/businesses/:id/tables", server.GetBusinessTables)
		protectedRoutes.GET("/businesses/:id/tables/:tableId", server.GetTable)
		protectedRoutes.PUT("/tables/:id", server.UpdateTableDetails)
		protectedRoutes.DELETE("/tables/:id", server.DeleteTableSoft)

		// Phase 3: Bill Management routes
		protectedRoutes.POST("/businesses/:id/bills", server.CreateBill)
		protectedRoutes.GET("/businesses/:id/bills", server.GetBusinessBills)
		protectedRoutes.GET("/businesses/:id/bills/open", server.GetOpenBusinessBills)
		protectedRoutes.GET("/bills/:bill_id", server.GetBill)
		protectedRoutes.PUT("/bills/:bill_id", server.UpdateBill)
		protectedRoutes.POST("/bills/:bill_id/items", server.AddBillItem)
		protectedRoutes.DELETE("/bills/:bill_id/items/:item_id", server.RemoveBillItem)
		protectedRoutes.POST("/bills/:bill_id/close", server.CloseBill)

		// Crypto Payment routes (public for guests)
		publicRoutes.POST("/guest/bills/:bill_id/create-onchain", paymentHandler.CreateOnChainBill)
		publicRoutes.POST("/guest/bills/:bill_id/crypto-payment", paymentHandler.ProcessCryptoPayment)

		// Phase 6: Analytics and Dashboard routes
		analyticsHandler := handlers.NewAnalyticsHandler(database.GetDBWrapper())
		protectedRoutes.GET("/businesses/:id/analytics/sales", analyticsHandler.GetSalesAnalytics)
		protectedRoutes.GET("/businesses/:id/analytics/tips", analyticsHandler.GetTipAnalytics)
		protectedRoutes.GET("/businesses/:id/analytics/items", analyticsHandler.GetItemAnalytics)
		protectedRoutes.GET("/businesses/:id/analytics/dashboard", analyticsHandler.GetDashboardSummary)

		// Phase 7: Order Management routes
		protectedRoutes.POST("/businesses/:id/orders", handlers.CreateOrder)
		protectedRoutes.GET("/businesses/:id/orders", handlers.GetOrders)
		protectedRoutes.GET("/businesses/:id/orders/:orderId", handlers.GetOrder)
		protectedRoutes.GET("/businesses/:id/bills/:billId/orders", handlers.GetOrdersByBillID)
		protectedRoutes.PUT("/businesses/:id/orders/:orderId/status", handlers.UpdateOrderStatus)
		protectedRoutes.GET("/businesses/:id/analytics/live-bills", analyticsHandler.GetLiveBills)
		protectedRoutes.GET("/businesses/:id/reports/export", analyticsHandler.ExportSalesData)

		// Alternative Payment routes (business owner functions)
		protectedRoutes.POST("/bills/:bill_id/alternative-payment", paymentHandler.MarkAlternativePayment)
		protectedRoutes.GET("/bills/:bill_id/pending-alternative-payments", paymentHandler.GetPendingAlternativePayments)

		// Staff Management routes (business owner functions)
		protectedRoutes.POST("/businesses/:id/staff/invite", server.InviteStaff)
		protectedRoutes.POST("/businesses/:id/staff/invitations/:invitationId/resend", server.ResendInvitation)
		protectedRoutes.GET("/businesses/:id/staff", server.GetBusinessStaff)
		protectedRoutes.PUT("/businesses/:id/staff/:staffId/role", server.UpdateStaffRole)
		protectedRoutes.DELETE("/businesses/:id/staff/:staffId", server.RemoveStaff)

		// Referral system routes (protected - require authentication)
		protectedRoutes.POST("/referrals/register", server.RegisterReferrer)
		protectedRoutes.GET("/referrals/referrer/:wallet_address", server.GetReferrer)
		protectedRoutes.GET("/referrals/referrer/:wallet_address/referrals", server.GetReferrerReferrals)
		protectedRoutes.POST("/referrals/process", server.ProcessReferral)
		protectedRoutes.POST("/referrals/claim", server.ClaimCommission)
		protectedRoutes.PUT("/referrals/referrer/:wallet_address/code", server.UpdateReferralCode)

		// Withdrawal History routes (protected - require authentication)
		withdrawalHandler := handlers.NewWithdrawalHandler(database.GetDBWrapper())
		protectedRoutes.POST("/businesses/:id/withdrawals", withdrawalHandler.CreateWithdrawal)
		protectedRoutes.GET("/businesses/:id/withdrawals", withdrawalHandler.GetWithdrawalHistory)
		protectedRoutes.GET("/businesses/:id/withdrawals/:withdrawalId", withdrawalHandler.GetWithdrawal)
		protectedRoutes.PUT("/businesses/:id/withdrawals/:withdrawalId/status", withdrawalHandler.UpdateWithdrawalStatus)

		// Multi-currency and multilingual routes (protected - require authentication)
		currencyHandler := handlers.NewCurrencyHandler(database.GetDBWrapper(), exchangeRateService, translationService)
		protectedRoutes.GET("/businesses/:id/currencies", currencyHandler.GetBusinessCurrencies)
		protectedRoutes.PUT("/businesses/:id/currencies", currencyHandler.UpdateBusinessCurrencies)
		protectedRoutes.GET("/businesses/:id/languages", currencyHandler.GetBusinessLanguages)
		protectedRoutes.PUT("/businesses/:id/languages", currencyHandler.UpdateBusinessLanguages)
		protectedRoutes.GET("/translations", currencyHandler.GetTranslatedContent)
		protectedRoutes.PUT("/translations", currencyHandler.UpdateTranslation)
		protectedRoutes.GET("/menu-translations", currencyHandler.GetMenuTranslations)

		// Batch translation routes
		protectedRoutes.POST("/businesses/:id/translate", server.TranslateEntireMenu)
		protectedRoutes.GET("/translation-jobs/:jobId/status", server.GetTranslationStatus)
	}

	// Admin routes (require authentication and admin role)
	adminRoutes := r.Group("/api/v1/admin")
	adminRoutes.Use(server.AuthenticationAdminMiddleware())
	{
		adminRoutes.GET("/get_all_users", server.GetAllUsers)

		// Admin Operations

		// Subscriber Routes
		adminRoutes.GET("/get_subscribers", server.GetSubscribers)

		adminRoutes.GET("/multisig-tx", server.GetMultisigTx)
		adminRoutes.PUT("/multisig-tx", server.PutMultisigTx)
		adminRoutes.DELETE("/multisig-tx", server.DeleteMultisigTx)
		adminRoutes.PATCH("/multisig-tx", server.PatchMultisigTx)

		// Codes Routes
		adminRoutes.POST("/create_code", server.CreateCode)
		adminRoutes.GET("/get_code", server.GetCode)
		adminRoutes.PUT("/update_code", server.UpdateCode)
		adminRoutes.DELETE("/delete_code", server.DeleteCode)
		adminRoutes.GET("/get_all_codes", server.GetAllCodes)

		// Admin referral management
		adminRoutes.PUT("/referrals/referrer/:wallet_address/deactivate", server.DeactivateReferrer)
	}

	// Go routine to clean up the challenge store every 5 minutes
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			server.ChallengeStore.Cleanup()
		}
	}()

	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}
	// Initializing the server in a goroutine so that
	// it won't block the graceful shutdown handling below
	go func() {
		log.Println("Starting server on port: 8080")
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %s\n", err)
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
	log.Println("Shutting down server...")

	// Stop payment monitor
	paymentMonitor.Stop()

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown: ", err)
	}

	log.Println("Server exiting")
}

// Removed old CORS middleware - now using secure CORS from middleware package
