"use client";

import React, { useCallback, useEffect, useRef, useState } from "react";
import { Hand, Check, ChevronRight, Clock, X } from "lucide-react";
import {
  createServiceCall,
  getServiceCallStatus,
  ServiceCallCooldownError,
  type ServiceCallReason,
} from "../../api/bills";
import { useGuestTranslation } from "../../i18n/GuestTranslationProvider";
import { toast } from "react-hot-toast";

interface CallWaiterButtonProps {
  tableCode: string;
  /** When true, show a soft hint that staff may still help (closed hours). */
  businessClosed?: boolean;
}

type Phase =
  | "idle"
  | "choosing"
  | "open"
  | "acknowledged"
  | "resolved"
  | "cooldown";

const POLL_INTERVAL_MS = 10_000;
const RESOLVED_BEAT_MS = 4_000;
const COOLDOWN_FALLBACK_SECONDS = 20;

const REASONS: { reason: ServiceCallReason; labelKey: string }[] = [
  { reason: "water", labelKey: "serviceCall.reasonWater" },
  { reason: "order", labelKey: "serviceCall.reasonOrder" },
  { reason: "check", labelKey: "serviceCall.reasonCheck" },
];

// Guest "raise a hand" affordance for the table landing page. State machine:
// idle → choosing (reason chips) → open (waiting) → acknowledged (on the way)
// → resolved (brief check) → idle. Chips stay mounted while a call is live so
// the guest can send a second reason without the chooser collapsing, and the
// open/acked copy always echoes the last chosen reason (#82).
const CallWaiterButton: React.FC<CallWaiterButtonProps> = ({
  tableCode,
  businessClosed = false,
}) => {
  const { t } = useGuestTranslation();
  const [phase, setPhase] = useState<Phase>("idle");
  const [selectedReason, setSelectedReason] =
    useState<ServiceCallReason | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const phaseRef = useRef<Phase>("idle");
  const timersRef = useRef<ReturnType<typeof setTimeout>[]>([]);
  const mountedRef = useRef(true);
  const idleButtonRef = useRef<HTMLButtonElement>(null);
  const firstChipRef = useRef<HTMLButtonElement>(null);
  const statusPanelRef = useRef<HTMLDivElement>(null);
  const prevPhaseRef = useRef<Phase>("idle");

  const reasonLabel = (reason: ServiceCallReason | null): string | null => {
    if (!reason) return null;
    const match = REASONS.find((entry) => entry.reason === reason);
    return match ? t(match.labelKey) : null;
  };

  const transition = useCallback((next: Phase) => {
    if (!mountedRef.current) return;
    phaseRef.current = next;
    setPhase(next);
  }, []);

  const schedule = useCallback((fn: () => void, delayMs: number) => {
    const id = setTimeout(() => {
      timersRef.current = timersRef.current.filter((t) => t !== id);
      fn();
    }, delayMs);
    timersRef.current.push(id);
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      timersRef.current.forEach(clearTimeout);
      timersRef.current = [];
    };
  }, []);

  // Resume an in-flight call after a reload so the waiting/acknowledged state
  // (and the last reason) survives navigation back to the landing page.
  useEffect(() => {
    let cancelled = false;
    getServiceCallStatus(tableCode)
      .then((snapshot) => {
        if (cancelled || !mountedRef.current) return;
        if (snapshot.reason) setSelectedReason(snapshot.reason);
        if (snapshot.status === "open") transition("open");
        else if (snapshot.status === "acknowledged") transition("acknowledged");
      })
      .catch(() => {
        // Non-fatal: the button just starts idle.
      });
    return () => {
      cancelled = true;
    };
  }, [tableCode, transition]);

  // Poll the call status every 10s while a call is live.
  useEffect(() => {
    if (phase !== "open" && phase !== "acknowledged") return;

    const intervalID = setInterval(() => {
      getServiceCallStatus(tableCode)
        .then((snapshot) => {
          if (!mountedRef.current) return;
          const current = phaseRef.current;
          if (current !== "open" && current !== "acknowledged") return;
          if (snapshot.reason) setSelectedReason(snapshot.reason);
          if (snapshot.status === "acknowledged") {
            transition("acknowledged");
          } else if (snapshot.status === "resolved" || snapshot.status === "none") {
            transition("resolved");
            schedule(() => {
              setSelectedReason(null);
              transition("idle");
            }, RESOLVED_BEAT_MS);
          }
        })
        .catch(() => {
          // Transient poll failure: keep the current state and retry next tick.
        });
    }, POLL_INTERVAL_MS);

    return () => clearInterval(intervalID);
  }, [phase, tableCode, transition, schedule]);

  // Keep keyboard/screen-reader focus attached through phase changes: the
  // idle button and the chooser unmount when the phase flips, which would
  // otherwise drop focus onto <body>.
  useEffect(() => {
    const prev = prevPhaseRef.current;
    prevPhaseRef.current = phase;
    if (phase === prev) return;
    if (phase === "choosing") {
      firstChipRef.current?.focus();
    } else if (prev === "choosing") {
      if (phase === "idle") {
        idleButtonRef.current?.focus();
      } else if (phase === "cooldown" || phase === "open" || phase === "acknowledged") {
        statusPanelRef.current?.focus();
      }
    }
  }, [phase]);

  const handleReason = async (reason: ServiceCallReason) => {
    if (submitting) return;
    setSelectedReason(reason);
    setSubmitting(true);
    const live = phaseRef.current === "open" || phaseRef.current === "acknowledged";
    try {
      const status = await createServiceCall(tableCode, reason);
      transition(status === "acknowledged" ? "acknowledged" : "open");
    } catch (error) {
      if (error instanceof ServiceCallCooldownError) {
        transition("cooldown");
        const seconds =
          error.retryAfterSeconds > 0
            ? error.retryAfterSeconds
            : COOLDOWN_FALLBACK_SECONDS;
        schedule(() => {
          setSelectedReason(null);
          transition("idle");
        }, seconds * 1000);
      } else {
        // Surface the failure instead of silently snapping back to idle, which
        // read as "sent" — the guest thought staff had been called.
        if (!live) {
          setSelectedReason(null);
          transition("idle");
        }
        toast.error(t("serviceCall.error"));
      }
    } finally {
      if (mountedRef.current) setSubmitting(false);
    }
  };

  if (phase === "idle") {
    return (
      <div className="flex flex-col gap-1.5">
        <button
          ref={idleButtonRef}
          type="button"
          onClick={() => transition("choosing")}
          className="pointer-events-auto group relative z-10 flex w-full items-center gap-4 rounded-2xl border border-warm-200 bg-white px-5 py-4 text-left transition-colors hover:border-brand/40"
        >
          <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-brand/10 text-brand">
            <Hand className="h-5 w-5" strokeWidth={1.75} />
          </div>
          <p className="flex-1 text-heading-sm text-ink-900">
            {t("serviceCall.button")}
          </p>
          <ChevronRight
            className="h-5 w-5 text-ink-400 transition-transform group-hover:translate-x-0.5 group-hover:text-brand rtl:rotate-180"
            strokeWidth={1.75}
          />
        </button>
        {businessClosed ? (
          <p
            data-testid="call-waiter-closed-hint"
            className="px-1 text-body-sm text-ink-500"
          >
            {t("serviceCall.closedHint") ||
              "Staff may still help with your bill or questions."}
          </p>
        ) : null}
      </div>
    );
  }

  if (phase === "cooldown") {
    return (
      <div
        ref={statusPanelRef}
        tabIndex={-1}
        role="status"
        className="flex items-center gap-4 rounded-2xl border border-dashed border-warm-200 bg-white px-5 py-4 text-ink-500 focus:outline-none"
      >
        <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-warm-100">
          <Clock className="h-5 w-5 text-ink-400" strokeWidth={1.75} />
        </div>
        <p className="flex-1 text-body-sm text-ink-600">
          {t("serviceCall.cooldown")}
        </p>
      </div>
    );
  }

  if (phase === "resolved") {
    return (
      <div
        ref={statusPanelRef}
        tabIndex={-1}
        className="flex items-center gap-4 rounded-2xl border border-warm-200 bg-white px-5 py-4 focus:outline-none"
        role="status"
      >
        <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-brand/10 text-brand">
          <Check className="h-5 w-5" strokeWidth={1.75} />
        </div>
        <p className="flex-1 text-heading-sm text-ink-900">
          {t("serviceCall.resolved")}
        </p>
      </div>
    );
  }

  const isLive = phase === "open" || phase === "acknowledged";
  const isAcknowledged = phase === "acknowledged";
  const echoedReason = reasonLabel(selectedReason);
  const handleChooserEscape = (event: React.KeyboardEvent) => {
    if (event.key === "Escape" && phase === "choosing" && !submitting) {
      event.stopPropagation();
      transition("idle");
    }
  };

  return (
    <div
      className="pointer-events-auto relative z-10 flex flex-col gap-3 rounded-2xl border border-brand/30 bg-white px-5 py-4"
      role={isLive ? undefined : "group"}
      aria-label={isLive ? undefined : t("serviceCall.button")}
    >
      <div
        ref={isLive ? statusPanelRef : undefined}
        tabIndex={isLive ? -1 : undefined}
        role={isLive ? "status" : undefined}
        className="flex items-center gap-3 focus:outline-none"
      >
        <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-brand/10 text-brand">
          {isLive ? (
            <span className="relative flex h-2.5 w-2.5">
              <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-brand opacity-60" />
              <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-brand" />
            </span>
          ) : (
            <Hand className="h-5 w-5" strokeWidth={1.75} />
          )}
        </div>
        <div className="min-w-0 flex-1">
          <p className="text-heading-sm text-ink-900">
            {isLive
              ? isAcknowledged
                ? t("serviceCall.onTheWay")
                : t("serviceCall.waiting")
              : t("serviceCall.button")}
          </p>
          {isLive && echoedReason ? (
            <p
              data-testid="call-waiter-reason-echo"
              className="mt-0.5 text-body-sm text-ink-500"
            >
              {t("serviceCall.askedFor", { reason: echoedReason })}
            </p>
          ) : null}
        </div>
        {phase === "choosing" ? (
          <button
            type="button"
            disabled={submitting}
            onClick={() => transition("idle")}
            onKeyDown={handleChooserEscape}
            aria-label={t("serviceCall.dismiss")}
            className="-me-1 flex h-8 w-8 items-center justify-center rounded-lg text-ink-400 transition-colors hover:bg-warm-100 hover:text-ink-600 disabled:opacity-50"
          >
            <X className="h-4 w-4" strokeWidth={1.75} />
          </button>
        ) : null}
      </div>
      <div className="flex flex-wrap gap-2">
        {REASONS.map(({ reason, labelKey }, index) => {
          const selected = selectedReason === reason;
          return (
            <button
              key={reason}
              ref={index === 0 ? firstChipRef : undefined}
              type="button"
              disabled={submitting}
              aria-pressed={isLive ? selected : undefined}
              onClick={() => void handleReason(reason)}
              onKeyDown={handleChooserEscape}
              className={`inline-flex items-center rounded-full border px-4 py-2 text-sm font-medium transition-colors disabled:opacity-50 ${
                selected
                  ? "border-brand bg-brand/10 text-brand"
                  : "border-warm-200 bg-white text-ink-700 hover:border-brand/60 hover:text-brand"
              }`}
            >
              {t(labelKey)}
            </button>
          );
        })}
      </div>
    </div>
  );
};

export default CallWaiterButton;
