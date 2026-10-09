package observability

import (
	"net/url"
	"reflect"
	"regexp"
	"strings"

	"github.com/getsentry/sentry-go"
	"github.com/getsentry/sentry-go/attribute"
)

const redactedValue = "[Filtered]"

var (
	sensitiveValuePattern = regexp.MustCompile(
		`(?i)(bearer\s+[a-z0-9._~+/=-]+|[a-z0-9_-]+\.[a-z0-9_-]+\.[a-z0-9_-]+|[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}|(?:sk|pk)_(?:live|test)_[a-z0-9]+|cs_(?:test|live)_[a-z0-9_]+|-----BEGIN [A-Z ]*PRIVATE KEY-----|0x[0-9a-f]{64}|0x[0-9a-f]{40})`,
	)
	safeKeyNames = map[string]struct{}{
		"auth_source":      {},
		"business_id":      {},
		"customer_id":      {},
		"environment":      {},
		"event_id":         {},
		"method":           {},
		"release":          {},
		"request_id":       {},
		"route":            {},
		"service":          {},
		"span_id":          {},
		"staff_id":         {},
		"status":           {},
		"trace_id":         {},
		"user_id":          {},
		"x_request_id":     {},
		"x_correlation_id": {},
	}
	sensitiveKeyNames = map[string]struct{}{
		"api_key":          {},
		"apikey":           {},
		"auth":             {},
		"authorization":    {},
		"card":             {},
		"cf_connecting_ip": {},
		"cfconnectingip":   {},
		"cookie":           {},
		"credentials":      {},
		"credential":       {},
		"csrf":             {},
		"email":            {},
		"forwarded":        {},
		"ip_address":       {},
		"ipaddress":        {},
		"name":             {},
		"passwd":           {},
		"password":         {},
		"payment":          {},
		"phone":            {},
		"private_key":      {},
		"privatekey":       {},
		"remote_addr":      {},
		"remoteaddr":       {},
		"secret":           {},
		"session":          {},
		"sessionid":        {},
		"set_cookie":       {},
		"setcookie":        {},
		"signature":        {},
		"token":            {},
		"true_client_ip":   {},
		"trueclientip":     {},
		"x_auth":           {},
		"x_forwarded_for":  {},
		"x_real_ip":        {},
		"xauth":            {},
		"xforwardedfor":    {},
		"xrealip":          {},
		"xsrf":             {},
	}
)

func ScrubEvent(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	if event == nil {
		return nil
	}

	event.User = sentry.User{ID: sanitizeUserID(event.User.ID)}
	event.Message = sanitizeStringValue(event.Message)
	event.Transaction = sanitizeStringValue(event.Transaction)
	event.Fingerprint = sanitizeStringSlice(event.Fingerprint)
	event.Exception = sanitizeExceptions(event.Exception)

	if event.Request != nil {
		event.Request.URL = sanitizeURL(event.Request.URL)
		event.Request.QueryString = ""
		event.Request.Cookies = ""
		event.Request.Data = ""
		event.Request.Headers = sanitizeStringMap(event.Request.Headers)
		event.Request.Env = sanitizeStringMap(event.Request.Env)
	}

	event.Tags = sanitizeStringMap(event.Tags)
	event.Contexts = sanitizeContexts(event.Contexts)
	event.Breadcrumbs = sanitizeBreadcrumbs(event.Breadcrumbs)
	event.Spans = sanitizeSpans(event.Spans)

	return event
}

func ScrubTransaction(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
	return ScrubEvent(event, hint)
}

func ScrubLog(log *sentry.Log) *sentry.Log {
	if log == nil {
		return nil
	}

	log.Body = sanitizeStringValue(log.Body)
	log.Attributes = sanitizeLogAttributes(log.Attributes)
	return log
}

func sanitizeUserID(id string) string {
	if isSensitiveString(id) {
		return ""
	}

	return id
}

func sanitizeURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return stripURLSecrets(rawURL)
	}

	parsed.RawQuery = ""
	parsed.ForceQuery = false
	parsed.Fragment = ""
	parsed.User = nil
	parsed.Path = sanitizePathSegments(parsed.Path, false)
	parsed.RawPath = ""
	return parsed.String()
}

