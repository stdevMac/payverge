/**
 * Operator-facing bill identity helpers (production-readiness Task 8).
 *
 * Operators must never see a `#DEMO-` prefix, and opaque UUID fragments should
 * not be the only handle — keep the full id available for support copy.
 */

export type OperatorBillIdentity = {
  id?: number | null;
  bill_number?: string | null;
};

/** Strip decorative `#` and any leading `DEMO-` (case-insensitive). */
export function stripDemoBillPrefix(raw: string): string {
  return raw
    .trim()
    .replace(/^#/, "")
    .replace(/^DEMO-/i, "")
    .trim();
}

/**
 * Human display number for operator lists and modals.
 * Prefer a non-opaque `bill_number` (after DEMO strip). Opaque UUID fragments
 * (L2-4 / L2-28) are support handles — prefer sequential `id` for the
 * visible label so operators never read an internal ID as "Factura #".
 */
export function operatorBillDisplayNumber(
  bill: OperatorBillIdentity,
): string {
  const raw = (bill.bill_number ?? "").toString();
  const cleaned = stripDemoBillPrefix(raw);
  if (cleaned && !isOpaqueBillNumber(cleaned)) {
    return cleaned;
  }
  if (bill.id != null && Number.isFinite(Number(bill.id))) {
    return String(bill.id);
  }
  // No sequential id — fall back to cleaned opaque string rather than "—".
  if (cleaned) return cleaned;
  return "—";
}

export type OperatorTenderIdentity = OperatorBillIdentity & {
  participant_name?: string | null;
};

/**
 * Label for a tender row that may identify itself by bill number, by the person
 * who paid, or only by an internal id (R2-10).
 *
 * Precedence: readable bill number (keeping the decorative `#`) -> participant
 * name -> `#id` -> fallback. Opaque UUID fragments (L2-4 / L2-28) never surface:
 * they fall through to the participant name, then to the sequential id. Unlike
 * `operatorBillDisplayNumber` this never renders a bare internal number — a row
 * with a bill_id but no bill_number must still read as a person or as `#id`.
 */
export function operatorTenderLabel(
  tender: OperatorTenderIdentity,
  fallback = "—",
): string {
  const cleaned = stripDemoBillPrefix((tender.bill_number ?? "").toString());
  if (cleaned && !isOpaqueBillNumber(cleaned)) {
    return `#${cleaned}`;
  }
  const participant = (tender.participant_name ?? "").toString().trim();
  if (participant) {
    return participant;
  }
  const id = Number(tender.id);
  if (tender.id != null && Number.isFinite(id) && id > 0) {
    return `#${id}`;
  }
  return fallback;
}

/**
 * Full identity for support / clipboard. Prefer the stored bill_number
 * (DEMO-stripped so operator surfaces stay clean); fall back to id.
 */
export function operatorBillSupportId(bill: OperatorBillIdentity): string {
  const cleaned = stripDemoBillPrefix((bill.bill_number ?? "").toString());
  if (cleaned) return cleaned;
  if (bill.id != null && Number.isFinite(Number(bill.id))) {
    return String(bill.id);
  }
  return "";
}

/** True when the stored number looks like an opaque UUID fragment (B{biz}-{hex}). */
export function isOpaqueBillNumber(billNumber: string): boolean {
  const cleaned = stripDemoBillPrefix(billNumber);
  // Legacy placeholders (PENDING-*) must not surface as operator bill numbers.
  if (/^PENDING-/i.test(cleaned)) return true;
  // Backend format: B{businessID}-{uuid.String()[:12]} — the slice can include
  // hyphens from the UUID (e.g. "B2-0d60c280-49f").
  return /^B\d+-[0-9a-f-]{8,}$/i.test(cleaned) || cleaned.length > 18;
}
