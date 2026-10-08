import { formatReservationHistoryNote } from "./reservationHistoryNote";

describe("formatReservationHistoryNote (L1-21)", () => {
  const statusLabels: Record<string, string> = {
    "status.pending": "Pendiente",
    "status.confirmed": "Confirmada",
    "status.no_show": "No se presentó",
  };
  const t = (key: string, params?: Record<string, string | number>) => {
    if (key === "historyNotes.assignedTo") return `Asignada a ${params?.name}`;
    if (key === "historyNotes.statusChanged")
      return `Estado: ${params?.from} → ${params?.to}`;
    if (key in statusLabels) return statusLabels[key];
    if (key === "historyNotes.autoDeclinedNoResponse")
      return "Rechazada automáticamente: sin respuesta";
    if (key === "historyNotes.cancelledByCustomer")
      return "Cancelada por el cliente";
    if (key === "historyNotes.cancelledByBusiness")
      return "Cancelada por el negocio";
    if (key === "historyNotes.created") return "Reserva creada";
    if (key === "historyNotes.autoNoShowGrace")
      return "No se presentó: venció la tolerancia";
    return key;
  };

  it("maps known tokens to translated strings", () => {
    expect(
      formatReservationHistoryNote("token:auto_declined_no_response", t),
    ).toBe("Rechazada automáticamente: sin respuesta");
    expect(
      formatReservationHistoryNote("token:cancelled_by_customer", t),
    ).toBe("Cancelada por el cliente");
    expect(formatReservationHistoryNote("token:created", t)).toBe(
      "Reserva creada",
    );
  });

  it("passes table name through assigned_to token", () => {
    expect(formatReservationHistoryNote("token:assigned_to:Patio A", t)).toBe(
      "Asignada a Patio A",
    );
  });

  it("maps status_changed tokens with from/to", () => {
    // The status codes themselves must be localized too — interpolating raw
    // English codes produced half-Spanish sentences ("de pending a confirmed").
    expect(
      formatReservationHistoryNote("token:status_changed:pending:confirmed", t),
    ).toBe("Estado: Pendiente → Confirmada");
  });

  it("falls back to the raw status code when it has no label", () => {
    expect(
      formatReservationHistoryNote("token:status_changed:pending:zzz_new", t),
    ).toBe("Estado: Pendiente → zzz_new");
  });

  it("detects a key echo from the namespaced production translator", () => {
    // ReservationManager's `t` prefixes every key with
    // `businessDashboard.reservations.` and getTranslation returns that FULL
    // dotted key on a miss — so a bare `status.<code>` comparison would let the
    // key path leak into the UI.
    const namespaced = (
      key: string,
      params?: Record<string, string | number>,
    ) => {
      const full = `businessDashboard.reservations.${key}`;
      if (key === "historyNotes.statusChanged")
        return `Estado: ${params?.from} → ${params?.to}`;
      if (key in statusLabels) return statusLabels[key];
      return full;
    };
    expect(
      formatReservationHistoryNote(
        "token:status_changed:pending:zzz_new",
        namespaced,
      ),
    ).toBe("Estado: Pendiente → zzz_new");
  });

  it("renders untokened notes verbatim (no pre-token English mapping)", () => {
    expect(formatReservationHistoryNote("Cancelled by customer", t)).toBe(
      "Cancelled by customer",
    );
  });

  it("localizes cancelled_by_business token", () => {
    expect(formatReservationHistoryNote("token:cancelled_by_business", t)).toBe(
      "Cancelada por el negocio",
    );
  });

  it("localizes the no-show sweeper token", () => {
    expect(formatReservationHistoryNote("token:auto_no_show_grace", t)).toBe(
      "No se presentó: venció la tolerancia",
    );
  });

  it("falls back to raw text for free-form operator notes", () => {
    expect(
      formatReservationHistoryNote("Guest called to confirm vegan menu", t),
    ).toBe("Guest called to confirm vegan menu");
  });
});
