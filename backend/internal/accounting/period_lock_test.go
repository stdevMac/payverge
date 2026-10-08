package accounting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIsDateLocked(t *testing.T) {
	through := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	require.False(t, IsDateLocked(nil, through))
	require.True(t, IsDateLocked(&through, time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)))
	require.True(t, IsDateLocked(&through, time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)))
	require.False(t, IsDateLocked(&through, time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)))
}
