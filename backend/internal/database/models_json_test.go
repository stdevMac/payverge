package database

import (
	"encoding/json"
	"testing"
	"time"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBillMarshalJSONEmitsMoneyAndOmitsEmptyRelations(t *testing.T) {
	bill := Bill{
		ID:               7,
		BusinessID:       3,
		TableID:          11,
		BillNumber:       "B-001",
		Notes:            `note with "quotes"`,
		Items:            `[{"name":"Burger"}]`,
		Subtotal:         12340,
		TaxAmount:        1234,
		ServiceFeeAmount: 0,
		TotalAmount:      13574,
		PaidAmount:       13574,
		TipAmount:        2000,
		Status:           BillStatusPaid,
		SettlementAddr:   "0xabc",
		TippingAddr:      "0xdef",
		CreatedAt:        time.Unix(1700000000, 0).UTC(),
		UpdatedAt:        time.Unix(1700000100, 0).UTC(),
	}

	raw, err := json.Marshal(bill)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))

	assert.Equal(t, float64(123.40), payload["subtotal"])
	assert.Equal(t, float64(12.34), payload["tax_amount"])
	assert.Equal(t, float64(0), payload["service_fee_amount"])
	assert.Equal(t, float64(135.74), payload["total_amount"])
	assert.Equal(t, float64(135.74), payload["paid_amount"])
	assert.Equal(t, float64(0), payload["remaining"])
	assert.Equal(t, float64(20), payload["tip_amount"])
	assert.Equal(t, `note with "quotes"`, payload["notes"])
	items, ok := payload["items"].([]any)
	require.True(t, ok, "bill.items must be a JSON array, not a string (got %T)", payload["items"])
	require.Len(t, items, 1)
	assert.Equal(t, "Burger", items[0].(map[string]any)["name"])
	assert.Contains(t, payload, "counter_id")
	assert.Nil(t, payload["counter_id"])
	assert.Contains(t, payload, "closed_at")
	assert.Nil(t, payload["closed_at"])
	assert.NotContains(t, payload, "settled_at")
	assert.NotContains(t, payload, "created_by_staff_id")
	assert.NotContains(t, payload, "feedback_email_sent_at")
	assert.NotContains(t, payload, "business")
	assert.NotContains(t, payload, "table")
	assert.Equal(t, "0xabc", payload["settlement_address"])
	assert.Equal(t, "0xdef", payload["tipping_address"])
}

// FIND-048/050: order-list preloadOrderListBillSummary leaves public_token /
// items / settlement / tipping unloaded. Emitting "" invents "no pay link /
// no wallets"; fiscal nulls + loyalty 0 invent "no fiscal customer / no loyalty".
func TestBillMarshalJSON_OmitsEmptyTokenWalletsAndItemsSnapshot(t *testing.T) {
	bill := Bill{
		ID:          749,
		BusinessID:  50,
		TableID:     483,
		BillNumber:  "B-749",
		Subtotal:    425,
		TotalAmount: 425,
		Status:      BillStatusOpen,
		CreatedAt:   time.Unix(1700000000, 0).UTC(),
		UpdatedAt:   time.Unix(1700000000, 0).UTC(),
	}

	raw, err := json.Marshal(bill)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))

	for _, banned := range []string{
		"public_token", "items", "settlement_address", "tipping_address",
		"loyalty_discount",
		"fiscal_customer_doc_type", "fiscal_customer_doc_number",
		"fiscal_customer_tax_condition", "fiscal_customer_name",
		"fiscal_customer_email",
	} {
		if _, ok := payload[banned]; ok {
			t.Errorf("order-list-style bill must not emit %q (got %v)", banned, payload[banned])
		}
	}
	assert.Equal(t, "B-749", payload["bill_number"])
	assert.Equal(t, float64(4.25), payload["total_amount"])
	assert.Equal(t, float64(4.25), payload["remaining"])

	// Full load still emits non-empty token / wallets / items + fiscal nulls + loyalty 0.
	bill.PublicToken = "tok_live_abc"
	bill.SettlementAddr = "0xsettle"
	bill.TippingAddr = "0xtip"
	bill.Items = `[{"name":"Tea"}]`
	raw, err = json.Marshal(bill)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &payload))
	assert.Equal(t, "tok_live_abc", payload["public_token"])
	assert.Equal(t, "0xsettle", payload["settlement_address"])
	assert.Equal(t, "0xtip", payload["tipping_address"])
	items, ok := payload["items"].([]any)
	require.True(t, ok, "bill.items must be a JSON array, not a string (got %T)", payload["items"])
	require.Len(t, items, 1)
	assert.Equal(t, "Tea", items[0].(map[string]any)["name"])
	assert.Equal(t, float64(0), payload["loyalty_discount"])
	assert.Contains(t, payload, "fiscal_customer_doc_type")
	assert.Nil(t, payload["fiscal_customer_doc_type"])
	assert.Contains(t, payload, "fiscal_customer_email")
	assert.Nil(t, payload["fiscal_customer_email"])
}

