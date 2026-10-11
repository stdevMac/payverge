"use client";

import React, { useMemo, useState } from "react";
import Link from "next/link";
import { Button } from "@nextui-org/react";
import { Building2, Plus } from "lucide-react";
import type { Business } from "@/api/business";
import type { DashboardSummary } from "@/api/analytics";
import { formatCurrency as formatCurrencyIntl } from "@/api/currency";
import { useSimpleLocale } from "@/i18n/SimpleTranslationProvider";
import { intlLocaleFor } from "@/utils/intlLocale";
import { groupSummariesByCurrency } from "./currencyAggregation";
import { BusinessOverviewPanel } from "./BusinessOverviewPanel";
import { CombinedOverviewPanel } from "./CombinedOverviewPanel";

interface BusinessOverviewListProps {
  businesses: Business[];
  stats: Record<number, DashboardSummary>;
  statsLoading: boolean;
  /** #832: already-translated message when the stats fetch failed. Without it,
   * rows render "—" and the combined view says "No data yet" — a silent
   * failure posing as a quiet day. */
  statsError?: string | null;
  onRetryStats?: () => void;
  onManage: (business: Business) => void;
  navigatingBusinessId: number | null;
  t: (key: string) => string;
  /**
   * Numeric id of the signed-in account, when the session has one (email/OAuth
   * owners). Undefined for web3 sessions, where the backend lists by wallet and
   * no foreign venue can appear.
   */
  viewerUserId?: number | null;
  /**
   * False on the public demo (DEMO_MODE), where POST /inside/businesses is
   * refused with DEMO_MODE_FORBIDDEN: the "Add business" link would only lead
   * to a dead end. Defaults to true.
   */
  canAddBusiness?: boolean;
}

/** Stable operator-facing id when two venues share a brand name (#432). */
function venueDifferentiator(business: Business): string {
  const slug = (business.custom_url || business.business_id || "").trim();
  if (slug) return slug;
  const street = business.address?.street?.trim() || "";
  const city = business.address?.city?.trim() || "";
  if (street && city) return `${street}, ${city}`;
  if (city) return city;
  return `ID ${business.id}`;
}

function venueIdentityLabel(business: Business): string {
  const extra = venueDifferentiator(business);
  return extra ? `${business.name} (${extra})` : business.name;
}

/**
 * True when this venue is listed only because the viewer is a platform admin.
 *
 * `GET /inside/businesses` answers "venues you may open", and for a platform
 * admin that set is `user_id = me OR demo_owner_user_id = me OR is_demo`
 * (backend ListBusinessesForInsideUser). So an admin's hub also lists demo
 * venues belonging to OTHER admin accounts — which is how the demo owner sees
 * four cards carrying only two distinct names (#832): two venues are theirs,
 * two are another admin's clones of the same demo. Ownership is exactly the
 * pair of columns the backend matches on for the owner path.
 *
 * Returns false when the viewer id is unknown, so a session we cannot attribute
 * never gets told its own venues belong to someone else.
 */
export function isAdminScopedVenue(
  business: Business,
  viewerUserId?: number | null,
): boolean {
  if (!viewerUserId) return false;
  if (business.user_id === viewerUserId) return false;
  if (business.demo_owner_user_id === viewerUserId) return false;
  return true;
}

