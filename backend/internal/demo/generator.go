package demo

import (
	"context"
	cryptorand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// predictableDemoTableCode matches legacy enumerable demo codes that must not
// ship for guest access (#293): demo-{bizId}-{profile}-table-0N, CORE-T0N, AI-T0N.
var predictableDemoTableCode = regexp.MustCompile(`(?i)^(demo-\d+-[a-z0-9-]+-table-\d+|core-t\d+|ai-t\d+)$`)

// guessableDemoDeliveryNumber matches leaked public track ids (#530):
//
//	DEMO-PAY-<businessID>
//	DEL-B<businessID>-<12hex>  (day_generator used DEL-<bill_number>)
var guessableDemoDeliveryNumber = regexp.MustCompile(`^(?:DEMO-PAY-[0-9]+|DEL-B[0-9]+-[0-9a-fA-F]{12})$`)

// menuImages maps carta item ids to our own-hosted food photography, as paths
// relative to seedAssetPrefix (embedded in the binary, self-healed into the
// public store by EnsureSeedAssets). Resolve with menuImage(id).
var menuImages = map[string]string{
	"demo-provoleta":   "carta/provoleta.jpg",
	"demo-empanadas":   "carta/empanadas.jpg",
	"demo-molleja":     "carta/mollejas.jpg",
	"demo-choripan":    "carta/choripan.jpg",
	"demo-bife":        "carta/bife-de-chorizo.jpg",
	"demo-ojo-de-bife": "carta/ojo-de-bife.jpg",
	"demo-asado-tira":  "carta/asado-de-tira.jpg",
	"demo-entrana":     "carta/entrana.jpg",
	"demo-matambrito":  "carta/matambrito.jpg",
	"demo-parrillada":  "carta/parrillada.jpg",
	"demo-milanesa":    "carta/milanesa-napolitana.jpg",
	"demo-sorrentinos": "carta/sorrentinos.jpg",
	"demo-flan":        "carta/flan-casero.jpg",
	"demo-malbec-copa": "carta/malbec.jpg",
}

// promoImages holds marketing tiles that are not dish photos — offer artwork
// rendered with the discount copy baked in. Same hosting contract as
// menuImages. Resolve with promoImage(id).
var promoImages = map[string]string{
	"almuerzo-15": "promos/offer-almuerzo-15.jpg",
}

// menuImage returns the public URL of a carta item's demo photo ("" if none).
func menuImage(id string) string { return demoAssetURLFor(menuImages, id) }

// promoImage returns the public URL of a promo tile ("" if none).
func promoImage(id string) string { return demoAssetURLFor(promoImages, id) }

func demoAssetURLFor(paths map[string]string, id string) string {
	rel, ok := paths[id]
	if !ok {
		return ""
	}
	return demoAssetURL(rel)
}

func (s *Service) ensureStaticBusinesses(ctx context.Context, tx *gorm.DB, instance *database.DemoInstance) error {
	for _, p := range profiles() {
		business, err := s.ensureBusiness(ctx, tx, instance.AdminUserID, p)
		if err != nil {
			return err
		}
		switch p.Key {
		case "primary":
			instance.PrimaryBusinessID = &business.ID
		case "secondary":
			instance.SecondaryBusinessID = &business.ID
		}
		if err := s.ensureStaticData(ctx, tx, instance.AdminUserID, business, p); err != nil {
			return err
		}
	}
	return tx.Omit(clause.Associations).Save(instance).Error
}

// demoVenueIdentity carries the per-venue Argentine identity (owner, address,
// story) so the two showroom businesses read as distinct restaurants. The
// contact details are fictional (see fake_contact.go): real barrio and postal
// code, a house number past the end of the street, a non-dialable phone.
type demoVenueIdentity struct {
	Owner       string
	Street      string
	PostalCode  string
	Phone       string
	Description string
	AboutStory  string
}

func venueIdentity(p profile) demoVenueIdentity {
	if p.Key == "secondary" {
		return demoVenueIdentity{
			Owner:       "Ernesto Villalba",
			Street:      demoFakeStreet("Costa Rica", 602) + ", Palermo",
			PostalCode:  "C1414",
			Phone:       demoFakePhoneDisplay(5602),
			Description: "Parrilla de barrio elevada en Palermo: cortes de novillo madurados, fuego de leña y carta de Malbec por bodega.",
			AboutStory:  "La familia Villalba reabrió la parrilla del abuelo en Palermo con la misma receta de chimichurri de 1974. Cortes madurados a la vista, fuego de leña de quebracho y una carta de Malbec elegida bodega por bodega.",
		}
	}
	return demoVenueIdentity{
		Owner:       "Rosa Beltrán",
		Street:      demoFakeStreet("Defensa", 148) + ", San Telmo",
		PostalCode:  "C1065",
		Phone:       demoFakePhoneDisplay(148),
		Description: "Bodegón familiar de San Telmo desde 1962: milanesas, sorrentinos caseros y flan con dulce de leche.",
		AboutStory:  "Desde 1962, tres generaciones de la familia Beltrán atienden la misma esquina de San Telmo. Rosa abrió con seis mesas y una sola mesa larga para el barrio; el salón conserva la vajilla enlozada, las fotos del barrio y el mostrador de madera original.",
	}
}

func (s *Service) ensureBusiness(ctx context.Context, tx *gorm.DB, adminUserID uint, p profile) (*database.Business, error) {
	now := s.now().UTC()
	identity := venueIdentity(p)
	userID := adminUserID
	if s.showroomOwner != 0 {
		userID = s.showroomOwner
	}
	// BusinessId remains an internal stable key (not public). Public CustomURL
	// is derived from the display name only — never admin user id / plan tier.
	businessID := fmt.Sprintf("demo-admin-%d-%s", adminUserID, p.BusinessIDSuffix)

	var existing database.Business
	excludeID := uint(0)
	if err := tx.WithContext(ctx).Where("business_id = ?", businessID).First(&existing).Error; err == nil {
		excludeID = existing.ID
	}

	customURL, err := database.AllocatePublicCustomURL(ctx, tx, p.Name, excludeID)
	if err != nil {
		return nil, err
	}

	business := database.Business{
		BusinessId:   businessID,
		OwnerAddress: demoWallet,
		UserID:       &userID,
		OwnerName:    identity.Owner,
		Name:         p.Name,
		// Leave Logo empty so Settings / sidebar show a neutral initials
		// placeholder. Using the hero stock photo as a logo reads as a real
		// uploaded brand mark (Task 35 / settings imagery).
		Logo:           "",
		Address:        database.BusinessAddress{Street: identity.Street, City: "Buenos Aires", State: "CABA", PostalCode: identity.PostalCode, Country: "AR"},
		SettlementAddr: "",
		// No settlement wallet and no enabled crypto rail means no tip can
		// ever land on-chain either, and the guest check renders whatever is
		// here as its tipping_address (#856). Blank omits the field.
		TippingAddr: "",
		// Carta prices are IVA-final (consumidor final) — no added tax line.
		// tax_inclusive says so on the check; tax_rate 0 alone reads as "this
		// venue charges no IVA at all" on the payment settings screen (#936).
		TaxRate:             0,
		TaxInclusive:        true,
		ServiceFeeRate:      p.ServiceFeeRate,
		IsActive:            true,
		Description:         identity.Description,
		CustomURL:           customURL,
		Phone:               identity.Phone,
		Email:               s.demoEmail(fmt.Sprintf("demo+admin%d-%s", adminUserID, p.Key)),
		Website:             demoFakeWebsite(p),
		SocialMedia:         demoNoSocialMedia,
		BannerImages:        fmt.Sprintf(`["%s","%s"]`, p.Hero, menuImage("demo-parrillada")),
		BusinessPageEnabled: true,
		ShowReviews:         true,
		BusinessType:        "restaurant",
		CounterEnabled:      true,
		CounterCount:        2,
		CounterPrefix:       "Barra",
		KitchenEnabled:      true,
		OrdersEnabled:       true,
		CRMEnabled:          true,
		DefaultCurrency:     "ARS",
		DisplayCurrency:     "ARS",
		DefaultLanguage:     "es",
		SourceLanguage:      "es",
		Timezone:            defaultTimezone,
		DesignSettings: database.BusinessDesignSettings{
			PrimaryColor: p.Accent,
			// Secondary must clear WCAG AA as body/accent text on white
			// (frontend/src/lib/contrast.ts SHIPPED_BRAND_PRESETS). #f59e0b
			// fails validateSecondaryBrandColor; warmBrown secondary passes.
			// Ideal long-term: one shared constant list between Go seed and TS.
			SecondaryColor:    "#b45309",
			FontFamily:        "Inter",
			Theme:             "light",
			MenuLayout:        "grid",
			ShowImages:        true,
			ShowDescriptions:  true,
			HeaderStyle:       "banner",
			CornerRadius:      "medium",
			ShadowIntensity:   "subtle",
			BackgroundPattern: "none",
			HeroLayout:        "immersive",
			SectionDensity:    "comfortable",
		},
		AiSettings: database.BusinessAiSettings{
			AiEnabled:             p.AIEnabled,
			AiName:                "Mozo",
			AiPriority:            "balanced",
			SpecialInstructions:   "Recommend high-margin dishes, respect allergies, never claim allergen-free without staff confirmation, never push 86'd or out-of-stock dishes, and escalate payment issues to staff.",
			BusinessPageAiEnabled: p.AIEnabled,
		},
		// Hours-honest: welcome must not claim the dining room is actively
		// serving when operating hours may mark the venue CLOSED on the storefront.
		WelcomeMessage:        "Bienvenidos. La carta está disponible a toda hora — los horarios de esta página indican cuándo tomamos pedidos.",
		AboutStory:            identity.AboutStory,
		ShowWelcomeMessage:    true,
		ShowAboutStory:        true,
		ShowGallery:           true,
		ShowOperatingHours:    true,
		ShowSpecialFeatures:   true,
		Kind:                  database.BusinessKindDemo,
		IsDemo:                true,
		DemoOwnerUserID:       &adminUserID,
		OnboardingState:       database.JSONRawMessage(`{"demo":true,"completed":true}`),
		OnboardingCompletedAt: &now,
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "business_id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"user_id":            userID,
			"is_demo":            true,
			"kind":               string(database.BusinessKindDemo),
			"demo_owner_user_id": adminUserID,
			"owner_name":         identity.Owner,
			// #823: legacy demo rows kept a pre-refresh address ("16 Demo
			// Market St") forever, so the /b storefront and /t table home —
			// which both render these same embedded columns — disagreed
			// across venues. Repair the canonical address on every ensure.
			"street":      business.Address.Street,
			"city":        business.Address.City,
			"state":       business.Address.State,
			"postal_code": business.Address.PostalCode,
			"country":     business.Address.Country,
			// Rewrite legacy demo-admin-{id}-* public slugs to display-name form.
			"custom_url":                  customURL,
			"name":                        p.Name,
			"business_page_enabled":       true,
			"kitchen_enabled":             true,
			"orders_enabled":              true,
			"crm_enabled":                 true,
			"ai_ai_enabled":               p.AIEnabled,
			"ai_business_page_ai_enabled": p.AIEnabled,
			"onboarding_completed_at":     now,
			"settlement_addr":             "",
			"tipping_addr":                "",
			// #936: settings that only ever got their GORM default on rows
			// seeded by an older generator.
			"tax_rate":       0,
			"tax_inclusive":  true,
			"counter_prefix": business.CounterPrefix,
			"updated_at":     now,
		}),
	}).Create(&business).Error; err != nil {
		return nil, err
	}
	var out database.Business
	if err := tx.WithContext(ctx).Where("business_id = ?", businessID).First(&out).Error; err != nil {
		return nil, err
	}
	return &out, nil
}

func (s *Service) ensureStaticData(ctx context.Context, tx *gorm.DB, adminUserID uint, business *database.Business, p profile) error {
	if err := s.ensureMenu(ctx, tx, business.ID); err != nil {
		return err
	}
	// FIND-064: guest Spanish demos need business languages + position-keyed
	// translations (entity_id = categoryIndex*1000 + itemIndex).
	if err := s.ensureDemoLanguagesAndTranslations(ctx, tx, business.ID); err != nil {
		return err
	}
	if err := s.ensureBusinessPageData(ctx, tx, business.ID, p); err != nil {
		return err
	}
	if err := s.ensureTablesAndCounters(ctx, tx, business.ID, p); err != nil {
		return err
	}
	staff, positions, err := s.ensureStaffAndSchedule(ctx, tx, adminUserID, business.ID)
	if err != nil {
		return err
	}
	if err := s.ensurePendingAlternativePaymentBill(ctx, tx, business.ID, p, staff); err != nil {
		return err
	}
	if err := s.ensureInventory(ctx, tx, business.ID); err != nil {
		return err
	}
	if err := s.ensureReservations(ctx, tx, business); err != nil {
		return err
	}
	if err := s.ensureDelivery(ctx, tx, business.ID, staff); err != nil {
		return err
	}
	if err := s.ensureAwaitingPaymentDelivery(ctx, tx, business.ID, p, staff); err != nil {
		return err
	}
	if err := s.ensureChatAndEngagement(ctx, tx, business.ID, staff, positions); err != nil {
		return err
	}
	if err := s.ensureAccountingPayrollLoyaltyAndPlugins(ctx, tx, adminUserID, business.ID, staff); err != nil {
		return err
	}
	if err := s.ensureScheduleWorkflowData(ctx, tx, business.ID, staff, positions); err != nil {
		return err
	}
	if err := s.ensureFiscalPrinterAIAndAlerts(ctx, tx, adminUserID, business.ID, staff, p); err != nil {
		return err
	}
	// Runs last: the floor must not read occupied for tables whose checks are
	// already closed, no matter who left the ticket behind (#904).
	if err := s.retireGhostFloorTickets(ctx, tx, business.ID); err != nil {
		return err
	}
	return s.sanitizeDemoCryptoSettlement(ctx, tx, business.ID)
}

