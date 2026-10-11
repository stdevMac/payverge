// Package llmeval is a hermetic, dependency-light eval harness for Payverge's
// LLM features. Cases live as YAML/JSON files (see Case); a Runner executes
// them against any llm.Provider; a Grader evaluates deterministic assertions
// (regex, JSON schema/field, language-id, contains) plus an optional
// LLM-judge assertion. Offline tests use FixtureProvider (recorded responses
// in testdata) so `go test ./internal/llmeval/...` runs with zero network.
// The cmd/llmeval binary runs a named suite against the live provider.
package llmeval
