/** @jest-environment jsdom */
/**
 * Round-3 handoff residual — Director Console tier-loading geometry.
 *
 * The console never renders through `DashboardTabShell`: it is a fixed-height
 * two-pane chat layout (`h-[calc(100dvh-8rem)]` flex column), which the shell's
 * `space-y-5` scrolling container cannot express. That much is deliberate.
 *
 * What was NOT deliberate: the `accessLoading` branch early-returned a bare
 * `<DirectorSkeleton />`, whose own wrapper is `p-4` and full-bleed. So on a
 * wide monitor the tab painted edge-to-edge and content-height while the tier
 * resolved, then snapped to `mx-auto max-w-7xl` at a fixed viewport height the
 * moment it did — a double jump (width AND height) on every visit to the tab.
 *
 * This is the same *class* as S-9 (chrome must not disappear while loading),
 * reached by the only route available to a component that legitimately cannot
 * use the shell: the loading state must reserve the loaded state's geometry.
 *
 * Locked here rather than in the skeleton, because `DirectorSkeleton` is also
 * rendered *inside* the loaded layout (message-list placeholder) where full
 * width is correct — the wrapper is the console's job, not the skeleton's.
 */

import React from "react";
import { render, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => {
    if (key === "directorConsole.quickPrompts") return ["Plan a promo"];
    return key;
  },
}));

jest.mock("@/hooks/useBusinessAccess", () => ({
  useBusinessAccess: jest.fn(),
}));

jest.mock("@/hooks/useAnalytics", () => ({
  useClickTracking: () => jest.fn(),
}));

jest.mock("@/api/directorConsole", () => ({
  getDirectorThreadMessages: jest.fn(),
  listDirectorThreads: jest.fn(),
  submitDirectorFeedback: jest.fn(),
}));

// Same rationale as Layout.test.tsx: BriefingStrip pulls the briefing API +
// shared SSE EventSource, which jsdom lacks and this suite does not exercise.
jest.mock("../BriefingStrip", () => ({ __esModule: true, default: () => null }));
jest.mock("../InsightsDrawer", () => ({ __esModule: true, default: () => null }));

import DirectorConsoleDashboard from "../DirectorConsoleDashboard";
import {
  getDirectorThreadMessages,
  listDirectorThreads,
} from "@/api/directorConsole";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";

const fakeBusiness = {
  id: 7,
  name: "Taqueria Verge",
  ai_settings: { ai_name: "Sage" },
} as any;

function primeTier(overrides: Record<string, unknown>) {
  (useBusinessAccess as jest.Mock).mockReturnValue({
    access: null,
    error: null,
    isSuspended: false,
    lockState: "active",
    refetch: jest.fn(),
    loading: false,
    hasAccess: true,
    aiConfigured: true,
    ...overrides,
  });
  (listDirectorThreads as jest.Mock).mockResolvedValue({ threads: [] });
  (getDirectorThreadMessages as jest.Mock).mockResolvedValue({ messages: [] });
}

/** Geometry the tab must hold in EVERY non-locked state. */
const GEOMETRY = ["mx-auto", "max-w-7xl", "h-[calc(100dvh-8rem)]"];

describe("Director console — tier-loading reserves the loaded geometry", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    (window.matchMedia as jest.Mock).mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: jest.fn(),
      removeListener: jest.fn(),
      addEventListener: jest.fn(),
      removeEventListener: jest.fn(),
      dispatchEvent: jest.fn(),
    }));
  });

  it("while the tier is loading, the root carries the same width and height as the loaded console", () => {
    primeTier({ loading: true });

    const { container } = render(
      <DirectorConsoleDashboard business={fakeBusiness} />,
    );

    const root = container.firstElementChild as HTMLElement;
    expect(root).toBeTruthy();
    for (const cls of GEOMETRY) {
      expect(root.className).toContain(cls);
    }
  });

  it("keeps the skeleton's busy semantics while loading", () => {
    primeTier({ loading: true });

    const { container } = render(
      <DirectorConsoleDashboard business={fakeBusiness} />,
    );

    expect(container.querySelector('[role="status"]')).toBeTruthy();
    expect(container.querySelector('[aria-busy="true"]')).toBeTruthy();
  });

  it("loaded console carries the same geometry — so nothing snaps when the tier resolves", async () => {
    primeTier({ loading: false });

    const { container } = render(
      <DirectorConsoleDashboard business={fakeBusiness} />,
    );

    await waitFor(() => expect(listDirectorThreads).toHaveBeenCalled());

    const root = container.firstElementChild as HTMLElement;
    for (const cls of GEOMETRY) {
      expect(root.className).toContain(cls);
    }
  });

  it("the locked access-gate view is still a full bypass — it must NOT be wrapped in console geometry", () => {
    primeTier({ loading: false, hasAccess: false });

    const { container } = render(
      <DirectorConsoleDashboard business={fakeBusiness} />,
    );

    const root = container.firstElementChild as HTMLElement;
    expect(root.className).not.toContain("h-[calc(100dvh-8rem)]");
  });
});
