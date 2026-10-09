package server

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"
	"github.com/stdevmac/payverge/backend/internal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const cartMenuItemID = "550e8400-e29b-41d4-a716-446655440000"

func orderableCartProjection() ([]database.MenuCategory, []database.Bundle) {
	return []database.MenuCategory{{
			ID: "mains", Name: "Principales",
			Items: []database.MenuItem{{
				ID: cartMenuItemID, Name: "Hamburguesa de la casa", Price: 18.5,
				Currency: "USD", IsAvailable: true,
			}},
		}}, []database.Bundle{{
			ID: 7, BusinessID: 42, Name: "Combo familiar", Price: 39,
			Currency: "USD", IsActive: true,
		}}
}

func TestValidateCartToolCalls_StableIDCanonicalMetadata(t *testing.T) {
	categories, bundles := orderableCartProjection()
	tests := []struct {
		name     string
		args     map[string]any
		wantArgs map[string]any
	}{
		{
			name: "menu UUID remains a string",
			args: map[string]any{"menu_item_id": cartMenuItemID, "bundle_id": nil, "quantity": float64(2), "item_name": "stale model label"},
			wantArgs: map[string]any{
				"item_type": "menu_item", "menu_item_id": cartMenuItemID,
				"item_name": "Hamburguesa de la casa", "price": 18.5, "currency": "USD", "quantity": float64(2),
			},
		},
		{
			name: "bundle string ID",
			args: map[string]any{"bundle_id": "7", "quantity": float64(1)},
			wantArgs: map[string]any{
				"item_type": "bundle", "bundle_id": "7",
				"item_name": "Combo familiar", "price": 39.0, "currency": "USD", "quantity": float64(1),
			},
		},
		{
			name: "bundle JSON number compatibility canonicalizes to string",
			args: map[string]any{"bundle_id": float64(7), "quantity": float64(1)},
			wantArgs: map[string]any{
				"item_type": "bundle", "bundle_id": "7",
				"item_name": "Combo familiar", "price": 39.0, "currency": "USD", "quantity": float64(1),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			valid, dropped := validateCartToolCalls([]llm.ToolCall{{ID: "call-1", Name: "add_to_cart", Args: test.args}}, categories, bundles)
			require.Equal(t, 0, dropped)
			require.Len(t, valid, 1)
			require.Equal(t, "call-1", valid[0].ID)
			require.Equal(t, test.wantArgs, valid[0].Args)
		})
	}
}

func TestValidateCartToolCalls_RequiresExactlyOneStableID(t *testing.T) {
	categories, bundles := orderableCartProjection()
	tests := []struct {
		name string
		args map[string]any
	}{
		{name: "neither", args: map[string]any{"quantity": float64(1)}},
		{name: "both", args: map[string]any{"menu_item_id": cartMenuItemID, "bundle_id": "7", "quantity": float64(1)}},
		{name: "blank menu item id", args: map[string]any{"menu_item_id": "  ", "quantity": float64(1)}},
		{name: "numeric UUID forbidden", args: map[string]any{"menu_item_id": float64(7), "quantity": float64(1)}},
		{name: "fractional bundle id", args: map[string]any{"bundle_id": 7.5, "quantity": float64(1)}},
		{name: "invalid preferred ID cannot downgrade to a name", args: map[string]any{
			"menu_item_id": "missing", "item_name": "Hamburguesa de la casa", "quantity": float64(1),
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			valid, dropped := validateCartToolCalls([]llm.ToolCall{{Name: "add_to_cart", Args: test.args}}, categories, bundles)
			require.Empty(t, valid)
			require.Equal(t, 1, dropped)
		})
	}
}

func TestValidateCartToolCalls_UnsafeNumericBundleIDRequiresStringIdentity(t *testing.T) {
	categories, bundles := orderableCartProjection()
	const largeBundleID = uint(9_007_199_254_740_992)
	bundles = append(bundles, database.Bundle{
		ID: largeBundleID, BusinessID: 42, Name: "Large ID bundle", IsActive: true,
	})

	valid, dropped := validateCartToolCalls([]llm.ToolCall{{
		Name: "add_to_cart", Args: map[string]any{"bundle_id": float64(largeBundleID), "quantity": float64(1)},
	}}, categories, bundles)
	require.Empty(t, valid, "unsafe JSON numbers can lose stable identity precision")
	require.Equal(t, 1, dropped)

	valid, dropped = validateCartToolCalls([]llm.ToolCall{{
		Name: "add_to_cart", Args: map[string]any{"bundle_id": "9007199254740992", "quantity": float64(1)},
	}}, categories, bundles)
	require.Equal(t, 0, dropped)
	require.Len(t, valid, 1)
	require.Equal(t, "9007199254740992", valid[0].Args["bundle_id"])
}

