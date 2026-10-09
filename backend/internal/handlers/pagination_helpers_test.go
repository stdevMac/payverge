package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClampLimit_UsesDefaultWhenMissing(t *testing.T) {
	assert.Equal(t, 50, ClampLimit("", 50, 200))
}

func TestClampLimit_UsesDefaultWhenUnparseable(t *testing.T) {
	assert.Equal(t, 50, ClampLimit("not-a-number", 50, 200))
}

func TestClampLimit_UsesDefaultWhenNonPositive(t *testing.T) {
	assert.Equal(t, 50, ClampLimit("0", 50, 200))
	assert.Equal(t, 50, ClampLimit("-1", 50, 200))
}

func TestClampLimit_CapsAtMax(t *testing.T) {
	assert.Equal(t, 200, ClampLimit("10000000", 50, 200))
	assert.Equal(t, 500, ClampLimit("10000000", 100, 500))
}

func TestClampLimit_PassesThroughValidValue(t *testing.T) {
	assert.Equal(t, 75, ClampLimit("75", 50, 200))
}

func TestClampLimit_AllowsExactMax(t *testing.T) {
	assert.Equal(t, 200, ClampLimit("200", 50, 200))
}
