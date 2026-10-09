package services

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"gorm.io/gorm"
)

// DriverPerformanceDTO is the per-driver scorecard returned to the
// admin dashboard. It surfaces operational efficiency metrics derived
// from the driver's delivery history.
type DriverPerformanceDTO struct {
	DriverID   uint   `json:"driver_id"`
	DriverName string `json:"driver_name"`
	Status     string `json:"status"`
	IsActive   bool   `json:"is_active"`

	// Throughput counters
	CompletedToday   int `json:"completed_today"`
	CompletedWeek    int `json:"completed_week"`
	CompletedAllTime int `json:"completed_all_time"`
	CancelledCount   int `json:"cancelled_count"`
	FailedCount      int `json:"failed_count"`
	InProgressCount  int `json:"in_progress_count"`

	// Time efficiency. Pointers so "no data yet" stays distinguishable from zero.
	AvgPickupMinutes   *float64 `json:"avg_pickup_minutes"`
	AvgDeliveryMinutes *float64 `json:"avg_delivery_minutes"`
	OnTimeRate         *float64 `json:"on_time_rate"`

	// Quality
	AverageRating *float64 `json:"average_rating"`

	// Money collected on delivered orders. Read-only — payouts are out of scope.
	GrossFeesCollected float64 `json:"gross_fees_collected"`
	GrossTipsCollected float64 `json:"gross_tips_collected"`

	// Active queue. Empty when no live work.
	ActiveQueue []DriverQueueItem `json:"active_queue"`
}

// DriverQueueItem is one in-flight order assigned to a driver.
type DriverQueueItem struct {
	DeliveryID            uint       `json:"delivery_id"`
	DeliveryNumber        string     `json:"delivery_number"`
	Status                string     `json:"status"`
	CustomerName          string     `json:"customer_name"`
	AddressShort          string     `json:"address_short"`
	AssignedAt            *time.Time `json:"assigned_at"`
	EstimatedDeliveryTime *time.Time `json:"estimated_delivery_time"`
	DeliveryFee           float64    `json:"delivery_fee"`
	DriverTip             float64    `json:"driver_tip"`
}

type driverPerformanceAggregateRow struct {
	DriverID           uint
	CompletedAllTime   int
	CompletedToday     int
	CompletedWeek      int
	CancelledCount     int
	FailedCount        int
	PickupSeconds      *float64
	PickupSamples      int
	DeliverySeconds    *float64
	DeliverySamples    int
	OnTimeHits         int
	OnTimeSamples      int
	RatingSum          *float64
	RatingSamples      int
	GrossFeesCollected int64
	GrossTipsCollected int64
}

// GetDriverPerformance returns the full scorecard for a single driver,
// including the live queue.
func (s *DeliveryService) GetDriverPerformance(businessID, driverID uint) (*DriverPerformanceDTO, error) {
	var driver database.DeliveryDriver
	if err := s.db.Where("id = ? AND business_id = ?", driverID, businessID).First(&driver).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("driver not found")
		}
		return nil, fmt.Errorf("failed to load driver: %w", err)
	}
	return s.computeDriverPerformance(&driver, time.Now().In(s.businessLocation(businessID)))
}

// ListDriverPerformance returns scorecards for every driver of a business,
// sorted by completed-this-week desc (with name as a stable tiebreaker).
// The active queue is included on each card so the leaderboard can show a
// "currently delivering N" badge.
func (s *DeliveryService) ListDriverPerformance(businessID uint) ([]*DriverPerformanceDTO, error) {
	drivers, err := s.GetBusinessDrivers(businessID)
	if err != nil {
		return nil, err
	}
	now := time.Now().In(s.businessLocation(businessID))
	driverIDs := make([]uint, 0, len(drivers))
	for i := range drivers {
		driverIDs = append(driverIDs, drivers[i].ID)
	}

	aggregatesByDriver, err := s.loadDriverPerformanceAggregates(businessID, driverIDs, now)
	if err != nil {
		return nil, fmt.Errorf("failed to load driver delivery aggregates: %w", err)
	}
	activeQueuesByDriver, err := s.loadDriverActiveQueues(businessID, driverIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to load active driver deliveries: %w", err)
	}

	out := make([]*DriverPerformanceDTO, 0, len(drivers))
	for i := range drivers {
		dto := driverPerformanceDTOFromAggregate(&drivers[i], aggregatesByDriver[drivers[i].ID], activeQueuesByDriver[drivers[i].ID])
		out = append(out, dto)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CompletedWeek != out[j].CompletedWeek {
			return out[i].CompletedWeek > out[j].CompletedWeek
		}
		return out[i].DriverName < out[j].DriverName
	})
	return out, nil
}

func (s *DeliveryService) secondsBetweenExpression(endColumn, startColumn string) string {
	if s.db.Name() == "sqlite" {
		return fmt.Sprintf("strftime('%%s', %s) - strftime('%%s', %s)", endColumn, startColumn)
	}
	return fmt.Sprintf("EXTRACT(EPOCH FROM (%s - %s))", endColumn, startColumn)
}