func TestValidateCartToolCalls_RejectsIDsOutsideOrderableSnapshotProjection(t *testing.T) {
	categories, bundles := orderableCartProjection()
	categories[0].Items = append(categories[0].Items, database.MenuItem{
		ID: "raw-unavailable", Name: "Raw unavailable dish", IsAvailable: false,
	})
	bundles = append(bundles, database.Bundle{ID: 8, BusinessID: 42, Name: "Inactive bundle", IsActive: false})

	for _, args := range []map[string]any{
		{"menu_item_id": "raw-unavailable", "quantity": float64(1)},
		{"bundle_id": "8", "quantity": float64(1)},
		{"menu_item_id": "foreign-business-item", "quantity": float64(1)},
		{"bundle_id": "999", "quantity": float64(1)},
	} {
		valid, dropped := validateCartToolCalls([]llm.ToolCall{{Name: "add_to_cart", Args: args}}, categories, bundles)
		require.Empty(t, valid, "raw, unavailable, inactive, or foreign identities are not snapshot-orderable: %#v", args)
		require.Equal(t, 1, dropped)
	}
}

func TestValidateCartToolCalls_QuantityMustBeIntegerFromOneToTwenty(t *testing.T) {
	categories, bundles := orderableCartProjection()
	for _, quantity := range []any{float64(0), float64(21), float64(1.5), float64(-1), "2", math.NaN(), math.Inf(1)} {
		valid, dropped := validateCartToolCalls([]llm.ToolCall{{
			Name: "add_to_cart", Args: map[string]any{"menu_item_id": cartMenuItemID, "quantity": quantity},
		}}, categories, bundles)
		require.Empty(t, valid, "quantity %#v must be rejected", quantity)
		require.Equal(t, 1, dropped)
	}
	for _, quantity := range []any{float64(1), 20} {
		valid, dropped := validateCartToolCalls([]llm.ToolCall{{
			Name: "add_to_cart", Args: map[string]any{"menu_item_id": cartMenuItemID, "quantity": quantity},
		}}, categories, bundles)
		require.Equal(t, 0, dropped)
		require.Len(t, valid, 1)
		require.Equal(t, float64(toIntForTest(quantity)), valid[0].Args["quantity"])
	}
}

func toIntForTest(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func TestValidateCartToolCalls_NotesAreSanitizedAndCappedByRunes(t *testing.T) {
	categories, bundles := orderableCartProjection()
	valid, dropped := validateCartToolCalls([]llm.ToolCall{{
		Name: "add_to_cart",
		Args: map[string]any{
			"menu_item_id": cartMenuItemID,
			"quantity":     float64(1),
			"notes":        "sin\ncebolla\t" + strings.Repeat("界", 220),
		},
	}}, categories, bundles)
	require.Equal(t, 0, dropped)
	require.Len(t, valid, 1)
	notes, ok := valid[0].Args["notes"].(string)
	require.True(t, ok)
	require.LessOrEqual(t, len([]rune(notes)), 200)
	require.NotContains(t, notes, "\n")
	require.NotContains(t, notes, "\t")
}

func TestValidateCartToolCalls_MalformedNotesRejectTheCall(t *testing.T) {
	categories, bundles := orderableCartProjection()
	valid, dropped := validateCartToolCalls([]llm.ToolCall{{
		Name: "add_to_cart",
		Args: map[string]any{"menu_item_id": cartMenuItemID, "quantity": float64(1), "notes": float64(42)},
	}}, categories, bundles)
	require.Empty(t, valid)
	require.Equal(t, 1, dropped)
}

func TestValidateCartToolCalls_V1ItemNameCompatibilityResolvesStableID(t *testing.T) {
	categories, bundles := orderableCartProjection()
	valid, dropped := validateCartToolCalls([]llm.ToolCall{{
		Name: "add_to_cart", Args: map[string]any{"item_name": "  hamburguesa  DE LA casa ", "quantity": float64(1)},
	}}, categories, bundles)
	require.Equal(t, 0, dropped)
	require.Len(t, valid, 1)
	require.Equal(t, cartMenuItemID, valid[0].Args["menu_item_id"])
	require.Equal(t, "menu_item", valid[0].Args["item_type"])
	require.Equal(t, "Hamburguesa de la casa", valid[0].Args["item_name"])
}

func TestValidateCartToolCalls_V1ItemTypeHintConstrainsNameResolution(t *testing.T) {
	categories, bundles := orderableCartProjection()
	tests := []struct {
		name        string
		args        map[string]any
		wantIDKey   string
		wantID      string
		wantDropped int
	}{
		{
			name:      "menu item hint resolves only menu items",
			args:      map[string]any{"item_type": " MENU_ITEM ", "item_name": "Hamburguesa de la casa", "quantity": float64(1)},
			wantIDKey: "menu_item_id", wantID: cartMenuItemID,
		},
		{
			name:      "bundle hint resolves bundle to stable id",
			args:      map[string]any{"item_type": " Bundle ", "item_name": "Combo familiar", "quantity": float64(1)},
			wantIDKey: "bundle_id", wantID: "7",
		},
		{
			name:        "bundle hint cannot select menu item",
			args:        map[string]any{"item_type": "bundle", "item_name": "Hamburguesa de la casa", "quantity": float64(1)},
			wantDropped: 1,
		},
		{
			name:        "menu item hint cannot select bundle",
			args:        map[string]any{"item_type": "menu_item", "item_name": "Combo familiar", "quantity": float64(1)},
			wantDropped: 1,
		},
		{
			name:        "unknown hint fails closed",
			args:        map[string]any{"item_type": "offer", "item_name": "Combo familiar", "quantity": float64(1)},
			wantDropped: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			valid, dropped := validateCartToolCalls([]llm.ToolCall{{Name: "add_to_cart", Args: test.args}}, categories, bundles)
			require.Equal(t, test.wantDropped, dropped)
			if test.wantDropped != 0 {
				require.Empty(t, valid)
				return
			}
			require.Len(t, valid, 1)
			require.Equal(t, test.wantID, valid[0].Args[test.wantIDKey])
		})
	}
}

