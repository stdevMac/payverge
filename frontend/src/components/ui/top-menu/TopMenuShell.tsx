"use client";

import Image from "next/image";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { House, Menu, X } from "lucide-react";
import type { ReactNode } from "react";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { localizedPublicHref } from "@/i18n/publicPageRoutes";
import type { Locale } from "@/i18n/localeRegistry";
import { useInstance } from "@/hooks/useInstance";

/** Portfolio + operator dashboard routes must never flash sign-in CTAs. */
export function isOperatorDashboardPath(pathname: string | null | undefined): boolean {
  if (!pathname) return false;
  return (
    pathname === "/dashboard" ||
    /^\/business\/[^/]+\/dashboard(\/|$)/.test(pathname)
  );
}

/**
 * Desktop center nav (Home, or Home + Dashboard when signed in) fits from
 * `md` up; below that the links live in the mobile drawer.
 */
export const DESKTOP_NAV_CLASS = "hidden md:flex items-center";

export const MOBILE_ONLY_CLASS = "md:hidden";

export const HEADER_STAFF_LINK_CLASS =
  "whitespace-nowrap shrink-0 px-3 py-2 text-sm font-medium text-ink-700 hover:text-ink-950 transition-colors";

export const HEADER_SIGN_IN_CLASS =
  "inline-flex shrink-0 items-center justify-center whitespace-nowrap rounded-full bg-brand px-3 py-2.5 text-sm font-semibold text-white shadow-sm shadow-brand/20 hover:bg-brand-dark hover:shadow-md hover:shadow-brand/30 transition-all focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-dark focus-visible:ring-offset-2 xl:px-5";

type TopMenuNavLabels = {
  home: string;
  staffLogin: string;
  signIn: string;
  toggleMenuAria: string;
};

type TopMenuShellProps = {
  pathname?: string | null;
  scrolled: boolean;
  labels: TopMenuNavLabels;
  /** Desktop center navigation — public links or authenticated links */
  centerSlot?: ReactNode;
  /** Desktop + mobile right rail (auth buttons, profile, wallet) */
  rightSlot?: ReactNode;
  /** When true, show the public Home link in the center on desktop */
  showPublicNav?: boolean;
  /** Mobile drawer open state */
  isMenuOpen?: boolean;
  onToggleMenu?: () => void;
  mobileDrawer?: ReactNode;
  /** Placeholder shimmer on the auth rail while session hydrates */
  authLoading?: boolean;
};

function navLinkClass(active: boolean) {
  return `px-3 py-2 rounded-full text-sm font-medium transition-all duration-200 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand ${
    active
      ? "text-brand bg-brand/10"
      : "text-ink-600 hover:text-ink-950 hover:bg-warm-100/80"
  }`;
}

function AuthRailSkeleton() {
  return (
    <div className="hidden md:flex items-center gap-2" aria-hidden="true">
      <div className="h-9 w-20 rounded-full bg-warm-200/70 animate-pulse" />
      <div className="h-10 w-24 rounded-full bg-brand/20 animate-pulse" />
    </div>
  );
}

