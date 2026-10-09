/**
 * Operator-tier i18n regression tests for the delivery vertical.
 *
 * - LIVE finding: the es delivery status badge read "Delivery activado"
 *   (mixed language on a Spanish dashboard) — es must be fully translated.
 * - I18N-DEL-3: es-ar renders one activation-card bullet in voseo (zones,
 *   overridden) next to a tuteo bullet (management, fell through to base es).
 *   The voseo delta layer must cover every second-person string on the card.
 * - I18N-DEL-4: deliveryLockdown is a dead namespace — it must not be registered.
 * - L3-40 follow-up: ZoneEditor's activation guard + confirm modal resolve six
 *   focused.zones.* keys. getTranslation NEVER returns falsy (it falls back to
 *   the sentence-cased key leaf), so a `tString(...) || "literal"` fallback is
 *   dead code and a missing key ships as "Activate confirm title". These
 *   assertions read the REAL message files so key drift fails the build —
 *   an identity `tString` stub cannot catch it.
 */
import enMessages from "@/i18n/messages/en";
import esMessages from "@/i18n/messages/es";
import esArMessages from "@/i18n/messages/es-ar";

import enDeliverySettings from "@/i18n/messages/en/deliverySettings.json";
import esDeliverySettings from "@/i18n/messages/es/deliverySettings.json";
import esArDeliverySettings from "@/i18n/messages/es-ar/deliverySettings.json";
import esDeliveryToggle from "@/i18n/messages/es/deliveryToggle.json";
import esArDeliveryToggle from "@/i18n/messages/es-ar/deliveryToggle.json";

type AnyRecord = Record<string, any>;

/** Keys ZoneEditor.tsx resolves for the activation guard + confirm modal. */
const ZONE_ACTIVATION_KEYS = [
  "activationNeedsName",
  "activationNeedsGeography",
  "activateConfirmTitle",
  "activateConfirmBody",
  "activateConfirm",
  "cancel",
] as const;

