import {
  localizeAlert,
  localizeClaimConflict,
} from "../operationalAlertCopy";
import type { OperationalAlert } from "@/api/operationalAlerts";

const base = (
  overrides: Partial<
    Pick<OperationalAlert, "alert_type" | "title" | "body" | "metadata">
  >,
): Pick<OperationalAlert, "alert_type" | "title" | "body" | "metadata"> => ({
  alert_type: "service_call",
  title: "Backend title",
  body: "Backend body",
  metadata: null,
  ...overrides,
});

describe("operationalAlertCopy — service_call table name", () => {
  // PV-LIVE-20260720-009: reason must appear so staff know what guests want.
  it("interpolates table_name and reason into the en title and body", () => {
    const copy = localizeAlert(
      base({ metadata: { table_name: "Table 4", reason: "check" } }),
      "en",
    );
    expect(copy.title).toBe("Service call — Table 4 · Check, please");
    expect(copy.body).toBe("Table 4 · Check, please");
  });

  it("interpolates table_name and reason into the es title and body", () => {
    const copy = localizeAlert(
      base({ metadata: { table_name: "Mesa 4", reason: "water" } }),
      "es",
    );
    expect(copy.title).toBe("Llamada de servicio — Mesa 4 · Agua");
    expect(copy.body).toBe("Mesa 4 · Agua");
  });

  it("es-AR inherits the es service_call copy with reason", () => {
    const copy = localizeAlert(
      base({ metadata: { table_name: "Mesa 4", reason: "order" } }),
      "es-AR",
    );
    expect(copy.title).toBe("Llamada de servicio — Mesa 4 · Listo para pedir");
  });

  it("falls back to table-only copy when reason is absent", () => {
    const en = localizeAlert(
      base({ metadata: { table_name: "Table 2" } }),
      "en",
    );
    expect(en.title).toBe("Service call — Table 2");
    expect(en.body).toBe("A guest at Table 2 asked for help");
  });

  it("falls back to generic copy when table_name is absent (old alerts)", () => {
    const en = localizeAlert(base({ metadata: {} }), "en");
    expect(en.title).toBe("Service call");
    expect(en.body).toBe("A guest asked for help");

    const es = localizeAlert(base({ metadata: null }), "es");
    expect(es.title).toBe("Llamada de servicio");
    expect(es.body).toBe("Un cliente pidió ayuda");
  });
});

describe("operationalAlertCopy — empty-name dangles", () => {
  it.each(["reservation_new", "reservation_approval"] as const)(
    "%s body never renders a trailing 'from ' dangle with empty customer_name",
    (alertType) => {
      const copy = localizeAlert(
        base({ alert_type: alertType, metadata: { customer_name: "" } }),
        "en",
      );
      expect(copy.body).not.toMatch(/from\s*$/);
      expect(copy.body).not.toBe("Reservation from");
      expect(copy.body.length).toBeGreaterThan(0);

      const es = localizeAlert(
        base({ alert_type: alertType, metadata: {} }),
        "es",
      );
      expect(es.body).not.toMatch(/de\s*$/);
      expect(es.body.length).toBeGreaterThan(0);
    },
  );

  it("still names the customer when present", () => {
    const copy = localizeAlert(
      base({
        alert_type: "reservation_approval",
        metadata: { customer_name: "Alex" },
      }),
      "en",
    );
    expect(copy.body).toBe("Reservation from Alex");
  });

  it("delivery_new body guards a missing customer_name", () => {
    const copy = localizeAlert(
      base({ alert_type: "delivery_new", metadata: { delivery_number: 9 } }),
      "en",
    );
    expect(copy.body).toBe("A delivery needs dispatch");
  });
});

describe("operationalAlertCopy — neutral es voseo/typo regressions", () => {
  it("neutral es ai_takeover uses tuteo (abre), not voseo (abrí)", () => {
    const copy = localizeAlert(base({ alert_type: "ai_takeover" }), "es");
    expect(copy.body).toContain("abre el espacio");
    expect(copy.body).not.toContain("abrí");
  });

  it("es-AR ai_takeover keeps the voseo delta (abrí)", () => {
    const copy = localizeAlert(base({ alert_type: "ai_takeover" }), "es-AR");
    expect(copy.body).toContain("abrí");
  });

  it("es payment_refund_review body spells Concílialo correctly", () => {
    const copy = localizeAlert(
      base({
        alert_type: "payment_refund_review",
        metadata: { bill_number: "12" },
      }),
      "es",
    );
    expect(copy.body).toContain("Concílialo");
    expect(copy.body).not.toContain("Concíliàlo");
  });
});

