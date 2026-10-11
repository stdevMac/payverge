"use client";

import React, { useId, useMemo, useState } from "react";
import { Receipt, ChevronDown } from "lucide-react";
import type { FiscalCustomerPayload } from "@/api/bills";

// FiscalIdentityFields — reusable, progressively-disclosed capture of the
// customer's fiscal identity at checkout (AFIP/ARCA, T22). Collapsed by default
// behind a "need a fiscal invoice?" toggle; when expanded it offers a doc-type
// select (Consumidor Final default / DNI / CUIT / CUIL), a doc-number input, a
// tax-condition select, and an optional name. Client-validates DNI (7-8 digits)
// and CUIT/CUIL (11 digits) and surfaces inline errors. Identity is OPTIONAL —
// an empty form is valid (the backend fiscal policy enforces the above-threshold
// requirement at issuance time).
//
// Light mode only (no dark-mode variants), per the design system.

export type FiscalDocTypeChoice = "" | "DNI" | "CUIT" | "CUIL";
export type FiscalTaxConditionChoice =
  | ""
  | "consumidor_final"
  | "responsable_inscripto"
  | "monotributo"
  | "exento";

export interface FiscalIdentityValue {
  docType: FiscalDocTypeChoice;
  docNumber: string;
  taxCondition: FiscalTaxConditionChoice;
  name: string;
  email: string;
}

export function emptyFiscalIdentity(): FiscalIdentityValue {
  return { docType: "", docNumber: "", taxCondition: "", name: "", email: "" };
}

// countDigits ignores separators (hyphens/dots), matching the backend's mapper
// which strips non-digits before validating CUIT/DNI length.
function countDigits(s: string): number {
  let n = 0;
  for (const ch of s) {
    if (ch >= "0" && ch <= "9") n += 1;
  }
  return n;
}

// validateFiscalIdentity returns the field key of the first problem
// ("docType" | "docNumber" | "email"), or null when the identity is acceptable.
// An empty identity is acceptable (optional). A doc number requires a doc type.
// Email is optional; when present it must look like a valid address.
export function validateFiscalIdentity(
  value: FiscalIdentityValue,
): "docType" | "docNumber" | "email" | null {
  const email = (value.email ?? "").trim();
  // Same shape as /^[^\s@]+@[^\s@]+\.[^\s@]+$/ without the overlapping
  // quantifiers that backtrack polynomially: one "@" with non-blank sides, and
  // a "." in the domain that is neither its first nor its last character.
  if (
    email &&
    !(/^[^\s@]+@[^\s@]+$/.test(email) &&
      email.slice(email.indexOf("@") + 2, -1).includes("."))
  ) {
    return "email";
  }
  const docNumber = value.docNumber.trim();
  if (!docNumber) {
    return null; // no document → nothing to validate (optional)
  }
  if (!value.docType) {
    return "docType"; // ambiguous: cannot validate digit shape without a type
  }
  const digits = countDigits(docNumber);
  if (value.docType === "CUIT" || value.docType === "CUIL") {
    return digits === 11 ? null : "docNumber";
  }
  if (value.docType === "DNI") {
    return digits >= 7 && digits <= 8 ? null : "docNumber";
  }
  return null;
}

// fiscalIdentityToPayload maps the controlled value to the backend wire shape,
// trimming and omitting blank fields so they persist as NULL (consumidor final).
export function fiscalIdentityToPayload(
  value: FiscalIdentityValue,
): FiscalCustomerPayload {
  const payload: FiscalCustomerPayload = {};
  const docType = value.docType.trim();
  const docNumber = value.docNumber.trim();
  const taxCondition = value.taxCondition.trim();
  const name = value.name.trim();
  const email = (value.email ?? "").trim();
  if (docType) payload.fiscal_customer_doc_type = docType;
  if (docNumber) payload.fiscal_customer_doc_number = docNumber;
  if (taxCondition) payload.fiscal_customer_tax_condition = taxCondition;
  if (name) payload.fiscal_customer_name = name;
  if (email) payload.fiscal_customer_email = email;
  return payload;
}

