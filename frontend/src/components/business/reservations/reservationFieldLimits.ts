// Mirrors backend/internal/services/reservation_field_limits.go. The server
// counts runes; maxLength counts UTF-16 units, so the browser cap is never
// looser than the API's (an emoji uses two units here, one rune there).
export const RESERVATION_CUSTOMER_NAME_MAX_LENGTH = 80;
export const RESERVATION_CUSTOMER_PHONE_MAX_LENGTH = 32;
export const RESERVATION_SPECIAL_REQUESTS_MAX_LENGTH = 300;
