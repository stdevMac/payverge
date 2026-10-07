package observability

import (
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/getsentry/sentry-go/attribute"
	"github.com/stretchr/testify/require"
)

func TestScrubEvent_NilSafe(t *testing.T) {
	require.Nil(t, ScrubEvent(nil, nil))
}

func TestScrubLog_NilSafe(t *testing.T) {
	require.Nil(t, ScrubLog(nil))
}

func TestScrubLog_RedactsBodyAndAttributes(t *testing.T) {
	log := &sentry.Log{
		Body: "guest@example.com",
		Attributes: map[string]attribute.Value{
			"authorization": attribute.StringValue("Bearer log-secret"),
			"safe_email":    attribute.StringValue("owner@example.com"),
			"safe_value":    attribute.StringValue("restaurant"),
			"count":         attribute.Int64Value(42),
			"emails":        attribute.StringSliceValue([]string{"safe", "owner@example.com"}),
		},
	}

	scrubbed := ScrubLog(log)

	require.Same(t, log, scrubbed)
	require.Equal(t, redactedValue, scrubbed.Body)
	require.Equal(t, redactedValue, scrubbed.Attributes["authorization"].AsString())
	require.Equal(t, redactedValue, scrubbed.Attributes["safe_email"].AsString())
	require.Equal(t, "restaurant", scrubbed.Attributes["safe_value"].AsString())
	require.Equal(t, int64(42), scrubbed.Attributes["count"].AsInt64())
	require.Equal(t, []string{"safe", redactedValue}, scrubbed.Attributes["emails"].AsStringSlice())
}

func TestScrubEvent_RedactsFirstClassTextFields(t *testing.T) {
	event := &sentry.Event{
		Message: "owner@example.com",
		Exception: []sentry.Exception{
			{
				Value: "Bearer exception-secret",
				Mechanism: &sentry.Mechanism{
					Data: map[string]interface{}{
						"token":      "secret",
						"safe_email": "owner@example.com",
						"nested": map[string]interface{}{
							"Authorization": "Bearer nested-secret",
						},
					},
				},
			},
		},
	}

	scrubbed := ScrubEvent(event, nil)

	require.Equal(t, redactedValue, scrubbed.Message)
	require.Equal(t, redactedValue, scrubbed.Exception[0].Value)
	require.Equal(t, redactedValue, scrubbed.Exception[0].Mechanism.Data["token"])
	require.Equal(t, redactedValue, scrubbed.Exception[0].Mechanism.Data["safe_email"])

	nested := scrubbed.Exception[0].Mechanism.Data["nested"].(map[string]interface{})
	require.Equal(t, redactedValue, nested["Authorization"])
}

func TestScrubEvent_RedactsTransactionAndFingerprintText(t *testing.T) {
	walletAddress := "0x1111111111111111111111111111111111111111"
	event := &sentry.Event{
		Transaction: "/guest/bill/123/participants/" + walletAddress + "?token=secret#checkout",
		Fingerprint: []string{
			"safe-fingerprint",
			"owner@example.com",
			walletAddress,
			"/inside/get_user/" + walletAddress + "?token=secret",
		},
	}

	scrubbed := ScrubEvent(event, nil)

	require.Contains(t, scrubbed.Transaction, "/guest/bill/%5BFiltered%5D/participants/%5BFiltered%5D")
	require.NotContains(t, scrubbed.Transaction, "/guest/bill/123/")
	require.NotContains(t, scrubbed.Transaction, walletAddress)
	require.NotContains(t, scrubbed.Transaction, "token=secret")
	require.NotContains(t, scrubbed.Transaction, "#checkout")

	require.Equal(t, "safe-fingerprint", scrubbed.Fingerprint[0])
	require.Equal(t, redactedValue, scrubbed.Fingerprint[1])
	require.Equal(t, redactedValue, scrubbed.Fingerprint[2])
	require.Contains(t, scrubbed.Fingerprint[3], "/inside/get_user/")
	require.Contains(t, scrubbed.Fingerprint[3], "%5BFiltered%5D")
	require.NotContains(t, scrubbed.Fingerprint[3], walletAddress)
	require.NotContains(t, scrubbed.Fingerprint[3], "token=secret")
}