interface FiscalIdentityFieldsProps {
  value: FiscalIdentityValue;
  onChange: (value: FiscalIdentityValue, isValid: boolean) => void;
  // Translator resolving keys relative to "fiscalCustomer.*" (the caller wires
  // the namespace, e.g. guest menu.fiscalCustomer or operator
  // billManager.fiscalCustomer).
  t: (key: string) => string;
  // Palette: "guest" uses warm/brand tokens (CartModal), "operator" uses the
  // neutral gray set (BillDetailsModal). Defaults to guest.
  variant?: "guest" | "operator";
  // Start expanded (e.g. when the bill already has a fiscal identity).
  defaultExpanded?: boolean;
  /** When true, selects/inputs are disabled (closed-bill operator review). */
  readOnly?: boolean;
  className?: string;
  /**
   * Optional actions (e.g. Save) rendered inside the expanded panel only.
   * Keeps CTAs from hanging below a collapsed accordion (#103).
   */
  actions?: React.ReactNode;
}

const PALETTES = {
  guest: {
    card: "rounded-2xl border border-warm-200 bg-white",
    toggle: "text-brand-dark hover:text-brand-800",
    label: "text-ink-700",
    field:
      "w-full rounded-xl border border-warm-200 bg-warm-50 px-3 py-2 text-sm text-ink-900 outline-none transition focus:border-brand focus:ring-1 focus:ring-brand",
    fieldError: "border-rose-400 focus:border-rose-500 focus:ring-rose-400",
    error: "text-rose-600",
    hint: "text-ink-500",
  },
  operator: {
    card: "rounded-xl border border-gray-200 bg-white",
    toggle: "text-teal-700 hover:text-teal-800",
    label: "text-gray-700",
    field:
      "w-full rounded-lg border border-gray-300 bg-white px-3 py-2 text-sm text-gray-900 outline-none transition focus:border-teal-600 focus:ring-1 focus:ring-teal-600",
    fieldError: "border-rose-400 focus:border-rose-500 focus:ring-rose-400",
    error: "text-rose-600",
    hint: "text-gray-500",
  },
} as const;