func TestValidateCartToolCalls_AmbiguousLegacyNameFailsClosed(t *testing.T) {
	categories, bundles := orderableCartProjection()
	bundles[0].Name = categories[0].Items[0].Name
	valid, dropped := validateCartToolCalls([]llm.ToolCall{{
		Name: "add_to_cart", Args: map[string]any{"item_name": "Hamburguesa de la casa", "quantity": float64(1)},
	}}, categories, bundles)
	require.Empty(t, valid)
	require.Equal(t, 1, dropped)
}

func TestValidateCartToolCalls_DuplicateStableItemIDFailsClosedForIDAndLegacyName(t *testing.T) {
	categories, bundles := orderableCartProjection()
	categories = append(categories, database.MenuCategory{
		ID: "specials", Name: "Specials",
		Items: []database.MenuItem{{ID: cartMenuItemID, Name: "Conflicting dish", IsAvailable: true}},
	})
	for _, args := range []map[string]any{
		{"menu_item_id": cartMenuItemID, "quantity": float64(1)},
		{"item_name": "Hamburguesa de la casa", "quantity": float64(1)},
		{"item_name": "Conflicting dish", "quantity": float64(1)},
	} {
		valid, dropped := validateCartToolCalls([]llm.ToolCall{{Name: "add_to_cart", Args: args}}, categories, bundles)
		require.Empty(t, valid, "duplicate stable identity must be unreachable through every path: %#v", args)
		require.Equal(t, 1, dropped)
	}
}

func TestValidateCartToolCalls_DuplicateStableBundleIDFailsClosedInEveryOrder(t *testing.T) {
	categories, _ := orderableCartProjection()
	orders := [][]database.Bundle{
		{
			{ID: 7, BusinessID: 42, Name: "First bundle", IsActive: true},
			{ID: 7, BusinessID: 42, Name: "Conflicting bundle", IsActive: true},
		},
		{
			{ID: 7, BusinessID: 42, Name: "Conflicting bundle", IsActive: true},
			{ID: 7, BusinessID: 42, Name: "First bundle", IsActive: true},
		},
	}
	for orderIndex, bundles := range orders {
		for _, args := range []map[string]any{
			{"bundle_id": "7", "quantity": float64(1)},
			{"item_name": "First bundle", "quantity": float64(1)},
			{"item_name": "Conflicting bundle", "quantity": float64(1)},
		} {
			valid, dropped := validateCartToolCalls([]llm.ToolCall{{Name: "add_to_cart", Args: args}}, categories, bundles)
			require.Empty(t, valid, "duplicate bundle identity must fail closed in order %d: %#v", orderIndex, args)
			require.Equal(t, 1, dropped)
		}
	}
}

