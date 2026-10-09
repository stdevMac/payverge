package services

import "embed"

// Persona prompt assets for the Ops Assistant.
// Embedded here (services package) because go:embed cannot reference paths
// outside the declaring package; agents/prompts.go reads this FS.
//
//go:embed prompts/ops_assistant/*.md
//go:embed prompts/ops_assistant/playbooks/*.md
var PersonaPromptFS embed.FS