describe("operationalAlertCopy — crypto payment review reasons", () => {
  // The backend raises crypto settlement failures, tx-hash conflicts and
  // refused wrong-amount transfers on the shared payment_refund_review type;
  // none of them is a partial refund.
  const review = (reason: string | undefined) =>
    base({
      alert_type: "payment_refund_review",
      metadata: { bill_number: "B-7", ...(reason ? { reason } : {}) },
    });

  it.each([
    ["crypto_settlement_failed", "Verified crypto payment failed to settle"],
    ["tx_hash_conflict", "Crypto payment reused on another bill"],
    ["amount_mismatch", "Crypto transfer with the wrong amount"],
  ])("en %s is not labelled a partial refund", (reason, title) => {
    const copy = localizeAlert(review(reason), "en");
    expect(copy.title).toBe(title);
    expect(copy.title).not.toMatch(/refund/i);
    expect(copy.body).toContain("bill #B-7");
    expect(`${copy.title} ${copy.body}`).not.toMatch(/partial/i);
  });

  it.each(["crypto_settlement_failed", "tx_hash_conflict", "amount_mismatch"])(
    "es and es-AR %s are not labelled a partial refund",
    (reason) => {
      for (const locale of ["es", "es-AR"] as const) {
        const copy = localizeAlert(review(reason), locale);
        expect(copy.body).toContain("cuenta #B-7");
        expect(`${copy.title} ${copy.body}`).not.toMatch(/reembolso/i);
      }
    },
  );

  it("es-AR crypto review bodies use voseo", () => {
    expect(localizeAlert(review("tx_hash_conflict"), "es-AR").body).toContain(
      "Verificá",
    );
    expect(localizeAlert(review("amount_mismatch"), "es-AR").body).toContain(
      "revisá",
    );
    expect(localizeAlert(review("tx_hash_conflict"), "es").body).toContain(
      "Verifica",
    );
  });

  // R2-M1: a replayed transfer that already paid a bill is reported against
  // that bill, and neither replay copy invites a manual settle or refund.
  const conflict = (metadata: Record<string, unknown>) =>
    base({
      alert_type: "payment_refund_review",
      metadata: {
        reason: "tx_hash_conflict",
        bill_id: 7,
        bill_number: "B-7",
        ...metadata,
      },
    });

  it("names the bill a replayed transfer already paid", () => {
    const en = localizeAlert(
      conflict({ recorded_bill_id: 3, recorded_bill_number: "B-3" }),
      "en",
    );
    expect(en.title).toBe("Crypto payment reused on another bill");
    expect(en.body).toContain("bill #B-7");
    expect(en.body).toContain("already paid bill #B-3");
    expect(en.body).toContain("Do not settle or refund anything");

    const es = localizeAlert(
      conflict({ recorded_bill_id: 3, recorded_bill_number: "B-3" }),
      "es",
    );
    expect(es.body).toContain("ya pagó la cuenta #B-3");
    expect(es.body).toContain("No saldes ni reembolses");
    const esAR = localizeAlert(
      conflict({ recorded_bill_id: 3, recorded_bill_number: "B-3" }),
      "es-AR",
    );
    expect(esAR.body).toContain("No saldés ni reembolsés");
  });

  it("calls a transfer replayed on the bill it already paid a duplicate", () => {
    const en = localizeAlert(
      conflict({ recorded_bill_id: 7, recorded_bill_number: "B-7" }),
      "en",
    );
    expect(en.title).toBe("Crypto payment presented twice");
    expect(en.body).toContain("already recorded on bill #B-7");
    expect(en.body).toContain("not counted twice");
    const es = localizeAlert(
      conflict({ recorded_bill_id: 7, recorded_bill_number: "B-7" }),
      "es",
    );
    expect(es.title).toBe("Pago cripto presentado dos veces");
    expect(
      localizeAlert(
        conflict({ recorded_bill_id: 7, recorded_bill_number: "B-7" }),
        "es-AR",
      ).title,
    ).toBe("Pago cripto presentado dos veces");
  });

  it("wrong-amount copy asks staff to rule out a transfer that is someone else's", () => {
    const en = localizeAlert(review("amount_mismatch"), "en");
    expect(en.body).toContain("It may belong to another guest");
    expect(en.body).toContain("no other bill has recorded it");
    expect(en.body).not.toContain("If the guest says they paid");
    expect(localizeAlert(review("amount_mismatch"), "es").body).toContain(
      "ninguna otra cuenta la tenga registrada",
    );
  });

  it("keeps the partial-refund copy for other reasons", () => {
    expect(localizeAlert(review(undefined), "en").title).toBe(
      "Partial refund needs manual reconciliation",
    );
    expect(localizeAlert(review("something_else"), "es-AR").body).toContain(
      "Concílialo",
    );
  });
});

describe("localizeClaimConflict", () => {
  it("names the claimer in en and es", () => {
    expect(localizeClaimConflict("en", "Sam")).toBe("Already claimed by Sam");
    expect(localizeClaimConflict("es", "Sam")).toBe("Ya lo reclamó Sam");
    expect(localizeClaimConflict("es-AR", "Sam")).toBe("Ya lo reclamó Sam");
  });

  it("falls back to a generic message without a name", () => {
    expect(localizeClaimConflict("en", null)).toBe(
      "Already claimed by another team member",
    );
    expect(localizeClaimConflict("es")).toBe(
      "Ya lo reclamó otra persona del equipo",
    );
  });
});

describe("bill identity matches the bills UI", () => {
  it("shows the sequential bill id instead of an opaque bill_number", () => {
    const copy = localizeAlert(
      {
        alert_type: "payment_received",
        title: "Payment received",
        body: "Payment received for bill #B5-5be6dfbf-bc1",
        metadata: { bill_id: 358, bill_number: "B5-5be6dfbf-bc1", amount: "12.00" },
      } as never,
      "en",
    );
    expect(copy.body).toContain("bill #358");
    expect(copy.body).not.toContain("B5-5be6dfbf");
  });

  it("keeps a readable bill_number", () => {
    const copy = localizeAlert(
      {
        alert_type: "payment_received",
        title: "Payment received",
        body: "",
        metadata: { bill_id: 358, bill_number: "A-104", amount: "12.00" },
      } as never,
      "en",
    );
    expect(copy.body).toContain("bill #A-104");
  });
});