func stripURLSecrets(rawURL string) string {
	withoutQuery := rawURL
	if index := strings.Index(withoutQuery, "?"); index >= 0 {
		withoutQuery = withoutQuery[:index]
	}
	if index := strings.Index(withoutQuery, "#"); index >= 0 {
		withoutQuery = withoutQuery[:index]
	}

	return sanitizeURLPath(stripURLUserinfo(withoutQuery), true)
}

func stripURLUserinfo(rawURL string) string {
	prefix, rest, ok := splitURLAuthority(rawURL)
	if !ok {
		return rawURL
	}

	authorityEnd := strings.Index(rest, "/")
	if authorityEnd < 0 {
		authorityEnd = len(rest)
	}

	authority := rest[:authorityEnd]
	if at := strings.LastIndex(authority, "@"); at >= 0 {
		authority = authority[at+1:]
	}

	return prefix + authority + rest[authorityEnd:]
}

func splitURLAuthority(rawURL string) (string, string, bool) {
	if index := strings.Index(rawURL, "://"); index >= 0 {
		return rawURL[:index+3], rawURL[index+3:], true
	}
	if strings.HasPrefix(rawURL, "//") {
		return "//", rawURL[2:], true
	}

	return "", "", false
}

func sanitizeURLPath(rawURL string, escapeRedaction bool) string {
	prefix, rest, ok := splitURLAuthority(rawURL)
	if !ok {
		return sanitizePathSegments(rawURL, escapeRedaction)
	}

	authorityEnd := strings.Index(rest, "/")
	if authorityEnd < 0 {
		return rawURL
	}

	return prefix + rest[:authorityEnd] + sanitizePathSegments(rest[authorityEnd:], escapeRedaction)
}

func sanitizePathSegments(path string, escapeRedaction bool) string {
	if path == "" {
		return ""
	}

	segments := strings.Split(path, "/")
	sensitiveRouteIndexes := sensitiveRouteSegmentIndexes(segments)
	for i, segment := range segments {
		if segment == "" {
			continue
		}

		value := segment
		if unescaped, err := url.PathUnescape(segment); err == nil {
			value = unescaped
		}
		_, isSensitiveRouteValue := sensitiveRouteIndexes[i]
		if isSensitiveRouteValue || isSensitiveString(value) {
			if escapeRedaction {
				segments[i] = url.PathEscape(redactedValue)
			} else {
				segments[i] = redactedValue
			}
		}
	}

	return strings.Join(segments, "/")
}

type routeSegment struct {
	index int
	value string
}

func sensitiveRouteSegmentIndexes(segments []string) map[int]struct{} {
	routeSegments := make([]routeSegment, 0, len(segments))
	for index, segment := range segments {
		if segment == "" {
			continue
		}

		value := segment
		if unescaped, err := url.PathUnescape(segment); err == nil {
			value = unescaped
		}

		routeSegments = append(routeSegments, routeSegment{
			index: index,
			value: strings.ToLower(value),
		})
	}

	redactIndexes := map[int]struct{}{}
	start := 0
	if len(routeSegments) >= 2 && routeSegments[0].value == "api" && routeSegments[1].value == "v1" {
		start = 2
	}

	addRedaction := func(routeIndex int) {
		if routeIndex >= 0 && routeIndex < len(routeSegments) {
			redactIndexes[routeSegments[routeIndex].index] = struct{}{}
		}
	}

	if len(routeSegments) > start+1 && routeSegments[start].value == "t" {
		addRedaction(start + 1)
	}

	if len(routeSegments) > start+1 && routeSegments[start].value == "table" {
		addRedaction(start + 1)
	}

	if len(routeSegments) > start+2 &&
		routeSegments[start].value == "guest" &&
		routeSegments[start+1].value == "table" {
		addRedaction(start + 2)
	}

	if len(routeSegments) > start+2 &&
		routeSegments[start].value == "guest" &&
		routeSegments[start+1].value == "bill" {
		addRedaction(start + 2)
	}

	if len(routeSegments) > start+1 && routeSegments[start].value == "reservations" {
		addRedaction(start + 1)
	}

	if len(routeSegments) > start+2 &&
		routeSegments[start].value == "delivery" &&
		routeSegments[start+2].value == "track" {
		addRedaction(start + 1)
	}

	return redactIndexes
}

func sanitizeStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}

	for key, value := range values {
		if isSensitiveKey(key) {
			values[key] = redactedValue
			continue
		}

		if sanitized, ok := sanitizeValue(value).(string); ok {
			values[key] = sanitized
		}
	}

	return values
}

func sanitizeLogAttributes(values map[string]attribute.Value) map[string]attribute.Value {
	if values == nil {
		return nil
	}

	for key, value := range values {
		if isSensitiveKey(key) {
			values[key] = attribute.StringValue(redactedValue)
			continue
		}

		values[key] = sanitizeAttributeValue(value)
	}

	return values
}

func sanitizeAttributeValue(value attribute.Value) attribute.Value {
	switch value.Type() {
	case attribute.STRING:
		return attribute.StringValue(sanitizeStringValue(value.AsString()))
	case attribute.STRINGSLICE:
		values := value.AsStringSlice()
		sanitized := make([]string, len(values))
		for i, entry := range values {
			sanitized[i] = sanitizeStringValue(entry)
		}
		return attribute.StringSliceValue(sanitized)
	default:
		return value
	}
}

func sanitizeContexts(contexts map[string]sentry.Context) map[string]sentry.Context {
	if contexts == nil {
		return nil
	}

	for key, context := range contexts {
		if isSensitiveKey(key) {
			contexts[key] = sentry.Context{"value": redactedValue}
			continue
		}
		contexts[key] = sanitizeInterfaceMap(context)
	}

	return contexts
}

func sanitizeBreadcrumbs(breadcrumbs []*sentry.Breadcrumb) []*sentry.Breadcrumb {
	for _, breadcrumb := range breadcrumbs {
		if breadcrumb == nil {
			continue
		}

		breadcrumb.Data = sanitizeInterfaceMap(breadcrumb.Data)
		breadcrumb.Message = sanitizeStringValue(breadcrumb.Message)
	}

	return breadcrumbs
}

func sanitizeSpans(spans []*sentry.Span) []*sentry.Span {
	for _, span := range spans {
		if span == nil {
			continue
		}

		span.Tags = sanitizeStringMap(span.Tags)
		span.Data = sanitizeInterfaceMap(span.Data)
		span.Name = sanitizeStringValue(span.Name)
		span.Op = sanitizeStringValue(span.Op)
		span.Description = sanitizeStringValue(span.Description)
	}

	return spans
}

func sanitizeExceptions(exceptions []sentry.Exception) []sentry.Exception {
	for i := range exceptions {
		exceptions[i].Value = sanitizeStringValue(exceptions[i].Value)
		if exceptions[i].Mechanism != nil {
			exceptions[i].Mechanism.Data = sanitizeInterfaceMap(exceptions[i].Mechanism.Data)
		}
	}

	return exceptions
}

func sanitizeInterfaceMap(values map[string]interface{}) map[string]interface{} {
	if values == nil {
		return nil
	}

	for key, value := range values {
		if isSensitiveKey(key) {
			values[key] = redactedValue
			continue
		}

		values[key] = sanitizeValue(value)
	}

	return values
}

func sanitizeValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case string:
		return sanitizeStringValue(typed)
	case []string:
		sanitized := make([]string, len(typed))
		for i, entry := range typed {
			sanitized[i] = sanitizeStringValue(entry)
		}
		return sanitized
	case []interface{}:
		sanitized := make([]interface{}, len(typed))
		for i, entry := range typed {
			sanitized[i] = sanitizeValue(entry)
		}
		return sanitized
	case map[string]string:
		return sanitizeStringMap(typed)
	case map[string]interface{}:
		return sanitizeInterfaceMap(typed)
	default:
		return sanitizeTypedValue(value)
	}
}

func sanitizeStringSlice(values []string) []string {
	for i, value := range values {
		values[i] = sanitizeStringValue(value)
	}

	return values
}

func sanitizeTypedValue(value interface{}) interface{} {
	if value == nil {
		return nil
	}

	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.String:
		if isSensitiveString(reflected.String()) {
			return redactedValue
		}
		return value
	case reflect.Map:
		return sanitizeReflectMap(reflected)
	case reflect.Slice:
		return sanitizeReflectSlice(reflected)
	case reflect.Array:
		return sanitizeReflectArray(reflected)
	default:
		return value
	}
}

