/**
 * Shared regex for the i18n leak safety net.
 *
 * Matches strings that look like raw dotted i18n keys — a lowercase camelCase
 * head segment followed by at least two `.camelCase` tail segments, e.g.
 * `businessDashboard.crm.title`. Anything that survives the i18n layer
 * unresolved gets echoed back verbatim by `getTranslation`, so the resulting
 * DOM contains tokens of this exact shape.
 */
export const I18N_KEY_RE = /^[a-z][a-zA-Z]*(\.[a-zA-Z][a-zA-Z0-9]*){2,}$/;
