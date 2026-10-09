package ar

import "time"

// argentinaLocation is the civil timezone AFIP uses for voucher dates
// (CbteFch and the QR "fecha"). Instants stay UTC everywhere we store them;
// only the calendar date sent to AFIP is shifted. America/Argentina/Buenos_Aires
// is UTC-3 year-round; if zoneinfo is missing, a fixed ART offset matches it.
var argentinaLocation = loadArgentinaLocation()

func loadArgentinaLocation() *time.Location {
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		return time.FixedZone("ART", -3*3600)
	}
	return loc
}
