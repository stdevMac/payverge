package database

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"gorm.io/gorm"
)

// safeIdentifier matches a bare or schema-qualified column name. GORM does
// not parameterize identifiers, so any caller-supplied column name MUST
// pass this gate before it is interpolated into a query fragment.
var safeIdentifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z_][a-zA-Z0-9_]*)?$`)

// Repository provides generic CRUD operations
type Repository[T any] struct {
	db *gorm.DB
}

// NewRepository creates a new repository instance
func NewRepository[T any](db *gorm.DB) *Repository[T] {
	return &Repository[T]{db: db}
}

// Create creates a new record
func (r *Repository[T]) Create(entity *T) error {
	if err := r.db.Create(entity).Error; err != nil {
		return fmt.Errorf("failed to create record: %w", err)
	}
	return nil
}

// GetByID retrieves a record by ID
func (r *Repository[T]) GetByID(id uint) (*T, error) {
	var entity T
	if err := r.db.First(&entity, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("record not found")
		}
		return nil, fmt.Errorf("failed to get record: %w", err)
	}
	return &entity, nil
}

// GetAll retrieves all records
func (r *Repository[T]) GetAll() ([]T, error) {
	var entities []T
	if err := r.db.Find(&entities).Error; err != nil {
		return nil, fmt.Errorf("failed to get records: %w", err)
	}
	return entities, nil
}

// GetWhere retrieves records matching a condition
func (r *Repository[T]) GetWhere(condition string, args ...interface{}) ([]T, error) {
	var entities []T
	if err := r.db.Where(condition, args...).Find(&entities).Error; err != nil {
		return nil, fmt.Errorf("failed to get records: %w", err)
	}
	return entities, nil
}

// GetFirstWhere retrieves the first record matching a condition
func (r *Repository[T]) GetFirstWhere(condition string, args ...interface{}) (*T, error) {
	var entity T
	if err := r.db.Where(condition, args...).First(&entity).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("record not found")
		}
		return nil, fmt.Errorf("failed to get record: %w", err)
	}
	return &entity, nil
}

// Update updates a record (only non-zero fields)
func (r *Repository[T]) Update(entity *T) error {
	if err := r.db.Model(entity).Updates(entity).Error; err != nil {
		return fmt.Errorf("failed to update record: %w", err)
	}
	return nil
}

// UpdateWhere updates records matching a condition
func (r *Repository[T]) UpdateWhere(condition string, updates map[string]interface{}, args ...interface{}) error {
	var entity T
	if err := r.db.Model(&entity).Where(condition, args...).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to update records: %w", err)
	}
	return nil
}

// Delete deletes a record by ID
func (r *Repository[T]) Delete(id uint) error {
	var entity T
	if err := r.db.Delete(&entity, id).Error; err != nil {
		return fmt.Errorf("failed to delete record: %w", err)
	}
	return nil
}

// DeleteWhere deletes records matching a condition
func (r *Repository[T]) DeleteWhere(condition string, args ...interface{}) error {
	var entity T
	if err := r.db.Where(condition, args...).Delete(&entity).Error; err != nil {
		return fmt.Errorf("failed to delete records: %w", err)
	}
	return nil
}

// Count counts records matching a condition
func (r *Repository[T]) Count(condition string, args ...interface{}) (int64, error) {
	var count int64
	var entity T
	query := r.db.Model(&entity)
	if condition != "" {
		query = query.Where(condition, args...)
	}
	if err := query.Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count records: %w", err)
	}
	return count, nil
}

// GetByDateRange retrieves records within a date range
func (r *Repository[T]) GetByDateRange(dateField string, startDate, endDate time.Time, additionalCondition string, args ...interface{}) ([]T, error) {
	if !safeIdentifier.MatchString(dateField) {
		return nil, fmt.Errorf("invalid date field %q: must be a bare column identifier", dateField)
	}
	var entities []T
	query := r.db.Where(fmt.Sprintf("%s >= ? AND %s < ?", dateField, dateField), startDate, endDate)
	if additionalCondition != "" {
		query = query.Where(additionalCondition, args...)
	}
	if err := query.Find(&entities).Error; err != nil {
		return nil, fmt.Errorf("failed to get records by date range: %w", err)
	}
	return entities, nil
}