// #771: GetBill returns both top-level items (array) and nested bill.items.
// A quoted snapshot made bill.items.length the character count (454 / 1346).
func TestBillMarshalJSON_ItemsIsArrayNeverString(t *testing.T) {
	bill := Bill{
		ID:             761,
		BusinessID:     86,
		BillNumber:     "B-761",
		Items:          `[{"name":"Steak","quantity":1},{"name":"Wine","quantity":1}]`,
		Subtotal:       8240,
		TotalAmount:    8240,
		Status:         BillStatusOpen,
		PublicToken:    "tok_761",
		SettlementAddr: "0xabc",
		TippingAddr:    "0xdef",
		CreatedAt:      time.Unix(1700000000, 0).UTC(),
		UpdatedAt:      time.Unix(1700000000, 0).UTC(),
	}
	topLevel := []BillItem{
		{Name: "Steak", Quantity: 1, Price: 50, Subtotal: 50},
		{Name: "Wine", Quantity: 1, Price: 32.4, Subtotal: 32.4},
	}

	raw, err := json.Marshal(map[string]any{"bill": bill, "items": topLevel})
	require.NoError(t, err)

	var payload struct {
		Bill  map[string]any `json:"bill"`
		Items []any          `json:"items"`
	}
	require.NoError(t, json.Unmarshal(raw, &payload))
	require.Len(t, payload.Items, 2)

	nested, ok := payload.Bill["items"].([]any)
	require.True(t, ok, "nested bill.items must be an array, got %T %v", payload.Bill["items"], payload.Bill["items"])
	require.Len(t, nested, 2, "bill.items.length must be the line count, not strlen")
	assert.Equal(t, "Steak", nested[0].(map[string]any)["name"])
	assert.Equal(t, "Wine", nested[1].(map[string]any)["name"])

	if _, isString := payload.Bill["items"].(string); isString {
		t.Fatal("nested bill.items must never be a JSON string")
	}

	empty := Bill{ID: 1, BillNumber: "B-empty", Items: "[]", Status: BillStatusOpen}
	emptyRaw, err := json.Marshal(empty)
	require.NoError(t, err)
	var emptyPayload map[string]any
	require.NoError(t, json.Unmarshal(emptyRaw, &emptyPayload))
	if _, ok := emptyPayload["items"]; ok {
		t.Fatalf("empty items snapshot must be omitted, got %v", emptyPayload["items"])
	}

	invalid := Bill{ID: 2, BillNumber: "B-bad", Items: "not-json", PublicToken: "tok", SettlementAddr: "0xa", TippingAddr: "0xb"}
	invalidRaw, err := json.Marshal(invalid)
	require.NoError(t, err)
	var invalidPayload map[string]any
	require.NoError(t, json.Unmarshal(invalidRaw, &invalidPayload))
	if _, ok := invalidPayload["items"]; ok {
		t.Fatalf("invalid items snapshot must be omitted, never a string, got %v", invalidPayload["items"])
	}
}

func TestBillUnmarshalJSON_AcceptsArrayOrLegacyString(t *testing.T) {
	var fromArray Bill
	require.NoError(t, json.Unmarshal([]byte(`{"id":1,"items":[{"name":"Tea"}]}`), &fromArray))
	assert.JSONEq(t, `[{"name":"Tea"}]`, fromArray.Items)

	var fromString Bill
	require.NoError(t, json.Unmarshal([]byte(`{"id":2,"items":"[{\"name\":\"Tea\"}]"}`), &fromString))
	assert.JSONEq(t, `[{"name":"Tea"}]`, fromString.Items)
}

