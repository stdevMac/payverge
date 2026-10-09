/**
 * L1-21 — map stable reservation history tokens to operator i18n keys.
 * Tokens are persisted by the backend in the existing notes text column
 * (migration-free). Free-text notes (staff-typed) render verbatim.
 */

const TOKEN_PREFIX = "token:";

export type HistoryNoteTranslator = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/**
 * Localize a reservation status code through the existing
 * `reservations.status.*` labels. Interpolating the raw code produced
 * half-translated sentences ("Estado cambiado de pending a confirmed").
 * An unmapped code (new backend status not yet in the message files) falls
 * back to the code itself rather than showing an empty span.
 */
function statusLabel(code: string | undefined, t: HistoryNoteTranslator): string {
  if (!code) return "?";
  const key = `status.${code}`;
  const translated = t(key);
  // Translators echo the key on a miss. ReservationManager's `t` prefixes with
  // `businessDashboard.reservations.`, so match on the suffix rather than the
  // bare key — otherwise the whole dotted path leaks into the UI.
  if (!translated || translated === key || translated.endsWith(`.${key}`)) {
    return code;
  }
  return translated;
}

/**
 * Format a stored history note for display. Known tokens become translated
 * strings under `historyNotes.*`; anything else is returned unchanged so
 * historical English prose and operator free-text still show.
 */
export function formatReservationHistoryNote(
  raw: string | null | undefined,
  t: HistoryNoteTranslator,
): string {
  if (!raw) return "";
  const trimmed = raw.trim();
  if (!trimmed.startsWith(TOKEN_PREFIX)) return trimmed;
  const body = trimmed.slice(TOKEN_PREFIX.length);

  if (body.startsWith("assigned_to:")) {
    const name = body.slice("assigned_to:".length);
    return t("historyNotes.assignedTo", { name });
  }
  if (body.startsWith("status_changed:")) {
    const rest = body.slice("status_changed:".length);
    const [from, to] = rest.split(":");
    return t("historyNotes.statusChanged", {
      from: statusLabel(from, t),
      to: statusLabel(to, t),
    });
  }

  const keyByToken: Record<string, string> = {
    assigned_table: "historyNotes.assignedTable",
    promoted_waitlist: "historyNotes.promotedWaitlist",
    added_waitlist: "historyNotes.addedWaitlist",
    created_with_table: "historyNotes.createdWithTable",
    created: "historyNotes.created",
    cancelled_by_customer: "historyNotes.cancelledByCustomer",
    cancelled_by_business: "historyNotes.cancelledByBusiness",
    auto_declined_no_response: "historyNotes.autoDeclinedNoResponse",
    auto_no_show_grace: "historyNotes.autoNoShowGrace",
    no_show_manager: "historyNotes.noShowManager",
    promoted_from_manager: "historyNotes.promotedFromManager",
  };
  const i18nKey = keyByToken[body];
  if (i18nKey) {
    return t(i18nKey);
  }
  // Unknown token — show body without prefix rather than raw token: noise.
  return body;
}