describe("delivery operator messages", () => {
  it("es delivery status badge is fully Spanish (no 'Delivery activado' leak)", () => {
    const status = (esDeliverySettings as AnyRecord).focused.status;
    expect(status.on).toBe("Entrega activada");
    expect(status.off).toBe("Entrega desactivada");
  });

  it("cancel modal discloses guest visibility in en and es (DEL-UX-3)", () => {
    const en = (enDeliverySettings as AnyRecord).dispatch.cancel;
    const es = (esDeliverySettings as AnyRecord).dispatch.cancel;
    expect(typeof en.visibleToCustomer).toBe("string");
    expect(en.visibleToCustomer.length).toBeGreaterThan(0);
    expect(typeof es.visibleToCustomer).toBe("string");
    expect(es.visibleToCustomer.length).toBeGreaterThan(0);
  });

  it("es-ar overrides every second-person activation-card bullet (I18N-DEL-3)", () => {
    // Base es is tuteo ("Pausa …"); the es-ar delta must carry the voseo form
    // so the card doesn't mix registers between adjacent bullets.
    const base = (esDeliveryToggle as AnyRecord).activationCard.features;
    expect(base.management.description).toMatch(/^Pausa /);
    const delta = (esArDeliveryToggle as AnyRecord).activationCard.features;
    expect(delta.management?.description).toBe(
      "Pausá la entrega sin perder tus proveedores",
    );
  });

  it("zone activation guard + confirm keys exist in en and es (L3-40)", () => {
    const en = (enDeliverySettings as AnyRecord).focused.zones;
    const es = (esDeliverySettings as AnyRecord).focused.zones;
    for (const key of ZONE_ACTIVATION_KEYS) {
      expect(typeof en[key]).toBe("string");
      expect(en[key].length).toBeGreaterThan(0);
      expect(typeof es[key]).toBe("string");
      expect(es[key].length).toBeGreaterThan(0);
    }
  });

  it("es-ar overrides the second-person zone activation strings in voseo (L3-40)", () => {
    const esZones = (esDeliverySettings as AnyRecord).focused.zones;
    const arZones = (esArDeliverySettings as AnyRecord).focused.zones;
    // Base es is tuteo; every imperative addressed to the operator needs a
    // Rioplatense delta so the modal doesn't mix registers.
    expect(esZones.activationNeedsName).toMatch(/^Ponle /);
    expect(arZones.activationNeedsName).toMatch(/^Ponele /);
    expect(esZones.activationNeedsGeography).toMatch(/^Agrega /);
    expect(arZones.activationNeedsGeography).toMatch(/^Agregá /);
    expect(esZones.activateConfirmBody).toContain("Confirma ");
    expect(arZones.activateConfirmBody).toContain("Confirmá ");
  });

  // The driver scorecard stacks these five counters in one column. They all
  // count *entregas* (feminine — `issuesAria` says "fallidas, canceladas"), so
  // a masculine participle next to a feminine one reads as a translation bug
  // on the operator's own dashboard.
  it("es driver KPI participles agree in gender (feminine: entregas)", () => {
    const kpi = (esDeliverySettings as AnyRecord).performance.kpi;
    for (const key of [
      "completedToday",
      "completedWeek",
      "completedAllTime",
      "cancelled",
      "failed",
    ]) {
      expect(typeof kpi[key]).toBe("string");
      expect(kpi[key]).toMatch(/^(Completadas|Canceladas|Fallidas)\b/);
    }
  });

  it("dead deliveryLockdown namespace is gone from all operator bundles (I18N-DEL-4)", () => {
    expect((enMessages as AnyRecord).deliveryLockdown).toBeUndefined();
    expect((esMessages as AnyRecord).deliveryLockdown).toBeUndefined();
    expect((esArMessages as AnyRecord).deliveryLockdown).toBeUndefined();
  });

  // issue 237: the online radio was titled "Pay online before preparation"
  // while the helper said guests pay after accept. Title + help must describe
  // the same accept-then-pay sequence — not pay-at-order-time vs after-accept.
  describe("online payment-mode title and help describe the same sequence", () => {
    const locales: Array<[string, AnyRecord]> = [
      ["en", enDeliverySettings],
      ["es", esDeliverySettings],
      ["es-ar", esArDeliverySettings],
    ];

    it("flags the original contradictory English pair", () => {
      expect(
        paymentModeSequencesAgree(
          "Pay online before preparation",
          "Guests pay after you accept; the kitchen starts on payment. Unpaid orders expire after 15 minutes — nothing is charged.",
        ),
      ).toBe(false);
    });

    it.each(locales)(
      "%s title and helper both describe accept-then-pay",
      (_locale, bundle) => {
        const cfg = bundle.focused.configuration;
        expect(typeof cfg.paymentModeOnline).toBe("string");
        expect(typeof cfg.paymentModeOnlineHelp).toBe("string");
        expect(
          paymentModeSequencesAgree(
            cfg.paymentModeOnline,
            cfg.paymentModeOnlineHelp,
          ),
        ).toBe(true);
      },
    );
  });
});

/** Pay-at-submit / pay-then-kitchen wording. */
function describesPayBeforePrep(text: string): boolean {
  return (
    /before (preparation|prep|the kitchen)/i.test(text) ||
    /pay online before/i.test(text) ||
    /antes de (la )?(preparaci[oó]n|cocina)/i.test(text) ||
    /pagar en l[ií]nea antes/i.test(text)
  );
}

/** Accept first, then the guest pays online. */
function describesAcceptThenPay(text: string): boolean {
  return (
    /accept first/i.test(text) ||
    /after you accept/i.test(text) ||
    /you accept the order/i.test(text) ||
    /aceptar primero/i.test(text) ||
    /acept[aá]s el pedido/i.test(text)
  );
}

/**
 * Title and helper agree only when both encode accept-then-pay and neither
 * encodes pay-before-prep. Mixed sequences (the issue 237 pair) return false.
 */
function paymentModeSequencesAgree(title: string, help: string): boolean {
  if (describesPayBeforePrep(title) || describesPayBeforePrep(help)) {
    return false;
  }
  return describesAcceptThenPay(title) && describesAcceptThenPay(help);
}
