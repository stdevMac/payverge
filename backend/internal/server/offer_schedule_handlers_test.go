package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateOfferRequestRejectsInvalidSchedule(t *testing.T) {
	validName := "Lunch"
	base := OfferRequest{Name: validName, DiscountType: "percentage", DiscountValue: 10}
	zero := int16(0)
	validMask := int16(127)
	minute := 600
	tooLate := 1440

	tests := []struct {
		name   string
		mutate func(*OfferRequest)
	}{
		{name: "zero weekday mask", mutate: func(req *OfferRequest) { req.WeekdayMask = &zero }},
		{name: "unpaired start", mutate: func(req *OfferRequest) { req.WeekdayMask = &validMask; req.StartMinute = &minute }},
		{name: "minute outside day", mutate: func(req *OfferRequest) {
			req.WeekdayMask = &validMask
			req.StartMinute = &minute
			req.EndMinute = &tooLate
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := base
			tt.mutate(&req)
			err := validateOfferRequest(req)
			require.Error(t, err)
			assert.ErrorIs(t, err, errInvalidOfferSchedule)
		})
	}
}
