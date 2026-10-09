package services

// InHouseDeliveryIsReachable reports whether any active delivery zone can match
// a guest address, i.e. whether in-house checkout is actually reachable.
//
// This is the exported face of the coverage probe the public storefront gate
// already applies (issue #714): "entrega activa" is suppressed when in-house
// delivery is switched on but no active zone has usable boundaries, because the
// quote/create path cannot place such an order. The guest waiter lives in
// package server and cannot reach the unexported method, and a second copy of
// the predicate would be free to drift from the storefront it must agree with —
// so it delegates here rather than re-deriving coverage.
func (s *DeliveryService) InHouseDeliveryIsReachable(businessID uint) bool {
	if s == nil || s.db == nil {
		return false
	}
	return s.hasMatchableActiveZone(businessID)
}