export function BusinessOverviewList({
  businesses,
  stats,
  statsLoading,
  statsError = null,
  onRetryStats,
  onManage,
  navigatingBusinessId,
  t,
  viewerUserId,
  canAddBusiness = true,
}: BusinessOverviewListProps) {
  const { locale } = useSimpleLocale();
  const intlLocale = intlLocaleFor(locale);
  const [selectedBusinessId, setSelectedBusinessId] = useState<number | null>(
    null,
  );

  const selectedBusiness =
    businesses.find((business) => business.id === selectedBusinessId) ?? null;

  const fmt = (amount: number, currency: string) =>
    formatCurrencyIntl(amount, currency, undefined, intlLocale);

  // Split the list by who owns what. Order inside each group is preserved.
  const ownedBusinesses = useMemo(
    () =>
      businesses.filter(
        (business) => !isAdminScopedVenue(business, viewerUserId),
      ),
    [businesses, viewerUserId],
  );
  const adminScopedBusinesses = useMemo(
    () =>
      businesses.filter((business) =>
        isAdminScopedVenue(business, viewerUserId),
      ),
    [businesses, viewerUserId],
  );
  // Only an admin viewing someone else's demo sees the grouped layout; every
  // ordinary owner keeps the flat list exactly as before.
  const hasAdminScopedVenues = adminScopedBusinesses.length > 0;

  // The combined view is titled "your businesses", so it must total only the
  // venues the viewer actually owns. Another admin's demo revenue landing in
  // this owner's portfolio total was the money half of #832.
  const buckets = useMemo(() => {
    const entries = ownedBusinesses
      .map((business) => {
        const summary = stats[business.id];
        return summary
          ? {
              currency: business.default_currency || "USD",
              summary,
            }
          : null;
      })
      .filter(
        (entry): entry is { currency: string; summary: DashboardSummary } =>
          entry !== null,
      );

    return groupSummariesByCurrency(entries);
  }, [ownedBusinesses, stats]);

  const totalTodayBills = buckets.reduce(
    (total, bucket) => total + bucket.summary.today.bills,
    0,
  );
  const totalWeekBills = buckets.reduce(
    (total, bucket) => total + bucket.summary.week.bills,
    0,
  );
  const todayHasActivity =
    totalTodayBills > 0 ||
    buckets.some((bucket) => (bucket.summary.today.revenue as number) > 0);

  const renderVenueRow = (business: Business) => {
    const summary = stats[business.id];
    const currency = business.default_currency || "USD";
    const isSelected = selectedBusinessId === business.id;
    const adminScoped = isAdminScopedVenue(business, viewerUserId);
    // Screen readers get the scope in the same breath as the name, so two
    // identically-named cards are still told apart without the chip.
    const identity = adminScoped
      ? `${venueIdentityLabel(business)} — ${t("overview.scope.adminChip")}`
      : venueIdentityLabel(business);

    return (
      <div
        key={business.id}
        data-testid="portfolio-business-row"
        className={`rounded-2xl border p-2 transition-colors ${
          isSelected
            ? "border-brand/30 bg-white shadow-sm"
            : "border-transparent hover:border-warm-200 hover:bg-white/70"
        }`}
      >
        <button
          type="button"
          aria-label={`${t("overview.viewMetrics")} ${identity}`}
          aria-pressed={isSelected}
          onClick={() => setSelectedBusinessId(business.id)}
          className="flex w-full items-center gap-3 rounded-xl p-1 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand"
        >
          <span className="relative flex h-10 w-10 shrink-0 items-center justify-center overflow-hidden rounded-xl border border-warm-200 bg-warm-50 text-ink-400">
            {business.logo ? (
              // eslint-disable-next-line @next/next/no-img-element
              <img
                src={business.logo}
                alt=""
                className="h-full w-full object-cover"
              />
            ) : (
              <Building2 className="h-4 w-4" aria-hidden="true" />
            )}
            <span
              aria-hidden="true"
              className={`absolute bottom-0.5 right-0.5 h-2 w-2 rounded-full ring-2 ring-white ${
                business.is_active ? "bg-emerald-500" : "bg-ink-300"
              }`}
            />
          </span>
          <span className="min-w-0 flex-1">
            <span
              className="block line-clamp-2 break-words text-sm font-semibold text-ink-900"
              title={business.name}
            >
              {business.name}
            </span>
            <span
              data-testid="portfolio-row-differentiator"
              className="mt-0.5 block truncate text-xs font-medium text-ink-500"
              title={venueDifferentiator(business)}
            >
              {venueDifferentiator(business)}
            </span>
            {adminScoped ? (
              <span
                data-testid="portfolio-row-admin-chip"
                className="mt-1 inline-flex w-fit items-center rounded-full border border-warm-300 bg-warm-50 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-[0.08em] text-ink-500"
              >
                {t("overview.scope.adminChip")}
              </span>
            ) : null}
            <span className="mt-0.5 flex flex-col gap-0.5 text-xs text-ink-500">
              <span className="inline-flex items-center gap-2">
                <span className="font-medium tabular-nums text-ink-700">
                  {summary
                    ? fmt(summary.today.revenue as number, currency)
                    : "—"}
                </span>
                <span aria-hidden="true">·</span>
                <span className="inline-flex gap-1">
                  <span className="tabular-nums">
                    {summary ? summary.today.bills : "—"}
                  </span>
                  <span>{t("overview.paidShort")}</span>
                </span>
                <span data-testid="portfolio-row-grain-today">
                  {t("overview.grainToday")}
                </span>
              </span>
              {summary ? (
                <span
                  data-testid="portfolio-row-grain-week"
                  className="tabular-nums"
                >
                  {fmt(summary.week.revenue as number, currency)}{" "}
                  {t("overview.grainWeek")}
                </span>
              ) : null}
            </span>
          </span>
        </button>
        <Button
          size="sm"
          fullWidth
          aria-label={`${t("overview.openBusiness")} ${identity}`}
          onPress={() => onManage(business)}
          isLoading={navigatingBusinessId === business.id}
          isDisabled={navigatingBusinessId === business.id}
          className="mt-2 h-8 bg-ink-900 text-xs font-semibold text-white"
        >
          {t("overview.openBusiness")}
        </Button>
      </div>
    );
  };

  return (
    <div className="grid overflow-hidden rounded-3xl border border-warm-200 bg-white shadow-sm xl:grid-cols-[22rem_minmax(0,1fr)]">
      <aside className="border-b border-warm-200 bg-warm-50/60 lg:border-b-0 lg:border-r">
        <div className="border-b border-warm-200 p-4">
          <p className="mb-2 px-2 text-[11px] font-semibold uppercase tracking-[0.16em] text-ink-500">
            {t("overview.businessesLabel")}
          </p>
          <button
            type="button"
            aria-pressed={selectedBusinessId === null}
            onClick={() => setSelectedBusinessId(null)}
            className={`w-full rounded-2xl px-3 py-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand ${
              selectedBusinessId === null
                ? "bg-white text-ink-900 shadow-sm ring-1 ring-warm-200"
                : "text-ink-600 hover:bg-white/70 hover:text-ink-900"
            }`}
          >
            <span className="block text-sm font-semibold">
              {t("overview.allBusinesses")}
            </span>
            <span className="mt-0.5 block text-xs text-ink-500">
              {t("overview.combinedView")}
            </span>
          </button>
        </div>

        <div className="space-y-2 p-3">
          {hasAdminScopedVenues ? (
            <>
              {ownedBusinesses.length > 0 ? (
                <p
                  data-testid="portfolio-scope-owned-label"
                  className="px-2 pt-1 text-[11px] font-semibold uppercase tracking-[0.14em] text-ink-500"
                >
                  {t("overview.scope.ownedLabel")}
                </p>
              ) : null}
              {ownedBusinesses.map(renderVenueRow)}
              <p
                data-testid="portfolio-scope-admin-label"
                className="px-2 pt-3 text-[11px] font-semibold uppercase tracking-[0.14em] text-ink-500"
              >
                {t("overview.scope.adminLabel")}
              </p>
              <p className="px-2 pb-1 text-xs leading-5 text-ink-500">
                {t("overview.scope.adminHint")}
              </p>
              {adminScopedBusinesses.map(renderVenueRow)}
            </>
          ) : (
            businesses.map(renderVenueRow)
          )}
        </div>

        {canAddBusiness ? (
          <div className="p-3 pt-1">
            {/* ?new=1 opts out of the register page's "owner already has a
                business → bounce to its dashboard" redirect (MIN-3). */}
            <Link
              href="/business/register?new=1"
              className="flex h-10 items-center justify-center gap-2 rounded-xl border border-dashed border-warm-300 text-sm font-semibold text-ink-600 transition-colors hover:border-brand/40 hover:bg-white hover:text-ink-900"
            >
              <Plus className="h-4 w-4" aria-hidden="true" />
              {t("overview.addBusiness")}
            </Link>
          </div>
        ) : null}
      </aside>

      <div className="min-w-0 bg-white">
        {statsError && !statsLoading ? (
          /* #832: a failed metrics load must announce itself — the "—" and
             "No data yet" placeholders below are otherwise indistinguishable
             from a genuinely quiet day. */
          <div
            role="alert"
            className="mx-5 mt-5 flex flex-col items-start gap-2 rounded-2xl border border-amber-200 bg-amber-50 p-4 sm:mx-7 sm:flex-row sm:items-center sm:justify-between"
          >
            <p className="text-sm text-amber-900">{statsError}</p>
            {onRetryStats ? (
              <Button
                size="sm"
                radius="full"
                variant="bordered"
                onPress={() => onRetryStats()}
                className="border-amber-300 text-amber-900"
              >
                {t("stats.retry")}
              </Button>
            ) : null}
          </div>
        ) : null}
        {selectedBusiness ? (
          <div>
            <div className="flex flex-col gap-3 border-b border-warm-200 px-5 py-5 sm:flex-row sm:items-center sm:justify-between sm:px-7">
              <div>
                <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-ink-500">
                  {t("overview.businessMetrics")}
                </p>
                <h2 className="mt-1 break-words text-xl font-bold tracking-tight text-ink-900">
                  {selectedBusiness.name}
                </h2>
                <p
                  data-testid="portfolio-detail-differentiator"
                  className="mt-1 break-words text-sm text-ink-500"
                >
                  {venueDifferentiator(selectedBusiness)}
                </p>
                {isAdminScopedVenue(selectedBusiness, viewerUserId) ? (
                  <p
                    data-testid="portfolio-detail-admin-scope"
                    className="mt-2 inline-flex items-center rounded-full border border-warm-300 bg-warm-50 px-2 py-0.5 text-[11px] font-semibold uppercase tracking-[0.08em] text-ink-500"
                  >
                    {t("overview.scope.adminChip")}
                  </p>
                ) : null}
              </div>
              <Button
                size="sm"
                aria-label={`${t("overview.openBusiness")} ${venueIdentityLabel(selectedBusiness)}`}
                onPress={() => onManage(selectedBusiness)}
                isLoading={navigatingBusinessId === selectedBusiness.id}
                isDisabled={navigatingBusinessId === selectedBusiness.id}
                className="bg-ink-900 font-semibold text-white"
              >
                {t("overview.openBusiness")}
              </Button>
            </div>
            <BusinessOverviewPanel
              business={selectedBusiness}
              summary={stats[selectedBusiness.id] ?? null}
              isOpen
              t={t}
            />
          </div>
        ) : (
          <div className="p-5 sm:p-7">
            <div className="max-w-2xl">
              <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-ink-500">
                {t("overview.combinedView")}
              </p>
              <h2 className="mt-1 text-2xl font-bold tracking-tight text-ink-900">
                {t("overview.allBusinesses")}
              </h2>
              <p className="mt-2 text-sm leading-6 text-ink-600">
                {t("overview.portfolioSubtitle")}
              </p>
              {hasAdminScopedVenues ? (
                <p
                  data-testid="portfolio-combined-scope-note"
                  className="mt-2 text-sm leading-6 text-ink-500"
                >
                  {t("overview.scope.combinedNote").replace(
                    "{count}",
                    String(adminScopedBusinesses.length),
                  )}
                </p>
              ) : null}
            </div>

            <div className="mt-7 grid gap-px overflow-hidden rounded-2xl border border-warm-200 bg-warm-200 sm:grid-cols-2">
              <section className="bg-white p-5">
                <p className="text-xs font-semibold uppercase tracking-[0.12em] text-ink-500">
                  {t("overview.todayRevenue")}
                </p>
                <div className="mt-3 space-y-1">
                  {buckets.length > 0 ? (
                    buckets.map((bucket) => (
                      <p
                        key={bucket.currency}
                        data-testid="overview-currency-total"
                        className="text-3xl font-bold tracking-tight text-ink-900 tabular-nums"
                      >
                        {fmt(
                          bucket.summary.today.revenue as number,
                          bucket.currency,
                        )}
                      </p>
                    ))
                  ) : (
                    <p className="text-3xl font-bold text-ink-500">—</p>
                  )}
                </div>
              </section>
              <section className="bg-white p-5">
                <p className="text-xs font-semibold uppercase tracking-[0.12em] text-ink-500">
                  {t("overview.todayPaidBills")}
                </p>
                <p
                  data-testid="portfolio-today-bills"
                  className="mt-3 text-3xl font-bold tracking-tight text-ink-900 tabular-nums"
                >
                  {totalTodayBills}
                </p>
              </section>
              <section className="bg-white p-5">
                <p className="text-xs font-semibold uppercase tracking-[0.12em] text-ink-500">
                  {t("overview.weekRevenue")}
                </p>
                <div className="mt-3 space-y-1">
                  {buckets.length > 0 ? (
                    buckets.map((bucket) => (
                      <p
                        key={bucket.currency}
                        data-testid="overview-week-total"
                        className="text-3xl font-bold tracking-tight text-ink-900 tabular-nums"
                      >
                        {fmt(
                          bucket.summary.week.revenue as number,
                          bucket.currency,
                        )}
                      </p>
                    ))
                  ) : (
                    <p className="text-3xl font-bold text-ink-500">—</p>
                  )}
                </div>
              </section>
              <section className="bg-white p-5">
                <p className="text-xs font-semibold uppercase tracking-[0.12em] text-ink-500">
                  {t("overview.weekPaidBills")}
                </p>
                <p
                  data-testid="portfolio-week-bills"
                  className="mt-3 text-3xl font-bold tracking-tight text-ink-900 tabular-nums"
                >
                  {totalWeekBills}
                </p>
              </section>
            </div>

            <CombinedOverviewPanel
              businesses={ownedBusinesses}
              todayHasActivity={todayHasActivity}
              statsLoading={statsLoading}
              t={t}
            />
          </div>
        )}
      </div>
    </div>
  );
}
