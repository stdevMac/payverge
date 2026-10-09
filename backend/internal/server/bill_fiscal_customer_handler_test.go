package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupFiscalCustomerTestDB spins up an in-memory SQLite DB with just the
// tables the SetBillFiscalCustomer handler touches.
func setupFiscalCustomerTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	gormDB, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	sqlDB, err := gormDB.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	database.SetTestDB(gormDB)
	// GetBillByID preloads Business/Table/Payments, so those tables must exist
	// even though the handler only mutates the four fiscal-customer columns.
	require.NoError(t, gormDB.AutoMigrate(
		&database.Business{},
		&database.Table{},
		&database.Bill{},
		&database.Payment{},
	))
	return gormDB
}

func seedFiscalCustomerBill(t *testing.T) (*database.Business, *database.Bill) {
	t.Helper()
	business := &database.Business{
		Name:         "Test Resto",
		OwnerAddress: "0xowner",
		Address:      database.BusinessAddress{Country: "AR"},
	}
	require.NoError(t, database.GetDB().Create(business).Error)

	bill := &database.Bill{
		BusinessID:     business.ID,
		BillNumber:     fmt.Sprintf("FC-%d", time.Now().UnixNano()),
		TotalAmount:    50000,
		Status:         database.BillStatusOpen,
		SettlementAddr: "0xsettle",
		TippingAddr:    "0xtip",
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	require.NoError(t, database.GetDB().Create(bill).Error)
	return business, bill
}

// newFiscalCustomerRequest builds an authorized PUT request context against the
// SetBillFiscalCustomer handler. Authorization mirrors the sibling bill routes:
// staff_business_id matching the bill's business grants access.
func newFiscalCustomerRequest(t *testing.T, businessID, billID uint, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/inside/bills/%d/fiscal-customer", billID), bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Params = gin.Params{{Key: "bill_id", Value: fmt.Sprintf("%d", billID)}}
	c.Set("staff_business_id", businessID)
	c.Set("token_type", "staff")
	c.Set("staff_id", uint(7))
	return c, w
}

func newGuestFiscalCustomerRequest(t *testing.T, token, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodPost, "/guest/bill/"+token+"/fiscal-customer", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Params = gin.Params{{Key: "bill_token", Value: token}}
	return c, w
}

func TestSetBillFiscalCustomerByNumber_RejectsPaidBill(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	_, bill := seedFiscalCustomerBill(t)
	require.NoError(t, database.GetDB().Model(bill).Update("status", database.BillStatusPaid).Error)

	body := `{"fiscal_customer_doc_type":"DNI","fiscal_customer_doc_number":"12345678"}`
	c, w := newGuestFiscalCustomerRequest(t, bill.PublicToken, body)
	SetBillFiscalCustomerByNumber(c)

	require.Equal(t, http.StatusConflict, w.Code, "body: %s", w.Body.String())
	var reloaded database.Bill
	require.NoError(t, database.GetDB().First(&reloaded, bill.ID).Error)
	assert.Nil(t, reloaded.FiscalCustomerDocNumber, "a bearer capability must not rewrite fiscal identity after settlement")
}

// TestSetBillFiscalCustomer_PersistsValidCUIT is the happy path: a valid 11-digit
// CUIT for a responsable_inscripto customer persists all four fields.
func TestSetBillFiscalCustomer_PersistsValidCUIT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	business, bill := seedFiscalCustomerBill(t)

	body := `{
		"fiscal_customer_doc_type": "CUIT",
		"fiscal_customer_doc_number": "20-11111111-2",
		"fiscal_customer_tax_condition": "responsable_inscripto",
		"fiscal_customer_name": "ACME SA"
	}`
	c, w := newFiscalCustomerRequest(t, business.ID, bill.ID, body)

	SetBillFiscalCustomer(c)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var reloaded database.Bill
	require.NoError(t, database.GetDB().First(&reloaded, bill.ID).Error)
	require.NotNil(t, reloaded.FiscalCustomerDocType)
	assert.Equal(t, "CUIT", *reloaded.FiscalCustomerDocType)
	require.NotNil(t, reloaded.FiscalCustomerDocNumber)
	assert.Equal(t, "20-11111111-2", *reloaded.FiscalCustomerDocNumber)
	require.NotNil(t, reloaded.FiscalCustomerTaxCondition)
	assert.Equal(t, "responsable_inscripto", *reloaded.FiscalCustomerTaxCondition)
	require.NotNil(t, reloaded.FiscalCustomerName)
	assert.Equal(t, "ACME SA", *reloaded.FiscalCustomerName)
}

