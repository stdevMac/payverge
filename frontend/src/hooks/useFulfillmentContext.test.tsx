/** @jest-environment jsdom */

import { act, renderHook } from "@testing-library/react";
import {
  fulfillmentContextStorageKey,
  useFulfillmentContext,
} from "@/hooks/useFulfillmentContext";
import { asDollars } from "@/types/money";

const buildContextInput = () => ({
  business_id: 42,
  custom_url: "demo-bistro",
  delivery_address: {
    street: "1 Main St",
    city: "Dubai",
    country: "AE",
    formatted_address: "1 Main St, Dubai, AE",
  },
  delivery_instructions: "Leave at concierge",
  contactless_delivery: true,
  leave_at_door: false,
  quote: {
    eligible: true,
    reason_code: "eligible",
    message: "Ready",
    delivery_fee: asDollars(5),
    minimum_order_amount: asDollars(20),
    free_delivery_minimum: asDollars(40),
    order_subtotal: asDollars(25),
    meets_minimum: true,
    estimated_prep_time: 20,
    estimated_delivery_minutes: 15,
    estimated_total_minutes: 35,
    fulfillment_mode: "in_house",
    partner_fallback_available: false,
    external_partner_links: [],
  },
});

describe("useFulfillmentContext", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
  });

  it("persists delivery quote context and reloads it for the same business", () => {
    const { result, unmount } = renderHook(() => useFulfillmentContext(42));

    act(() => {
      result.current.setContext(buildContextInput());
    });

    const raw = window.sessionStorage.getItem(fulfillmentContextStorageKey(42));
    expect(raw).toContain("demo-bistro");
    expect(result.current.context?.mode).toBe("delivery");

    unmount();

    const { result: reloaded } = renderHook(() => useFulfillmentContext(42));
    expect(reloaded.current.context?.delivery_address.city).toBe("Dubai");
    expect(reloaded.current.context?.quote.estimated_total_minutes).toBe(35);
  });

  it("clears stored delivery quote context", () => {
    const { result } = renderHook(() => useFulfillmentContext(42));

    act(() => {
      result.current.setContext(buildContextInput());
    });

    act(() => {
      result.current.clearContext();
    });

    expect(result.current.context).toBeNull();
    expect(window.sessionStorage.getItem(fulfillmentContextStorageKey(42))).toBeNull();
  });
});