export function FiscalIdentityFields({
  value,
  onChange,
  t,
  variant = "guest",
  defaultExpanded = false,
  readOnly = false,
  className = "",
  actions,
}: FiscalIdentityFieldsProps) {
  const [expanded, setExpanded] = useState(defaultExpanded);
  const palette = PALETTES[variant];
  const docNumberErrorId = useId();
  const cuitHintId = useId();

  const problem = useMemo(() => validateFiscalIdentity(value), [value]);

  // Soft coupling (T11): a Responsable Inscripto can only receive a factura A,
  // which AFIP requires to carry a CUIT/CUIL. When the customer declares
  // "responsable_inscripto" but the document is missing or a DNI, the backend
  // correctly downgrades to factura B — so nudge toward a CUIT/CUIL. This is
  // advisory only: it never changes validity (validateFiscalIdentity is
  // untouched) so the bill can still be issued as factura B.
  const showCuitHint =
    value.taxCondition === "responsable_inscripto" &&
    (value.docType === "" || value.docType === "DNI");

  const update = (patch: Partial<FiscalIdentityValue>) => {
    if (readOnly) return;
    const next = { ...value, ...patch };
    onChange(next, validateFiscalIdentity(next) === null);
  };

  const docNumberError = problem === "docNumber" || problem === "docType";
  const docNumberErrorMsg =
    value.docType === "DNI" ? t("fiscalCustomer.errorDni") : t("fiscalCustomer.errorCuit");

  return (
    <div className={`${palette.card} ${className}`}>
      <button
        type="button"
        onClick={() => setExpanded((v) => !v)}
        aria-expanded={expanded}
        className={`flex w-full items-center justify-between gap-2 px-4 py-3 text-sm font-medium ${palette.toggle}`}
      >
        <span className="flex items-center gap-2">
          <Receipt className="h-4 w-4" strokeWidth={1.75} />
          {t("fiscalCustomer.toggle")}
        </span>
        <ChevronDown
          className={`h-4 w-4 transition-transform ${expanded ? "rotate-180" : ""}`}
          strokeWidth={1.75}
        />
      </button>

      {expanded && (
        <div className="space-y-3 border-t border-inherit px-4 py-4">
          <p className={`text-xs ${palette.hint}`}>
            {t("fiscalCustomer.hint")}
          </p>

          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <label className="block">
              <span className={`mb-1 block text-xs font-medium ${palette.label}`}>
                {t("fiscalCustomer.docTypeLabel")}
              </span>
              <select
                aria-label={t("fiscalCustomer.docTypeLabel")}
                value={value.docType}
                disabled={readOnly}
                onChange={(e) =>
                  update({ docType: e.target.value as FiscalDocTypeChoice })
                }
                className={palette.field}
              >
                <option value="">{t("fiscalCustomer.docTypeNone")}</option>
                <option value="DNI">{t("fiscalCustomer.docTypeDni")}</option>
                <option value="CUIT">{t("fiscalCustomer.docTypeCuit")}</option>
                <option value="CUIL">{t("fiscalCustomer.docTypeCuil")}</option>
              </select>
            </label>

            <label className="block">
              <span className={`mb-1 block text-xs font-medium ${palette.label}`}>
                {t("fiscalCustomer.docNumberLabel")}
              </span>
              <input
                type="text"
                inputMode="numeric"
                autoComplete="off"
                aria-label={t("fiscalCustomer.docNumberLabel")}
                aria-invalid={docNumberError}
                aria-describedby={docNumberError ? docNumberErrorId : undefined}
                placeholder={t("fiscalCustomer.docNumberPlaceholder")}
                value={value.docNumber}
                disabled={readOnly}
                onChange={(e) => update({ docNumber: e.target.value })}
                className={`${palette.field} ${docNumberError ? palette.fieldError : ""}`}
              />
            </label>
          </div>

          {docNumberError && (
            <p
              id={docNumberErrorId}
              role="alert"
              className={`text-xs ${palette.error}`}
            >
              {docNumberErrorMsg}
            </p>
          )}

          <label className="block">
            <span className={`mb-1 block text-xs font-medium ${palette.label}`}>
              {t("fiscalCustomer.taxConditionLabel")}
            </span>
            <select
              aria-label={t("fiscalCustomer.taxConditionLabel")}
              aria-describedby={showCuitHint ? cuitHintId : undefined}
              value={value.taxCondition}
              disabled={readOnly}
              onChange={(e) =>
                update({
                  taxCondition: e.target.value as FiscalTaxConditionChoice,
                })
              }
              className={palette.field}
            >
              <option value="">{t("fiscalCustomer.taxConsumidorFinal")}</option>
              <option value="responsable_inscripto">
                {t("fiscalCustomer.taxResponsableInscripto")}
              </option>
              <option value="monotributo">
                {t("fiscalCustomer.taxMonotributo")}
              </option>
              <option value="exento">{t("fiscalCustomer.taxExento")}</option>
            </select>
          </label>

          {showCuitHint && (
            <p id={cuitHintId} className="text-xs text-amber-900" role="note">
              {t("fiscalCustomer.cuitHint")}
            </p>
          )}

          <label className="block">
            <span className={`mb-1 block text-xs font-medium ${palette.label}`}>
              {t("fiscalCustomer.nameLabel")}
            </span>
            <input
              type="text"
              autoComplete="off"
              aria-label={t("fiscalCustomer.nameLabel")}
              placeholder={t("fiscalCustomer.namePlaceholder")}
              value={value.name}
              disabled={readOnly}
              onChange={(e) => update({ name: e.target.value })}
              className={palette.field}
            />
          </label>

          <label className="block">
            <span className={`mb-1 block text-xs font-medium ${palette.label}`}>
              {t("fiscalCustomer.emailLabel")}
            </span>
            <input
              type="email"
              inputMode="email"
              autoComplete="email"
              aria-label={t("fiscalCustomer.emailLabel")}
              aria-invalid={problem === "email"}
              placeholder={t("fiscalCustomer.emailPlaceholder")}
              value={value.email}
              disabled={readOnly}
              onChange={(e) => update({ email: e.target.value })}
              className={`${palette.field} ${problem === "email" ? palette.fieldError : ""}`}
            />
            <span className={`mt-1 block text-xs ${palette.hint}`}>
              {t("fiscalCustomer.emailHint")}
            </span>
          </label>
          {problem === "email" && (
            <p className={`text-xs ${palette.error}`}>
              {t("fiscalCustomer.errorEmail")}
            </p>
          )}

          {actions ? (
            <div className="flex justify-end pt-1" data-testid="fiscal-identity-actions">
              {actions}
            </div>
          ) : null}
        </div>
      )}
    </div>
  );
}