// TestSetBillFiscalCustomer_PersistsValidDNI verifies a 7-8 digit DNI is accepted.
func TestSetBillFiscalCustomer_PersistsValidDNI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	business, bill := seedFiscalCustomerBill(t)

	body := `{
		"fiscal_customer_doc_type": "DNI",
		"fiscal_customer_doc_number": "12345678",
		"fiscal_customer_tax_condition": "consumidor_final",
		"fiscal_customer_name": "Juan Perez"
	}`
	c, w := newFiscalCustomerRequest(t, business.ID, bill.ID, body)

	SetBillFiscalCustomer(c)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var reloaded database.Bill
	require.NoError(t, database.GetDB().First(&reloaded, bill.ID).Error)
	require.NotNil(t, reloaded.FiscalCustomerDocType)
	assert.Equal(t, "DNI", *reloaded.FiscalCustomerDocType)
	require.NotNil(t, reloaded.FiscalCustomerDocNumber)
	assert.Equal(t, "12345678", *reloaded.FiscalCustomerDocNumber)
}

// TestSetBillFiscalCustomer_RejectsBadCUITLength rejects an 11-digit-violation
// (10 digits) CUIT with 400.
func TestSetBillFiscalCustomer_RejectsBadCUITLength(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	business, bill := seedFiscalCustomerBill(t)

	body := `{
		"fiscal_customer_doc_type": "CUIT",
		"fiscal_customer_doc_number": "2012345678",
		"fiscal_customer_tax_condition": "responsable_inscripto"
	}`
	c, w := newFiscalCustomerRequest(t, business.ID, bill.ID, body)

	SetBillFiscalCustomer(c)

	require.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())

	var reloaded database.Bill
	require.NoError(t, database.GetDB().First(&reloaded, bill.ID).Error)
	assert.Nil(t, reloaded.FiscalCustomerDocNumber, "malformed doc number must not persist")
}

// TestSetBillFiscalCustomer_RejectsBadDocType rejects an unknown doc type.
func TestSetBillFiscalCustomer_RejectsBadDocType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	business, bill := seedFiscalCustomerBill(t)

	body := `{
		"fiscal_customer_doc_type": "PASSPORT",
		"fiscal_customer_doc_number": "AB123456"
	}`
	c, w := newFiscalCustomerRequest(t, business.ID, bill.ID, body)

	SetBillFiscalCustomer(c)

	require.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
}

// TestSetBillFiscalCustomer_RejectsBadTaxCondition rejects an unknown tax condition.
func TestSetBillFiscalCustomer_RejectsBadTaxCondition(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	business, bill := seedFiscalCustomerBill(t)

	body := `{
		"fiscal_customer_tax_condition": "not_a_condition"
	}`
	c, w := newFiscalCustomerRequest(t, business.ID, bill.ID, body)

	SetBillFiscalCustomer(c)

	require.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
}

// TestSetBillFiscalCustomer_EmptyClearsToNull verifies that blank fields persist
// as NULL (consumidor final, no identification) rather than empty strings.
func TestSetBillFiscalCustomer_EmptyClearsToNull(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	business, bill := seedFiscalCustomerBill(t)

	// Pre-set some identity so we can confirm it clears.
	docType := "DNI"
	docNum := "12345678"
	require.NoError(t, database.GetDB().Model(&database.Bill{}).Where("id = ?", bill.ID).
		Updates(map[string]any{
			"fiscal_customer_doc_type":   &docType,
			"fiscal_customer_doc_number": &docNum,
		}).Error)

	body := `{
		"fiscal_customer_doc_type": "",
		"fiscal_customer_doc_number": "  ",
		"fiscal_customer_tax_condition": "",
		"fiscal_customer_name": ""
	}`
	c, w := newFiscalCustomerRequest(t, business.ID, bill.ID, body)

	SetBillFiscalCustomer(c)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var reloaded database.Bill
	require.NoError(t, database.GetDB().First(&reloaded, bill.ID).Error)
	assert.Nil(t, reloaded.FiscalCustomerDocType, "blank doc type must clear to NULL")
	assert.Nil(t, reloaded.FiscalCustomerDocNumber, "blank doc number must clear to NULL")
	assert.Nil(t, reloaded.FiscalCustomerTaxCondition, "blank tax condition must clear to NULL")
	assert.Nil(t, reloaded.FiscalCustomerName, "blank name must clear to NULL")
}

