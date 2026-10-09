"use client";

import React from "react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import PaymentCelebration, { type MoneyMoment } from "./PaymentCelebration";
import { parsePaymentEvent } from "./paymentMoment";

interface MoneyMomentsContextValue {
  /**
   * Celebrate a `payment.received` SSE payload. Returns false (and shows
   * nothing) when the payload isn't a real positive payment.
   */
  celebrate: (data: Record<string, unknown>) => boolean;
}

const MoneyMomentsContext = React.createContext<MoneyMomentsContextValue | null>(null);

/** Trigger celebrations from anywhere inside the provider; no-op outside it. */
export function useMoneyMoments(): MoneyMomentsContextValue {
  return React.useContext(MoneyMomentsContext) ?? { celebrate: () => false };
}

export interface MoneyMomentsProviderProps {
  /** ISO currency code for formatting the amount. */
  currency?: string;
  /** Maximum simultaneously-visible celebrations (oldest drop off). */
  maxVisible?: number;
  /** Auto-dismiss delay per celebration, in ms. */
  durationMs?: number;
  children: React.ReactNode;
}

export function MoneyMomentsProvider({
  currency = "USD",
  maxVisible = 3,
  durationMs = 6000,
  children,
}: MoneyMomentsProviderProps) {
  const { locale } = useSimpleLocale();
  const [moments, setMoments] = React.useState<MoneyMoment[]>([]);
  const seqRef = React.useRef(0);
  const timersRef = React.useRef<Map<string, ReturnType<typeof setTimeout>>>(new Map());

  const t = React.useCallback(
    (key: string): string => {
      const r = getTranslation(key, locale);
      return Array.isArray(r) ? r[0] || key : (r as string);
    },
    [locale],
  );

  const clearTimer = React.useCallback((id: string) => {
    const timer = timersRef.current.get(id);
    if (timer) {
      clearTimeout(timer);
      timersRef.current.delete(id);
    }
  }, []);

  const dismiss = React.useCallback(
    (id: string) => {
      setMoments((prev) => prev.filter((m) => m.id !== id));
      clearTimer(id);
    },
    [clearTimer],
  );

  const celebrate = React.useCallback(
    (data: Record<string, unknown>): boolean => {
      const parsed = parsePaymentEvent(data);
      if (!parsed) return false;

      seqRef.current += 1;
      const id = `mm-${seqRef.current}`;
      const moment: MoneyMoment = {
        id,
        amount: parsed.amount,
        method: parsed.method,
        billId: parsed.billId,
      };

      setMoments((prev) => {
        const next = [...prev, moment];
        if (next.length <= maxVisible) return next;
        // Drop the oldest beyond the cap and cancel their timers.
        const dropped = next.slice(0, next.length - maxVisible);
        dropped.forEach((d) => clearTimer(d.id));
        return next.slice(next.length - maxVisible);
      });

      const timer = setTimeout(() => {
        setMoments((prev) => prev.filter((m) => m.id !== id));
        timersRef.current.delete(id);
      }, durationMs);
      timersRef.current.set(id, timer);

      return true;
    },
    [maxVisible, durationMs, clearTimer],
  );

  React.useEffect(() => {
    const timers = timersRef.current;
    return () => {
      timers.forEach((timer) => clearTimeout(timer));
      timers.clear();
    };
  }, []);

  const ctx = React.useMemo(() => ({ celebrate }), [celebrate]);

  return (
    <MoneyMomentsContext.Provider value={ctx}>
      {children}
      <PaymentCelebration
        moments={moments}
        currency={currency}
        t={t}
        onDismiss={dismiss}
      />
    </MoneyMomentsContext.Provider>
  );
}
