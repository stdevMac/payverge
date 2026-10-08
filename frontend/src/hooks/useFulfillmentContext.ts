"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import type { DeliveryQuoteDto } from "@/api/delivery";

interface FulfillmentDeliveryAddress {
  street: string;
  apartment?: string;
  city: string;
  state?: string;
  postal_code?: string;
  country: string;
  formatted_address: string;
}

export interface FulfillmentContext {
  business_id: number;
  custom_url?: string;
  mode: "delivery";
  delivery_address: FulfillmentDeliveryAddress;
  delivery_instructions?: string;
  contactless_delivery: boolean;
  leave_at_door: boolean;
  quote: DeliveryQuoteDto;
  saved_at: string;
}

const getStorageKey = (businessId: number) =>
  `payverge_delivery_context_${businessId}`;

const isDeliveryContext = (value: unknown): value is FulfillmentContext => {
  if (!value || typeof value !== "object") {
    return false;
  }

  const context = value as Record<string, unknown>;
  return (
    typeof context.business_id === "number" &&
    typeof context.saved_at === "string" &&
    typeof context.mode === "string" &&
    context.mode === "delivery" &&
    typeof context.delivery_address === "object" &&
    context.delivery_address !== null &&
    typeof context.quote === "object" &&
    context.quote !== null
  );
};

const readFulfillmentContext = (
  businessId?: number,
): FulfillmentContext | null => {
  if (!businessId || typeof window === "undefined") {
    return null;
  }

  try {
    const raw = window.sessionStorage.getItem(getStorageKey(businessId));
    if (!raw) {
      return null;
    }

    const parsed = JSON.parse(raw);
    if (isDeliveryContext(parsed)) {
      return parsed;
    }
  } catch (error) {
    console.warn("Failed to parse fulfillment context", error);
  }

  return null;
};

export function useFulfillmentContext(businessId?: number) {
  const [context, setContextState] = useState<FulfillmentContext | null>(() =>
    readFulfillmentContext(businessId),
  );

  useEffect(() => {
    setContextState(readFulfillmentContext(businessId));
  }, [businessId]);

  const setContext = useCallback(
    (nextContext: Omit<FulfillmentContext, "saved_at" | "mode">) => {
      if (!businessId || typeof window === "undefined") {
        return;
      }

      const value: FulfillmentContext = {
        ...nextContext,
        mode: "delivery",
        saved_at: new Date().toISOString(),
      };

      window.sessionStorage.setItem(getStorageKey(businessId), JSON.stringify(value));
      setContextState(value);
    },
    [businessId],
  );

  const clearContext = useCallback(() => {
    if (!businessId || typeof window === "undefined") {
      setContextState(null);
      return;
    }

    window.sessionStorage.removeItem(getStorageKey(businessId));
    setContextState(null);
  }, [businessId]);

  return useMemo(
    () => ({
      context,
      setContext,
      clearContext,
    }),
    [clearContext, context, setContext],
  );
}

export const fulfillmentContextStorageKey = getStorageKey;
