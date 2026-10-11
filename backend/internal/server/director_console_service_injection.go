package server

import (
	"context"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

// DirectorConsoleServiceAPI is the slice of *services.DirectorConsoleService
// the HTTP handlers depend on. Defining it here lets the SSE handler test
// inject a lightweight stub without standing up a full Gemini-backed service.
// The real *services.DirectorConsoleService satisfies this interface, so
// production wiring keeps using the concrete type.
type DirectorConsoleServiceAPI interface {
	Ask(ctx context.Context, req services.DirectorAskRequest) (*services.DirectorAskResult, error)
	AskStreaming(ctx context.Context, req services.DirectorAskRequest, sink services.Sink) (*services.DirectorAskResult, error)
	ListThreads(businessID uint) ([]services.DirectorThreadDTO, error)
	ListThreadsPaged(businessID uint, opts services.ListThreadsOptions) (services.ListThreadsResult, error)
	ListThreadMessages(businessID uint, threadID uint, limit int) ([]services.DirectorMessageDTO, error)
	SubmitFeedback(businessID uint, messageID uint, vote database.DirectorFeedbackVote) (*services.DirectorMessageDTO, error)
}

var directorConsoleService DirectorConsoleServiceAPI

// SetDirectorConsoleService sets the director console service for the server package.
func SetDirectorConsoleService(service DirectorConsoleServiceAPI) {
	directorConsoleService = service
}

// GetDirectorConsoleService returns the director console service.
func GetDirectorConsoleService() DirectorConsoleServiceAPI {
	return directorConsoleService
}