type cartToolCaptureProvider struct {
	request llm.GenerateRequest
}

func (provider *cartToolCaptureProvider) Generate(_ context.Context, request llm.GenerateRequest) (*llm.Response, error) {
	provider.request = request
	return &llm.Response{Text: "ok"}, nil
}

func captureWaiterRequest(t *testing.T, locale string) llm.GenerateRequest {
	t.Helper()
	provider := &cartToolCaptureProvider{}
	service, err := services.NewAIService(provider, llm.ModelConfig{
		Chat: "test-chat", Image: "test-image", Director: "test-director", Menu: "test-menu",
	})
	require.NoError(t, err)
	_, err = service.ChatWithWaiter(context.Background(), services.WaiterChatParams{
		AIName: "Sage", BusinessName: "Test", Language: locale, Mode: "ordering",
		MenuData:    `[{"id":"` + cartMenuItemID + `","name":"Burger"}]`,
		BundlesData: `[{"id":7,"name":"Combo"}]`,
		History:     []services.WaiterMessage{{Role: "user", Content: "Add it"}},
	})
	require.NoError(t, err)
	return provider.request
}

func TestAddToCartToolSchemaUsesStableIDs(t *testing.T) {
	request := captureWaiterRequest(t, "en")
	require.Len(t, request.Tools, 1)
	tool := request.Tools[0]
	require.Equal(t, "add_to_cart", tool.Name)
	require.NotNil(t, tool.Parameters)
	require.Equal(t, llm.TypeObject, tool.Parameters.Type)
	require.Equal(t, []string{"quantity"}, tool.Parameters.Required, "XOR identity is enforced by the server validator")
	require.NotNil(t, tool.Parameters.AdditionalProperties)
	require.False(t, *tool.Parameters.AdditionalProperties)

	properties := tool.Parameters.Properties
	require.Equal(t, llm.TypeString, properties["menu_item_id"].Type)
	require.Equal(t, llm.TypeString, properties["bundle_id"].Type)
	require.Equal(t, llm.TypeInteger, properties["quantity"].Type)
	require.Equal(t, llm.TypeString, properties["notes"].Type)
	require.NotNil(t, properties["notes"].MaxLength)
	require.Equal(t, 200, *properties["notes"].MaxLength)
	require.NotContains(t, properties, "item_name")
	require.NotContains(t, properties, "item_type")
}

func TestWaiterPromptRequiresStableDataBlockIDsAndDefersCartSuccess(t *testing.T) {
	tests := []struct {
		locale             string
		identityRule       string
		acknowledgeRule    string
		legacyNameIdentity string
	}{
		{locale: "en", identityRule: "Localized names are presentation only, never identity", acknowledgeRule: "Never say the item was added until the application acknowledges success", legacyNameIdentity: "Use the exact item name;"},
		{locale: "es", identityRule: "Los nombres localizados son solo presentación, nunca identidad", acknowledgeRule: "Nunca digas que se añadió hasta que la aplicación confirme el éxito", legacyNameIdentity: "Usa el nombre exacto del plato;"},
		{locale: "es-AR", identityRule: "Los nombres localizados son solo presentación, nunca identidad", acknowledgeRule: "Nunca digas que se agregó hasta que la aplicación confirme el éxito", legacyNameIdentity: "Usá el nombre exacto del plato;"},
	}
	for _, test := range tests {
		t.Run(test.locale, func(t *testing.T) {
			request := captureWaiterRequest(t, test.locale)
			require.Contains(t, request.System, "menu_item_id")
			require.Contains(t, request.System, "bundle_id")
			require.Contains(t, request.System, test.identityRule)
			require.Contains(t, request.System, test.acknowledgeRule)
			require.NotContains(t, request.System, test.legacyNameIdentity)
			require.NotContains(t, request.System, `item_type="bundle"`)
		})
	}
}

func TestValidateCartToolCalls_PreservesUnrelatedTools(t *testing.T) {
	categories, bundles := orderableCartProjection()
	call := llm.ToolCall{ID: "other", Name: "not_a_cart_tool", Args: map[string]any{"safe": true}}
	valid, dropped := validateCartToolCalls([]llm.ToolCall{call}, categories, bundles)
	require.Equal(t, 0, dropped)
	require.Equal(t, []llm.ToolCall{call}, valid)
	assert.Equal(t, true, valid[0].Args["safe"])
}
