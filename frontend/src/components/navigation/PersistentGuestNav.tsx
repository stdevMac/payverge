"use client";

import React, { useEffect } from "react";

import { Menu, Receipt, Home, User, Lock } from "lucide-react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useGuestTranslation } from "../../i18n/GuestTranslationProvider";
import {
  formatConvertedGuestCurrency,
  normalizeGuestLocale,
} from "@/utils/guestCurrencyFormatter";
import { useGuestConversionRate } from "@/hooks/useGuestConversionRate";
import { SimpleLanguageSelector } from "../guest/SimpleLanguageSelector";
import { CurrencyPrice } from "../common/CurrencyConverter";
import { useCustomerAuth } from "@/contexts/CustomerAuthContext";
import { countGuestFacingBillItems } from "@/lib/billBundleGrouping";

/**
 * Fixed design token for the guest bottom dock chrome height (tabs + language
 * row + vertical padding). Safe-area is NOT included — consumers add
 * `env(safe-area-inset-bottom)` themselves. Published as CSS var
 * `--guest-nav-height` on mount so landing/menu/bill/profile share one value.
 */
export const GUEST_NAV_HEIGHT_PX = 88;
export const GUEST_NAV_CSS_VAR = "--guest-nav-height";

interface PersistentGuestNavProps {
  tableCode: string;
  currentBill?: {
    bill: {
      total_amount: number;
      paid_amount?: number;
    };
    items: any[];
  } | null;
  showBackToTable?: boolean;
  isOrderingEnabled?: boolean;
  defaultCurrency?: string;
  displayCurrency?: string;
}

interface TabProps {
  href: string;
  active: boolean;
  icon: React.ReactNode;
  label: React.ReactNode;
  badge?: React.ReactNode;
  tone?: "brand" | "neutral";
  disabled?: boolean;
  onDisabledTap?: () => void;
  ariaLabel?: string;
}

function Tab({
  href,
  active,
  icon,
  label,
  badge,
  tone = "brand",
  disabled,
  onDisabledTap,
  ariaLabel,
}: TabProps) {
  // Stable width via fixed-min basis. The badge sits above the icon as a chip
  // so the running bill total cannot reflow neighbouring tabs.
  // `min-h-12` (48px) keeps each tab above the 44px iOS HIG tap-target
  // baseline even when the label wraps short. `py-2.5` adds 4px of
  // breathing room over the previous py-2 without changing the icon size.
  const baseClasses =
    "group relative flex flex-1 basis-0 flex-col items-center justify-center gap-1 rounded-2xl px-2 py-2.5 min-h-12 text-[10px] font-semibold uppercase tracking-[0.18em] transition-colors duration-150";
  const stateClasses = disabled
    ? "text-ink-400"
    : active
      ? tone === "brand"
        ? "bg-brand text-white"
        : "bg-ink-900 text-white"
      : "text-ink-500 hover:bg-ink-50 hover:text-ink-900";

  const content = (
    <>
      {badge && (
        <span className="pointer-events-none absolute -top-2 inline-flex h-4 min-w-[1.25rem] items-center justify-center rounded-full bg-ink-900 px-1.5 text-[10px] font-semibold tabular-nums text-white shadow-sm">
          {badge}
        </span>
      )}
      <span className="flex h-6 w-6 items-center justify-center">{icon}</span>
      <span className="leading-none">{label}</span>
    </>
  );

  if (disabled) {
    // A locked tab gets a lock glyph and, when a tap handler is supplied,
    // becomes an interactive button that explains why it's unavailable
    // instead of silently doing nothing.
    const lockedContent = (
      <>
        <span className="pointer-events-none absolute top-0.5 end-0.5 inline-flex h-4 w-4 items-center justify-center rounded-full bg-ink-200 text-ink-600 shadow-sm">
          <Lock className="h-2.5 w-2.5" strokeWidth={2.25} />
        </span>
        {content}
      </>
    );
    if (onDisabledTap) {
      return (
        <button
          type="button"
          onClick={onDisabledTap}
          aria-disabled="true"
          className={`${baseClasses} ${stateClasses} cursor-not-allowed`}
        >
          {lockedContent}
        </button>
      );
    }
    return (
      <div className={`${baseClasses} ${stateClasses}`}>{lockedContent}</div>
    );
  }

  return (
    <Link
      href={href}
      className={`${baseClasses} ${stateClasses}`}
      aria-label={ariaLabel}
      aria-current={active ? "page" : undefined}
    >
      {content}
    </Link>
  );
}

