package paymentcontract

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
)

var sourceSHAPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

type CaseProof struct {
	ID     string `json:"id"`
	Result string `json:"result"`
	Test   string `json:"test"`
}

type HermeticContractProof struct {
	Result        string      `json:"result"`
	Checks        []string    `json:"checks"`
	MissingChecks []string    `json:"missing_checks"`
	Cases         []CaseProof `json:"cases"`
}

type ProviderProof struct {
	Provider         string                `json:"provider"`
	HermeticContract HermeticContractProof `json:"hermetic_contract"`
}

type HermeticProof struct {
	SchemaVersion  int             `json:"schema_version"`
	SourceSHA      string          `json:"source_sha"`
	GeneratedAt    string          `json:"generated_at"`
	Result         string          `json:"result"`
	Evidence       string          `json:"evidence_boundary"`
	ExecutedChecks []string        `json:"executed_checks"`
	MissingChecks  []string        `json:"missing_checks"`
	Providers      []ProviderProof `json:"providers"`
}

type goTestEvent struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
}

func BuildHermeticProof(input io.Reader, sourceSHA string, generatedAt time.Time) (*HermeticProof, error) {
	sourceSHA = strings.ToLower(strings.TrimSpace(sourceSHA))
	if !sourceSHAPattern.MatchString(sourceSHA) {
		return nil, errors.New("source SHA must be a full 40-character lowercase Git SHA")
	}
	if generatedAt.IsZero() {
		return nil, errors.New("generated time is required")
	}

	actions := make(map[string]string)
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	prefix := "TestProductionPaymentProviderContract/"
	for scanner.Scan() {
		var event goTestEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("decode go test event: %w", err)
		}
		if !strings.HasPrefix(event.Test, prefix) || event.Action == "run" || event.Action == "output" {
			continue
		}
		if event.Package != "github.com/stdevmac/payverge/backend/internal/handlers" {
			return nil, fmt.Errorf("contract event %s came from unexpected package %s", event.Test, event.Package)
		}
		parts := strings.Split(strings.TrimPrefix(event.Test, prefix), "/")
		if len(parts) < 3 {
			continue
		}
		if previous, exists := actions[event.Test]; exists && previous != event.Action {
			return nil, fmt.Errorf("contract case %s has conflicting actions %s and %s", event.Test, previous, event.Action)
		}
		actions[event.Test] = event.Action
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read go test events: %w", err)
	}

	missingChecks := make([]string, 0, len(RequiredChecks)-len(ExecutedChecks))
	for _, check := range RequiredChecks {
		if !contains(ExecutedChecks, check) {
			missingChecks = append(missingChecks, check)
		}
	}
	proof := &HermeticProof{
		SchemaVersion:  2,
		SourceSHA:      sourceSHA,
		GeneratedAt:    generatedAt.UTC().Format(time.RFC3339),
		Result:         "partial",
		Evidence:       "executable-hermetic-contract",
		ExecutedChecks: append([]string(nil), ExecutedChecks...),
		MissingChecks:  missingChecks,
	}
	if len(missingChecks) == 0 {
		proof.Result = "pass"
	}

	for _, provider := range ProductionProviders {
		providerProof := ProviderProof{
			Provider: provider,
			HermeticContract: HermeticContractProof{
				Result:        "partial",
				Checks:        append([]string(nil), ExecutedChecks...),
				MissingChecks: append([]string{}, missingChecks...),
			},
		}
		if len(missingChecks) == 0 {
			providerProof.HermeticContract.Result = "pass"
		}
		for _, testCase := range ExecutableCases {
			testName := fmt.Sprintf("%s%s/%s", prefix, provider, testCase.ID)
			action, exists := actions[testName]
			if !exists {
				return nil, fmt.Errorf("missing passing contract case %s", testName)
			}
			if action != "pass" {
				return nil, fmt.Errorf("contract case %s did not pass: %s", testName, action)
			}
			providerProof.HermeticContract.Cases = append(providerProof.HermeticContract.Cases, CaseProof{
				ID:     testCase.ID,
				Result: "pass",
				Test:   testName,
			})
		}
		proof.Providers = append(proof.Providers, providerProof)
	}

	for testName := range actions {
		parts := strings.SplitN(strings.TrimPrefix(testName, prefix), "/", 2)
		if len(parts) != 2 || !contains(ProductionProviders, parts[0]) || !containsExecutableCase(parts[1]) {
			return nil, fmt.Errorf("unknown executable contract case %s", testName)
		}
	}
	sort.Slice(proof.Providers, func(i, j int) bool { return proof.Providers[i].Provider < proof.Providers[j].Provider })
	return proof, nil
}

func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func containsExecutableCase(candidate string) bool {
	for _, testCase := range ExecutableCases {
		if testCase.ID == candidate {
			return true
		}
	}
	return false
}