// TestSetBillFiscalCustomer_AbsentKeysLeaveFieldsUntouched guards against the
// wipe vector: a POST that omits the fiscal keys entirely (e.g. `{}` from an
// attacker who guessed the bill number) must NOT blank identity the customer
// already entered. Only explicitly-present keys are written.
func TestSetBillFiscalCustomer_AbsentKeysLeaveFieldsUntouched(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	business, bill := seedFiscalCustomerBill(t)

	// Pre-set a full fiscal identity.
	docType, docNum, taxCond, name := "CUIT", "20123456780", "responsable_inscripto", "Acme SA"
	require.NoError(t, database.GetDB().Model(&database.Bill{}).Where("id = ?", bill.ID).
		Updates(map[string]any{
			"fiscal_customer_doc_type":      &docType,
			"fiscal_customer_doc_number":    &docNum,
			"fiscal_customer_tax_condition": &taxCond,
			"fiscal_customer_name":          &name,
		}).Error)

	// Empty body: no keys present.
	c, w := newFiscalCustomerRequest(t, business.ID, bill.ID, `{}`)
	SetBillFiscalCustomer(c)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var reloaded database.Bill
	require.NoError(t, database.GetDB().First(&reloaded, bill.ID).Error)
	require.NotNil(t, reloaded.FiscalCustomerDocType)
	assert.Equal(t, "CUIT", *reloaded.FiscalCustomerDocType, "absent key must NOT wipe doc type")
	require.NotNil(t, reloaded.FiscalCustomerDocNumber)
	assert.Equal(t, "20123456780", *reloaded.FiscalCustomerDocNumber, "absent key must NOT wipe doc number")
	require.NotNil(t, reloaded.FiscalCustomerName)
	assert.Equal(t, "Acme SA", *reloaded.FiscalCustomerName, "absent key must NOT wipe name")
}

// TestSetBillFiscalCustomer_PartialUpdateLeavesOthers: supplying only one field
// updates that field and leaves the rest intact.
func TestSetBillFiscalCustomer_PartialUpdateLeavesOthers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	business, bill := seedFiscalCustomerBill(t)

	docType, docNum := "DNI", "12345678"
	require.NoError(t, database.GetDB().Model(&database.Bill{}).Where("id = ?", bill.ID).
		Updates(map[string]any{
			"fiscal_customer_doc_type":   &docType,
			"fiscal_customer_doc_number": &docNum,
		}).Error)

	// Update only the name.
	c, w := newFiscalCustomerRequest(t, business.ID, bill.ID, `{"fiscal_customer_name":"Jane Doe"}`)
	SetBillFiscalCustomer(c)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var reloaded database.Bill
	require.NoError(t, database.GetDB().First(&reloaded, bill.ID).Error)
	require.NotNil(t, reloaded.FiscalCustomerName)
	assert.Equal(t, "Jane Doe", *reloaded.FiscalCustomerName)
	require.NotNil(t, reloaded.FiscalCustomerDocType, "doc type must survive a name-only update")
	assert.Equal(t, "DNI", *reloaded.FiscalCustomerDocType)
	require.NotNil(t, reloaded.FiscalCustomerDocNumber)
	assert.Equal(t, "12345678", *reloaded.FiscalCustomerDocNumber)
}

// TestSetBillFiscalCustomer_RejectsForeignBusiness blocks IDOR: a staff member
// of a different business cannot mutate this bill.
func TestSetBillFiscalCustomer_RejectsForeignBusiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	_, bill := seedFiscalCustomerBill(t)

	body := `{"fiscal_customer_doc_type": "DNI", "fiscal_customer_doc_number": "12345678"}`
	// staff_business_id of a DIFFERENT business (999)
	c, w := newFiscalCustomerRequest(t, 999, bill.ID, body)

	SetBillFiscalCustomer(c)

	require.Equal(t, http.StatusForbidden, w.Code, "body: %s", w.Body.String())

	var reloaded database.Bill
	require.NoError(t, database.GetDB().First(&reloaded, bill.ID).Error)
	assert.Nil(t, reloaded.FiscalCustomerDocNumber, "foreign business must not mutate the bill")
}

// TestSetBillFiscalCustomer_DocNumberRequiresDocType rejects a doc number with no
// doc type (ambiguous — we can't validate digit shape).
func TestSetBillFiscalCustomer_DocNumberRequiresDocType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	business, bill := seedFiscalCustomerBill(t)

	body := `{"fiscal_customer_doc_number": "12345678"}`
	c, w := newFiscalCustomerRequest(t, business.ID, bill.ID, body)

	SetBillFiscalCustomer(c)

	require.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
}

