package fiscal

import (
	"context"
	"strings"
	"sync"
)

type Provider interface {
	Country() string
	Name() string
	ValidateSettings(ctx context.Context, settings Settings) error
	IssueReceipt(ctx context.Context, input IssueInput) (*ReceiptResult, error)
	IssueCreditNote(ctx context.Context, input CreditNoteInput) (*ReceiptResult, error)
	GetStatus(ctx context.Context, providerReceiptID string) (*ReceiptStatus, error)
}

type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{providers: make(map[string]Provider)}
}

func (r *ProviderRegistry) Register(provider Provider) {
	if provider == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.providers == nil {
		r.providers = make(map[string]Provider)
	}
	r.providers[providerKey(provider.Country(), provider.Name())] = provider
}

func (r *ProviderRegistry) Get(country, name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, ok := r.providers[providerKey(country, name)]
	return provider, ok
}

func providerKey(country, name string) string {
	return strings.ToUpper(strings.TrimSpace(country)) + ":" + strings.ToLower(strings.TrimSpace(name))
}