func TestScrubEvent_SanitizesUnsafeUserIDs(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{name: "safe user ID", id: "user-42", want: "user-42"},
		{name: "safe staff ID", id: "staff:7", want: "staff:7"},
		{name: "safe customer ID", id: "customer:9", want: "customer:9"},
		{name: "email user ID", id: "owner@example.com", want: ""},
		{name: "wallet user ID", id: "0x0123456789abcdef0123456789abcdef01234567", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &sentry.Event{
				User: sentry.User{
					ID:        tt.id,
					Email:     "owner@example.com",
					IPAddress: "203.0.113.10",
					Username:  "owner",
					Name:      "Owner Name",
					Data: map[string]string{
						"token": "secret",
					},
				},
			}

			scrubbed := ScrubEvent(event, nil)

			require.Equal(t, tt.want, scrubbed.User.ID)
			require.Empty(t, scrubbed.User.Email)
			require.Empty(t, scrubbed.User.IPAddress)
			require.Empty(t, scrubbed.User.Username)
			require.Empty(t, scrubbed.User.Name)
			require.Empty(t, scrubbed.User.Data)
		})
	}
}

func TestScrubEvent_RemovesSensitiveRequestData(t *testing.T) {
	event := &sentry.Event{
		User: sentry.User{
			ID:        "user_123",
			Email:     "guest@example.com",
			IPAddress: "203.0.113.10",
			Username:  "guest",
			Name:      "Guest Name",
			Data: map[string]string{
				"phone": "+15551234567",
			},
		},
		Request: &sentry.Request{
			URL:         "https://api.payverge.io/api/v1/orders?token=secret#checkout",
			QueryString: "token=secret",
			Cookies:     "session=secret",
			Data:        `{"password":"secret"}`,
			Headers: map[string]string{
				"Authorization": "Bearer secret",
				"X-Auth":        "secret",
				"X-Request-Id":  "req_123",
			},
		},
		Tags: map[string]string{
			"payment_token": "tok_secret",
			"business_id":   "biz_123",
		},
		Contexts: map[string]sentry.Context{
			"payverge": {
				"customer_email": "guest@example.com",
				"business_id":    "biz_123",
			},
		},
	}

	scrubbed := ScrubEvent(event, nil)

	require.Same(t, event, scrubbed)
	require.Equal(t, "user_123", scrubbed.User.ID)
	require.Empty(t, scrubbed.User.Email)
	require.Empty(t, scrubbed.User.Username)
	require.Empty(t, scrubbed.User.IPAddress)
	require.Empty(t, scrubbed.User.Name)
	require.Empty(t, scrubbed.User.Data)

	require.Equal(t, "https://api.payverge.io/api/v1/orders", scrubbed.Request.URL)
	require.Empty(t, scrubbed.Request.QueryString)
	require.Empty(t, scrubbed.Request.Cookies)
	require.Empty(t, scrubbed.Request.Data)
	require.Equal(t, redactedValue, scrubbed.Request.Headers["Authorization"])
	require.Equal(t, redactedValue, scrubbed.Request.Headers["X-Auth"])
	require.Equal(t, "req_123", scrubbed.Request.Headers["X-Request-Id"])

	require.Equal(t, redactedValue, scrubbed.Tags["payment_token"])
	require.Equal(t, "biz_123", scrubbed.Tags["business_id"])
	require.Equal(t, redactedValue, scrubbed.Contexts["payverge"]["customer_email"])
	require.Equal(t, "biz_123", scrubbed.Contexts["payverge"]["business_id"])
}

func TestScrubEvent_RedactsRequestURLPathSensitiveSegments(t *testing.T) {
	walletAddress := "0x1111111111111111111111111111111111111111"
	event := &sentry.Event{
		Request: &sentry.Request{
			URL: "/guest/bill/123/participants/" + walletAddress + "?token=secret#checkout",
		},
	}

	scrubbed := ScrubEvent(event, nil)

	require.Contains(t, scrubbed.Request.URL, "/guest/bill/%5BFiltered%5D/participants/%5BFiltered%5D")
	require.NotContains(t, scrubbed.Request.URL, "/guest/bill/123/")
	require.NotContains(t, scrubbed.Request.URL, walletAddress)
	require.NotContains(t, scrubbed.Request.URL, "token=secret")
	require.NotContains(t, scrubbed.Request.URL, "#checkout")
}

