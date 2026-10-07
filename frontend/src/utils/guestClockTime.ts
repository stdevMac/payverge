/** HH:MM (24h) clock strings rendered in the diner's locale convention. */

function canonicalLocaleTag(locale?: string): string {
  return (locale || "").trim().toLowerCase().replace(/_/g, "-");
}

export function guestHourCycle(locale?: string): "h12" | "h23" {
  const tag = canonicalLocaleTag(locale);
  // Argentine Spanish: hospitality is 24-hour. ICU CLDR still resolves es-AR
  // to h12 ("11:00 p. m."), which is wrong on a Buenos Aires table.
  // Only real guest locale codes reach here — the resolver folds generic
  // LatAm browser tags ("es-419") into es-AR before render — so this list
  // stays to the tags the provider can actually emit.
  if (tag === "es-ar" || tag.startsWith("es-ar-")) {
    return "h23";
  }
  try {
    const resolved = new Intl.DateTimeFormat(locale || undefined, {
      hour: "numeric",
    }).resolvedOptions();
    if (resolved.hourCycle === "h23" || resolved.hourCycle === "h24") {
      return "h23";
    }
    if (resolved.hour12 === false) return "h23";
    return "h12";
  } catch {
    return "h12";
  }
}

export function formatGuestClockTime(time: string, locale?: string): string {
  const [hStr, mStr] = (time || "").split(":");
  const h = Number(hStr);
  const m = Number(mStr);
  if (Number.isNaN(h) || Number.isNaN(m)) {
    return time;
  }
  try {
    const d = new Date();
    d.setHours(h, m, 0, 0);
    return new Intl.DateTimeFormat(locale || undefined, {
      hour: "numeric",
      minute: "2-digit",
      hourCycle: guestHourCycle(locale),
    }).format(d);
  } catch {
    return time;
  }
}
