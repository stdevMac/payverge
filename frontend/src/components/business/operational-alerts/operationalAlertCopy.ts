import type {
  OperationalAlert,
  OperationalAlertType,
} from "@/api/operationalAlerts";
import type { Locale } from "@/i18n/localeRegistry";
import { operatorBillDisplayNumber } from "@/lib/operatorBillNumber";

/**
 * Render-time localization for operational alert titles/bodies.
 *
 * The backend (`operational_alerts/service.go`) stores alert `title`/`body` in
 * English only, but carries structured `metadata` (order_number, customer_name,
 * bill_number, table reason, …) plus a stable `alert_type`. Rather than a
 * data migration, we localize at render time here by (alert_type → template) +
 * metadata params. This keeps the wire/data shape English and lets any client
 * present the operator's locale. (R3-AI-8)
 *
 * Fallback contract: an unknown alert_type or a template that references a
 * missing param falls back to the backend-provided English `alert.title` /
 * `alert.body`. We never render a raw key or a blank string.
 *
 * These dictionaries are intentionally self-contained (not routed through the
 * orchestrator-owned businessDashboard.json bundle) so this fix ships without
 * touching that file; the operator tier is en/es/es-AR only.
 */

type Params = Record<string, string | number>;
type Template = (p: Params) => string;

interface AlertCopy {
  title: Template;
  body?: Template;
}

const p = (params: Record<string, unknown> | null | undefined): Params => {
  const out: Params = {};
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (typeof v === "string" || typeof v === "number") out[k] = v;
    }
  }
  return out;
};

/**
 * Rewrite bill_number / recorded_bill_number to the number the bills UI shows
 * (operatorBillDisplayNumber): an opaque stored handle such as
 * "B5-5be6dfbf-bc1" becomes the sequential id ("358"), so a toast and the
 * bill it names read the same.
 */
function withOperatorBillNumbers(params: Params): Params {
  const out = { ...params };
  for (const [numberKey, idKey] of [
    ["bill_number", "bill_id"],
    ["recorded_bill_number", "recorded_bill_id"],
  ] as const) {
    if (out[numberKey] == null && out[idKey] == null) continue;
    const id = out[idKey] != null ? Number(out[idKey]) : null;
    const shown = operatorBillDisplayNumber({
      id: id != null && Number.isFinite(id) ? id : null,
      bill_number: out[numberKey] != null ? String(out[numberKey]) : null,
    });
    if (shown !== "—") out[numberKey] = shown;
  }
  return out;
}

/** Guest service-call reason codes stored in alert metadata.reason. */
const SERVICE_CALL_REASON_EN: Record<string, string> = {
  water: "Water",
  order: "Ready to order",
  check: "Check, please",
};

const SERVICE_CALL_REASON_ES: Record<string, string> = {
  water: "Agua",
  order: "Listo para pedir",
  check: "La cuenta",
};

function paymentBody(prefix: string, v: Params): string {
  const bill = v.bill_number != null && String(v.bill_number) !== ""
    ? `#${v.bill_number}`
    : "";
  const amount =
    v.amount ??
    v.amount_paid ??
    (typeof v.amount_cents === "number" ? (v.amount_cents / 100).toFixed(2) : undefined);
  const method = v.method ?? v.payment_method ?? v.provider;
  // No structured identity → keep the backend-stored body (demo rows carry
  // the bill token only in `alert.body`, with metadata `{demo:true}`).
  if (!bill && (amount == null || String(amount) === "") && (method == null || String(method) === "")) {
    return "";
  }
  let out = bill ? `${prefix} ${bill}` : prefix.replace(/ for bill$| para la cuenta$/, "");
  if (amount != null && String(amount) !== "") {
    const currency = v.currency ? String(v.currency) : "";
    out += ` · ${currency}${currency && !String(amount).startsWith(currency) ? " " : ""}${amount}`;
  }
  if (method != null && String(method) !== "") {
    out += ` · ${method}`;
  }
  return out.trim();
}

/**
 * payment_refund_review is shared by several backend producers. The crypto
 * ones (operational_alerts.CryptoReviewReason*) set metadata.reason; any other
 * reason keeps the partial-refund copy.
 */