func (s *DeliveryService) loadDriverPerformanceAggregates(businessID uint, driverIDs []uint, now time.Time) (map[uint]driverPerformanceAggregateRow, error) {
	if len(driverIDs) == 0 {
		return map[uint]driverPerformanceAggregateRow{}, nil
	}
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).UTC()
	startOfWeek := startOfDay.AddDate(0, 0, -7)
	pickupSecondsExpr := s.secondsBetweenExpression("actual_pickup_time", "assigned_at")
	deliverySecondsExpr := s.secondsBetweenExpression("actual_delivery_time", "assigned_at")

	var rows []driverPerformanceAggregateRow
	err := s.db.Model(&database.DeliveryOrder{}).
		Select(fmt.Sprintf(`
			driver_id,
			SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) AS completed_all_time,
			SUM(CASE WHEN status = ? AND actual_delivery_time >= ? THEN 1 ELSE 0 END) AS completed_today,
			SUM(CASE WHEN status = ? AND actual_delivery_time >= ? THEN 1 ELSE 0 END) AS completed_week,
			SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) AS cancelled_count,
			SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) AS failed_count,
			SUM(CASE WHEN status = ? AND assigned_at IS NOT NULL AND actual_pickup_time IS NOT NULL AND actual_pickup_time > assigned_at THEN %s ELSE 0 END) AS pickup_seconds,
			SUM(CASE WHEN status = ? AND assigned_at IS NOT NULL AND actual_pickup_time IS NOT NULL AND actual_pickup_time > assigned_at THEN 1 ELSE 0 END) AS pickup_samples,
			SUM(CASE WHEN status = ? AND assigned_at IS NOT NULL AND actual_delivery_time IS NOT NULL AND actual_delivery_time > assigned_at THEN %s ELSE 0 END) AS delivery_seconds,
			SUM(CASE WHEN status = ? AND assigned_at IS NOT NULL AND actual_delivery_time IS NOT NULL AND actual_delivery_time > assigned_at THEN 1 ELSE 0 END) AS delivery_samples,
			SUM(CASE WHEN status = ? AND estimated_delivery_time IS NOT NULL AND actual_delivery_time IS NOT NULL AND actual_delivery_time <= estimated_delivery_time THEN 1 ELSE 0 END) AS on_time_hits,
			SUM(CASE WHEN status = ? AND estimated_delivery_time IS NOT NULL AND actual_delivery_time IS NOT NULL THEN 1 ELSE 0 END) AS on_time_samples,
			SUM(CASE WHEN status = ? AND customer_rating IS NOT NULL THEN customer_rating ELSE 0 END) AS rating_sum,
			SUM(CASE WHEN status = ? AND customer_rating IS NOT NULL THEN 1 ELSE 0 END) AS rating_samples,
			COALESCE(SUM(CASE WHEN status = ? THEN delivery_fee ELSE 0 END), 0) AS gross_fees_collected,
			COALESCE(SUM(CASE WHEN status = ? THEN driver_tip ELSE 0 END), 0) AS gross_tips_collected
		`, pickupSecondsExpr, deliverySecondsExpr),
			database.DeliveryStatusDelivered,
			database.DeliveryStatusDelivered, startOfDay,
			database.DeliveryStatusDelivered, startOfWeek,
			database.DeliveryStatusCancelled,
			database.DeliveryStatusFailed,
			database.DeliveryStatusDelivered,
			database.DeliveryStatusDelivered,
			database.DeliveryStatusDelivered,
			database.DeliveryStatusDelivered,
			database.DeliveryStatusDelivered,
			database.DeliveryStatusDelivered,
			database.DeliveryStatusDelivered,
			database.DeliveryStatusDelivered,
			database.DeliveryStatusDelivered,
			database.DeliveryStatusDelivered,
		).
		Where("business_id = ? AND driver_id IN ?", businessID, driverIDs).
		Group("driver_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	byDriver := make(map[uint]driverPerformanceAggregateRow, len(rows))
	for _, row := range rows {
		byDriver[row.DriverID] = row
	}
	return byDriver, nil
}

func (s *DeliveryService) loadDriverActiveQueues(businessID uint, driverIDs []uint) (map[uint][]database.DeliveryOrder, error) {
	if len(driverIDs) == 0 {
		return map[uint][]database.DeliveryOrder{}, nil
	}
	statuses := []database.DeliveryStatus{
		database.DeliveryStatusAssigned,
		database.DeliveryStatusPickedUp,
		database.DeliveryStatusInTransit,
		database.DeliveryStatusNearby,
	}
	var orders []database.DeliveryOrder
	if err := s.db.Where("business_id = ? AND driver_id IN ? AND status IN ?", businessID, driverIDs, statuses).
		Order("assigned_at DESC NULLS LAST, created_at DESC").
		Find(&orders).Error; err != nil {
		return nil, err
	}
	byDriver := make(map[uint][]database.DeliveryOrder, len(driverIDs))
	for _, order := range orders {
		if order.DriverID == nil {
			continue
		}
		byDriver[*order.DriverID] = append(byDriver[*order.DriverID], order)
	}
	return byDriver, nil
}

