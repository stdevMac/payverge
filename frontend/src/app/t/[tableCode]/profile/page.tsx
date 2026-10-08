"use client";

import React, { useState, useEffect } from "react";
import { useParams } from "next/navigation";
import CustomerProfile from "@/components/customer/CustomerProfile";
import PersistentGuestNav from "@/components/navigation/PersistentGuestNav";
import { getOpenBillByTableCode, getBusinessByTableCode } from "@/api/bills";
import type { BillWithItemsResponse } from "@/api/bills";
import { getRouteParam } from "@/utils/nextRouteParams";

export default function TableCustomerProfilePage() {
  const params = useParams();
  const tableCode = getRouteParam(params, "tableCode");
  const [currentBill, setCurrentBill] = useState<BillWithItemsResponse | null>(null);
  const [businessCurrencies, setBusinessCurrencies] = useState({
    default_currency: "USD",
    display_currency: "USD",
  });

  useEffect(() => {
    // Load current bill and business currency settings for the bottom nav.
    // This data is supplementary — the profile surface must never wait on it
    // (issue 824: gating the whole page here left guests staring at a bare
    // full-screen spinner whenever these reads sat on a rate-limit/timeout).
    const loadData = async () => {
      const [billResult, businessResult] = await Promise.allSettled([
        getOpenBillByTableCode(tableCode),
        getBusinessByTableCode(tableCode),
      ]);

      if (billResult.status === "fulfilled") {
        // No-active-bill now resolves to { bill: null } (H3) — coerce to a
        // falsy currentBill so PersistentGuestNav keeps receiving null.
        setCurrentBill(billResult.value.bill ? billResult.value : null);
      }

      if (
        businessResult.status === "fulfilled" &&
        businessResult.value?.business
      ) {
        setBusinessCurrencies({
          default_currency:
            businessResult.value.business.default_currency || "USD",
          display_currency:
            businessResult.value.business.display_currency || "USD",
        });
      }
    };
    loadData().catch((err) => console.error("loadData failed:", err));
  }, [tableCode]);

  // Render immediately: CustomerProfile owns the signed-in state and the
  // honest sign-in gate (both bounded by CustomerAuthContext), and
  // PersistentGuestNav accepts a null bill and hydrates when the fetches
  // settle. No full-page spinner may gate this surface (issue 824).
  // PG-21: layout seeds GuestTranslationProvider with SSR messages.
  return (
    <>
      <div
        data-testid="guest-profile-shell"
        className="min-h-[100dvh] bg-warm-50 pb-[calc(var(--guest-nav-height,5.5rem)+var(--cookie-banner-height,0px)+env(safe-area-inset-bottom))]"
      >
        <CustomerProfile />
      </div>
      <PersistentGuestNav
        tableCode={tableCode}
        currentBill={currentBill}
        defaultCurrency={businessCurrencies.default_currency}
        displayCurrency={businessCurrencies.display_currency}
      />
    </>
  );
}
