package llm

import (
	"fmt"
	"strings"
)

// FeaturePrivacyPolicy is the mandatory privacy posture for a Feature string.
type FeaturePrivacyPolicy struct {
	Class      PrivacyClass
	RequireZDR bool
}

// productionFeaturePrivacy is the exhaustive map of production Feature values.
// Waiter, WhatsApp, Ops, Director, guardrails, extraction, wizard,
// image, and marketing all require ZDR. Only hermetic eval/test features may
// use PrivacyPublic.
var productionFeaturePrivacy = map[string]FeaturePrivacyPolicy{
	"waiter":          {Class: PrivacyCustomerSensitive, RequireZDR: true},
	"waiter_whatsapp": {Class: PrivacyCustomerSensitive, RequireZDR: true},
	"ops_assistant":   {Class: PrivacyBusinessConfidential, RequireZDR: true},
	"director":        {Class: PrivacyBusinessConfidential, RequireZDR: true},
	"guardrail":       {Class: PrivacyCustomerSensitive, RequireZDR: true},
	"wizard":          {Class: PrivacyBusinessConfidential, RequireZDR: true},
	"extraction":      {Class: PrivacyBusinessConfidential, RequireZDR: true},
	"image":           {Class: PrivacyBusinessConfidential, RequireZDR: true},
	"marketing":       {Class: PrivacyBusinessConfidential, RequireZDR: true},
	// Hermetic eval / offline fixtures may use public class without ZDR.
	"eval": {Class: PrivacyPublic, RequireZDR: false},
	"test": {Class: PrivacyPublic, RequireZDR: false},
}

// ErrPrivacyPolicy is returned when a request fails privacy classification or
// ZDR policy validation before any provider spend.
var ErrPrivacyPolicy = fmt.Errorf("ai privacy policy violation")

// PrivacyPolicyForFeature returns the policy for a feature tag.
func PrivacyPolicyForFeature(feature string) (FeaturePrivacyPolicy, bool) {
	p, ok := productionFeaturePrivacy[strings.TrimSpace(feature)]
	return p, ok
}

// NewGenerateRequest builds a request with the mandatory privacy class derived
// from the feature policy. Unknown features fail closed.
func NewGenerateRequest(feature string) (GenerateRequest, error) {
	policy, ok := PrivacyPolicyForFeature(feature)
	if !ok {
		return GenerateRequest{}, fmt.Errorf("%w: unknown AI feature %q", ErrPrivacyPolicy, feature)
	}
	return GenerateRequest{
		Feature:      strings.TrimSpace(feature),
		PrivacyClass: policy.Class,
	}, nil
}

// ApplyFeaturePrivacy sets PrivacyClass from the feature policy. Callers that
// omit a class get the policy class; a weaker class than policy is rejected.
func ApplyFeaturePrivacy(req *GenerateRequest) error {
	if req == nil {
		return fmt.Errorf("%w: nil generate request", ErrPrivacyPolicy)
	}
	policy, ok := PrivacyPolicyForFeature(req.Feature)
	if !ok {
		return fmt.Errorf("%w: unknown AI feature %q", ErrPrivacyPolicy, req.Feature)
	}
	if req.PrivacyClass == "" {
		req.PrivacyClass = policy.Class
		return nil
	}
	if privacyWeaker(req.PrivacyClass, policy.Class) {
		return fmt.Errorf("%w: privacy class %q weaker than policy %q for feature %q", ErrPrivacyPolicy, req.PrivacyClass, policy.Class, req.Feature)
	}
	return nil
}

// ValidatePrivacy rejects empty/unknown privacy classes before network spend.
func (r GenerateRequest) ValidatePrivacy() error {
	switch r.PrivacyClass {
	case PrivacyPublic, PrivacyCustomerSensitive, PrivacyBusinessConfidential:
		return nil
	case "":
		return fmt.Errorf("%w: privacy class is required", ErrPrivacyPolicy)
	default:
		return fmt.Errorf("%w: unknown privacy class %q", ErrPrivacyPolicy, r.PrivacyClass)
	}
}

// RequiresZDR reports whether the request must force zero-data-retention routing.
func (r GenerateRequest) RequiresZDR() bool {
	policy, ok := PrivacyPolicyForFeature(r.Feature)
	if ok {
		return policy.RequireZDR
	}
	// Unknown features fail closed for ZDR when class is sensitive.
	return r.PrivacyClass == PrivacyCustomerSensitive || r.PrivacyClass == PrivacyBusinessConfidential
}

func privacyWeaker(got, required PrivacyClass) bool {
	rank := map[PrivacyClass]int{
		PrivacyPublic:               1,
		PrivacyCustomerSensitive:    2,
		PrivacyBusinessConfidential: 3,
	}
	return rank[got] < rank[required]
}

// KnownProductionFeatures returns feature keys for coverage tests.
func KnownProductionFeatures() []string {
	out := make([]string, 0, len(productionFeaturePrivacy))
	for k := range productionFeaturePrivacy {
		out = append(out, k)
	}
	return out
}
