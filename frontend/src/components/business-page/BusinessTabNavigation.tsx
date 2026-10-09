"use client";

import React, { useMemo, useCallback, useRef, useState, useEffect } from "react";
import {
  Sparkles,
  Menu as MenuIcon,
  Navigation,
  Calendar,
  Phone,
} from "lucide-react";
import { Z_FAB, zStyle } from "./designLayers";
import { getRadiusClass } from "./designClasses";
import { preferredScrollBehavior } from "@/lib/scrollBehavior";

interface BusinessTabNavigationProps {
  activeTab: string;
  onChangeTab: (tab: string) => void;
  hasDeliveryTab: boolean;
  hasReservationsTab: boolean;
  designSettings: {
    primary_color: string;
    secondary_color: string;
    corner_radius?: string;
  };
  t: (key: string) => string;
  /** Optional element rendered at the right edge of the tab bar (e.g. language switcher). */
  rightSlot?: React.ReactNode;
  businessName?: string;
  logoUrl?: string;
  primaryCta?: { label: string; onClick: () => void } | null;
}

export default function BusinessTabNavigation({
  activeTab,
  onChangeTab,
  hasDeliveryTab,
  hasReservationsTab,
  designSettings,
  t,
  rightSlot,
  businessName,
  logoUrl,
  primaryCta,
}: BusinessTabNavigationProps) {
  const tabRefs = useRef<Record<string, HTMLButtonElement | null>>({});
  const sentinelRef = useRef<HTMLDivElement | null>(null);
  const [scrolled, setScrolled] = useState(false);
  const [logoError, setLogoError] = useState(false);

  const handleChangeTab = useCallback(
    (tab: string) => {
      onChangeTab(tab);
      const contentArea = document.getElementById("tab-content");
      if (contentArea) {
        contentArea.scrollIntoView({
          behavior: preferredScrollBehavior(),
          block: "start",
        });
      }
    },
    [onChangeTab],
  );

  const tabs = useMemo(() => {
    const list: Array<{ key: string; label: string; icon: React.ElementType }> = [
      { key: "about", label: t("businessPage.about"), icon: Sparkles },
      { key: "menu", label: t("businessPage.menuTab"), icon: MenuIcon },
    ];
    if (hasDeliveryTab) {
      list.push({
        key: "delivery",
        label: t("businessPage.deliveryTab"),
        icon: Navigation,
      });
    }
    if (hasReservationsTab) {
      list.push({
        key: "reservations",
        label: t("businessPage.reservations"),
        icon: Calendar,
      });
    }
    list.push({ key: "contact", label: t("businessPage.contact"), icon: Phone });
    return list;
  }, [hasDeliveryTab, hasReservationsTab, t]);

  const activateTabAt = useCallback(
    (index: number) => {
      const next = tabs[index];
      if (!next) return;
      handleChangeTab(next.key);
      tabRefs.current[next.key]?.focus();
    },
    [handleChangeTab, tabs],
  );

  const handleTabKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      const currentIdx = tabs.findIndex((tab) => tab.key === activeTab);
      if (currentIdx === -1) return;
      if (e.key === "ArrowRight" || e.key === "ArrowDown") {
        e.preventDefault();
        activateTabAt((currentIdx + 1) % tabs.length);
      } else if (e.key === "ArrowLeft" || e.key === "ArrowUp") {
        e.preventDefault();
        activateTabAt((currentIdx - 1 + tabs.length) % tabs.length);
      } else if (e.key === "Home") {
        e.preventDefault();
        activateTabAt(0);
      } else if (e.key === "End") {
        e.preventDefault();
        activateTabAt(tabs.length - 1);
      }
    },
    [activateTabAt, activeTab, tabs],
  );

  useEffect(() => {
    const sentinel = sentinelRef.current;
    if (!sentinel || typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver(
      ([entry]) => setScrolled(!entry.isIntersecting),
      { threshold: 0 },
    );
    observer.observe(sentinel);
    return () => observer.disconnect();
  }, []);

  const radiusClass = getRadiusClass(designSettings?.corner_radius);
  const showIdentity = scrolled && Boolean(businessName);
  const showStickyCta = scrolled && Boolean(primaryCta);

  return (
    <>
      <div ref={sentinelRef} className="h-px" aria-hidden />
      <section
        className="sticky top-0 max-w-full overflow-x-clip bg-white/85 backdrop-blur-md border-b border-warm-200"
        style={zStyle(Z_FAB)}
      >
        <div
          data-desktop-tab-bar=""
          className="mx-auto flex min-w-0 max-w-6xl items-center gap-2 overflow-hidden px-4 md:px-6"
        >
          <div
            data-testid="tab-bar-identity"
            className={`flex min-w-0 items-center gap-2 overflow-hidden transition-all duration-200 ${
              showIdentity
                ? "max-w-[10rem] opacity-100 sm:max-w-[14rem]"
                : "max-w-0 opacity-0 pointer-events-none"
            }`}
          >
            {logoUrl && !logoError && (
              <div className="h-6 w-6 shrink-0 overflow-hidden rounded bg-white ring-1 ring-warm-200">
                {/* eslint-disable-next-line @next/next/no-img-element, jsx-a11y/no-noninteractive-element-interactions -- tiny sticky identity tile; onError is load-fallback, not a user interaction */}
                <img
                  src={logoUrl}
                  alt=""
                  className="h-full w-full object-contain p-0.5"
                  onError={() => setLogoError(true)}
                />
              </div>
            )}
            {businessName && (
              <span className="truncate text-sm font-semibold text-ink-900">
                {businessName}
              </span>
            )}
          </div>

          <div
            className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto overflow-y-hidden overscroll-x-contain snap-x snap-mandatory [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
            role="tablist"
            aria-label={t("businessPage.tabsAria") || "Business sections"}
          >
            {tabs.map((tab) => {
              const Icon = tab.icon;
              const isActive = activeTab === tab.key;
              return (
                <button
                  key={tab.key}
                  ref={(el) => {
                    tabRefs.current[tab.key] = el;
                  }}
                  onClick={() => handleChangeTab(tab.key)}
                  role="tab"
                  aria-selected={isActive}
                  aria-controls={`tabpanel-${tab.key}`}
                  id={`tab-${tab.key}`}
                  tabIndex={isActive ? 0 : -1}
                  onKeyDown={handleTabKeyDown}
                  className={`relative flex snap-start items-center gap-2 px-3 py-4 text-sm font-semibold tracking-wide whitespace-nowrap transition-colors md:px-4 ${
                    isActive
                      ? "text-ink-900"
                      : "text-ink-500 hover:text-ink-900"
                  }`}
                >
                  <Icon className="w-4 h-4" />
                  {tab.label}
                  <span
                    aria-hidden
                    className={`absolute left-3 right-3 bottom-0 h-0.5 rounded-full transition-opacity ${
                      isActive ? "opacity-100" : "opacity-0"
                    }`}
                    style={{ backgroundColor: designSettings.primary_color }}
                  />
                </button>
              );
            })}
          </div>

          {showStickyCta && primaryCta && (
            <button
              type="button"
              data-testid="tab-bar-primary-cta"
              onClick={primaryCta.onClick}
              className={`shrink-0 px-3 py-1.5 text-xs font-semibold text-white ${radiusClass}`}
              style={{ backgroundColor: designSettings.primary_color }}
            >
              {primaryCta.label}
            </button>
          )}

          {rightSlot && (
            <div
              data-tab-bar-right-slot=""
              className="ml-auto flex shrink-0 items-center pl-2 md:pl-4"
            >
              {rightSlot}
            </div>
          )}
        </div>
      </section>
    </>
  );
}
