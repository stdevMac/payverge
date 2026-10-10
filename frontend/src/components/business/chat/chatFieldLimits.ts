// Mirrors ChatContentMaxRunes and AnnouncementTitleMaxRunes in
// backend/internal/database/chat_messages_service.go and
// chat_announcement_service.go. The server counts runes; maxLength counts
// UTF-16 units, so the browser cap is never looser than the API's (an emoji
// uses two units here, one rune there).
export const CHAT_CONTENT_MAX_LENGTH = 4000;
export const ANNOUNCEMENT_TITLE_MAX_LENGTH = 200;
