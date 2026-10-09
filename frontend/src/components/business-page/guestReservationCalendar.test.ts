import {
  buildReservationCalendarInvite,
  escapeIcsText,
} from "./guestReservationCalendar";

describe("guest reservation calendar invite", () => {
  const start = new Date("2026-06-20T19:00:00.000Z");
  const end = new Date("2026-06-20T20:00:00.000Z");

  it("localizes Google + ICS payloads from the provided title/details", () => {
    const invite = buildReservationCalendarInvite({
      title: "Reserva en Test Bistro",
      details: "Reserva para 2 invitados en Test Bistro.",
      start,
      end,
    });

    expect(invite.googleUrl).toContain("calendar.google.com");
    const googleParams = new URL(invite.googleUrl).searchParams;
    expect(googleParams.get("text")).toBe("Reserva en Test Bistro");
    expect(googleParams.get("details")).toBe(
      "Reserva para 2 invitados en Test Bistro.",
    );
    expect(invite.googleUrl).not.toContain("Reservation+at");
    expect(invite.googleUrl).not.toContain("Reservation%20at");

    expect(invite.ics).toContain("BEGIN:VCALENDAR");
    expect(invite.ics).toContain("SUMMARY:Reserva en Test Bistro");
    expect(invite.ics).toContain(
      "DESCRIPTION:Reserva para 2 invitados en Test Bistro.",
    );
    expect(invite.icsDataUri.startsWith("data:text/calendar")).toBe(true);
    expect(invite.filename).toBe("reservation.ics");
  });

  it("escapes ICS reserved characters", () => {
    expect(escapeIcsText("A, B; C\\D\nE")).toBe("A\\, B\\; C\\\\D\\nE");
  });
});
