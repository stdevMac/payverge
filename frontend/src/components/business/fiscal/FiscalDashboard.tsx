"use client";

import {
  type ChangeEvent,
  type FormEvent,
  type ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  AlertTriangle,
  CheckCircle2,
  Clock3,
  Download,
  FileText,
  Fingerprint,
  Pencil,
  QrCode,
  ReceiptText,
  RefreshCw,
  RotateCcw,
  Search,
  Send,
  ShieldCheck,
  Upload,
  X,
  type LucideIcon,
} from "lucide-react";

import {
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from "@nextui-org/react";

import {
  creditFiscalReceipt,
  getSettings,
  issueReceipt,
  listReceiptDelivery,
  listReceipts,
  resendFiscalReceipt,
  retryDeliveryTask,
  retryReceipt,
  updateSettings,
  uploadFiscalCredentials,
  validateFiscalSettings,
  type FiscalDeliveryTask,
  type FiscalEnvironment,
  type FiscalMode,
  type FiscalReceipt,
  type FiscalSettings as FiscalSettingsRecord,
  type FiscalStatus,
  type UpdateFiscalSettingsPayload,
} from "@/api/fiscal";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import StatusBadge, {
  type StatusTone,
} from "@/components/business/accounting/StatusBadge";
import { downloadCsv, rowsToCsv } from "@/utils/csvExport";
import { intlLocaleFor } from "@/utils/intlLocale";
import {
  DATE_SHORT,
  formatBusinessDateTime,
} from "@/utils/businessTime";
import {
  getApiErrorCode,
  getApiErrorStatus,
  getSafeApiErrorMessage,
} from "@/utils/apiError";
import { TAX_CONDITIONS_BY_COUNTRY } from "./taxConditions";
import { displayReceiptType, receiptHistoryColumnKey } from "./receiptTypes";
import { shouldShowFiscalIdentityFields } from "@/lib/fiscalIdentityAvailability";

// Backend code (server.ErrCodeFiscalReceiptAlreadyIssued): this bill already has
// a live fiscal receipt, so a second issue would be a duplicate factura.
const FISCAL_RECEIPT_ALREADY_ISSUED = "fiscal_receipt_already_issued";

type FiscalTranslate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/**
 * Every fiscal write (issue / retry / re-send / credit note / validate /
 * credential upload) is gated server-side by a runtime containment control. While
 * that control is off the backend answers 503, and its global 5xx sanitizer has
 * already replaced the body with one constant English envelope — so there is no
 * per-action detail left to show and getSafeApiErrorMessage would print that
 * English sentence verbatim into a Spanish operator console (#906).
 *
 * Answer the status directly with localized copy carrying the two facts an
 * operator can act on: invoicing is switched off server-side, and nothing was
 * sent to the tax authority. Every other status keeps the existing behaviour, so
 * genuine backend detail (bill not paid, bad receiver, …) still surfaces.
 */
function fiscalActionErrorMessage(
  error: unknown,
  fallback: string,
  t: FiscalTranslate,
): string {
  if (getApiErrorStatus(error) === 503) {
    return t("fiscal.serviceUnavailable");
  }
  return getSafeApiErrorMessage(error, fallback);
}

interface FiscalDashboardProps {
  businessId: number;
  // canManageSensitive gates the owner-only fiscal controls — emitting a credit
  // note (tax reversal) and uploading the AFIP credential bundle. The backend
  // enforces these as owner-only (fiscal:credit / fiscal:credentials); this hides
  // the controls for non-owners so they don't see actions that would 403. Defaults
  // to true so existing callers (and owners) are unaffected.
  canManageSensitive?: boolean;
  // onReceiptsChanged fires after any mutation that alters the receipt set
  // (issue / retry / credit-note). The parent AccountingDashboard loads the
  // receipts once for its Invoices KPI + tab badge; without this callback those
  // never refreshed while the operator worked inside this panel. See R3-AC.
  onReceiptsChanged?: () => void;
  // Business IANA timezone so receipt issuance / credential expiry dates render
  // on the business calendar day, not the operator's device timezone. Null →
  // UTC fallback (stable/auditable), never the browser TZ.
  businessTimezone?: string | null;
  /**
   * "full" (default) — setup cards + invoice history table.
   * "setup-only" — provider/status/E-invoicing cards only; InvoicesTab owns the
   * paginated receipts list so this mode returns before invoice history.
   */
  variant?: "full" | "setup-only";
}

type SupportedCountry = "AR" | "AE";
type SupportedProvider = "arca" | "edicom";
type HistoryFilter = "all" | "authorized" | "pending" | "attention";

interface FiscalSettingsFormState {
  /**
   * The business's country as stored, which is NOT always one of
   * COUNTRY_OPTIONS — the demo/US businesses carry an unsupported code. Form
   * state has to be able to hold it verbatim (L6-22), otherwise entering edit
   * mode rewrites the country as a side effect of rendering the form.
   */
  country: string;
  /** Likewise: an unsupported country's stored provider (e.g. "demo"). */
  provider: string;
  mode: FiscalMode;
  environment: FiscalEnvironment;
  tax_id: string;
  tax_condition: string;
  point_of_sale: string;
}

interface CountryOption {
  value: SupportedCountry;
  provider: SupportedProvider;
  available: boolean;
}

const COUNTRY_OPTIONS: CountryOption[] = [
  { value: "AR", provider: "arca", available: true },
  { value: "AE", provider: "edicom", available: false },
];

const COUNTRY_PROVIDER: Record<SupportedCountry, SupportedProvider> = {
  AR: "arca",
  AE: "edicom",
};

// Countries/providers that carry a real e-invoicing integration with a
// certificate + private-key credential flow. The backend strictly allows only
// AR+arca / AE+edicom; a "demo" provider (or any unsupported country, e.g. the
// "US" demo business) has no AFIP/ARCA credential bundle, so we render a neutral
// state instead of the Argentina certificate-upload form.
const CREDENTIAL_PROVIDERS = new Set<string>(["arca", "edicom"]);

function hasCredentialFlow(
  settings: FiscalSettingsRecord | null | undefined,
): boolean {
  if (!settings) return false;
  return CREDENTIAL_PROVIDERS.has((settings.provider || "").toLowerCase());
}


interface ModeOption {
  value: FiscalMode;
  titleKey: string;
  descriptionKey: string;
}

const MODE_OPTIONS: ModeOption[] = [
  {
    value: "off",
    titleKey: "fiscal.modes.off.title",
    descriptionKey: "fiscal.modes.off.description",
  },
  {
    value: "manual",
    titleKey: "fiscal.modes.manual.title",
    descriptionKey: "fiscal.modes.manual.description",
  },
  {
    value: "automatic_non_blocking",
    titleKey: "fiscal.modes.automatic_non_blocking.title",
    descriptionKey: "fiscal.modes.automatic_non_blocking.description",
  },
];

const ENVIRONMENT_OPTIONS: FiscalEnvironment[] = ["sandbox", "production"];

const fieldClass =
  "mt-1 h-10 w-full rounded-xl border border-warm-200 bg-white/90 px-3 text-sm text-ink-900 shadow-sm outline-none transition-colors focus:border-brand focus:ring-2 focus:ring-brand/15 disabled:bg-warm-100";
const labelClass =
  "text-xs font-semibold uppercase tracking-[0.16em] text-ink-500";
const fiscalPanelClass =
  "overflow-hidden rounded-2xl border border-warm-200/90 bg-white/90 shadow-card";
const fiscalPanelHeaderClass =
  "flex flex-wrap items-start justify-between gap-3 border-b border-warm-200/80 bg-gradient-to-r from-warm-50/85 via-white to-brand/5 px-5 py-4";
const fiscalPanelDescriptionClass = "mt-1 text-sm leading-6 text-ink-600";
const fiscalPrimaryButtonClass =
  "inline-flex h-10 items-center justify-center gap-2 rounded-xl bg-brand px-4 text-sm font-semibold text-white shadow-cta-glow transition-all duration-200 hover:-translate-y-0.5 hover:bg-brand-dark disabled:cursor-not-allowed disabled:opacity-60";
const fiscalSecondaryButtonClass =
  "inline-flex h-10 items-center justify-center gap-1.5 rounded-xl border border-warm-200/90 bg-white/85 px-3 text-sm font-medium text-ink-700 shadow-sm shadow-warm-900/5 transition-all duration-200 hover:-translate-y-0.5 hover:border-brand/25 hover:bg-brand/5 hover:text-brand-700 disabled:cursor-not-allowed disabled:opacity-50";
const fiscalSmallSecondaryButtonClass =
  "inline-flex h-8 items-center justify-center gap-1.5 rounded-lg border border-warm-200/90 bg-white/85 px-3 text-sm font-medium text-ink-700 shadow-sm shadow-warm-900/5 transition-colors hover:border-brand/25 hover:bg-brand/5 hover:text-brand-700 disabled:cursor-not-allowed disabled:opacity-60";
const fiscalTableClass = "min-w-full divide-y divide-warm-200/80 text-sm";
const fiscalTableHeadClass =
  "bg-warm-50/75 text-left text-xs font-semibold uppercase tracking-[0.16em] text-ink-500";
const fiscalTableBodyClass = "divide-y divide-warm-100";
const fiscalTableRowClass = "transition-colors hover:bg-brand/5";

function parsePositiveInteger(value: string): number | null {
  const trimmed = value.trim();
  if (!/^[1-9]\d*$/.test(trimmed)) return null;
  const parsed = Number(trimmed);
  return Number.isSafeInteger(parsed) ? parsed : null;
}

function parsePointOfSale(value: string): number | null {
  const trimmed = value.trim();
  if (!trimmed) return null;
  return parsePositiveInteger(trimmed);
}

// Emitter CUIT gate: ARCA rejects any tax_id that is not exactly 11 digits,
// but only at WSFE call time with an opaque provider error. Validate at save.
// Separators (hyphens/dots) are tolerated — only digits are counted, matching
// the backend mapper. Empty stays valid so a draft can be saved incrementally.
export function isValidEmitterTaxId(taxId: string, provider: string): boolean {
  if (provider !== "arca") return true;
  const trimmed = taxId.trim();
  if (trimmed === "") return true;
  return trimmed.replace(/\D/g, "").length === 11;
}

function formatDataLabel(value: string | null | undefined): string {
  if (!value) return "";
  return value
    .split("_")
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(" ");
}

/** Returns a supported country, or null when outside COUNTRY_OPTIONS (L6-22). */
function parseSupportedCountry(
  value: string | null | undefined,
): SupportedCountry | null {
  if (value === "AR" || value === "AE") return value;
  return null;
}

/** Country select change only — options are already supported. */
function normalizeCountry(value: string | null | undefined): SupportedCountry {
  return parseSupportedCountry(value) ?? "AR";
}

function createFormState(
  settings: FiscalSettingsRecord | null,
): FiscalSettingsFormState {
  // L6-22 (edit path): the saved country is carried through verbatim. Coercing
  // an unsupported code to "AR" here meant that merely opening "Edit setup" and
  // pressing save — without ever touching the country picker — rewrote the
  // business to Argentina/ARCA. Only handleCountryChange may move the country.
  // "AR" is the default for a business that has no settings row yet.
  const savedCountry = (settings?.country || "").trim().toUpperCase();
  const country = savedCountry || "AR";
  const supported = parseSupportedCountry(country);
  const provider = supported
    ? COUNTRY_PROVIDER[supported]
    : (settings?.provider || "").trim().toLowerCase();
  const pointOfSale = settings?.point_of_sale;
  // Demo provider cannot honestly be "production" — keep the form on sandbox.
  const environment: FiscalEnvironment =
    provider === "demo"
      ? "sandbox"
      : settings?.environment || "sandbox";
  return {
    country,
    provider,
    mode: settings?.mode || "manual",
    environment,
    tax_id: settings?.tax_id || "",
    tax_condition: settings?.tax_condition || "",
    point_of_sale:
      pointOfSale === null || pointOfSale === undefined
        ? ""
        : String(pointOfSale),
  };
}

function isFailureStatus(status: FiscalStatus): boolean {
  return (
    status === "failed_retryable" ||
    status === "failed_permanent" ||
    status === "rejected" ||
    status === "cancelled"
  );
}

function isRetryableStatus(status: FiscalStatus): boolean {
  return status === "failed_retryable";
}

// An authorized receipt carries a deliverable PDF/QR, so the operator can
// re-send / re-print it (forcing delivery even if it was already delivered).
function isResendableStatus(status: FiscalStatus): boolean {
  return status === "authorized";
}

// A receipt can be credited only when it carries a live AFIP authorization
// (status "authorized") and is an original issue/debit document — never an
// already-credited receipt or a credit note itself.
function isCreditableReceipt(receipt: FiscalReceipt): boolean {
  return receipt.status === "authorized" && receipt.action !== "credit_note";
}

interface SetupStatusVisual {
  tone: StatusTone;
  Icon: LucideIcon;
}

function setupStatusVisual(status: string): SetupStatusVisual {
  if (status === "validated" || status === "ready")
    return { tone: "success", Icon: ShieldCheck };
  if (status === "credentials_set") return { tone: "pending", Icon: Clock3 };
  return { tone: "warning", Icon: AlertTriangle };
}

function formatExpiryDate(
  value: string | null | undefined,
  locale: string,
  businessTimezone: string | null = null,
) {
  if (!value) return null;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return null;
  return formatBusinessDateTime(value, locale, businessTimezone, DATE_SHORT);
}

interface StatusVisual {
  tone: StatusTone;
  Icon: LucideIcon;
}

function statusVisual(status: FiscalStatus): StatusVisual {
  if (status === "authorized" || status === "credited") {
    return { tone: "success", Icon: CheckCircle2 };
  }
  if (status === "pending") {
    return { tone: "pending", Icon: Clock3 };
  }
  if (status === "failed_retryable") {
    return { tone: "warning", Icon: AlertTriangle };
  }
  if (isFailureStatus(status)) {
    return { tone: "danger", Icon: AlertTriangle };
  }
  return { tone: "neutral", Icon: FileText };
}

function formatMoney(cents: number, currency: string, locale: string): string {
  const amount = cents / 100;
  const normalized = currency.trim().toUpperCase() || "USD";
  try {
    return new Intl.NumberFormat(intlLocaleFor(locale), {
      style: "currency",
      currency: normalized,
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(amount);
  } catch {
    return `${normalized} ${amount.toFixed(2)}`;
  }
}

function formatIssuedDate(
  value: string | null,
  locale: string,
  businessTimezone: string | null = null,
): string | null {
  if (!value) return null;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return null;
  return formatBusinessDateTime(value, locale, businessTimezone, DATE_SHORT);
}

function formatRelativeTime(
  value: string | null,
  t: (key: string, params?: Record<string, string | number>) => string,
): string | null {
  if (!value) return null;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return null;
  const diffMs = Date.now() - date.getTime();
  const diffMinutes = Math.max(0, Math.round(diffMs / 60000));
  if (diffMinutes < 1) return t("fiscal.status.relativeNow");
  if (diffMinutes < 60)
    return t("fiscal.status.relativeMinutes", { n: diffMinutes });
  const diffHours = Math.round(diffMinutes / 60);
  if (diffHours < 24) return t("fiscal.status.relativeHours", { n: diffHours });
  const diffDays = Math.round(diffHours / 24);
  return t("fiscal.status.relativeDays", { n: diffDays });
}

export default function FiscalDashboard({
  businessId,
  canManageSensitive = true,
  onReceiptsChanged,
  businessTimezone = null,
  variant = "full",
}: FiscalDashboardProps) {
  const setupOnly = variant === "setup-only";
  const { locale } = useSimpleLocale();
  const [settings, setSettings] = useState<FiscalSettingsRecord | null>(null);
  const [receipts, setReceipts] = useState<FiscalReceipt[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [form, setForm] = useState<FiscalSettingsFormState>(() =>
    createFormState(null),
  );
  const [saving, setSaving] = useState(false);
  const [saveMessage, setSaveMessage] = useState<string | null>(null);
  const [saveError, setSaveError] = useState<string | null>(null);
  const [billID, setBillID] = useState("");
  const [issuing, setIssuing] = useState(false);
  const [retryingReceiptID, setRetryingReceiptID] = useState<number | null>(
    null,
  );
  const [resendingReceiptID, setResendingReceiptID] = useState<number | null>(
    null,
  );
  const [actionMessage, setActionMessage] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [historyFilter, setHistoryFilter] = useState<HistoryFilter>("all");
  const [billSearch, setBillSearch] = useState("");
  const [editing, setEditing] = useState(false);
  const loadRequestRef = useRef(0);

  // Credentials (T26)
  const [certFile, setCertFile] = useState<File | null>(null);
  const [keyFile, setKeyFile] = useState<File | null>(null);
  const [uploading, setUploading] = useState(false);
  const [validating, setValidating] = useState(false);
  const [credentialsMessage, setCredentialsMessage] = useState<string | null>(
    null,
  );
  const [credentialsError, setCredentialsError] = useState<string | null>(null);

  // Credit note (T27)
  const [creditTarget, setCreditTarget] = useState<FiscalReceipt | null>(null);
  const [creditReason, setCreditReason] = useState("");
  const [crediting, setCrediting] = useState(false);

  const t = useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      const value = getTranslation(key, locale, params);
      if (Array.isArray(value)) return value[0] || key;
      return value;
    },
    [locale],
  );

  const tLabel = useCallback(
    (value: string | null | undefined) => {
      if (!value) return t("fiscal.common.notSet");
      const key = `fiscal.labels.${value}`;
      const translated = t(key);
      return translated === key ? formatDataLabel(value) : translated;
    },
    [t],
  );

  const tTaxCondition = useCallback(
    (country: SupportedCountry, value: string | null | undefined): string => {
      if (!value) return "";
      const key = `fiscal.setup.taxConditions.${country}.${value}`;
      const translated = t(key);
      return translated === key ? formatDataLabel(value) : translated;
    },
    [t],
  );

  const loadFiscalData = useCallback(
    async (showLoading = true) => {
      const requestID = loadRequestRef.current + 1;
      loadRequestRef.current = requestID;

      if (showLoading) setLoading(true);
      else setRefreshing(true);
      setError(null);

      try {
        if (setupOnly) {
          // InvoicesTab owns the paginated receipts list — only load settings
          // for the provider / credentials / mode cards.
          const nextSettings = await getSettings(businessId);
          if (loadRequestRef.current === requestID) {
            setSettings(nextSettings);
            setReceipts([]);
            if (!showLoading) {
              onReceiptsChanged?.();
            }
          }
        } else {
          const [settingsResult, receiptsResult] = await Promise.allSettled([
            getSettings(businessId),
            listReceipts(businessId),
          ]);
          if (loadRequestRef.current === requestID) {
            let loadError: unknown = null;
            if (settingsResult.status === "fulfilled") {
              setSettings(settingsResult.value);
            } else {
              loadError = settingsResult.reason;
            }
            if (receiptsResult.status === "fulfilled") {
              setReceipts(receiptsResult.value);
            } else if (!loadError) {
              loadError = receiptsResult.reason;
            }
            if (loadError) {
              // FIND-058: never surface axios transport / gin dumps in fiscal UI.
              setError(
                getSafeApiErrorMessage(
                  loadError,
                  t("fiscal.dashboard.unexpectedError"),
                ),
              );
            }
            // A background refresh (showLoading === false) only happens after a
            // receipt-mutating action (issue/retry/resend/credit/credentials).
            // Notify the parent so its Invoices KPI + tab badge re-fetch. The
            // initial mount uses showLoading === true and is skipped. See R3-AC.
            if (!showLoading && receiptsResult.status === "fulfilled") {
              onReceiptsChanged?.();
            }
          }
        }
      } catch (loadError) {
        if (loadRequestRef.current === requestID) {
          // FIND-058: never surface axios transport / gin dumps in fiscal UI.
          setError(
            getSafeApiErrorMessage(
              loadError,
              t("fiscal.dashboard.unexpectedError"),
            ),
          );
        }
      } finally {
        if (loadRequestRef.current === requestID) {
          setLoading(false);
          setRefreshing(false);
        }
      }
    },
    [businessId, t, onReceiptsChanged, setupOnly],
  );

  useEffect(() => {
    void loadFiscalData(true);
  }, [loadFiscalData]);

  useEffect(() => {
    setForm(createFormState(settings));
  }, [settings]);

  useEffect(() => {
    if (!saveMessage) return undefined;
    const timer = window.setTimeout(() => setSaveMessage(null), 4000);
    return () => window.clearTimeout(timer);
  }, [saveMessage]);

  useEffect(() => {
    if (!actionMessage) return undefined;
    const timer = window.setTimeout(() => setActionMessage(null), 4000);
    return () => window.clearTimeout(timer);
  }, [actionMessage]);

  useEffect(() => {
    if (!credentialsMessage) return undefined;
    const timer = window.setTimeout(() => setCredentialsMessage(null), 4000);
    return () => window.clearTimeout(timer);
  }, [credentialsMessage]);

  const lastIssuedReceipt = useMemo(
    () => receipts.find((r) => r.issued_at) || null,
    [receipts],
  );
  const lastIssuedRelative = useMemo(
    () => formatRelativeTime(lastIssuedReceipt?.issued_at || null, t),
    [lastIssuedReceipt, t],
  );

  const pointOfSaleTrimmed = form.point_of_sale.trim();
  const pointOfSaleIsValid =
    pointOfSaleTrimmed === "" || parsePointOfSale(pointOfSaleTrimmed) !== null;
  const taxIdIsValid = isValidEmitterTaxId(form.tax_id, form.provider);
  const parsedBillID = parsePositiveInteger(billID);
  const canIssue = parsedBillID !== null;
  const canManualIssue = Boolean(settings) && settings?.mode !== "off";
  const showSetupForm = !settings || editing;
  const isSandbox = settings?.environment === "sandbox";
  const isDemoProvider =
    (settings?.provider || "").toLowerCase() === "demo";
  // Production environment is only meaningful for a real e-invoicing provider
  // (e.g. ARCA). Demo tenants cannot "go live" by flipping a select.
  const canSwitchToProduction = Boolean(settings) && !isDemoProvider;
  const setupStatus = settings?.setup_status || "draft";
  const hasCredentials = Boolean(settings?.credentials_fingerprint);
  const canValidate = hasCredentials && !validating && !uploading;

  const filteredReceipts = useMemo(() => {
    let result = receipts;
    if (historyFilter !== "all") {
      result = result.filter((r) => {
        if (historyFilter === "authorized")
          return r.status === "authorized" || r.status === "credited";
        if (historyFilter === "pending") return r.status === "pending";
        if (historyFilter === "attention") return isFailureStatus(r.status);
        return true;
      });
    }
    const trimmedSearch = billSearch.trim();
    if (trimmedSearch) {
      const needle = trimmedSearch.toLowerCase();
      result = result.filter(
        (r) =>
          String(r.bill_id).includes(needle) ||
          (r.receipt_number || "").toLowerCase().includes(needle),
      );
    }
    return result;
  }, [receipts, historyFilter, billSearch]);

  const counts = useMemo(
    () => ({
      all: receipts.length,
      authorized: receipts.filter(
        (r) => r.status === "authorized" || r.status === "credited",
      ).length,
      pending: receipts.filter((r) => r.status === "pending").length,
      attention: receipts.filter((r) => isFailureStatus(r.status)).length,
    }),
    [receipts],
  );

  const handleCountryChange = (event: ChangeEvent<HTMLSelectElement>) => {
    const country = normalizeCountry(event.target.value);
    setForm((current) => ({
      ...current,
      country,
      provider: COUNTRY_PROVIDER[country],
      tax_condition: "",
    }));
  };

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!pointOfSaleIsValid) {
      setSaveError(t("fiscal.setup.invalidPointOfSale"));
      setSaveMessage(null);
      return;
    }
    if (!taxIdIsValid) {
      setSaveError(t("fiscal.setup.invalidTaxId"));
      setSaveMessage(null);
      return;
    }

    setSaving(true);
    setSaveError(null);
    setSaveMessage(null);

    const payload: UpdateFiscalSettingsPayload = {
      country: form.country,
      provider: form.provider,
      mode: form.mode,
      environment: form.environment,
      tax_id: form.tax_id.trim(),
      tax_condition: form.tax_condition.trim(),
      point_of_sale: parsePointOfSale(form.point_of_sale),
    };

    try {
      await updateSettings(businessId, payload);
      setSaveMessage(t("fiscal.setup.saveSuccess"));
      setEditing(false);
      await loadFiscalData(false);
    } catch (saveErrorValue) {
      setSaveError(
        getSafeApiErrorMessage(saveErrorValue, t("fiscal.setup.saveError")),
      );
    } finally {
      setSaving(false);
    }
  };

  const handleIssue = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!canIssue) return;

    setIssuing(true);
    setActionError(null);
    setActionMessage(null);

    try {
      await issueReceipt(businessId, parsedBillID);
      setBillID("");
      setActionMessage(t("fiscal.history.issue.success"));
      await loadFiscalData(false);
    } catch (issueError) {
      const status = (issueError as { status?: number } | null)?.status;
      const rawMessage = issueError instanceof Error ? issueError.message : "";
      if (
        getApiErrorCode(issueError) === FISCAL_RECEIPT_ALREADY_ISSUED ||
        getApiErrorStatus(issueError) === 409
      ) {
        // The bill already carries a live factura. Manual issue by bill ID
        // bypasses the picker (which marks these rows), and the backend used to
        // swallow the duplicate and answer 202 — so the host was told an invoice
        // was queued that never was (#907). Say what actually happened.
        setActionError(t("fiscal.history.issue.alreadyIssued"));
      } else if (status === 404 || /not found/i.test(rawMessage)) {
        setActionError(t("fiscal.history.issue.billNotFound"));
      } else {
        setActionError(
          fiscalActionErrorMessage(
            issueError,
            t("fiscal.history.issue.error"),
            t,
          ),
        );
      }
    } finally {
      setIssuing(false);
    }
  };

  const handleRetry = async (receiptID: number) => {
    setRetryingReceiptID(receiptID);
    setActionError(null);
    setActionMessage(null);

    try {
      await retryReceipt(businessId, receiptID);
      setActionMessage(t("fiscal.history.retry.success"));
      await loadFiscalData(false);
    } catch (retryError) {
      setActionError(
        fiscalActionErrorMessage(
          retryError,
          t("fiscal.history.retry.error"),
          t,
        ),
      );
    } finally {
      setRetryingReceiptID(null);
    }
  };

  const handleResend = async (receiptID: number) => {
    setResendingReceiptID(receiptID);
    setActionError(null);
    setActionMessage(null);

    try {
      await resendFiscalReceipt(businessId, receiptID);
      setActionMessage(t("fiscal.history.resend.success"));
      await loadFiscalData(false);
    } catch (resendError) {
      setActionError(
        fiscalActionErrorMessage(
          resendError,
          t("fiscal.history.resend.error"),
          t,
        ),
      );
    } finally {
      setResendingReceiptID(null);
    }
  };

  const handleUploadCredentials = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!certFile || !keyFile) {
      setCredentialsError(t("fiscal.credentials.missingFiles"));
      setCredentialsMessage(null);
      return;
    }

    setUploading(true);
    setCredentialsError(null);
    setCredentialsMessage(null);

    try {
      await uploadFiscalCredentials(businessId, certFile, keyFile);
      // The private key is never echoed; clear the inputs immediately so the
      // selected key file never lingers in component state longer than needed.
      setCertFile(null);
      setKeyFile(null);
      setCredentialsMessage(t("fiscal.credentials.uploadSuccess"));
      await loadFiscalData(false);
    } catch (uploadError) {
      setCredentialsError(
        fiscalActionErrorMessage(
          uploadError,
          t("fiscal.credentials.uploadError"),
          t,
        ),
      );
    } finally {
      setUploading(false);
    }
  };

  const handleValidate = async () => {
    if (!hasCredentials) {
      setCredentialsError(t("fiscal.credentials.validateNeedsCredentials"));
      return;
    }
    setValidating(true);
    setCredentialsError(null);
    setCredentialsMessage(null);

    try {
      const updated = await validateFiscalSettings(businessId);
      setSettings(updated);
      if (updated.setup_status === "ready") {
        setCredentialsMessage(t("fiscal.credentials.validateSuccess"));
      }
      // A validation failure (setup_status not "ready") returns 200 with
      // last_validation_error populated; that error renders inline from settings.
    } catch (validateError) {
      setCredentialsError(
        fiscalActionErrorMessage(
          validateError,
          t("fiscal.credentials.validateError"),
          t,
        ),
      );
    } finally {
      setValidating(false);
    }
  };

  const handleConfirmCredit = async () => {
    if (!creditTarget) return;
    const receiptID = creditTarget.id;
    setCrediting(true);
    setActionError(null);
    setActionMessage(null);

    try {
      await creditFiscalReceipt(businessId, receiptID, creditReason);
      setCreditTarget(null);
      setCreditReason("");
      setActionMessage(t("fiscal.history.creditNote.success"));
      await loadFiscalData(false);
    } catch (creditError) {
      setActionError(
        fiscalActionErrorMessage(
          creditError,
          t("fiscal.history.creditNote.error"),
          t,
        ),
      );
    } finally {
      setCrediting(false);
    }
  };

  const exportInvoicesCsv = () => {
    const filename = `${t("fiscal.history.csvFile")}-${new Date()
      .toISOString()
      .slice(0, 10)}.csv`;
    const csv = rowsToCsv(
      [
        { key: "bill_id", header: t("fiscal.history.columns.billId") },
        {
          key: "receipt_type",
          header: t(receiptHistoryColumnKey(settings?.country)),
        },
        {
          key: "receipt_number",
          header: t("fiscal.history.columns.receiptNumber"),
        },
        { key: "amount", header: t("fiscal.history.columns.amount") },
        { key: "currency", header: t("fiscal.history.columns.currency") },
        { key: "status", header: t("fiscal.history.columns.status") },
        { key: "issued_at", header: t("fiscal.history.columns.issuedAt") },
        { key: "error", header: t("fiscal.history.columns.error") },
      ],
      filteredReceipts.map((r) => ({
        bill_id: r.bill_id,
        receipt_type: tLabel(
          displayReceiptType(settings?.country, r.receipt_type),
        ),
        receipt_number: r.receipt_number || "",
        amount: (r.total_amount_cents / 100).toFixed(2),
        currency: r.currency,
        status: tLabel(r.status),
        issued_at: r.issued_at || "",
        error: r.error_message || "",
      })),
    );
    downloadCsv(filename, csv);
  };

  if (loading) {
    return (
      <section className="space-y-4" aria-busy="true">
        <div className="h-12 animate-pulse rounded-2xl border border-warm-200/80 bg-white/80 shadow-sm shadow-warm-900/5" />
        <div className="h-64 animate-pulse rounded-2xl border border-warm-200/80 bg-white/80 shadow-sm shadow-warm-900/5" />
      </section>
    );
  }

  const errorBanner = error ? (
    <div
      role="alert"
      className="flex items-start gap-3 rounded-2xl border border-rose-200 bg-rose-50 p-4 text-sm text-rose-700 shadow-sm shadow-rose-900/5"
    >
      <AlertTriangle className="mt-0.5 h-5 w-5 flex-shrink-0" />
      <div className="flex-1">
        <div className="font-semibold text-rose-800">
          {t("fiscal.dashboard.errorTitle")}
        </div>
        <div className="mt-1 leading-6">{error}</div>
        <button
          className="mt-3 inline-flex h-9 items-center gap-2 rounded-xl border border-rose-200 bg-white px-3 text-sm font-medium text-rose-700 transition-colors hover:bg-rose-100"
          type="button"
          onClick={() => void loadFiscalData(true)}
        >
          <RefreshCw className="h-4 w-4" /> {t("fiscal.dashboard.tryAgain")}
        </button>
      </div>
    </div>
  ) : null;

  return (
    <section className="space-y-4">
      {errorBanner}
      {settings && (isSandbox || isDemoProvider) ? (
        <div
          className="rounded-2xl border border-amber-200 bg-amber-50/90 px-4 py-3 text-xs leading-5 text-amber-800 shadow-sm shadow-amber-900/5"
          data-testid="fiscal-sandbox-banner"
          data-demo-provider={isDemoProvider ? "true" : "false"}
        >
          <span className="font-medium">
            {isDemoProvider
              ? t("fiscal.summary.demoBannerPrefix")
              : t("fiscal.summary.sandboxBannerPrefix")}{" "}
            ·{" "}
          </span>
          {isDemoProvider
            ? t("fiscal.summary.demoBanner")
            : t("fiscal.summary.sandboxBanner")}
        </div>
      ) : null}

      {settings ? (
        <SummaryBar
          settings={settings}
          lastIssuedRelative={lastIssuedRelative}
          refreshing={refreshing}
          onRefresh={() => void loadFiscalData(false)}
          editing={editing}
          onToggleEdit={() => setEditing((value) => !value)}
          tLabel={tLabel}
          t={t}
        />
      ) : null}

      {saveMessage && !editing ? (
        <Banner tone="emerald" Icon={CheckCircle2} role="status">
          {saveMessage}
        </Banner>
      ) : null}

      {showSetupForm ? (
        <SetupForm
          form={form}
          setForm={setForm}
          onSubmit={handleSubmit}
          onCountryChange={handleCountryChange}
          canSwitchToProduction={canSwitchToProduction}
          saving={saving}
          saveError={saveError}
          pointOfSaleIsValid={pointOfSaleIsValid}
          taxIdIsValid={taxIdIsValid}
          settings={settings}
          editing={editing}
          onCancelEdit={() => {
            setEditing(false);
            setForm(createFormState(settings));
            setSaveError(null);
            setSaveMessage(null);
          }}
          tLabel={tLabel}
          tTaxCondition={tTaxCondition}
          t={t}
        />
      ) : null}

      {settings && canManageSensitive && !hasCredentialFlow(settings) ? (
        <UnsupportedRegionCard settings={settings} t={t} />
      ) : null}

      {settings && canManageSensitive && hasCredentialFlow(settings) ? (
        <CredentialsCard
          settings={settings}
          businessTimezone={businessTimezone}
          setupStatus={setupStatus}
          hasCredentials={hasCredentials}
          certFile={certFile}
          keyFile={keyFile}
          onCertChange={setCertFile}
          onKeyChange={setKeyFile}
          onUpload={handleUploadCredentials}
          uploading={uploading}
          onValidate={handleValidate}
          validating={validating}
          canValidate={canValidate}
          credentialsMessage={credentialsMessage}
          credentialsError={credentialsError}
          tLabel={tLabel}
          t={t}
          locale={locale}
        />
      ) : null}

      {settings && !setupOnly ? (
        <HistoryCard
          businessId={businessId}
          businessTimezone={businessTimezone}
          fiscalCountry={settings.country}
          receipts={receipts}
          filteredReceipts={filteredReceipts}
          historyFilter={historyFilter}
          onHistoryFilter={setHistoryFilter}
          billSearch={billSearch}
          onBillSearch={setBillSearch}
          counts={counts}
          billID={billID}
          setBillID={setBillID}
          onIssue={handleIssue}
          issuing={issuing}
          canIssue={canIssue}
          canManualIssue={canManualIssue}
          mode={settings.mode}
          actionError={actionError}
          actionMessage={actionMessage}
          retryingReceiptID={retryingReceiptID}
          onRetry={handleRetry}
          resendingReceiptID={resendingReceiptID}
          onResend={handleResend}
          onCredit={
            canManageSensitive
              ? (receipt) => {
                  setCreditTarget(receipt);
                  setCreditReason("");
                  setActionError(null);
                }
              : undefined
          }
          onExportCsv={exportInvoicesCsv}
          tLabel={tLabel}
          t={t}
          locale={locale}
        />
      ) : null}

      {creditTarget && !setupOnly ? (
        <CreditNoteDialog
          receipt={creditTarget}
          reason={creditReason}
          onReasonChange={setCreditReason}
          crediting={crediting}
          onConfirm={handleConfirmCredit}
          onCancel={() => {
            if (crediting) return;
            setCreditTarget(null);
            setCreditReason("");
          }}
          t={t}
        />
      ) : null}
    </section>
  );
}

