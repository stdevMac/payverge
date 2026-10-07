package database

import "strings"

// ReservationGuestHistory summarizes a guest's prior reservation outcomes at
// one business, keyed by lowercased customer_email. Computed in a single
// grouped query for a page of pending approval requests (no N+1).
type ReservationGuestHistory struct {
	PriorNoShows int64 `json:"prior_no_shows"`
	PriorVisits  int64 `json:"prior_visits"`
}

type reservationGuestHistoryRow struct {
	Email        string
	PriorNoShows int64
	PriorVisits  int64
}

// GetReservationGuestHistory returns, per email, how many prior reservations
// at this business ended as no_show vs. an actual visit (seated/completed).
// Emails are matched case-insensitively; blanks are skipped.
func GetReservationGuestHistory(businessID uint, emails []string) (map[string]ReservationGuestHistory, error) {
	normalized := make([]string, 0, len(emails))
	seen := make(map[string]struct{}, len(emails))
	for _, email := range emails {
		e := strings.ToLower(strings.TrimSpace(email))
		if e == "" {
			continue
		}
		if _, dup := seen[e]; dup {
			continue
		}
		seen[e] = struct{}{}
		normalized = append(normalized, e)
	}
	result := make(map[string]ReservationGuestHistory, len(normalized))
	if len(normalized) == 0 {
		return result, nil
	}

	var rows []reservationGuestHistoryRow
	err := db.Raw(`
		SELECT
			LOWER(customer_email) AS email,
			SUM(CASE WHEN status = 'no_show' THEN 1 ELSE 0 END) AS prior_no_shows,
			SUM(CASE WHEN status IN ('seated', 'completed') THEN 1 ELSE 0 END) AS prior_visits
		FROM table_reservations
		WHERE business_id = ?
			AND customer_email <> ''
			AND LOWER(customer_email) IN ?
		GROUP BY LOWER(customer_email)
	`, businessID, normalized).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.Email] = ReservationGuestHistory{
			PriorNoShows: row.PriorNoShows,
			PriorVisits:  row.PriorVisits,
		}
	}
	return result, nil
}