func TestScrubEvent_RedactsStripeCheckoutSessionURLPathSegment(t *testing.T) {
	sessionID := "cs_test_1234567890abcdef"
	event := &sentry.Event{
		Request: &sentry.Request{
			URL: "/api/v1/subscription/stripe/status/" + sessionID + "?x=y",
		},
	}

	scrubbed := ScrubEvent(event, nil)

	require.Contains(t, scrubbed.Request.URL, "/api/v1/subscription/stripe/status/")
	require.Contains(t, scrubbed.Request.URL, "%5BFiltered%5D")
	require.NotContains(t, scrubbed.Request.URL, sessionID)
	require.NotContains(t, scrubbed.Request.URL, "x=y")
}

func TestScrubEvent_RedactsPublicRouteAccessTokens(t *testing.T) {
	tests := []struct {
		name       string
		rawURL     string
		secret     string
		wantPrefix string
		wantSuffix string
	}{
		{
			name:       "legacy public table code",
			rawURL:     "/api/v1/table/TABLE-guest-abc123?token=secret",
			secret:     "TABLE-guest-abc123",
			wantPrefix: "/api/v1/table/",
			wantSuffix: "",
		},
		{
			name:       "guest table code",
			rawURL:     "/api/v1/guest/table/TABLE-guest-abc123/bill?token=secret",
			secret:     "TABLE-guest-abc123",
			wantPrefix: "/api/v1/guest/table/",
			wantSuffix: "/bill",
		},
		{
			name:       "guest bill number",
			rawURL:     "https://api.payverge.io/api/v1/guest/bill/BILL-guest-abc123/payments/history?token=secret",
			secret:     "BILL-guest-abc123",
			wantPrefix: "https://api.payverge.io/api/v1/guest/bill/",
			wantSuffix: "/payments/history",
		},
		{
			name:       "reservation confirmation code",
			rawURL:     "/api/v1/reservations/RES-CONF-abc123/confirm?email=guest@example.com",
			secret:     "RES-CONF-abc123",
			wantPrefix: "/api/v1/reservations/",
			wantSuffix: "/confirm",
		},
		{
			name:       "delivery number",
			rawURL:     "/api/v1/delivery/DEL-guest-abc123/track?x=y",
			secret:     "DEL-guest-abc123",
			wantPrefix: "/api/v1/delivery/",
			wantSuffix: "/track",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := &sentry.Event{
				Request: &sentry.Request{
					URL: tt.rawURL,
				},
			}

			scrubbed := ScrubEvent(event, nil)

			require.Contains(t, scrubbed.Request.URL, tt.wantPrefix)
			require.Contains(t, scrubbed.Request.URL, "%5BFiltered%5D")
			require.Contains(t, scrubbed.Request.URL, tt.wantSuffix)
			require.NotContains(t, scrubbed.Request.URL, tt.secret)
			require.NotContains(t, scrubbed.Request.URL, "token=secret")
			require.NotContains(t, scrubbed.Request.URL, "guest@example.com")
		})
	}
}

func TestScrubEvent_RedactsSensitiveValuesUnderSafeKeys(t *testing.T) {
	event := &sentry.Event{
		Tags: map[string]string{
			"bearer_value":      "Bearer super-secret-token",
			"jwt_value":         "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjMifQ.signature",
			"stripe_value":      "sk_test_1234567890abcdef",
			"private_key_value": "-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----",
			"evm_key_value":     "0x0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			"wallet_value":      "0x0123456789abcdef0123456789abcdef01234567",
			"safe_value":        "biz_123",
		},
		Contexts: map[string]sentry.Context{
			"payverge": {
				"nested_safe_key": "Bearer nested-secret",
			},
		},
	}

	scrubbed := ScrubEvent(event, nil)

	require.Equal(t, redactedValue, scrubbed.Tags["bearer_value"])
	require.Equal(t, redactedValue, scrubbed.Tags["jwt_value"])
	require.Equal(t, redactedValue, scrubbed.Tags["stripe_value"])
	require.Equal(t, redactedValue, scrubbed.Tags["private_key_value"])
	require.Equal(t, redactedValue, scrubbed.Tags["evm_key_value"])
	require.Equal(t, redactedValue, scrubbed.Tags["wallet_value"])
	require.Equal(t, "biz_123", scrubbed.Tags["safe_value"])
	require.Equal(t, redactedValue, scrubbed.Contexts["payverge"]["nested_safe_key"])
}

