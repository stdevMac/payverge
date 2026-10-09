"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { Button } from "@nextui-org/react";
import { BanknoteIcon, CheckCircle2, CreditCard } from "lucide-react";

import type { PublicDeliveryTrackingDto } from "@/api/delivery";
import { formatConvertedGuestCurrency } from "@/utils/guestCurrencyFormatter";
import { useGuestConversionRate } from "@/hooks/useGuestConversionRate";
import { deliveryGuestPath } from "@/lib/deliveryLocale";
import { PaymentCountdown } from "./_PaymentCountdown";

export interface PaymentBlockProps {
  tracking: PublicDeliveryTrackingDto;
  deliveryNumber: string;
  onExpired: () => void;
  tString: (key: string, vars?: Record<string, string | number>) => string;
  guestLocale?: string;
}

export function PaymentBlock({
  tracking,
  deliveryNumber,
  onExpired,
  tString,
  guestLocale,
}: PaymentBlockProps) {
  const { awaiting_payment, payment_expires_at, payment_mode, bill } = tracking;
  const status = tracking.status;
  // Convert default→display before formatting so the symbol matches the magnitude.
  const defaultCurrency = tracking.default_currency || "USD";
  const displayCurrency = tracking.display_currency || defaultCurrency;
  const { rate: displayRate } = useGuestConversionRate(
    defaultCurrency,
    displayCurrency,
  );
  const isTerminalStatus =
    status === "delivered" || status === "cancelled" || status === "failed";

  // Local expiry latch: the countdown flips this synchronously so the Pay Now
  // CTA can never be clicked during the ~1s refetch after the window closes
  // (the parent's onExpired refetch is the source of truth; this is the guard).
  const [expiredLocally, setExpiredLocally] = useState(false);
  useEffect(() => {
    setExpiredLocally(false);
  }, [tracking.payment_expires_at]);

  // A cancelled/failed delivery must never lead the guest into payment, even
  // if a backend race leaves awaiting_payment set on the tracking payload.
  if (status === "cancelled" || status === "failed") {
    return null;
  }

  // Pay-now card
  if (awaiting_payment && !expiredLocally) {
    return (
      <div
        className="rounded-2xl border-2 border-amber-300 bg-amber-50 p-5"
        data-testid="pay-now-card"
      >
        <div className="flex items-start gap-3">
          <CreditCard className="mt-0.5 h-5 w-5 shrink-0 text-amber-600" />
          <div className="flex-1 min-w-0">
            <p className="font-semibold text-amber-800">
              {tString("deliveryTracking.payNow")}
            </p>
            {payment_expires_at
              ? (() => {
                  const sentence = tString("deliveryTracking.paymentExpires", {
                    time: "__COUNTDOWN__",
                  });
                  // Defensive: a locale string missing {time} must not break layout.
                  const parts = sentence.split("__COUNTDOWN__");
                  const before = parts[0] ?? "";
                  const after = parts[1] ?? "";
                  return (
                    <p className="text-sm text-ink-500 mt-0.5">
                      {before}
                      <PaymentCountdown
                        expiresAt={payment_expires_at}
                        onExpired={() => {
                          setExpiredLocally(true);
                          onExpired();
                        }}
                      />
                      {after}
                    </p>
                  );
                })()
              : null}
          </div>
        </div>
        <div className="mt-4">
          <Button
            as={Link}
            href={deliveryGuestPath(deliveryNumber, "pay", guestLocale)}
            color="warning"
            className="w-full font-semibold"
            data-testid="pay-now-link"
          >
            {tString("deliveryTracking.payNow")}
          </Button>
        </div>
      </div>
    );
  }

  // Paid chip
  if (bill?.paid) {
    return (
      <div
        className="flex items-center gap-2 rounded-2xl border border-emerald-200 bg-emerald-50 px-4 py-3"
        data-testid="paid-chip"
      >
        <CheckCircle2 className="h-4 w-4 shrink-0 text-emerald-600" />
        <span className="text-sm font-semibold text-emerald-700">
          {tString("deliveryTracking.paid")}
        </span>
      </div>
    );
  }

  // COD row — only while delivery is still live
  if (payment_mode === "cash_on_delivery" && !isTerminalStatus) {
    return (
      <div
        className="flex items-center gap-3 rounded-2xl border border-ink-200 bg-ink-50 px-4 py-3"
        data-testid="cod-row"
      >
        <BanknoteIcon className="h-4 w-4 shrink-0 text-ink-500" />
        <span className="text-sm text-ink-700">
          {tString("deliveryTracking.payDriverCash", {
            amount:
              bill?.total_amount != null
                ? formatConvertedGuestCurrency(
                    bill.total_amount,
                    defaultCurrency,
                    displayCurrency,
                    guestLocale,
                    displayRate,
                  )
                : "",
          })}
        </span>
      </div>
    );
  }

  return null;
}
