package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// P3: city is optional at registration and nothing in guest ordering consumes
// it — it must not gate the "required" profile step (operators were shown an
// incomplete checklist over a field the product never uses).
func TestProfileDoneDoesNotRequireCity(t *testing.T) {
	status := ComputeSetupStatusFromInput(SetupStatusInput{
		BusinessName:    "No City Bistro",
		AddressCity:     "", // ← intentionally empty
		DefaultCurrency: "USD",
		TableCount:      1,
		MenuCategories:  1,
		MenuItems:       3,
	})

	assert.True(t, status.ProfileDone, "name + currency must complete the profile step without a city")
	assert.False(t, status.HasAddress, "has_address stays an informational display signal")
	assert.True(t, status.RequiredDone)
}
