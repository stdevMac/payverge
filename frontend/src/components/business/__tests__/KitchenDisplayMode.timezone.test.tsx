/** @jest-environment jsdom */
/**
 * #662: KDS cards must print the venue clock for the filed 22:12Z stamp.
 * Expected New York walls: EN 6:12 PM, es 18:12, es-AR 6:12 p. m. — never 22:12.
 */
import React from "react";
import { cleanup, render, screen } from "@testing-library/react";
import {
  FILED_NOW,
  FILED_UTC_FIRE,
  expectVenueNyFireClock,
  type OperatorLocale,
} from "./_kdsVenueClock";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  getTranslation: (key: string) => {
    if (key.endsWith("time.elapsedAgo")) return "{{duration}} ago";
    if (key.endsWith("time.justNow")) return "Just now";
    return key;
  },
}));

jest.mock("@/api/orders", () => ({
  updateOrderStatus: jest.fn(),
  parseOrderItems: (items: string) => JSON.parse(items || "[]"),
}));

jest.mock(
  "@/components/business/operational-alerts/EnableAlertSoundButton",
  () => ({ __esModule: true, default: () => null }),
);
jest.mock(
  "@/components/business/operational-alerts/OperationalAlertClaimStatus",
  () => ({ __esModule: true, default: () => null }),
);

import KitchenDisplayMode from "@/components/business/KitchenDisplayMode";
import type { Order } from "@/api/orders";
import type { Locale } from "@/i18n/config";

beforeEach(() => {
  jest.useFakeTimers();
  jest.setSystemTime(new Date(FILED_NOW));
});

afterEach(() => {
  cleanup();
  jest.useRealTimers();
});

const order = {
  id: 1,
  bill_id: 10,
  business_id: 1,
  order_number: "K-NY",
  status: "approved",
  items: "[]",
  currency: "USD",
  created_at: FILED_UTC_FIRE,
  updated_at: FILED_UTC_FIRE,
} as Order;

it.each(["en", "es", "es-AR"] as const)(
  "prints the filed 22:12Z stamp as the New York clock in %s",
  (locale: OperatorLocale) => {
    render(
      <KitchenDisplayMode
        orders={[order]}
        onExit={jest.fn()}
        onRefresh={jest.fn()}
        businessId={1}
        locale={locale as Locale}
        onOrderStatusChange={jest.fn()}
        businessTimezone="America/New_York"
      />,
    );
    expect(screen.getByText("#K-NY")).toBeInTheDocument();
    expectVenueNyFireClock(
      screen.getByTestId("kds-ticket-fire-time").textContent ?? "",
      locale,
    );
    const elapsed = screen.getByTestId("kds-ticket-elapsed").textContent ?? "";
    expect(elapsed).toMatch(/5m/);
    expect(elapsed).not.toMatch(/4h/);
  },
);
