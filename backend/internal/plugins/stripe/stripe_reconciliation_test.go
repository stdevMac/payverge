package stripe

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStripeCheckoutReconciliationStatus(t *testing.T) {
	tests := []struct {
		name     string
		response map[string]interface{}
		want     string
		wantErr  bool
	}{
		{name: "paid", response: map[string]interface{}{"payment_status": "paid", "status": "complete"}, want: "completed"},
		{name: "no payment required", response: map[string]interface{}{"payment_status": "no_payment_required"}, want: "completed"},
		{name: "expired", response: map[string]interface{}{"payment_status": "unpaid", "status": "expired"}, want: "expired"},
		{name: "delayed pending", response: map[string]interface{}{"payment_status": "unpaid", "status": "complete"}, want: "pending"},
		{name: "unknown fails closed", response: map[string]interface{}{"status": "mystery"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := stripeCheckoutReconciliationStatus(tt.response)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}
