"use client";

import React, { useEffect, useRef, useState } from "react";
import { Gift } from "lucide-react";
import { getPointsEarned, type PointsEarnedResult } from "@/api/loyalty";
import { getApiErrorCode } from "@/utils/apiError";
import { useGuestTranslation } from "@/i18n/GuestTranslationProvider";

interface LoyaltyEarnedCardProps {
  tableCode: string;
  billNumber: string;
}

const RETRY_DELAYS_MS = [2_000, 5_000];

// Post-payment loyalty confirmation for signed-in members. The settlement
// visit is written asynchronously after the bill settles, so a
// visit_not_recorded miss retries briefly. Anonymous guests (401) and
// non-members render nothing — CRMSignupCard owns the anonymous prompt.
export default function LoyaltyEarnedCard({ tableCode, billNumber }: LoyaltyEarnedCardProps) {
  const { t } = useGuestTranslation();
  const [result, setResult] = useState<PointsEarnedResult | null>(null);
  const timersRef = useRef<ReturnType<typeof setTimeout>[]>([]);

  useEffect(() => {
    let cancelled = false;
    const attempt = (retryIndex: number) => {
      getPointsEarned(tableCode, billNumber)
        .then((data) => {
          if (!cancelled) setResult(data);
        })
        .catch((err: unknown) => {
          if (cancelled) return;
          if (getApiErrorCode(err) !== "visit_not_recorded") return;
          const delay = RETRY_DELAYS_MS[retryIndex];
          if (delay !== undefined) {
            const id = setTimeout(() => attempt(retryIndex + 1), delay);
            timersRef.current.push(id);
          }
        });
    };
    attempt(0);
    return () => {
      cancelled = true;
      timersRef.current.forEach(clearTimeout);
      timersRef.current = [];
    };
  }, [tableCode, billNumber]);

  if (!result || result.points_earned <= 0) return null;

  return (
    <section
      className="mb-6 overflow-hidden rounded-2xl border border-amber-200 bg-amber-50/60"
      data-testid="loyalty-earned-card"
    >
      <div className="flex items-center gap-3 px-5 py-4">
        <div className="flex h-8 w-8 flex-shrink-0 items-center justify-center rounded-lg bg-amber-100">
          <Gift className="h-4 w-4 text-amber-700" strokeWidth={1.75} />
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-sm font-semibold text-amber-900">
            {t("bill.pointsEarned", { points: result.points_earned })}
          </p>
          <p className="text-xs text-amber-700">
            {t("bill.pointsBalance", { points: result.total_points })}
          </p>
        </div>
      </div>
    </section>
  );
}
