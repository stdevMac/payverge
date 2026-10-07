"use client";

import React from "react";
import { Check } from "lucide-react";

const STEPS = [
  { status: "pending", labelKey: "orders.statusSent" },
  { status: "approved", labelKey: "orders.statusAccepted" },
  { status: "in_kitchen", labelKey: "orders.statusInKitchen" },
  { status: "ready", labelKey: "orders.statusReady" },
] as const;

const STATUS_INDEX: Record<string, number> = {
  pending: 0,
  approved: 1,
  in_kitchen: 2,
  ready: 3,
  delivered: 3,
};

/**
 * G-6: four-step guest order progress (sent → accepted → in the kitchen →
 * ready), driven by `order.status` from the existing 10s bill poll.
 */
function OrderStatusTimeline({
  status,
  t,
}: {
  status: string;
  t: (key: string, params?: Record<string, string | number>) => string;
}) {
  const activeIndex = STATUS_INDEX[status] ?? 0;
  const readyAnnouncement =
    status === "ready" || status === "delivered"
      ? t("orders.readyAnnouncement")
      : "";
  return (
    <>
      <span
        className="sr-only"
        role="status"
        aria-live="polite"
        data-testid="order-status-live"
      >
        {readyAnnouncement}
      </span>
      <ol
        className="flex items-start gap-1"
        aria-label={t("orders.statusTimeline")}
      >
        {STEPS.map((step, index) => {
          const reached = index <= activeIndex;
          const current = index === activeIndex;
          return (
            <li
              key={step.status}
              className="flex flex-1 flex-col items-center gap-1"
            >
              <span
                className={`flex h-5 w-5 items-center justify-center rounded-full text-[10px] font-semibold ${
                  reached
                    ? "bg-brand text-white"
                    : "bg-warm-100 text-ink-600 ring-1 ring-warm-200"
                }`}
                aria-hidden="true"
              >
                {reached && index < activeIndex ? (
                  <Check className="h-3 w-3" strokeWidth={2.5} />
                ) : (
                  index + 1
                )}
              </span>
              <span
                className={`text-center text-xs leading-tight ${
                  current ? "font-semibold text-ink-900" : "text-ink-600"
                }`}
                aria-current={current ? "step" : undefined}
              >
                {t(step.labelKey)}
              </span>
            </li>
          );
        })}
      </ol>
    </>
  );
}

export default OrderStatusTimeline;
