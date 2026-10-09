package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/plugins/mercadopago"
	"github.com/stdevmac/payverge/backend/internal/plugins/paypal"
	"github.com/stdevmac/payverge/backend/internal/plugins/stripe"
	"github.com/stdevmac/payverge/backend/internal/server"

	"github.com/gin-gonic/gin"
)

// pluginConnectionTestTimeout bounds each provider probe so a hung provider
// can't hold the operator's request open.
const pluginConnectionTestTimeout = 8 * time.Second

// TestPluginConnection performs a cheap, read-only call against a payment
// provider using the business's stored credentials so operators can verify a
// configuration before a guest's payment fails. It returns {ok, provider,
// message} with user-safe messages and never echoes credentials. Auth failures
// map to an "invalid credentials" message; providers without a testable API
// return ok:false with "not supported".
func (ph *PluginHandlers) TestPluginConnection(c *gin.Context) {
	businessID, err := resolveBusinessID(c.Param("id"))
	if err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid business ID")
		return
	}

	provider := strings.ToLower(strings.TrimSpace(c.Param("plugin_id")))
	if provider == "" {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Plugin ID is required")
		return
	}

	configMap, err := database.GetBusinessPluginConfig(businessID, provider)
	if err != nil {
		if errors.Is(err, database.ErrBusinessPluginNotEnabled) {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Plugin is not enabled for this business")
			return
		}
		log.Printf("Failed to load %s config for business %d: %v", provider, businessID, err)
		server.RespondWithError(c, http.StatusInternalServerError, "", "Internal server error")
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), pluginConnectionTestTimeout)
	defer cancel()

	var testErr error
	var authFailed bool
	switch provider {
	case "stripe":
		testErr = stripe.TestConnection(ctx, configMap)
		authFailed = errors.Is(testErr, stripe.ErrStripeAuthFailed)
	case "mercadopago":
		testErr = mercadopago.TestConnection(ctx, configMap)
		authFailed = errors.Is(testErr, mercadopago.ErrMercadoPagoAuthFailed)
	case "paypal":
		testErr = paypal.TestConnection(ctx, configMap)
		authFailed = errors.Is(testErr, paypal.ErrPayPalAuthFailed)
	default:
		c.JSON(http.StatusOK, gin.H{
			"ok":       false,
			"provider": provider,
			"message":  "Connection testing is not supported for this payment method.",
		})
		return
	}

	if testErr != nil {
		message := "Couldn't reach the payment provider. Please try again."
		if authFailed {
			message = "Invalid credentials. Please double-check the keys you entered."
		}
		// Log the underlying error server-side only; the response never carries
		// provider error detail (which could echo config back to the operator's
		// screen or leak provider internals).
		log.Printf("Plugin connection test failed for business %d provider %s: %v", businessID, provider, testErr)
		c.JSON(http.StatusOK, gin.H{
			"ok":       false,
			"provider": provider,
			"message":  message,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":       true,
		"provider": provider,
		"message":  "Connection successful.",
	})
}