function PersistentGuestNav({
  tableCode,
  currentBill,
  showBackToTable: _showBackToTable = false,
  isOrderingEnabled = true,
  defaultCurrency = "USD",
  displayCurrency = "USD",
}: PersistentGuestNavProps) {
  const { t, currentLanguage } = useGuestTranslation();
  const moneyLocale = normalizeGuestLocale(currentLanguage);
  const { rate: displayRate } = useGuestConversionRate(
    defaultCurrency,
    displayCurrency,
  );
  const remainderDue = currentBill?.bill
    ? Math.max(
        currentBill.bill.total_amount - (currentBill.bill.paid_amount ?? 0),
        0,
      )
    : 0;
  const convertedRemainder = formatConvertedGuestCurrency(
    remainderDue,
    defaultCurrency,
    displayCurrency || defaultCurrency,
    currentLanguage,
    displayRate,
  );
  const pathname = usePathname();
  const { customerId } = useCustomerAuth();

  // Publish dock height so page shells can clear the fixed chrome without
  // magic rem values that drift when tabs/language chrome changes.
  useEffect(() => {
    if (typeof document === "undefined") return;
    document.documentElement.style.setProperty(
      GUEST_NAV_CSS_VAR,
      `${GUEST_NAV_HEIGHT_PX}px`,
    );
  }, []);

  const isMenuPage = pathname?.includes("/menu");
  const isBillPage = pathname?.includes("/bill");
  const isProfilePage = pathname?.includes("/profile");
  const isTablePage = !isMenuPage && !isBillPage && !isProfilePage;

  // Defensive: a no-active-bill response can be `{ bill: null }` (H3) if a
  // caller forgets to coerce it to null — never treat that as a live bill.
  const hasBill = Boolean(currentBill?.bill);

  // Guest-facing count: parents only (exclude nested bundle children / discounts)
  // so a handful of Date Night bundles does not badge as 32+ "items".
  const totalItemQuantity = countGuestFacingBillItems(currentBill?.items);

  return (
    <div
      data-testid="persistent-guest-nav"
      className="pointer-events-none fixed bottom-[var(--cookie-banner-height,0px)] left-0 right-0 z-50 border-t border-warm-200 bg-warm-50/85 backdrop-blur-xl"
      style={{
        paddingBottom: "max(env(safe-area-inset-bottom), 0px)",
      }}
    >
      <div className="pointer-events-auto mx-auto flex max-w-md items-stretch gap-1 px-3 py-2 sm:max-w-xl sm:gap-2 sm:px-4 sm:py-3">
        <Tab
          href={`/t/${tableCode}`}
          active={!!isTablePage}
          icon={<Home className="h-5 w-5" strokeWidth={1.75} />}
          label={t("navigation.table")}
        />
        <Tab
          href={`/t/${tableCode}/menu`}
          active={!!isMenuPage}
          icon={<Menu className="h-5 w-5" strokeWidth={1.75} />}
          label={t("navigation.menu")}
        />
        {/* Show the Bill tab whenever the guest has something to pay — even at
            venues where in-app ordering is disabled (the operator can still open
            a bill for the table and the guest pays via QR). Gating this purely on
            isOrderingEnabled stranded pay-only guests with no way to reach their
            bill. The "add an item to start your bill" locked state only makes
            sense when ordering is actually available. */}
        {/* Bill tab: empty bill is still navigable (empty state on /bill).
            Do not lock merely because bill is null — guests should reach the
            empty bill shell. Hidden only when ordering is off AND no bill. */}
        {(isOrderingEnabled || hasBill || !!isBillPage) && (
          <Tab
            href={`/t/${tableCode}/bill`}
            active={!!isBillPage}
            tone="brand"
            icon={<Receipt className="h-5 w-5" strokeWidth={1.75} />}
            ariaLabel={
              currentBill?.bill
                ? totalItemQuantity === 1
                  ? t("navigation.billWithSummaryOne", {
                      amount: convertedRemainder,
                    })
                  : t("navigation.billWithSummary", {
                      count: totalItemQuantity,
                      amount: convertedRemainder,
                    })
                : t("navigation.bill")
            }
            label={
              currentBill?.bill ? (
                <span className="font-mono normal-case tracking-normal">
                  <span className="me-1 uppercase tracking-[0.12em]">
                    {t("navigation.bill")}
                  </span>
                  {/* Show the amount still owed, not the gross total. After a
                      partial payment the gross total contradicts the bill hero
                      (which shows what's left). Money is float64 dollars from
                      JSON — never divide. */}
                  <CurrencyPrice
                    amount={Math.max(
                      currentBill.bill.total_amount -
                        (currentBill.bill.paid_amount ?? 0),
                      0,
                    )}
                    fromCurrency={defaultCurrency}
                    displayCurrency={displayCurrency}
                    locale={moneyLocale}
                  />
                </span>
              ) : (
                t("navigation.bill")
              )
            }
            badge={
              hasBill && totalItemQuantity > 0 ? totalItemQuantity : undefined
            }
          />
        )}
        {customerId && (
          <Tab
            href={`/t/${tableCode}/profile`}
            active={!!isProfilePage}
            icon={<User className="h-5 w-5" strokeWidth={1.75} />}
            label={t("navigation.profile")}
          />
        )}
        <div className="flex flex-1 basis-0 items-center justify-center">
          <SimpleLanguageSelector tableCode={tableCode} />
        </div>
      </div>
    </div>
  );
}

export default PersistentGuestNav;