func TestOrderMarshalJSON_ItemsIsArrayNeverString(t *testing.T) {
	order := Order{
		ID:          1128,
		BillID:      761,
		BusinessID:  86,
		OrderNumber: "K-1128",
		Status:      OrderStatusInKitchen,
		Items:       `[{"menu_item_name":"Steak","quantity":1},{"menu_item_name":"Wine","quantity":1}]`,
		Bill: Bill{
			ID:          761,
			BillNumber:  "B-761",
			PublicToken: "tok_761",
			TotalAmount: 8240,
			Status:      BillStatusOpen,
		},
	}

	raw, err := json.Marshal(order)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))

	items, ok := payload["items"].([]any)
	require.True(t, ok, "order.items must be an array, got %T %v", payload["items"], payload["items"])
	require.Len(t, items, 2)
	assert.Equal(t, "Steak", items[0].(map[string]any)["menu_item_name"])

	if _, isString := payload["items"].(string); isString {
		t.Fatal("order.items must never be a JSON string")
	}

	empty := Order{ID: 1, BillID: 2, Items: ""}
	emptyRaw, err := json.Marshal(empty)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(emptyRaw, &payload))
	emptyItems, ok := payload["items"].([]any)
	require.True(t, ok, "empty order.items must still be an array, got %T", payload["items"])
	require.Len(t, emptyItems, 0)
}

func TestOrderUnmarshalJSON_AcceptsArrayOrLegacyString(t *testing.T) {
	var fromArray Order
	require.NoError(t, json.Unmarshal([]byte(`{"id":1,"items":[{"menu_item_name":"Soup"}]}`), &fromArray))
	assert.JSONEq(t, `[{"menu_item_name":"Soup"}]`, fromArray.Items)

	var fromString Order
	require.NoError(t, json.Unmarshal([]byte(`{"id":2,"items":"[{\"menu_item_name\":\"Soup\"}]"}`), &fromString))
	assert.JSONEq(t, `[{"menu_item_name":"Soup"}]`, fromString.Items)
}

func TestBillMarshalJSONIncludesLoadedRelations(t *testing.T) {
	bill := Bill{
		ID:             1,
		BusinessID:     2,
		TableID:        3,
		BillNumber:     "B-002",
		Subtotal:       1000,
		TotalAmount:    1000,
		Status:         BillStatusOpen,
		SettlementAddr: "0xabc",
		TippingAddr:    "0xdef",
		CreatedAt:      time.Unix(1700000000, 0).UTC(),
		UpdatedAt:      time.Unix(1700000100, 0).UTC(),
		Business: Business{
			ID:   2,
			Name: "Loaded Business",
		},
		Table: Table{
			ID:   3,
			Name: "T1",
		},
		Payments: []Payment{
			{ID: 4, BillID: 1, Amount: 1000, Status: PaymentStatusConfirmed},
		},
		ItemsRelation: []BillItem{
			{ID: "item-1", BillID: 1, Name: "Burger", Price: 10, Quantity: 1, Subtotal: 10},
		},
	}

	raw, err := json.Marshal(bill)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))

	assert.Contains(t, payload, "business")
	assert.Contains(t, payload, "table")
	assert.Contains(t, payload, "payments")
	assert.Contains(t, payload, "items_relation")
}

func TestBillMarshalJSONReplacesInvalidUTF8(t *testing.T) {
	bill := Bill{
		BillNumber: string([]byte{'b', 'a', 'd', 0xff, 'x'}),
		Notes:      string([]byte{0xfe, 'n', 'o', 't', 'e'}),
		CreatedAt:  time.Unix(1700000000, 0).UTC(),
		UpdatedAt:  time.Unix(1700000100, 0).UTC(),
	}

	raw, err := json.Marshal(bill)
	require.NoError(t, err)
	assert.True(t, json.Valid(raw))

	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))

	want := string([]rune{unicode.ReplacementChar, 'n', 'o', 't', 'e'})
	assert.Equal(t, want, payload["notes"])
	assert.Equal(t, "bad"+string(unicode.ReplacementChar)+"x", payload["bill_number"])
}