func (s *Service) ensureMenu(ctx context.Context, tx *gorm.DB, businessID uint) error {
	// Allergens are intentional demo data for AI Waiter safety demos (FIND-061).
	// When empty, allergen intent correctly fails closed to staff — but marketing
	// promises allergen answers grounded in menu data, so demos must ship labels.
	// Carta porteña — prices are ARS pesos (float64 wire shape), IVA-final.
	// Menu-engineering roles the sales demo narrates on the analysis screens:
	// demo-bife = star, demo-ojo-de-bife = puzzle (high margin, sells slowly),
	// demo-milanesa = plowhorse, demo-ensalada = dog.
	categories := []database.MenuCategory{
		{ID: "para-picar", Name: "Para picar", Description: "Entradas para compartir mientras se prende el fuego", SortOrder: 1, Items: []database.MenuItem{
			{ID: "demo-provoleta", Name: "Provoleta", Description: "Provolone fundido a la parrilla con orégano y ají molido", Price: 9800, Currency: "ARS", Image: menuImage("demo-provoleta"), Images: []string{menuImage("demo-provoleta")}, DietaryTags: []string{"vegetarian"}, Allergens: []string{"dairy"}, IsAvailable: true, SortOrder: 1},
			{ID: "demo-empanadas", Name: "Empanada de carne", Description: "Carne cortada a cuchillo, receta salteña, horneada al momento", Price: 2900, Currency: "ARS", Image: menuImage("demo-empanadas"), Images: []string{menuImage("demo-empanadas")}, Allergens: []string{"gluten", "eggs"}, IsAvailable: true, SortOrder: 2},
			{ID: "demo-molleja", Name: "Mollejas al limón", Description: "Mollejas doradas al fierro, terminadas con limón y sal gruesa", Price: 16500, Currency: "ARS", Image: menuImage("demo-molleja"), Images: []string{menuImage("demo-molleja")}, Allergens: []string{}, IsAvailable: true, SortOrder: 3},
			{ID: "demo-choripan", Name: "Choripán", Description: "Chorizo criollo en pan crocante con chimichurri de la casa", Price: 7500, Currency: "ARS", Image: menuImage("demo-choripan"), Images: []string{menuImage("demo-choripan")}, Allergens: []string{"gluten"}, IsAvailable: true, SortOrder: 4},
		}},
		{ID: "parrilla", Name: "De la parrilla", Description: "Cortes a las brasas de quebracho, punto a elección", SortOrder: 2, Items: []database.MenuItem{
			{ID: "demo-bife", Name: "Bife de chorizo", Description: "Corte alto de novillo, madurado 21 días, sellado a fuego fuerte", Price: 34000, Currency: "ARS", Image: menuImage("demo-bife"), Images: []string{menuImage("demo-bife")}, Allergens: []string{}, IsAvailable: true, SortOrder: 1},
			{ID: "demo-ojo-de-bife", Name: "Ojo de bife", Description: "Ojo de bife de 400 g, marmoleado premium, con hueso", Price: 39500, Currency: "ARS", Image: menuImage("demo-ojo-de-bife"), Images: []string{menuImage("demo-ojo-de-bife")}, Allergens: []string{}, IsAvailable: true, SortOrder: 2},
			{ID: "demo-asado-tira", Name: "Asado de tira", Description: "Tira banderita de cocción lenta, crocante por fuera", Price: 29800, Currency: "ARS", Image: menuImage("demo-asado-tira"), Images: []string{menuImage("demo-asado-tira")}, Allergens: []string{}, IsAvailable: true, SortOrder: 3},
			{ID: "demo-entrana", Name: "Entraña", Description: "Entraña jugosa con chimichurri y sal parrillera", Price: 31500, Currency: "ARS", Image: menuImage("demo-entrana"), Images: []string{menuImage("demo-entrana")}, Allergens: []string{}, IsAvailable: true, SortOrder: 4},
			{ID: "demo-parrillada", Name: "Parrillada para dos", Description: "Selección del parrillero: bife, chorizo, morcilla, mollejas y provoleta", Price: 68000, Currency: "ARS", Image: menuImage("demo-parrillada"), Images: []string{menuImage("demo-parrillada")}, Allergens: []string{"dairy"}, IsAvailable: true, SortOrder: 5},
			{ID: "demo-matambrito", Name: "Matambrito de cerdo", Description: "Matambrito tierno al limón, dorado a la parrilla", Price: 26500, Currency: "ARS", Image: menuImage("demo-matambrito"), Images: []string{menuImage("demo-matambrito")}, Allergens: []string{}, IsAvailable: true, SortOrder: 6},
		}},
		{ID: "bodegon", Name: "Platos de bodegón", Description: "Los clásicos de siempre, en porción abundante", SortOrder: 3, Items: []database.MenuItem{
			{ID: "demo-milanesa", Name: "Milanesa napolitana", Description: "Milanesa de nalga con jamón, muzzarella y salsa de tomate, con fritas", Price: 21500, Currency: "ARS", Image: menuImage("demo-milanesa"), Images: []string{menuImage("demo-milanesa")}, Allergens: []string{"gluten", "dairy", "eggs"}, IsAvailable: true, SortOrder: 1},
			{ID: "demo-sorrentinos", Name: "Sorrentinos caseros", Description: "Sorrentinos de jamón y muzzarella con salsa rosa", Price: 19800, Currency: "ARS", Image: menuImage("demo-sorrentinos"), Images: []string{menuImage("demo-sorrentinos")}, Allergens: []string{"gluten", "dairy", "eggs"}, IsAvailable: true, SortOrder: 2},
		}},
		{ID: "guarniciones", Name: "Guarniciones y ensaladas", Description: "Para acompañar los cortes", SortOrder: 4, Items: []database.MenuItem{
			{ID: "demo-ensalada", Name: "Ensalada mixta", Description: "Lechuga, tomate y cebolla con aceite de oliva", Price: 12500, Currency: "ARS", DietaryTags: []string{"vegan"}, Allergens: []string{}, IsAvailable: true, SortOrder: 1},
			{ID: "demo-fritas", Name: "Papas fritas", Description: "Papas caseras, doradas en tandas chicas", Price: 8900, Currency: "ARS", DietaryTags: []string{"vegan"}, Allergens: []string{}, IsAvailable: true, SortOrder: 2},
			{ID: "demo-pure", Name: "Puré de papa", Description: "Puré cremoso con manteca", Price: 7800, Currency: "ARS", DietaryTags: []string{"vegetarian"}, Allergens: []string{"dairy"}, IsAvailable: true, SortOrder: 3},
		}},
		{ID: "postres", Name: "Postres", Description: "Dulces de la casa", SortOrder: 5, Items: []database.MenuItem{
			{ID: "demo-flan", Name: "Flan casero", Description: "Flan de huevo con dulce de leche y crema", Price: 8900, Currency: "ARS", Image: menuImage("demo-flan"), Images: []string{menuImage("demo-flan")}, DietaryTags: []string{"vegetarian"}, Allergens: []string{"dairy", "eggs"}, IsAvailable: true, SortOrder: 1},
			{ID: "demo-panqueques", Name: "Panqueques con dulce de leche", Description: "Panqueques finitos rellenos de dulce de leche", Price: 9800, Currency: "ARS", DietaryTags: []string{"vegetarian"}, Allergens: []string{"gluten", "dairy", "eggs"}, IsAvailable: true, SortOrder: 2},
			{ID: "demo-vigilante", Name: "Queso y dulce", Description: "El vigilante: queso fresco con dulce de membrillo", Price: 7900, Currency: "ARS", DietaryTags: []string{"vegetarian"}, Allergens: []string{"dairy"}, IsAvailable: true, SortOrder: 3},
		}},
		{ID: "bodega", Name: "Bodega y barra", Description: "Vinos mendocinos y clásicos de barra", SortOrder: 6, Items: []database.MenuItem{
			{ID: "demo-malbec-copa", Name: "Copa de Malbec", Description: "Malbec mendocino de bodega boutique, por copa", Price: 6500, Currency: "ARS", Image: menuImage("demo-malbec-copa"), Images: []string{menuImage("demo-malbec-copa")}, DietaryTags: []string{"vegan"}, Allergens: []string{"so2"}, IsAvailable: true, SortOrder: 1},
			{ID: "demo-malbec-botella", Name: "Malbec (botella)", Description: "Botella de Malbec Valle de Uco, cosecha 2022", Price: 28500, Currency: "ARS", DietaryTags: []string{"vegan"}, Allergens: []string{"so2"}, IsAvailable: true, SortOrder: 2},
			{ID: "demo-fernet", Name: "Fernet con coca", Description: "El clásico argentino, servido con mucho hielo", Price: 7800, Currency: "ARS", Allergens: []string{}, IsAvailable: true, SortOrder: 3},
			{ID: "demo-agua", Name: "Agua sin gas", Description: "Botella de 500 ml", Price: 3500, Currency: "ARS", DietaryTags: []string{"vegan"}, Allergens: []string{}, IsAvailable: true, SortOrder: 4},
			{ID: "demo-cafe", Name: "Café", Description: "Espresso de especialidad, torrado en Buenos Aires", Price: 3800, Currency: "ARS", Allergens: []string{}, IsAvailable: true, SortOrder: 5},
		}},
	}
	raw, err := json.Marshal(categories)
	if err != nil {
		return err
	}
	var menu database.Menu
	err = tx.WithContext(ctx).Where("business_id = ?", businessID).First(&menu).Error
	switch {
	case err == nil:
		return tx.WithContext(ctx).Model(&menu).Updates(map[string]interface{}{"categories": string(raw), "is_active": true}).Error
	case errors.Is(err, gorm.ErrRecordNotFound):
		return tx.WithContext(ctx).Create(&database.Menu{BusinessID: businessID, Categories: string(raw), IsActive: true, Version: 1}).Error
	default:
		return err
	}
}

