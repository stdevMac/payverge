/**
 * Filed #662 stamp: EN/es painted 22:12 (UTC) while es-AR showed the
 * America/New_York clock (06:17 p. m. on a ticket a few minutes later).
 * 22:12Z is 18:12 / 6:12 p.m. in that venue.
 */
export const FILED_UTC_FIRE = "2026-08-19T22:12:00.000Z";
export const FILED_NAIVE_FIRE = "2026-08-19T22:12:00";
export const FILED_NOW = "2026-08-19T22:17:00.000Z";

export type OperatorLocale = "en" | "es" | "es-AR";

export function compactClock(value: string): string {
  return value.replace(/[\s\u00a0\u202f]+/g, " ").trim();
}

export function expectVenueNyFireClock(
  raw: string,
  locale: OperatorLocale,
): void {
  const clock = compactClock(raw);
  expect(clock).not.toMatch(/22:12/);
  expect(clock).not.toMatch(/10:12/);
  switch (locale) {
    case "en":
      expect(clock).toMatch(/^6:12 PM$/i);
      return;
    case "es":
      expect(clock).toMatch(/^18:12$/);
      return;
    case "es-AR":
      expect(clock).toMatch(/^6:12 p\. m\.$/i);
      return;
    default: {
      const _never: never = locale;
      throw new Error(`unexpected locale: ${_never}`);
    }
  }
}