func TestBillMarshalJSON_BusinessOmitsSensitiveFields(t *testing.T) {
	b := Bill{
		ID: 1, BusinessID: 7, Status: BillStatusOpen,
		Business: Business{
			ID: 7, BusinessId: "biz_7", Name: "Demo", Logo: "logo.png",
			Timezone: "Asia/Dubai", DefaultCurrency: "AED", DisplayCurrency: "AED",
			OwnerAddress: "0xOWNER", OwnerName: "Alice", SettlementAddr: "0xSETTLE",
			TippingAddr: "0xTIP", Email: "owner@x.com", Phone: "+100",
		},
	}
	raw, err := json.Marshal(b)
	require.NoError(t, err)
	var out struct {
		Business map[string]any `json:"business"`
	}
	require.NoError(t, json.Unmarshal(raw, &out))

	for _, leaked := range []string{
		"owner_address", "owner_name", "settlement_address", "tipping_address",
		"email", "phone", "stripe_customer_id", "subscription_status",
	} {
		if _, ok := out.Business[leaked]; ok {
			t.Errorf("bill.business leaked %q", leaked)
		}
	}

	for _, want := range []string{
		"id", "business_id", "name", "timezone", "display_currency",
	} {
		if _, ok := out.Business[want]; !ok {
			t.Errorf("bill.business missing safe field %q", want)
		}
	}
}

func TestTableMarshalJSON_BusinessOmitsSensitiveFields(t *testing.T) {
	// A Table embeds the full Business row. When a bill emits its table relation,
	// table.business must not leak owner/settlement/Stripe fields. (SEC-2 cont.)
	tb := Table{
		ID: 3, BusinessID: 7, Name: "T1", TableCode: "ABC",
		Business: Business{
			ID: 7, BusinessId: "biz_7", Name: "Demo", Timezone: "Asia/Dubai",
			DefaultCurrency: "AED", DisplayCurrency: "AED",
			OwnerAddress: "0xOWNER", OwnerName: "Alice", SettlementAddr: "0xSETTLE",
			TippingAddr: "0xTIP", Email: "owner@x.com", Phone: "+100",
		},
	}
	raw, err := json.Marshal(tb)
	require.NoError(t, err)
	var loaded struct {
		Business map[string]any `json:"business"`
	}
	require.NoError(t, json.Unmarshal(raw, &loaded))
	require.NotNil(t, loaded.Business, "loaded table should carry a safe business summary")
	for _, leaked := range []string{
		"owner_address", "owner_name", "settlement_address", "tipping_address",
		"email", "phone", "stripe_customer_id", "subscription_status",
	} {
		if _, ok := loaded.Business[leaked]; ok {
			t.Errorf("table.business leaked %q", leaked)
		}
	}
	if _, ok := loaded.Business["name"]; !ok {
		t.Error("table.business missing safe field \"name\"")
	}

	// An unloaded Business (zero ID) must be omitted entirely, not emitted empty.
	rawEmpty, err := json.Marshal(Table{ID: 3, Name: "T1"})
	require.NoError(t, err)
	var unloaded map[string]any
	require.NoError(t, json.Unmarshal(rawEmpty, &unloaded))
	if _, ok := unloaded["business"]; ok {
		t.Error("unloaded table.business should be omitted, not emitted")
	}
}

func TestTableMarshalJSON_PartialProjectionIsSlim(t *testing.T) {
	// Mirrors preloadOrderListTableName: only id + name are selected for KDS.
	raw, err := json.Marshal(Table{ID: 484, Name: "Table 2"})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	require.Equal(t, float64(484), m["id"])
	require.Equal(t, "Table 2", m["name"])
	// Must not invent zero-value QR / activity fields that look authoritative.
	for _, banned := range []string{
		"business_id", "table_code", "capacity", "qr_code", "is_active",
		"qr_logo_url", "qr_foreground_color", "qr_background_color",
		"qr_logo_size", "qr_show_business_name", "qr_show_table_name",
		"qr_text_font", "created_at", "updated_at", "business",
	} {
		if _, ok := m[banned]; ok {
			t.Errorf("partial table projection must not emit %q (got %v)", banned, m[banned])
		}
	}
	require.Len(t, m, 2, "partial table JSON must be exactly {id,name}")
}

