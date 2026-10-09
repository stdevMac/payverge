"use client";

import React, { useState, useEffect, useCallback, useRef } from "react";
import { useModalOpenKey } from "@/hooks/useModalOpenKey";
import {
  Card,
  CardBody,
  CardHeader,
  Input,
  Button,
  Table,
  TableHeader,
  TableColumn,
  TableBody,
  TableRow,
  TableCell,
  Chip,
  Pagination,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Textarea,
} from "@nextui-org/react";
import {
  Search,
  Eye,
  Star,
  UserPlus,
  Users,
  ArrowUp,
  ArrowDown,
  ChevronsUpDown,
  X,
  AlertTriangle,
  UserMinus,
} from "lucide-react";
import { isValidEmail, isValidPhone } from "@/lib/fieldValidation";
import {
  businessCRMAPI,
  CustomerBusiness,
  BusinessCustomerSummary,
} from "@/api/crm";
import { getBusiness } from "@/api/business";
import { getLoyalty, type LoyaltyProgram, type LoyaltyTier } from "@/api/loyalty";
import { formatCurrency } from "@/api/currency";
import toast from "react-hot-toast";
import { runWithFeedback } from "@/lib/runWithFeedback";
import { formatDisplayDate } from "@/utils/displayDate";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import {
  parseCustomerTags,
  serializeCustomerTagsInput,
} from "@/utils/customerTags";
import { CurrencyPrice } from "@/components/common/CurrencyConverter";
import { intlLocaleFor } from "@/utils/intlLocale";
import { CRMCustomersSkeleton } from "./CRMCustomersSkeleton";
import { PremiumPanel } from "../premium";
import ConfirmationModal from "../modals/ConfirmationModal";

interface CustomersTabProps {
  businessId: number;
  isLocked?: boolean;
  /** Dashboard cross-tab seam: jump to the guest capture path (tables/QR). */
  onNavigateToTab?: (tab: string) => void;
  /**
   * When arriving from a Segments drilldown or a Marketing win-back deep link,
   * the list opens filtered to this behavioral segment with a visible chip
   * (fix 7). One of lapsed/vip/new/at-risk.
   */
  initialSegment?: string;
  /** Clears the active segment filter (dismisses the chip). */
  onClearSegment?: () => void;
  /** Controlled add-customer modal — used when the page header owns the CTA. */
  addModalOpen?: boolean;
  onAddModalOpenChange?: (open: boolean) => void;
  /** Hide the in-list Add Customer button when the page header already has it. */
  hideListAddButton?: boolean;
}

/** Human labels for the segment filter chip, keyed by the segment param. */
const SEGMENT_LABEL_KEYS: Record<string, string> = {
  lapsed: "segments.lapsed.title",
  vip: "segments.vip.title",
  new: "segments.new.title",
  "at-risk": "segments.atRisk.title",
};

const TIERS = ["All", "Bronze", "Silver", "Gold"] as const;
type TierFilter = (typeof TIERS)[number];

/** Normalize allergies for display — accepts JSON arrays or free text. */
function formatAllergiesDisplay(raw?: string | null): string {
  if (!raw) return "";
  const trimmed = raw.trim();
  if (!trimmed || trimmed === "[]") return "";
  if (trimmed.startsWith("[")) {
    try {
      const parsed = JSON.parse(trimmed) as unknown;
      if (Array.isArray(parsed)) {
        return parsed
          .map((item) => String(item).trim())
          .filter(Boolean)
          .join(", ");
      }
    } catch {
      // Fall through to free-text display.
    }
  }
  return trimmed;
}

// Columns the BACKEND can sort (crm/service.go customerSortColumns). Only these
// render a sort affordance — a header that can't reorder the whole base must not
// pretend it can (audit §3.5 MED, fix 6). loyalty_points is intentionally absent
// (the server sorts loyalty_tier, not points).
const SERVER_SORTABLE = new Set([
  "name",
  "total_spent",
  "visit_count",
  "last_visit_at",
]);

/**
 * Accessor helpers — the customer list payload can arrive in two shapes:
 *  - Production: nested `{ customer: { name, email } }` from CustomerBusiness.
 *  - Lightweight test fixtures (and future flattened endpoints): flat
 *    `{ name, email, visits }`.
 * These tolerate both so the summary cards + table render either way without
 * a server-side shape migration.
 */
const customerName = (c: CustomerBusiness & { name?: string }): string =>
  c.customer?.name ?? c.name ?? "";
const customerEmail = (c: CustomerBusiness & { email?: string }): string =>
  c.customer?.email ?? c.email ?? "";
const customerVisits = (c: CustomerBusiness & { visits?: number }): number =>
  c.visit_count ?? c.visits ?? 0;

const crmEmptyRows = ["w-24", "w-16", "w-20"];

