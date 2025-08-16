package main

import (
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"invoice-generator/database"
	"invoice-generator/handlers"
	"invoice-generator/middleware"
	"invoice-generator/services"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/ulule/limiter/v3"
	mgin "github.com/ulule/limiter/v3/drivers/middleware/gin"
	"github.com/ulule/limiter/v3/drivers/store/memory"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using system environment variables")
	}

	// Connect to database
	database.Connect()
	database.Migrate()

	// Initialize services
	blockchainService, err := services.NewBlockchainService()
	if err != nil {
		log.Printf("Warning: Failed to initialize blockchain service: %v", err)
		log.Println("Continuing without blockchain integration...")
		blockchainService = nil
	}
	emailService := services.NewEmailService()
	_ = services.NewQRLinkService() // QR service for future use
	var eventListener *services.EventListenerService
	if blockchainService != nil {
		eventListener = services.NewEventListenerService(blockchainService, emailService)
		if err := eventListener.Start(); err != nil {
			log.Printf("Warning: Failed to start event listener: %v", err)
		}
	}

	// Initialize middleware
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "development-secret-key-change-in-production"
		log.Println("Warning: Using default JWT secret. Set JWT_SECRET environment variable in production.")
	}
	authMiddleware := middleware.NewAuthMiddleware(jwtSecret)

	// Initialize handlers
	invoiceHandler := handlers.NewInvoiceHandler()
	websocketHandler := handlers.NewWebSocketHandler()

	// Setup Gin router
	router := gin.Default()

	// Setup rate limiting
	memoryStore := memory.NewStore()
	rate := limiter.Rate{
		Period: 60000000000, // 1 minute in nanoseconds
		Limit:  100,         // 100 requests per minute
	}
	instance := limiter.New(memoryStore, rate)
	router.Use(mgin.NewMiddleware(instance))

	// CORS middleware with security headers
	corsConfig := cors.DefaultConfig()
	allowedOrigins := os.Getenv("ALLOWED_ORIGINS")
	if allowedOrigins != "" {
		corsConfig.AllowOrigins = strings.Split(allowedOrigins, ",")
	} else {
		// Default origins for development
		corsConfig.AllowOrigins = []string{"http://localhost:3000", "http://localhost:3001"}
	}
	corsConfig.AllowCredentials = true
	corsConfig.AllowHeaders = []string{"Origin", "Content-Length", "Content-Type", "Authorization"}
	router.Use(cors.New(corsConfig))

	// Security headers middleware
	router.Use(func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'self'")
		c.Next()
	})

	// Serve static files (QR codes)
	router.Static("/static", "./static")

	// Authentication routes (public)
	auth := router.Group("/api/v1/auth")
	{
		auth.GET("/challenge", authMiddleware.GetChallengeHandler)
		auth.POST("/authenticate", authMiddleware.AuthenticateHandler)
	}

	// API routes
	api := router.Group("/api/v1")
	{
		// Public routes
		api.GET("/invoices/:id", invoiceHandler.GetInvoice)
		api.GET("/invoices/:id/payments", invoiceHandler.GetInvoicePayments)

		// Protected routes (require authentication)
		protected := api.Group("")
		protected.Use(authMiddleware.RequireAuth())
		{
			protected.POST("/invoices", invoiceHandler.CreateInvoice)
			protected.GET("/invoices", invoiceHandler.GetInvoicesByCreator)
			protected.DELETE("/invoices/:id", authMiddleware.RequireOwnership(), invoiceHandler.CancelInvoice)
		}
	}

	// WebSocket endpoint
	router.GET("/ws", websocketHandler.HandleWebSocket)

	// Payment page routes (for frontend)
	router.GET("/pay/:id", func(c *gin.Context) {
		// In production, this would serve the React app
		c.Redirect(302, "http://localhost:3000/pay/"+c.Param("id"))
	})

	router.GET("/p/:id", func(c *gin.Context) {
		// Short link redirect
		c.Redirect(302, "http://localhost:3000/pay/"+c.Param("id"))
	})

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"service": "invoice-generator-backend",
		})
	})

	// Graceful shutdown setup
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Println("Shutting down server...")
		if eventListener != nil {
			eventListener.Stop()
		}
		os.Exit(0)
	}()

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
