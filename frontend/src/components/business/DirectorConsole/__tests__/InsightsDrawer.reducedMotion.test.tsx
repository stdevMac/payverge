/** @jest-environment jsdom */
import React from "react";
import { render } from "@testing-library/react";

import type { Business } from "@/api/business";
import type { BriefingResponse } from "@/api/directorConsole";

// Capture the props NextUI's Drawer is invoked with so we can assert that the
// slide animation is disabled for reduced-motion users. Every other NextUI
// export used by the drawer is passed through as a thin presentational stub.
const drawerProps: Array<Record<string, unknown>> = [];
jest.mock("@nextui-org/react", () => ({
  Drawer: (props: Record<string, unknown>) => {
    drawerProps.push(props);
    return props.isOpen ? <div>{props.children as React.ReactNode}</div> : null;
  },
  DrawerContent: ({ children }: { children: unknown }) => (
    <div>{typeof children === "function" ? (children as () => React.ReactNode)() : (children as React.ReactNode)}</div>
  ),
  DrawerBody: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));

// useReducedMotion is the lever under test.
let mockReducedMotion = false;
jest.mock("framer-motion", () => ({
  ...jest.requireActual("framer-motion"),
  useReducedMotion: () => mockReducedMotion,
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

jest.mock("../BriefingContent", () => ({
  __esModule: true,
  default: () => <div data-testid="dc-briefing-read" />,
}));
jest.mock("../PreShiftCard", () => ({
  __esModule: true,
  default: () => <div data-testid="preshift-card" />,
}));

import InsightsDrawer from "../InsightsDrawer";

const business = { id: 42, default_currency: "USD" } as unknown as Business;

function briefing(): BriefingResponse {
  return {
    state: "active",
    pulse: {
      revenue: 4200,
      projected: 4700,
      typical_day: 4200,
      pace_pct: 12,
      orders: 84,
      avg_ticket: 37,
      food_cost_pct: 0.28,
      labor_cost_pct: 0.24,
      open_bills: 3,
    } as unknown as BriefingResponse["pulse"],
    insights: [],
    play: null,
    win: null,
  };
}

describe("InsightsDrawer reduced motion", () => {
  beforeEach(() => {
    drawerProps.length = 0;
  });

  it("disables the drawer slide animation when the user prefers reduced motion", () => {
    mockReducedMotion = true;
    render(
      <InsightsDrawer
        open
        onClose={jest.fn()}
        briefing={briefing()}
        business={business}
        onOpenTab={jest.fn()}
      />,
    );
    expect(drawerProps).toHaveLength(1);
    expect(drawerProps[0].disableAnimation).toBe(true);
  });

  it("keeps the drawer slide animation when reduced motion is not requested", () => {
    mockReducedMotion = false;
    render(
      <InsightsDrawer
        open
        onClose={jest.fn()}
        briefing={briefing()}
        business={business}
        onOpenTab={jest.fn()}
      />,
    );
    expect(drawerProps).toHaveLength(1);
    expect(drawerProps[0].disableAnimation).toBe(false);
  });
});
