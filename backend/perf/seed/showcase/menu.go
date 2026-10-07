package main

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// menuItemSpec describes a single menu item. We keep the full menu definition
// in code so it's reviewable and reproducible — no external CSV/JSON files.
type menuItemSpec struct {
	ID          string
	Name        string
	Description string
	Price       float64
	Allergens   []string
	Dietary     []string // vegetarian, vegan, gluten_free, dairy_free, spicy, popular, chef_choice
}

// menuCategorySpec wraps a category name + its items.
type menuCategorySpec struct {
	ID    string
	Name  string
	Desc  string
	Items []menuItemSpec
}

// showcaseMenu returns the full Trattoria Bella Vista menu. 6 categories,
// ~34 named items with descriptions, prices, allergens and dietary tags.
func showcaseMenu() []menuCategorySpec {
	return []menuCategorySpec{
		{
			ID:   "showcase-bellavista-cat-antipasti",
			Name: "Antipasti",
			Desc: "Small plates to start, perfect for sharing",
			Items: []menuItemSpec{
				{
					ID: "showcase-item-burrata", Name: "Burrata Pugliese",
					Description: "Creamy burrata from Puglia, heirloom tomatoes, basil oil, sourdough crostini.",
					Price:       16.00,
					Allergens:   []string{"dairy", "gluten"},
					Dietary:     []string{"vegetarian", "popular"},
				},
				{
					ID: "showcase-item-bruschetta", Name: "Bruschetta al Pomodoro",
					Description: "Toasted Tuscan bread, San Marzano tomatoes, garlic, basil, extra-virgin olive oil.",
					Price:       11.00,
					Allergens:   []string{"gluten"},
					Dietary:     []string{"vegan"},
				},
				{
					ID: "showcase-item-carpaccio", Name: "Carpaccio di Manzo",
					Description: "Thin-sliced beef tenderloin, arugula, shaved parmigiano, lemon, capers.",
					Price:       18.00,
					Allergens:   []string{"dairy"},
					Dietary:     []string{"gluten_free", "chef_choice"},
				},
				{
					ID: "showcase-item-calamari", Name: "Calamari Fritti",
					Description: "Crispy fried calamari, lemon aioli, marinara, parsley.",
					Price:       15.00,
					Allergens:   []string{"shellfish", "eggs", "gluten"},
					Dietary:     []string{},
				},
				{
					ID: "showcase-item-tagliere", Name: "Tagliere Italiano",
					Description: "Prosciutto di Parma, sopressata, taleggio, pecorino, olives, grilled bread.",
					Price:       22.00,
					Allergens:   []string{"dairy", "gluten"},
					Dietary:     []string{"chef_choice"},
				},
			},
		},
		{
			ID:   "showcase-bellavista-cat-pasta",
			Name: "Pasta Fresca",
			Desc: "Hand-rolled every morning — ask for gluten-free pasta",
			Items: []menuItemSpec{
				{
					ID: "showcase-item-tagliatelle", Name: "Tagliatelle alla Bolognese",
					Description: "Hand-cut tagliatelle, slow-braised beef and pork ragù, parmigiano.",
					Price:       22.00,
					Allergens:   []string{"gluten", "dairy", "eggs"},
					Dietary:     []string{"popular"},
				},
				{
					ID: "showcase-item-cacio", Name: "Cacio e Pepe",
					Description: "Tonnarelli, aged pecorino romano, fresh-cracked Tellicherry pepper.",
					Price:       19.00,
					Allergens:   []string{"gluten", "dairy", "eggs"},
					Dietary:     []string{"vegetarian"},
				},
				{
					ID: "showcase-item-carbonara", Name: "Spaghetti Carbonara",
					Description: "Spaghetti, guanciale, egg yolk, pecorino, black pepper. No cream — the Roman way.",
					Price:       21.00,
					Allergens:   []string{"gluten", "eggs", "dairy"},
					Dietary:     []string{"popular"},
				},
				{
					ID: "showcase-item-vongole", Name: "Linguine alle Vongole",
					Description: "Manila clams, white wine, garlic, chili flake, parsley.",
					Price:       26.00,
					Allergens:   []string{"gluten", "shellfish"},
					Dietary:     []string{"dairy_free"},
				},
				{
					ID: "showcase-item-ravioli", Name: "Ravioli di Ricotta e Spinaci",
					Description: "Ricotta and spinach ravioli, brown butter, crispy sage.",
					Price:       20.00,
					Allergens:   []string{"gluten", "dairy", "eggs"},
					Dietary:     []string{"vegetarian"},
				},
				{
					ID: "showcase-item-arrabbiata", Name: "Penne all'Arrabbiata",
					Description: "Penne, San Marzano tomato, garlic, Calabrian chili, parsley.",
					Price:       18.00,
					Allergens:   []string{"gluten"},
					Dietary:     []string{"vegan", "spicy"},
				},
				{
					ID: "showcase-item-lasagna", Name: "Lasagna della Nonna",
					Description: "Layered house pasta, beef-veal ragù, béchamel, parmigiano, mozzarella. Baked to order.",
					Price:       24.00,
					Allergens:   []string{"gluten", "dairy", "eggs"},
					Dietary:     []string{"chef_choice"},
				},
				{
					ID: "showcase-item-pappardelle", Name: "Pappardelle al Cinghiale",
					Description: "Wide pappardelle, slow-braised wild boar, Chianti red wine, rosemary.",
					Price:       28.00,
					Allergens:   []string{"gluten", "dairy", "eggs"},
					Dietary:     []string{"chef_choice"},
				},
			},
		},
		{
			ID:   "showcase-bellavista-cat-pizza",
			Name: "Pizza al Forno",
			Desc: "Wood-fired Neapolitan dough — 48-hour fermented",
			Items: []menuItemSpec{
				{
					ID: "showcase-item-margherita", Name: "Margherita D.O.C.",
					Description: "San Marzano tomato, mozzarella di bufala, basil, EVOO, sea salt.",
					Price:       18.00,
					Allergens:   []string{"gluten", "dairy"},
					Dietary:     []string{"vegetarian", "popular"},
				},
				{
					ID: "showcase-item-diavola", Name: "Diavola",
					Description: "Tomato, mozzarella, spicy sopressata, Calabrian chili, basil.",
					Price:       20.00,
					Allergens:   []string{"gluten", "dairy"},
					Dietary:     []string{"spicy"},
				},
				{
					ID: "showcase-item-quattro", Name: "Quattro Formaggi",
					Description: "Mozzarella, gorgonzola dolce, taleggio, parmigiano, honey drizzle.",
					Price:       22.00,
					Allergens:   []string{"gluten", "dairy"},
					Dietary:     []string{"vegetarian"},
				},
				{
					ID: "showcase-item-prosciutto-funghi", Name: "Prosciutto e Funghi",
					Description: "Tomato, mozzarella, prosciutto cotto, wild mushrooms, parsley.",
					Price:       23.00,
					Allergens:   []string{"gluten", "dairy"},
					Dietary:     []string{},
				},
				{
					ID: "showcase-item-capricciosa", Name: "Capricciosa",
					Description: "Tomato, mozzarella, prosciutto, mushrooms, artichokes, olives, egg.",
					Price:       24.00,
					Allergens:   []string{"gluten", "dairy", "eggs"},
					Dietary:     []string{},
				},
				{
					ID: "showcase-item-tartufo", Name: "Tartufo Nero",
					Description: "Fior di latte, fontina, black truffle paste, fresh shaved black truffle, EVOO.",
					Price:       32.00,
					Allergens:   []string{"gluten", "dairy"},
					Dietary:     []string{"vegetarian", "chef_choice"},
				},
			},
		},
		{
			ID:   "showcase-bellavista-cat-mains",
			Name: "Secondi",
			Desc: "Mains from land and sea",
			Items: []menuItemSpec{
				{
					ID: "showcase-item-ossobuco", Name: "Osso Buco alla Milanese",
					Description: "Slow-braised veal shank, gremolata, saffron risotto.",
					Price:       42.00,
					Allergens:   []string{"dairy"},
					Dietary:     []string{"chef_choice"},
				},
				{
					ID: "showcase-item-branzino", Name: "Branzino al Forno",
					Description: "Whole Mediterranean sea bass, lemon, capers, roasted potatoes, salsa verde.",
					Price:       38.00,
					Allergens:   []string{"fish"},
					Dietary:     []string{"gluten_free", "dairy_free"},
				},
				{
					ID: "showcase-item-pollo", Name: "Pollo Parmigiana",
					Description: "Hand-breaded chicken breast, marinara, mozzarella, parmigiano, spaghetti.",
					Price:       28.00,
					Allergens:   []string{"gluten", "dairy", "eggs"},
					Dietary:     []string{"popular"},
				},
				{
					ID: "showcase-item-bistecca", Name: "Bistecca Fiorentina (per lb)",
					Description: "Dry-aged Tuscan-style porterhouse, rosemary olive oil, sea salt. Served rare.",
					Price:       58.00,
					Allergens:   []string{},
					Dietary:     []string{"gluten_free", "dairy_free", "chef_choice"},
				},
				{
					ID: "showcase-item-saltimbocca", Name: "Saltimbocca alla Romana",
					Description: "Veal scaloppine, prosciutto, sage, marsala butter, sautéed spinach.",
					Price:       34.00,
					Allergens:   []string{"dairy"},
					Dietary:     []string{},
				},
			},
		},
		{
			ID:   "showcase-bellavista-cat-dolci",
			Name: "Dolci",
			Desc: "House-made desserts",
			Items: []menuItemSpec{
				{
					ID: "showcase-item-tiramisu", Name: "Tiramisù Classico",
					Description: "Espresso-soaked savoiardi, mascarpone cream, cocoa.",
					Price:       11.00,
					Allergens:   []string{"gluten", "dairy", "eggs"},
					Dietary:     []string{"vegetarian", "popular"},
				},
				{
					ID: "showcase-item-cannoli", Name: "Cannoli Siciliani",
					Description: "Crisp shells, sweet ricotta, pistachio, candied orange.",
					Price:       10.00,
					Allergens:   []string{"gluten", "dairy", "nuts"},
					Dietary:     []string{"vegetarian"},
				},
				{
					ID: "showcase-item-pannacotta", Name: "Panna Cotta ai Frutti di Bosco",
					Description: "Vanilla bean panna cotta, mixed berry compote.",
					Price:       11.00,
					Allergens:   []string{"dairy"},
					Dietary:     []string{"vegetarian", "gluten_free"},
				},
				{
					ID: "showcase-item-affogato", Name: "Affogato al Caffè",
					Description: "Vanilla gelato drowned in a shot of single-origin espresso.",
					Price:       9.00,
					Allergens:   []string{"dairy"},
					Dietary:     []string{"vegetarian", "gluten_free"},
				},
			},
		},
		{
			ID:   "showcase-bellavista-cat-drinks",
			Name: "Drinks",
			Desc: "Wine, spritz, and digestivi",
			Items: []menuItemSpec{
				{
					ID: "showcase-item-chianti", Name: "House Chianti (glass)",
					Description: "Sangiovese-based table red from Castellina in Chianti.",
					Price:       12.00,
					Allergens:   []string{"so2"},
					Dietary:     []string{},
				},
				{
					ID: "showcase-item-spritz", Name: "Aperol Spritz",
					Description: "Aperol, prosecco, soda, orange slice.",
					Price:       14.00,
					Allergens:   []string{"so2"},
					Dietary:     []string{},
				},
				{
					ID: "showcase-item-negroni", Name: "Negroni",
					Description: "Campari, sweet vermouth, gin, orange peel.",
					Price:       15.00,
					Allergens:   []string{},
					Dietary:     []string{},
				},
				{
					ID: "showcase-item-espresso", Name: "Espresso",
					Description: "Single-origin Italian roast, pulled to order.",
					Price:       4.00,
					Allergens:   []string{},
					Dietary:     []string{"vegan", "gluten-free"},
				},
				{
					ID: "showcase-item-pellegrino", Name: "San Pellegrino (500ml)",
					Description: "Sparkling mineral water.",
					Price:       6.00,
					Allergens:   []string{},
					Dietary:     []string{"vegan", "gluten-free"},
				},
				{
					ID: "showcase-item-grappa", Name: "Grappa di Barolo",
					Description: "Single-vineyard grappa, aged in oak.",
					Price:       16.00,
					Allergens:   []string{"so2"},
					Dietary:     []string{},
				},
			},
		},
	}
}