func sanitizeReflectMap(value reflect.Value) interface{} {
	if value.IsNil() {
		return value.Interface()
	}

	sanitized := reflect.MakeMapWithSize(value.Type(), value.Len())
	elementType := value.Type().Elem()
	hasStringKeys := value.Type().Key().Kind() == reflect.String
	for _, key := range value.MapKeys() {
		if hasStringKeys && isSensitiveKey(key.String()) {
			sanitized.SetMapIndex(key, redactedReflectValue(elementType))
			continue
		}

		sanitized.SetMapIndex(key, reflectValueForType(sanitizeValue(value.MapIndex(key).Interface()), elementType))
	}

	return sanitized.Interface()
}

func sanitizeReflectSlice(value reflect.Value) interface{} {
	if value.IsNil() {
		return value.Interface()
	}

	sanitized := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
	elementType := value.Type().Elem()
	for i := 0; i < value.Len(); i++ {
		sanitized.Index(i).Set(reflectValueForType(sanitizeValue(value.Index(i).Interface()), elementType))
	}

	return sanitized.Interface()
}

func sanitizeReflectArray(value reflect.Value) interface{} {
	sanitized := reflect.New(value.Type()).Elem()
	elementType := value.Type().Elem()
	for i := 0; i < value.Len(); i++ {
		sanitized.Index(i).Set(reflectValueForType(sanitizeValue(value.Index(i).Interface()), elementType))
	}

	return sanitized.Interface()
}

func reflectValueForType(value interface{}, target reflect.Type) reflect.Value {
	if value == nil {
		return reflect.Zero(target)
	}

	reflected := reflect.ValueOf(value)
	if reflected.Type().AssignableTo(target) {
		return reflected
	}
	if reflected.Type().ConvertibleTo(target) {
		return reflected.Convert(target)
	}
	if target.Kind() == reflect.Interface && reflected.Type().Implements(target) {
		return reflected
	}

	return reflect.Zero(target)
}

func redactedReflectValue(target reflect.Type) reflect.Value {
	redacted := reflect.ValueOf(redactedValue)
	if redacted.Type().AssignableTo(target) {
		return redacted
	}
	if redacted.Type().ConvertibleTo(target) {
		return redacted.Convert(target)
	}

	switch target.Kind() {
	case reflect.Interface:
		return redacted
	case reflect.Slice:
		sanitized := reflect.MakeSlice(target, 1, 1)
		sanitized.Index(0).Set(redactedReflectValue(target.Elem()))
		return sanitized
	case reflect.Array:
		sanitized := reflect.New(target).Elem()
		if target.Len() > 0 {
			sanitized.Index(0).Set(redactedReflectValue(target.Elem()))
		}
		return sanitized
	case reflect.Map:
		sanitized := reflect.MakeMap(target)
		if target.Key().Kind() == reflect.String {
			sanitized.SetMapIndex(reflect.ValueOf("value").Convert(target.Key()), redactedReflectValue(target.Elem()))
		}
		return sanitized
	default:
		return reflect.Zero(target)
	}
}

func isSensitiveKey(key string) bool {
	normalized := normalizeKey(key)
	if normalized == "" {
		return false
	}
	if _, ok := safeKeyNames[normalized]; ok {
		return false
	}
	if _, ok := sensitiveKeyNames[normalized]; ok {
		return true
	}

	compact := strings.ReplaceAll(normalized, "_", "")
	if _, ok := sensitiveKeyNames[compact]; ok {
		return true
	}

	for _, part := range strings.Split(normalized, "_") {
		if _, ok := sensitiveKeyNames[part]; ok {
			return true
		}
	}

	return false
}

func normalizeKey(key string) string {
	var builder strings.Builder
	lastSeparator := false
	for _, char := range strings.ToLower(strings.TrimSpace(key)) {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') {
			builder.WriteRune(char)
			lastSeparator = false
			continue
		}
		if !lastSeparator {
			builder.WriteByte('_')
			lastSeparator = true
		}
	}

	return strings.Trim(builder.String(), "_")
}

func sanitizeStringValue(value string) string {
	if isURLLikeString(value) {
		value = sanitizeURL(value)
	}
	if isSensitiveString(value) {
		return redactedValue
	}

	return value
}

func isURLLikeString(value string) bool {
	return strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || strings.Contains(value, "://")
}

func isSensitiveString(value string) bool {
	return sensitiveValuePattern.MatchString(value)
}