func driverPerformanceDTOFromAggregate(driver *database.DeliveryDriver, row driverPerformanceAggregateRow, queue []database.DeliveryOrder) *DriverPerformanceDTO {
	dto := &DriverPerformanceDTO{
		DriverID:           driver.ID,
		DriverName:         driver.Name,
		Status:             string(driver.Status),
		IsActive:           driver.IsActive,
		CompletedToday:     row.CompletedToday,
		CompletedWeek:      row.CompletedWeek,
		CompletedAllTime:   row.CompletedAllTime,
		CancelledCount:     row.CancelledCount,
		FailedCount:        row.FailedCount,
		InProgressCount:    len(queue),
		GrossFeesCollected: centsToDollarsFloat(row.GrossFeesCollected),
		GrossTipsCollected: centsToDollarsFloat(row.GrossTipsCollected),
		ActiveQueue:        []DriverQueueItem{},
	}
	if row.PickupSamples > 0 && row.PickupSeconds != nil {
		v := *row.PickupSeconds / float64(row.PickupSamples) / 60.0
		dto.AvgPickupMinutes = &v
	}
	if row.DeliverySamples > 0 && row.DeliverySeconds != nil {
		v := *row.DeliverySeconds / float64(row.DeliverySamples) / 60.0
		dto.AvgDeliveryMinutes = &v
	}
	if row.OnTimeSamples > 0 {
		v := float64(row.OnTimeHits) / float64(row.OnTimeSamples)
		dto.OnTimeRate = &v
	}
	if row.RatingSamples > 0 && row.RatingSum != nil {
		v := *row.RatingSum / float64(row.RatingSamples)
		dto.AverageRating = &v
	}
	for i := range queue {
		dto.ActiveQueue = append(dto.ActiveQueue, queueItemFromOrder(&queue[i]))
	}
	return dto
}

// businessLocation resolves the business's IANA timezone and falls back to UTC
// when the row is missing or the zone string is unparseable. Day/week boundaries
// derive from this so a UAE business sees their "today" wall-clock day, not UTC.
func (s *DeliveryService) businessLocation(businessID uint) *time.Location {
	business, err := database.GetBusinessByID(businessID)
	if err != nil || business == nil {
		return time.UTC
	}
	if loc, locErr := time.LoadLocation(business.Timezone); locErr == nil && loc != nil {
		return loc
	}
	return time.UTC
}

// computeDriverPerformance builds the single-driver scorecard. It is
// "now"-injectable so tests can pin the current time without polluting the
// package with a clock. `now` is expected to be in the business's local
// timezone — boundary math (startOfDay / startOfWeek) preserves whatever
// location it carries.
//
// DELIV-PERF-1: KPIs come from the same SQL aggregate query the leaderboard
// (ListDriverPerformance) uses, plus one bounded read for the live queue. The
// previous implementation hydrated the driver's ENTIRE all-time delivery
// history as full rows and reduced it in Go — O(history) rows and allocations
// per dashboard poll.
func (s *DeliveryService) computeDriverPerformance(driver *database.DeliveryDriver, now time.Time) (*DriverPerformanceDTO, error) {
	driverIDs := []uint{driver.ID}
	aggregates, err := s.loadDriverPerformanceAggregates(driver.BusinessID, driverIDs, now)
	if err != nil {
		return nil, fmt.Errorf("failed to load driver delivery aggregates: %w", err)
	}
	queues, err := s.loadDriverActiveQueues(driver.BusinessID, driverIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to load active driver deliveries: %w", err)
	}
	return driverPerformanceDTOFromAggregate(driver, aggregates[driver.ID], queues[driver.ID]), nil
}

func queueItemFromOrder(order *database.DeliveryOrder) DriverQueueItem {
	return DriverQueueItem{
		DeliveryID:            order.ID,
		DeliveryNumber:        order.DeliveryNumber,
		Status:                string(order.Status),
		CustomerName:          order.CustomerName,
		AddressShort:          shortenAddress(order.DeliveryAddress.Street, order.DeliveryAddress.City),
		AssignedAt:            order.AssignedAt,
		EstimatedDeliveryTime: order.EstimatedDeliveryTime,
		DeliveryFee:           centsToDollarsFloat(order.DeliveryFee),
		DriverTip:             centsToDollarsFloat(order.DriverTip),
	}
}

func shortenAddress(street, city string) string {
	switch {
	case street == "" && city == "":
		return ""
	case street == "":
		return city
	case city == "":
		return street
	default:
		return street + ", " + city
	}
}

func centsToDollarsFloat(cents int64) float64 {
	return float64(cents) / 100.0
}