/* -------------------------------------------------------------------------- */
/*                                Setup form                                  */
/* -------------------------------------------------------------------------- */

interface SetupFormProps {
  form: FiscalSettingsFormState;
  setForm: React.Dispatch<React.SetStateAction<FiscalSettingsFormState>>;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onCountryChange: (event: ChangeEvent<HTMLSelectElement>) => void;
  /** False for demo providers — Production is not a real go-live switch. */
  canSwitchToProduction: boolean;
  saving: boolean;
  saveError: string | null;
  pointOfSaleIsValid: boolean;
  taxIdIsValid: boolean;
  settings: FiscalSettingsRecord | null;
  editing: boolean;
  onCancelEdit: () => void;
  tLabel: (value: string | null | undefined) => string;
  tTaxCondition: (
    country: SupportedCountry,
    value: string | null | undefined,
  ) => string;
  t: (key: string, params?: Record<string, string | number>) => string;
}

function SetupForm({
  form,
  setForm,
  onSubmit,
  onCountryChange,
  canSwitchToProduction,
  saving,
  saveError,
  pointOfSaleIsValid,
  taxIdIsValid,
  settings,
  editing,
  onCancelEdit,
  tLabel,
  tTaxCondition,
  t,
}: SetupFormProps) {
  const selectedMode = MODE_OPTIONS.find(
    (m) => m.value === form.mode,
  );
  const heading = settings
    ? t("fiscal.setup.title")
    : t("fiscal.status.titleSetup");
  // Null for a business whose stored country is outside COUNTRY_OPTIONS: it has
  // no tax-condition vocabulary, and its code has to be offered as its own
  // (unselectable) option so the picker shows the truth instead of falling back
  // to whatever the browser picks for an unmatched value (L6-22).
  const supportedCountry = parseSupportedCountry(form.country);
  const taxConditions = supportedCountry
    ? TAX_CONDITIONS_BY_COUNTRY[supportedCountry] || []
    : [];

  return (
    <form
      className={fiscalPanelClass}
      onSubmit={onSubmit}
      aria-labelledby="invoice-setup-heading"
    >
      <div className={fiscalPanelHeaderClass}>
        <div className="min-w-0">
          <h2 id="invoice-setup-heading" className="text-heading-sm font-semibold text-ink-950">
            {heading}
          </h2>
          <p className={fiscalPanelDescriptionClass}>
            {settings
              ? t("fiscal.setup.description")
              : t("fiscal.setup.emptyDescription")}
          </p>
        </div>
      </div>

      <div className="space-y-5 px-5 py-5">
        <div className="grid gap-4 sm:grid-cols-[minmax(0,1fr)_auto]">
          <label className="block">
            <span className={labelClass}>
              {t("fiscal.setup.fields.country")}
            </span>
            <select
              className={fieldClass}
              value={form.country}
              onChange={onCountryChange}
            >
              {supportedCountry ? null : (
                <option value={form.country} disabled>
                  {tLabel(form.country)}
                </option>
              )}
              {COUNTRY_OPTIONS.map((option) => (
                <option
                  key={option.value}
                  value={option.value}
                  disabled={!option.available}
                >
                  {tLabel(option.value)}
                  {!option.available
                    ? ` — ${t("fiscal.setup.comingSoon")}`
                    : ""}
                </option>
              ))}
            </select>
          </label>
          <div className="flex flex-col justify-end">
            <span className={labelClass}>
              {t("fiscal.setup.fields.provider")}
            </span>
            <div className="mt-1 inline-flex h-10 items-center rounded-xl border border-warm-200 bg-warm-50/80 px-3 text-sm font-semibold text-ink-700 shadow-sm shadow-warm-900/5">
              {tLabel(form.provider)}
            </div>
          </div>
        </div>

        {shouldShowFiscalIdentityFields({
          fiscalCountry: form.country,
        }) ? (
        <div
          className={`grid gap-3 ${
            supportedCountry === "AR" ? "md:grid-cols-3" : "md:grid-cols-1"
          }`}
        >
          <label className="block">
            <span className={labelClass}>{t("fiscal.setup.fields.taxId")}</span>
            <input
              className={fieldClass}
              placeholder={t("fiscal.setup.placeholders.taxId")}
              value={form.tax_id}
              aria-invalid={!taxIdIsValid}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  tax_id: event.target.value,
                }))
              }
            />
            {!taxIdIsValid ? (
              <p className="mt-1 text-xs text-amber-700">
                {t("fiscal.setup.invalidTaxId")}
              </p>
            ) : null}
          </label>
          {supportedCountry === "AR" || supportedCountry === "AE" ? (
            <label className="block">
              <span className={labelClass}>
                {t("fiscal.setup.fields.taxCondition")}
              </span>
              <select
                className={fieldClass}
                value={form.tax_condition}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    tax_condition: event.target.value,
                  }))
                }
              >
                <option value="">
                  {t("fiscal.setup.fields.taxConditionPlaceholder")}
                </option>
                {taxConditions.map((condition) => (
                  <option key={condition} value={condition}>
                    {tTaxCondition(supportedCountry, condition)}
                  </option>
                ))}
              </select>
            </label>
          ) : null}
          {supportedCountry === "AR" ? (
            <div>
              <label className="block">
                <span className={labelClass}>
                  {t("fiscal.setup.fields.pointOfSale")}
                </span>
                <input
                  aria-invalid={!pointOfSaleIsValid}
                  aria-describedby="point-of-sale-help"
                  className={`${fieldClass} ${
                    !pointOfSaleIsValid
                      ? "border-amber-400 focus:border-amber-500 focus:ring-amber-200"
                      : ""
                  }`}
                  inputMode="numeric"
                  pattern="[0-9]*"
                  placeholder={t("fiscal.setup.placeholders.pointOfSale")}
                  type="text"
                  value={form.point_of_sale}
                  onChange={(event) =>
                    setForm((current) => ({
                      ...current,
                      point_of_sale: event.target.value,
                    }))
                  }
                />
              </label>
              <span
                id="point-of-sale-help"
                className={`mt-1 block text-xs ${
                  !pointOfSaleIsValid ? "text-amber-700" : "text-ink-500"
                }`}
              >
                {!pointOfSaleIsValid
                  ? t("fiscal.setup.invalidPointOfSale")
                  : t("fiscal.setup.fields.pointOfSaleHelp")}
              </span>
            </div>
          ) : null}
        </div>
        ) : null}

        <div className="grid gap-3 md:grid-cols-2">
          <div>
            <label className="block">
              <span className={labelClass}>
                {t("fiscal.setup.fields.mode")}
              </span>
              <select
                className={fieldClass}
                value={form.mode}
                onChange={(event) =>
                  setForm((current) => ({
                    ...current,
                    mode: event.target.value as FiscalMode,
                  }))
                }
              >
                {MODE_OPTIONS.map((option) => (
                  <option key={option.value} value={option.value}>
                    {t(option.titleKey)}
                  </option>
                ))}
              </select>
            </label>
            {selectedMode ? (
              <span className="mt-1 block text-xs leading-5 text-ink-500">
                {t(selectedMode.descriptionKey)}
              </span>
            ) : null}
          </div>
          <label className="block">
            <span className={labelClass}>
              {t("fiscal.setup.fields.environment")}
            </span>
            <select
              className={fieldClass}
              data-testid="fiscal-environment-select"
              aria-label={t("fiscal.setup.fields.environment")}
              value={form.environment}
              disabled={!canSwitchToProduction}
              aria-disabled={!canSwitchToProduction}
              onChange={(event) =>
                setForm((current) => ({
                  ...current,
                  environment: event.target.value as FiscalEnvironment,
                }))
              }
            >
              {(canSwitchToProduction
                ? ENVIRONMENT_OPTIONS
                : (["sandbox"] as FiscalEnvironment[])
              ).map((environment) => (
                <option key={environment} value={environment}>
                  {tLabel(environment)}
                </option>
              ))}
            </select>
            {!canSwitchToProduction ? (
              <span
                className="mt-1 block text-xs leading-5 text-amber-800"
                data-testid="fiscal-env-demo-locked"
              >
                {t("fiscal.setup.environmentDemoLocked")}
              </span>
            ) : null}
          </label>
        </div>

        <details className="group rounded-2xl border border-warm-200/80 bg-white/75 px-3 py-2 shadow-sm shadow-warm-900/5">
          <summary className="cursor-pointer text-xs font-semibold text-ink-700 transition-colors hover:text-ink-950">
            {t("fiscal.setup.compareModes")}
          </summary>
          <dl className="mt-2 grid gap-2 text-xs text-ink-600 md:grid-cols-2">
            {MODE_OPTIONS.map((option) => {
              const isActive = option.value === form.mode;
              return (
                <div
                  key={option.value}
                  className={`rounded-xl px-2 py-1.5 ${
                    isActive
                      ? "border border-brand/30 bg-brand/[0.06]"
                      : "border border-transparent"
                  }`}
                >
                  <dt className="flex items-center gap-1.5 font-semibold text-ink-900">
                    {t(option.titleKey)}
                    {isActive ? (
                      <span className="rounded-full bg-brand px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-white">
                        {t("fiscal.setup.currentMode")}
                      </span>
                    ) : null}
                  </dt>
                  <dd className="mt-0.5 leading-5">
                    {t(option.descriptionKey)}
                  </dd>
                </div>
              );
            })}
          </dl>
        </details>

        {settings?.last_validation_error ? (
          <Banner tone="red" Icon={AlertTriangle}>
            <span className="font-medium">
              {t("fiscal.setup.validationErrorTitle")}
            </span>
            <span className="block">{settings.last_validation_error}</span>
          </Banner>
        ) : null}
        {saveError ? (
          <Banner tone="red" Icon={AlertTriangle} role="alert">
            {saveError}
          </Banner>
        ) : null}
      </div>

      <div className="flex flex-wrap items-center justify-end gap-2 border-t border-warm-200/80 bg-warm-50/50 px-5 py-3">
        {editing ? (
          <button
            className={fiscalSecondaryButtonClass}
            type="button"
            onClick={onCancelEdit}
          >
            <X className="h-4 w-4" />
            {t("fiscal.setup.actions.cancel")}
          </button>
        ) : null}
        <button
          className={fiscalPrimaryButtonClass}
          type="submit"
          disabled={saving || !pointOfSaleIsValid || !taxIdIsValid}
        >
          {saving
            ? t("fiscal.setup.actions.saving")
            : t("fiscal.setup.actions.save")}
        </button>
      </div>
    </form>
  );
}

