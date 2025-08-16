package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// Module for metrics related functions
var (
	// HTTP metrics
	TotalRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"path", "method", "status"},
	)

	// Car metrics
	CarMetrics = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "car_metrics_total",
			Help: "Car-related metrics by type",
		},
		[]string{"metric_type"}, // listed, sold, bought
	)

	// Fleet metrics
	FleetInvestmentAmount = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "fleet_investment_amount",
			Help:    "Distribution of investment amounts in fleets",
			Buckets: prometheus.LinearBuckets(100, 100, 10), // Starts at 100, steps of 100, 10 buckets
		},
	)

	FleetRentalDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "fleet_rental_duration_days",
			Help:    "Distribution of rental durations in days",
			Buckets: prometheus.LinearBuckets(1, 1, 30), // 1 to 30 days
		},
	)

	// Financial metrics
	FinancialMetrics = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "financial_metrics_total",
			Help: "Financial metrics by type",
		},
		[]string{"metric_type"}, // invested, rewarded
	)

	// User metrics
	UserMetrics = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "user_metrics_total",
			Help: "User-related metrics by type",
		},
		[]string{"metric_type"}, // users, subscribers, fleets
	)

	ActiveSubscribers = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "active_subscribers_total",
			Help: "Current number of active subscribers",
		},
	)

	// Operation tracking
	FleetOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "fleet_operations_total",
			Help: "Total number of fleet operations by type",
		},
		[]string{"operation"},
	)

	CarOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "car_operations_total",
			Help: "Total number of car operations by type",
		},
		[]string{"operation"},
	)

	// Authentication metrics
	AuthOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "auth_operations_total",
			Help: "Total number of authentication operations by type",
		},
		[]string{"operation"},
	)

	// KYC metrics
	KYCOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "kyc_operations_total",
			Help: "Total number of KYC operations by type and status",
		},
		[]string{"operation", "status"},
	)

	// Telegram Bot Metrics
	TelegramCommands = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "telegram_commands_total",
			Help: "Total number of Telegram bot commands by type",
		},
		[]string{"command", "status"},
	)

	TelegramActiveUsers = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "telegram_active_users",
			Help: "Number of users with connected Telegram accounts",
		},
	)

	TelegramMessageLength = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "telegram_message_length_chars",
			Help:    "Distribution of Telegram message lengths in characters",
			Buckets: prometheus.LinearBuckets(50, 50, 20), // 50 to 1000 chars in steps of 50
		},
	)

	TelegramResponseTime = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "telegram_response_time_seconds",
			Help:    "Time taken to process and respond to Telegram commands",
			Buckets: prometheus.ExponentialBuckets(0.01, 2, 10), // From 10ms to ~10s
		},
		[]string{"command"},
	)

	// NFT Metrics
	NFTOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "nft_operations_total",
			Help: "Total number of NFT operations by type and status",
		},
		[]string{"operation", "status"},
	)

	NFTResponseTime = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "nft_response_time_seconds",
			Help:    "Time taken to process NFT operations",
			Buckets: prometheus.ExponentialBuckets(0.01, 2, 10), // From 10ms to ~10s
		},
		[]string{"operation"},
	)

	// Reward Metrics
	RewardOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "reward_operations_total",
			Help: "Total number of reward operations by type and status",
		},
		[]string{"operation", "status"},
	)

	RewardAmounts = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "reward_amounts_distribution",
			Help:    "Distribution of reward amounts by type",
			Buckets: prometheus.ExponentialBuckets(1, 2, 15), // From $1 to ~$16k
		},
		[]string{"type"}, // direct, referral, car
	)

	RewardResponseTime = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "reward_response_time_seconds",
			Help:    "Time taken to process reward operations",
			Buckets: prometheus.ExponentialBuckets(0.01, 2, 10), // From 10ms to ~10s
		},
		[]string{"operation"},
	)

	// Subscription Metrics
	SubscriptionOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "subscription_operations_total",
			Help: "Total number of subscription operations by type and status",
		},
		[]string{"operation", "status"},
	)

	SubscriptionResponseTime = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "subscription_response_time_seconds",
			Help:    "Time taken to process subscription operations",
			Buckets: prometheus.ExponentialBuckets(0.01, 2, 10), // From 10ms to ~10s
		},
		[]string{"operation"},
	)

	SubscribersByContact = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "subscribers_by_contact_method",
			Help: "Number of subscribers by preferred contact method",
		},
		[]string{"contact_method"},
	)

	// User Operation Metrics
	UserOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "user_operations_total",
			Help: "Total number of user operations by type and status",
		},
		[]string{"operation", "status"},
	)

	UserResponseTime = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "user_response_time_seconds",
			Help:    "Time taken to process user operations",
			Buckets: prometheus.ExponentialBuckets(0.01, 2, 10), // From 10ms to ~10s
		},
		[]string{"operation"},
	)

	UserReferrals = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "user_referrals_total",
			Help: "Total number of user referrals by status",
		},
		[]string{"status"},
	)

	ActiveUsers = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "active_users_total",
			Help: "Current number of active users",
		},
	)

	// Distribution metrics
	FleetMetrics = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "fleet_metrics_distribution",
			Help:    "Distribution of various fleet metrics",
			Buckets: prometheus.LinearBuckets(1, 1, 30),
		},
		[]string{"metric_type"}, // investment_amount, rental_duration
	)

	CarOperationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "car_operation_duration_days_distribution",
			Help:    "Duration of various car operations in days",
			Buckets: prometheus.LinearBuckets(1, 1, 30),
		},
		[]string{"operation"},
	)
)

func Init() {
	// Register HTTP metrics
	prometheus.MustRegister(TotalRequests)

	// Register consolidated metrics
	prometheus.MustRegister(CarMetrics)
	prometheus.MustRegister(FinancialMetrics)
	prometheus.MustRegister(UserMetrics)
	prometheus.MustRegister(ActiveSubscribers)
	prometheus.MustRegister(FleetInvestmentAmount)
	prometheus.MustRegister(FleetRentalDuration)

	// Register operation tracking
	prometheus.MustRegister(FleetOperations)
	prometheus.MustRegister(CarOperations)

	// Register auth and KYC metrics
	prometheus.MustRegister(AuthOperations)
	prometheus.MustRegister(KYCOperations)

	// Register Telegram metrics
	prometheus.MustRegister(TelegramCommands)
	prometheus.MustRegister(TelegramActiveUsers)
	prometheus.MustRegister(TelegramMessageLength)
	prometheus.MustRegister(TelegramResponseTime)

	// Register NFT metrics
	prometheus.MustRegister(NFTOperations)
	prometheus.MustRegister(NFTResponseTime)

	// Register Reward metrics
	prometheus.MustRegister(RewardOperations)
	prometheus.MustRegister(RewardAmounts)
	prometheus.MustRegister(RewardResponseTime)

	// Register subscription metrics
	prometheus.MustRegister(SubscriptionOperations)
	prometheus.MustRegister(SubscriptionResponseTime)
	prometheus.MustRegister(SubscribersByContact)

	// Register User operation metrics
	prometheus.MustRegister(UserOperations)
	prometheus.MustRegister(UserResponseTime)
	prometheus.MustRegister(UserReferrals)
	prometheus.MustRegister(ActiveUsers)

	// Register distribution metrics
	prometheus.MustRegister(FleetMetrics)
	prometheus.MustRegister(CarOperationDuration)
}
