package loyalty

import "errors"

// Sentinel errors for redeem/undo. Handlers map these to stable wire codes so
// guest toasts can localize them instead of falling through to the English
// string or VALIDATION_INVALID_INPUT ("Please check the form").
var (
	ErrPointsMustBePositive   = errors.New("points must be positive")
	ErrProgramNotEnabled      = errors.New("loyalty program is not enabled for this business")
	ErrRateNotConfigured      = errors.New("loyalty redemption rate is not configured for this business")
	ErrPointsTooSmall         = errors.New("point amount too small to produce a discount")
	ErrDiscountAlreadyApplied = errors.New("a loyalty discount is already applied to this bill; undo it before redeeming again")
	ErrNotConnected           = errors.New("customer is not connected to this business")
	ErrInsufficientPoints     = errors.New("insufficient loyalty points")
	ErrBillNotOpen            = errors.New("bill not found or is no longer open")
	ErrBillFullyCovered       = errors.New("bill is already fully covered by payments; nothing to discount")
	ErrNoDiscountToUndo       = errors.New("no loyalty discount to undo on this bill")
	ErrUndoNotOwner           = errors.New("only the guest who applied this loyalty discount can undo it")
	ErrCannotDetermineRefund  = errors.New("cannot determine points to refund for this redemption")
)
