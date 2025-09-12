package database

import (
	"encoding/json"
	"time"
)

// BusinessService provides business-specific operations
type BusinessService struct {
	repo *Repository[Business]
}

// NewBusinessService creates a new business service
func NewBusinessService() *BusinessService {
	return &BusinessService{
		repo: NewRepository[Business](db),
	}
}

func (s *BusinessService) Create(business *Business) error {
	return s.repo.Create(business)
}

func (s *BusinessService) GetByID(id uint) (*Business, error) {
	return s.repo.GetByID(id)
}

func (s *BusinessService) GetByOwnerAddress(ownerAddress string) ([]Business, error) {
	return s.repo.GetWhere("owner_address = ? AND is_active = ?", ownerAddress, true)
}

func (s *BusinessService) Update(business *Business) error {
	return s.repo.Update(business)
}

func (s *BusinessService) SoftDelete(id uint) error {
	return s.repo.UpdateWhere("id = ?", map[string]interface{}{"is_active": false}, id)
}

// TableService provides table-specific operations
type TableService struct {
	repo *Repository[Table]
}

func NewTableService() *TableService {
	return &TableService{
		repo: NewRepository[Table](db),
	}
}

func (s *TableService) Create(table *Table) error {
	return s.repo.Create(table)
}

func (s *TableService) GetByID(id uint) (*Table, error) {
	return s.repo.GetByID(id)
}

func (s *TableService) GetByBusinessID(businessID uint) ([]Table, error) {
	return s.repo.GetWhere("business_id = ? AND is_active = ?", businessID, true)
}

func (s *TableService) GetByCode(tableCode string) (*Table, error) {
	return s.repo.GetFirstWhere("table_code = ?", tableCode)
}

func (s *TableService) Update(table *Table) error {
	return s.repo.Update(table)
}

func (s *TableService) SoftDelete(id uint) error {
	return s.repo.UpdateWhere("id = ?", map[string]interface{}{"is_active": false}, id)
}

// BillService provides bill-specific operations
type BillService struct {
	repo *Repository[Bill]
}

func NewBillService() *BillService {
	return &BillService{
		repo: NewRepository[Bill](db),
	}
}

func (s *BillService) Create(bill *Bill) error {
	return s.repo.Create(bill)
}

func (s *BillService) GetByID(id uint) (*Bill, error) {
	return s.repo.GetByID(id)
}

func (s *BillService) GetByStatus(status BillStatus) ([]Bill, error) {
	return s.repo.GetWhere("status = ?", status)
}

func (s *BillService) GetByBusinessAndStatus(businessID uint, status BillStatus) ([]Bill, error) {
	return s.repo.GetWhere("business_id = ? AND status = ?", businessID, status)
}

func (s *BillService) GetByDateRange(businessID uint, startDate, endDate time.Time) ([]Bill, error) {
	return s.repo.GetByDateRange("created_at", startDate, endDate, "business_id = ?", businessID)
}

func (s *BillService) Update(bill *Bill) error {
	return s.repo.Update(bill)
}

// PaymentService provides payment-specific operations
type PaymentService struct {
	repo *Repository[Payment]
}

func NewPaymentService() *PaymentService {
	return &PaymentService{
		repo: NewRepository[Payment](db),
	}
}

func (s *PaymentService) Create(payment *Payment) error {
	return s.repo.Create(payment)
}

func (s *PaymentService) GetByID(id uint) (*Payment, error) {
	return s.repo.GetByID(id)
}

func (s *PaymentService) GetByBillID(billID uint) ([]Payment, error) {
	return s.repo.GetWhere("bill_id = ?", billID)
}

func (s *PaymentService) GetByDateRange(businessID uint, startDate, endDate time.Time) ([]Payment, error) {
	var payments []Payment
	err := db.Joins("JOIN bills ON payments.bill_id = bills.id").
		Where("bills.business_id = ? AND payments.created_at >= ? AND payments.created_at < ?", 
			businessID, startDate, endDate).
		Find(&payments).Error
	return payments, err
}

func (s *PaymentService) Update(payment *Payment) error {
	return s.repo.Update(payment)
}

// MenuService provides menu-specific operations with JSON handling
type MenuService struct {
	repo *Repository[Menu]
}

func NewMenuService() *MenuService {
	return &MenuService{
		repo: NewRepository[Menu](db),
	}
}

func (s *MenuService) Create(menu *Menu, categories []MenuCategory) error {
	categoriesJSON, err := json.Marshal(categories)
	if err != nil {
		return err
	}
	menu.Categories = string(categoriesJSON)
	return s.repo.Create(menu)
}

func (s *MenuService) GetByBusinessID(businessID uint) (*Menu, []MenuCategory, error) {
	menu, err := s.repo.GetFirstWhere("business_id = ? AND is_active = ?", businessID, true)
	if err != nil {
		return nil, nil, err
	}

	var categories []MenuCategory
	if err := json.Unmarshal([]byte(menu.Categories), &categories); err != nil {
		return nil, nil, err
	}

	return menu, categories, nil
}

func (s *MenuService) Update(menu *Menu, categories []MenuCategory) error {
	categoriesJSON, err := json.Marshal(categories)
	if err != nil {
		return err
	}
	menu.Categories = string(categoriesJSON)
	return s.repo.Update(menu)
}

// CodeService provides code-specific operations
type CodeService struct {
	repo *Repository[Code]
}

func NewCodeService() *CodeService {
	return &CodeService{
		repo: NewRepository[Code](db),
	}
}

func (s *CodeService) GetByCode(code string) (*Code, error) {
	return s.repo.GetFirstWhere("code = ?", code)
}

func (s *CodeService) GetAll() ([]Code, error) {
	return s.repo.GetAll()
}

func (s *CodeService) Update(code *Code) error {
	return s.repo.UpdateWhere("code = ?", map[string]interface{}{
		"used": code.Used,
		"address": code.Address,
		"claimed_at": code.ClaimedAt,
	}, code.Code)
}

func (s *CodeService) Delete(code string) error {
	return s.repo.DeleteWhere("code = ?", code)
}
