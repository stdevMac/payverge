package services

import (
	"log"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Canonical plugin name constants. These are the `Plugin.Name` values seeded
// by InitializeDefaultPlugins below; other packages (e.g. EnableDefaultPaymentPlugins)
// look plugins up by these names rather than re-typing string literals.
const (
	PluginNameUSDCPayment       = "usdc_payment"
	PluginNameCrossChainPayment = "cross_chain_payment"
	PluginNameStripe            = "stripe"
)

// CrossChainGuestSettlementAvailable says whether guests can actually settle
// a bill through the cross_chain_payment (LI.FI) rail. It is the single source
// for every place that offers or advertises that rail: the guest handlers
// derive their gate from it, and while it is false the catalog seed marks the
// plugin coming-soon (so operators cannot enable it and existing
// subscriptions are switched off at boot) with copy that says why.
//
// It is false because a bridged delivery cannot be payer-bound: the venue
// wallet receives whatever the route produced after fees and slippage, sent by
// the bridge contract rather than the guest, so no exact quoted amount can be
// required and one guest's bridged payment could settle another guest's bill.
// See handlers.guestCrossChainSettlementEnabled. Flipping it back to true also
// needs the plugins.coming_soon column cleared for cross_chain_payment: the
// seed only writes coming_soon on the coming-soon path.
const CrossChainGuestSettlementAvailable = false

// crossChainSeedCopy picks the cross-chain catalog copy that matches
// CrossChainGuestSettlementAvailable, so the seed never describes a rail
// guests cannot use as if it were live.
func crossChainSeedCopy(live, unavailable string) string {
	if CrossChainGuestSettlementAvailable {
		return live
	}
	return unavailable
}

// crossChainSeedMessage is the cross_chain_payment catalog message per
// language (en is the base row; es / es-AR are plugin translations).
var crossChainSeedMessage = map[string]string{
	"en": crossChainSeedCopy(
		"Advanced / selective crypto rail. Not the primary way restaurants get paid.",
		"Not available yet: a converted payment cannot be matched to a single guest's bill, so guests cannot pay with other tokens. Use USDC Payment for crypto checkout.",
	),
	"es": crossChainSeedCopy(
		"Rail cripto avanzado y selectivo. No es el camino principal de cobro para un restaurante.",
		"Todavía no disponible: un pago convertido no se puede asociar a la cuenta de un solo cliente, así que los clientes no pueden pagar con otros tokens. Usa Pago con USDC para cobrar en cripto.",
	),
	"es-AR": crossChainSeedCopy(
		"Rail cripto avanzado y selectivo. No es el camino principal de cobro para un restaurante.",
		"Todavía no disponible: un pago convertido no se puede asociar a la cuenta de un solo cliente, así que los clientes no pueden pagar con otros tokens. Usá Pago con USDC para cobrar en cripto.",
	),
}

// SINGLE SOURCE OF TRUTH RULE (do not re-add code-backed plugins here):
//
// A plugin's catalog row comes from exactly ONE place.
//   - Plugins that ship as Go code (stripe, paypal, mercadopago,
//     telegram, trustpilot) are the authoritative source for their own row: the
//     registry's SyncPluginsToDatabase writes them from each plugin's Get* /
//     IsActive() methods at boot (main.go runs the registry sync BEFORE this
//     seed). Seeding them here too is dead weight that silently drifts out of
//     sync (e.g. the PayPal schema once diverged), so they are intentionally
//     ABSENT from defaultPlugins below.
//   - Only genuinely code-less plugins are seeded here: the crypto rails
//     (usdc_payment, cross_chain_payment), the email reports
//     (weekly_email_report, daily_email_report). The catalog advertises no
//     placeholder plugins for features that do not exist yet.
//
// TestSeedAndRegistryPluginNamesDisjoint enforces that no name appears in both
// the Go registry and this seed. If you add a plugin, pick ONE home for its row.

// DefaultSeedPluginNames returns the names of every plugin seeded by
// InitializeDefaultPlugins. Exported so the catalog source-of-truth guard test
// (in the plugins package, which can see the live registry) can assert the seed
// and the registry never claim the same plugin name.
func DefaultSeedPluginNames() []string {
	names := make([]string, 0, len(defaultSeedPlugins))
	for _, p := range defaultSeedPlugins {
		names = append(names, p.Name)
	}
	return names
}

// defaultSeedPlugins is the seed catalog. Per the SINGLE SOURCE OF TRUTH RULE
// above it contains ONLY code-less plugins; the five code-backed plugins
// (stripe, paypal, mercadopago, telegram, trustpilot) are written by the Go registry's SyncPluginsToDatabase
// and must never be re-added here.
var defaultSeedPlugins = []database.Plugin{
	// Optional crypto rails — selective / never door-lead. Catalog copy uses
	// restaurant-operator language (not engineer SaaS / blockchain jargon).
	{
		Name:        PluginNameUSDCPayment,
		DisplayName: "USDC Payment",
		Description: "Optional: let guests pay in USDC. Settlement follows network confirmation; funds arrive in your settlement wallet afterward. Guests pay the applicable network fee.",
		Message:     "Optional crypto rail. Not required for dinner service — set up Mercado Pago or another checkout method first.",
		Image:       "/images/plugins/usdc.png",
		Category:    "payment",
		Version:     "1.0.0",
		Features:    `["Optional crypto checkout", "Settlement follows network confirmation", "Guest pays network fee", "Stable USDC value", "Enable only if you want it"]`,
		ConfigSchema: `{
				"type": "object",
				"properties": {
					"enabled": {
						"type": "boolean",
						"title": "Accept USDC at checkout",
						"description": "Allow guests to pay with USDC (optional crypto rail)",
						"default": true
					},
					"show_recommended": {
						"type": "boolean",
						"title": "Highlight at checkout",
						"description": "Show USDC as a recommended payment method (off by default)",
						"default": false
					}
				}
			}`,
		IsActive: true,
	},
	{
		Name:        PluginNameCrossChainPayment,
		DisplayName: "Any Token Payment",
		Description: "Optional: accept other crypto tokens and convert them to USDC for settlement",
		Message:     crossChainSeedMessage["en"],
		Image:       "/images/plugins/cross-chain.png",
		Category:    "payment",
		Version:     "1.0.0",
		Features:    `["Optional crypto checkout", "Converts to USDC", "Multiple networks", "Advanced / selective", "Not required for dinner service"]`,
		ConfigSchema: `{
				"type": "object",
				"properties": {
					"enabled": {
						"type": "boolean",
						"title": "Accept other crypto tokens",
						"description": "Allow guests to pay with supported tokens converted to USDC",
						"default": true
					},
					"supported_chains": {
						"type": "array",
						"title": "Supported networks",
						"description": "Networks to accept payments from",
						"items": {
							"type": "string",
							"enum": ["ethereum", "polygon", "arbitrum", "optimism", "avalanche", "bsc"]
						},
						"default": ["ethereum", "polygon", "arbitrum", "optimism"]
					}
				}
			}`,
		IsActive:   true,
		ComingSoon: !CrossChainGuestSettlementAvailable,
	},
	{
		Name:        "weekly_email_report",
		DisplayName: "Weekly Email Report",
		Description: "Receive comprehensive weekly business analytics reports via email",
		Image:       "/images/plugins/weekly-report.png",
		Category:    "analytics",
		Version:     "1.0.0",
		Features:    `["Weekly Analytics", "Sales Summary", "Top Items", "Customer Insights", "Revenue Trends", "Custom Schedule"]`,
		ConfigSchema: `{
				"type": "object",
				"required": ["day_of_week", "hour"],
				"properties": {
					"enabled": {
						"type": "boolean",
						"title": "Enable Weekly Reports",
						"description": "Receive weekly analytics reports via email",
						"default": true
					},
					"day_of_week": {
						"type": "integer",
						"title": "Day of Week",
						"description": "Day to receive report (0=Sunday, 1=Monday, etc.)",
						"enum": [0, 1, 2, 3, 4, 5, 6],
						"default": 1
					},
					"hour": {
						"type": "integer",
						"title": "Hour of Day",
						"description": "Hour to send report (0-23)",
						"minimum": 0,
						"maximum": 23,
						"default": 9
					},
					"timezone": {
						"type": "string",
						"title": "Timezone",
						"description": "Your business timezone",
						"default": "UTC"
					}
				}
			}`,
		IsActive: true,
	},
	{
		Name:        "daily_email_report",
		DisplayName: "Daily Email Report",
		Description: "Receive daily business performance summaries delivered to your inbox",
		Image:       "/images/plugins/daily-report.png",
		Category:    "analytics",
		Version:     "1.0.0",
		Features:    `["Daily Analytics", "Sales Overview", "Order Summary", "Payment Breakdown", "Performance Metrics", "Custom Time"]`,
		ConfigSchema: `{
				"type": "object",
				"required": ["hour"],
				"properties": {
					"enabled": {
						"type": "boolean",
						"title": "Enable Daily Reports",
						"description": "Receive daily analytics reports via email",
						"default": true
					},
					"hour": {
						"type": "integer",
						"title": "Hour of Day",
						"description": "Hour to send report (0-23)",
						"minimum": 0,
						"maximum": 23,
						"default": 8
					},
					"timezone": {
						"type": "string",
						"title": "Timezone",
						"description": "Your business timezone",
						"default": "UTC"
					}
				}
			}`,
		IsActive: true,
	},
}

// InitializeDefaultPlugins creates the code-less default plugins in the database
// (see defaultSeedPlugins and the SINGLE SOURCE OF TRUTH RULE above). Code-backed
// plugins are handled by the registry sync, not here.
func InitializeDefaultPlugins() error {
	log.Println("Initializing default plugins...")

	// Create plugins if they don't exist
	for _, plugin := range defaultSeedPlugins {
		existingPlugin, err := database.GetPluginByName(plugin.Name)
		if err != nil {
			// Plugin doesn't exist, create it
			_, err := database.CreatePlugin(plugin)
			if err != nil {
				log.Printf("Failed to create plugin %s: %v", plugin.Name, err)
				continue
			}
			log.Printf("Created plugin: %s", plugin.DisplayName)
		} else {
			if plugin.ComingSoon {
				if err := updateComingSoonDefaultPlugin(existingPlugin.ID, plugin); err != nil {
					log.Printf("Failed to update coming-soon plugin %s: %v", plugin.Name, err)
					continue
				}
				if err := disableBusinessPluginSubscriptionsForComingSoonPlugin(plugin.Name); err != nil {
					log.Printf("Failed to disable coming-soon business plugin subscriptions for %s: %v", plugin.Name, err)
					continue
				}
				log.Printf("Updated coming-soon plugin: %s (ID: %d)", plugin.DisplayName, existingPlugin.ID)
				continue
			}
			// Marketing copy sync: description/features/message are catalog copy,
			// not operator state — keep existing rows aligned with honest seed strings.
			if existingPlugin.Description != plugin.Description ||
				existingPlugin.Features != plugin.Features ||
				existingPlugin.Message != plugin.Message ||
				existingPlugin.ConfigSchema != plugin.ConfigSchema {
				if err := database.GetDB().Model(&database.Plugin{}).
					Where("id = ?", existingPlugin.ID).
					Updates(map[string]interface{}{
						"description":   plugin.Description,
						"features":      plugin.Features,
						"message":       plugin.Message,
						"config_schema": plugin.ConfigSchema,
					}).Error; err != nil {
					log.Printf("Failed to sync catalog copy for plugin %s: %v", plugin.Name, err)
				} else {
					log.Printf("Synced catalog copy for plugin: %s", plugin.DisplayName)
				}
			}
			log.Printf("Plugin %s already exists (ID: %d)", existingPlugin.DisplayName, existingPlugin.ID)
		}
	}

	// Initialize translations for all plugins
	if err := InitializePluginTranslations(); err != nil {
		log.Printf("Failed to initialize plugin translations: %v", err)
	}

	log.Println("Default plugins initialization completed")
	return nil
}

func updateComingSoonDefaultPlugin(pluginID uint, plugin database.Plugin) error {
	updates := map[string]interface{}{
		"display_name":  plugin.DisplayName,
		"description":   plugin.Description,
		"image":         plugin.Image,
		"is_active":     plugin.IsActive,
		"coming_soon":   plugin.ComingSoon,
		"category":      plugin.Category,
		"version":       plugin.Version,
		"features":      plugin.Features,
		"config_schema": plugin.ConfigSchema,
	}
	if plugin.Message != "" {
		updates["message"] = plugin.Message
	}

	return database.GetDB().
		Model(&database.Plugin{}).
		Where("id = ?", pluginID).
		Updates(updates).Error
}

func disableBusinessPluginSubscriptionsForComingSoonPlugin(pluginName string) error {
	return database.GetDB().Exec(`
		UPDATE business_plugins
		SET is_enabled = ?, updated_at = ?
		WHERE plugin_id IN (
			SELECT id FROM plugins WHERE name = ? AND coming_soon = ?
		)
	`, false, time.Now(), pluginName, true).Error
}

// pluginSeedTranslationsByLanguage maps a language code to that language's
// plugin translation table. Hoisted to package level so the honesty test can
// assert seed copy never claims gasless payments.
var pluginSeedTranslationsByLanguage = map[string]map[string]map[string]string{
	"es":    spanishTranslations,
	"es-AR": argentineSpanishTranslations,
}

// spanishTranslations holds the es plugin catalog copy.
var spanishTranslations = map[string]map[string]string{
	"usdc_payment": {
		"display_name": "Pago con USDC",
		"description":  "Opcional: deja que los clientes paguen en USDC. El dinero llega a tu billetera de liquidación cuando se confirma el pago.",
		"message":      "Rail cripto opcional. No es necesario para el servicio de cena — activa Mercado Pago u otros métodos de cobro primero.",
		"features":     `["Checkout cripto opcional","Se acredita en tu billetera de liquidación","El cliente paga la comisión de red","Valor estable en USDC","Actívalo solo si lo necesitás"]`,
	},
	"cross_chain_payment": {
		"display_name": "Pago con Cualquier Token",
		"description":  "Opcional: acepta otras criptomonedas y conviértelas a USDC para la liquidación",
		"message":      crossChainSeedMessage["es"],
		"features":     `["Checkout cripto opcional","Convierte a USDC","Varias redes","Avanzado / selectivo","No requerido para el servicio de cena"]`,
	},
	"stripe": {
		"display_name": "Stripe",
		"description":  "Acepta pagos con tarjeta de crédito a través de Stripe con protección avanzada contra fraudes y alcance global",
		"message":      "Procesa pagos con tarjeta de crédito de forma segura con Stripe. Acepta Visa, Mastercard, American Express y más con protección contra fraudes integrada.",
		"features":     `["Procesamiento de tarjetas de crédito","Protección avanzada contra fraudes","Alcance global","Múltiples métodos de pago","Pagos recurrentes","Facturación automática","Reportes detallados","API segura"]`,
	},
	"mercadopago": {
		"display_name": "MercadoPago",
		"description":  "Acepta pagos en América Latina con Checkout Pro, Point y QR",
		"message":      "Acepta tarjetas, dinero en cuenta y cuotas vía Checkout Pro; cobros presenciales con Point y QR.",
		"features":     `["Checkout Pro","Point (terminales)","QR dinámico","Tarjetas de débito/crédito","Dinero en cuenta","Cuotas","Cobertura LATAM"]`,
	},
	"paypal": {
		"display_name": "PayPal",
		"description":  "Acepta pagos de PayPal con protección al comprador y cobertura global",
		"message":      "Permite a tus clientes pagar con PayPal de forma rápida y segura. Incluye protección al comprador y soporte para múltiples monedas.",
		"features":     `["Pagos con PayPal","Protección al comprador","Múltiples monedas","Pagos express","Facturación recurrente","Pagos móviles","Cobertura global","Integración simple"]`,
	},
	"trustpilot": {
		"display_name": "Reseñas Trustpilot",
		"description":  "Recopila y muestra reseñas de clientes con integración simple del perfil comercial",
		"message":      "Construye confianza con reseñas auténticas de clientes. Invita automáticamente a clientes satisfechos a dejar reseñas en Trustpilot.",
		"features":     `["Invitaciones automáticas","Reseñas verificadas","Gestión de reputación","Respuestas a reseñas","Análisis de sentimientos","Integración del perfil","Widgets personalizables","Reportes de rendimiento"]`,
	},
	"telegram": {
		"display_name": "Notificaciones de Telegram",
		"description":  "Envía notificaciones en tiempo real a Telegram para pedidos, pagos y alertas comerciales",
		"message":      "Mantente conectado con tu negocio 24/7 con notificaciones instantáneas de Telegram. Recibe alertas de pedidos, confirmaciones de pago y resúmenes diarios.",
		"features":     `["Notificaciones de pedidos en tiempo real","Confirmaciones de pago","Resúmenes diarios/semanales/mensuales","Alertas de stock bajo","Notificaciones de nuevos clientes","Alertas de pedidos de alto valor","Insights de rendimiento comercial","Conexión segura del bot"]`,
	},
	"weekly_email_report": {
		"display_name": "Reporte Semanal por Email",
		"description":  "Recibe reportes completos de análisis de negocio semanales por correo electrónico",
		"message":      "Mantente informado con reportes semanales automáticos. Recibe resúmenes de ventas, artículos más vendidos, tendencias de ingresos y análisis de clientes directamente en tu bandeja de entrada.",
		"features":     `["Análisis semanal","Resumen de ventas","Artículos principales","Insights de clientes","Tendencias de ingresos","Horario personalizado"]`,
	},
	"daily_email_report": {
		"display_name": "Reporte Diario por Email",
		"description":  "Recibe resúmenes diarios de rendimiento del negocio entregados en tu bandeja de entrada",
		"message":      "Comienza cada día informado con reportes diarios automáticos. Obtén resúmenes de ventas, órdenes, pagos y métricas de rendimiento cada mañana.",
		"features":     `["Análisis diario","Resumen de ventas","Resumen de órdenes","Desglose de pagos","Métricas de rendimiento","Hora personalizada"]`,
	},
}

// argentineSpanishTranslations holds the es-AR plugin catalog copy.
var argentineSpanishTranslations = map[string]map[string]string{
	"usdc_payment": {
		"display_name": "Pago con USDC",
		"description":  "Opcional: dejá que los clientes paguen en USDC. El dinero llega a tu billetera de liquidación cuando se confirma el pago.",
		"message":      "Rail cripto opcional. No es necesario para el servicio de cena — activá Mercado Pago u otros métodos de cobro primero.",
		"features":     `["Checkout cripto opcional","Se acredita en tu billetera de liquidación","El cliente paga la comisión de red","Valor estable en USDC","Activalo solo si lo necesitás"]`,
	},
	"cross_chain_payment": {
		"display_name": "Pago con cualquier token",
		"description":  "Opcional: aceptá otras criptomonedas y convertílas a USDC para la liquidación",
		"message":      crossChainSeedMessage["es-AR"],
		"features":     `["Checkout cripto opcional","Convierte a USDC","Varias redes","Avanzado / selectivo","No requerido para el servicio de cena"]`,
	},
	"stripe": {
		"display_name": "Stripe",
		"description":  "Aceptá pagos con tarjeta de crédito a través de Stripe con protección avanzada contra fraude y alcance global",
		"message":      "Procesá pagos con tarjeta de crédito de forma segura con Stripe. Aceptá Visa, Mastercard, American Express y más, con protección contra fraude integrada.",
		"features":     `["Procesamiento de tarjetas de crédito","Protección avanzada contra fraude","Alcance global","Múltiples métodos de pago","Pagos recurrentes","Facturación automática","Reportes detallados","API segura"]`,
	},
	"mercadopago": {
		"display_name": "MercadoPago",
		"description":  "Aceptá pagos en América Latina con Checkout Pro, Point y QR",
		"message":      "Aceptá tarjetas, dinero en cuenta y cuotas vía Checkout Pro; cobros presenciales con Point y QR.",
		"features":     `["Checkout Pro","Point (terminales)","QR dinámico","Tarjetas de débito/crédito","Dinero en cuenta","Cuotas","Cobertura LATAM"]`,
	},
	"paypal": {
		"display_name": "PayPal",
		"description":  "Aceptá pagos con PayPal con protección al comprador y cobertura global",
		"message":      "Permití que tus clientes paguen con PayPal de forma rápida y segura. Incluye protección al comprador y soporte para múltiples monedas.",
		"features":     `["Pagos con PayPal","Protección al comprador","Múltiples monedas","Checkout express","Facturación recurrente","Pagos móviles","Cobertura global","Integración simple"]`,
	},
	"trustpilot": {
		"display_name": "Reseñas Trustpilot",
		"description":  "Recolectá y mostrá reseñas de clientes con una integración simple del perfil comercial",
		"message":      "Construí confianza con reseñas auténticas de clientes. Invitá automáticamente a clientes satisfechos a dejar reseñas en Trustpilot.",
		"features":     `["Invitaciones automáticas","Reseñas verificadas","Gestión de reputación","Respuestas a reseñas","Análisis de sentimiento","Integración del perfil","Widgets personalizables","Reportes de rendimiento"]`,
	},
	"telegram": {
		"display_name": "Notificaciones de Telegram",
		"description":  "Enviá notificaciones en tiempo real a Telegram para pedidos, pagos y alertas comerciales",
		"message":      "Mantenete conectado con tu negocio 24/7 con notificaciones instantáneas de Telegram. Recibí alertas de pedidos, confirmaciones de pago y resúmenes diarios.",
		"features":     `["Notificaciones de pedidos en tiempo real","Confirmaciones de pago","Resúmenes diarios/semanales/mensuales","Alertas de stock bajo","Notificaciones de nuevos clientes","Alertas de pedidos de alto valor","Insights de rendimiento comercial","Conexión segura del bot"]`,
	},
	"weekly_email_report": {
		"display_name": "Reporte semanal por email",
		"description":  "Recibí reportes semanales completos de analítica del negocio por email",
		"message":      "Mantenete informado con reportes semanales automáticos. Recibí resúmenes de ventas, productos más vendidos, tendencias de ingresos y análisis de clientes directo en tu bandeja de entrada.",
		"features":     `["Analítica semanal","Resumen de ventas","Productos principales","Insights de clientes","Tendencias de ingresos","Horario personalizado"]`,
	},
	"daily_email_report": {
		"display_name": "Reporte diario por email",
		"description":  "Recibí resúmenes diarios del rendimiento del negocio en tu bandeja de entrada",
		"message":      "Arrancá cada día informado con reportes diarios automáticos. Obtené resúmenes de ventas, órdenes, pagos y métricas de rendimiento cada mañana.",
		"features":     `["Analítica diaria","Resumen de ventas","Resumen de órdenes","Desglose de pagos","Métricas de rendimiento","Hora personalizada"]`,
	},
}

// InitializePluginTranslations creates localized translations for all plugins.
func InitializePluginTranslations() error {
	log.Println("Initializing plugin translations...")

	translationsByLanguage := pluginSeedTranslationsByLanguage

	// Get all plugins and create translations
	plugins, err := database.GetAllPlugins(false)
	if err != nil {
		return err
	}

	for _, plugin := range plugins {
		for languageCode, translations := range translationsByLanguage {
			if pluginTranslations, exists := translations[plugin.Name]; exists {
				for fieldName, content := range pluginTranslations {
					err := database.CreateOrUpdatePluginTranslation(plugin.ID, languageCode, fieldName, content)
					if err != nil {
						log.Printf("Failed to create translation for plugin %s, language %s, field %s: %v", plugin.Name, languageCode, fieldName, err)
						continue
					}
				}
				log.Printf("Created %s translations for plugin: %s", languageCode, plugin.DisplayName)
			}
		}
	}

	log.Println("Plugin translations initialization completed")
	return nil
}