function CRMCustomersEmptyPanel({
  actionLabel,
  onAction,
  subtitle,
  title,
  secondaryActionLabel,
  onSecondaryAction,
}: {
  actionLabel: string;
  onAction: () => void;
  subtitle: string;
  title: string;
  secondaryActionLabel?: string;
  onSecondaryAction?: () => void;
}) {
  return (
    <div className="overflow-hidden rounded-2xl border border-brand/10 bg-brand/5 p-4 sm:p-5">
      <div className="grid items-center gap-6 lg:grid-cols-[minmax(0,1fr)_280px]">
        <div>
          <div className="mb-5 flex h-12 w-12 items-center justify-center rounded-2xl border border-white/80 bg-white shadow-sm">
            <Users className="h-5 w-5 text-brand" strokeWidth={1.7} />
          </div>
          <h3 className="font-title text-2xl text-ink-950">{title}</h3>
          <p className="mt-3 max-w-xl text-sm leading-6 text-ink-600">
            {subtitle}
          </p>
          <div className="mt-6 flex flex-wrap items-center gap-3">
            <Button
              className="bg-brand text-white font-medium hover:bg-brand-dark"
              startContent={<UserPlus className="h-4 w-4" />}
              onPress={onAction}
            >
              {actionLabel}
            </Button>
            {secondaryActionLabel && onSecondaryAction ? (
              <Button
                variant="bordered"
                className="border-brand/40 font-medium text-brand"
                onPress={onSecondaryAction}
              >
                {secondaryActionLabel}
              </Button>
            ) : null}
          </div>
        </div>

        <div
          className="relative min-h-[220px] rounded-2xl border border-white/80 bg-white/85 p-4 shadow-[0_20px_54px_rgba(46,42,37,0.1)]"
          aria-hidden="true"
        >
          <div className="mb-4 flex items-center justify-between">
            <div className="h-2 w-24 rounded-full bg-warm-200" />
            <div className="flex h-9 w-9 items-center justify-center rounded-xl bg-brand/10">
              <Star className="h-4 w-4 text-brand" />
            </div>
          </div>

          <div className="space-y-3">
            {crmEmptyRows.map((widthClass, index) => (
              <div
                key={widthClass}
                className="rounded-xl border border-warm-200 bg-warm-50/80 p-3"
              >
                <div className="flex items-center gap-3">
                  <span
                    className={`flex h-9 w-9 items-center justify-center rounded-xl ${
                      index === 1 ? "bg-amber-100" : "bg-brand/15"
                    }`}
                  >
                    {index === 1 ? (
                      <Star className="h-4 w-4 text-amber-700" />
                    ) : (
                      <Users className="h-4 w-4 text-brand" />
                    )}
                  </span>
                  <span
                    className={`h-2 rounded-full bg-ink-200 ${widthClass}`}
                  />
                  <span className="h-2 w-10 rounded-full bg-emerald-200" />
                </div>
              </div>
            ))}
          </div>

          <div className="mt-4 rounded-xl border border-warm-200 bg-warm-50 p-3">
            <div className="mb-3 h-2 w-20 rounded-full bg-ink-200" />
            <div className="flex flex-wrap gap-2">
              <span className="h-7 w-16 rounded-full bg-brand/15" />
              <span className="h-7 w-20 rounded-full bg-warm-200" />
              <span className="h-7 w-14 rounded-full bg-amber-100" />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

export default function CustomersTab({
  businessId,
  isLocked = false,
  onNavigateToTab,
  initialSegment,
  onClearSegment,
  addModalOpen,
  onAddModalOpenChange,
  hideListAddButton = false,
}: CustomersTabProps) {
  const { locale } = useSimpleLocale();
  // Resolved BCP-47 tag so operator money grouping follows the operator locale.
  const intlLocale = intlLocaleFor(locale);
  const [currentLocale, setCurrentLocale] = useState(locale);

  const [customers, setCustomers] = useState<CustomerBusiness[]>([]);
  const [summary, setSummary] = useState<BusinessCustomerSummary | null>(null);
  // First-load skeleton only (L5-1 / S-9). Refetches use isRefetching so the
  // search input stays mounted mid-typing instead of unmounting into skeleton.
  // hasLoadedRef drives the first/refetch branch without re-creating the
  // callback (and re-firing the effect) when the first load completes.
  const [loading, setLoading] = useState(true);
  const [isRefetching, setIsRefetching] = useState(false);
  const [hasLoaded, setHasLoaded] = useState(false);
  const hasLoadedRef = useRef(false);
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  // `search` drives the input value (instant); `debouncedSearch` is what the
  // network call keys on, so typing a name fires one request, not one per key.
  const [search, setSearch] = useState("");
  const [debouncedSearch, setDebouncedSearch] = useState("");
  const [tier, setTier] = useState<TierFilter>("All");
  // Monotonic request id: stale responses (slow link / racing keystrokes) are
  // discarded so the table always reflects the latest query.
  const requestIdRef = useRef(0);
  const [selectedCustomer, setSelectedCustomer] =
    useState<CustomerBusiness | null>(null);
  const [detailsModal, setDetailsModal] = useState(false);
  const [notesModal, setNotesModal] = useState(false);
  const [notes, setNotes] = useState("");
  const [allergiesModal, setAllergiesModal] = useState(false);
  const [allergies, setAllergies] = useState("");
  const [pointsModal, setPointsModal] = useState(false);
  const [pointsValue, setPointsValue] = useState("");
  const [pointsReason, setPointsReason] = useState("");
  const [tagsModal, setTagsModal] = useState(false);
  const [tags, setTags] = useState("");
  const [addModal, setAddModal] = useState(false);
  const [newCustomer, setNewCustomer] = useState({ name: "", email: "", phone: "" });
  const [addFieldError, setAddFieldError] = useState<string | null>(null);
  const [addLoading, setAddLoading] = useState(false);
  const [notesSaving, setNotesSaving] = useState(false);
  const [allergiesSaving, setAllergiesSaving] = useState(false);
  const [pointsSaving, setPointsSaving] = useState(false);
  const [tagsSaving, setTagsSaving] = useState(false);
  const [sortKey, setSortKey] = useState<string>("last_visit_at");
  const [sortDir, setSortDir] = useState<"asc" | "desc">("desc");
  const [businessCurrency, setBusinessCurrency] = useState("USD");
  const [loyaltyTiers, setLoyaltyTiers] = useState<LoyaltyTier[]>([]);
  const [loyaltyProgram, setLoyaltyProgram] = useState<LoyaltyProgram | null>(
    null,
  );
  const isAddControlled = typeof onAddModalOpenChange === "function";
  const addOpen = isAddControlled ? Boolean(addModalOpen) : addModal;
  // L3-13: remount add-modal content on each open (pairs with form reset).
  const addModalOpenKey = useModalOpenKey(addOpen);
  // Active segment drilldown (Fix 7): when set, the list is scoped to one
  // behavioral band and a dismissible chip is shown. Seeded from the prop and
  // kept in sync so re-entering the tab from a different segment re-scopes.
  const [segment, setSegment] = useState<string>(initialSegment ?? "");
  useEffect(() => {
    setSegment(initialSegment ?? "");
    setPage(1);
  }, [initialSegment]);
  // True when the details modal is showing a stale list-row snapshot because the
  // fresh fetch failed (Fix 8) — surfaced as a degraded-data banner + retry
  // instead of silently letting the operator edit from stale data.
  const [detailsStale, setDetailsStale] = useState(false);
  const [detailsRetrying, setDetailsRetrying] = useState(false);
  // L5-6: true while the details modal is open and a fresh fetch is in flight.
  const [detailsLoading, setDetailsLoading] = useState(false);

  useEffect(() => {
    let cancelled = false;
    getBusiness(businessId)
      .then((data) => {
        if (!cancelled) {
          setBusinessCurrency(data.default_currency || "USD");
        }
      })
      .catch((err) => {
        console.error("Failed to load business currency:", err);
      });
    getLoyalty(businessId)
      .then((data) => {
        if (!cancelled) {
          setLoyaltyTiers(
            [...(data.tiers || [])].sort(
              (a, b) => a.min_lifetime_spent - b.min_lifetime_spent,
            ),
          );
          setLoyaltyProgram(data.program ?? null);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setLoyaltyTiers([]);
          setLoyaltyProgram(null);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [businessId]);

  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  const t = useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      const fullKey = `businessDashboard.crm.${key}`;
      const result = getTranslation(fullKey, currentLocale, params);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  const loadCustomers = useCallback(async () => {
    const requestId = ++requestIdRef.current;
    const firstLoad = !hasLoadedRef.current;
    try {
      if (firstLoad) {
        setLoading(true);
      } else {
        setIsRefetching(true);
      }
      // Sort is resolved server-side over the whole base (fix 6); only send it
      // for whitelisted columns. When a segment is active, the server scopes to
      // that band and ignores sort/tier — the client passes only the segment.
      const response = await businessCRMAPI.getCustomers(
        businessId,
        page,
        20,
        debouncedSearch,
        tier,
        segment
          ? { segment }
          : SERVER_SORTABLE.has(sortKey)
            ? { sortBy: sortKey, sortDir }
            : {},
      );
      // Discard stale responses — only the latest in-flight request wins.
      if (requestId !== requestIdRef.current) return;
      setCustomers(response.customers || []);
      setTotalPages(response.total_pages || 1);
      setSummary(response.summary ?? null);
    } catch (error) {
      // Only the latest in-flight request surfaces a failure — a stale/raced
      // request that lost the id race stays silent so we don't toast over a
      // newer load that may still succeed.
      if (requestId !== requestIdRef.current) return;
      console.error("Failed to load customers:", error);
      toast.error(t("toasts.loadError"));
    } finally {
      if (requestId === requestIdRef.current) {
        hasLoadedRef.current = true;
        setHasLoaded(true);
        setLoading(false);
        setIsRefetching(false);
      }
    }
  }, [businessId, page, debouncedSearch, tier, segment, sortKey, sortDir, t]);

  useEffect(() => {
    if (!isLocked) {
      loadCustomers();
    }
  }, [loadCustomers, isLocked]);

  // Debounce the search term that drives the network call so typing a name
  // fires a single paginated fetch instead of one per keystroke.
  useEffect(() => {
    const handle = setTimeout(() => setDebouncedSearch(search), 350);
    return () => clearTimeout(handle);
  }, [search]);

  const handleSearch = (value: string) => {
    setSearch(value);
    setPage(1);
  };

  const handleTierChange = (next: TierFilter) => {
    setTier(next);
    setPage(1);
  };

  // L5-5: clear add-customer fields whenever the modal closes or (re)opens so
  // values never resurrect during the exit fade / next open.
  const resetNewCustomerForm = () => {
    setNewCustomer({ name: "", email: "", phone: "" });
    setAddFieldError(null);
  };
  const setAddOpen = (open: boolean) => {
    if (isAddControlled) {
      onAddModalOpenChange?.(open);
      return;
    }
    setAddModal(open);
  };
  const openAddModal = () => {
    resetNewCustomerForm();
    setAddOpen(true);
  };
  const closeAddModal = () => {
    resetNewCustomerForm();
    setAddOpen(false);
  };
  const prevAddOpen = useRef(false);
  useEffect(() => {
    if (addOpen && !prevAddOpen.current) {
      resetNewCustomerForm();
    }
    prevAddOpen.current = addOpen;
  }, [addOpen]);

  // L5-6: open the details modal first (with list-row snapshot), then refresh
  // from the API. Awaiting the fetch before setDetailsModal(true) races with
  // post-save loadCustomers() which swaps the table for a skeleton and
  // unmounts the pressed row before the modal opens.
  const handleViewDetails = async (customer: CustomerBusiness) => {
    setSelectedCustomer(customer);
    setDetailsStale(false);
    setDetailsLoading(true);
    setDetailsModal(true);
    try {
      const details = await businessCRMAPI.getCustomerDetails(
        businessId,
        customer.id,
      );
      setSelectedCustomer(details.customer);
      setDetailsStale(false);
    } catch {
      // Fresh fetch failed — keep the list-row snapshot but flag degraded data.
      setDetailsStale(true);
    } finally {
      setDetailsLoading(false);
    }
  };

  // L5-9: soft-unlink from this business (not global hard delete).
  const [customerToUnlink, setCustomerToUnlink] =
    useState<CustomerBusiness | null>(null);

  const confirmUnlinkCustomer = async () => {
    if (!customerToUnlink) return;
    try {
      await businessCRMAPI.unlinkCustomer(businessId, customerToUnlink.id);
      toast.success(t("remove.success"));
      setCustomerToUnlink(null);
      await loadCustomers();
    } catch {
      toast.error(t("remove.error"));
    }
  };

  // Retry the details fetch from inside the modal after a degraded open (fix 8).
  const retryDetails = async () => {
    if (!selectedCustomer) return;
    setDetailsRetrying(true);
    try {
      const details = await businessCRMAPI.getCustomerDetails(
        businessId,
        selectedCustomer.id,
      );
      setSelectedCustomer(details.customer);
      setDetailsStale(false);
    } catch {
      // Stay degraded; the banner remains so the operator knows the data is old.
    } finally {
      setDetailsRetrying(false);
    }
  };

  const handleUpdateNotes = async () => {
    if (!selectedCustomer) return;
    const result = await runWithFeedback(
      () =>
        businessCRMAPI.updateCustomerNotes(
          businessId,
          selectedCustomer.id,
          notes,
        ),
      {
        success: t("toasts.notesSaved"),
        error: t("toasts.notesError"),
        setBusy: setNotesSaving,
      },
    );
    // On failure the modal stays open so the operator can retry; only close
    // and refresh once the save actually succeeds.
    if (result === undefined) return;
    setSelectedCustomer({ ...selectedCustomer, notes });
    setNotesModal(false);
    // L5-10: return to Detalles after a successful notes save.
    setDetailsModal(true);
    void loadCustomers();
  };

  const handleUpdateAllergies = async () => {
    if (!selectedCustomer) return;
    const result = await runWithFeedback(
      () =>
        businessCRMAPI.updateCustomerAllergies(
          businessId,
          selectedCustomer.id,
          allergies,
        ),
      {
        success: t("toasts.allergiesSaved"),
        error: t("toasts.allergiesError"),
        setBusy: setAllergiesSaving,
      },
    );
    if (result === undefined) return;
    setSelectedCustomer({ ...selectedCustomer, allergies });
    setAllergiesModal(false);
    setDetailsModal(true);
    void loadCustomers();
  };

  const handleAdjustPoints = async () => {
    if (!selectedCustomer) return;
    const parsed = Number.parseInt(pointsValue.trim(), 10);
    if (!Number.isFinite(parsed) || parsed < 0) {
      toast.error(t("toasts.pointsAdjustError"));
      return;
    }
    const result = await runWithFeedback(
      () =>
        businessCRMAPI.adjustCustomerLoyaltyPoints(
          businessId,
          selectedCustomer.id,
          parsed,
          pointsReason.trim(),
        ),
      {
        success: t("toasts.pointsAdjusted"),
        error: t("toasts.pointsAdjustError"),
        setBusy: setPointsSaving,
      },
    );
    if (result === undefined) return;
    setSelectedCustomer({ ...selectedCustomer, loyalty_points: parsed });
    setPointsModal(false);
    setDetailsModal(true);
    void loadCustomers();
  };

  const handleUpdateTags = async () => {
    if (!selectedCustomer) return;
    const serializedTags = serializeCustomerTagsInput(tags);
    const result = await runWithFeedback(
      () =>
        businessCRMAPI.updateCustomerTags(
          businessId,
          selectedCustomer.id,
          serializedTags,
        ),
      {
        success: t("toasts.tagsSaved"),
        error: t("toasts.tagsError"),
        setBusy: setTagsSaving,
      },
    );
    if (result === undefined) return;
    setSelectedCustomer({ ...selectedCustomer, tags: serializedTags });
    setTagsModal(false);
    // L5-10: return to Detalles after a successful tags save.
    setDetailsModal(true);
    void loadCustomers();
  };

  const handleAddCustomer = async () => {
    // L5-3: localized validation instead of silent disable + native email bubble.
    if (!newCustomer.name.trim()) {
      setAddFieldError(t("validation.nameRequired"));
      return;
    }
    if (!newCustomer.email.trim()) {
      setAddFieldError(t("validation.emailRequired"));
      return;
    }
    if (!isValidEmail(newCustomer.email)) {
      setAddFieldError(t("validation.emailInvalid"));
      return;
    }
    if (
      newCustomer.phone.trim() &&
      !isValidPhone(newCustomer.phone)
    ) {
      setAddFieldError(
        t("validation.phoneInvalid"),
      );
      return;
    }
    setAddFieldError(null);
    // L5-4: omit success toast here — branch created-vs-linked after response.
    const result = await runWithFeedback(
      () =>
        businessCRMAPI.addCustomer(businessId, {
          name: newCustomer.name.trim(),
          email: newCustomer.email.trim(),
          phone: newCustomer.phone.trim() || undefined,
        }),
      {
        error: t("toasts.addCustomerError"),
        setBusy: setAddLoading,
      },
    );
    // On failure the modal stays open with the inputs intact so the operator
    // can fix and retry; only close + reset + refresh once the add succeeds.
    if (result === undefined) return;
    const created =
      result &&
      typeof result === "object" &&
      "created" in result &&
      (result as { created?: boolean }).created === false
        ? false
        : true;
    toast.success(
      created ? t("toasts.addCustomerSuccess") : t("toasts.customerLinked"),
    );
    closeAddModal();
    void loadCustomers();
  };

  // Server-side sort (fix 6): toggling a header re-queries the whole base and
  // resets to page 1, so the top rows are truly the highest/lowest — not just a
  // reordering of the visible page. Only whitelisted columns get here.
  const toggleSort = (key: string) => {
    if (segment) return; // sort is disabled while a segment scopes the list
    if (sortKey === key) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortKey(key);
      setSortDir(key === "name" ? "asc" : "desc");
    }
    setPage(1);
  };

  const formatDate = (dateString?: string) => {
    if (!dateString) return t("never");
    return formatDisplayDate(dateString, currentLocale);
  };

  // A sortable column header: renders a chevron affordance only for
  // server-sortable columns, disabled while a segment scopes the list.
  const renderSortHeader = (key: string, label: string) => {
    const active = sortKey === key && !segment;
    return (
      <button
        type="button"
        className="flex items-center gap-1 hover:text-brand-dark disabled:cursor-default disabled:opacity-70"
        onClick={() => toggleSort(key)}
        disabled={!!segment}
        aria-label={label}
      >
        {label}
        {active ? (
          sortDir === "asc" ? (
            <ArrowUp className="h-3.5 w-3.5" aria-hidden />
          ) : (
            <ArrowDown className="h-3.5 w-3.5" aria-hidden />
          )
        ) : (
          <ChevronsUpDown
            className="h-3.5 w-3.5 text-ink-400"
            aria-hidden
          />
        )}
      </button>
    );
  };

  return (
    <div>
      {/* CRMManager mounts this tab only when CRM is enabled (the manager
          renders the activation card itself when disabled), so the body
          opens straight into the customer list. */}
      {loading && !hasLoaded ? (
        <CRMCustomersSkeleton />
      ) : customers.length === 0 &&
        tier === "All" &&
        debouncedSearch === "" &&
        (summary ? summary.total_customers === 0 : true) &&
        !isRefetching ? (
        // Truly-empty base only — an empty page caused by a tier filter or
        // search falls through to the list UI + the inline no-match state.
        <CRMCustomersEmptyPanel
          title={t("empty.customers.title")}
          subtitle={t("empty.customers.subtitle")}
          actionLabel={t("addCustomer")}
          onAction={openAddModal}
          secondaryActionLabel={
            onNavigateToTab ? t("empty.customers.captureCta") : undefined
          }
          onSecondaryAction={
            onNavigateToTab ? () => onNavigateToTab("tables") : undefined
          }
        />
      ) : (
      <>
      {/* Summary Cards — total / active / avg spend / top-tier */}
      <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-3 mb-6">
        {(() => {
          // Prefer server-computed whole-base aggregates (audit L6 #3); fall
          // back to the current page only if the meta is absent (older API).
          let totalCustomers: number;
          let activeMonth: number;
          let avgSpent: number;
          let topTierCount: number;
          if (summary) {
            totalCustomers = summary.total_customers;
            activeMonth = summary.active_this_month;
            avgSpent = summary.avg_lifetime_spend;
            topTierCount = summary.top_tier_count;
          } else {
            const now = Date.now();
            const monthMs = 30 * 24 * 60 * 60 * 1000;
            totalCustomers = customers.length;
            activeMonth = customers.filter((c) => {
              if (!c.last_visit_at) return false;
              const ts = new Date(c.last_visit_at).getTime();
              return Number.isFinite(ts) && now - ts <= monthMs;
            }).length;
            const totalSpentSum = customers.reduce(
              (acc, c) => acc + (c.total_spent || 0),
              0,
            );
            avgSpent =
              customers.length > 0 ? totalSpentSum / customers.length : 0;
            topTierCount = customers.filter(
              (c) => c.loyalty_tier === "Gold",
            ).length;
          }
          const summaryCards: Array<{
            label: string;
            value: React.ReactNode;
          }> = [
            { label: t("summary.totalCustomers"), value: totalCustomers },
            { label: t("summary.activeThisMonth"), value: activeMonth },
            {
              label: t("summary.avgLifetimeSpend"),
              value: (
                <CurrencyPrice
                  amount={avgSpent}
                  fromCurrency={businessCurrency}
                  displayCurrency={businessCurrency}
                  locale={intlLocale}
                />
              ),
            },
            { label: t("summary.topTierCustomers"), value: topTierCount },
          ];
          return summaryCards.map((c) => (
            <PremiumPanel
              key={c.label}
              withTexture={false}
              className="p-4"
            >
              <p className="text-xs uppercase text-ink-500 tracking-wider">
                {c.label}
              </p>
              <p className="text-xl sm:text-2xl font-semibold text-ink-900 mt-1 tabular-nums">
                {c.value}
              </p>
            </PremiumPanel>
          ));
        })()}
      </div>

      {/* Search and Filters — Add Customer lives in the page header so this
          row stays filter-only (search + tier chips + loyalty rules). */}
      <Card className="rounded-2xl border border-warm-200 bg-white shadow-sm shadow-warm-900/5">
        <CardBody>
          <div
            className="flex flex-col gap-3"
            data-testid="crm-customer-filters"
          >
            <div className="flex-1">
              <Input
                placeholder={t("searchPlaceholder")}
                value={search}
                onValueChange={handleSearch}
                startContent={<Search size={16} className="text-ink-400" />}
                // L5-1: never gate this input on a refetch — the spinner in
                // endContent + aria-busy carry the loading signal instead.
                aria-busy={isRefetching || undefined}
                endContent={
                  isRefetching ? (
                    <span
                      className="h-3.5 w-3.5 animate-spin rounded-full border-2 border-ink-300 border-t-brand"
                      aria-hidden
                    />
                  ) : null
                }
                classNames={{
                  input: "",
                  inputWrapper: "border-warm-200 bg-white",
                }}
              />
            </div>
            <div
              className="flex gap-1 flex-wrap"
              role="group"
              aria-label={t("filterByTierAria")}
            >
              {TIERS.map((option) => {
                const active = tier === option;
                // Bronze/Silver/Gold are brand-like tier names that stay
                // identical across locales; only the "All" chip needs to
                // localize ("Todos" in Spanish).
                const label = option === "All" ? t("tiers.all") : option;
                const threshold = loyaltyTiers.find(
                  (lt) => lt.name.toLowerCase() === option.toLowerCase(),
                );
                const title =
                  option !== "All" && threshold
                    ? t("tiers.thresholdHint", {
                        tier: option,
                        amount: formatCurrency(
                          threshold.min_lifetime_spent,
                          businessCurrency,
                          undefined,
                          intlLocale,
                        ),
                      })
                    : undefined;
                return (
                  <button
                    key={option}
                    type="button"
                    onClick={() => handleTierChange(option)}
                    aria-pressed={active}
                    title={title}
                    // While a segment scopes the list, tier chips are disabled —
                    // the two filters don't combine in the UI.
                    disabled={!!segment}
                    className={`px-3 py-1 rounded-full text-sm transition-colors disabled:opacity-40 disabled:cursor-not-allowed ${
                      active
                        ? "bg-brand text-white"
                        : "border border-warm-200 text-ink-700 hover:bg-warm-50"
                    }`}
                  >
                    {label}
                  </button>
                );
              })}
            </div>
            <div
              className="space-y-1"
              data-testid="crm-loyalty-rules"
            >
              <p
                className="text-xs text-ink-500"
                data-testid="crm-tier-legend"
              >
                {loyaltyTiers.length > 0
                  ? `${t("tiers.legendPrefix")} ${loyaltyTiers
                      .map(
                        (lt) =>
                          `${lt.name} ≥ ${formatCurrency(
                            lt.min_lifetime_spent,
                            businessCurrency,
                            undefined,
                            intlLocale,
                          )}`,
                      )
                      .join(" · ")}`
                  : t("tiers.fallbackRule")}
              </p>
              <p
                className="text-xs text-ink-500"
                data-testid="crm-points-rule"
              >
                {loyaltyProgram
                  ? t("points.rule", {
                      rate: String(loyaltyProgram.points_per_dollar),
                      amount: formatCurrency(
                        1,
                        businessCurrency,
                        undefined,
                        intlLocale,
                      ),
                    })
                  : t("points.fallbackRule")}
              </p>
            </div>
          </div>
          {segment && (
            // Visible, dismissible filter chip for a Segments/Marketing
            // drilldown (fix 7). Dismissing clears the segment and returns to the
            // full list.
            <div className="mt-3 flex items-center gap-2">
              <span className="text-xs font-medium text-ink-500">
                {t("segmentFilter.label")}
              </span>
              <Chip
                variant="flat"
                onClose={() => {
                  setSegment("");
                  setPage(1);
                  onClearSegment?.();
                }}
                endContent={<X className="h-3 w-3" />}
                classNames={{
                  base: "bg-brand/10 border border-brand/20",
                  content: "text-brand font-medium",
                }}
              >
                {SEGMENT_LABEL_KEYS[segment]
                  ? t(SEGMENT_LABEL_KEYS[segment])
                  : segment}
              </Chip>
            </div>
          )}
        </CardBody>
      </Card>

      {/* Customers Table */}
      <Card className="rounded-2xl border border-warm-200 bg-white shadow-sm shadow-warm-900/5">
        <CardHeader className="flex flex-row items-center justify-between gap-3 pb-3">
          <h3 className="text-lg font-semibold text-ink-950">{t("customerList")}</h3>
          {!hideListAddButton ? (
            <Button
              onPress={openAddModal}
              className="bg-brand text-white font-medium hover:bg-brand-dark"
              startContent={<UserPlus size={16} />}
              data-testid="crm-add-customer"
            >
              {t("addCustomer")}
            </Button>
          ) : null}
        </CardHeader>
        <CardBody>
          {(() => {
            // Tier/segment filtering AND sort are server-side, so `customers`
            // already holds the correct, correctly-ordered page — render as-is.
            const tierFiltered = customers;
            if (tierFiltered.length === 0) {
              return (
                <div className="px-6 py-12 text-center">
                  <div className="mx-auto mb-6 flex h-16 w-16 items-center justify-center rounded-2xl border border-warm-200 bg-warm-50">
                    <Star className="w-8 h-8 text-ink-400" />
                  </div>
                  <h3 className="text-lg font-semibold text-ink-900 mb-2">
                    {t("noCustomers")}
                  </h3>
                  {tier !== "All" && (
                    <p className="mb-6 text-sm text-ink-600">
                      {t("noCustomersInTier")}
                    </p>
                  )}
                  {tier !== "All" && (
                    <Button
                      onPress={() => handleTierChange("All")}
                      className="bg-brand text-white font-medium hover:bg-brand-dark"
                    >
                      {t("clearTierFilter")}
                    </Button>
                  )}
                </div>
              );
            }
            return (
              <Table aria-label={t("tableAria")}>
                <TableHeader>
                  <TableColumn>{renderSortHeader("name", t("name"))}</TableColumn>
                  <TableColumn className="hidden md:table-cell">{t("email")}</TableColumn>
                  {/* loyalty_points is NOT server-sortable — no affordance. */}
                  <TableColumn>{t("loyaltyPoints")}</TableColumn>
                  <TableColumn>
                    {renderSortHeader("total_spent", t("totalSpent"))}
                  </TableColumn>
                  <TableColumn className="hidden md:table-cell">
                    {renderSortHeader("visit_count", t("visits"))}
                  </TableColumn>
                  <TableColumn>
                    {renderSortHeader("last_visit_at", t("lastVisit"))}
                  </TableColumn>
                  <TableColumn>{t("actions")}</TableColumn>
                </TableHeader>
                <TableBody>
                  {tierFiltered.map((customer) => (
                    <TableRow key={customer.id}>
                      <TableCell>
                        <div>
                          <button
                            type="button"
                            className="font-medium text-left text-brand-dark hover:underline"
                            onClick={() => handleViewDetails(customer)}
                          >
                            {customerName(customer) || t("unknown")}
                          </button>
                          {customer.loyalty_tier && (
                            <Chip
                              size="sm"
                              color="warning"
                              variant="flat"
                              title={
                                loyaltyTiers.find(
                                  (lt) =>
                                    lt.name.toLowerCase() ===
                                    customer.loyalty_tier?.toLowerCase(),
                                )
                                  ? t("tiers.thresholdHint", {
                                      tier: customer.loyalty_tier,
                                      amount: formatCurrency(
                                        loyaltyTiers.find(
                                          (lt) =>
                                            lt.name.toLowerCase() ===
                                            customer.loyalty_tier?.toLowerCase(),
                                        )!.min_lifetime_spent,
                                        businessCurrency,
                                        undefined,
                                        intlLocale,
                                      ),
                                    })
                                  : undefined
                              }
                            >
                              {customer.loyalty_tier}
                            </Chip>
                          )}
                        </div>
                      </TableCell>
                      <TableCell className="hidden md:table-cell">
                        {/* Email column header already implies "email";
                            repeating the mail icon on every row was just
                            visual noise. */}
                        <span className="text-sm">
                          {customerEmail(customer)}
                        </span>
                      </TableCell>
                      <TableCell>
                        <div className="flex items-center gap-1">
                          <Star size={16} className="text-amber-500" />
                          <span className="font-medium">
                            {customer.loyalty_points}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell>
                        <span className="font-medium">
                          <CurrencyPrice
                            amount={customer.total_spent}
                            fromCurrency={businessCurrency}
                            displayCurrency={businessCurrency}
                            locale={intlLocale}
                          />
                        </span>
                      </TableCell>
                      <TableCell className="hidden md:table-cell">{customerVisits(customer)}</TableCell>
                      <TableCell>
                        {formatDate(customer.last_visit_at)}
                      </TableCell>
                      <TableCell>
                        <div className="flex flex-wrap items-center gap-1">
                          <Button
                            size="sm"
                            variant="flat"
                            className="bg-brand/10 font-medium text-brand-dark"
                            onPress={() => handleViewDetails(customer)}
                            startContent={<Eye size={16} />}
                            data-testid="crm-customer-view"
                          >
                            {t("view")}
                          </Button>
                          {/* Remove is destructive and confirm-gated — keep it
                              icon-only so it never reads equal-weight to View. */}
                          <Button
                            size="sm"
                            isIconOnly
                            variant="light"
                            color="danger"
                            aria-label={`${t("remove.action")} ${customerName(customer)}`}
                            title={t("remove.action")}
                            onPress={() => setCustomerToUnlink(customer)}
                            data-testid="crm-customer-remove"
                          >
                            <UserMinus size={16} />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            );
          })()}

          {/* Pagination */}
          {totalPages > 1 && (
            <div className="flex justify-center mt-4">
              <Pagination
                total={totalPages}
                page={page}
                onChange={setPage}
                showControls
              />
            </div>
          )}
        </CardBody>
      </Card>
      </>
      )}

      {/* Customer Details Modal */}
      <Modal
        isOpen={detailsModal}
        onClose={() => setDetailsModal(false)}
        size="4xl"
        scrollBehavior="inside"
      >
        <ModalContent className="overflow-hidden rounded-3xl border border-warm-200 bg-white shadow-2xl shadow-warm-900/15">
          <ModalHeader className="border-b border-warm-200/80 bg-warm-50/70 px-6 py-5 text-lg font-semibold text-ink-950">{t("customerDetails")}</ModalHeader>
          <ModalBody className="px-6 py-5">
            {detailsLoading && (
              <p
                className="mb-3 text-sm text-ink-500"
                data-testid="crm-details-loading"
                role="status"
              >
                {/* R2-3c: `t(...) || "…"` was dead code — getTranslation never
                    returns a falsy value (a total miss yields the sentence-cased
                    key leaf), so the English fallback could never render and the
                    missing businessDashboard.crm.loading key surfaced as
                    "Loading" in every locale. The key now exists in en + es. */}
                {t("loading")}
              </p>
            )}
            {detailsStale && (
              // Degraded-data indicator (fix 8): the fresh fetch failed, so these
              // fields are the stale list-row snapshot. Offer a retry rather than
              // silently letting the operator edit from stale data.
              <div className="mb-4 flex flex-col gap-2 rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-800 sm:flex-row sm:items-center sm:justify-between">
                <span className="flex items-center gap-2 font-medium">
                  <AlertTriangle className="h-4 w-4" aria-hidden />
                  {t("details.staleData")}
                </span>
                <Button
                  size="sm"
                  variant="flat"
                  isLoading={detailsRetrying}
                  onPress={() => void retryDetails()}
                  className="border border-amber-300 bg-white font-semibold text-amber-800"
                >
                  {t("details.retry")}
                </Button>
              </div>
            )}
            {selectedCustomer && (
              <div className="space-y-4">
                {/* Basic Info */}
                <div className="grid grid-cols-2 gap-4">
                  <div>
                    <p className="text-sm text-ink-600">{t("name")}</p>
                    <p className="font-medium">
                      {selectedCustomer.customer?.name}
                    </p>
                  </div>
                  <div>
                    <p className="text-sm text-ink-600">{t("email")}</p>
                    <p className="font-medium">
                      {selectedCustomer.customer?.email}
                    </p>
                  </div>
                  {selectedCustomer.customer?.phone && (
                    <div>
                      <p className="text-sm text-ink-600">{t("phone")}</p>
                      <p className="font-medium">
                        {selectedCustomer.customer.phone}
                      </p>
                    </div>
                  )}
                </div>

                {/* Stats */}
                <div className="grid grid-cols-3 gap-4 rounded-2xl border border-warm-200/80 bg-warm-50/70 p-4">
                  <div className="text-center">
                    <p className="text-2xl font-bold text-ink-950">
                      {selectedCustomer.loyalty_points}
                    </p>
                    <p className="text-sm text-ink-600">
                      {t("loyaltyPoints")}
                    </p>
                    <p
                      className="mt-1 text-xs text-ink-500"
                      data-testid="crm-details-points-rule"
                    >
                      {t("points.independentHint")}
                    </p>
                  </div>
                  <div className="text-center">
                    <p className="text-2xl font-bold text-ink-950">
                      <CurrencyPrice
                        amount={selectedCustomer.total_spent}
                        fromCurrency={businessCurrency}
                        displayCurrency={businessCurrency}
                        locale={intlLocale}
                      />
                    </p>
                    <p className="text-sm text-ink-600">{t("totalSpent")}</p>
                  </div>
                  <div className="text-center">
                    <p className="text-2xl font-bold text-ink-950">
                      {selectedCustomer.visit_count}
                    </p>
                    <p className="text-sm text-ink-600">{t("visits")}</p>
                  </div>
                </div>

                {/* Preferences */}
                <div>
                  <p className="text-sm font-medium mb-2">
                    {t("marketingPreferences")}
                  </p>
                  <div className="flex gap-2">
                    {selectedCustomer.opt_in_email && (
                      <Chip size="sm" color="success" variant="flat">
                        {t("email")}
                      </Chip>
                    )}
                    {selectedCustomer.opt_in_sms && (
                      <Chip size="sm" color="success" variant="flat">
                        {t("sms")}
                      </Chip>
                    )}
                    {selectedCustomer.opt_in_marketing && (
                      <Chip size="sm" color="success" variant="flat">
                        {t("marketing")}
                      </Chip>
                    )}
                  </div>
                </div>

                {/* Notes */}
                {selectedCustomer.notes && (
                  <div>
                    <p className="text-sm font-medium mb-2">{t("notes")}</p>
                    <p className="text-sm text-ink-600">
                      {selectedCustomer.notes}
                    </p>
                  </div>
                )}

                {/* Allergies */}
                <div>
                  <p className="text-sm font-medium mb-2">{t("allergies")}</p>
                  <p className="text-sm text-ink-600" data-testid="crm-customer-allergies">
                    {formatAllergiesDisplay(selectedCustomer.allergies) || "—"}
                  </p>
                </div>

                {/* Tags */}
                {parseCustomerTags(selectedCustomer.tags).length > 0 && (
                  <div>
                    <p className="text-sm font-medium mb-2">{t("tags")}</p>
                    <div className="flex flex-wrap gap-2">
                      {parseCustomerTags(selectedCustomer.tags).map((tag) => (
                        <Chip
                          key={tag}
                          size="sm"
                          color="primary"
                          variant="flat"
                        >
                          {tag}
                        </Chip>
                      ))}
                    </div>
                  </div>
                )}
              </div>
            )}
          </ModalBody>
          <ModalFooter className="border-t border-warm-200/80 bg-warm-50/70 px-0 py-0">
            <div
              data-testid="crm-details-actions"
              className="flex w-full flex-col items-stretch gap-2 px-6 py-4 sm:flex-row sm:flex-wrap sm:items-center sm:justify-end"
            >
            {onNavigateToTab && selectedCustomer ? (
              <Button
                variant="flat"
                className="h-auto min-h-10 whitespace-normal bg-brand/10 font-medium text-brand-dark"
                onPress={() =>
                  onNavigateToTab(
                    `bills?billTab=history&billCustomer=${selectedCustomer.customer_id}`,
                  )
                }
              >
                {t("details.viewBills")}
              </Button>
            ) : null}
            <Button
              variant="light"
              className="h-auto min-h-10 whitespace-normal"
              onPress={() => {
                setNotes(selectedCustomer?.notes || "");
                setNotesModal(true);
                setDetailsModal(false);
              }}
            >
              {t("editNotes")}
            </Button>
            <Button
              variant="light"
              className="h-auto min-h-10 whitespace-normal"
              onPress={() => {
                setAllergies(
                  formatAllergiesDisplay(selectedCustomer?.allergies),
                );
                setAllergiesModal(true);
                setDetailsModal(false);
              }}
            >
              {t("editAllergies")}
            </Button>
            <Button
              variant="light"
              className="h-auto min-h-10 whitespace-normal"
              onPress={() => {
                setPointsValue(String(selectedCustomer?.loyalty_points ?? 0));
                setPointsReason("");
                setPointsModal(true);
                setDetailsModal(false);
              }}
              data-testid="crm-adjust-points"
            >
              {t("adjustPoints")}
            </Button>
            <Button
              variant="light"
              className="h-auto min-h-10 whitespace-normal"
              onPress={() => {
                setTags(parseCustomerTags(selectedCustomer?.tags).join(", "));
                setTagsModal(true);
                setDetailsModal(false);
              }}
            >
              {t("editTags")}
            </Button>
            <Button
              className="h-auto min-h-10 whitespace-normal"
              onPress={() => setDetailsModal(false)}
            >
              {t("close")}
            </Button>
            </div>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* Notes Modal */}
      <Modal isOpen={notesModal} onClose={() => setNotesModal(false)} scrollBehavior="inside">
        <ModalContent className="overflow-hidden rounded-3xl border border-warm-200 bg-white shadow-2xl shadow-warm-900/15">
          <ModalHeader className="border-b border-warm-200/80 bg-warm-50/70 px-6 py-5 text-lg font-semibold text-ink-950">{t("editNotes")}</ModalHeader>
          <ModalBody className="px-6 py-5">
            <Textarea
              label={t("notes")}
              placeholder={t("notesPlaceholder")}
              description={t("notesHint")}
              value={notes}
              onValueChange={setNotes}
              minRows={4}
            />
          </ModalBody>
          <ModalFooter className="border-t border-warm-200/80 bg-warm-50/70 px-6 py-4">
            <Button variant="light" onPress={() => setNotesModal(false)}>
              {t("cancel")}
            </Button>
            <Button
              className="bg-brand text-white hover:bg-brand-dark"
              onPress={handleUpdateNotes}
              isLoading={notesSaving}
            >
              {t("save")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* Allergies Modal */}
      <Modal
        isOpen={allergiesModal}
        onClose={() => setAllergiesModal(false)}
        scrollBehavior="inside"
      >
        <ModalContent className="overflow-hidden rounded-3xl border border-warm-200 bg-white shadow-2xl shadow-warm-900/15">
          <ModalHeader className="border-b border-warm-200/80 bg-warm-50/70 px-6 py-5 text-lg font-semibold text-ink-950">
            {t("editAllergies")}
          </ModalHeader>
          <ModalBody className="px-6 py-5">
            <Textarea
              label={t("allergies")}
              placeholder={t("allergiesPlaceholder")}
              value={allergies}
              onValueChange={setAllergies}
              minRows={3}
              data-testid="crm-allergies-input"
            />
          </ModalBody>
          <ModalFooter className="border-t border-warm-200/80 bg-warm-50/70 px-6 py-4">
            <Button variant="light" onPress={() => setAllergiesModal(false)}>
              {t("cancel")}
            </Button>
            <Button
              className="bg-brand text-white hover:bg-brand-dark"
              onPress={handleUpdateAllergies}
              isLoading={allergiesSaving}
            >
              {t("save")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* Adjust Points Modal */}
      <Modal
        isOpen={pointsModal}
        onClose={() => setPointsModal(false)}
        scrollBehavior="inside"
      >
        <ModalContent className="overflow-hidden rounded-3xl border border-warm-200 bg-white shadow-2xl shadow-warm-900/15">
          <ModalHeader className="border-b border-warm-200/80 bg-warm-50/70 px-6 py-5 text-lg font-semibold text-ink-950">
            {t("adjustPointsTitle")}
          </ModalHeader>
          <ModalBody className="px-6 py-5 space-y-3">
            <p className="text-sm text-ink-600">{t("adjustPointsDescription")}</p>
            <Input
              label={t("loyaltyPoints")}
              type="number"
              min={0}
              value={pointsValue}
              onValueChange={setPointsValue}
              data-testid="crm-points-input"
            />
            <Input
              label={t("adjustPointsReason")}
              placeholder={t("adjustPointsReasonPlaceholder")}
              value={pointsReason}
              onValueChange={setPointsReason}
            />
          </ModalBody>
          <ModalFooter className="border-t border-warm-200/80 bg-warm-50/70 px-6 py-4">
            <Button variant="light" onPress={() => setPointsModal(false)}>
              {t("cancel")}
            </Button>
            <Button
              className="bg-brand text-white hover:bg-brand-dark"
              onPress={handleAdjustPoints}
              isLoading={pointsSaving}
              data-testid="crm-points-save"
            >
              {t("save")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* Tags Modal */}
      <Modal isOpen={tagsModal} onClose={() => setTagsModal(false)} scrollBehavior="inside">
        <ModalContent className="overflow-hidden rounded-3xl border border-warm-200 bg-white shadow-2xl shadow-warm-900/15">
          <ModalHeader className="border-b border-warm-200/80 bg-warm-50/70 px-6 py-5 text-lg font-semibold text-ink-950">{t("editTags")}</ModalHeader>
          <ModalBody className="px-6 py-5">
            <Input
              label={t("tags")}
              placeholder={t("tagsPlaceholder")}
              value={tags}
              onValueChange={setTags}
            />
          </ModalBody>
          <ModalFooter className="border-t border-warm-200/80 bg-warm-50/70 px-6 py-4">
            <Button variant="light" onPress={() => setTagsModal(false)}>
              {t("cancel")}
            </Button>
            <Button
              className="bg-brand text-white hover:bg-brand-dark"
              onPress={handleUpdateTags}
              isLoading={tagsSaving}
            >
              {t("save")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* Add Customer Modal — L5-5: reset form on close so reopen never
          resurrects values during the exit fade / next open.
          L3-13: key-by-open-count remounts content on each open. */}
      <Modal
        isOpen={addOpen}
        onClose={closeAddModal}
        scrollBehavior="inside"
      >
        <ModalContent
          key={addModalOpenKey}
          className="overflow-hidden rounded-3xl border border-warm-200 bg-white shadow-2xl shadow-warm-900/15"
        >
          <ModalHeader className="border-b border-warm-200/80 bg-warm-50/70 px-6 py-5 text-lg font-semibold text-ink-950">{t("addCustomer")}</ModalHeader>
          <ModalBody className="px-6 py-5">
            <div className="space-y-3">
              {addFieldError && (
                <p className="text-sm text-rose-700" role="alert" data-testid="crm-add-field-error">
                  {addFieldError}
                </p>
              )}
              <Input
                label={t("name")}
                placeholder={t("customerNamePlaceholder")}
                value={newCustomer.name}
                onValueChange={(v) => setNewCustomer((p) => ({ ...p, name: v }))}
                isRequired
                data-testid="crm-add-name"
              />
              <Input
                label={t("email")}
                type="text"
                inputMode="email"
                placeholder="email@example.com"
                value={newCustomer.email}
                onValueChange={(v) => setNewCustomer((p) => ({ ...p, email: v }))}
                isRequired
                isInvalid={
                  newCustomer.email.trim() !== "" &&
                  !isValidEmail(newCustomer.email)
                }
                errorMessage={
                  newCustomer.email.trim() !== "" &&
                  !isValidEmail(newCustomer.email)
                    ? t("validation.emailInvalid")
                    : undefined
                }
                data-testid="crm-add-email"
              />
              <Input
                label={t("phone")}
                type="tel"
                inputMode="tel"
                placeholder={t("phonePlaceholder")}
                value={newCustomer.phone}
                onValueChange={(v) => setNewCustomer((p) => ({ ...p, phone: v }))}
                isInvalid={
                  newCustomer.phone.trim() !== "" &&
                  !isValidPhone(newCustomer.phone)
                }
                errorMessage={
                  newCustomer.phone.trim() !== "" &&
                  !isValidPhone(newCustomer.phone)
                    ? t("validation.phoneInvalid") ||
                      "Enter a valid phone number."
                    : undefined
                }
                data-testid="crm-add-phone"
              />
              {(!newCustomer.name.trim() || !newCustomer.email.trim()) && (
                <p className="text-xs text-ink-500" data-testid="crm-add-required-hint">
                  {t("validation.nameEmailRequired") ||
                    "Name and email are required to save."}
                </p>
              )}
            </div>
          </ModalBody>
          <ModalFooter className="border-t border-warm-200/80 bg-warm-50/70 px-6 py-4">
            <Button variant="light" onPress={closeAddModal}>
              {t("cancel")}
            </Button>
            <Button
              className="bg-brand text-white hover:bg-brand-dark"
              onPress={handleAddCustomer}
              isLoading={addLoading}
              // L5-3: keep enabled so click explains missing fields (was silent).
              isDisabled={addLoading}
            >
              {t("save")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      <ConfirmationModal
        isOpen={customerToUnlink !== null}
        onOpenChange={() => setCustomerToUnlink(null)}
        isDanger
        title={t("remove.confirmTitle")}
        description={t("remove.confirmDescription").replace(
          "{name}",
          customerToUnlink ? customerName(customerToUnlink) : "",
        )}
        confirmLabel={t("remove.confirmAction")}
        onConfirm={() => confirmUnlinkCustomer()}
      />
    </div>
  );
}