export function TopMenuShell({
  pathname,
  scrolled,
  labels,
  centerSlot,
  rightSlot,
  showPublicNav = true,
  isMenuOpen = false,
  onToggleMenu,
  mobileDrawer,
  authLoading = false,
}: TopMenuShellProps) {
  const { locale } = useSimpleLocale();
  const { instance, productName } = useInstance();
  const instanceLogoUrl = instance?.logo_url || "";
  const href = (route: string) => localizedPublicHref(locale, route);
  const pathMatches = (route: string) =>
    pathname === route || pathname === href(route);

  return (
    <>
      <nav
        className={`fixed top-0 left-0 right-0 z-50 transition-all duration-300 ${
          scrolled
            ? "bg-white/95 backdrop-blur-md shadow-sm border-b border-warm-200/60"
            : "bg-white md:bg-warm-50/70 md:backdrop-blur-md md:border-b md:border-warm-200/40"
        }`}
      >
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="flex items-center justify-between h-14 md:h-16">
            <Link
              href={href("/")}
              className="flex-shrink-0 flex items-center py-2 cursor-pointer group"
            >
              <div className="relative w-28 h-7 sm:w-32 sm:h-8 md:w-36 md:h-9 transition-transform duration-300 group-hover:scale-[1.02]">
                {instanceLogoUrl ? (
                  // Operator logo from /instance (LOGO_URL). A plain <img>
                  // skips next/image's allow-list, but the CSP img-src still
                  // applies: LOGO_URL must be same-origin (e.g. /media/...),
                  // on the API origin, or listed in MEDIA_ORIGINS.
                  // eslint-disable-next-line @next/next/no-img-element
                  <img
                    src={instanceLogoUrl}
                    alt={productName}
                    className="h-full w-full object-contain object-left"
                    data-testid="instance-logo"
                  />
                ) : (
                  <Image
                    src="/images/PayvergeLogo.png"
                    alt={productName}
                    fill
                    sizes="(max-width: 640px) 112px, (max-width: 768px) 128px, 144px"
                    className="object-contain"
                    priority
                    quality={50}
                  />
                )}
              </div>
            </Link>

            <div className={DESKTOP_NAV_CLASS}>
              {centerSlot ??
                (showPublicNav ? (
                  <div className="flex items-center gap-0.5">
                    <Link
                      href={href("/")}
                      className={navLinkClass(pathMatches("/"))}
                    >
                      <span className="inline-flex items-center gap-1.5">
                        <House size={17} aria-hidden />
                        <span>{labels.home}</span>
                      </span>
                    </Link>
                  </div>
                ) : null)}
            </div>

            <div className="flex shrink-0 items-center gap-2 md:gap-3">
              {authLoading ? <AuthRailSkeleton /> : rightSlot}

              {onToggleMenu && (
                <button
                  type="button"
                  className={`${MOBILE_ONLY_CLASS} p-2.5 rounded-full text-ink-600 hover:bg-warm-100 transition-all outline-none focus-visible:ring-2 focus-visible:ring-brand active:scale-95`}
                  onClick={onToggleMenu}
                  aria-label={labels.toggleMenuAria}
                  aria-expanded={isMenuOpen}
                >
                  {isMenuOpen ? <X size={24} /> : <Menu size={24} />}
                </button>
              )}
            </div>
          </div>
        </div>
      </nav>
      {mobileDrawer}
    </>
  );
}

/**
 * Instant chrome for SSR / chunk-load — real links, no auth deps.
 * Labels resolve from the operator locale (seeded by SimpleTranslationProvider
 * from x-payverge-locale) so a lang=es first paint never flashes English nav.
 *
 * On operator dashboard routes, never paint Sign in / Staff Login / public
 * center links — a mid-shift skeleton must stay neutral until session resolves.
 */
export function TopMenuInstantChrome() {
  const pathname = usePathname();
  const onDashboard = isOperatorDashboardPath(pathname);
  const { locale } = useSimpleLocale();
  const tNav = (key: string): string => {
    const result = getTranslation(`navigation.${key}`, locale as Locale);
    return Array.isArray(result) ? result[0] || key : (result as string);
  };
  const labels: TopMenuNavLabels = {
    home: tNav("home"),
    staffLogin: tNav("staffLogin"),
    signIn: tNav("signIn"),
    toggleMenuAria: tNav("toggleMenuAria"),
  };

  return (
    <TopMenuShell
      pathname={pathname}
      scrolled={false}
      labels={labels}
      showPublicNav={!onDashboard}
      authLoading={onDashboard}
      rightSlot={
        onDashboard ? undefined : (
          <div
            data-testid="header-auth-rail"
            className="hidden md:flex items-center gap-2"
          >
            <Link
              href={localizedPublicHref(locale, "/staff/login")}
              className={HEADER_STAFF_LINK_CLASS}
            >
              {labels.staffLogin}
            </Link>
            <Link
              href="/dashboard?auth=signin"
              className={HEADER_SIGN_IN_CLASS}
            >
              {labels.signIn}
            </Link>
          </div>
        )
      }
    />
  );
}