// TestSetBillFiscalCustomer_ResponseShape confirms the handler returns the bill
// JSON (so the FE can reflect the saved identity). Bill.MarshalJSON is the wire
// path — fiscal_customer_email must appear alongside the other fiscal fields.
func TestSetBillFiscalCustomer_ResponseShape(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	business, bill := seedFiscalCustomerBill(t)

	body := `{"fiscal_customer_doc_type": "CUIT", "fiscal_customer_doc_number": "20111111112", "fiscal_customer_tax_condition": "responsable_inscripto", "fiscal_customer_email": "guest@example.com"}`
	c, w := newFiscalCustomerRequest(t, business.ID, bill.ID, body)

	SetBillFiscalCustomer(c)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())

	var resp struct {
		Bill map[string]any `json:"bill"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.NotNil(t, resp.Bill)
	assert.Equal(t, "CUIT", resp.Bill["fiscal_customer_doc_type"])
	assert.Equal(t, "20111111112", resp.Bill["fiscal_customer_doc_number"])
	assert.Equal(t, "guest@example.com", resp.Bill["fiscal_customer_email"])
}

func TestSetBillFiscalCustomerPersistsEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	business, bill := seedFiscalCustomerBill(t)

	c, w := newFiscalCustomerRequest(t, business.ID, bill.ID,
		`{"fiscal_customer_email":"  guest@example.com  "}`)
	SetBillFiscalCustomer(c)
	require.Equal(t, http.StatusOK, w.Code)

	var got database.Bill
	require.NoError(t, database.GetDB().First(&got, bill.ID).Error)
	require.NotNil(t, got.FiscalCustomerEmail)
	assert.Equal(t, "guest@example.com", *got.FiscalCustomerEmail)

	// Wire contract: the PUT response bill JSON must echo the email so the FE
	// can re-seed without a second fetch (Bill.MarshalJSON, not the model tag).
	var resp struct {
		Bill map[string]any `json:"bill"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "guest@example.com", resp.Bill["fiscal_customer_email"])
}

func TestSetBillFiscalCustomerRejectsInvalidEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	business, bill := seedFiscalCustomerBill(t)

	c, w := newFiscalCustomerRequest(t, business.ID, bill.ID,
		`{"fiscal_customer_email":"not-an-email"}`)
	SetBillFiscalCustomer(c)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// Presence-flag contract: a request without the email key must not clear a
// stored email (same anti-wipe rule as the other four fiscal columns).
func seedStoredFiscalIdentity(t *testing.T, billID uint, docType, docNumber, taxCondition, name, email string) {
	t.Helper()
	cols := map[string]any{}
	if docType != "" {
		cols["fiscal_customer_doc_type"] = docType
	}
	if docNumber != "" {
		cols["fiscal_customer_doc_number"] = docNumber
	}
	if taxCondition != "" {
		cols["fiscal_customer_tax_condition"] = taxCondition
	}
	if name != "" {
		cols["fiscal_customer_name"] = name
	}
	if email != "" {
		cols["fiscal_customer_email"] = email
	}
	require.NotEmpty(t, cols)
	require.NoError(t, database.GetDB().Model(&database.Bill{}).Where("id = ?", billID).Updates(cols).Error)
}

func reloadFiscalBill(t *testing.T, billID uint) database.Bill {
	t.Helper()
	var got database.Bill
	require.NoError(t, database.GetDB().First(&got, billID).Error)
	return got
}

func requirePtrEq(t *testing.T, got *string, want string, msg string) {
	t.Helper()
	require.NotNil(t, got, msg)
	assert.Equal(t, want, *got, msg)
}

// TestSetBillFiscalCustomerByNumber_RejectsBlankWipeOfExistingIdentity is #545:
// explicit empty strings on the guest token must not erase an identity that is
// already stored (including one entered by an operator).
func TestSetBillFiscalCustomerByNumber_RejectsBlankWipeOfExistingIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	_, bill := seedFiscalCustomerBill(t)
	seedStoredFiscalIdentity(t, bill.ID, "DNI", "12345678", "consumidor_final", "Juan Perez", "victim@example.com")

	body := `{
		"fiscal_customer_doc_type": "",
		"fiscal_customer_doc_number": "  ",
		"fiscal_customer_tax_condition": "",
		"fiscal_customer_name": "",
		"fiscal_customer_email": ""
	}`
	c, w := newGuestFiscalCustomerRequest(t, bill.PublicToken, body)
	SetBillFiscalCustomerByNumber(c)

	require.Equal(t, http.StatusConflict, w.Code, "body: %s", w.Body.String())
	got := reloadFiscalBill(t, bill.ID)
	requirePtrEq(t, got.FiscalCustomerDocType, "DNI", "guest blank wipe must not clear doc type")
	requirePtrEq(t, got.FiscalCustomerDocNumber, "12345678", "guest blank wipe must not clear doc number")
	requirePtrEq(t, got.FiscalCustomerTaxCondition, "consumidor_final", "guest blank wipe must not clear tax condition")
	requirePtrEq(t, got.FiscalCustomerName, "Juan Perez", "guest blank wipe must not clear name")
	requirePtrEq(t, got.FiscalCustomerEmail, "victim@example.com", "guest blank wipe must not clear email")
}

