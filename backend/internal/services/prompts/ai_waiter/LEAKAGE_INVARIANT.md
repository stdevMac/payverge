# AI Waiter Prompt Leakage Invariant

**Rule (OWASP LLM07):** Every rendered system prompt must be free of secrets, API keys, internal hostnames, and credential-bearing tokens. The prompt assembler (`buildWaiterSystemPrompt`) is the single render path; all security-relevant fields are sanitized before interpolation, and every untrusted data block is spotlighted with crypto/rand markers inside `data_block` tags.

**Verification:** `TestWaiterPromptsCarryNoSecrets` in `waiter_leakage_test.go` renders every (mode, locale) combination and asserts the output contains none of the forbidden tokens (API keys, `sk-`, `Bearer`, connection strings, `127.0.0.1`, `://localhost`, `internal.`).

**Last verified:** 2026-06-07 (Lane B initial implementation).
