function formatCalendarDates(date: Date): string {
  return date
    .toISOString()
    .replace(/[-:]/g, "")
    .replace(/\.\d{3}Z$/, "Z");
}

export function escapeIcsText(value: string): string {
  return value
    .replace(/\\/g, "\\\\")
    .replace(/\n/g, "\\n")
    .replace(/,/g, "\\,")
    .replace(/;/g, "\\;");
}

export function buildReservationCalendarInvite({
  title,
  details,
  start,
  end,
}: {
  title: string;
  details: string;
  start: Date;
  end: Date;
}): {
  googleUrl: string;
  ics: string;
  icsDataUri: string;
  filename: string;
} {
  const dates = `${formatCalendarDates(start)}/${formatCalendarDates(end)}`;
  const url = new URL("https://calendar.google.com/calendar/render");
  url.searchParams.set("action", "TEMPLATE");
  url.searchParams.set("text", title);
  url.searchParams.set("dates", dates);
  url.searchParams.set("details", details);

  const ics = [
    "BEGIN:VCALENDAR",
    "VERSION:2.0",
    "PRODID:-//Payverge//Guest Reservation//EN",
    "CALSCALE:GREGORIAN",
    "METHOD:PUBLISH",
    "BEGIN:VEVENT",
    `DTSTART:${formatCalendarDates(start)}`,
    `DTEND:${formatCalendarDates(end)}`,
    `SUMMARY:${escapeIcsText(title)}`,
    `DESCRIPTION:${escapeIcsText(details)}`,
    "END:VEVENT",
    "END:VCALENDAR",
  ].join("\r\n");

  return {
    googleUrl: url.toString(),
    ics,
    icsDataUri: `data:text/calendar;charset=utf-8,${encodeURIComponent(ics)}`,
    filename: "reservation.ics",
  };
}

export function downloadReservationIcs(
  icsDataUri: string,
  filename: string,
): void {
  const link = document.createElement("a");
  link.href = icsDataUri;
  link.download = filename;
  link.rel = "noopener";
  document.body.appendChild(link);
  link.click();
  link.remove();
}