// TestSetBillFiscalCustomerByNumber_RejectsOverwriteOfDifferentIdentity is #545:
// first writer wins. A later guest token holder cannot replace a stored CUIT/DNI.
func TestSetBillFiscalCustomerByNumber_RejectsOverwriteOfDifferentIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	_, bill := seedFiscalCustomerBill(t)
	seedStoredFiscalIdentity(t, bill.ID, "DNI", "12345678", "consumidor_final", "Juan Perez", "")

	body := `{
		"fiscal_customer_doc_type": "DNI",
		"fiscal_customer_doc_number": "87654321",
		"fiscal_customer_tax_condition": "monotributo",
		"fiscal_customer_name": "Attacker"
	}`
	c, w := newGuestFiscalCustomerRequest(t, bill.PublicToken, body)
	SetBillFiscalCustomerByNumber(c)

	require.Equal(t, http.StatusConflict, w.Code, "body: %s", w.Body.String())
	got := reloadFiscalBill(t, bill.ID)
	requirePtrEq(t, got.FiscalCustomerDocNumber, "12345678", "guest must not overwrite a stored document number")
	requirePtrEq(t, got.FiscalCustomerName, "Juan Perez", "guest must not overwrite a stored legal name")
	requirePtrEq(t, got.FiscalCustomerTaxCondition, "consumidor_final", "guest must not overwrite a stored tax condition")
}

// TestSetBillFiscalCustomerByNumber_AllowsIdempotentSameIdentity keeps a double
// submit of the same first-writer payload as 200 rather than a false conflict.
func TestSetBillFiscalCustomerByNumber_AllowsIdempotentSameIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	_, bill := seedFiscalCustomerBill(t)
	seedStoredFiscalIdentity(t, bill.ID, "DNI", "12345678", "consumidor_final", "Juan Perez", "guest@example.com")

	body := `{
		"fiscal_customer_doc_type": "DNI",
		"fiscal_customer_doc_number": "12345678",
		"fiscal_customer_tax_condition": "consumidor_final",
		"fiscal_customer_name": "Juan Perez",
		"fiscal_customer_email": "guest@example.com"
	}`
	c, w := newGuestFiscalCustomerRequest(t, bill.PublicToken, body)
	SetBillFiscalCustomerByNumber(c)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	got := reloadFiscalBill(t, bill.ID)
	requirePtrEq(t, got.FiscalCustomerDocNumber, "12345678", "idempotent retry must leave identity intact")
	requirePtrEq(t, got.FiscalCustomerEmail, "guest@example.com", "idempotent retry must leave email intact")
}

// TestSetBillFiscalCustomerByNumber_RejectsEmailRedirectOnExistingIdentity is
// #545: a later caller cannot point fiscal_customer_email at a different inbox
// (factura PDF redirect) once any identity is already stored.
func TestSetBillFiscalCustomerByNumber_RejectsEmailRedirectOnExistingIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	_, bill := seedFiscalCustomerBill(t)
	seedStoredFiscalIdentity(t, bill.ID, "DNI", "12345678", "consumidor_final", "Juan Perez", "victim@example.com")

	c, w := newGuestFiscalCustomerRequest(t, bill.PublicToken,
		`{"fiscal_customer_email":"attacker@evil.example"}`)
	SetBillFiscalCustomerByNumber(c)

	require.Equal(t, http.StatusConflict, w.Code, "body: %s", w.Body.String())
	got := reloadFiscalBill(t, bill.ID)
	requirePtrEq(t, got.FiscalCustomerEmail, "victim@example.com", "guest must not redirect factura email")
}

// TestSetBillFiscalCustomerByNumber_RejectsAddingEmailToExistingIdentity blocks
// attaching an attacker inbox to an operator/guest identity that had no email.
func TestSetBillFiscalCustomerByNumber_RejectsAddingEmailToExistingIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	_, bill := seedFiscalCustomerBill(t)
	seedStoredFiscalIdentity(t, bill.ID, "DNI", "12345678", "consumidor_final", "Juan Perez", "")

	c, w := newGuestFiscalCustomerRequest(t, bill.PublicToken,
		`{"fiscal_customer_email":"attacker@evil.example"}`)
	SetBillFiscalCustomerByNumber(c)

	require.Equal(t, http.StatusConflict, w.Code, "body: %s", w.Body.String())
	got := reloadFiscalBill(t, bill.ID)
	assert.Nil(t, got.FiscalCustomerEmail, "guest must not attach an email to an existing identity")
	requirePtrEq(t, got.FiscalCustomerDocNumber, "12345678", "identity must survive the rejected email attach")
}

// TestSetBillFiscalCustomerByNumber_AllowsEmailOnNewIdentity is the allowed
// first-writer case: email may be set when no fiscal identity exists yet.
// TestSetBillFiscalCustomerByNumber_EmptyObjectIsNoOp keeps the documented
// `{}` contract: no keys present means nothing is written, including after
// an identity already exists.
func TestSetBillFiscalCustomerByNumber_EmptyObjectIsNoOp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	_, bill := seedFiscalCustomerBill(t)
	seedStoredFiscalIdentity(t, bill.ID, "DNI", "12345678", "consumidor_final", "Juan Perez", "victim@example.com")

	c, w := newGuestFiscalCustomerRequest(t, bill.PublicToken, `{}`)
	SetBillFiscalCustomerByNumber(c)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	got := reloadFiscalBill(t, bill.ID)
	requirePtrEq(t, got.FiscalCustomerDocNumber, "12345678", "{} must not wipe a stored document")
	requirePtrEq(t, got.FiscalCustomerEmail, "victim@example.com", "{} must not wipe a stored email")
}

