package services

// sanitizeField is the single entry point all Lane D builders use.
// It delegates to Lane B's SanitizePromptField.
func sanitizeField(s string, max int) string {
	return SanitizePromptField(s, max)
}
