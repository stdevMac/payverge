package demo

import "fmt"

// Demo contact data is deliberately FAKE. A self-hosted showroom must never
// print, dial, map or invoice a real person or business:
//
//   - Phones sit in the +54 11 0000-xxxx block. Buenos Aires subscriber
//     numbers never start with 0, so none of these can ring a real line.
//   - House numbers are in the 9000s, past the last real number of every
//     street the demo uses (Defensa, Costa Rica, Estados Unidos, Carlos
//     Calvo). Barrios and postal codes stay real so the delivery-zone matcher
//     (demoZoneBoundaries) still quotes them.
//   - Websites use the reserved .example TLD (RFC 2606), and the venues carry
//     no social handles, so nothing routes to an account somebody owns.
//   - CUITs and DNIs are documentation placeholders (see demoFiscalCUIT and
//     demoValidCUIT). They carry a valid AFIP mod-11 check digit only because
//     the fiscal receiver rejects a bad one.

// demoFakePhoneBlock is the unassigned Buenos Aires local prefix every demo
// phone uses.
const demoFakePhoneBlock = "0000"

// demoFakeHouseNumberBase is added to every demo street number so it falls
// past the end of the real street.
const demoFakeHouseNumberBase = 9000

// demoNoSocialMedia is the venues' social_media blob: an empty JSON object,
// so the storefront renders no social links and marketing copy no @handle.
const demoNoSocialMedia = `{}`

// demoFakePhoneDisplay renders a non-dialable Buenos Aires landline in the
// printed "+54 11 0000-xxxx" form the venue storefront shows.
func demoFakePhoneDisplay(n int) string {
	return fmt.Sprintf("+54 11 %s-%04d", demoFakePhoneBlock, n%10000)
}

// demoFakeMobile renders the same non-dialable block in the compact E.164
// mobile form ("+54 9 11 0000-xxxx" without spaces) that driver, CRM,
// delivery and reservation rows store.
func demoFakeMobile(n int) string {
	return fmt.Sprintf("+54911%s%04d", demoFakePhoneBlock, n%10000)
}

// demoFakeStreet renders "<street> <9000+n>", a house number past the end of
// any street the demo uses.
func demoFakeStreet(street string, n int) string {
	return fmt.Sprintf("%s %d", street, demoFakeHouseNumberBase+n%1000)
}

// demoFakeWebsite is a per-venue website on the reserved .example TLD.
func demoFakeWebsite(p profile) string {
	if p.Key == "secondary" {
		return "https://parrilla-quebracho-azul.example"
	}
	return "https://bodegon-mesa-larga.example"
}