func TestSetBillFiscalCustomerByNumber_AllowsEmailOnNewIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	_, bill := seedFiscalCustomerBill(t)

	c, w := newGuestFiscalCustomerRequest(t, bill.PublicToken,
		`{"fiscal_customer_email":"first@example.com"}`)
	SetBillFiscalCustomerByNumber(c)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	got := reloadFiscalBill(t, bill.ID)
	requirePtrEq(t, got.FiscalCustomerEmail, "first@example.com", "first writer may create an email-only identity")
}

// TestSetBillFiscalCustomer_OperatorCanStillOverwrite confirms the authenticated
// operator PUT is not locked to first-writer-wins.
func TestSetBillFiscalCustomer_OperatorCanStillOverwrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	business, bill := seedFiscalCustomerBill(t)
	seedStoredFiscalIdentity(t, bill.ID, "DNI", "12345678", "consumidor_final", "Juan Perez", "victim@example.com")

	body := `{
		"fiscal_customer_doc_type": "DNI",
		"fiscal_customer_doc_number": "87654321",
		"fiscal_customer_tax_condition": "monotributo",
		"fiscal_customer_name": "Corrected Name",
		"fiscal_customer_email": "corrected@example.com"
	}`
	c, w := newFiscalCustomerRequest(t, business.ID, bill.ID, body)
	SetBillFiscalCustomer(c)

	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
	got := reloadFiscalBill(t, bill.ID)
	requirePtrEq(t, got.FiscalCustomerDocNumber, "87654321", "operator may correct a stored document")
	requirePtrEq(t, got.FiscalCustomerName, "Corrected Name", "operator may correct a stored name")
	requirePtrEq(t, got.FiscalCustomerEmail, "corrected@example.com", "operator may correct the factura email")
}

// TestSetBillFiscalCustomer_RejectsInvalidCUITChecksum is #546: 11 digits is
// not enough — the handler must call fiscal.ValidateCUITMod11.
func TestSetBillFiscalCustomer_RejectsInvalidCUITChecksum(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	business, bill := seedFiscalCustomerBill(t)

	// 20-11111111-3 has 11 digits; the mod-11 check digit is 2, not 3.
	body := `{
		"fiscal_customer_doc_type": "CUIT",
		"fiscal_customer_doc_number": "20-11111111-3",
		"fiscal_customer_tax_condition": "responsable_inscripto",
		"fiscal_customer_name": "QA Automation Test"
	}`
	c, w := newFiscalCustomerRequest(t, business.ID, bill.ID, body)
	SetBillFiscalCustomer(c)

	require.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
	got := reloadFiscalBill(t, bill.ID)
	assert.Nil(t, got.FiscalCustomerDocNumber, "checksum-invalid CUIT must not persist")
}

func TestSetBillFiscalCustomerByNumber_RejectsInvalidCUITChecksum(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	_, bill := seedFiscalCustomerBill(t)

	body := `{
		"fiscal_customer_doc_type": "CUIT",
		"fiscal_customer_doc_number": "20-11111111-3",
		"fiscal_customer_name": "QA Automation Test"
	}`
	c, w := newGuestFiscalCustomerRequest(t, bill.PublicToken, body)
	SetBillFiscalCustomerByNumber(c)

	require.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
	got := reloadFiscalBill(t, bill.ID)
	assert.Nil(t, got.FiscalCustomerDocNumber, "guest checksum-invalid CUIT must not persist")
}

// TestSetBillFiscalCustomer_RejectsOversizedName is #546: a 10KB name must 400.
func TestSetBillFiscalCustomer_RejectsOversizedName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	business, bill := seedFiscalCustomerBill(t)

	body := fmt.Sprintf(`{"fiscal_customer_name":%q}`, strings.Repeat("x", 10011))
	c, w := newFiscalCustomerRequest(t, business.ID, bill.ID, body)
	SetBillFiscalCustomer(c)

	require.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
	got := reloadFiscalBill(t, bill.ID)
	assert.Nil(t, got.FiscalCustomerName, "oversized name must not persist")
}

