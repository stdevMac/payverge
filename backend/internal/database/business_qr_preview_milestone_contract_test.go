package database

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBusinessCarriesDurableQRPreviewMilestone(t *testing.T) {
	field, ok := reflect.TypeOf(Business{}).FieldByName("QRPreviewedAt")
	require.True(t, ok,
		"Business needs a persisted QRPreviewedAt milestone; analytics events or client state are not authoritative")
	require.Equal(t, reflect.TypeOf((*time.Time)(nil)), field.Type,
		"QRPreviewedAt must be a nullable timestamp so pre-preview and recorded-preview states are distinguishable")
}
