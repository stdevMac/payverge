package activation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/metrics"

	"gorm.io/gorm"
)

type EventSink func(distinctID, event string, properties map[string]interface{}) error

func postHogSink(distinctID, event string, properties map[string]interface{}) error {
	if !metrics.PostHogConfigured() {
		return errors.New("analytics delivery is not configured")
	}
	return metrics.TrackEvent(distinctID, event, properties)
}

type Dispatcher struct {
	db   *gorm.DB
	sink EventSink
	now  func() time.Time
}

func NewDispatcher(db *gorm.DB) *Dispatcher {
	return &Dispatcher{db: db, sink: postHogSink, now: time.Now}
}

func (d *Dispatcher) DispatchPending(ctx context.Context, limit int) (int, error) {
	if d == nil || d.db == nil {
		return 0, gorm.ErrInvalidDB
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	now := d.now().UTC()
	var rows []OutboxEvent
	err := d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Raw(`
			WITH candidates AS (
				SELECT id FROM activation_event_outbox
				WHERE delivery_status IN ('pending', 'failed')
				   OR (delivery_status = 'processing' AND updated_at < ?)
				ORDER BY occurred_at, id
				FOR UPDATE SKIP LOCKED
				LIMIT ?
			)
			UPDATE activation_event_outbox AS events
			SET delivery_status = 'processing', delivery_attempts = delivery_attempts + 1,
				updated_at = ?
			FROM candidates
			WHERE events.id = candidates.id
			RETURNING events.*
		`, now.Add(-5*time.Minute), limit, now).Scan(&rows).Error
	})
	if err != nil {
		return 0, err
	}
	return d.deliverRows(ctx, rows, now)
}

func (d *Dispatcher) deliverRows(ctx context.Context, rows []OutboxEvent, now time.Time) (int, error) {
	delivered := 0
	for _, row := range rows {
		properties := make(map[string]interface{})
		if err := json.Unmarshal(row.Dimensions, &properties); err != nil {
			d.markFailed(ctx, row.ID, now, "invalid safe dimensions")
			continue
		}
		definition, ok := DefinitionFor(row.EventName)
		if !ok {
			d.markFailed(ctx, row.ID, now, "unknown activation event")
			continue
		}
		properties["schema_version"] = row.SchemaVersion
		properties["server_authoritative"] = definition.ServerAuthoritative
		properties["denominator"] = string(definition.Denominator)
		properties["occurred_at"] = row.OccurredAt.UTC().Format(time.RFC3339Nano)
		properties["$insert_id"] = row.IdempotencyKey
		distinctID := ""
		if row.BusinessID != nil {
			distinctID = "business:" + strconv.FormatUint(uint64(*row.BusinessID), 10)
		}
		if row.FunnelID != nil {
			distinctID = "funnel:" + *row.FunnelID
		}
		if err := d.sink(distinctID, string(row.EventName), properties); err != nil {
			d.markFailed(ctx, row.ID, now, err.Error())
			continue
		}
		if err := d.db.WithContext(ctx).Model(&OutboxEvent{}).Where("id = ? AND delivery_status = 'processing'", row.ID).
			Updates(map[string]interface{}{"delivery_status": "delivered", "delivered_at": now, "last_error": "", "updated_at": now}).Error; err != nil {
			return delivered, err
		}
		delivered++
	}
	return delivered, nil
}

func (d *Dispatcher) markFailed(ctx context.Context, id uint, now time.Time, message string) {
	message = strings.TrimSpace(message)
	if len(message) > 500 {
		message = message[:500]
	}
	_ = d.db.WithContext(ctx).Model(&OutboxEvent{}).Where("id = ?", id).
		Updates(map[string]interface{}{"delivery_status": "failed", "last_error": message, "updated_at": now}).Error
}

type Scheduler struct {
	dispatcher *Dispatcher
	interval   time.Duration
	stop       chan struct{}
	done       chan struct{}
	once       sync.Once
	ctx        context.Context
	cancel     context.CancelFunc
}

func NewScheduler(db *gorm.DB) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		dispatcher: NewDispatcher(db),
		interval:   time.Minute,
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
		ctx:        ctx,
		cancel:     cancel,
	}
}

func (s *Scheduler) Start() error {
	if s == nil || s.dispatcher == nil {
		return fmt.Errorf("activation dispatcher is not configured")
	}
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
		for {
			logger.SafeTick("activation-dispatcher", func() {
				_, err := s.dispatcher.DispatchPending(s.ctx, 100)
				if err != nil && s.ctx.Err() == nil {
					logger.Logger.Warnf("activation dispatcher: %v", err)
				}
			})
			select {
			case <-ticker.C:
			case <-s.stop:
				return
			case <-s.ctx.Done():
				return
			}
		}
	}()
	return nil
}

func (s *Scheduler) Stop() {
	if s == nil {
		return
	}
	s.once.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		if s.stop != nil {
			close(s.stop)
		}
	})
	if s.done != nil {
		<-s.done
	}
}