// TestSetBillFiscalCustomer_RejectsPaddedDocNumber is #546: separators must not
// sidestep the length cap. The digits form a valid CUIT (20111111112).
func TestSetBillFiscalCustomer_RejectsPaddedDocNumber(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	business, bill := seedFiscalCustomerBill(t)

	padded := "20" + strings.Repeat("-", 5000) + "111111112"
	body := fmt.Sprintf(`{"fiscal_customer_doc_type":"CUIT","fiscal_customer_doc_number":%q}`, padded)
	c, w := newFiscalCustomerRequest(t, business.ID, bill.ID, body)
	SetBillFiscalCustomer(c)

	require.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
	got := reloadFiscalBill(t, bill.ID)
	assert.Nil(t, got.FiscalCustomerDocNumber, "separator-padded doc number must not persist")
}

// TestSetBillFiscalCustomer_RejectsNULByte is #546: a NUL in any string field
// must be 400, never an unhandled 500 from the database.
func TestSetBillFiscalCustomer_RejectsNULByte(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	business, bill := seedFiscalCustomerBill(t)

	// JSON \u0000 unmarshals to a real NUL; a raw 0x00 would fail at bind time.
	body := `{"fiscal_customer_name":"Acme\u0000SA"}`
	c, w := newFiscalCustomerRequest(t, business.ID, bill.ID, body)
	SetBillFiscalCustomer(c)

	require.Equal(t, http.StatusBadRequest, w.Code, "NUL byte must be 400, not %d; body: %s", w.Code, w.Body.String())
	require.NotEqual(t, http.StatusInternalServerError, w.Code)
	got := reloadFiscalBill(t, bill.ID)
	assert.Nil(t, got.FiscalCustomerName, "NUL-bearing name must not persist")
}

func TestSetBillFiscalCustomerByNumber_RejectsNULByte(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	_, bill := seedFiscalCustomerBill(t)

	body := `{"fiscal_customer_name":"Acme\u0000SA"}`
	c, w := newGuestFiscalCustomerRequest(t, bill.PublicToken, body)
	SetBillFiscalCustomerByNumber(c)

	require.Equal(t, http.StatusBadRequest, w.Code, "guest NUL byte must be 400, not %d; body: %s", w.Code, w.Body.String())
	require.NotEqual(t, http.StatusInternalServerError, w.Code)
	got := reloadFiscalBill(t, bill.ID)
	assert.Nil(t, got.FiscalCustomerName, "guest NUL-bearing name must not persist")
}

// The guest-typed fiscal name and document end up in the receipt PDF mailed to
// the guest-typed address, so control characters (a forged line break in the
// PDF) and bidi overrides (visually reordered text) are refused on both the
// guest and the operator route. RLM/LRM marks and non-Latin names still pass.
func TestSetBillFiscalCustomer_RejectsControlAndBidiCharacters(t *testing.T) {
	gin.SetMode(gin.TestMode)

	refused := map[string]string{
		"newline in name":       `{"fiscal_customer_name":"Acme SA\nVisit evil.example"}`,
		"carriage return":       `{"fiscal_customer_name":"Acme\rSA"}`,
		"escape":                `{"fiscal_customer_name":"Acme\u001bSA"}`,
		"DEL":                   `{"fiscal_customer_name":"Acme\u007fSA"}`,
		"C1 control":            `{"fiscal_customer_name":"Acme\u0085SA"}`,
		"RLO override":          `{"fiscal_customer_name":"Acme \u202Egro.live\u202C SA"}`,
		"LRE embedding":         `{"fiscal_customer_name":"Acme \u202A SA"}`,
		"FSI isolate":           `{"fiscal_customer_name":"Acme \u2068 SA"}`,
		"PDI isolate":           `{"fiscal_customer_name":"Acme \u2069 SA"}`,
		"control in doc number": `{"fiscal_customer_doc_type":"DNI","fiscal_customer_doc_number":"1234\t5678"}`,
		"bidi in doc number":    `{"fiscal_customer_doc_type":"DNI","fiscal_customer_doc_number":"1234\u202E5678"}`,
		"control in doc type":   `{"fiscal_customer_doc_type":"DNI\u0007"}`,
		"control in email":      `{"fiscal_customer_email":"guest\u0001@example.com"}`,
	}
	for name, body := range refused {
		t.Run("guest/"+name, func(t *testing.T) {
			setupFiscalCustomerTestDB(t)
			_, bill := seedFiscalCustomerBill(t)
			c, w := newGuestFiscalCustomerRequest(t, bill.PublicToken, body)
			SetBillFiscalCustomerByNumber(c)
			require.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
			got := reloadFiscalBill(t, bill.ID)
			assert.Nil(t, got.FiscalCustomerName)
			assert.Nil(t, got.FiscalCustomerDocNumber)
			assert.Nil(t, got.FiscalCustomerEmail)
		})
		t.Run("operator/"+name, func(t *testing.T) {
			setupFiscalCustomerTestDB(t)
			business, bill := seedFiscalCustomerBill(t)
			c, w := newFiscalCustomerRequest(t, business.ID, bill.ID, body)
			SetBillFiscalCustomer(c)
			require.Equal(t, http.StatusBadRequest, w.Code, "body: %s", w.Body.String())
		})
	}

	allowed := map[string]string{
		"accented name":     "Panadería Núñez S.R.L.",
		"right-to-left":     "\u0645\u0637\u0639\u0645 \u200F\u0627\u0644\u0634\u0631\u0642",
		"LRM mark":          "Acme\u200E SA",
		"CJK legal name":    "\u682a\u5f0f\u4f1a\u793e\u30c6\u30b9\u30c8",
		"punctuation & co.": "O'Brien & Hijos (Sucursal 2) - SA",
	}
	for name, value := range allowed {
		t.Run("allowed/"+name, func(t *testing.T) {
			setupFiscalCustomerTestDB(t)
			_, bill := seedFiscalCustomerBill(t)
			body := `{"fiscal_customer_name":"` + value + `"}`
			c, w := newGuestFiscalCustomerRequest(t, bill.PublicToken, body)
			SetBillFiscalCustomerByNumber(c)
			require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
			got := reloadFiscalBill(t, bill.ID)
			require.NotNil(t, got.FiscalCustomerName)
		})
	}
}

