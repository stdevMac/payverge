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

	"github.com/stdevMac/shares/internal/faucet"

	"github.com/stdevMac/shares/internal/s3"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/stdevMac/shares/internal/database"
	"github.com/stdevMac/shares/internal/emails"
	"github.com/stdevMac/shares/internal/metrics"
	"github.com/stdevMac/shares/internal/notifications"
	"github.com/stdevMac/shares/internal/server"
	"github.com/stdevMac/shares/internal/telegram"
)

func main() {
	// Get flags and initialize the database
	var (
		uriFlag                = flag.String("uri", "", "Database URI")
		usernameFlag           = flag.String("username", "", "Database username")
		passwordFlag           = flag.String("password", "", "Database password")
		production             = flag.Bool("production", false, "Production mode")
		faucetPrivateKey       = flag.String("faucet-private-key", "", "Private key for Faucet Address")
		rpcUrl                 = flag.String("rpc-url", "", "RPC URL for the Ethereum node")
		chainId                = flag.Int64("chain-id", 1, "Chain ID for the Ethereum network")
		usdcContractAddress    = flag.String("usdc-contract", "", "USDC token contract address")
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
		"Token Fleet Info <info@tokenfleet.io>",
		"Token Fleet News <info@news.tokenfleet.io>",
		"Token Fleet Updates <info@updates.tokenfleet.io>",
		"63494ce7-430a-43c6-b950-fa732da363a3",
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
	config := database.NewConfig(*uriFlag, *usernameFlag, *passwordFlag)
	database.InitCarsDB(config)

	// Initialize S3
	if err := s3.InitS3(*s3Bucket, *awsAccessKey, *awsSecretKey, *awsRegion, *s3EndpointURL); err != nil {
		log.Fatalf("Failed to initialize S3: %v", err)
	}
	// Initialize S3 Protected
	if err := s3.InitS3Protected(*s3ProtectedBucket, *awsProtectedAccessKey, *awsProtectedSecretKey, *awsRegion, *s3ProtectedEndpointURL); err != nil {
		log.Fatalf("Failed to initialize S3: %v", err)
	}
	// Initialize the metrics
	metrics.Init()

	// Initialize PostHog
	metrics.InitPostHogClient("phc_lHcZzDzjvzncDeqCoRugImiNoTS0RXOH7lyTRs7pBjI", "https://us.i.posthog.com")
	defer metrics.ClosePostHogClient()

	r := gin.Default()
	r.Use(gin.Recovery())
	r.Use(CORSMiddleware())
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
		// Health check endpoint
		publicRoutes.GET("/health", func(c *gin.Context) {
			c.JSON(200, gin.H{
				"status": "UP",
			})
		})

		// Telegram webhook
		publicRoutes.POST("/telegram/webhook", server.WebhookHandler)

		// Serve static files from a directory
		publicRoutes.POST("/subscribe", server.Subscribe)
		publicRoutes.GET("/unsubscribe", server.UnSubscribe)

		// General Routes
		// TODO: implement

		// Error logging endpoint
		publicRoutes.POST("/logs/error", server.LogError)
	}

	// Protected routes (require authentication)
	protectedRoutes := r.Group("/api/v1/inside")
	protectedRoutes.Use(server.AuthenticationMiddleware())
	{
		// Faucet endpoint
		protectedRoutes.POST("/faucet", server.CheckAndTopUp)
		protectedRoutes.GET("/faucet/check/:address", server.CheckFaucetAvailability)

		// File upload endpoint
		protectedRoutes.POST("/upload", server.UploadFile)

		// File upload endpoint
		protectedRoutes.POST("/upload_protected", server.UploadFileProtected)

		// User routes
		protectedRoutes.GET("/get_user/:address", server.GetUser)
		protectedRoutes.PUT("/update_notification_preference", server.UpdateNotificationPreference)
		protectedRoutes.PUT("/update_user", server.UpdateUser)
		protectedRoutes.PUT("/set_language", server.SetLanguage)
		protectedRoutes.POST("/set_referrer", server.SetReferrer)

		// User settings routes
		protectedRoutes.PUT("/settings/notifications", server.UpdateNotificationPreferences)
		protectedRoutes.GET("/settings/notifications", server.GetNotificationPreferences)
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

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown: ", err)
	}

	log.Println("Server exiting")
}

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, PATCH, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.Status(http.StatusOK)
			return
		}

		c.Next()
	}
}
