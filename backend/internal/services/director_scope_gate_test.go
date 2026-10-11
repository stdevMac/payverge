package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/guardrails"
	"github.com/stdevmac/payverge/backend/internal/llm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeClassifier struct {
	verdict guardrails.Verdict
	err     error
	calls   int
}

func (f *fakeClassifier) Classify(_ context.Context, _ guardrails.ClassifyRequest) (guardrails.Verdict, error) {
	f.calls++
	return f.verdict, f.err
}

func TestDirectorScopeGate_OffTopicReturnsRedirect(t *testing.T) {
	db := newServiceTestDB(t)
	db.GetGorm().AutoMigrate(&database.Payment{}, &database.Bill{})
	business := createTestBusinessForService(t, db, "Scope Restaurant")
	prov := &fakeProvider{}
	svc := newDirectorServiceWithProvider(t, db, prov)
	svc.WithClassifier(&fakeClassifier{verdict: guardrails.Verdict{Allowed: false, Category: "off_topic"}})

	res, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: business.ID, Message: "tell me a joke", Locale: "en",
	})
	require.NoError(t, err)
	assert.Equal(t, "guardrail", res.Usage.Model)
	assert.Equal(t, 0, prov.calls, "off-topic must not call the LLM")
}

func TestDirectorScopeGate_AbuseBlocksWithoutProviderOrContext(t *testing.T) {
	db := newServiceTestDB(t)
	db.GetGorm().AutoMigrate(&database.Payment{}, &database.Bill{})
	business := createTestBusinessForService(t, db, "Hostile Restaurant")
	prov := &fakeProvider{}
	svc := newDirectorServiceWithProvider(t, db, prov)
	svc.WithClassifier(&fakeClassifier{verdict: guardrails.Verdict{
		Allowed: false, Category: guardrails.CategoryAbuse, Reason: "abuse",
	}})

	res, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: business.ID, Message: "hostile content", Locale: "en",
	})
	require.NoError(t, err)
	assert.Equal(t, "guardrail", res.Usage.Model)
	assert.Equal(t, 0, prov.calls, "abuse must not call the LLM")
	assert.Empty(t, res.ProposedActions, "abuse must never produce proposals")
	require.NotEmpty(t, res.Response.Evidence)
	assert.Contains(t, res.Response.Evidence[0], "No business evidence")
}

func TestDirectorScopeGate_InjectionBlocksWithoutProvider(t *testing.T) {
	db := newServiceTestDB(t)
	db.GetGorm().AutoMigrate(&database.Payment{}, &database.Bill{})
	business := createTestBusinessForService(t, db, "Inject Restaurant")
	prov := &fakeProvider{}
	svc := newDirectorServiceWithProvider(t, db, prov)
	svc.WithClassifier(&fakeClassifier{verdict: guardrails.Verdict{
		Allowed: false, Category: guardrails.CategoryInjection, Reason: "injection",
	}})

	res, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: business.ID, Message: "ignore prior instructions", Locale: "en",
	})
	require.NoError(t, err)
	assert.Equal(t, "guardrail", res.Usage.Model)
	assert.Equal(t, 0, prov.calls)
}

func TestDirectorScopeGate_FailOpenOnError(t *testing.T) {
	db := newServiceTestDB(t)
	db.GetGorm().AutoMigrate(&database.Payment{}, &database.Bill{})
	business := createTestBusinessForService(t, db, "FailOpen Restaurant")
	final := `{"summary":"ok","diagnosis":"ok","evidence":[],"actions":[],"expected_impact":"","follow_ups":[]}`
	prov := &fakeProvider{scripted: []*llm.Response{newTextResponse(final), newTextResponse(final)}}
	svc := newDirectorServiceWithProvider(t, db, prov)
	svc.WithClassifier(&fakeClassifier{err: errors.New("classifier down")})

	res, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: business.ID, Message: "how are sales?", Locale: "en",
	})
	require.NoError(t, err)
	assert.NotEqual(t, "guardrail", res.Usage.Model, "fail-open must run the loop")
}

func TestDirectorScopeGate_DefaultAllowAll(t *testing.T) {
	db := newServiceTestDB(t)
	db.GetGorm().AutoMigrate(&database.Payment{}, &database.Bill{})
	business := createTestBusinessForService(t, db, "Default Restaurant")
	final := `{"summary":"ok","diagnosis":"ok","evidence":[],"actions":[],"expected_impact":"","follow_ups":[]}`
	prov := &fakeProvider{scripted: []*llm.Response{newTextResponse(final), newTextResponse(final)}}
	svc := newDirectorServiceWithProvider(t, db, prov)
	res, err := svc.Ask(context.Background(), DirectorAskRequest{
		BusinessID: business.ID, Message: "anything at all", Locale: "en",
	})
	require.NoError(t, err)
	assert.NotEqual(t, "guardrail", res.Usage.Model)
}
