package services

import (
	"context"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/llm"

	"github.com/stretchr/testify/require"
)

func TestMenuExtractionWorkerPanicFailsJob(t *testing.T) {
	setupExtractionWorkerDB(t)
	job := seedWorkerJob(t, database.ExtractionStatusPending)

	svc := NewMenuAIService(&countingExtractionProvider{}, llm.ModelConfig{Menu: "m", Image: "i"})
	w := NewMenuExtractionWorker(svc,
		func(images []database.MenuExtractionImage) ([]MenuExtractionInput, error) {
			panic("boom")
		},
		func(images []database.MenuExtractionImage) {},
	)

	require.NotPanics(t, func() {
		w.ProcessOne(context.Background(), job.ID)
	})

	reloaded, err := database.GetExtractionJobByID(job.ID)
	require.NoError(t, err)
	require.Equal(t, database.ExtractionStatusFailed, reloaded.Status)
	require.Equal(t, MenuExtractionErrInternal, reloaded.ErrorMessage)
	require.NotEqual(t, database.ExtractionStatusProcessing, reloaded.Status)
}
