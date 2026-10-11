package fiscal

import (
	"context"

	"github.com/stdevmac/payverge/backend/internal/runtimecontrol"

	"gorm.io/gorm"
)

func automaticFiscalEnabled(ctx context.Context, db *gorm.DB) (bool, error) {
	if db == nil {
		return false, gorm.ErrInvalidDB
	}
	return runtimecontrol.New(db).Enabled(ctx, runtimecontrol.ControlFiscal)
}