func TestScrubEvent_RedactsBreadcrumbDataAndMessage(t *testing.T) {
	event := &sentry.Event{
		Breadcrumbs: []*sentry.Breadcrumb{
			nil,
			{
				Message: "Bearer breadcrumb-secret",
				Data: map[string]interface{}{
					"safe_bearer": "Bearer data-secret",
					"safe_email":  "guest@example.com",
					"token":       "secret",
					"nested": map[string]interface{}{
						"Authorization": "Bearer nested-secret",
					},
				},
			},
		},
	}

	scrubbed := ScrubEvent(event, nil)

	require.Nil(t, scrubbed.Breadcrumbs[0])
	breadcrumb := scrubbed.Breadcrumbs[1]
	require.Equal(t, redactedValue, breadcrumb.Message)
	require.Equal(t, redactedValue, breadcrumb.Data["safe_bearer"])
	require.Equal(t, redactedValue, breadcrumb.Data["safe_email"])
	require.Equal(t, redactedValue, breadcrumb.Data["token"])

	nested := breadcrumb.Data["nested"].(map[string]interface{})
	require.Equal(t, redactedValue, nested["Authorization"])
}

func TestScrubEvent_PreservesSafeOperationalKeys(t *testing.T) {
	safeValues := map[string]string{
		"auth_source": "oauth",
		"business_id": "biz_123",
		"request_id":  "req_123",
		"route":       "/inside/orders",
		"method":      "GET",
		"status":      "200",
		"service":     "backend",
		"environment": "production",
		"release":     "sha_123",
		"user_id":     "user-42",
		"staff_id":    "staff:7",
		"customer_id": "customer:9",
	}
	context := sentry.Context{}
	for key, value := range safeValues {
		context[key] = value
	}

	event := &sentry.Event{
		Tags: safeValues,
		Contexts: map[string]sentry.Context{
			"safe_keys": context,
		},
	}

	scrubbed := ScrubEvent(event, nil)

	for key, value := range map[string]string{
		"auth_source": "oauth",
		"business_id": "biz_123",
		"request_id":  "req_123",
		"route":       "/inside/orders",
		"method":      "GET",
		"status":      "200",
		"service":     "backend",
		"environment": "production",
		"release":     "sha_123",
		"user_id":     "user-42",
		"staff_id":    "staff:7",
		"customer_id": "customer:9",
	} {
		require.Equal(t, value, scrubbed.Tags[key], key)
		require.Equal(t, value, scrubbed.Contexts["safe_keys"][key], key)
	}
}

func TestScrubTransaction_DelegatesToScrubEvent(t *testing.T) {
	event := &sentry.Event{
		User: sentry.User{
			ID:    "user_123",
			Email: "guest@example.com",
		},
		Request: &sentry.Request{
			URL:         "/api/v1/orders?token=secret#checkout",
			QueryString: "token=secret",
			Cookies:     "session=secret",
			Data:        `{"token":"secret"}`,
			Headers: map[string]string{
				"Authorization": "Bearer secret",
				"X-Request-Id":  "req_123",
			},
		},
	}

	scrubbed := ScrubTransaction(event, nil)

	require.Same(t, event, scrubbed)
	require.Equal(t, "user_123", scrubbed.User.ID)
	require.Empty(t, scrubbed.User.Email)
	require.Equal(t, "/api/v1/orders", scrubbed.Request.URL)
	require.Empty(t, scrubbed.Request.QueryString)
	require.Empty(t, scrubbed.Request.Cookies)
	require.Empty(t, scrubbed.Request.Data)
	require.Equal(t, redactedValue, scrubbed.Request.Headers["Authorization"])
	require.Equal(t, "req_123", scrubbed.Request.Headers["X-Request-Id"])
}

