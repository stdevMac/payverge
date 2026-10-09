"use client";

import React from "react";
import { useParams } from "next/navigation";
import CustomerProfile from "@/components/customer/CustomerProfile";
import PersistentGuestNav from "@/components/navigation/PersistentGuestNav";
import { getRouteParam } from "@/utils/nextRouteParams";

export default function TableSignInPage() {
  const params = useParams();
  const tableCode = getRouteParam(params, "tableCode");

  return (
    <>
      <div
        data-testid="guest-signin-shell"
        className="min-h-[100dvh] bg-warm-50 pb-[calc(var(--guest-nav-height,5.5rem)+var(--cookie-banner-height,0px)+env(safe-area-inset-bottom))]"
      >
        <CustomerProfile />
      </div>
      <PersistentGuestNav tableCode={tableCode} currentBill={null} />
    </>
  );
}