/* -------------------------------------------------------------------------- */
/*                                Summary bar                                 */
/* -------------------------------------------------------------------------- */

interface SummaryBarProps {
  settings: FiscalSettingsRecord;
  lastIssuedRelative: string | null;
  refreshing: boolean;
  onRefresh: () => void;
  editing: boolean;
  onToggleEdit: () => void;
  tLabel: (value: string | null | undefined) => string;
  t: (key: string) => string;
}

function SummaryBar({
  settings,
  lastIssuedRelative,
  refreshing,
  onRefresh,
  editing,
  onToggleEdit,
  tLabel,
  t,
}: SummaryBarProps) {
  const isOff = settings.mode === "off";
  return (
    <div className="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-warm-200/90 bg-white/90 px-4 py-3 shadow-sm shadow-warm-900/5">
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm text-ink-700">
        <span
          className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-semibold ${
            isOff
              ? "bg-warm-100 text-ink-700"
              : "bg-emerald-50 text-emerald-700"
          }`}
        >
          <span
            className={`h-1.5 w-1.5 rounded-full ${
              isOff ? "bg-ink-400" : "bg-emerald-500"
            }`}
          />
          {tLabel(settings.mode)}
        </span>
        <span className="text-ink-400">·</span>
        <span>{tLabel(settings.country)}</span>
        <span className="text-ink-400">·</span>
        <span>{tLabel(settings.provider)}</span>
        <span className="text-ink-400">·</span>
        <span className="text-ink-500">{tLabel(settings.environment)}</span>
        {lastIssuedRelative ? (
          <>
            <span className="text-ink-400">·</span>
            <span className="inline-flex items-center gap-1 text-ink-500">
              <ReceiptText className="h-3.5 w-3.5" />
              {t("fiscal.summary.lastIssued")} {lastIssuedRelative}
            </span>
          </>
        ) : null}
      </div>
      <div className="flex items-center gap-2">
        <button
          className="inline-flex h-9 items-center gap-1.5 rounded-xl border border-warm-200/90 bg-white/85 px-2.5 text-sm text-ink-700 shadow-sm shadow-warm-900/5 transition-colors hover:bg-brand/5 hover:text-brand-700 disabled:opacity-50"
          type="button"
          onClick={onRefresh}
          disabled={refreshing}
          aria-label={t("fiscal.summary.refresh")}
        >
          <RefreshCw
            className={`h-4 w-4 ${refreshing ? "animate-spin" : ""}`}
          />
        </button>
        <button
          className="inline-flex h-9 items-center gap-1.5 rounded-xl border border-warm-200/90 bg-white/85 px-3 text-sm font-medium text-ink-700 shadow-sm shadow-warm-900/5 transition-colors hover:bg-brand/5 hover:text-brand-700"
          type="button"
          onClick={onToggleEdit}
          aria-pressed={editing}
        >
          <Pencil className="h-4 w-4" />
          {t("fiscal.summary.editSetup")}
        </button>
      </div>
    </div>
  );
}

/* -------------------------------------------------------------------------- */
/*                                  History                                   */
/* -------------------------------------------------------------------------- */

interface HistoryCardProps {
  businessId: number;
  businessTimezone?: string | null;
  fiscalCountry?: string | null;
  receipts: FiscalReceipt[];
  filteredReceipts: FiscalReceipt[];
  historyFilter: HistoryFilter;
  onHistoryFilter: (next: HistoryFilter) => void;
  billSearch: string;
  onBillSearch: (next: string) => void;
  counts: {
    all: number;
    authorized: number;
    pending: number;
    attention: number;
  };
  billID: string;
  setBillID: (value: string) => void;
  onIssue: (event: FormEvent<HTMLFormElement>) => void;
  issuing: boolean;
  canIssue: boolean;
  canManualIssue: boolean;
  mode: FiscalMode;
  actionError: string | null;
  actionMessage: string | null;
  retryingReceiptID: number | null;
  onRetry: (receiptID: number) => void;
  resendingReceiptID: number | null;
  onResend: (receiptID: number) => void;
  // Optional: omitted for non-owners (F-RBAC), which hides the per-receipt Credit
  // button — crediting is owner-only on the backend.
  onCredit?: (receipt: FiscalReceipt) => void;
  onExportCsv: () => void;
  tLabel: (value: string | null | undefined) => string;
  t: (key: string, params?: Record<string, string | number>) => string;
  locale: string;
}

function HistoryCard({
  businessId,
  businessTimezone = null,
  fiscalCountry = null,
  receipts,
  filteredReceipts,
  historyFilter,
  onHistoryFilter,
  billSearch,
  onBillSearch,
  counts,
  billID,
  setBillID,
  onIssue,
  issuing,
  canIssue,
  canManualIssue,
  mode,
  actionError,
  actionMessage,
  retryingReceiptID,
  onRetry,
  resendingReceiptID,
  onResend,
  onCredit,
  onExportCsv,
  tLabel,
  t,
  locale,
}: HistoryCardProps) {
  const hasFilters = historyFilter !== "all" || billSearch.trim() !== "";
  const showHeader = receipts.length > 0;
  const issueForm = canManualIssue ? (
    <form className="flex flex-col gap-1" onSubmit={onIssue}>
      <div className="flex items-end gap-2">
        <label className="flex-1">
          <span className={labelClass}>{t("fiscal.history.issue.billId")}</span>
          <input
            aria-invalid={billID.trim() !== "" && !canIssue}
            aria-describedby="invoice-bill-id-help"
            className={`${fieldClass} ${
              billID.trim() !== "" && !canIssue
                ? "border-amber-400 focus:border-amber-500 focus:ring-amber-200"
                : ""
            }`}
            inputMode="numeric"
            pattern="[0-9]*"
            placeholder={t("fiscal.history.issue.placeholder")}
            type="text"
            value={billID}
            onChange={(event) => setBillID(event.target.value)}
          />
        </label>
        <button
          className={fiscalPrimaryButtonClass}
          type="submit"
          disabled={!canIssue || issuing}
        >
          {issuing
            ? t("fiscal.history.issue.issuing")
            : t("fiscal.history.issue.button")}
        </button>
      </div>
      <span id="invoice-bill-id-help" className="text-xs text-ink-500">
        {t("fiscal.history.issue.hint")}
      </span>
    </form>
  ) : null;
  return (
    <div className={fiscalPanelClass}>
      {showHeader ? (
        <div className={fiscalPanelHeaderClass}>
          <div className="min-w-0">
            <h2 id="invoice-history-heading" className="text-heading-sm font-semibold text-ink-950">
              {t("fiscal.history.title")}
            </h2>
            <p className={fiscalPanelDescriptionClass}>
              {t("fiscal.history.description")}
            </p>
          </div>

          <div className="flex flex-wrap items-end gap-2">
            {issueForm}
            <button
              type="button"
              className={fiscalSecondaryButtonClass}
              onClick={onExportCsv}
            >
              <Download className="h-4 w-4" />
              {t("fiscal.history.exportCsv")}
            </button>
          </div>
        </div>
      ) : null}

      <div className="space-y-3 px-5 py-4">
        {actionError ? (
          <Banner tone="red" Icon={AlertTriangle} role="alert">
            {actionError}
          </Banner>
        ) : null}
        {actionMessage ? (
          <Banner tone="emerald" Icon={CheckCircle2} role="status">
            {actionMessage}
          </Banner>
        ) : null}

        {receipts.length === 0 ? (
          <EmptyHistory t={t} mode={mode} issueForm={issueForm} />
        ) : (
          <>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <HistoryFilters
                active={historyFilter}
                onChange={onHistoryFilter}
                counts={counts}
                t={t}
              />
              <div className="flex items-center gap-2">
                <div className="relative">
                  <Search
                    className="absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-ink-400"
                    aria-hidden
                  />
                  <input
                    type="text"
                    inputMode="numeric"
                    placeholder={t("fiscal.history.searchPlaceholder")}
                    aria-label={t("fiscal.history.search")}
                    value={billSearch}
                    onChange={(event) => onBillSearch(event.target.value)}
                    className="h-8 w-44 rounded-lg border border-warm-200 bg-white/90 pl-7 pr-2 text-xs text-ink-900 shadow-sm outline-none transition-colors focus:border-brand focus:ring-2 focus:ring-brand/15"
                  />
                </div>
                {hasFilters ? (
                  <button
                    type="button"
                    onClick={() => {
                      onHistoryFilter("all");
                      onBillSearch("");
                    }}
                    className="text-xs font-medium text-ink-500 transition-colors hover:text-ink-700"
                  >
                    {t("fiscal.history.clearFilters")}
                  </button>
                ) : null}
              </div>
            </div>

            {filteredReceipts.length === 0 ? (
              <div className="rounded-2xl border border-dashed border-warm-300 bg-warm-50/70 px-4 py-6 text-center text-sm text-ink-600">
                {t("fiscal.history.noMatch")}
              </div>
            ) : (
              <div className="overflow-x-auto rounded-2xl border border-warm-200/80">
                <table
                  aria-labelledby="invoice-history-heading"
                  className={fiscalTableClass}
                >
                  <thead className={fiscalTableHeadClass}>
                    <tr>
                      <th className="px-3 py-2">
                        {t("fiscal.history.columns.billId")}
                      </th>
                      <th className="px-3 py-2">
                        {t(receiptHistoryColumnKey(fiscalCountry))}
                      </th>
                      <th className="px-3 py-2">
                        {t("fiscal.history.columns.amount")}
                      </th>
                      <th className="px-3 py-2">
                        {t("fiscal.history.columns.status")}
                      </th>
                      <th className="px-3 py-2">
                        {t("fiscal.history.columns.error")}
                      </th>
                      <th className="px-3 py-2 text-right">
                        {t("fiscal.history.columns.actions")}
                      </th>
                    </tr>
                  </thead>
                  <tbody className={fiscalTableBodyClass}>
                    {filteredReceipts.map((receipt) => (
                      <tr key={receipt.id} className={fiscalTableRowClass}>
                        <td className="px-3 py-3 align-top font-semibold text-ink-950">
                          #{receipt.bill_id}
                          {receipt.issued_at ? (
                            <div className="text-xs font-normal text-ink-500">
                              {formatIssuedDate(receipt.issued_at, locale, businessTimezone)}
                            </div>
                          ) : null}
                        </td>
                        <td className="px-3 py-3 align-top text-ink-900">
                          <div className="flex items-center gap-2">
                            <span>
                              {tLabel(
                                displayReceiptType(
                                  receipt.country || fiscalCountry,
                                  receipt.receipt_type,
                                ),
                              )}
                            </span>
                            {receipt.pdf_path ? (
                              <a
                                href={receipt.pdf_path}
                                target="_blank"
                                rel="noopener noreferrer"
                                aria-label={t("fiscal.history.viewPdf")}
                                title={t("fiscal.history.viewPdf")}
                                className="inline-flex rounded-full transition-opacity hover:opacity-80 focus:outline-none focus:ring-2 focus:ring-brand/30"
                              >
                                <StatusBadge
                                  tone="info"
                                  label={t("fiscal.history.pdfReady")}
                                  size="sm"
                                />
                              </a>
                            ) : null}
                            {receipt.qr_image_path || receipt.qr_payload ? (
                              <a
                                href={
                                  receipt.qr_image_path ||
                                  receipt.qr_payload ||
                                  "#"
                                }
                                target="_blank"
                                rel="noopener noreferrer"
                                aria-label={t("fiscal.history.viewQr")}
                                title={t("fiscal.history.viewQr")}
                                className="inline-flex rounded-full p-0.5 text-ink-500 transition-colors hover:text-brand focus:outline-none focus:ring-2 focus:ring-brand/30"
                              >
                                <QrCode className="h-4 w-4" />
                              </a>
                            ) : null}
                          </div>
                          <div className="text-xs text-ink-500">
                            {receipt.receipt_number ||
                              t("fiscal.history.unnumbered")}
                          </div>
                          {receipt.auth_code ? (
                            <div className="mt-0.5 text-xs text-ink-500">
                              <span className="font-medium text-ink-600">
                                {t("fiscal.history.cae")}:
                              </span>{" "}
                              <span className="font-mono tabular-nums">
                                {receipt.auth_code}
                              </span>
                            </div>
                          ) : null}
                          {receipt.status === "authorized" ? (
                            <DeliveryChannelBadges
                              businessId={businessId}
                              receiptId={receipt.id}
                              t={t}
                            />
                          ) : null}
                        </td>
                        <td className="px-3 py-3 align-top font-medium text-ink-950">
                          {formatMoney(
                            receipt.total_amount_cents,
                            receipt.currency,
                            locale,
                          )}
                        </td>
                        <td className="px-3 py-3 align-top">
                          <StatusPill status={receipt.status} t={t} />
                        </td>
                        <td className="max-w-xs px-3 py-3 align-top text-sm text-ink-600">
                          {receipt.error_message ? (
                            <span
                              className="line-clamp-2"
                              title={receipt.error_message}
                            >
                              {receipt.error_message}
                            </span>
                          ) : (
                            <span className="text-ink-500">
                              {t("fiscal.common.none")}
                            </span>
                          )}
                        </td>
                        <td className="px-3 py-3 align-top text-right">
                          {isRetryableStatus(receipt.status) ||
                          isResendableStatus(receipt.status) ||
                          isCreditableReceipt(receipt) ? (
                            <div className="flex flex-wrap items-center justify-end gap-1.5">
                              {isRetryableStatus(receipt.status) ? (
                                <button
                                  className="inline-flex h-8 items-center justify-center gap-1.5 rounded-lg border border-orange-200 bg-orange-50 px-3 text-sm font-medium text-orange-800 transition-colors hover:bg-orange-100 disabled:cursor-not-allowed disabled:opacity-60"
                                  type="button"
                                  disabled={retryingReceiptID !== null}
                                  onClick={() => void onRetry(receipt.id)}
                                >
                                  <RefreshCw
                                    className={`h-3.5 w-3.5 ${
                                      retryingReceiptID === receipt.id
                                        ? "animate-spin"
                                        : ""
                                    }`}
                                  />
                                  {retryingReceiptID === receipt.id
                                    ? t("fiscal.history.retry.retrying")
                                    : t("fiscal.history.retry.button")}
                                </button>
                              ) : null}
                              {onCredit && isCreditableReceipt(receipt) ? (
                                <button
                                  className={fiscalSmallSecondaryButtonClass}
                                  type="button"
                                  onClick={() => onCredit(receipt)}
                                >
                                  <RotateCcw className="h-3.5 w-3.5" />
                                  {t("fiscal.history.creditNote.button")}
                                </button>
                              ) : null}
                              {isResendableStatus(receipt.status) ? (
                                <button
                                  className={fiscalSmallSecondaryButtonClass}
                                  type="button"
                                  disabled={resendingReceiptID !== null}
                                  onClick={() => void onResend(receipt.id)}
                                >
                                  <Send
                                    className={`h-3.5 w-3.5 ${
                                      resendingReceiptID === receipt.id
                                        ? "animate-pulse"
                                        : ""
                                    }`}
                                  />
                                  {resendingReceiptID === receipt.id
                                    ? t("fiscal.history.resend.resending")
                                    : t("fiscal.history.resend.button")}
                                </button>
                              ) : null}
                            </div>
                          ) : (
                            <span className="text-xs text-ink-500">
                              {t("fiscal.common.none")}
                            </span>
                          )}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}

/* -------------------------------------------------------------------------- */
/*                     Wave 4 delivery channel badges                         */
/* -------------------------------------------------------------------------- */

function deliveryTone(
  status: string,
): "success" | "warning" | "danger" | "pending" | "neutral" {
  if (status === "succeeded") return "success";
  if (status === "dead") return "danger";
  if (status === "leased" || status === "pending") return "pending";
  return "neutral";
}

function DeliveryChannelBadges({
  businessId,
  receiptId,
  t,
}: {
  businessId: number;
  receiptId: number;
  t: (key: string, params?: Record<string, string | number>) => string;
}) {
  const [tasks, setTasks] = useState<FiscalDeliveryTask[]>([]);
  const [retryingId, setRetryingId] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    void listReceiptDelivery(businessId, receiptId)
      .then((items) => {
        if (!cancelled) setTasks(items);
      })
      .catch(() => {
        if (!cancelled) setTasks([]);
      });
    return () => {
      cancelled = true;
    };
  }, [businessId, receiptId]);

  if (tasks.length === 0) return null;

  const handleRetry = async (taskId: number) => {
    setRetryingId(taskId);
    setError(null);
    try {
      await retryDeliveryTask(businessId, taskId);
      const next = await listReceiptDelivery(businessId, receiptId);
      setTasks(next);
    } catch (e) {
      setError(
        getSafeApiErrorMessage(e, t("fiscal.history.delivery.retryError")),
      );
    } finally {
      setRetryingId(null);
    }
  };

  return (
    <div className="mt-1.5 flex flex-wrap items-center gap-1" role="group" aria-label={t("fiscal.history.delivery.label")}>
      {tasks.map((task) => {
        const channelLabel = t(`fiscal.history.delivery.channel.${task.channel}`);
        const statusLabel = t(`fiscal.history.delivery.status.${task.status}`);
        const canRetry = task.status === "dead";
        return (
          <span key={task.id} className="inline-flex items-center gap-0.5">
            <StatusBadge
              tone={deliveryTone(task.status)}
              label={`${channelLabel}: ${statusLabel}`}
              size="sm"
            />
            {canRetry ? (
              <button
                type="button"
                className="inline-flex h-6 items-center rounded-md border border-rose-200 bg-rose-50 px-1.5 text-[10px] font-medium text-rose-800 hover:bg-rose-100 disabled:opacity-60"
                disabled={retryingId !== null}
                aria-busy={retryingId === task.id}
                aria-label={t("fiscal.history.delivery.retryAria", {
                  channel: channelLabel,
                })}
                onClick={() => void handleRetry(task.id)}
              >
                {retryingId === task.id
                  ? t("fiscal.history.delivery.retrying")
                  : t("fiscal.history.delivery.retry")}
              </button>
            ) : null}
          </span>
        );
      })}
      {error ? (
        <span className="text-[10px] text-rose-700" role="alert">
          {error}
        </span>
      ) : null}
    </div>
  );
}

/* -------------------------------------------------------------------------- */
/*                             Credentials card                              */
/* -------------------------------------------------------------------------- */

interface CredentialsCardProps {
  settings: FiscalSettingsRecord;
  businessTimezone?: string | null;
  setupStatus: string;
  hasCredentials: boolean;
  certFile: File | null;
  keyFile: File | null;
  onCertChange: (file: File | null) => void;
  onKeyChange: (file: File | null) => void;
  onUpload: (event: FormEvent<HTMLFormElement>) => void;
  uploading: boolean;
  onValidate: () => void;
  validating: boolean;
  canValidate: boolean;
  credentialsMessage: string | null;
  credentialsError: string | null;
  tLabel: (value: string | null | undefined) => string;
  t: (key: string, params?: Record<string, string | number>) => string;
  locale: string;
}

function CredentialsCard({
  settings,
  businessTimezone = null,
  setupStatus,
  hasCredentials,
  certFile,
  keyFile,
  onCertChange,
  onKeyChange,
  onUpload,
  uploading,
  onValidate,
  validating,
  canValidate,
  credentialsMessage,
  credentialsError,
  t,
  locale,
}: CredentialsCardProps) {
  const visual = setupStatusVisual(setupStatus);
  const statusLabelKey = `fiscal.credentials.status.${setupStatus}`;
  const statusLabel = t(statusLabelKey);
  const expiry = formatExpiryDate(settings.credentials_expires_at, locale, businessTimezone);
  const lastValidated = formatExpiryDate(settings.last_validated_at, locale, businessTimezone);

  return (
    <section
      className={fiscalPanelClass}
      aria-labelledby="fiscal-credentials-heading"
    >
      <div className={fiscalPanelHeaderClass}>
        <div className="min-w-0">
          <h2 id="fiscal-credentials-heading" className="text-heading-sm font-semibold text-ink-950">
            {t("fiscal.credentials.title")}
          </h2>
          <p className={fiscalPanelDescriptionClass}>
            {t("fiscal.credentials.description")}
          </p>
        </div>
        <div className="flex items-center gap-2">
          <span className={labelClass}>
            {t("fiscal.credentials.status.label")}
          </span>
          <StatusBadge
            tone={visual.tone}
            label={
              statusLabel === statusLabelKey
                ? formatDataLabel(setupStatus)
                : statusLabel
            }
            Icon={visual.Icon}
          />
        </div>
      </div>

      <div className="space-y-4 px-5 py-5">
        {hasCredentials ? (
          <dl className="grid gap-3 rounded-2xl border border-warm-200/80 bg-warm-50/70 px-4 py-3 text-sm shadow-sm shadow-warm-900/5 sm:grid-cols-2">
            <div className="min-w-0">
              <dt className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-[0.16em] text-ink-500">
                <Fingerprint className="h-3.5 w-3.5" />
                {t("fiscal.credentials.fingerprint")}
              </dt>
              <dd className="mt-0.5 break-all font-mono text-xs text-ink-800">
                {settings.credentials_fingerprint}
              </dd>
            </div>
            {expiry ? (
              <div>
                <dt className={labelClass}>
                  {t("fiscal.credentials.expiresAt")}
                </dt>
                <dd className="mt-0.5 text-ink-800">{expiry}</dd>
              </div>
            ) : null}
            {lastValidated ? (
              <div>
                <dt className={labelClass}>
                  {t("fiscal.credentials.lastValidated")}
                </dt>
                <dd className="mt-0.5 text-ink-800">{lastValidated}</dd>
              </div>
            ) : null}
          </dl>
        ) : null}

        {settings.last_validation_error ? (
          <Banner tone="red" Icon={AlertTriangle} role="alert">
            <span className="font-medium">
              {t("fiscal.credentials.validationFailed")}
            </span>
            <span className="block">{settings.last_validation_error}</span>
          </Banner>
        ) : null}

        <form className="space-y-4" onSubmit={onUpload}>
          <div className="grid gap-4 md:grid-cols-2">
            <CredentialFileInput
              id="fiscal-cert-input"
              label={t("fiscal.credentials.certLabel")}
              hint={t("fiscal.credentials.certHint")}
              accept=".crt,.pem,.cer"
              file={certFile}
              onChange={onCertChange}
              t={t}
            />
            <CredentialFileInput
              id="fiscal-key-input"
              label={t("fiscal.credentials.keyLabel")}
              hint={t("fiscal.credentials.keyHint")}
              accept=".key,.pem"
              file={keyFile}
              onChange={onKeyChange}
              t={t}
            />
          </div>

          <p className="text-xs leading-5 text-ink-500">
            {t("fiscal.credentials.keyNeverStored")}
          </p>

          {credentialsError ? (
            <Banner tone="red" Icon={AlertTriangle} role="alert">
              {credentialsError}
            </Banner>
          ) : null}
          {credentialsMessage ? (
            <Banner tone="emerald" Icon={CheckCircle2} role="status">
              {credentialsMessage}
            </Banner>
          ) : null}

          <div className="flex flex-wrap items-center justify-end gap-2">
            <button
              className={fiscalSecondaryButtonClass}
              type="button"
              onClick={onValidate}
              disabled={!canValidate}
            >
              <ShieldCheck
                className={`h-4 w-4 ${validating ? "animate-pulse" : ""}`}
              />
              {validating
                ? t("fiscal.credentials.validating")
                : t("fiscal.credentials.validate")}
            </button>
            <button
              className={fiscalPrimaryButtonClass}
              type="submit"
              disabled={uploading || !certFile || !keyFile}
            >
              <Upload className="h-4 w-4" />
              {uploading
                ? t("fiscal.credentials.uploading")
                : t("fiscal.credentials.upload")}
            </button>
          </div>
        </form>
      </div>
    </section>
  );
}

/* -------------------------------------------------------------------------- */
/*                          Unsupported-region card                          */
/* -------------------------------------------------------------------------- */

interface UnsupportedRegionCardProps {
  settings: FiscalSettingsRecord;
  t: (key: string, params?: Record<string, string | number>) => string;
}

// Rendered in place of the AFIP/ARCA credential-upload form when the business
// has no real e-invoicing integration — a "demo" provider (e.g. the US demo
// business) or any country outside AR/AE. The strict AR certificate + private
// key form is Argentina-only, so surfacing it for a US business is misleading.
function UnsupportedRegionCard({ settings, t }: UnsupportedRegionCardProps) {
  const isDemo = (settings.provider || "").toLowerCase() === "demo";
  const countryLabel = (settings.country || "").toUpperCase() || "—";
  const title = isDemo
    ? t("fiscal.region.demoTitle")
    : t("fiscal.region.unavailableTitle");
  const description = isDemo
    ? t("fiscal.region.demoDescription")
    : t("fiscal.region.unavailableDescription", { country: countryLabel });
  const pathToLive = isDemo
    ? t("fiscal.region.demoPathToLive")
    : t("fiscal.region.unavailablePathToLive", { country: countryLabel });

  return (
    <section className={fiscalPanelClass} aria-labelledby="fiscal-region-heading">
      <div className={fiscalPanelHeaderClass}>
        <div className="min-w-0">
          <h2
            id="fiscal-region-heading"
            className="text-heading-sm font-semibold text-ink-950"
          >
            {t("fiscal.region.title")}
          </h2>
        </div>
      </div>
      <div className="px-5 py-6">
        <div className="flex items-start gap-3 rounded-2xl border border-amber-200/80 bg-amber-50/70 px-4 py-4 text-sm shadow-sm shadow-amber-900/5">
          <FileText className="mt-0.5 h-5 w-5 flex-shrink-0 text-amber-700" />
          <div className="flex-1">
            <div className="font-semibold text-ink-900">{title}</div>
            <p className="mt-1 leading-6 text-ink-700">{description}</p>
            <p className="mt-2 leading-6 text-ink-600">{pathToLive}</p>
          </div>
        </div>
      </div>
    </section>
  );
}

interface CredentialFileInputProps {
  id: string;
  label: string;
  hint: string;
  accept: string;
  file: File | null;
  onChange: (file: File | null) => void;
  t: (key: string, params?: Record<string, string | number>) => string;
}

function CredentialFileInput({
  id,
  label,
  hint,
  accept,
  file,
  onChange,
  t,
}: CredentialFileInputProps) {
  return (
    <label className="block" htmlFor={id}>
      <span className={labelClass}>{label}</span>
      <input
        id={id}
        type="file"
        accept={accept}
        aria-label={label}
        className="mt-1 block w-full cursor-pointer rounded-xl border border-warm-200 bg-white/90 text-sm text-ink-700 shadow-sm file:mr-3 file:cursor-pointer file:border-0 file:bg-warm-100 file:px-3 file:py-2 file:text-sm file:font-medium file:text-ink-700 hover:file:bg-brand/10 focus:border-brand focus:outline-none focus:ring-2 focus:ring-brand/15"
        onChange={(event) => onChange(event.target.files?.[0] || null)}
      />
      <span className="mt-1 block text-xs text-ink-500">
        {file ? file.name : hint}
      </span>
    </label>
  );
}

/* -------------------------------------------------------------------------- */
/*                            Credit note dialog                             */
/* -------------------------------------------------------------------------- */

interface CreditNoteDialogProps {
  receipt: FiscalReceipt;
  reason: string;
  onReasonChange: (value: string) => void;
  crediting: boolean;
  onConfirm: () => void;
  onCancel: () => void;
  t: (key: string, params?: Record<string, string | number>) => string;
}

function CreditNoteDialog({
  receipt,
  reason,
  onReasonChange,
  crediting,
  onConfirm,
  onCancel,
  t,
}: CreditNoteDialogProps) {
  // NextUI Modal supplies the focus trap, initial focus, Escape-to-close, and
  // backdrop dismissal the hand-rolled dialog lacked. It's mounted only while a
  // credit target is set, so isOpen stays true and onClose maps to onCancel
  // (a no-op mid-request via the guard in the parent). See R3-AC (LOW).
  return (
    <Modal
      isOpen
      onClose={onCancel}
      isDismissable={!crediting}
      isKeyboardDismissDisabled={crediting}
      size="md"
      aria-labelledby="credit-note-title"
    >
      <ModalContent className="border border-warm-200/90 bg-white">
        <ModalHeader id="credit-note-title" className="text-ink-950">
          {t("fiscal.history.creditNote.confirmTitle")}
        </ModalHeader>
        <ModalBody className="space-y-4">
          <p className="text-sm leading-6 text-ink-600">
            {t("fiscal.history.creditNote.confirmDescription", {
              bill: receipt.bill_id,
            })}
          </p>

          <label className="block" htmlFor="credit-note-reason">
            <span className={labelClass}>
              {t("fiscal.history.creditNote.reasonLabel")}
            </span>
            <textarea
              id="credit-note-reason"
              aria-label={t("fiscal.history.creditNote.reasonLabel")}
              className="mt-1 w-full rounded-xl border border-warm-200 bg-white/90 px-3 py-2 text-sm text-ink-900 shadow-sm outline-none transition-colors focus:border-brand focus:ring-2 focus:ring-brand/15"
              rows={3}
              placeholder={t("fiscal.history.creditNote.reasonPlaceholder")}
              value={reason}
              onChange={(event) => onReasonChange(event.target.value)}
            />
          </label>
        </ModalBody>
        <ModalFooter>
          <button
            type="button"
            className={fiscalSecondaryButtonClass}
            onClick={onCancel}
            disabled={crediting}
          >
            {t("fiscal.history.creditNote.cancel")}
          </button>
          <button
            type="button"
            className={fiscalPrimaryButtonClass}
            onClick={() => void onConfirm()}
            disabled={crediting}
          >
            <RotateCcw
              className={`h-4 w-4 ${crediting ? "animate-spin" : ""}`}
            />
            {crediting
              ? t("fiscal.history.creditNote.crediting")
              : t("fiscal.history.creditNote.confirm")}
          </button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}

/* -------------------------------------------------------------------------- */
/*                             Helper components                              */
/* -------------------------------------------------------------------------- */

interface BannerProps {
  tone: "red" | "emerald" | "amber";
  Icon: LucideIcon;
  role?: "alert" | "status";
  children: React.ReactNode;
}

function Banner({ tone, Icon, role, children }: BannerProps) {
  const toneClasses =
    tone === "red"
      ? "border-rose-200 bg-rose-50 text-rose-700"
      : tone === "amber"
        ? "border-amber-200 bg-amber-50/90 text-amber-800"
        : "border-emerald-200 bg-emerald-50 text-emerald-700";
  return (
    <div
      role={role}
      className={`flex items-start gap-2.5 rounded-xl border px-3 py-2 text-sm shadow-sm ${toneClasses}`}
    >
      <Icon className="mt-0.5 h-4 w-4 flex-shrink-0" />
      <div className="flex-1 leading-6">{children}</div>
    </div>
  );
}

interface StatusPillProps {
  status: FiscalStatus;
  t: (key: string) => string;
}

function StatusPill({ status, t }: StatusPillProps) {
  const visual = statusVisual(status);
  const labelKey = `fiscal.labels.${status}`;
  const translated = t(labelKey);
  const label = translated === labelKey ? formatDataLabel(status) : translated;
  return <StatusBadge tone={visual.tone} label={label} Icon={visual.Icon} />;
}

interface HistoryFiltersProps {
  active: HistoryFilter;
  onChange: (next: HistoryFilter) => void;
  counts: {
    all: number;
    authorized: number;
    pending: number;
    attention: number;
  };
  t: (key: string) => string;
}

function HistoryFilters({ active, onChange, counts, t }: HistoryFiltersProps) {
  const items: Array<{ key: HistoryFilter; label: string; count: number }> = [
    { key: "all", label: t("fiscal.history.filters.all"), count: counts.all },
    {
      key: "authorized",
      label: t("fiscal.history.filters.authorized"),
      count: counts.authorized,
    },
    {
      key: "pending",
      label: t("fiscal.history.filters.pending"),
      count: counts.pending,
    },
    {
      key: "attention",
      label: t("fiscal.history.filters.attention"),
      count: counts.attention,
    },
  ];
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {items.map((item) => {
        const isActive = item.key === active;
        return (
          <button
            key={item.key}
            type="button"
            onClick={() => onChange(item.key)}
            aria-pressed={isActive}
            className={`inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium transition-colors ${
              isActive
                ? "bg-ink-950 text-white"
                : "border border-warm-200 bg-white text-ink-700 hover:bg-brand/5 hover:text-brand-700"
            }`}
          >
            {item.label}
            <span
              className={`rounded-full px-1.5 py-0.5 text-[10px] font-semibold ${
                isActive ? "bg-white/15 text-white" : "bg-warm-100 text-ink-700"
              }`}
            >
              {item.count}
            </span>
          </button>
        );
      })}
    </div>
  );
}

interface EmptyHistoryProps {
  t: (key: string) => string;
  mode: FiscalMode;
  issueForm?: ReactNode;
}

function EmptyHistory({ t, mode, issueForm }: EmptyHistoryProps) {
  const descriptionKey =
    mode === "manual"
      ? "fiscal.history.emptyDescriptionManual"
      : mode === "off"
        ? "fiscal.history.emptyDescriptionOff"
        : "fiscal.history.emptyDescription";
  return (
    <div className="rounded-2xl border border-dashed border-warm-300 bg-warm-50/70 px-4 py-8 text-center">
      <h3 className="text-sm font-semibold text-ink-950">
        {t("fiscal.history.emptyTitle")}
      </h3>
      <p className="mx-auto mt-1 max-w-md text-sm leading-6 text-ink-600">
        {t(descriptionKey)}
      </p>
      {issueForm ? (
        <div className="mx-auto mt-4 inline-flex max-w-md text-left">
          {issueForm}
        </div>
      ) : null}
    </div>
  );
}
