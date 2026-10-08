/**
 * Guest reservation public paths. Keep language on every post-booking link so
 * confirmation/cancel pages open in the booking locale (i18n Batch B).
 */
export function guestReservationDetailsPath(
  confirmationCode: string,
  language: string = "en",
): string {
  const code = encodeURIComponent(String(confirmationCode || "").trim());
  const lang = encodeURIComponent(language || "en");
  return `/reservations/${code}?lang=${lang}`;
}

export function guestReservationCancelPath(
  confirmationCode: string,
  language: string = "en",
): string {
  const code = encodeURIComponent(String(confirmationCode || "").trim());
  const lang = encodeURIComponent(language || "en");
  return `/reservations/${code}/cancel?lang=${lang}`;
}