func TestScrubTransaction_RedactsSpanTagsDataAndDescription(t *testing.T) {
	walletAddress := "0x1111111111111111111111111111111111111111"
	event := &sentry.Event{
		Spans: []*sentry.Span{
			nil,
			{
				Name:        "/inside/get_user/" + walletAddress + "?token=secret",
				Op:          "owner@example.com",
				Description: "Bearer span-secret",
				Tags: map[string]string{
					"safe_bearer": "Bearer tag-secret",
					"session":     "session-secret",
					"business_id": "biz_123",
				},
				Data: map[string]interface{}{
					"safe_email":    "guest@example.com",
					"payment_token": "tok_secret",
					"business_id":   "biz_123",
					"nested": map[string]interface{}{
						"safe_bearer": "Bearer nested-secret",
					},
				},
			},
		},
	}

	scrubbed := ScrubTransaction(event, nil)

	require.Nil(t, scrubbed.Spans[0])
	span := scrubbed.Spans[1]
	require.Contains(t, span.Name, "/inside/get_user/")
	require.Contains(t, span.Name, "%5BFiltered%5D")
	require.NotContains(t, span.Name, walletAddress)
	require.NotContains(t, span.Name, "token=secret")
	require.Equal(t, redactedValue, span.Op)
	require.Equal(t, redactedValue, span.Description)
	require.Equal(t, redactedValue, span.Tags["safe_bearer"])
	require.Equal(t, redactedValue, span.Tags["session"])
	require.Equal(t, "biz_123", span.Tags["business_id"])
	require.Equal(t, redactedValue, span.Data["safe_email"])
	require.Equal(t, redactedValue, span.Data["payment_token"])
	require.Equal(t, "biz_123", span.Data["business_id"])

	nested := span.Data["nested"].(map[string]interface{})
	require.Equal(t, redactedValue, nested["safe_bearer"])
}

func TestScrubEvent_RedactsRequiredSensitiveKeyFamilies(t *testing.T) {
	requiredKeys := []string{
		"auth",
		"authorization",
		"x-auth",
		"cookie",
		"session",
		"sessionid",
		"csrf",
		"xsrf",
		"credentials",
		"cf-connecting-ip",
		"forwarded",
		"x-forwarded-for",
		"x-real-ip",
		"true-client-ip",
		"remote-addr",
		"ip-address",
		"token",
		"secret",
		"password",
		"api key",
		"signature",
		"private key",
		"payment",
		"card",
		"email",
		"phone",
		"name",
	}
	context := sentry.Context{}
	for _, key := range requiredKeys {
		context[key] = "visible"
	}

	scrubbed := ScrubEvent(&sentry.Event{
		Contexts: map[string]sentry.Context{
			"sensitive_keys": context,
		},
	}, nil)

	for _, key := range requiredKeys {
		require.Equal(t, redactedValue, scrubbed.Contexts["sensitive_keys"][key], key)
	}
}

func TestScrubEvent_RedactsRecursiveNestedContextValues(t *testing.T) {
	walletAddress := "0x0123456789abcdef0123456789abcdef01234567"
	event := &sentry.Event{
		Contexts: map[string]sentry.Context{
			"recursive": {
				"level1": map[string]interface{}{
					"customer": map[string]interface{}{
						"email":       "guest@example.com",
						"business_id": "biz_123",
					},
					"items": []interface{}{
						map[string]interface{}{
							"phone": "+15551234567",
							"safe":  "ok",
						},
						"Bearer nested-secret",
						[]interface{}{walletAddress},
					},
				},
			},
		},
	}

	scrubbed := ScrubEvent(event, nil)

	level1 := scrubbed.Contexts["recursive"]["level1"].(map[string]interface{})
	customer := level1["customer"].(map[string]interface{})
	require.Equal(t, redactedValue, customer["email"])
	require.Equal(t, "biz_123", customer["business_id"])

	items := level1["items"].([]interface{})
	firstItem := items[0].(map[string]interface{})
	require.Equal(t, redactedValue, firstItem["phone"])
	require.Equal(t, "ok", firstItem["safe"])
	require.Equal(t, redactedValue, items[1])

	nestedArray := items[2].([]interface{})
	require.Equal(t, redactedValue, nestedArray[0])
}