type CryptoReviewReason =
  | "crypto_settlement_failed"
  | "tx_hash_conflict"
  | "amount_mismatch";

function cryptoReviewReason(v: Params): CryptoReviewReason | null {
  switch (v.reason) {
    case "crypto_settlement_failed":
    case "tx_hash_conflict":
    case "amount_mismatch":
      return v.reason;
    default:
      return null;
  }
}

function billRef(v: Params): string {
  return v.bill_number != null && String(v.bill_number) !== ""
    ? ` #${v.bill_number}`
    : "";
}

/**
 * tx_hash_conflict may name the bill that already records the transfer
 * (metadata.recorded_bill_*, same business only). "same" means the transfer
 * was presented again on the bill it already paid, "other" that it already
 * paid another bill, null that the holder is unknown.
 */
function recordedHolder(
  v: Params,
): { kind: "same" | "other"; ref: string } | null {
  const number = v.recorded_bill_number;
  if (number == null || String(number) === "") return null;
  const same =
    v.recorded_bill_id != null &&
    v.bill_id != null &&
    String(v.recorded_bill_id) === String(v.bill_id);
  return { kind: same ? "same" : "other", ref: `#${number}` };
}

const CRYPTO_REVIEW_EN: Record<CryptoReviewReason, AlertCopy> = {
  crypto_settlement_failed: {
    title: () => "Verified crypto payment failed to settle",
    body: (v) =>
      `A verified USDC transfer for bill${billRef(v)} could not be recorded as a payment. Reconcile it before closing the bill.`,
  },
  tx_hash_conflict: {
    title: (v) =>
      recordedHolder(v)?.kind === "same"
        ? "Crypto payment presented twice"
        : "Crypto payment reused on another bill",
    body: (v) => {
      const holder = recordedHolder(v);
      if (holder?.kind === "same") {
        return `A transfer already recorded on bill${billRef(v)} was presented again under a different payment quote. It was not counted twice. Do not settle or refund anything with it.`;
      }
      if (holder) {
        return `Someone tried to settle bill${billRef(v)} with a transfer that already paid bill ${holder.ref}. The bill was not marked paid. Do not settle or refund anything with this transfer.`;
      }
      return `Someone tried to settle bill${billRef(v)} with a transfer that is already recorded. The bill was not marked paid. Check who paid before settling or refunding anything by hand.`;
    },
  },
  amount_mismatch: {
    title: () => "Crypto transfer with the wrong amount",
    body: (v) =>
      `A USDC transfer for bill${billRef(v)} did not carry the quoted amount, so it was not recorded. It may belong to another guest. Before settling or refunding anything by hand, check that the guest's own wallet sent it and that no other bill has recorded it.`,
  },
};

const CRYPTO_REVIEW_ES: Record<CryptoReviewReason, AlertCopy> = {
  crypto_settlement_failed: {
    title: () => "Un pago cripto verificado no se pudo registrar",
    body: (v) =>
      `Una transferencia USDC verificada para la cuenta${billRef(v)} no se pudo registrar como pago. Concíliala antes de cerrar la cuenta.`,
  },
  tx_hash_conflict: {
    title: (v) =>
      recordedHolder(v)?.kind === "same"
        ? "Pago cripto presentado dos veces"
        : "Pago cripto reutilizado en otra cuenta",
    body: (v) => {
      const holder = recordedHolder(v);
      if (holder?.kind === "same") {
        return `Una transferencia ya registrada en la cuenta${billRef(v)} se presentó otra vez con otra cotización. No se contó dos veces. No saldes ni reembolses nada con ella.`;
      }
      if (holder) {
        return `Alguien intentó saldar la cuenta${billRef(v)} con una transferencia que ya pagó la cuenta ${holder.ref}. La cuenta no se marcó como pagada. No saldes ni reembolses nada con esta transferencia.`;
      }
      return `Alguien intentó saldar la cuenta${billRef(v)} con una transferencia que ya está registrada. La cuenta no se marcó como pagada. Verifica quién pagó antes de saldar o reembolsar algo a mano.`;
    },
  },
  amount_mismatch: {
    title: () => "Transferencia cripto con monto incorrecto",
    body: (v) =>
      `Una transferencia USDC para la cuenta${billRef(v)} no tenía el monto cotizado, así que no se registró. Puede ser de otro cliente. Antes de saldar o reembolsar algo a mano, revisa que la haya enviado la billetera del propio cliente y que ninguna otra cuenta la tenga registrada.`,
  },
};