// ensureDemoLanguagesAndTranslations enables Spanish (default) + English on the
// demo business and upserts English names/descriptions for every demo menu
// position so GET /guest/table/:code/menu?language=en returns translated copy
// for tourists while the carta itself stays porteña.
// Position keys match applyTranslationsToMenu / translateMenuIntoLanguages.
func (s *Service) ensureDemoLanguagesAndTranslations(ctx context.Context, tx *gorm.DB, businessID uint) error {
	// Business languages (idempotent upsert by business+code).
	type langRow struct {
		code  string
		def   bool
		order int
	}
	for _, l := range []langRow{{"es", true, 0}, {"en", false, 1}} {
		var existing database.BusinessLanguage
		err := tx.WithContext(ctx).
			Where("business_id = ? AND language_code = ?", businessID, l.code).
			First(&existing).Error
		switch {
		case err == nil:
			// keep row; heal the default flag toward the es-first carta
			if existing.IsDefault != l.def {
				if err := tx.WithContext(ctx).Model(&existing).Update("is_default", l.def).Error; err != nil {
					return err
				}
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			if err := tx.WithContext(ctx).Create(&database.BusinessLanguage{
				BusinessID:   businessID,
				LanguageCode: l.code,
				IsDefault:    l.def,
				DisplayOrder: l.order,
			}).Error; err != nil {
				return err
			}
		default:
			return err
		}
	}

	// English strings for position-based entity IDs (catIndex*1000 + itemIndex).
	// Source copy is the Spanish carta; en is the translated guest tier.
	type trSpec struct {
		entityType string
		entityID   uint
		field      string
		original   string
		english    string
	}
	specs := []trSpec{
		{"category", 0, "name", "Para picar", "Starters to share"},
		{"category", 0, "description", "Entradas para compartir mientras se prende el fuego", "Small plates to share while the fire gets going"},
		{"menu_item", 0, "name", "Provoleta", "Grilled provoleta cheese"},
		{"menu_item", 0, "description", "Provolone fundido a la parrilla con orégano y ají molido", "Provolone melted on the grill with oregano and crushed chili"},
		{"menu_item", 1, "name", "Empanada de carne", "Beef empanada"},
		{"menu_item", 1, "description", "Carne cortada a cuchillo, receta salteña, horneada al momento", "Hand-cut beef, Salta-style recipe, baked to order"},
		{"menu_item", 2, "name", "Mollejas al limón", "Sweetbreads with lemon"},
		{"menu_item", 2, "description", "Mollejas doradas al fierro, terminadas con limón y sal gruesa", "Crisped sweetbreads finished with lemon and coarse salt"},
		{"menu_item", 3, "name", "Choripán", "Choripán sausage sandwich"},
		{"menu_item", 3, "description", "Chorizo criollo en pan crocante con chimichurri de la casa", "Criollo sausage on crusty bread with house chimichurri"},
		{"category", 1, "name", "De la parrilla", "From the grill"},
		{"category", 1, "description", "Cortes a las brasas de quebracho, punto a elección", "Cuts over quebracho embers, cooked to your liking"},
		{"menu_item", 1000, "name", "Bife de chorizo", "Sirloin strip steak"},
		{"menu_item", 1000, "description", "Corte alto de novillo, madurado 21 días, sellado a fuego fuerte", "Thick-cut steer sirloin, dry-aged 21 days, seared over high heat"},
		{"menu_item", 1001, "name", "Ojo de bife", "Ribeye"},
		{"menu_item", 1001, "description", "Ojo de bife de 400 g, marmoleado premium, con hueso", "400 g bone-in ribeye with premium marbling"},
		{"menu_item", 1002, "name", "Asado de tira", "Short ribs"},
		{"menu_item", 1002, "description", "Tira banderita de cocción lenta, crocante por fuera", "Slow-cooked flanken-cut short ribs, crisp outside"},
		{"menu_item", 1003, "name", "Entraña", "Skirt steak"},
		{"menu_item", 1003, "description", "Entraña jugosa con chimichurri y sal parrillera", "Juicy skirt steak with chimichurri and grill salt"},
		{"menu_item", 1004, "name", "Parrillada para dos", "Mixed grill for two"},
		{"menu_item", 1004, "description", "Selección del parrillero: bife, chorizo, morcilla, mollejas y provoleta", "Grill master's selection: steak, chorizo, blood sausage, sweetbreads and provoleta"},
		{"menu_item", 1005, "name", "Matambrito de cerdo", "Pork matambre"},
		{"menu_item", 1005, "description", "Matambrito tierno al limón, dorado a la parrilla", "Tender pork flank with lemon, browned on the grill"},
		{"category", 2, "name", "Platos de bodegón", "Bodegón classics"},
		{"category", 2, "description", "Los clásicos de siempre, en porción abundante", "The timeless classics, generously portioned"},
		{"menu_item", 2000, "name", "Milanesa napolitana", "Milanesa napolitana"},
		{"menu_item", 2000, "description", "Milanesa de nalga con jamón, muzzarella y salsa de tomate, con fritas", "Breaded beef cutlet with ham, mozzarella and tomato sauce, served with fries"},
		{"menu_item", 2001, "name", "Sorrentinos caseros", "Homemade sorrentinos"},
		{"menu_item", 2001, "description", "Sorrentinos de jamón y muzzarella con salsa rosa", "Ham and mozzarella stuffed pasta rounds in rosa sauce"},
		{"category", 3, "name", "Guarniciones y ensaladas", "Sides and salads"},
		{"category", 3, "description", "Para acompañar los cortes", "To go with the cuts"},
		{"menu_item", 3000, "name", "Ensalada mixta", "Mixed salad"},
		{"menu_item", 3000, "description", "Lechuga, tomate y cebolla con aceite de oliva", "Lettuce, tomato and onion with olive oil"},
		{"menu_item", 3001, "name", "Papas fritas", "French fries"},
		{"menu_item", 3001, "description", "Papas caseras, doradas en tandas chicas", "House-cut potatoes, fried in small batches"},
		{"menu_item", 3002, "name", "Puré de papa", "Mashed potatoes"},
		{"menu_item", 3002, "description", "Puré cremoso con manteca", "Creamy mash with butter"},
		{"category", 4, "name", "Postres", "Desserts"},
		{"category", 4, "description", "Dulces de la casa", "House desserts"},
		{"menu_item", 4000, "name", "Flan casero", "Homemade flan"},
		{"menu_item", 4000, "description", "Flan de huevo con dulce de leche y crema", "Egg custard flan with dulce de leche and cream"},
		{"menu_item", 4001, "name", "Panqueques con dulce de leche", "Dulce de leche crêpes"},
		{"menu_item", 4001, "description", "Panqueques finitos rellenos de dulce de leche", "Thin crêpes filled with dulce de leche"},
		{"menu_item", 4002, "name", "Queso y dulce", "Cheese and quince"},
		{"menu_item", 4002, "description", "El vigilante: queso fresco con dulce de membrillo", "The classic vigilante: fresh cheese with quince paste"},
		{"category", 5, "name", "Bodega y barra", "Wine and bar"},
		{"category", 5, "description", "Vinos mendocinos y clásicos de barra", "Mendoza wines and bar classics"},
		{"menu_item", 5000, "name", "Copa de Malbec", "Malbec by the glass"},
		{"menu_item", 5000, "description", "Malbec mendocino de bodega boutique, por copa", "Boutique Mendoza Malbec, by the glass"},
		{"menu_item", 5001, "name", "Malbec (botella)", "Malbec (bottle)"},
		{"menu_item", 5001, "description", "Botella de Malbec Valle de Uco, cosecha 2022", "Bottle of Valle de Uco Malbec, 2022 vintage"},
		{"menu_item", 5002, "name", "Fernet con coca", "Fernet and cola"},
		{"menu_item", 5002, "description", "El clásico argentino, servido con mucho hielo", "The Argentine classic, served over plenty of ice"},
		{"menu_item", 5003, "name", "Agua sin gas", "Still water"},
		{"menu_item", 5003, "description", "Botella de 500 ml", "500 ml bottle"},
		{"menu_item", 5004, "name", "Café", "Coffee"},
		{"menu_item", 5004, "description", "Espresso de especialidad, torrado en Buenos Aires", "Specialty espresso, roasted in Buenos Aires"},
	}

	for _, spec := range specs {
		var row database.Translation
		err := tx.WithContext(ctx).
			Where("business_id = ? AND entity_type = ? AND entity_id = ? AND field_name = ? AND language_code = ?",
				businessID, spec.entityType, spec.entityID, spec.field, "en").
			First(&row).Error
		switch {
		case err == nil:
			if row.TranslatedText != spec.english || row.OriginalText != spec.original {
				if err := tx.WithContext(ctx).Model(&row).Updates(map[string]interface{}{
					"translated_text":    spec.english,
					"original_text":      spec.original,
					"is_auto_translated": false,
					"translation_source": "demo_seed",
				}).Error; err != nil {
					return err
				}
			}
		case errors.Is(err, gorm.ErrRecordNotFound):
			if err := tx.WithContext(ctx).Create(&database.Translation{
				BusinessID:        businessID,
				EntityType:        spec.entityType,
				EntityID:          spec.entityID,
				FieldName:         spec.field,
				LanguageCode:      "en",
				OriginalText:      spec.original,
				TranslatedText:    spec.english,
				IsAutoTranslated:  false,
				TranslationSource: "demo_seed",
			}).Error; err != nil {
				return err
			}
		default:
			return err
		}
	}
	return nil
}

func (s *Service) ensureBusinessPageData(ctx context.Context, tx *gorm.DB, businessID uint, p profile) error {
	var count int64
	if err := tx.WithContext(ctx).Model(&database.BusinessGalleryImage{}).Where("business_id = ?", businessID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		for i, img := range galleryImageSources(p) {
			// Cache-only: warmDemoAssets already did the network work outside
			// this transaction. A miss means the CDN or the bucket was
			// unavailable, and a demo with stock photos beats a failed seed.
			img = s.hostedAsset(img)
			captions := []string{"El salón", "De la parrilla", "Para picar"}
			caption := fmt.Sprintf("Galería %d", i+1)
			if i < len(captions) {
				caption = captions[i]
			}
			if err := tx.Create(&database.BusinessGalleryImage{BusinessID: businessID, ImageURL: img, Caption: caption, DisplayOrder: i + 1, IsActive: true}).Error; err != nil {
				return err
			}
		}
	}
	if err := tx.WithContext(ctx).Model(&database.BusinessOperatingHours{}).Where("business_id = ?", businessID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		for day := 0; day < 7; day++ {
			if err := tx.Create(&database.BusinessOperatingHours{BusinessID: businessID, DayOfWeek: day, OpenTime: "11:00", CloseTime: "23:00", IsClosed: false}).Error; err != nil {
				return err
			}
		}
	}
	return s.reconcileSpecialFeatures(ctx, tx, businessID, p)
}

// aiClaimPattern matches storefront copy that advertises the AI assistant.
// ASCII \b keeps it from firing inside Spanish words ("familia", "día").
var aiClaimPattern = regexp.MustCompile(`(?i)\b(ia|ai)\b|inteligencia artificial|artificial intelligence|asistente virtual|chatbot`)

// unbackedRailPatterns are claims no demo venue can honour: every crypto plugin
// seeds disabled and the settlement wallet is wiped, so on-chain copy sells a
// rail the guest cannot use (#935).
var unbackedRailPatterns = []string{
	"cripto", "crypto", "on-chain", "onchain", "usdc", "blockchain",
	"wallet", "billetera", "zero-fee", "zero fee", "sin comisi",
}

// demoSpecialFeatures is the canonical storefront claim set. Each row names a
// rail the demo actually runs: QR ordering, Mercado Pago / card / cash, and —
// only on the secondary (AI-enabled) venue — the AI assistant. Icons must exist in FEATURE_ICON_KEYS
// (frontend/src/components/business-page/featureIcons.tsx) or the storefront
// falls back to a generic glyph.
func demoSpecialFeatures(businessID uint, p profile) []database.BusinessSpecialFeature {
	features := []database.BusinessSpecialFeature{
		{
			BusinessID:   businessID,
			Title:        "Pedidos con QR",
			Description:  "Cada mesa pide desde el celular, sin esperar al mozo.",
			Icon:         "phone",
			DisplayOrder: 1,
			IsActive:     true,
		},
		{
			BusinessID:   businessID,
			Title:        "Mercado Pago, tarjeta y efectivo",
			Description:  "Pagá como quieras y la cuenta se cierra en el momento, con el comprobante en pantalla.",
			Icon:         "credit-card",
			DisplayOrder: 2,
			IsActive:     true,
		},
	}
	third := database.BusinessSpecialFeature{
		BusinessID:   businessID,
		Title:        "Cuenta dividida en la mesa",
		Description:  "Cada comensal paga su parte desde el celular, sin cuentas aparte ni vueltos.",
		Icon:         "users",
		DisplayOrder: 3,
		IsActive:     true,
	}
	if p.AIEnabled {
		third = database.BusinessSpecialFeature{
			BusinessID:   businessID,
			Title:        "Atención con IA",
			Description:  "Mozo, el asistente de la casa, responde preguntas de la carta y alérgenos.",
			Icon:         "zap",
			DisplayOrder: 3,
			IsActive:     true,
		}
	}
	return append(features, third)
}

// featureClaimIsUnbacked reports whether a legacy feature row advertises
// something this profile cannot deliver.
func featureClaimIsUnbacked(row database.BusinessSpecialFeature, p profile) bool {
	blob := strings.ToLower(row.Title + " " + row.Description)
	for _, pattern := range unbackedRailPatterns {
		if strings.Contains(blob, pattern) {
			return true
		}
	}
	return !p.AIEnabled && aiClaimPattern.MatchString(blob)
}

// reconcileSpecialFeatures replaces the old insert-if-empty seeding (plus its
// zero-fee-only copy heal), which left months-old demos advertising crypto
// rails and, on Core, AI it does not have (#935). It rewrites the canonical
// slots in place and retires — never deletes — leftover unbacked rows, so it is
// safe to run on every ensure.
func (s *Service) reconcileSpecialFeatures(ctx context.Context, tx *gorm.DB, businessID uint, p profile) error {
	want := demoSpecialFeatures(businessID, p)

	var existing []database.BusinessSpecialFeature
	if err := tx.WithContext(ctx).
		Where("business_id = ?", businessID).
		Order("display_order ASC, id ASC").
		Find(&existing).Error; err != nil {
		return err
	}

	// First row per slot wins; duplicates fall through to the retire pass.
	slot := make(map[int]*database.BusinessSpecialFeature, len(existing))
	for i := range existing {
		if _, taken := slot[existing[i].DisplayOrder]; !taken {
			slot[existing[i].DisplayOrder] = &existing[i]
		}
	}

	for _, feature := range want {
		current, ok := slot[feature.DisplayOrder]
		if !ok {
			if err := tx.WithContext(ctx).Create(&feature).Error; err != nil {
				return err
			}
			continue
		}
		if current.Title == feature.Title && current.Description == feature.Description &&
			current.Icon == feature.Icon && current.IsActive {
			continue
		}
		if err := tx.WithContext(ctx).Model(&database.BusinessSpecialFeature{}).
			Where("id = ?", current.ID).
			Updates(map[string]any{
				"title":       feature.Title,
				"description": feature.Description,
				"icon":        feature.Icon,
				"is_active":   true,
			}).Error; err != nil {
			return err
		}
	}

	for i := range existing {
		row := &existing[i]
		if kept, ok := slot[row.DisplayOrder]; ok && kept.ID == row.ID && row.DisplayOrder <= len(want) && row.DisplayOrder >= 1 {
			continue
		}
		if !row.IsActive || !featureClaimIsUnbacked(*row, p) {
			continue
		}
		if err := tx.WithContext(ctx).Model(&database.BusinessSpecialFeature{}).
			Where("id = ?", row.ID).
			Update("is_active", false).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ensureTablesAndCounters(ctx context.Context, tx *gorm.DB, businessID uint, p profile) error {
	_ = p // profile retained for call-site stability; table codes are no longer profile-derived.
	for i := 1; i <= 10; i++ {
		name := fmt.Sprintf("Mesa %d", i)
		var existing database.Table
		err := tx.WithContext(ctx).Where("business_id = ? AND name = ?", businessID, name).First(&existing).Error
		if err == nil {
			// Rotate legacy predictable codes to high-entropy ones on ensure (#293).
			if predictableDemoTableCode.MatchString(existing.TableCode) {
				code, genErr := uniqueTableCodeTx(ctx, tx)
				if genErr != nil {
					return genErr
				}
				if err := tx.WithContext(ctx).Model(&existing).Update("table_code", code).Error; err != nil {
					return err
				}
			}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		code, genErr := uniqueTableCodeTx(ctx, tx)
		if genErr != nil {
			return genErr
		}
		table := database.Table{
			BusinessID: businessID,
			TableCode:  code,
			Name:       name,
			Capacity:   2 + (i % 5),
			IsActive:   true,
		}
		if err := tx.WithContext(ctx).Create(&table).Error; err != nil {
			return err
		}
	}
	var biz database.Business
	if err := tx.WithContext(ctx).Select("counter_prefix", "counter_count").First(&biz, businessID).Error; err != nil {
		return err
	}
	prefix := strings.TrimSpace(biz.CounterPrefix)
	if prefix == "" {
		prefix = "C"
	}
	count := biz.CounterCount
	if count < 1 {
		count = 2
	}
	// Honor Counter Name Prefix so demo tiles match the setting (prefix "D"
	// → D1/D2), not stale "Counter 1"/"Counter 2" labels (#192). Stay on `tx`
	// so this participates in the outer demo seed transaction.
	for i := 1; i <= count; i++ {
		desired := fmt.Sprintf("%s%d", prefix, i)
		var counter database.Counter
		err := tx.WithContext(ctx).
			Where("business_id = ? AND counter_number = ?", businessID, i).
			First(&counter).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			counter = database.Counter{
				BusinessID:    businessID,
				CounterNumber: i,
				Name:          desired,
				IsActive:      true,
			}
			if err := tx.WithContext(ctx).Create(&counter).Error; err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Model(&counter).Updates(map[string]any{
			"name":      desired,
			"is_active": true,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

// uniqueTableCodeTx mirrors database.GenerateUniqueTableCode but uses the
// caller's transaction so in-tx uniqueness checks see sibling inserts.
func uniqueTableCodeTx(ctx context.Context, tx *gorm.DB) (string, error) {
	const codeLength = 10
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	max := big.NewInt(int64(len(charset)))
	for attempts := 0; attempts < 10; attempts++ {
		buf := make([]byte, codeLength)
		for i := range buf {
			n, err := cryptorand.Int(cryptorand.Reader, max)
			if err != nil {
				return "", err
			}
			buf[i] = charset[n.Int64()]
		}
		code := string(buf)
		var existing database.Table
		err := tx.WithContext(ctx).Where("table_code = ?", code).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return code, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("failed to generate unique demo table code after 10 attempts")
}

// uniqueDeliveryNumberTx returns a production-shaped DEL- + 16 hex id that is
// unused in this transaction. Same generator as live CreateDeliveryOrder.
func uniqueDeliveryNumberTx(ctx context.Context, tx *gorm.DB) (string, error) {
	for attempts := 0; attempts < 10; attempts++ {
		code := services.GenerateDeliveryNumber()
		var existing database.DeliveryOrder
		err := tx.WithContext(ctx).Where("delivery_number = ?", code).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return code, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("failed to generate unique demo delivery number after 10 attempts")
}

func uniqueBillPublicTokenTx(ctx context.Context, tx *gorm.DB) (string, error) {
	for attempts := 0; attempts < 10; attempts++ {
		token, err := database.GenerateBillPublicToken()
		if err != nil {
			return "", err
		}
		var existing database.Bill
		err = tx.WithContext(ctx).Where("public_token = ?", token).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return token, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("failed to generate unique demo bill public token after 10 attempts")
}

func (s *Service) ensureStaffAndSchedule(ctx context.Context, tx *gorm.DB, adminUserID, businessID uint) ([]database.Staff, []database.Position, error) {
	positions := []database.Position{
		{BusinessID: businessID, Name: "Manager", ColorHex: "#1a6b6a", Department: "FOH", IsActive: true, SortOrder: 1},
		{BusinessID: businessID, Name: "Server", ColorHex: "#f59e0b", Department: "FOH", IsActive: true, SortOrder: 2},
		{BusinessID: businessID, Name: "Host", ColorHex: "#e11d48", Department: "FOH", IsActive: true, SortOrder: 3},
		{BusinessID: businessID, Name: "Kitchen", ColorHex: "#16a34a", Department: "BOH", IsActive: true, SortOrder: 4},
	}
	for idx := range positions {
		var out database.Position
		if err := tx.WithContext(ctx).Where("business_id = ? AND name = ?", businessID, positions[idx].Name).FirstOrCreate(&out, positions[idx]).Error; err != nil {
			return nil, nil, err
		}
		positions[idx] = out
	}
	staffSpecs := []struct {
		Name     string
		Role     database.StaffRole
		Position int
	}{
		{"Camila Suárez", database.StaffRoleManager, 0},
		{"Joaquín Paredes", database.StaffRoleServer, 1},
		{"Milagros Ruiz", database.StaffRoleHost, 2},
		{"Tomás Aguirre", database.StaffRoleKitchen, 3},
		{"Sofía Benítez", database.StaffRoleServer, 1},
	}
	// Pre-assign pairwise-distinct PINs in email-sorted order so accessIdentities
	// can re-derive the same values without storing plaintext.
	emails := make([]string, len(staffSpecs))
	for idx := range staffSpecs {
		emails[idx] = s.demoEmail(fmt.Sprintf("demo+admin%d-business%d-staff%d", adminUserID, businessID, idx+1))
	}
	pinByEmail := assignDemoStaffPINs(emails)

	staff := make([]database.Staff, 0, len(staffSpecs))
	now := s.now().UTC()
	for idx, spec := range staffSpecs {
		email := emails[idx]
		pin := pinByEmail[email]
		pinHash, err := bcrypt.GenerateFromPassword([]byte(pin), bcrypt.MinCost)
		if err != nil {
			return nil, nil, err
		}
		// ARS wage scale (cents): ~AR$6.500–8.500/hora seed values; payroll is
		// later rescaled toward the 28% revenue target by scalePayrollToRevenue.
		row := database.Staff{BusinessID: businessID, Email: email, Name: spec.Name, Role: spec.Role, IsActive: true, InvitedBy: demoWallet, LastLoginAt: &now, EmploymentType: "hourly", HourlyRateCents: 650000 + int64(idx*50000), PinHash: string(pinHash), PinSetAt: &now}
		// Staff email uniqueness is business-scoped. Do not rely on a bare
		// ON CONFLICT here: the production constraint is an expression index
		// over normalized email, while focused SQLite tests intentionally use
		// the model schema only. An explicit business/email lookup keeps demo
		// seeding idempotent in both environments and avoids recreating every
		// staff-dependent static row on a second ensure.
		if err := tx.WithContext(ctx).
			Where("business_id = ? AND LOWER(TRIM(email)) = LOWER(TRIM(?))", businessID, email).
			Attrs(row).
			FirstOrCreate(&row).Error; err != nil {
			return nil, nil, err
		}
		// Always re-hash to the derived per-staff PIN so seed-version bumps and
		// re-ensures never leave a shared legacy PIN on an existing row.
		if err := tx.WithContext(ctx).Model(&row).Updates(map[string]interface{}{"pin_hash": string(pinHash), "pin_set_at": now}).Error; err != nil {
			return nil, nil, err
		}
		row.PinHash = string(pinHash)
		row.PinSetAt = &now
		staff = append(staff, row)
		link := database.StaffPosition{BusinessID: businessID, StaffID: row.ID, PositionID: positions[spec.Position].ID, PayRateCents: row.HourlyRateCents, IsPrimary: true}
		if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&link).Error; err != nil {
			return nil, nil, err
		}
		for weekday := 1; weekday <= 5; weekday++ {
			avail := database.StaffAvailability{BusinessID: businessID, StaffID: row.ID, Weekday: weekday, StartMin: 10 * 60, EndMin: 18 * 60, Kind: database.AvailabilityKindPreferred}
			if err := tx.WithContext(ctx).FirstOrCreate(&avail, database.StaffAvailability{BusinessID: businessID, StaffID: row.ID, Weekday: weekday, StartMin: 10 * 60, EndMin: 18 * 60}).Error; err != nil {
				return nil, nil, err
			}
		}
	}
	// Build the published week in the venue timezone with restaurant-shaped
	// coverage (lunch + dinner, overlapping staff, weekend included). Seeding
	// wall times in UTC previously rendered as 4:00 AM–12:00 PM for Pacific
	// operators looking at an America/New_York demo.
	loc := database.ResolveLocation(defaultTimezone)
	week := weekStartMonday(normalizeBusinessDate(s.now(), loc))
	schedule := database.Schedule{BusinessID: businessID, WeekStart: week, Status: database.ScheduleStatusPublished, PublishedAt: &now, PublishedByStaffID: &staff[0].ID, Notes: "Semana publicada"}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&schedule).Error; err != nil {
		return nil, nil, err
	}
	if schedule.ID == 0 {
		if err := tx.WithContext(ctx).Where("business_id = ? AND week_start = ?", businessID, week).First(&schedule).Error; err != nil {
			return nil, nil, err
		}
	}
	// Replace the week's shifts so older UTC/staircase seeds cannot linger.
	// Coverage claims/swaps/notes hang off shift_id — clear them first so a
	// second ensure does not accumulate orphan rows (idempotency gate).
	var legacyShiftIDs []uint
	if err := tx.WithContext(ctx).Model(&database.Shift{}).
		Where("business_id = ? AND schedule_id = ?", businessID, schedule.ID).
		Pluck("id", &legacyShiftIDs).Error; err != nil {
		return nil, nil, err
	}
	if len(legacyShiftIDs) > 0 {
		if err := tx.WithContext(ctx).Where("business_id = ? AND shift_id IN ?", businessID, legacyShiftIDs).
			Delete(&database.OpenShiftClaim{}).Error; err != nil {
			return nil, nil, err
		}
		if err := tx.WithContext(ctx).Where("business_id = ? AND shift_id IN ?", businessID, legacyShiftIDs).
			Delete(&database.ShiftSwapRequest{}).Error; err != nil {
			return nil, nil, err
		}
		if err := tx.WithContext(ctx).Where("business_id = ? AND shift_id IN ?", businessID, legacyShiftIDs).
			Delete(&database.ShiftNote{}).Error; err != nil {
			return nil, nil, err
		}
		if err := tx.WithContext(ctx).Where("business_id = ? AND schedule_id = ?", businessID, schedule.ID).
			Delete(&database.Shift{}).Error; err != nil {
			return nil, nil, err
		}
	}
	type demoShiftSpec struct {
		staffIdx  int
		dayOffset int
		startHour int
		hours     int
		breakMin  int
		notes     string
	}
	// Mon–Sun coverage: openers (10–18) + closers (16–00), multiple FOH/BOH
	// overlapping so the board is not a one-shift-per-person staircase.
	shiftSpecs := []demoShiftSpec{
		{0, 0, 10, 8, 30, "Apertura encargada"}, // Camila lunes mediodía
		{1, 0, 10, 8, 30, "Mozo mediodía"},      // Joaquín lunes mediodía
		{2, 0, 10, 8, 0, "Recepción apertura"},  // Milagros lunes
		{3, 0, 10, 8, 30, "Cocina mediodía"},    // Tomás lunes
		{4, 0, 16, 8, 30, "Moza noche"},         // Sofía lunes noche
		{1, 1, 10, 8, 30, "Mozo mediodía"},
		{4, 1, 16, 8, 30, "Moza noche"},
		{3, 1, 12, 8, 30, "Cocina turno medio"},
		{0, 2, 12, 8, 30, "Encargada turno medio"},
		{1, 2, 16, 8, 30, "Mozo noche"},
		{2, 2, 16, 6, 0, "Recepción noche"},
		{4, 3, 10, 8, 30, "Moza mediodía"},
		{1, 3, 16, 8, 30, "Mozo noche"},
		{3, 3, 10, 10, 30, "Cocina turno largo"},
		{0, 4, 10, 8, 30, "Apertura encargada"},
		{4, 4, 16, 8, 30, "Moza noche"},
		{2, 4, 10, 8, 0, "Recepción apertura"},
		{1, 5, 10, 8, 30, "Sábado mediodía"}, // fin de semana
		{4, 5, 16, 8, 30, "Sábado noche"},
		{3, 5, 11, 9, 30, "Sábado cocina"},
		{2, 5, 10, 8, 0, "Sábado recepción"},
		{1, 6, 10, 8, 30, "Domingo mediodía"},
		{4, 6, 16, 8, 30, "Domingo noche"},
		{0, 6, 12, 6, 0, "Domingo encargada"},
	}
	for _, spec := range shiftSpecs {
		st := staff[spec.staffIdx]
		day := week.AddDate(0, 0, spec.dayOffset)
		shiftStart := time.Date(day.Year(), day.Month(), day.Day(), spec.startHour, 0, 0, 0, loc)
		shiftEnd := shiftStart.Add(time.Duration(spec.hours) * time.Hour)
		attrs := database.Shift{
			BusinessID:       businessID,
			ScheduleID:       schedule.ID,
			StaffID:          &st.ID,
			PositionID:       positions[staffSpecs[spec.staffIdx].Position].ID,
			StartsAt:         shiftStart.UTC(),
			EndsAt:           shiftEnd.UTC(),
			BreakMinutes:     spec.breakMin,
			Status:           database.ShiftStatusFilled,
			Published:        true,
			Notes:            spec.notes,
			CreatedByStaffID: staff[0].ID,
		}
		if err := tx.WithContext(ctx).Create(&attrs).Error; err != nil {
			return nil, nil, err
		}
	}
	inviteEmail := s.demoEmail(fmt.Sprintf("demo+admin%d-business%d-pending-invite", adminUserID, businessID))
	invite := database.StaffInvitation{
		BusinessID: businessID,
		Email:      inviteEmail,
		Name:       "Lucía Ferreyra",
		Role:       database.StaffRoleServer,
		Token:      deterministicUUID("demo-invite", adminUserID, businessID),
		Status:     database.InvitationStatusPending,
		InvitedBy:  demoWallet,
		ExpiresAt:  now.AddDate(0, 0, 14),
	}
	// Idempotency must key on the deterministic token — the column carrying the
	// global unique index (idx_staff_invitations_token) — NOT on (business_id,
	// email). The demo email domain is config-driven (s.emailDomain), so if it
	// changes between deploys the same (adminUserID, businessID) produces a new
	// email while the token stays fixed. A (business_id, email) lookup then
	// misses the existing token-holding row and the INSERT collides on the
	// token index, aborting the whole demo transaction (25P02 cascade).
	//
	// The token is the ONLY query condition; the row's other fields go through
	// Attrs (create-only) so a drifted email does not leak into the WHERE and
	// re-trigger the miss-then-collide.
	if err := tx.WithContext(ctx).
		Where("token = ?", invite.Token).
		Attrs(invite).
		FirstOrCreate(&invite).Error; err != nil {
		return nil, nil, err
	}
	return staff, positions, nil
}

func (s *Service) ensureInventory(ctx context.Context, tx *gorm.DB, businessID uint) error {
	settings := database.InventorySettings{BusinessID: businessID, InventoryEnabled: true, AutoDeductOnOrderApproval: true, LowStockWarningsEnabled: true, AvailabilitySyncMode: database.InventoryAvailabilityModeWarn}
	if err := tx.WithContext(ctx).Where("business_id = ?", businessID).Attrs(settings).FirstOrCreate(&settings).Error; err != nil {
		return err
	}
	// Load business locale so category badges match the operator UI (MIN-10).
	var invBiz database.Business
	_ = tx.WithContext(ctx).Select("id", "default_language", "source_language").First(&invBiz, businessID).Error
	invLocale := demoLocale(&invBiz)
	// Named specs — never zip ingredients to dishes by seed-slice index.
	// A stale FirstOrCreate pairing (Premium Beef → demo-bowl) 86'd the
	// vegetarian Harvest Bowl while Steak Plate stayed sellable (#727).
	// A single stock item can feed several dishes — the media res is butchered
	// into every cut on the parrilla — so the link list is per item, not 1:1.
	type demoRecipeLink struct {
		menuItemID   string
		menuItemName string
		qtyRequired  float64
	}
	type demoInventorySpec struct {
		item    database.InventoryItem
		recipes []demoRecipeLink
	}
	// Costs are ARS pesos per unit (float64 wire shape). The -BEEF SKU suffix is
	// load-bearing: day_generator's restock exception keys on it so the star cut
	// can run an out-of-stock narrative between deliveries.
	specs := []demoInventorySpec{
		{
			item:    database.InventoryItem{BusinessID: businessID, Name: "Verdura de estación", SKU: fmt.Sprintf("DEMO-%d-GREENS", businessID), Category: inventoryCategoryLabel(invLocale, "Produce"), Unit: "kg", CurrentQuantity: 48, ReorderThreshold: 10, CostPerUnit: 2400, IsActive: true},
			recipes: []demoRecipeLink{{menuItemID: "demo-ensalada", menuItemName: "Ensalada mixta", qtyRequired: 0.20}},
		},
		{
			item: database.InventoryItem{BusinessID: businessID, Name: "Bife de chorizo (media res)", SKU: fmt.Sprintf("DEMO-%d-BEEF", businessID), Category: inventoryCategoryLabel(invLocale, "Protein"), Unit: "kg", CurrentQuantity: 22, ReorderThreshold: 6, CostPerUnit: 14800, IsActive: true},
			// Every cut on the parrilla is butchered off this carcass, so an
			// empty walk-in has to 86 all of them — not just the bife (#945).
			// Yields are kg of trimmed beef per plate. Matambrito is pork and
			// the bodegón plates are prepped ahead, so they keep selling.
			recipes: []demoRecipeLink{
				{menuItemID: "demo-bife", menuItemName: "Bife de chorizo", qtyRequired: 0.40},
				{menuItemID: "demo-ojo-de-bife", menuItemName: "Ojo de bife", qtyRequired: 0.45},
				{menuItemID: "demo-asado-tira", menuItemName: "Asado de tira", qtyRequired: 0.50},
				{menuItemID: "demo-entrana", menuItemName: "Entraña", qtyRequired: 0.35},
				{menuItemID: "demo-parrillada", menuItemName: "Parrillada para dos", qtyRequired: 0.90},
			},
		},
		{
			item:    database.InventoryItem{BusinessID: businessID, Name: "Malbec de bodega", SKU: fmt.Sprintf("DEMO-%d-MALBEC", businessID), Category: inventoryCategoryLabel(invLocale, "Beverage"), Unit: "l", CurrentQuantity: 35, ReorderThreshold: 8, CostPerUnit: 9500, IsActive: true},
			recipes: []demoRecipeLink{{menuItemID: "demo-malbec-copa", menuItemName: "Copa de Malbec", qtyRequired: 0.15}},
		},
		{
			item:    database.InventoryItem{BusinessID: businessID, Name: "Provolone para provoleta", SKU: fmt.Sprintf("DEMO-%d-PROVOLONE", businessID), Category: inventoryCategoryLabel(invLocale, "Dairy"), Unit: "kg", CurrentQuantity: 14, ReorderThreshold: 4, CostPerUnit: 11200, IsActive: true},
			recipes: []demoRecipeLink{{menuItemID: "demo-provoleta", menuItemName: "Provoleta", qtyRequired: 0.18}},
		},
		{
			item:    database.InventoryItem{BusinessID: businessID, Name: "Chorizo criollo", SKU: fmt.Sprintf("DEMO-%d-CHORIZO", businessID), Category: inventoryCategoryLabel(invLocale, "Protein"), Unit: "unidad", CurrentQuantity: 60, ReorderThreshold: 15, CostPerUnit: 1900, IsActive: true},
			recipes: []demoRecipeLink{{menuItemID: "demo-choripan", menuItemName: "Choripán", qtyRequired: 1.0}},
		},
	}
	for i := range specs {
		spec := specs[i]
		// Idempotent seed on the natural key (business_id, sku). Use Attrs
		// (create-only) so the item's fields never leak into the WHERE: later
		// simulated orders deduct current_quantity, and passing the full struct
		// as match conditions made the second ensure miss the drifted row and
		// re-INSERT a duplicate SKU — the exact bug this closes. Uniqueness is
		// enforced by a PARTIAL unique index (WHERE sku <> '')
		// so blank operator SKUs still coexist; GORM's ON CONFLICT can't name a
		// partial index as its arbiter, so the index (not app idempotency) is
		// the real dup backstop and a losing concurrent reseed simply rolls back
		// for the self-healing hourly append to retry.
		var item database.InventoryItem
		if err := tx.WithContext(ctx).Where("business_id = ? AND sku = ?", businessID, spec.item.SKU).Attrs(spec.item).FirstOrCreate(&item).Error; err != nil {
			return err
		}
		// Drop drifted pairings for this SKU (e.g. beef still pointing at
		// demo-bowl from an older parallel-array seed) so re-ensure heals #727.
		// The seed owns the whole link set for its own items, so anything
		// outside it goes — that is also what backfills the cuts a pre-#945
		// demo is missing, since the survivors below are simply re-created.
		wantedMenuItemIDs := make([]string, 0, len(spec.recipes))
		for _, link := range spec.recipes {
			wantedMenuItemIDs = append(wantedMenuItemIDs, link.menuItemID)
		}
		if err := tx.WithContext(ctx).
			Where("business_id = ? AND inventory_item_id = ? AND menu_item_id NOT IN ?", businessID, item.ID, wantedMenuItemIDs).
			Delete(&database.InventoryRecipe{}).Error; err != nil {
			return err
		}
		for _, link := range spec.recipes {
			recipe := database.InventoryRecipe{
				BusinessID:       businessID,
				MenuItemID:       link.menuItemID,
				MenuItemName:     link.menuItemName,
				InventoryItemID:  item.ID,
				QuantityRequired: link.qtyRequired,
			}
			if err := tx.WithContext(ctx).FirstOrCreate(&recipe, database.InventoryRecipe{BusinessID: businessID, MenuItemID: recipe.MenuItemID, InventoryItemID: item.ID}).Error; err != nil {
				return err
			}
			if recipe.MenuItemName != link.menuItemName || recipe.QuantityRequired != link.qtyRequired {
				if err := tx.WithContext(ctx).Model(&recipe).Updates(map[string]interface{}{
					"menu_item_name":    link.menuItemName,
					"quantity_required": link.qtyRequired,
				}).Error; err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (s *Service) ensureReservations(ctx context.Context, tx *gorm.DB, business *database.Business) error {
	businessID := business.ID
	settings := database.ReservationSettings{BusinessID: businessID, Enabled: true, MaxAdvanceDays: 45, MinAdvanceMinutes: 30, MinPartySize: 1, MaxPartySize: 12, DefaultDuration: 120, SlotIntervalMinutes: 30, MaxCoversPerSlot: 48, AutoAssignTables: true, ApprovalMode: "auto", AllowWaitlist: true, SendConfirmationEmail: true, SendReminderEmail: true, ExternalPartnerLinks: database.JSONRawMessage(`[]`)}
	if err := tx.WithContext(ctx).Where("business_id = ?", businessID).Attrs(settings).FirstOrCreate(&settings).Error; err != nil {
		return err
	}
	return s.healDemoReservationLanguages(ctx, tx, business)
}

// healDemoReservationLanguages repairs seeded reservations that carry the wrong
// guest locale (issue 853). Rows written before the day generator stamped a
// language took the column default "en", so an es-AR venue's showroom bookings
// read as English — and the generator alone can never fix them, because it
// returns early on any day whose confirmation code already exists.
//
// Scoped to codes the generator owns (RSV-<business>-<yyyymmdd>), so a guest
// who actually booked on a demo venue keeps the locale they chose. Runs on
// every ensure, not just the seeding branch: a live instance with
// last_simulated_business_date set never re-enters generateDays at all.
func (s *Service) healDemoReservationLanguages(ctx context.Context, tx *gorm.DB, business *database.Business) error {
	language := demoLocale(business)
	return tx.WithContext(ctx).Model(&database.TableReservation{}).
		Where("business_id = ? AND confirmation_code LIKE ? AND (language IS NULL OR language <> ?)",
			business.ID, demoReservationCodePrefix(business.ID)+"%", language).
		Update("language", language).Error
}

func (s *Service) ensurePendingAlternativePaymentBill(ctx context.Context, tx *gorm.DB, businessID uint, p profile, staff []database.Staff) error {
	tableID, err := nthTableID(ctx, tx, businessID, 98)
	if err != nil {
		return err
	}
	staffID, err := nthStaffID(ctx, tx, businessID, 0)
	if err != nil {
		return err
	}
	if len(staff) > 0 {
		staffID = staff[0].ID
	}
	now := s.now().UTC()
	createdAt := now.Add(-45 * time.Minute)
	lines := withLineSubtotals([]billLine{
		{MenuItemID: "demo-milanesa", Name: "Milanesa napolitana", Price: 21500, Quantity: 1},
		{MenuItemID: "demo-fernet", Name: "Fernet con coca", Price: 7800, Quantity: 2},
	})
	// AR carta prices are IVA-final — no added tax line on demo bills.
	total := computeTotals(lines, 0, p.ServiceFeeRate)
	paidAmount := total.TotalCents / 2
	pendingAmount := total.TotalCents - paidAmount
	raw, err := json.Marshal(lines)
	if err != nil {
		return err
	}
	billNumber := demoBillNumber(businessID, "pending")
	var bill database.Bill
	err = tx.WithContext(ctx).Where("bill_number = ?", billNumber).First(&bill).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		bill = database.Bill{
			BusinessID:       businessID,
			TableID:          tableID,
			BillNumber:       billNumber,
			Notes:            "Cuenta parcial — tarjeta pendiente de confirmación",
			Items:            string(raw),
			Subtotal:         total.SubtotalCents,
			TaxAmount:        total.TaxCents,
			ServiceFeeAmount: total.ServiceCents,
			TotalAmount:      total.TotalCents,
			PaidAmount:       paidAmount,
			Status:           database.BillStatusPartial,
			SettlementAddr:   "",
			CreatedByStaffID: &staffID,
			CreatedAt:        createdAt,
			UpdatedAt:        createdAt,
		}
		if err := tx.WithContext(ctx).Create(&bill).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	for idx, line := range lines {
		item := database.BillItem{
			ID:         deterministicUUID("pending-bill-item", bill.ID, idx),
			BillID:     bill.ID,
			MenuItemID: line.MenuItemID,
			Name:       line.Name,
			Price:      line.Price,
			Quantity:   line.Quantity,
			ItemType:   "menu_item",
			Subtotal:   line.Subtotal,
			CreatedAt:  createdAt.Add(time.Duration(idx+1) * time.Minute),
		}
		if err := tx.WithContext(ctx).Where("id = ?", item.ID).Attrs(item).FirstOrCreate(&item).Error; err != nil {
			return err
		}
	}
	confirmedAt := createdAt.Add(3 * time.Minute)
	// Counter card rail, not crypto — the demo venue seeds no settlement wallet
	// and no enabled crypto plugin, so it could never have taken this tender on
	// chain (#795). See createPayment for the full rationale.
	payment := database.Payment{
		BillID:        bill.ID,
		PayerAddr:     demoPayerAddr(businessID + 3),
		Amount:        paidAmount,
		TxHash:        demoSyntheticTxHash("pending", businessID, bill.ID),
		Status:        database.PaymentStatusConfirmed,
		PaymentMethod: "card",
		ConfirmedAt:   &confirmedAt,
		CreatedAt:     confirmedAt,
		UpdatedAt:     confirmedAt,
	}
	if err := tx.WithContext(ctx).Where("tx_hash = ?", payment.TxHash).Attrs(payment).FirstOrCreate(&payment).Error; err != nil {
		return err
	}
	// FirstOrCreate leaves an already-seeded fixture untouched, and the
	// settlement_chain column default fills 'base' behind a zero value. This is
	// a standing floor fixture rather than settled history, so heal the rail on
	// the row the demo owns instead of leaving a crypto claim on it.
	if err := tx.WithContext(ctx).Model(&payment).
		Updates(map[string]interface{}{"payment_method": "card", "settlement_chain": ""}).Error; err != nil {
		return err
	}
	alt := database.AlternativePayment{
		BillID:          bill.ID,
		ParticipantAddr: "demo-walkup",
		ParticipantName: "Cliente de paso",
		Amount:          pendingAmount,
		BillAmountCents: pendingAmount,
		PaymentMethod:   database.PaymentMethodCard,
		Status:          database.AltPaymentStatusPending,
		IdempotencyKey:  fmt.Sprintf("demo-pending-alt-%d", bill.ID),
		ConfirmedBy:     "demo-manager",
		CreatedAt:       createdAt.Add(5 * time.Minute),
		UpdatedAt:       createdAt.Add(5 * time.Minute),
	}
	if err := tx.WithContext(ctx).Where("bill_id = ? AND idempotency_key = ?", bill.ID, alt.IdempotencyKey).Attrs(alt).FirstOrCreate(&alt).Error; err != nil {
		return err
	}
	return nil
}

// demoZoneBoundaries is the matcher-compatible boundary blob for the demo
// delivery zone. The zone matcher (services.zoneMatchesAddress) only
// understands `{"postal_codes":[...],"cities":[...]}` — the previous GeoJSON
// polygon unmarshalled to empty lists and matched NO address on earth,
// dead-ending every guest delivery walkthrough. Prefix wildcards cover both
// demo venues' printed addresses (Defensa 9148 C1065 San Telmo, Costa Rica
// 9602 C1414 Palermo) in the CPA form ("C1065") AND the bare 4-digit form
// ("1065") a porteño prospect is likely to type.
const demoZoneBoundaries = `{"postal_codes":["C10*","C14*","10*","14*"],"cities":["Buenos Aires","CABA","Capital Federal"]}`

// demoZoneOperatingHours matches the seeded business hours so quotes are
// eligible every service day, not Monday-only (#714).
const demoZoneOperatingHours = `{"mon":"11:00-23:00","tue":"11:00-23:00","wed":"11:00-23:00","thu":"11:00-23:00","fri":"11:00-23:00","sat":"11:00-23:00","sun":"11:00-23:00"}`

// The showroom keeps exactly ONE delivery zone per venue, and these are its
// canonical values. Pre-v8 seeds named that zone "Downtown" with Monday-only
// hours, a USD 4.99 fee and NY boundaries; the seed and its heal are keyed on
// the name, so on those venues the ensure stacked a fresh CABA zone beside the
// legacy row and healed only the new one — leaving a stale zone quoting
// `mon 11:00-22:00` next to a business open 11:00-23:00 daily (#905).
const (
	demoZoneName              = "CABA"
	demoZoneDescription       = "San Telmo, Palermo y barrios linderos"
	demoZoneDeliveryFeeCents  = int64(290000)  // AR$2.900 envío
	demoZoneMinimumOrderCents = int64(1500000) // AR$15.000 pedido mínimo
	demoZoneEstimatedMinutes  = 32
)

const demoOperatorDispatchSeed = "Use the delivery dispatch board for assignment."

// demoZoneCoversCABADemoPostals reports whether a stored boundaries blob still
// covers both demo venues' CABA postal prefixes (C10* San Telmo, C14* Palermo)
// and the Buenos Aires city fallback. Legacy NY blobs and hand-edited zones
// fail this and get healed back to demoZoneBoundaries.
func demoZoneCoversCABADemoPostals(raw string) bool {
	var b struct {
		PostalCodes []string `json:"postal_codes"`
		Cities      []string `json:"cities"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &b); err != nil {
		return false
	}
	hasC10, hasC14 := false, false
	for _, code := range b.PostalCodes {
		code = strings.TrimSpace(code)
		if strings.HasPrefix(code, "C10") && strings.Contains(code, "*") {
			hasC10 = true
		}
		if strings.HasPrefix(code, "C14") && strings.Contains(code, "*") {
			hasC14 = true
		}
		switch code {
		case "C1065", "C1414":
			// exact venue codes count for their own prefix
			if code == "C1065" {
				hasC10 = true
			} else {
				hasC14 = true
			}
		}
	}
	hasBA := false
	for _, city := range b.Cities {
		if strings.EqualFold(strings.TrimSpace(city), "Buenos Aires") {
			hasBA = true
			break
		}
	}
	return hasC10 && hasC14 && hasBA
}

// demoZoneSnapshot mirrors the JSON shape of services.DeliveryZoneDTO — the
// shape syncDeliveryZones serializes into DeliverySettings.DeliveryZones.
// Money fields are DOLLARS here (the DTO divides cents by 100).
type demoZoneSnapshot struct {
	ID                  uint            `json:"id"`
	Name                string          `json:"name"`
	Description         string          `json:"description"`
	DeliveryFee         float64         `json:"delivery_fee"`
	MinimumOrderAmount  float64         `json:"minimum_order_amount"`
	EstimatedTime       int             `json:"estimated_time"`
	Priority            int             `json:"priority"`
	CutoffBufferMinutes int             `json:"cutoff_buffer_minutes"`
	OperatingHours      string          `json:"operating_hours"`
	Boundaries          json.RawMessage `json:"boundaries,omitempty"`
	IsActive            bool            `json:"is_active"`
}

// zoneBoundariesMatchable reports whether a stored boundaries blob can ever
// match an address under the real zone matcher: it must be JSON carrying at
// least one postal code or city. GeoJSON polygons and empty objects fail.
func zoneBoundariesMatchable(raw string) bool {
	var b struct {
		PostalCodes []string `json:"postal_codes"`
		Cities      []string `json:"cities"`
	}
	if err := json.Unmarshal([]byte(raw), &b); err != nil {
		return false
	}
	return len(b.PostalCodes) > 0 || len(b.Cities) > 0
}

// zoneHoursMondayOnly reports the pre-#714 demo blob that only listed Monday.
func zoneHoursMondayOnly(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	var hours map[string]string
	if err := json.Unmarshal([]byte(raw), &hours); err != nil {
		return false
	}
	if len(hours) != 1 {
		return false
	}
	_, ok := hours["mon"]
	return ok
}

// zoneHoursCoverFullWeek reports whether every weekday is present with a window.
func zoneHoursCoverFullWeek(raw string) bool {
	var hours map[string]string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &hours); err != nil {
		return false
	}
	for _, day := range []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"} {
		if strings.TrimSpace(hours[day]) == "" {
			return false
		}
	}
	return true
}

func isOperatorDispatchSeedNote(raw string) bool {
	return strings.Contains(strings.ToLower(raw), "dispatch board")
}

// canonicalizeDemoZones collapses a demo venue onto the single canonical zone
// BEFORE the name-keyed seed runs. A legacy row is adopted by rename rather
// than replaced so delivery_orders.zone_id references survive (that FK is NO
// ACTION in the genesis schema, so a blind delete would abort the whole demo
// transaction); surplus rows are retired with their orders repointed at the
// survivor. Idempotent: a venue already holding one CABA zone is untouched.
func (s *Service) canonicalizeDemoZones(ctx context.Context, tx *gorm.DB, businessID uint) error {
	var zones []database.DeliveryZone
	if err := tx.WithContext(ctx).Where("business_id = ?", businessID).Order("id ASC").Find(&zones).Error; err != nil {
		return err
	}
	if len(zones) == 0 {
		return nil
	}
	keep := 0
	for i, zone := range zones {
		if zone.Name == demoZoneName {
			keep = i
			break
		}
	}
	if zones[keep].Name != demoZoneName {
		if err := tx.WithContext(ctx).Model(&database.DeliveryZone{}).
			Where("id = ?", zones[keep].ID).
			Updates(map[string]interface{}{"name": demoZoneName, "description": demoZoneDescription}).Error; err != nil {
			return err
		}
	}
	staleIDs := make([]uint, 0, len(zones))
	for i, zone := range zones {
		if i != keep {
			staleIDs = append(staleIDs, zone.ID)
		}
	}
	if len(staleIDs) == 0 {
		return nil
	}
	if err := tx.WithContext(ctx).Model(&database.DeliveryOrder{}).
		Where("business_id = ? AND zone_id IN ?", businessID, staleIDs).
		Update("zone_id", zones[keep].ID).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Where("business_id = ? AND id IN ?", businessID, staleIDs).
		Delete(&database.DeliveryZone{}).Error
}

func (s *Service) ensureDelivery(ctx context.Context, tx *gorm.DB, businessID uint, staff []database.Staff) error {
	// ARS cents: envío AR$2.900, envío gratis desde AR$40.000, pedido mínimo AR$15.000.
	settings := database.DeliverySettings{BusinessID: businessID, DeliveryEnabled: true, InHouseDeliveryEnabled: true, ThirdPartyEnabled: true, UberEatsEnabled: true, PaymentMode: string(database.DeliveryPaymentOnline), FlatDeliveryFee: 290000, FreeDeliveryMinimum: 4000000, MinimumOrderAmount: 1500000, DeliveryRadius: 4.5, EstimatedPrepTime: 24, MaxConcurrentDeliveries: 6, DeliveryHoursSameAsBusiness: true, DeliveryInstructions: "", DeliveryZones: database.JSONRawMessage(`[]`), ExternalPartnerLinks: database.JSONRawMessage(`[{"name":"PedidosYa","url":"https://www.pedidosya.com.ar","provider_key":"pedidosya"},{"name":"Rappi","url":"https://www.rappi.com.ar","provider_key":"rappi"}]`), AutoAssignDrivers: true}
	if err := tx.WithContext(ctx).Where("business_id = ?", businessID).Attrs(settings).FirstOrCreate(&settings).Error; err != nil {
		return err
	}
	if err := s.canonicalizeDemoZones(ctx, tx, businessID); err != nil {
		return err
	}
	zone := database.DeliveryZone{BusinessID: businessID, Name: demoZoneName, Description: demoZoneDescription, Boundaries: demoZoneBoundaries, DeliveryFee: demoZoneDeliveryFeeCents, MinimumOrderAmount: demoZoneMinimumOrderCents, EstimatedTime: demoZoneEstimatedMinutes, Priority: 1, IsActive: true, OperatingHours: demoZoneOperatingHours}
	if err := tx.WithContext(ctx).Where("business_id = ? AND name = ?", businessID, zone.Name).Attrs(zone).FirstOrCreate(&zone).Error; err != nil {
		return err
	}
	// Self-heal production demo zone rows. Matchable-but-wrong postals (legacy
	// NY blobs, leftover GeoJSON that unmarshals empty, Monday-only hours)
	// still fail guest quotes for the CABA demo addresses — always rewrite to
	// the canonical matcher JSON + full-week window (#714).
	// An adopted legacy row also carries legacy money (USD 4.99 envío, a
	// US$15 minimum) and a legacy ETA, all of which the guest quote and the
	// Delivery tab render verbatim — re-assert the whole canonical zone.
	zoneNeedsHeal := !zone.IsActive ||
		!zoneBoundariesMatchable(zone.Boundaries) ||
		!demoZoneCoversCABADemoPostals(zone.Boundaries) ||
		!zoneHoursCoverFullWeek(zone.OperatingHours) ||
		zone.Description != demoZoneDescription ||
		zone.DeliveryFee != demoZoneDeliveryFeeCents ||
		zone.MinimumOrderAmount != demoZoneMinimumOrderCents ||
		zone.EstimatedTime != demoZoneEstimatedMinutes
	if zoneNeedsHeal {
		if err := tx.WithContext(ctx).Model(&database.DeliveryZone{}).Where("id = ?", zone.ID).
			Updates(map[string]interface{}{
				"description":          demoZoneDescription,
				"boundaries":           demoZoneBoundaries,
				"is_active":            true,
				"operating_hours":      demoZoneOperatingHours,
				"delivery_fee":         demoZoneDeliveryFeeCents,
				"minimum_order_amount": demoZoneMinimumOrderCents,
				"estimated_time":       demoZoneEstimatedMinutes,
			}).Error; err != nil {
			return err
		}
		zone.Description = demoZoneDescription
		zone.Boundaries = demoZoneBoundaries
		zone.IsActive = true
		zone.OperatingHours = demoZoneOperatingHours
		zone.DeliveryFee = demoZoneDeliveryFeeCents
		zone.MinimumOrderAmount = demoZoneMinimumOrderCents
		zone.EstimatedTime = demoZoneEstimatedMinutes
	}
	if isOperatorDispatchSeedNote(settings.DeliveryInstructions) {
		if err := tx.WithContext(ctx).Model(&database.DeliverySettings{}).Where("id = ?", settings.ID).
			Update("delivery_instructions", "").Error; err != nil {
			return err
		}
		settings.DeliveryInstructions = ""
	}
	// Keep the DeliverySettings.DeliveryZones snapshot blob consistent with the
	// DTO shape the settings surface writes (syncDeliveryZones serializes
	// []DeliveryZoneDTO — the legacy `[{"name":"Downtown","fee":4.99}]` stub
	// decoded to zero zones). Money fields are dollars in this blob.
	snapshot, err := json.Marshal([]demoZoneSnapshot{{
		ID:                  zone.ID,
		Name:                zone.Name,
		Description:         zone.Description,
		DeliveryFee:         float64(zone.DeliveryFee) / 100.0,
		MinimumOrderAmount:  float64(zone.MinimumOrderAmount) / 100.0,
		EstimatedTime:       zone.EstimatedTime,
		Priority:            zone.Priority,
		CutoffBufferMinutes: zone.CutoffBufferMinutes,
		OperatingHours:      zone.OperatingHours,
		Boundaries:          json.RawMessage(zone.Boundaries),
		IsActive:            zone.IsActive,
	}})
	if err != nil {
		return err
	}
	if string(settings.DeliveryZones) != string(snapshot) {
		if err := tx.WithContext(ctx).Model(&database.DeliverySettings{}).Where("id = ?", settings.ID).
			Update("delivery_zones", database.JSONRawMessage(snapshot)).Error; err != nil {
			return err
		}
	}
	driverNames := []string{"Nahuel Giménez", "Brenda Acosta"}
	for i := 0; i < 2; i++ {
		var staffID *uint
		if len(staff) > i+1 {
			staffID = &staff[i+1].ID
		}
		// TotalEarnings is ARS pesos (float64 wire shape) accumulated per driver.
		driver := database.DeliveryDriver{BusinessID: businessID, StaffID: staffID, Name: driverNames[i], Phone: demoFakeMobile(2300 + i), Email: s.demoEmail(fmt.Sprintf("demo+business%d-driver%d", businessID, i+1)), VehicleType: database.VehicleTypeMotorcycle, VehiclePlate: fmt.Sprintf("A%03dBCD", 120+i), Status: database.DriverStatusOnline, IsAvailable: true, TotalDeliveries: 12 + i, CompletedDeliveries: 11 + i, AverageRating: 4.8, TotalEarnings: 68500, IsActive: true}
		if err := tx.WithContext(ctx).Where("business_id = ? AND email = ?", businessID, driver.Email).Attrs(driver).FirstOrCreate(&driver).Error; err != nil {
			return err
		}
	}
	return nil
}

// deliveryPayFixtureNote marks the generator-owned "awaiting online payment"
// delivery check. Nothing but this seeder writes it, so it is the safe
// ownership predicate for the self-heal below: guest walk-outs and QA
// leftovers carry their own notes and are never matched.
const deliveryPayFixtureNote = "Demo delivery bill awaiting online payment"

// purgeStaleDeliveryPayFixtures deletes generator-owned pay-online delivery
// checks that can never be paid and are not the live fixture.
//
// #855: the fixture's payment window is 15 minutes and the re-arm runs
// hourly, so every tick found a dead check and minted ANOTHER one. The
// clones are unpaid and only `voided` is excluded from the collection-gap
// predicate, so six hours of dinner produced a fake ARS 251.950 hole on the
// accounting board. Retiring the dead predecessors before minting keeps the
// showroom at exactly one live pay-online check per venue, and repairs
// venues that already accumulated a stack of them.
//
// Deliberately narrow so QA/live leftovers survive:
//   - notes must equal the generator's own marker
//   - paid_amount must be 0 (a partially/fully settled check is history)
//   - status paid/partial is never touched
//   - the newest still-open check survives as the live fixture
//   - a check carrying confirmed money or a fiscal receipt is skipped
func (s *Service) purgeStaleDeliveryPayFixtures(ctx context.Context, tx *gorm.DB, businessID uint) error {
	var candidates []database.Bill
	if err := tx.WithContext(ctx).
		Where("business_id = ? AND notes = ? AND paid_amount = 0", businessID, deliveryPayFixtureNote).
		Where("status NOT IN ?", []database.BillStatus{database.BillStatusPaid, database.BillStatusPartial}).
		Order("id ASC").Find(&candidates).Error; err != nil {
		return err
	}
	if len(candidates) == 0 {
		return nil
	}
	// The newest open check is the live fixture the storefront is pointing at.
	keepID := uint(0)
	for _, bill := range candidates {
		if bill.Status == database.BillStatusOpen && bill.ID > keepID {
			keepID = bill.ID
		}
	}
	billIDs := make([]uint, 0, len(candidates))
	for _, bill := range candidates {
		if bill.ID == keepID {
			continue
		}
		var settled int64
		if err := tx.WithContext(ctx).Model(&database.Payment{}).
			Where("bill_id = ? AND status = ?", bill.ID, database.PaymentStatusConfirmed).
			Count(&settled).Error; err != nil {
			return err
		}
		if settled == 0 {
			if err := tx.WithContext(ctx).Model(&database.AlternativePayment{}).
				Where("bill_id = ? AND status = ?", bill.ID, database.AltPaymentStatusConfirmed).
				Count(&settled).Error; err != nil {
				return err
			}
		}
		if settled == 0 {
			if err := tx.WithContext(ctx).Model(&database.FiscalReceipt{}).
				Where("bill_id = ?", bill.ID).Count(&settled).Error; err != nil {
				return err
			}
		}
		if settled > 0 {
			continue
		}
		billIDs = append(billIDs, bill.ID)
	}
	if len(billIDs) == 0 {
		return nil
	}

	var deliveryIDs []uint
	if err := tx.WithContext(ctx).Model(&database.DeliveryOrder{}).
		Where("bill_id IN ?", billIDs).Pluck("id", &deliveryIDs).Error; err != nil {
		return err
	}
	if len(deliveryIDs) > 0 {
		if err := tx.WithContext(ctx).Where("delivery_order_id IN ?", deliveryIDs).
			Delete(&database.DeliveryStatusHistory{}).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Where("id IN ?", deliveryIDs).
			Delete(&database.DeliveryOrder{}).Error; err != nil {
			return err
		}
	}
	for _, model := range []interface{}{
		&database.Order{},
		&database.BillItem{},
		&database.BillSplitShare{},
		&database.BillHistoryEvent{},
		&database.Payment{},
		&database.AlternativePayment{},
	} {
		if err := tx.WithContext(ctx).Where("bill_id IN ?", billIDs).Delete(model).Error; err != nil {
			return err
		}
	}

	// Three more tables carry a BLOCKING foreign key into `bills` — the
	// genesis schema declares business_milestone_events.bill_id,
	// counters.current_bill_id and customer_visits.bill_id with no ON DELETE
	// clause — and none of them is a child this purge owns. A counter is a
	// physical takeaway station, a milestone event is an achievement the
	// operator was already notified about (and is uniquely keyed, so deleting
	// it would let the same milestone fire again), and a customer visit is the
	// guest's own CRM/loyalty history. Release the pointer and keep the row.
	//
	// Without this, the DELETE below raises a constraint violation on Postgres
	// and aborts the whole ensure/re-arm transaction: the clones survive and
	// the hourly re-arm stops working entirely.
	for _, ref := range []struct {
		model  interface{}
		column string
	}{
		{&database.BusinessMilestoneEvent{}, "bill_id"},
		{&database.Counter{}, "current_bill_id"},
		{&database.CustomerVisit{}, "bill_id"},
	} {
		if err := tx.WithContext(ctx).Model(ref.model).
			Where(ref.column+" IN ?", billIDs).
			Update(ref.column, gorm.Expr("NULL")).Error; err != nil {
			return err
		}
	}

	return tx.WithContext(ctx).Where("id IN ?", billIDs).Delete(&database.Bill{}).Error
}

func (s *Service) loadOrMintUnpaidDeliveryPayBill(ctx context.Context, tx *gorm.DB, businessID uint, billNumber string, staffID uint, total totals, itemsJSON string, createdAt, now time.Time) (database.Bill, error) {
	// #855: retire dead/superseded predecessors BEFORE looking for a fixture
	// to reuse. This frees the canonical bill number again, so the hourly
	// re-arm recycles one identity instead of stacking a new unpaid ARS
	// 38.900 check on the collection gap every hour.
	if err := s.purgeStaleDeliveryPayFixtures(ctx, tx, businessID); err != nil {
		return database.Bill{}, err
	}

	// Whatever live fixture survived the purge is the one the storefront is
	// pointing at — canonical or a successor minted after a guest paid.
	var live database.Bill
	err := tx.WithContext(ctx).
		Where("business_id = ? AND notes = ? AND status = ? AND paid_amount = 0",
			businessID, deliveryPayFixtureNote, database.BillStatusOpen).
		Order("id DESC").First(&live).Error
	if err == nil {
		if err := tx.WithContext(ctx).Model(&live).Updates(map[string]interface{}{
			"status":       database.BillStatusOpen,
			"paid_amount":  0,
			"total_amount": total.TotalCents,
			"updated_at":   now,
		}).Error; err != nil {
			return database.Bill{}, err
		}
		return live, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return database.Bill{}, err
	}

	// Nothing live left. A PAID leftover must not be recycled — reopening
	// 762/760 made hourly append + the 15m expiry sweeper restamp closed_at
	// every hour (#772) — so walk candidate identities until one is free.
	// The canonical number comes first: the purge above usually frees it, so
	// the fixture keeps a stable identity instead of drifting once per hour.
	minuteStamp := now.Format("200601021504")
	candidates := []string{
		billNumber,
		demoBillNumber(businessID, "delivery-pay", now.Format("20060102")),
		demoBillNumber(businessID, "delivery-pay", minuteStamp),
		demoBillNumber(businessID, "delivery-pay", minuteStamp, 2),
		demoBillNumber(businessID, "delivery-pay", minuteStamp, 3),
	}
	for _, candidate := range candidates {
		var existing database.Bill
		lookupErr := tx.WithContext(ctx).Where("bill_number = ?", candidate).First(&existing).Error
		if lookupErr == nil {
			continue
		}
		if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return database.Bill{}, lookupErr
		}
		fresh := database.Bill{
			BusinessID:       businessID,
			BillNumber:       candidate,
			Notes:            deliveryPayFixtureNote,
			Items:            itemsJSON,
			Subtotal:         total.SubtotalCents,
			TaxAmount:        total.TaxCents,
			ServiceFeeAmount: total.ServiceCents,
			TotalAmount:      total.TotalCents,
			PaidAmount:       0,
			Status:           database.BillStatusOpen,
			SettlementAddr:   "",
			CreatedByStaffID: &staffID,
			CreatedAt:        createdAt,
			UpdatedAt:        now,
		}
		if createErr := tx.WithContext(ctx).Create(&fresh).Error; createErr != nil {
			// Concurrent ensure (startup goroutine vs hourly cron) can both
			// pass the look-before-create check; the loser adopts the winner.
			if database.IsUniqueConstraintError(createErr) {
				var winner database.Bill
				if fetchErr := tx.WithContext(ctx).Where("bill_number = ?", candidate).First(&winner).Error; fetchErr == nil {
					return winner, nil
				}
			}
			return database.Bill{}, createErr
		}
		return fresh, nil
	}
	return database.Bill{}, fmt.Errorf("demo: no free pay-online bill number for business %d", businessID)
}

func (s *Service) ensureAwaitingPaymentDelivery(ctx context.Context, tx *gorm.DB, businessID uint, p profile, staff []database.Staff) error {
	staffID, err := nthStaffID(ctx, tx, businessID, 0)
	if err != nil {
		return err
	}
	if len(staff) > 0 {
		staffID = staff[0].ID
	}
	now := s.now().UTC()
	createdAt := now.Add(-20 * time.Minute)
	// Match production DeliveryPaymentWindow (15m). A 24h demo window made the
	// config copy ("Unpaid orders expire after 15 minutes") look like a lie and
	// kept the Bills/Kitchen intake card forever.
	expiresAt := now.Add(15 * time.Minute)
	lines := withLineSubtotals([]billLine{
		{MenuItemID: "demo-empanadas", Name: "Empanada de carne", Price: 2900, Quantity: 6},
		{MenuItemID: "demo-milanesa", Name: "Milanesa napolitana", Price: 21500, Quantity: 1},
	})
	// AR carta prices are IVA-final — no added tax line on demo bills.
	total := computeTotals(lines, 0, p.ServiceFeeRate)
	rawLines, err := json.Marshal(lines)
	if err != nil {
		return err
	}
	billNumber := demoBillNumber(businessID, "delivery-pay")
	bill, err := s.loadOrMintUnpaidDeliveryPayBill(ctx, tx, businessID, billNumber, staffID, total, string(rawLines), createdAt, now)
	if err != nil {
		return err
	}

	for idx, line := range lines {
		item := database.BillItem{
			ID:         deterministicUUID("delivery-pay-bill-item", bill.ID, idx),
			BillID:     bill.ID,
			MenuItemID: line.MenuItemID,
			Name:       line.Name,
			Price:      line.Price,
			Quantity:   line.Quantity,
			ItemType:   "menu_item",
			Subtotal:   line.Subtotal,
			CreatedAt:  createdAt.Add(time.Duration(idx+1) * time.Minute),
		}
		if err := tx.WithContext(ctx).Where("id = ?", item.ID).Attrs(item).FirstOrCreate(&item).Error; err != nil {
			return err
		}
	}

	orderItems := []database.OrderItem{
		{ID: "demo-delivery-pay-empanadas", ItemType: "menu_item", MenuItemID: "demo-empanadas", MenuItemName: "Empanada de carne", Quantity: 6, Price: 2900, Subtotal: 17400},
		{ID: "demo-delivery-pay-milanesa", ItemType: "menu_item", MenuItemID: "demo-milanesa", MenuItemName: "Milanesa napolitana", Quantity: 1, Price: 21500, Subtotal: 21500},
	}
	rawOrder, err := json.Marshal(orderItems)
	if err != nil {
		return err
	}
	var order database.Order
	orderNumber := fmt.Sprintf("ORD-%s", bill.BillNumber)
	err = tx.WithContext(ctx).Where("business_id = ? AND order_number = ?", businessID, orderNumber).First(&order).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// One guest-checkout identity per successor bill. A per-business
		// constant collided with orders_guest_request_identity_uq the first
		// time #772 minted a successor for a consumed fixture, and that 23505
		// aborted the entire hourly append transaction — so no new bills and
		// no caja reconcile landed for days (#796).
		requestID := fmt.Sprintf("demo-delivery-pay-%d-%d", businessID, bill.ID)
		order = database.Order{
			BillID:          bill.ID,
			BusinessID:      businessID,
			OrderNumber:     orderNumber,
			Status:          database.OrderStatusPending,
			CreatedBy:       "guest",
			ClientRequestID: &requestID,
			Notes:           "Demo delivery order awaiting payment",
			Items:           string(rawOrder),
			CreatedAt:       createdAt,
			UpdatedAt:       createdAt,
		}
		if err := tx.WithContext(ctx).Create(&order).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	var zone database.DeliveryZone
	if err := tx.WithContext(ctx).Where("business_id = ?", businessID).First(&zone).Error; err != nil {
		return err
	}
	var driver database.DeliveryDriver
	if err := tx.WithContext(ctx).Where("business_id = ? AND is_active = ?", businessID, true).First(&driver).Error; err != nil {
		return err
	}
	deliveryNumber, err := uniqueDeliveryNumberTx(ctx, tx)
	if err != nil {
		return err
	}
	delivery := database.DeliveryOrder{
		BusinessID: businessID, BillID: bill.ID, OrderID: &order.ID, ZoneID: &zone.ID,
		DeliveryNumber: deliveryNumber, DeliveryType: database.DeliveryTypeInHouse, Status: database.DeliveryStatusConfirmed, Priority: database.PriorityHigh,
		FulfillmentMode: "in_house", DriverID: &driver.ID, AssignedAt: ptrTime(createdAt.Add(4 * time.Minute)),
		CustomerName: "Valentina Ocampo", CustomerPhone: demoFakeMobile(988), CustomerEmail: s.demoEmail(fmt.Sprintf("demo+delivery-pay-%d", businessID)), CustomerLocale: "es",
		DeliveryAddress:       database.DeliveryAddress{Street: demoFakeStreet("Carlos Calvo", 720) + ", 2º B", City: "Buenos Aires", State: "CABA", PostalCode: "C1065", Country: "AR", FormattedAddress: demoFakeStreet("Carlos Calvo", 720) + ", 2º B, San Telmo, Buenos Aires"},
		EstimatedPickupTime:   ptrTime(now.Add(25 * time.Minute)),
		EstimatedDeliveryTime: ptrTime(now.Add(55 * time.Minute)),
		DeliveryFee:           290000,
		DriverTip:             200000,
		PlatformFee:           0,
		DeliveryInstructions:  "Tocar timbre 2B. Pagar online antes de que cocina arranque.",
		ContactlessDelivery:   true,
		DeliveryCode:          "2048",
		QuoteMetadata: database.JSONRawMessage(
			`{"demo":true,"awaiting_payment":true,"order_subtotal":38900,"item_count":7}`,
		),
		PaymentExpiresAt: &expiresAt,
		CreatedAt:        createdAt,
		UpdatedAt:        now,
	}
	// Key on bill_id. Delivery numbers are now crypto/rand, so the previous
	// delivery_number cond would create a second row on every re-seed.
	// Attrs supplies create-only values; found rows keep their number unless
	// it is a leaked DEMO-PAY-<id> / DEL-B<id>-<hex> identifier (#530).
	err = tx.WithContext(ctx).Where("bill_id = ?", bill.ID).Attrs(delivery).FirstOrCreate(&delivery).Error
	if err != nil {
		return err
	}
	if guessableDemoDeliveryNumber.MatchString(delivery.DeliveryNumber) {
		rotated, rotErr := uniqueDeliveryNumberTx(ctx, tx)
		if rotErr != nil {
			return rotErr
		}
		if err := tx.WithContext(ctx).Model(&delivery).Update("delivery_number", rotated).Error; err != nil {
			return err
		}
		delivery.DeliveryNumber = rotated
		token, tokErr := uniqueBillPublicTokenTx(ctx, tx)
		if tokErr != nil {
			return tokErr
		}
		if err := tx.WithContext(ctx).Model(&database.Bill{}).Where("id = ?", bill.ID).Update("public_token", token).Error; err != nil {
			return err
		}
	}
	// Re-arm / clamp the payment window:
	//  - missing or already past → fresh 15m window (demo pay page stays usable)
	//  - still open but longer than 15m (legacy 24h seed) → clamp to 15m so the
	//    config copy stays honest and the sweeper can expire the row
	//  - still open within 15m → leave alone so ensure cannot keep extending it
	updates := map[string]interface{}{
		"status":             database.DeliveryStatusConfirmed,
		"payment_expires_at": expiresAt,
		"updated_at":         now,
	}
	return tx.WithContext(ctx).Model(&delivery).Updates(updates).Error
}

func (s *Service) ensureChatAndEngagement(ctx context.Context, tx *gorm.DB, businessID uint, staff []database.Staff, positions []database.Position) error {
	if len(staff) < 2 {
		return nil
	}
	channel := database.ChatChannel{BusinessID: businessID, Type: database.ChatChannelTypeGroup, Name: "Equipo de salón", IsArchived: false, CreatedByStaffID: &staff[0].ID}
	if err := tx.WithContext(ctx).Where("business_id = ? AND name = ?", businessID, channel.Name).Attrs(channel).FirstOrCreate(&channel).Error; err != nil {
		return err
	}
	for _, st := range staff[:2] {
		member := database.ChatChannelMember{ChannelID: channel.ID, StaffID: st.ID, BusinessID: businessID, Role: database.ChatMemberRoleMember, JoinedAt: s.now().UTC()}
		if st.ID == staff[0].ID {
			member.Role = database.ChatMemberRoleAdmin
		}
		if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&member).Error; err != nil {
			return err
		}
	}
	msg := database.ChatMessage{BusinessID: businessID, ChannelID: channel.ID, SenderStaffID: staff[0].ID, SenderName: staff[0].Name, Content: "Pre-turno: esta noche empujemos los postres — el flan casero tiene que salir en cada mesa."}
	if err := tx.WithContext(ctx).FirstOrCreate(&msg, database.ChatMessage{BusinessID: businessID, ChannelID: channel.ID, Content: msg.Content}).Error; err != nil {
		return err
	}
	read := database.ChatRead{ChannelID: channel.ID, StaffID: staff[0].ID, BusinessID: businessID, LastReadMessageID: msg.ID, LastReadAt: s.now().UTC()}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&read).Error; err != nil {
		return err
	}
	ann := database.Announcement{BusinessID: businessID, AuthorStaffID: staff[0].ID, Title: "Noche a salón lleno", Content: "Mesa VIP a las 21:30. Alertas de cocina activadas para todos.", RequireAck: true, AudienceFilter: "all"}
	if err := tx.WithContext(ctx).Where("business_id = ? AND title = ?", businessID, ann.Title).Attrs(ann).FirstOrCreate(&ann).Error; err != nil {
		return err
	}
	ack := database.AnnouncementAck{AnnouncementID: ann.ID, StaffID: staff[1].ID, BusinessID: businessID, AcknowledgedAt: s.now().UTC()}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&ack).Error; err != nil {
		return err
	}
	template := database.ChecklistTemplate{BusinessID: businessID, Name: "Apertura del salón", Kind: database.ChecklistKindOpening, IsActive: true, CreatedByStaffID: staff[0].ID}
	if len(positions) > 0 {
		template.PositionID = &positions[0].ID
	}
	if err := tx.WithContext(ctx).Where("business_id = ? AND name = ?", businessID, template.Name).Attrs(template).FirstOrCreate(&template).Error; err != nil {
		return err
	}
	for idx, label := range []string{"Contar la caja", "Confirmar lista de faltantes (86)", "Revisar papel de la impresora"} {
		item := database.ChecklistItem{BusinessID: businessID, TemplateID: template.ID, Label: label, SortOrder: idx + 1, IsRequired: true}
		if err := tx.WithContext(ctx).Where("template_id = ? AND label = ?", template.ID, label).Attrs(item).FirstOrCreate(&item).Error; err != nil {
			return err
		}
	}
	doc := database.Document{BusinessID: businessID, CreatedByStaffID: staff[0].ID, Title: "Estándares de servicio", Content: "Recibir, acompañar, confirmar el pedido, cerrar la mesa.", Version: 1, RequireAck: true, AudienceFilter: "all", IsActive: true}
	if err := tx.WithContext(ctx).Where("business_id = ? AND title = ?", businessID, doc.Title).Attrs(doc).FirstOrCreate(&doc).Error; err != nil {
		return err
	}
	docAck := database.DocumentAck{BusinessID: businessID, DocumentID: doc.ID, StaffID: staff[1].ID, Version: doc.Version, AcknowledgedAt: s.now().UTC()}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&docAck).Error; err != nil {
		return err
	}
	if err := tx.WithContext(ctx).FirstOrCreate(&database.Shoutout{BusinessID: businessID, FromStaffID: staff[0].ID, ToStaffID: staff[1].ID, Message: "Gran manejo de la mesa 12 — el cliente se fue feliz.", Emoji: "star", Visibility: database.ShoutoutVisibilityTeam}, database.Shoutout{BusinessID: businessID, FromStaffID: staff[0].ID, ToStaffID: staff[1].ID}).Error; err != nil {
		return err
	}
	poll := database.Poll{BusinessID: businessID, AuthorStaffID: staff[0].ID, Question: "¿Qué sugerencia destacamos mañana?", AudienceFilter: "all", Status: database.PollStatusOpen}
	if err := tx.WithContext(ctx).Where("business_id = ? AND question = ?", businessID, poll.Question).Attrs(poll).FirstOrCreate(&poll).Error; err != nil {
		return err
	}
	var firstOption database.PollOption
	for idx, label := range []string{"Ojo de bife", "Mollejas al limón"} {
		opt := database.PollOption{BusinessID: businessID, PollID: poll.ID, Label: label, SortOrder: idx + 1}
		if err := tx.WithContext(ctx).Where("poll_id = ? AND label = ?", poll.ID, label).Attrs(opt).FirstOrCreate(&opt).Error; err != nil {
			return err
		}
		if idx == 0 {
			firstOption = opt
		}
	}
	if firstOption.ID != 0 {
		vote := database.PollVote{BusinessID: businessID, PollID: poll.ID, OptionID: firstOption.ID, StaffID: staff[1].ID}
		if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&vote).Error; err != nil {
			return err
		}
	}
	return nil
}

// demoFiscalCUIT renders a FAKE AR CUIT for a demo venue. The body is a
// sequential documentation placeholder (12345678 / 23456789) that no taxpayer
// holds; only the AFIP mod-11 check digit is real, because the fiscal receiver
// rejects a bad one (#936). Demo fiscal rows are never sent to AFIP
// (Provider "demo", manual mode, sandbox).
func demoFiscalCUIT(body int) string {
	digits := demoValidCUIT(30, body-10000000)
	return fmt.Sprintf("%s-%s-%s", digits[:2], digits[2:10], digits[10:])
}

// ensureFiscalSettings seeds — and repairs — the AR fiscal row. Attrs +
// FirstOrCreate only ever filled a row it created, so demos that have been live
// since before per-venue identities kept one shared placeholder CUIT, a null
// punto de venta, and (when the lookup key drifted off "AR") a row the AR
// lookup no longer matched at all (#936). AR + responsable_inscripto also
// activates the factura A/B/C variety in day_generator's demoAFIPReceiptType.
func (s *Service) ensureFiscalSettings(ctx context.Context, tx *gorm.DB, businessID uint, p profile) error {
	pointOfSale := p.FiscalPointOfSale
	if pointOfSale < 1 {
		pointOfSale = 1
	}
	want := database.BusinessFiscalSettings{
		BusinessID:      businessID,
		Country:         "AR",
		Provider:        "demo",
		Mode:            database.FiscalModeManual,
		Environment:     "sandbox",
		TaxID:           demoFiscalCUIT(p.FiscalCUITBody),
		TaxCondition:    "responsable_inscripto",
		PointOfSale:     &pointOfSale,
		SetupStatus:     "validated",
		LastValidatedAt: ptrTime(s.now().UTC()),
		ProviderConfig:  map[string]interface{}{"demo": true},
	}

	var rows []database.BusinessFiscalSettings
	if err := tx.WithContext(ctx).
		Where("business_id = ?", businessID).
		Order("id ASC").
		Find(&rows).Error; err != nil {
		return err
	}

	// Prefer the canonical AR/demo row; otherwise adopt the oldest legacy row
	// rather than minting a second one the fiscal reader could pick at random.
	var target *database.BusinessFiscalSettings
	for i := range rows {
		if rows[i].Country == want.Country && rows[i].Provider == want.Provider {
			target = &rows[i]
			break
		}
	}
	if target == nil && len(rows) > 0 {
		target = &rows[0]
	}
	if target == nil {
		return tx.WithContext(ctx).Create(&want).Error
	}
	return tx.WithContext(ctx).Model(&database.BusinessFiscalSettings{}).
		Where("id = ?", target.ID).
		Updates(map[string]any{
			"country":       want.Country,
			"provider":      want.Provider,
			"mode":          want.Mode,
			"environment":   want.Environment,
			"tax_id":        want.TaxID,
			"tax_condition": want.TaxCondition,
			"point_of_sale": pointOfSale,
			"setup_status":  want.SetupStatus,
		}).Error
}

func (s *Service) ensureFiscalPrinterAIAndAlerts(ctx context.Context, tx *gorm.DB, adminUserID, businessID uint, staff []database.Staff, p profile) error {
	if err := s.ensureFiscalSettings(ctx, tx, businessID, p); err != nil {
		return err
	}
	var count int64
	if err := tx.WithContext(ctx).Model(&database.Printer{}).Where("business_id = ?", businessID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		printerNames := map[string]string{"bill": "Impresora de cuentas", "kitchen": "Comandera de cocina"}
		for _, role := range []string{"bill", "kitchen"} {
			if err := tx.Create(&database.Printer{BusinessID: businessID, Name: printerNames[role], Role: role, Transport: "browser", PaperWidthMM: 80, CodePage: "CP858", Enabled: true}).Error; err != nil {
				return err
			}
		}
	}
	usage := database.AIImageUsage{BusinessID: businessID, DailyUsed: 6, DailyPeriodStart: time.Date(s.now().UTC().Year(), s.now().UTC().Month(), s.now().UTC().Day(), 0, 0, 0, 0, time.UTC), MonthlyUsed: 6, MonthlyAnchorDay: 1, MonthlyPeriodStart: time.Date(s.now().UTC().Year(), s.now().UTC().Month(), 1, 0, 0, 0, 0, time.UTC)}
	if err := tx.WithContext(ctx).Where("business_id = ?", businessID).Attrs(usage).FirstOrCreate(&usage).Error; err != nil {
		return err
	}
	img := database.AIGeneratedImage{BusinessID: businessID, S3Key: fmt.Sprintf("demo/admin-%d/business-%d/hero.webp", adminUserID, businessID), Source: "generate", Model: "demo-seed", CreatedAt: s.now().UTC()}
	if err := tx.WithContext(ctx).Where("business_id = ? AND s3_key = ?", businessID, img.S3Key).Attrs(img).FirstOrCreate(&img).Error; err != nil {
		return err
	}
	// MIN-4 / #820: director demo copy is operator-facing — it follows the demo
	// owner's UI language first (the business default language is the guest
	// menu tier and only serves as a fallback).
	var dirBiz database.Business
	_ = tx.WithContext(ctx).Select("id", "default_language", "source_language").First(&dirBiz, businessID).Error
	var dirOwner database.User
	_ = tx.WithContext(ctx).Select("id", "language_selected").First(&dirOwner, adminUserID).Error
	dirCopy := directorDemoSeed(demoOperatorLocale(dirOwner.LanguageSelected, &dirBiz))
	// #820 self-heal: a reseed after the operator/guest language diverged must
	// convert a previously seeded wrong-language card IN PLACE — never leave a
	// voseo briefing on EN chrome or a duplicate pair of pinned cards.
	staleCopy := directorDemoSeed("en")
	if dirCopy.Locale == "en" {
		staleCopy = directorDemoSeed("es")
	}
	var staleThread database.DirectorConsoleThread
	if err := tx.WithContext(ctx).Where("business_id = ? AND title = ? AND pinned = ?", businessID, staleCopy.ThreadTitle, true).First(&staleThread).Error; err == nil {
		// B7: renaming is only safe when no current-locale thread already
		// exists — otherwise the rename itself would mint the duplicate pair
		// of identically-titled pinned cards #820 complained about. With a
		// live twin present, the stale card is archived and unpinned instead.
		var currentTwins int64
		if err := tx.WithContext(ctx).Model(&database.DirectorConsoleThread{}).
			Where("business_id = ? AND title = ? AND archived_at IS NULL", businessID, dirCopy.ThreadTitle).
			Count(&currentTwins).Error; err != nil {
			return err
		}
		if currentTwins > 0 {
			archivedAt := s.now().UTC()
			if err := tx.WithContext(ctx).Model(&database.DirectorConsoleThread{}).Where("id = ?", staleThread.ID).
				Updates(map[string]interface{}{"pinned": false, "archived_at": archivedAt}).Error; err != nil {
				return err
			}
		} else {
			if err := tx.WithContext(ctx).Model(&database.DirectorConsoleThread{}).Where("id = ?", staleThread.ID).
				Updates(map[string]interface{}{"title": dirCopy.ThreadTitle, "locale": dirCopy.Locale}).Error; err != nil {
				return err
			}
			if err := tx.WithContext(ctx).Model(&database.DirectorConsoleMessage{}).
				Where("thread_id = ? AND model_name = ?", staleThread.ID, "demo-seed").
				Updates(map[string]interface{}{"locale": dirCopy.Locale, "content": dirCopy.Content, "structured_response": dirCopy.Structured}).Error; err != nil {
				return err
			}
		}
	}
	thread := database.DirectorConsoleThread{BusinessID: businessID, Title: dirCopy.ThreadTitle, Locale: dirCopy.Locale, LastMessageAt: s.now().UTC(), Pinned: true}
	if err := tx.WithContext(ctx).Where("business_id = ? AND title = ?", businessID, thread.Title).Attrs(thread).FirstOrCreate(&thread).Error; err != nil {
		return err
	}
	// #820: FirstOrCreate/Attrs only ever writes on CREATE, so a card whose title
	// already matches the current copy but whose locale drifted was left wrong by
	// every reseed — the rename branch above only fires for a stale-TITLE card.
	// Live thread 10 is exactly that hole: title "Demo Director Briefing" (the
	// English seed) carrying locale es-AR. Converge the label in place.
	if thread.Locale != dirCopy.Locale {
		if err := tx.WithContext(ctx).Model(&database.DirectorConsoleThread{}).Where("id = ?", thread.ID).
			Update("locale", dirCopy.Locale).Error; err != nil {
			return err
		}
		thread.Locale = dirCopy.Locale
	}
	msg := database.DirectorConsoleMessage{ThreadID: thread.ID, BusinessID: businessID, Role: database.DirectorMessageRoleAssistant, Locale: dirCopy.Locale, Content: dirCopy.Content, StructuredResponse: dirCopy.Structured, ModelName: "demo-seed", LatencyMs: 120, CreatedAt: s.now().UTC()}
	if err := tx.WithContext(ctx).Where("thread_id = ? AND content = ?", thread.ID, msg.Content).Attrs(msg).FirstOrCreate(&msg).Error; err != nil {
		return err
	}
	// Same hole one level down: the seeded message is keyed on CONTENT, so a row
	// whose text already matches keeps whatever locale it was last stamped with.
	if msg.Locale != dirCopy.Locale {
		if err := tx.WithContext(ctx).Model(&database.DirectorConsoleMessage{}).Where("id = ?", msg.ID).
			Update("locale", dirCopy.Locale).Error; err != nil {
			return err
		}
		msg.Locale = dirCopy.Locale
	}
	// Proposed action targets the flan (AR$ 8.900 → AR$ 9.500): a believable
	// inflation-era price nudge on the highest-velocity dessert.
	action := database.DirectorProposedAction{PublicID: fmt.Sprintf("demo_admin_%d_business_%d", adminUserID, businessID), BusinessID: businessID, ThreadID: thread.ID, MessageID: &msg.ID, Kind: "menu_price_suggestion", ParamsJSON: `{"item_id":"demo-flan"}`, PreviewJSON: `{"price":9500}`, MenuVersion: 1, Status: database.DirectorProposalPending, CreatedAt: s.now().UTC(), ExpiresAt: s.now().UTC().AddDate(0, 0, 7)}
	if err := tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&action).Error; err != nil {
		return err
	}
	alertSettings := database.DefaultBusinessAlertSettings(businessID)
	if err := tx.WithContext(ctx).Where("business_id = ?", businessID).Attrs(alertSettings).FirstOrCreate(&alertSettings).Error; err != nil {
		return err
	}
	if len(staff) > 0 {
		var notif database.StaffNotification
		if err := tx.WithContext(ctx).Where("business_id = ? AND staff_id = ? AND kind = ?", businessID, staff[0].ID, "demo").Attrs(database.StaffNotification{BusinessID: businessID, StaffID: staff[0].ID, Kind: "demo", Title: "Demo lista", Body: "Tu demo aislada de Payverge está lista.", URL: fmt.Sprintf("/business/%d/dashboard", businessID), CreatedAt: s.now().UTC()}).FirstOrCreate(&notif).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ensureAccountingPayrollLoyaltyAndPlugins(ctx context.Context, tx *gorm.DB, adminUserID, businessID uint, staff []database.Staff) error {
	if err := s.ensureManualLedger(ctx, tx, adminUserID, businessID); err != nil {
		return err
	}
	if err := s.ensurePayroll(ctx, tx, adminUserID, businessID, staff); err != nil {
		return err
	}
	if err := s.ensureLoyalty(ctx, tx, businessID); err != nil {
		return err
	}
	if err := s.ensureMarketing(ctx, tx, businessID); err != nil {
		return err
	}
	return s.ensurePluginConfigs(ctx, tx, businessID)
}

// ensureMarketing seeds a couple of Offers and one Bundle so the AI-enabled
// Marketing/Offers surfaces are not bare in the demo. Idempotent: each row is
// keyed on its stable business-scoped natural key (business_id + name) via
// Where(...).Attrs(...).FirstOrCreate, so drift-prone timestamps never leak
// into the lookup and reseeds create no duplicates. Offer.DiscountValue and
// Bundle.Price are plain float64 pesos (these models are NOT in the
// cents/MarshalJSON contract list).
func (s *Service) ensureMarketing(ctx context.Context, tx *gorm.DB, businessID uint) error {
	const weekdayMaskMondayToFriday int16 = 62
	// Lunch daypart: 12:00–16:00 local minutes-from-midnight (porteño lunch runs
	// late). Without bounds the weekday lunch offer incorrectly auto-applied at
	// dinner (Guest QA #78).
	lunchStartMinute := 12 * 60
	lunchEndMinute := 16 * 60

	now := s.now().UTC()
	start := now.AddDate(0, 0, -7)
	end := now.AddDate(0, 0, 30)
	bifeTarget := "demo-bife"
	offers := []database.Offer{
		{BusinessID: businessID, Name: "Almuerzo de semana 15% off", Description: "15% de descuento en todo el pedido, de lunes a viernes de 12 a 16 h.", Image: promoImage("almuerzo-15"), DiscountType: "percentage", DiscountValue: 15, StartDate: &start, EndDate: &end, WeekdayMask: weekdayMaskMondayToFriday, StartMinute: &lunchStartMinute, EndMinute: &lunchEndMinute, IsActive: true, ApplicableTo: "all", CreatedAt: now, UpdatedAt: now},
		{BusinessID: businessID, Name: "AR$ 3.000 menos en el bife", Description: "Tres mil pesos de descuento en nuestro bife de chorizo a la parrilla.", Image: menuImage("demo-bife"), DiscountType: "fixed", DiscountValue: 3000, StartDate: &start, EndDate: &end, IsActive: true, ApplicableTo: "item", TargetID: &bifeTarget, CreatedAt: now, UpdatedAt: now},
	}
	for idx := range offers {
		var out database.Offer
		query := tx.WithContext(ctx).
			Where("business_id = ? AND name = ?", businessID, offers[idx].Name).
			Attrs(offers[idx])
		// Demo offers are managed showroom fixtures, so the idempotent ensure
		// keeps this one seed-owned schedule and copy in sync with the seed.
		// Keep the assignment narrowly scoped to the stable demo natural key
		// rather than rewriting customer offers that happen to have similar copy.
		if offers[idx].Name == "Almuerzo de semana 15% off" {
			query = query.Assign(map[string]interface{}{
				"weekday_mask": offers[idx].WeekdayMask,
				"start_minute": offers[idx].StartMinute,
				"end_minute":   offers[idx].EndMinute,
				"description":  offers[idx].Description,
				"image":        offers[idx].Image,
			})
		}
		if err := query.FirstOrCreate(&out).Error; err != nil {
			return err
		}
	}

	bundleItems := []database.BundleItemRef{
		{MenuItemID: "demo-parrillada", Name: "Parrillada para dos", Quantity: 1},
		{MenuItemID: "demo-malbec-botella", Name: "Botella de Malbec", Quantity: 1},
		{MenuItemID: "demo-flan", Name: "Flan casero", Quantity: 2},
	}
	itemsJSON, err := json.Marshal(bundleItems)
	if err != nil {
		return err
	}
	// Bundle hero uses the parrillada photo; the fixed offer already owns the
	// bife photo, so Marketing posts keep one image per dish in the demo seed.
	// À-la-carte value is AR$ 114.300 (68.000 + 28.500 + 2×8.900); the bundle
	// sells at AR$ 105.000 so the discount is visible but believable.
	bundle := database.Bundle{BusinessID: businessID, Name: "Noche de parrilla para dos", Description: "Parrillada completa para dos, botella de Malbec y dos flanes caseros para cerrar.", Price: 105000.00, Currency: "ARS", Image: s.hostedAsset(menuImage("demo-parrillada")), Items: string(itemsJSON), IsActive: true, CreatedAt: now, UpdatedAt: now}
	var outBundle database.Bundle
	// Attrs alone cannot repair a stale Image on an existing Date Night row.
	// Assign the seed-owned photo (and fixture fields) so Marketing preview
	// and photo_ready stay aligned after reload.
	return tx.WithContext(ctx).
		Where("business_id = ? AND name = ?", businessID, bundle.Name).
		Attrs(bundle).
		Assign(map[string]interface{}{
			"image":       bundle.Image,
			"description": bundle.Description,
			"price":       bundle.Price,
			"currency":    bundle.Currency,
			"items":       bundle.Items,
			"is_active":   bundle.IsActive,
		}).
		FirstOrCreate(&outBundle).Error
}

func (s *Service) ensureManualLedger(ctx context.Context, tx *gorm.DB, adminUserID, businessID uint) error {
	now := s.now().UTC()
	// Amounts are int64 ARS cents (money wire contract): AR$ 2.500.000 alquiler,
	// AR$ 680.000 expensas y servicios, AR$ 1.850.000 seña de evento privado.
	entries := []database.ManualLedgerEntry{
		{BusinessID: businessID, EntryType: database.AccountingEntryTypeExpense, Category: "rent", Amount: 250000000, Currency: "ARS", OccurredAt: now.AddDate(0, 0, -25), Description: "Alquiler del local (mes en curso)", Notes: "Asiento manual de demo para el panel contable", Reference: fmt.Sprintf("demo-%d-rent", businessID), CreatedByUserID: &adminUserID},
		{BusinessID: businessID, EntryType: database.AccountingEntryTypeExpense, Category: "utilities", Amount: 68000000, Currency: "ARS", OccurredAt: now.AddDate(0, 0, -14), Description: "Expensas y servicios (luz, gas, agua)", Notes: "Asiento manual de demo para el panel contable", Reference: fmt.Sprintf("demo-%d-utilities", businessID), CreatedByUserID: &adminUserID},
		{BusinessID: businessID, EntryType: database.AccountingEntryTypeIncome, Category: "catering", Amount: 185000000, Currency: "ARS", OccurredAt: now.AddDate(0, 0, -9), Description: "Seña de evento privado (cena de empresa)", Notes: "Ejemplo de ingreso manual", Reference: fmt.Sprintf("demo-%d-catering", businessID), CreatedByUserID: &adminUserID},
	}
	for idx := range entries {
		var out database.ManualLedgerEntry
		if err := tx.WithContext(ctx).Where("business_id = ? AND reference = ?", businessID, entries[idx].Reference).Attrs(entries[idx]).FirstOrCreate(&out).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ensurePayroll(ctx context.Context, tx *gorm.DB, adminUserID, businessID uint, staff []database.Staff) error {
	if len(staff) == 0 {
		return nil
	}
	// Idempotent: if paid runs already exist for this business, only re-scale
	// amounts. Recreating by period_start equality is fragile across SQLite
	// date/timestamp storage and previously doubled line items on re-ensure.
	var existingCount int64
	if err := tx.WithContext(ctx).Model(&database.PayrollRun{}).
		Where("business_id = ? AND status = ?", businessID, database.PayrollRunStatusPaid).
		Count(&existingCount).Error; err != nil {
		return err
	}
	if existingCount == 0 {
		now := s.now().UTC()
		currentWeek := weekStartMonday(normalizeBusinessDate(now, time.UTC))
		for weekOffset := 1; weekOffset <= 4; weekOffset++ {
			periodStart := currentWeek.AddDate(0, 0, -7*weekOffset)
			periodEnd := periodStart.AddDate(0, 0, 6)
			paidAt := periodEnd.Add(14 * time.Hour)
			run := database.PayrollRun{
				BusinessID:      businessID,
				PeriodStart:     periodStart,
				PeriodEnd:       periodEnd,
				Status:          database.PayrollRunStatusPaid,
				Currency:        "ARS",
				PaidAt:          &paidAt,
				PaidByUserID:    &adminUserID,
				CreatedByUserID: &adminUserID,
				Notes:           "Sueldos semanales de demo",
				GrossTotal:      0,
				BonusTotal:      0,
				DeductionTotal:  0,
				NetTotal:        0,
				CreatedAt:       paidAt,
				UpdatedAt:       paidAt,
			}
			if err := tx.WithContext(ctx).Create(&run).Error; err != nil {
				return err
			}
			for _, st := range staff {
				line := database.PayrollLineItem{
					PayrollRunID: run.ID,
					BusinessID:   businessID,
					PayeeType:    database.PayrollPayeeTypeStaff,
					StaffID:      &st.ID,
					PayeeName:    st.Name,
					Notes:        "Línea de sueldo de demo",
					CreatedAt:    paidAt,
					UpdatedAt:    paidAt,
				}
				if err := tx.WithContext(ctx).Create(&line).Error; err != nil {
					return err
				}
			}
		}
	}
	return s.scalePayrollToRevenue(ctx, tx, businessID, staff)
}

// scalePayrollToRevenue sets paid payroll net total to TargetPayrollRevenueRatio
// of paid bill revenue (capped at MaxPayrollRevenueRatio). When no revenue
// exists yet (static ensure before day gen), falls back to a profile estimate.
func (s *Service) scalePayrollToRevenue(ctx context.Context, tx *gorm.DB, businessID uint, staff []database.Staff) error {
	if len(staff) == 0 {
		return nil
	}
	var revenueCents int64
	if err := tx.WithContext(ctx).Model(&database.Bill{}).
		Where("business_id = ? AND status = ?", businessID, database.BillStatusPaid).
		Select("COALESCE(SUM(total_amount), 0)").Scan(&revenueCents).Error; err != nil {
		return err
	}
	if revenueCents <= 0 {
		// Rough estimate: ~7 bills/day × average AR$ 60.000 (6M cents) × baseline days.
		revenueCents = int64(s.baselineDays) * 7 * 6000000
	}
	targetNetAllWeeks := int64(float64(revenueCents) * TargetPayrollRevenueRatio)
	if max := int64(float64(revenueCents) * MaxPayrollRevenueRatio); targetNetAllWeeks > max {
		targetNetAllWeeks = max
	}
	// Floor so payroll still shows as present (AR$ 100.000/week per staff).
	if targetNetAllWeeks < int64(len(staff))*4*10000000 {
		targetNetAllWeeks = int64(len(staff)) * 4 * 10000000
	}

	var runs []database.PayrollRun
	if err := tx.WithContext(ctx).
		Where("business_id = ? AND status = ?", businessID, database.PayrollRunStatusPaid).
		Order("period_start ASC").
		Find(&runs).Error; err != nil {
		return err
	}
	if len(runs) == 0 {
		return nil
	}
	// Even split across weeks; within a week, weight by staff index slightly.
	weekNet := targetNetAllWeeks / int64(len(runs))
	remainder := targetNetAllWeeks - weekNet*int64(len(runs))

	for i, run := range runs {
		thisWeek := weekNet
		if i == len(runs)-1 {
			thisWeek += remainder
		}
		// Weight: staff[i] gets (base + i) share units.
		weightSum := 0
		for idx := range staff {
			weightSum += 10 + idx
		}
		var grossTotal, bonusTotal, deductionTotal, netTotal int64
		for idx, st := range staff {
			share := thisWeek * int64(10+idx) / int64(weightSum)
			if idx == len(staff)-1 {
				// Absorb rounding into last staff member.
				share = thisWeek - netTotal
			}
			// gross = net + deduction - bonus with small fixed-ratio pieces.
			bonus := share / 20
			deduction := share / 10
			gross := share - bonus + deduction
			if gross < 0 {
				gross = share
				bonus = 0
				deduction = 0
			}
			net := gross + bonus - deduction
			// Re-align net to share exactly.
			if net != share {
				gross = share - bonus + deduction
				net = share
			}
			grossTotal += gross
			bonusTotal += bonus
			deductionTotal += deduction
			netTotal += net

			if err := tx.WithContext(ctx).Model(&database.PayrollLineItem{}).
				Where("payroll_run_id = ? AND staff_id = ?", run.ID, st.ID).
				Updates(map[string]interface{}{
					"gross_amount":     gross,
					"bonus_amount":     bonus,
					"deduction_amount": deduction,
					"net_amount":       net,
					"updated_at":       s.now().UTC(),
				}).Error; err != nil {
				return err
			}
		}
		if err := tx.WithContext(ctx).Model(&run).Updates(map[string]interface{}{
			"gross_total":     grossTotal,
			"bonus_total":     bonusTotal,
			"deduction_total": deductionTotal,
			"net_total":       netTotal,
			"updated_at":      s.now().UTC(),
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ensureLoyalty(ctx context.Context, tx *gorm.DB, businessID uint) error {
	// ARS scale: earn 0.01 pts per peso with redeem 1 pt/peso ≈ 1% cashback —
	// keeps point balances readable (a AR$ 60.000 dinner earns 600 pts).
	program := database.LoyaltyProgram{
		BusinessID:                businessID,
		Enabled:                   true,
		PointsPerDollar:           0.01,
		RedemptionPointsPerDollar: 1.0,
	}
	if err := tx.WithContext(ctx).Where("business_id = ?", businessID).Attrs(program).FirstOrCreate(&program).Error; err != nil {
		return err
	}
	if err := tx.WithContext(ctx).Model(&program).Updates(map[string]interface{}{
		"enabled":                      true,
		"points_per_dollar":            0.01,
		"redemption_points_per_dollar": 1.0,
	}).Error; err != nil {
		return err
	}
	// Thresholds are ARS cents: Plata at AR$ 150.000, Oro at AR$ 450.000
	// lifetime spend — a handful of dinners, believable for a regular.
	tiers := []database.LoyaltyTier{
		{LoyaltyProgramID: program.ID, Name: "Bronce", MinLifetimeSpentCents: 0, SortOrder: 0, Color: "#b45309"},
		{LoyaltyProgramID: program.ID, Name: "Plata", MinLifetimeSpentCents: 15000000, SortOrder: 1, Color: "#64748b"},
		{LoyaltyProgramID: program.ID, Name: "Oro", MinLifetimeSpentCents: 45000000, SortOrder: 2, Color: "#d97706"},
	}
	// Delete-and-recreate so reseeding is idempotent: a per-name FirstOrCreate
	// leaves behind any pre-existing duplicate/drifted rows (e.g. a second
	// "Bronze at $0"), so wipe the ladder first, then insert exactly the three
	// seeded tiers. Twice-seeding now always yields exactly this set.
	if err := tx.WithContext(ctx).Where("loyalty_program_id = ?", program.ID).Delete(&database.LoyaltyTier{}).Error; err != nil {
		return err
	}
	for idx := range tiers {
		if err := tx.WithContext(ctx).Create(&tiers[idx]).Error; err != nil {
			return err
		}
	}
	return nil
}

// sanitizeDemoCryptoSettlement clears leftover placeholder settlement
// addresses so a showcase tenant can never be paired with a live USDC rail.
// sanitizeDemoCryptoSettlement strips both payout wallets from the venue and
// from every bill it has ever issued. The tipping wallet rides on the guest
// check payload next to the settlement one, so a demo that takes no crypto has
// to clear both or it keeps offering guests an on-chain tip rail (#856). Runs
// on every ensure, so demos seeded before this heal on the next tick.
func (s *Service) sanitizeDemoCryptoSettlement(ctx context.Context, tx *gorm.DB, businessID uint) error {
	wallets := map[string]interface{}{"settlement_addr": "", "tipping_addr": ""}
	if err := tx.WithContext(ctx).Model(&database.Business{}).
		Where("id = ?", businessID).
		Updates(wallets).Error; err != nil {
		return err
	}
	return tx.WithContext(ctx).Model(&database.Bill{}).
		Where("business_id = ?", businessID).
		Updates(wallets).Error
}

func (s *Service) ensurePluginConfigs(ctx context.Context, tx *gorm.DB, businessID uint) error {
	type pluginSpec struct {
		plugin  database.Plugin
		enabled bool
		config  map[string]interface{}
	}
	specs := []pluginSpec{
		{
			plugin:  database.Plugin{Name: "mercadopago", DisplayName: "MercadoPago", Description: "Demo card and local checkout", Message: "Accept cards and local payment methods in demo mode.", Image: "/images/plugins/mercadopago-logo.png", Category: database.PluginCategoryPayment, Version: "1.0.0", Features: `["Cards","Local methods","Demo sandbox"]`, ConfigSchema: `{}`, IsActive: true},
			enabled: false,
			config:  map[string]interface{}{"demo": true, "enabled": false, "environment": "sandbox", "country": "AR", "connection_mode": "manual"},
		},
		{
			plugin:  database.Plugin{Name: "usdc_payment", DisplayName: "USDC Payment", Description: "Demo USDC payment option", Message: "Optional crypto rail — not the primary checkout path.", Image: "/images/plugins/usdc.png", Category: database.PluginCategoryPayment, Version: "1.0.0", Features: `["Optional crypto checkout"]`, ConfigSchema: `{}`, IsActive: true},
			enabled: false,
			config:  map[string]interface{}{"enabled": false, "show_recommended": false, "demo": true},
		},
		{
			plugin:  database.Plugin{Name: "cross_chain_payment", DisplayName: "Cross-chain Payment", Description: "Demo any-token checkout", Message: "Optional crypto rail — disabled on showcase tenants.", Image: "/images/plugins/cross_chain_payment-logo.png", Category: database.PluginCategoryPayment, Version: "1.0.0", Features: `["Optional crypto checkout"]`, ConfigSchema: `{}`, IsActive: true},
			enabled: false,
			config:  map[string]interface{}{"enabled": false, "show_recommended": false, "demo": true},
		},
		{
			plugin:  database.Plugin{Name: "telegram", DisplayName: "Telegram", Description: "Demo Telegram notifications", Message: "Connect Telegram notifications for demo operations.", Image: "/images/plugins/telegram.svg", Category: database.PluginCategoryIntegration, Version: "1.0.0", Features: `["Order notifications","Daily summaries"]`, ConfigSchema: `{}`, IsActive: true},
			enabled: false,
			config:  map[string]interface{}{"demo": true, "notifications": map[string]interface{}{"order_created": true, "payment_received": true}},
		},
		{
			plugin:  database.Plugin{Name: "stripe", DisplayName: "Stripe", Description: "Demo Stripe card payments", Message: "Stripe card payments are staged in test mode.", Image: "/images/plugins/stripe.png", Category: database.PluginCategoryPayment, Version: "1.0.0", Features: `["Test mode","Card payments"]`, ConfigSchema: `{}`, IsActive: true},
			enabled: false,
			config:  map[string]interface{}{"demo": true, "test_mode": true, "enabled": false},
		},
	}
	now := s.now().UTC()
	for _, spec := range specs {
		// plugins is a GLOBAL table the plugin system pre-populates at startup with
		// its own display_name/description/price. Match on name ONLY — Attrs carries
		// the create-time values without adding them to the WHERE — so an existing
		// global plugin row is reused (we only need its ID) instead of missing the
		// match and triggering a duplicate INSERT on idx_plugins_name (23505).
		plugin := database.Plugin{}
		if err := tx.WithContext(ctx).Where("name = ?", spec.plugin.Name).Attrs(spec.plugin).FirstOrCreate(&plugin).Error; err != nil {
			return err
		}
		raw, err := json.Marshal(spec.config)
		if err != nil {
			return err
		}
		// #795: last_status is webhook health, not decoration. Stamping "ok" on
		// rails seeded disabled made every showcase venue advertise healthy
		// payment rails while all of them were off. Only an enabled rail gets
		// seeded health; disabled rows keep empty health, and the Updates pass
		// self-heals rows the old seeder already stamped.
		lastStatus := ""
		var lastSuccessPtr *time.Time
		var lastSuccessVal interface{}
		if spec.enabled {
			lastStatus = "ok"
			lastSuccessPtr = &now
			lastSuccessVal = now
		}
		bp := database.BusinessPlugin{BusinessID: businessID, PluginID: plugin.ID, IsEnabled: spec.enabled, Config: string(raw), LastStatus: lastStatus, LastSuccessAt: lastSuccessPtr}
		var out database.BusinessPlugin
		if err := tx.WithContext(ctx).Where("business_id = ? AND plugin_id = ?", businessID, plugin.ID).Attrs(bp).FirstOrCreate(&out).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Model(&out).Updates(map[string]interface{}{"config": string(raw), "is_enabled": spec.enabled, "last_status": lastStatus, "last_success_at": lastSuccessVal}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ensureScheduleWorkflowData(ctx context.Context, tx *gorm.DB, businessID uint, staff []database.Staff, positions []database.Position) error {
	if len(staff) < 3 || len(positions) == 0 {
		return nil
	}
	var schedule database.Schedule
	if err := tx.WithContext(ctx).Where("business_id = ?", businessID).Order("week_start DESC").First(&schedule).Error; err != nil {
		return err
	}
	now := s.now().UTC()
	// Venue walls for workflow seeds (open shift + pending time-off). UTC
	// wall clocks previously rendered as 2:00–10:00 AM and sat off-week (#249).
	loc := database.ResolveLocation(defaultTimezone)
	position := positions[0]
	if len(positions) > 1 {
		position = positions[1]
	}
	// Saturday dinner open shift in the venue zone (not WeekStart+12h UTC,
	// which collapsed to 4:00 AM–12:00 PM for America/New_York operators — #247).
	// Normalize to venue-local midnight before +5d so DST / non-midnight WeekStart
	// cannot skew the day (#249 + #247).
	weekLocal := schedule.WeekStart.In(loc)
	openDay := time.Date(weekLocal.Year(), weekLocal.Month(), weekLocal.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 5)
	openStart := time.Date(openDay.Year(), openDay.Month(), openDay.Day(), 16, 0, 0, 0, loc)
	openEnd := openStart.Add(6 * time.Hour)
	openShift := database.Shift{
		BusinessID:       businessID,
		ScheduleID:       schedule.ID,
		PositionID:       position.ID,
		StartsAt:         openStart.UTC(),
		EndsAt:           openEnd.UTC(),
		BreakMinutes:     30,
		Status:           database.ShiftStatusOpen,
		Published:        true,
		Notes:            "Turno abierto: falta cobertura para el sábado a la noche",
		CreatedByStaffID: staff[0].ID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	var existingOpen database.Shift
	err := tx.WithContext(ctx).
		Where("business_id = ? AND schedule_id = ? AND position_id = ? AND starts_at = ? AND staff_id IS NULL", businessID, schedule.ID, position.ID, openStart.UTC()).
		First(&existingOpen).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := tx.WithContext(ctx).Create(&openShift).Error; err != nil {
			return err
		}
		existingOpen = openShift
	} else if err != nil {
		return err
	}

	claim := database.OpenShiftClaim{BusinessID: businessID, ShiftID: existingOpen.ID, ClaimingStaffID: staff[1].ID, Status: database.OpenClaimStatusPending, CreatedAt: now, UpdatedAt: now}
	if err := tx.WithContext(ctx).Where("business_id = ? AND shift_id = ? AND claiming_staff_id = ?", businessID, existingOpen.ID, staff[1].ID).Attrs(claim).FirstOrCreate(&claim).Error; err != nil {
		return err
	}

	var filled database.Shift
	if err := tx.WithContext(ctx).Where("business_id = ? AND staff_id = ? AND status = ?", businessID, staff[1].ID, database.ShiftStatusFilled).First(&filled).Error; err == nil {
		acceptingStaffID := staff[2].ID
		swap := database.ShiftSwapRequest{BusinessID: businessID, ShiftID: filled.ID, RequestingStaffID: staff[1].ID, Kind: database.SwapKindGiveup, Target: database.SwapTargetAllInRole, Status: database.SwapStatusPendingApproval, AcceptingStaffID: &acceptingStaffID, CreatedAt: now, UpdatedAt: now}
		if err := tx.WithContext(ctx).Where("business_id = ? AND shift_id = ? AND requesting_staff_id = ? AND kind = ?", businessID, filled.ID, staff[1].ID, database.SwapKindGiveup).Attrs(swap).FirstOrCreate(&swap).Error; err != nil {
			return err
		}
	}

	// Pending time-off on Wednesday of the displayed week, 09:00–17:00 venue-local.
	timeOffDay := schedule.WeekStart.In(loc).AddDate(0, 0, 2)
	timeOffStart := time.Date(timeOffDay.Year(), timeOffDay.Month(), timeOffDay.Day(), 9, 0, 0, 0, loc).UTC()
	timeOffEnd := timeOffStart.Add(8 * time.Hour)
	timeOff := database.TimeOffRequest{BusinessID: businessID, StaffID: staff[2].ID, StartsAt: timeOffStart, EndsAt: timeOffEnd, Reason: "Trámite familiar", Status: database.TimeOffStatusPending, CreatedAt: now, UpdatedAt: now}
	if err := tx.WithContext(ctx).Where("business_id = ? AND staff_id = ? AND starts_at = ?", businessID, staff[2].ID, timeOffStart).Attrs(timeOff).FirstOrCreate(&timeOff).Error; err != nil {
		return err
	}
	// Heal older UTC/+8-day seeds so re-ensure does not leave the off-week row.
	_ = tx.WithContext(ctx).
		Where("business_id = ? AND staff_id = ? AND status = ? AND starts_at <> ?", businessID, staff[2].ID, database.TimeOffStatusPending, timeOffStart).
		Delete(&database.TimeOffRequest{}).Error

	note := database.ShiftNote{BusinessID: businessID, ShiftID: &existingOpen.ID, ForDate: normalizeBusinessDate(now, loc), AuthorStaffID: staff[0].ID, Category: database.ShiftNoteCategoryStaffing, Content: "Pase de turno: hay un pedido de cobertura abierto para revisar.", CreatedAt: now}
	if err := tx.WithContext(ctx).Where("business_id = ? AND for_date = ? AND author_staff_id = ? AND category = ?", businessID, note.ForDate, note.AuthorStaffID, note.Category).Attrs(note).FirstOrCreate(&note).Error; err != nil {
		return err
	}
	return nil
}

func ptrTime(t time.Time) *time.Time { return &t }
