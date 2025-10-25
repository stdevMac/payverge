package server

import (
	"log"
	"net/http"
	"net/mail"
	"time"

	"payverge/internal/database"
	"payverge/internal/metrics"
	"payverge/internal/structs"

	"github.com/gin-gonic/gin"
)

// Subscribe it's a function for subscribing our emails to our newsletter
func Subscribe(c *gin.Context) {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Seconds()
		metrics.SubscriptionResponseTime.WithLabelValues("subscribe").Observe(duration)
	}()

	var subscriberRequest structs.SubscriptionRequest

	if err := c.ShouldBindJSON(&subscriberRequest); err != nil {
		metrics.SubscriptionOperations.WithLabelValues("subscribe", "error").Inc()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid data"})
		return
	}
	subscriber := structs.Subscriber{
		Name:             subscriberRequest.Name,
		Email:            subscriberRequest.Email,
		PhoneNumber:      subscriberRequest.PhoneNumber,
		CompanyName:      subscriberRequest.CompanyName,
		PreferredContact: subscriberRequest.ContactWay,
		Message:          subscriberRequest.Message,
		SubscriptionDate: time.Now(),
	}

	switch subscriberRequest.SocialMedia {
	case "X":
		subscriber.TwitterHandle = subscriberRequest.Username
	case "TG":
		subscriber.TelegramHandle = subscriberRequest.Username
	}

	if !valid(subscriber.Email) {
		metrics.SubscriptionOperations.WithLabelValues("subscribe", "error").Inc()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid email"})
		return
	}

	err := database.AddSubscriber(subscriber)
	if err != nil {
		metrics.SubscriptionOperations.WithLabelValues("subscribe", "error").Inc()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error subscribing"})
		return
	}

	// Update metrics
	metrics.SubscriptionOperations.WithLabelValues("subscribe", "success").Inc()
	metrics.SubscribersByContact.WithLabelValues(subscriber.PreferredContact).Inc()
	metrics.ActiveSubscribers.Inc()

	c.JSON(http.StatusOK, gin.H{"message": "Subscribed successfully"})

	// Track subscription
	properties := map[string]interface{}{
		"email":             subscriber.Email,
		"preferred_contact": subscriber.PreferredContact,
		"company_name":      subscriber.CompanyName,
		"social_media":      subscriberRequest.SocialMedia,
		"timestamp":         time.Now(),
	}
	err = metrics.TrackGeneralEvent("Newsletter Subscription", properties)
	if err != nil {
		log.Printf("Failed to track newsletter subscription: %v", err)
	}

	// Track subscription
	properties = map[string]interface{}{
		"email":     subscriber.Email,
		"timestamp": time.Now(),
	}
	err = metrics.TrackGeneralEvent("User Subscription", properties)
	if err != nil {
		log.Printf("Error tracking subscription: %v", err)
	}
}

// UnSubscribe it's a function for unsubscribing our emails from our newsletter
func UnSubscribe(c *gin.Context) {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Seconds()
		metrics.SubscriptionResponseTime.WithLabelValues("unsubscribe").Observe(duration)
	}()

	subscriber := c.Query("email")

	if !valid(subscriber) {
		metrics.SubscriptionOperations.WithLabelValues("unsubscribe", "error").Inc()
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid email"})
		return
	}

	err := database.UnSubscribe(subscriber)
	if err != nil {
		metrics.SubscriptionOperations.WithLabelValues("unsubscribe", "error").Inc()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error unsubscribing"})
		return
	}

	// Update metrics
	metrics.SubscriptionOperations.WithLabelValues("unsubscribe", "success").Inc()
	metrics.ActiveSubscribers.Dec()

	c.JSON(http.StatusOK, gin.H{"message": "Unsubscribed successfully"})

	// Track unsubscription
	properties := map[string]interface{}{
		"email":     subscriber,
		"timestamp": time.Now(),
	}
	err = metrics.TrackGeneralEvent("Newsletter Unsubscription", properties)
	if err != nil {
		log.Printf("Failed to track newsletter unsubscription: %v", err)
	}
}

// Protected Asserts

func valid(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

// GetSubscribers it's a function for getting all subscribers
func GetSubscribers(c *gin.Context) {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Seconds()
		metrics.SubscriptionResponseTime.WithLabelValues("list").Observe(duration)
	}()

	subscribers, err := database.GetSubscribers()
	if err != nil {
		metrics.SubscriptionOperations.WithLabelValues("list", "error").Inc()
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error getting subscribers"})
		return
	}

	metrics.SubscriptionOperations.WithLabelValues("list", "success").Inc()
	c.JSON(http.StatusOK, subscribers)

	// Track subscribers list view
	properties := map[string]interface{}{
		"subscribers_count": len(subscribers),
		"timestamp":         time.Now(),
	}
	err = metrics.TrackGeneralEvent("Subscribers List Viewed", properties)
	if err != nil {
		log.Printf("Failed to track subscribers list view: %v", err)
	}
}