const CRYPTO_REVIEW_ES_AR: Record<CryptoReviewReason, AlertCopy> = {
  crypto_settlement_failed: {
    title: CRYPTO_REVIEW_ES.crypto_settlement_failed.title,
    body: (v) =>
      `Una transferencia USDC verificada para la cuenta${billRef(v)} no se pudo registrar como pago. Conciliala antes de cerrar la cuenta.`,
  },
  tx_hash_conflict: {
    title: CRYPTO_REVIEW_ES.tx_hash_conflict.title,
    body: (v) => {
      const holder = recordedHolder(v);
      if (holder?.kind === "same") {
        return `Una transferencia ya registrada en la cuenta${billRef(v)} se presentó otra vez con otra cotización. No se contó dos veces. No saldés ni reembolsés nada con ella.`;
      }
      if (holder) {
        return `Alguien intentó saldar la cuenta${billRef(v)} con una transferencia que ya pagó la cuenta ${holder.ref}. La cuenta no se marcó como pagada. No saldés ni reembolsés nada con esta transferencia.`;
      }
      return `Alguien intentó saldar la cuenta${billRef(v)} con una transferencia que ya está registrada. La cuenta no se marcó como pagada. Verificá quién pagó antes de saldar o reembolsar algo a mano.`;
    },
  },
  amount_mismatch: {
    title: CRYPTO_REVIEW_ES.amount_mismatch.title,
    body: (v) =>
      `Una transferencia USDC para la cuenta${billRef(v)} no tenía el monto cotizado, así que no se registró. Puede ser de otro cliente. Antes de saldar o reembolsar algo a mano, revisá que la haya enviado la billetera del propio cliente y que ninguna otra cuenta la tenga registrada.`,
  },
};

const PARTIAL_REFUND_ES: AlertCopy = {
  title: () => "Un reembolso parcial necesita conciliación manual",
  body: (v) =>
    `Se recibió un reembolso parcial para la cuenta #${v.bill_number ?? ""}. Concílialo manualmente.`,
};

/** payment_refund_review copy: crypto reasons first, partial refund otherwise. */
function refundReviewCopy(
  crypto: Record<CryptoReviewReason, AlertCopy>,
  partialRefund: AlertCopy,
): AlertCopy {
  return {
    title: (v) => {
      const reason = cryptoReviewReason(v);
      return reason ? crypto[reason].title(v) : partialRefund.title(v);
    },
    body: (v) => {
      const reason = cryptoReviewReason(v);
      const template = reason ? crypto[reason].body : partialRefund.body;
      return template ? template(v) : "";
    },
  };
}

export function serviceCallReasonLabel(
  reason: string | number | undefined | null,
  locale: "en" | "es",
): string {
  if (reason === undefined || reason === null) return "";
  const key = String(reason).trim().toLowerCase();
  if (!key) return "";
  const dict =
    locale === "es" ? SERVICE_CALL_REASON_ES : SERVICE_CALL_REASON_EN;
  return dict[key] || String(reason);
}

