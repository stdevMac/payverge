package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stdevmac/payverge/backend/internal/database"
)

type customerSpec struct {
	Email       string
	Name        string
	Phone       string
	LoyaltyTier string
	Points      int
	Spent       float64
	Visits      int
	LastVisitAt time.Time
	Tags        []string
	Notes       string
	Favorites   []string
	Allergies   []string
}

func showcaseCustomers() []customerSpec {
	now := time.Now().UTC()
	return []customerSpec{
		{
			Email: "amelia.rivera@example.com", Name: "Amelia Rivera", Phone: "+1 (415) 555-1001",
			LoyaltyTier: "Maestro", Points: 4280, Spent: 2840.50, Visits: 47,
			LastVisitAt: now.Add(-2 * 24 * time.Hour),
			Tags:        []string{"VIP", "Wine lover", "Anniversary 6/14"},
			Notes:       "Prefers patio. Allergic to shellfish. Always orders the Pappardelle.",
			Favorites:   []string{"showcase-item-pappardelle", "showcase-item-tartufo", "showcase-item-tiramisu"},
			Allergies:   []string{"shellfish"},
		},
		{
			Email: "james.chen@example.com", Name: "James Chen", Phone: "+1 (415) 555-1002",
			LoyaltyTier: "Famiglia", Points: 1820, Spent: 980.25, Visits: 22,
			LastVisitAt: now.Add(-5 * 24 * time.Hour),
			Tags:        []string{"Regular", "Tech"},
			Notes:       "Comes Tuesdays after work. Likes window seat.",
			Favorites:   []string{"showcase-item-carbonara", "showcase-item-margherita"},
		},
		{
			Email: "priya.shah@example.com", Name: "Priya Shah", Phone: "+1 (415) 555-1003",
			LoyaltyTier: "Famiglia", Points: 1340, Spent: 720.80, Visits: 18,
			LastVisitAt: now.Add(-1 * 24 * time.Hour),
			Tags:        []string{"Vegetarian", "Date-night"},
			Notes:       "Vegetarian. Husband eats fish only — recommend branzino or pasta.",
			Favorites:   []string{"showcase-item-ravioli", "showcase-item-quattro"},
			Allergies:   []string{},
		},
		{
			Email: "carlos.mendes@example.com", Name: "Carlos Mendes", Phone: "+1 (415) 555-1004",
			LoyaltyTier: "Cucina", Points: 480, Spent: 240.00, Visits: 8,
			LastVisitAt: now.Add(-10 * 24 * time.Hour),
			Tags:        []string{"Local"},
			Notes:       "Lives down the block. Picks up takeout on weeknights.",
			Favorites:   []string{"showcase-item-margherita"},
		},
		{
			Email: "rachel.gold@example.com", Name: "Rachel Gold", Phone: "+1 (415) 555-1005",
			LoyaltyTier: "Cucina", Points: 320, Spent: 410.00, Visits: 6,
			LastVisitAt: now.Add(-14 * 24 * time.Hour),
			Tags:        []string{"Birthday 3/8"},
			Notes:       "Brings her book club every other month.",
		},
		{
			Email: "ethan.brooks@example.com", Name: "Ethan Brooks", Phone: "+1 (415) 555-1006",
			LoyaltyTier: "Famiglia", Points: 1610, Spent: 845.30, Visits: 19,
			LastVisitAt: now.Add(-3 * 24 * time.Hour),
			Tags:        []string{"Wine"},
			Notes:       "Often pairs Barolo with the bistecca. Generous tipper.",
			Favorites:   []string{"showcase-item-bistecca", "showcase-item-grappa"},
		},
		{
			Email: "sofia.lopez@example.com", Name: "Sofia Lopez", Phone: "+1 (415) 555-1007",
			LoyaltyTier: "Welcome", Points: 80, Spent: 65.20, Visits: 2,
			LastVisitAt: now.Add(-7 * 24 * time.Hour),
			Tags:        []string{"New"},
			Notes:       "First-timer in October. Came back two weeks later — flag for follow-up.",
		},
		{
			Email: "noah.adams@example.com", Name: "Noah Adams", Phone: "+1 (415) 555-1008",
			LoyaltyTier: "Maestro", Points: 5120, Spent: 3240.75, Visits: 52,
			LastVisitAt: now.Add(-1 * 24 * time.Hour),
			Tags:        []string{"VIP", "Owner friend"},
			Notes:       "Marco's old college friend — always says hi.",
			Favorites:   []string{"showcase-item-ossobuco", "showcase-item-chianti"},
		},
		{
			Email: "maya.singh@example.com", Name: "Maya Singh", Phone: "+1 (415) 555-1009",
			LoyaltyTier: "Cucina", Points: 510, Spent: 285.40, Visits: 9,
			LastVisitAt: now.Add(-21 * 24 * time.Hour),
			Tags:        []string{"Gluten-free"},
			Notes:       "Celiac — always confirms the gluten-free pasta before ordering.",
			Favorites:   []string{"showcase-item-branzino"},
			Allergies:   []string{"gluten"},
		},
		{
			Email: "lucas.foster@example.com", Name: "Lucas Foster", Phone: "+1 (415) 555-1010",
			LoyaltyTier: "Welcome", Points: 40, Spent: 28.50, Visits: 1,
			LastVisitAt: now.Add(-30 * 24 * time.Hour),
			Tags:        []string{"Bar regular"},
			Notes:       "Stops by for a Negroni after work.",
		},
		{
			Email: "isabella.green@example.com", Name: "Isabella Green", Phone: "+1 (415) 555-1011",
			LoyaltyTier: "Famiglia", Points: 980, Spent: 540.20, Visits: 14,
			LastVisitAt: now.Add(-4 * 24 * time.Hour),
			Tags:        []string{"Family"},
			Notes:       "Brings the kids on Sundays. Loves the lasagna.",
			Favorites:   []string{"showcase-item-lasagna", "showcase-item-cannoli"},
		},
		{
			Email: "henry.walsh@example.com", Name: "Henry Walsh", Phone: "+1 (415) 555-1012",
			LoyaltyTier: "Cucina", Points: 240, Spent: 195.00, Visits: 5,
			LastVisitAt: now.Add(-11 * 24 * time.Hour),
			Tags:        []string{"Takeout"},
			Notes:       "Only orders pickup. Lives in SoMa.",
		},
		{
			Email: "zoe.nakamura@example.com", Name: "Zoe Nakamura", Phone: "+1 (415) 555-1013",
			LoyaltyTier: "Cucina", Points: 380, Spent: 312.40, Visits: 7,
			LastVisitAt: now.Add(-6 * 24 * time.Hour),
			Tags:        []string{"Wine pairing"},
			Notes:       "Loves the sommelier nights.",
		},
		{
			Email: "miguel.santos@example.com", Name: "Miguel Santos", Phone: "+1 (415) 555-1014",
			LoyaltyTier: "Famiglia", Points: 1240, Spent: 660.10, Visits: 17,
			LastVisitAt: now.Add(-8 * 24 * time.Hour),
			Tags:        []string{"Couple", "Friday"},
			Notes:       "Date night Fridays with his husband. Loves the bar.",
		},
		{
			Email: "eleanor.hayes@example.com", Name: "Eleanor Hayes", Phone: "+1 (415) 555-1015",
			LoyaltyTier: "Welcome", Points: 120, Spent: 84.00, Visits: 2,
			LastVisitAt: now.Add(-17 * 24 * time.Hour),
			Tags:        []string{"Local press"},
			Notes:       "Writer at SF Gate — be welcoming.",
		},
	}
}

