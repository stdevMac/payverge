"use client";

import { useEffect, useState } from "react";
import { convertAmount } from "@/api/currency";

export type GuestConversionStatus = "pending" | "ready" | "failed";

/**
 * Shared default→display FX rate for guest money that cannot render
 * `<CurrencyPrice>` (aria-labels, interpolated CTA strings).
 */
export function useGuestConversionRate(
  fromCurrency?: string,
  displayCurrency?: string,
): { rate: number | null; status: GuestConversionStatus } {
  const from = (fromCurrency || "").trim().toUpperCase();
  const to = (displayCurrency || from).trim().toUpperCase();
  const same = !from || !to || from === to;
  const [rate, setRate] = useState<number | null>(same ? 1 : null);
  const [status, setStatus] = useState<GuestConversionStatus>(
    same ? "ready" : "pending",
  );

  useEffect(() => {
    if (same) {
      setRate(1);
      setStatus("ready");
      return;
    }

    let cancelled = false;
    setStatus("pending");
    setRate(null);
    convertAmount(1, from, to)
      .then((result) => {
        if (cancelled) return;
        setRate(result.converted_amount);
        setStatus("ready");
      })
      .catch(() => {
        if (cancelled) return;
        setRate(null);
        setStatus("failed");
      });

    return () => {
      cancelled = true;
    };
  }, [from, to, same]);

  return { rate, status };
}