// en / es / es-AR templates keyed by alert_type. es-AR falls back to es unless
// an override is present.
const EN: Partial<Record<OperationalAlertType, AlertCopy>> = {
  order_new: {
    title: (v) => `New order #${v.order_number ?? ""}`.trim(),
    body: (v) => `Order #${v.order_number ?? ""} needs review`,
  },
  kitchen_order_ready: {
    title: (v) => `Kitchen order #${v.order_number ?? ""}`.trim(),
    body: (v) => `Order #${v.order_number ?? ""} is ready for kitchen`,
  },
  reservation_new: {
    title: () => "New reservation",
    // Guard the empty-name case so we never render a dangling
    // "Reservation from " (metadata.customer_name can be blank).
    body: (v) =>
      v.customer_name
        ? `Reservation from ${v.customer_name}`
        : "A new reservation was received",
  },
  reservation_approval: {
    title: () => "Reservation needs approval",
    body: (v) =>
      v.customer_name
        ? `Reservation from ${v.customer_name}`
        : "A reservation is awaiting approval",
  },
  delivery_new: {
    title: (v) => `New delivery #${v.delivery_number ?? ""}`.trim(),
    body: (v) =>
      v.customer_name
        ? `Delivery for ${v.customer_name} needs dispatch`
        : "A delivery needs dispatch",
  },
  bill_new: {
    title: (v) => `New bill #${v.bill_number ?? ""}`.trim(),
    body: () => "A new bill was created",
  },
  payment_requested: {
    title: () => "Payment requested",
    body: (v) => paymentBody("Payment requested for bill", v),
  },
  payment_received: {
    title: () => "Payment received",
    body: (v) => paymentBody("Payment received for bill", v),
  },
  payment_refund_review: refundReviewCopy(CRYPTO_REVIEW_EN, {
    title: () => "Partial refund needs manual reconciliation",
    body: (v) =>
      `A partial provider refund was received for bill #${v.bill_number ?? ""}. Reconcile it manually.`,
  }),
  report_delivery_failed: {
    title: () => "Scheduled report delivery failed",
    body: () =>
      "A scheduled report exhausted its delivery attempts and needs review.",
  },
  service_call: {
    // Backend metadata carries `table_name` + `reason` (water|order|check).
    // Older alerts may lack either — fall back without dangling separators.
    // PV-LIVE-20260720-009: reason must appear in toast title/body so floor
    // staff know what the guest wants without opening the popover.
    title: (v) => {
      const reason = serviceCallReasonLabel(v.reason, "en");
      if (v.table_name && reason) {
        return `Service call — ${v.table_name} · ${reason}`;
      }
      if (v.table_name) return `Service call — ${v.table_name}`;
      if (reason) return `Service call — ${reason}`;
      return "Service call";
    },
    body: (v) => {
      const reason = serviceCallReasonLabel(v.reason, "en");
      if (v.table_name && reason) {
        return `${v.table_name} · ${reason}`;
      }
      if (v.table_name) return `A guest at ${v.table_name} asked for help`;
      if (reason) return `Guest needs: ${reason}`;
      return "A guest asked for help";
    },
  },
  ai_takeover: {
    title: () => "Guest waiting for a human",
    body: () =>
      "A guest wrote in a paused AI conversation — open the AI Waiter workspace to reply",
  },
};