// seedCustomers inserts 15 customers and their CRM business connections.
// Idempotent on customer.email and (customer_id, business_id) for the join.
func seedCustomers(ctx context.Context, db *gorm.DB, bizID uint) error {
	specs := showcaseCustomers()

	customers := make([]database.Customer, 0, len(specs))
	for _, s := range specs {
		customers = append(customers, database.Customer{
			Email:         s.Email,
			Name:          s.Name,
			Phone:         s.Phone,
			IsActive:      true,
			EmailVerified: true,
		})
	}
	if err := db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "email"}},
			DoNothing: true,
		}).
		CreateInBatches(customers, 50).Error; err != nil {
		return fmt.Errorf("create customers: %w", err)
	}

	// Re-load customers by email to capture DB-assigned IDs.
	emails := make([]string, 0, len(specs))
	for _, s := range specs {
		emails = append(emails, s.Email)
	}
	var loaded []database.Customer
	if err := db.WithContext(ctx).
		Where("email IN ?", emails).
		Find(&loaded).Error; err != nil {
		return fmt.Errorf("reload customers: %w", err)
	}
	byEmail := make(map[string]database.Customer, len(loaded))
	for _, c := range loaded {
		byEmail[c.Email] = c
	}

	connections := make([]database.CustomerBusiness, 0, len(specs))
	for _, s := range specs {
		c, ok := byEmail[s.Email]
		if !ok {
			continue
		}
		lastVisit := s.LastVisitAt
		favsJSON, _ := json.Marshal(s.Favorites)
		tagsJSON, _ := json.Marshal(s.Tags)
		allergiesJSON, _ := json.Marshal(s.Allergies)
		connections = append(connections, database.CustomerBusiness{
			CustomerID:     c.ID,
			BusinessID:     bizID,
			LoyaltyPoints:  s.Points,
			LoyaltyTier:    s.LoyaltyTier,
			TotalSpent:     s.Spent,
			VisitCount:     s.Visits,
			LastVisitAt:    &lastVisit,
			FirstVisitAt:   lastVisit.Add(-time.Duration(s.Visits*14) * 24 * time.Hour),
			OptInMarketing: true,
			OptInEmail:     true,
			FavoriteItems:  string(favsJSON),
			Allergies:      string(allergiesJSON),
			Notes:          s.Notes,
			Tags:           string(tagsJSON),
			IsActive:       true,
		})
	}

	if err := db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "customer_id"}, {Name: "business_id"}},
			DoNothing: true,
		}).
		CreateInBatches(connections, 50).Error; err != nil {
		return fmt.Errorf("create customer_businesses: %w", err)
	}

	return nil
}