// seedMenu upserts the showcase menu via FirstOrCreate on `business_id`.
// If a menu already exists for the business, the existing rows pass through
// untouched (so re-running the seed never overwrites manual edits).
func seedMenu(ctx context.Context, db *gorm.DB, bizID uint) error {
	cats := showcaseMenu()
	dbCats := make([]database.MenuCategory, 0, len(cats))
	for i, c := range cats {
		items := make([]database.MenuItem, 0, len(c.Items))
		for j, it := range c.Items {
			items = append(items, database.MenuItem{
				ID:          it.ID,
				Name:        it.Name,
				Description: it.Description,
				Price:       it.Price,
				Currency:    "USD",
				Allergens:   it.Allergens,
				DietaryTags: it.Dietary,
				IsAvailable: true,
				SortOrder:   j,
			})
		}
		dbCats = append(dbCats, database.MenuCategory{
			ID:          c.ID,
			Name:        c.Name,
			Description: c.Desc,
			Items:       items,
			SortOrder:   i,
		})
	}
	catsJSON, err := json.Marshal(dbCats)
	if err != nil {
		return fmt.Errorf("marshal categories: %w", err)
	}
	menu := database.Menu{}
	if err := db.WithContext(ctx).
		Where(database.Menu{BusinessID: bizID}).
		Attrs(database.Menu{
			Categories: string(catsJSON),
			IsActive:   true,
			Version:    1,
		}).
		FirstOrCreate(&menu).Error; err != nil {
		return fmt.Errorf("upsert menu: %w", err)
	}
	return nil
}

// menuItemPrices returns a map from menu_item_id → price, used by order/bill
// seeders so totals are consistent with the seeded menu.
func menuItemPrices() map[string]float64 {
	out := map[string]float64{}
	for _, c := range showcaseMenu() {
		for _, it := range c.Items {
			out[it.ID] = it.Price
		}
	}
	return out
}

// menuItemNames returns id → display name (for OrderItem.MenuItemName).
func menuItemNames() map[string]string {
	out := map[string]string{}
	for _, c := range showcaseMenu() {
		for _, it := range c.Items {
			out[it.ID] = it.Name
		}
	}
	return out
}
