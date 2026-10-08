"use client";

import { useEffect, useRef, useState } from "react";
import { formatCountdownLabel } from "@/utils/formatCountdownLabel";

export interface PaymentCountdownProps {
  expiresAt: string;
  onExpired: () => void;
}

/**
 * Live countdown to the payment-window expiry. Fires onExpired exactly once
 * per expiresAt value. Label format: mm:ss under 1h, "Xh Ym" under 1d, "Xd Yh"
 * beyond — never a multi-day "1378:23" wall of minutes.
 *
 * Wave 4 TDZ fix: the first tick() used to run BEFORE `const id = setInterval`
 * existed, so an ALREADY-expired expiresAt hit clearInterval(id) inside the
 * temporal dead zone and threw ReferenceError, crashing the tracking page at
 * the exact moment guests most need it. `id` is now declared (let, undefined)
 * before tick and guarded.
 */
export function PaymentCountdown({ expiresAt, onExpired }: PaymentCountdownProps) {
  const [remaining, setRemaining] = useState<number>(() =>
    Math.max(0, new Date(expiresAt).getTime() - Date.now()),
  );
  const onExpiredRef = useRef(onExpired);
  onExpiredRef.current = onExpired;

  // Guard so onExpired fires at most once per expiresAt value.
  const firedRef = useRef(false);
  useEffect(() => {
    firedRef.current = false;
  }, [expiresAt]);

  useEffect(() => {
    let id: number | undefined;
    const tick = () => {
      const ms = Math.max(0, new Date(expiresAt).getTime() - Date.now());
      setRemaining(ms);
      if (ms <= 0) {
        if (!firedRef.current) {
          firedRef.current = true;
          void onExpiredRef.current();
        }
        if (id !== undefined) window.clearInterval(id);
        id = undefined;
      }
    };
    tick();
    if (!firedRef.current) {
      id = window.setInterval(tick, 1_000);
    }
    return () => {
      if (id !== undefined) window.clearInterval(id);
    };
  }, [expiresAt]);

  const label = formatCountdownLabel(remaining);
  const isUrgent = remaining < 60_000;

  return (
    <span
      className={`inline font-mono font-semibold tabular-nums ${isUrgent ? "text-rose-600" : "text-amber-600"}`}
      data-testid="payment-countdown"
    >
      {label}
    </span>
  );
}