// FIND-046: reservation list preloads Select("id","name","table_code","capacity").
// FIND-040 only slimmed when table_code was empty, so reservation nests still
// invented is_active:false, business_id:0, empty QR fields, and zero timestamps.
func TestTableMarshalJSON_ReservationSummaryProjectionIsSlim(t *testing.T) {
	raw, err := json.Marshal(Table{
		ID:        486,
		Name:      "Table 4",
		TableCode: "demo-50-core-table-04",
		Capacity:  6,
	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	require.Equal(t, float64(486), m["id"])
	require.Equal(t, "Table 4", m["name"])
	require.Equal(t, "demo-50-core-table-04", m["table_code"])
	require.Equal(t, float64(6), m["capacity"])
	for _, banned := range []string{
		"business_id", "qr_code", "is_active",
		"qr_logo_url", "qr_foreground_color", "qr_background_color",
		"qr_logo_size", "qr_show_business_name", "qr_show_table_name",
		"qr_text_font", "created_at", "updated_at", "business",
	} {
		if _, ok := m[banned]; ok {
			t.Errorf("reservation table summary must not emit %q (got %v)", banned, m[banned])
		}
	}
	require.Len(t, m, 4, "reservation table summary must be exactly {id,name,table_code,capacity}")
}

func TestTableMarshalJSON_FullRowStillEmitsCoreFields(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	raw, err := json.Marshal(Table{
		ID: 1, BusinessID: 50, TableCode: "demo-50-core-table-01",
		Name: "Table 1", Capacity: 4, IsActive: true,
		CreatedAt: now, UpdatedAt: now,
	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	require.Equal(t, "demo-50-core-table-01", m["table_code"])
	require.Equal(t, true, m["is_active"])
	require.Equal(t, float64(50), m["business_id"])
	require.Equal(t, float64(4), m["capacity"])
}

// FIND-047: delivery dispatch preloads Select("id","name","phone","email") on
// Customer. Default marshal invented is_active:false + email_verified:false +
// empty wallet/profile + zero timestamps.
func TestCustomerMarshalJSON_PartialProjectionIsSlim(t *testing.T) {
	raw, err := json.Marshal(Customer{
		ID:    19,
		Name:  "Demo Guest 04",
		Phone: "+12125551003",
		Email: "demo+admin8-business50-customer3@payverge.local",
	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	require.Equal(t, float64(19), m["id"])
	require.Equal(t, "Demo Guest 04", m["name"])
	require.Equal(t, "+12125551003", m["phone"])
	require.Equal(t, "demo+admin8-business50-customer3@payverge.local", m["email"])
	for _, banned := range []string{
		"wallet_address", "birthday", "profile_image_url", "is_active",
		"email_verified", "last_login_at", "created_at", "updated_at",
		"business_connections", "preferences",
	} {
		if _, ok := m[banned]; ok {
			t.Errorf("partial customer must not emit %q (got %v)", banned, m[banned])
		}
	}
	require.Len(t, m, 4, "partial customer JSON must be exactly {id,name,phone,email}")
}

func TestCustomerMarshalJSON_FullRowKeepsStatusFields(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	raw, err := json.Marshal(Customer{
		ID:            21,
		Email:         "full@example.com",
		Name:          "Full Guest",
		Phone:         "+10000000000",
		WalletAddress: "0xabc",
		IsActive:      true,
		EmailVerified: true,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	require.Equal(t, true, m["is_active"])
	require.Equal(t, true, m["email_verified"])
	require.Equal(t, "0xabc", m["wallet_address"])
	require.Equal(t, "full@example.com", m["email"])
}

// FIND-052: RBAC audit preload Select("id","name","email") on Staff. Default
// marshal invented is_active:false, authz_version:0, empty role, business_id:0.
func TestStaffMarshalJSON_PartialProjectionIsSlim(t *testing.T) {
	raw, err := json.Marshal(Staff{
		ID:    12,
		Name:  "Demo Server",
		Email: "server@local.test",
	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	require.Equal(t, float64(12), m["id"])
	require.Equal(t, "Demo Server", m["name"])
	require.Equal(t, "server@local.test", m["email"])
	for _, banned := range []string{
		"business_id", "role", "is_active", "authz_version", "role_level",
		"custom_permissions", "invited_by", "last_login_at",
		"permissions_updated_at", "permissions_updated_by",
		"created_at", "updated_at",
	} {
		if _, ok := m[banned]; ok {
			t.Errorf("partial staff must not emit %q (got %v)", banned, m[banned])
		}
	}
	require.Len(t, m, 3, "partial staff JSON must be exactly {id,name,email}")
}

func TestStaffMarshalJSON_FullRowKeepsRoleAndActive(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	raw, err := json.Marshal(Staff{
		ID:           3,
		BusinessID:   50,
		Email:        "manager@local.test",
		Name:         "Demo Manager",
		Role:         StaffRoleManager,
		IsActive:     true,
		AuthzVersion: 2,
		InvitedBy:    "0xowner",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	require.Equal(t, true, m["is_active"])
	require.Equal(t, string(StaffRoleManager), m["role"])
	require.Equal(t, float64(2), m["authz_version"])
	require.Equal(t, float64(50), m["business_id"])
}