func TestSetBillFiscalCustomerAbsentEmailKeyLeavesEmailUntouched(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	business, bill := seedFiscalCustomerBill(t)

	prior := "keep@example.com"
	require.NoError(t, database.GetDB().Model(&database.Bill{}).
		Where("id = ?", bill.ID).
		Update("fiscal_customer_email", &prior).Error)

	c, w := newFiscalCustomerRequest(t, business.ID, bill.ID,
		`{"fiscal_customer_doc_type":"DNI","fiscal_customer_doc_number":"12345678"}`)
	SetBillFiscalCustomer(c)
	require.Equal(t, http.StatusOK, w.Code)

	var got database.Bill
	require.NoError(t, database.GetDB().First(&got, bill.ID).Error)
	require.NotNil(t, got.FiscalCustomerEmail)
	assert.Equal(t, "keep@example.com", *got.FiscalCustomerEmail)
}

func TestSetBillFiscalCustomerByNumber_RejectsNonARVenue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	business, bill := seedFiscalCustomerBill(t)
	require.NoError(t, database.GetDB().Model(business).Update("country", "US").Error)

	body := `{"fiscal_customer_doc_type":"CUIT","fiscal_customer_doc_number":"20-11111111-2","fiscal_customer_tax_condition":"responsable_inscripto"}`
	c, w := newGuestFiscalCustomerRequest(t, bill.PublicToken, body)
	SetBillFiscalCustomerByNumber(c)

	require.Equal(t, http.StatusConflict, w.Code, "body: %s", w.Body.String())
	var reloaded database.Bill
	require.NoError(t, database.GetDB().First(&reloaded, bill.ID).Error)
	assert.Nil(t, reloaded.FiscalCustomerDocNumber)
}

func TestSetBillFiscalCustomer_RejectsNonARVenue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	business, bill := seedFiscalCustomerBill(t)
	require.NoError(t, database.GetDB().Model(business).Update("country", "US").Error)

	body := `{"fiscal_customer_doc_type":"CUIT","fiscal_customer_doc_number":"20-11111111-2"}`
	c, w := newFiscalCustomerRequest(t, business.ID, bill.ID, body)
	SetBillFiscalCustomer(c)

	require.Equal(t, http.StatusConflict, w.Code, "body: %s", w.Body.String())
	var reloaded database.Bill
	require.NoError(t, database.GetDB().First(&reloaded, bill.ID).Error)
	assert.Nil(t, reloaded.FiscalCustomerDocNumber)
}

func TestSetBillFiscalCustomerByNumber_RejectsUnknownCountry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	business, bill := seedFiscalCustomerBill(t)
	require.NoError(t, database.GetDB().Model(business).Update("country", "").Error)

	body := `{"fiscal_customer_doc_type":"DNI","fiscal_customer_doc_number":"12345678"}`
	c, w := newGuestFiscalCustomerRequest(t, bill.PublicToken, body)
	SetBillFiscalCustomerByNumber(c)

	require.Equal(t, http.StatusConflict, w.Code, "body: %s", w.Body.String())
}

func TestSetBillFiscalCustomerByNumber_EmptyObjectStillNoOpOnUSVenue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFiscalCustomerTestDB(t)
	business, bill := seedFiscalCustomerBill(t)
	require.NoError(t, database.GetDB().Model(business).Update("country", "US").Error)

	c, w := newGuestFiscalCustomerRequest(t, bill.PublicToken, `{}`)
	SetBillFiscalCustomerByNumber(c)
	require.Equal(t, http.StatusOK, w.Code, "body: %s", w.Body.String())
}
