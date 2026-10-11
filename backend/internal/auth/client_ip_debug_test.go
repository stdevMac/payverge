package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClientIPDebugEnabled(t *testing.T) {
	t.Setenv("CLIENT_IP_DEBUG", "")
	assert.False(t, clientIPDebugEnabled())

	t.Setenv("CLIENT_IP_DEBUG", "1")
	assert.True(t, clientIPDebugEnabled())

	t.Setenv("CLIENT_IP_DEBUG", "true")
	assert.True(t, clientIPDebugEnabled())

	t.Setenv("CLIENT_IP_DEBUG", "0")
	assert.False(t, clientIPDebugEnabled())
}
