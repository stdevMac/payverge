"use client";

import React, { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useParams, useRouter, useSearchParams } from "next/navigation";
import { Card, CardBody } from "@nextui-org/react";
import {
  guestDeliveryApi,
  type PublicDeliveryTrackingDto,
} from "@/api/delivery";
import PaymentSection from "@/components/guest/PaymentSection";
import { tryGuestBillRef } from "@/api/bills";
import {
  GuestTranslationProvider,
  useGuestTranslation,
} from "@/i18n/GuestTranslationProvider";
import {
  deliveryGuestPath,
  resolveDeliveryInitialLanguage,
} from "@/lib/deliveryLocale";

// ── Inner page ────────────────────────────────────────────────────────────────

function isPaymentWindowExpired(
  tracking: PublicDeliveryTrackingDto | null,
  now: number,
): boolean {
  return (
    tracking?.payment_expires_at != null &&
    new Date(tracking.payment_expires_at).getTime() < now
  );
}

function DeliveryPayInner() {
  const rawParams = useParams<{ deliveryNumber: string }>();
  const router = useRouter();
  const searchParams = useSearchParams();
  const { t, currentLanguage } = useGuestTranslation();

  const [tracking, setTracking] =
    useState<PublicDeliveryTrackingDto | null>(null);
  const [error, setError] = useState<string | null>(null);
  // Clock lives in a ref so the pay form does not re-render every second.
  // `expired` flips only when the payment window actually crosses, which is
  // what tears the form down. Crypto payments are irreversible, so a stale
  // page must not keep an expired pay form live.
  const nowRef = useRef(Date.now());
  const [expired, setExpired] = useState(false);

  const deliveryNumber = (rawParams?.deliveryNumber ?? "") as string;
  const trackHref = deliveryGuestPath(deliveryNumber, "track", currentLanguage);

  // Tracks whether the first load succeeded so background re-fetch failures
  // (network blips) don't nuke an already-rendered pay form into an error.
  const hasTrackingRef = useRef(false);

  // Read `t` through a ref so a language change does not rebuild `load` and
  // restart the tracking poll.
  const tRef = useRef(t);
  tRef.current = t;
  const trackingRef = useRef(tracking);
  trackingRef.current = tracking;

  // If the guest returns from a plugin redirect with ?payment=success, route
  // them straight to the track page — it polls every 15 s and will show
  // paid/preparing once the webhook lands. On ?payment=cancelled we stay
  // (the form re-shows, which is acceptable).
  const paymentReturn = searchParams?.get("payment");
  useEffect(() => {
    if (paymentReturn === "success") {
      router.replace(trackHref);
    }
  }, [paymentReturn, trackHref, router]);

  const load = useCallback(async () => {
    try {
      const data = await guestDeliveryApi.track(deliveryNumber);
      hasTrackingRef.current = true;
      setTracking(data);
    } catch {
      if (!hasTrackingRef.current) {
        setError(tRef.current("deliveryPay.loadError"));
      }
    }
  }, [deliveryNumber]);

  useEffect(() => {
    void load();
  }, [load]);

  // Advance the gate clock every second, but only publish a render when the
  // expired flag itself flips.
  useEffect(() => {
    const id = window.setInterval(() => {
      nowRef.current = Date.now();
      const next = isPaymentWindowExpired(trackingRef.current, nowRef.current);
      setExpired((current) => (current === next ? current : next));
    }, 1_000);
    return () => window.clearInterval(id);
  }, []);

  // Tracking can arrive already past the deadline, between ticks. Sync the
  // flag from the ref clock during render so the form never commits expired.
  const nextExpired = isPaymentWindowExpired(tracking, nowRef.current);
  if (nextExpired !== expired) {
    setExpired(nextExpired);
  }

  // Re-validate the payment window against the backend: the expiry sweeper
  // can cancel the delivery (and close the bill) server-side, so poll fresh
  // tracking every 15 s while the tab is visible and immediately when the tab
  // becomes visible again. Without this a stale page would let a guest send
  // irreversible USDC for an already-expired/cancelled order.
  const hasTracking = tracking != null;
  useEffect(() => {
    if (!hasTracking) return;

    const interval = window.setInterval(() => {
      if (!document.hidden) {
        void load();
      }
    }, 15_000);
    const handleVisibility = () => {
      if (!document.hidden) {
        void load();
      }
    };
    document.addEventListener("visibilitychange", handleVisibility);
    return () => {
      window.clearInterval(interval);
      document.removeEventListener("visibilitychange", handleVisibility);
    };
  }, [hasTracking, load]);

  // Redirect to track when outside the payable window. Evaluated in an effect
  // so router.replace never fires synchronously in the render body. `expired`
  // flips when the ref clock crosses the deadline.
  useEffect(() => {
    if (!tracking) return;

    if (
      !tracking.awaiting_payment ||
      expired ||
      !tracking.bill ||
      !tracking.business_id
    ) {
      router.replace(trackHref);
    }
  }, [tracking, expired, trackHref, router]);

  // Guest returning from a plugin redirect — show redirecting message while
  // the effect above calls router.replace to the track page.
  if (paymentReturn === "success") {
    return <PayMessage text={t("deliveryPay.redirecting")} />;
  }

  // Loading state
  if (!tracking && !error) {
    return <PayMessage text={t("deliveryPay.loading")} />;
  }

  if (error) {
    // Mirrors the /t table-not-found recovery pattern: retry in place plus a
    // safe exit to the tracking page (which owns its own error handling).
    return (
      <main className="flex min-h-screen flex-col items-center justify-center gap-4 bg-warm-50 px-6 text-center">
        <h1 className="font-serif text-2xl text-ink-900">
          {t("deliveryPay.title")}
        </h1>
        <p className="max-w-sm text-sm text-ink-600">{error}</p>
        <div className="flex gap-3">
          <button
            type="button"
            onClick={() => {
              setError(null);
              void load();
            }}
            className="rounded-xl bg-brand px-5 py-2.5 text-sm font-semibold text-white"
          >
            {t("deliveryPay.retry")}
          </button>
          <Link
            href={trackHref}
            className="rounded-xl border border-warm-200 bg-white px-5 py-2.5 text-sm font-semibold text-ink-700"
          >
            {t("deliveryPay.trackOrder")}
          </Link>
        </div>
      </main>
    );
  }

  if (!tracking) {
    return <PayMessage text={t("deliveryPay.loading")} />;
  }

  // While we still have tracking data but the gate fired, stay in the
  // "redirecting" message until the effect's router.replace resolves.
  if (
    !tracking.awaiting_payment ||
    expired ||
    !tracking.bill ||
    !tracking.business_id
  ) {
    return <PayMessage text={t("deliveryPay.redirecting")} />;
  }

  const { bill } = tracking;
  const defaultCurrency = tracking.default_currency || "USD";
  const displayCurrency = tracking.display_currency || defaultCurrency;
  // Charge the REMAINING balance, not the full bill total — the bill can be
  // partially paid (e.g. staff recorded an alternative payment) while
  // awaiting_payment is still true. Mirrors GuestBill's remaining computation.
  const remainingAmount = Math.max(0, bill.total_amount - (bill.paid_amount ?? 0));
  // Prefer tryGuestBillRef during render so a missing token is a recoverable
  // empty state instead of an uncaught throw into the error boundary
  // (PV-LIVE-20260720-001 / delivery track historically omitted public_token).
  const billToken = tryGuestBillRef(bill);

  if (!billToken) {
    return (
      <main className="flex min-h-screen flex-col items-center justify-center gap-4 bg-warm-50 px-6 text-center">
        <h1 className="font-serif text-2xl text-ink-900">
          {t("deliveryPay.title")}
        </h1>
        <p className="max-w-sm text-sm text-ink-600">
          {t("deliveryPay.loadError")}
        </p>
        <Link
          href={trackHref}
          className="rounded-xl border border-warm-200 bg-white px-5 py-2.5 text-sm font-semibold text-ink-700"
        >
          {t("deliveryPay.trackOrder")}
        </Link>
      </main>
    );
  }

  return (
    <main className="min-h-screen bg-warm-50 px-4 py-12 text-ink-900 sm:px-6 sm:py-16">
      <div className="mx-auto flex max-w-lg flex-col items-center justify-center">
        <Card className="w-full border border-ink-200 bg-white shadow-2xl">
          <CardBody className="gap-6 p-6 sm:p-8">
            {/* Header */}
            <div className="space-y-2 text-center">
              <p className="text-xs font-medium uppercase tracking-[0.3em] text-ink-500">
                {tracking.business_name ?? "Payverge"}
              </p>
              <h1 className="text-2xl font-semibold sm:text-3xl">
                {t("deliveryPay.title")}
              </h1>
              <p className="text-sm text-ink-500">
                {t("deliveryPay.subtitle", { number: deliveryNumber })}
              </p>
              {tracking.external_partner_links &&
              tracking.external_partner_links.length > 0 ? (
                <p className="text-sm text-ink-600" data-testid="pay-couriers">
                  {t("deliveryPay.couriersNamed", {
                    names: tracking.external_partner_links
                      .map((link) => link.name)
                      .filter(Boolean)
                      .join(", "),
                  })}
                </p>
              ) : null}
            </div>

            {/* Payment section — thin shell composing the shared component */}
            <PaymentSection
              billId={bill.id}
              billToken={billToken}
              businessId={tracking.business_id}
              amount={remainingAmount}
              businessName={tracking.business_name ?? ""}
              businessAddress={bill.settlement_address ?? ""}
              tipAddress={bill.tipping_address ?? ""}
              tableCode="delivery" /* sentinel — goes only into opaque plugin metadata */
              defaultCurrency={defaultCurrency}
              displayCurrency={displayCurrency}
              hideCashier
              // The driver tip entered at checkout is already baked into
              // bill.total_amount; a second pay-page tip would double-charge
              // gratuity on top of the authoritative bill total.
              hideTip
              returnUrlBase={deliveryGuestPath(
                deliveryNumber,
                "pay",
                currentLanguage,
              )}
              onPaymentComplete={() =>
                router.replace(trackHref)
              }
              onCashierPayment={() => {
                // Cashier is hidden for delivery orders (hideCashier=true); this
                // callback is a no-op safety net in case it fires unexpectedly.
              }}
            />
          </CardBody>
        </Card>
      </div>
    </main>
  );
}

// ── Utility ───────────────────────────────────────────────────────────────────

function PayMessage({ text }: { text: string }) {
  return (
    <div className="max-w-xl mx-auto px-4 py-16 text-center text-ink-500">
      {text}
    </div>
  );
}

// ── Main page ─────────────────────────────────────────────────────────────────

export default function DeliveryPayPage() {
  const searchParams = useSearchParams();
  const initialLanguage = resolveDeliveryInitialLanguage(
    searchParams?.get("lang"),
  );
  return (
    <GuestTranslationProvider
      initialLanguage={initialLanguage}
      preferInitialLanguage
    >
      <DeliveryPayInner />
    </GuestTranslationProvider>
  );
}
