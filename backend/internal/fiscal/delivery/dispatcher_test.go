package delivery

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/fiscal"
)

func TestDeferIfBudgetExceeded(t *testing.T) {
	require.NoError(t, deferIfBudgetExceeded(nil))

	refused := fmt.Errorf("%w: business_daily_standard", emails.ErrTenantMailBudgetExceeded)
	err := deferIfBudgetExceeded(refused)
	var deferred *fiscal.DeliveryDeferredError
	require.True(t, errors.As(err, &deferred), "a budget refusal is deferred, not retried on the backoff schedule")
	require.Equal(t, fiscal.DefaultDeliveryDeferral, deferred.After)
	require.True(t, errors.Is(err, emails.ErrTenantMailBudgetExceeded))

	other := errors.New("provider down")
	require.Same(t, other, deferIfBudgetExceeded(other), "other failures keep the normal retry path")
}