const ES: Partial<Record<OperationalAlertType, AlertCopy>> = {
  order_new: {
    title: (v) => `Nuevo pedido #${v.order_number ?? ""}`.trim(),
    body: (v) => `El pedido #${v.order_number ?? ""} necesita revisión`,
  },
  kitchen_order_ready: {
    title: (v) => `Pedido de cocina #${v.order_number ?? ""}`.trim(),
    body: (v) => `El pedido #${v.order_number ?? ""} está listo para cocina`,
  },
  reservation_new: {
    title: () => "Nueva reserva",
    body: (v) =>
      v.customer_name
        ? `Reserva de ${v.customer_name}`
        : "Se recibió una nueva reserva",
  },
  reservation_approval: {
    title: () => "La reserva necesita aprobación",
    body: (v) =>
      v.customer_name
        ? `Reserva de ${v.customer_name}`
        : "Una reserva espera aprobación",
  },
  delivery_new: {
    title: (v) => `Nuevo envío #${v.delivery_number ?? ""}`.trim(),
    body: (v) =>
      v.customer_name
        ? `El envío para ${v.customer_name} necesita despacho`
        : "Un envío necesita despacho",
  },
  bill_new: {
    title: (v) => `Nueva cuenta #${v.bill_number ?? ""}`.trim(),
    body: () => "Se creó una nueva cuenta",
  },
  payment_requested: {
    title: () => "Pago solicitado",
    body: (v) => paymentBody("Pago solicitado para la cuenta", v),
  },
  payment_received: {
    title: () => "Pago recibido",
    body: (v) => paymentBody("Pago recibido para la cuenta", v),
  },
  payment_refund_review: refundReviewCopy(CRYPTO_REVIEW_ES, PARTIAL_REFUND_ES),
  report_delivery_failed: {
    title: () => "Falló la entrega del informe programado",
    body: () =>
      "Un informe programado agotó sus intentos de entrega y necesita revisión.",
  },
  service_call: {
    title: (v) => {
      const reason = serviceCallReasonLabel(v.reason, "es");
      if (v.table_name && reason) {
        return `Llamada de servicio — ${v.table_name} · ${reason}`;
      }
      if (v.table_name) return `Llamada de servicio — ${v.table_name}`;
      if (reason) return `Llamada de servicio — ${reason}`;
      return "Llamada de servicio";
    },
    body: (v) => {
      const reason = serviceCallReasonLabel(v.reason, "es");
      if (v.table_name && reason) {
        return `${v.table_name} · ${reason}`;
      }
      if (v.table_name) return `Un cliente en ${v.table_name} pidió ayuda`;
      if (reason) return `El cliente necesita: ${reason}`;
      return "Un cliente pidió ayuda";
    },
  },
  ai_takeover: {
    title: () => "Un cliente espera a una persona",
    // Neutral es is tuteo ("abre"); the voseo form ("abrí") lives only in the
    // es-AR delta below.
    body: () =>
      "Un cliente escribió en una conversación de IA en pausa — abre el espacio de IA para responder",
  },
};

// es-AR: only the deltas from es (voseo, local vocab).
const ES_AR: Partial<Record<OperationalAlertType, AlertCopy>> = {
  // Crypto review bodies need voseo; the partial-refund copy is shared with es.
  payment_refund_review: refundReviewCopy(
    CRYPTO_REVIEW_ES_AR,
    PARTIAL_REFUND_ES,
  ),
  ai_takeover: {
    title: () => "Un cliente espera a una persona",
    body: () =>
      "Un cliente escribió en una conversación de IA en pausa — abrí el espacio de IA para responder",
  },
};

function dictFor(
  locale: Locale,
): Partial<Record<OperationalAlertType, AlertCopy>> {
  switch (locale) {
    case "es":
      return ES;
    case "es-AR":
      return { ...ES, ...ES_AR };
    default:
      return EN;
  }
}

export interface LocalizedAlertCopy {
  title: string;
  body: string;
}

/**
 * Returns the operator-locale title/body for an alert, falling back to the
 * backend English `alert.title`/`alert.body` when no template exists.
 */
export function localizeAlert(
  alert: Pick<OperationalAlert, "alert_type" | "title" | "body" | "metadata">,
  locale: Locale,
): LocalizedAlertCopy {
  const entry = dictFor(locale)[alert.alert_type];
  const params = withOperatorBillNumbers(p(alert.metadata));

  let title = alert.title;
  let body = alert.body;

  if (entry) {
    try {
      const t = entry.title(params).trim();
      if (t) title = t;
      if (entry.body) {
        const b = entry.body(params).trim();
        if (b) body = b;
      }
    } catch {
      // Any template error → keep the backend English strings.
    }
  }

  return { title, body: body ?? "" };
}

/**
 * Toast copy for a claim/resolve HTTP 409 conflict ("someone else got there
 * first"). Lives here (not businessDashboard.json) for the same reason as the
 * alert dictionaries above: self-contained operator-tier copy, en/es only,
 * with es-AR inheriting es (no voseo needed in these strings).
 */
export function localizeClaimConflict(
  locale: Locale,
  claimedByName?: string | null,
): string {
  const isSpanish = locale === "es" || locale === "es-AR";
  if (claimedByName) {
    return isSpanish
      ? `Ya lo reclamó ${claimedByName}`
      : `Already claimed by ${claimedByName}`;
  }
  return isSpanish
    ? "Ya lo reclamó otra persona del equipo"
    : "Already claimed by another team member";
}