func TestScrubEvent_RedactsNonStringKeyedMapValues(t *testing.T) {
	event := &sentry.Event{
		Contexts: map[string]sentry.Context{
			"non_string_maps": {
				"string_values": map[int]string{
					1: "Bearer int-secret",
					2: "ok",
				},
				"interface_values": map[int]interface{}{
					1: "guest@example.com",
					2: "ok",
				},
			},
		},
	}

	scrubbed := ScrubEvent(event, nil)

	stringValues := scrubbed.Contexts["non_string_maps"]["string_values"].(map[int]string)
	require.Equal(t, redactedValue, stringValues[1])
	require.Equal(t, "ok", stringValues[2])

	interfaceValues := scrubbed.Contexts["non_string_maps"]["interface_values"].(map[int]interface{})
	require.Equal(t, redactedValue, interfaceValues[1])
	require.Equal(t, "ok", interfaceValues[2])
}

func TestScrubEvent_RedactsTypedNestedMapAndSliceValues(t *testing.T) {
	event := &sentry.Event{
		Contexts: map[string]sentry.Context{
			"typed": {
				"typed_maps": []map[string]interface{}{
					{
						"email":       "guest@example.com",
						"business_id": "biz_123",
					},
				},
				"typed_string_map": map[string][]string{
					"token": {"secret"},
					"safe":  {"ok"},
				},
			},
		},
	}

	scrubbed := ScrubEvent(event, nil)

	typedMaps := scrubbed.Contexts["typed"]["typed_maps"].([]map[string]interface{})
	require.Equal(t, redactedValue, typedMaps[0]["email"])
	require.Equal(t, "biz_123", typedMaps[0]["business_id"])

	typedStringMap := scrubbed.Contexts["typed"]["typed_string_map"].(map[string][]string)
	require.Equal(t, []string{redactedValue}, typedStringMap["token"])
	require.Equal(t, []string{"ok"}, typedStringMap["safe"])
}

func TestScrubEvent_RedactsTypedStringAliasValues(t *testing.T) {
	type testToken string

	event := &sentry.Event{
		Contexts: map[string]sentry.Context{
			"typed_aliases": {
				"direct_alias": testToken("Bearer direct-secret"),
				"alias_slice": []testToken{
					testToken("Bearer slice-secret"),
					testToken("ok"),
				},
				"alias_map": map[string]testToken{
					"safe_key":   testToken("Bearer map-secret"),
					"safe_plain": testToken("ok"),
				},
			},
		},
	}

	scrubbed := ScrubEvent(event, nil)

	require.Equal(t, redactedValue, scrubbed.Contexts["typed_aliases"]["direct_alias"])

	aliasSlice := scrubbed.Contexts["typed_aliases"]["alias_slice"].([]testToken)
	require.Equal(t, testToken(redactedValue), aliasSlice[0])
	require.Equal(t, testToken("ok"), aliasSlice[1])

	aliasMap := scrubbed.Contexts["typed_aliases"]["alias_map"].(map[string]testToken)
	require.Equal(t, testToken(redactedValue), aliasMap["safe_key"])
	require.Equal(t, testToken("ok"), aliasMap["safe_plain"])
}

func TestSanitizeURL_FallbackStripsQuery(t *testing.T) {
	require.Equal(t, "/api/v1/orders", sanitizeURL("/api/v1/orders?token=secret"))
	require.Equal(t, "https://api.payverge.io/api/v1/orders", sanitizeURL("https://api.payverge.io/api/v1/orders?token=secret#checkout"))
	require.Equal(t, "https://api.payverge.io/api/v1/orders", sanitizeURL("https://user:pass@api.payverge.io/api/v1/orders?token=secret#checkout"))

	malformed := sanitizeURL("https://user:pass@api.payverge.io/%zz?token=secret#checkout")
	require.NotContains(t, malformed, "user:pass@")
	require.Equal(t, "https://api.payverge.io/%zz", malformed)
}
