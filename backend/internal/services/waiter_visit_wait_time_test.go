package services

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWaiterWaitTimeAnswer_EsIsTuteoNotVoseo(t *testing.T) {
	es := WaiterWaitTimeAnswer("es")
	require.NotContains(t, es, "usá")
	require.NotContains(t, es, "mozo")
	require.Contains(t, es, "usa ")
	require.Contains(t, es, "camarero")

	ar := WaiterWaitTimeAnswer("es-AR")
	require.Contains(t, ar, "usá")
	require.Contains(t, ar, "mozo")
	require.NotEqual(t, es, ar)
}
