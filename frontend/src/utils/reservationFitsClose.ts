/**
 * L1-17: a reservation start must finish (duration + service buffer) before
 * the operating-window close. Shared by create-form validation so the operator
 * time picker cannot silently offer a slot the backend will reject.
 *
 * Overnight venues (e.g. 18:00–02:00) roll close forward by 24h, matching
 * backend reservation_service.go when !closeTime.After(openTime).
 */

export function reservationFitsClose(
  startWallHHMM: string,
  closeHHMM: string,
  serviceMinutes: number,
  openHHMM?: string,
): boolean {
  let start = parseHHMMToMinutes(startWallHHMM);
  let close = parseHHMMToMinutes(closeHHMM);
  if (start === null || close === null) return false;
  const svc = Math.max(0, Math.floor(serviceMinutes));

  // Overnight close: when open is provided and close is not after open on the
  // same clock, treat close as next-day (backend rolls closeTime.Add(24h)).
  // Starts after midnight (before open) also roll forward so 01:00+90 vs 02:00
  // is compared in the same extended day window.
  if (openHHMM) {
    const open = parseHHMMToMinutes(openHHMM);
    if (open !== null && close <= open) {
      close += 24 * 60;
      if (start < open) {
        start += 24 * 60;
      }
    }
  } else if (close <= start && close < 12 * 60) {
    // No open supplied: late start with early-morning close → overnight.
    close += 24 * 60;
  } else if (close < start && start >= 18 * 60 && close < 12 * 60) {
    close += 24 * 60;
  }

  // End must not be after close (same rule as backend reservationFitsOperatingWindow).
  return start + svc <= close;
}

function parseHHMMToMinutes(value: string): number | null {
  const m = /^(\d{1,2}):(\d{2})$/.exec((value || "").trim());
  if (!m) return null;
  const h = Number(m[1]);
  const min = Number(m[2]);
  if (
    !Number.isFinite(h) ||
    !Number.isFinite(min) ||
    h < 0 ||
    h > 23 ||
    min < 0 ||
    min > 59
  ) {
    return null;
  }
  return h * 60 + min;
}
