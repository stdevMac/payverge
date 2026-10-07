/** @jest-environment jsdom */
import React from "react";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import MarketingDashboard from "../MarketingDashboard";
import { useBusinessAccess } from "@/hooks/useBusinessAccess";
import { useMarketingSuggestions } from "../hooks/useMarketingSuggestions";
import { useSuggestionCaption } from "../hooks/useSuggestionCaptions";
import {
  useMarketingActivity,
  useMarketingActivityMutations,
} from "../hooks/useMarketingActivity";
import { useMarketingSettings } from "../hooks/useMarketingSettings";

// This suite renders the full NextUI + framer-motion MarketingDashboard tree.
// Per the documented worker-oversubscription note in jest.config.js, when
// several heavy RTL render suites co-schedule on one worker the event loop
// starves and waitFor's polling can cross the 5s wall-clock deadline even
// though the assertions themselves settle quickly. Give this render-heavy suite
// the same headroom used by src/app/admin/users/page.test.tsx rather than
// masking it with a global testTimeout bump.
jest.setTimeout(15000);

let mockStaffData: unknown = null;
let mockPermissions: string[] = [];
const mockRecordMutate = jest.fn();

jest.mock("@/hooks/useBusinessAccess");
jest.mock("../hooks/useMarketingSuggestions");
jest.mock("@/api/marketing", () => ({
  ...jest.requireActual("@/api/marketing"),
  generateMarketingCaption: jest.fn(() => Promise.resolve("Editor caption")),
  generateMarketingImage: jest.fn(),
}));
jest.mock("@/api/business", () => ({
  ...jest.requireActual("@/api/business"),
  getMenu: jest.fn().mockResolvedValue({ categories: [] }),
}));
// Own suite covers scoring; avoid async setState act() noise in the dashboard
// smoke tree when the panel races menu load against unmount/rerender.
jest.mock("../PhotoReadinessPanel", () => ({
  PhotoReadinessPanel: () => <div data-testid="photo-readiness-panel" />,
}));
jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));
jest.mock("../postContent", () => ({
  ...jest.requireActual("../postContent"),
  downloadPostPack: jest.fn(() => Promise.resolve()),
  shareOrDownloadPostPack: jest.fn(() => Promise.resolve("shared")),
  copyCaption: jest.fn(() => Promise.resolve()),
}));
jest.mock("../hooks/useSuggestionCaptions", () => ({
  useSuggestionCaption: jest.fn(),
}));
jest.mock("../hooks/useMarketingActivity");
jest.mock("../hooks/useMarketingSettings");
jest.mock("@/providers/HybridAuthProvider", () => ({
  useAuth: () => ({ staffData: mockStaffData }),
}));
jest.mock("@/contexts/StaffPermissionsContext", () => ({
  useStaffPermissionsContext: () => ({ permissions: mockPermissions }),
}));
jest.mock("../PostPreview", () => ({
  PostPreview: ({ onRenderStateChange }: any) => {
    const ReactRuntime = jest.requireActual("react") as typeof React;
    ReactRuntime.useEffect(() => {
      onRenderStateChange?.("ready");
    }, [onRenderStateChange]);
    return <div data-testid="post-preview" />;
  },
}));
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (k: string) => k,
}));
// Locked-view child renders heavy lock UI; stub to a marker.
jest.mock("../../DashboardLockedTabView", () => ({
  __esModule: true,
  default: () => <div data-testid="locked-view" />,
}));

const business = {
  id: 42,
  name: "Trattoria",
  social_media: "",
  default_currency: "USD",
  default_language: "en",
  design_settings: {},
} as any;

const tier = (over: Partial<ReturnType<typeof useBusinessAccess>>) =>
  (useBusinessAccess as jest.Mock).mockReturnValue({
    hasAccess: true,
    isSuspended: false,
    aiConfigured: true,
    loading: false,
    ...over,
  });

beforeEach(() => {
  jest.clearAllMocks();
  mockStaffData = null;
  mockPermissions = [];
  (useMarketingSuggestions as jest.Mock).mockReturnValue({
    suggestions: [],
    paused: false,
    emptyReason: "no_data",
    loading: false,
    error: null,
    refetch: jest.fn(),
  });
  (useSuggestionCaption as jest.Mock).mockReturnValue({
    caption: "Fallback caption",
    source: "fallback",
    tone: "warm",
    generating: false,
    error: null,
    retry: jest.fn(),
    selectTone: jest.fn(),
    observeRef: jest.fn(),
    canGenerate: false,
  });
  (useMarketingActivity as jest.Mock).mockReturnValue({
    items: [],
    total: 0,
    loading: false,
    error: null,
    hasMore: false,
    loadMore: jest.fn(),
    refetch: jest.fn(),
  });
  (useMarketingActivityMutations as jest.Mock).mockReturnValue({
    record: { mutate: mockRecordMutate, isPending: false },
    restore: { mutate: jest.fn() },
    locallyHidden: new Set(),
    unhide: jest.fn(),
  });
  (useMarketingSettings as jest.Mock).mockReturnValue({
    settings: {
      enabled: true,
      disabled_plays: [],
      creative_profile: { avoid_phrases: [] },
    },
    hasSnapshot: true,
    loading: false,
    saving: false,
    loadError: null,
    saveError: null,
    rollbackOccurred: false,
    saved: false,
    save: jest.fn(),
    retryLoad: jest.fn(),
    retrySave: jest.fn(),
  });
});

describe("MarketingDashboard smoke", () => {
  it("dismisses and restores a suggestion in place through the Dashboard-owned lifecycle", async () => {
    tier({});
    const suggestion = {
      id: "same-mount",
      play: "featured_dish",
      title: "Feature Carbonara",
      why_data: "Only live feed reason",
      source: "menu_engineering",
      copy_angle: "Simple",
      rank: 1,
      target_name: "Carbonara",
      image_url: "https://cdn/carbonara.jpg",
      image_source: "menu",
    };
    let resolveRefresh!: (suggestions: (typeof suggestion)[]) => void;
    const refreshSuggestions = jest.fn(
      () =>
        new Promise<(typeof suggestion)[]>((resolve) => {
          resolveRefresh = resolve;
        }),
    );
    (useMarketingSuggestions as jest.Mock).mockReturnValue({
      suggestions: [suggestion],
      paused: false,
      emptyReason: "",
      loading: false,
      error: null,
      refetch: refreshSuggestions,
    });
    (useMarketingActivity as jest.Mock).mockReturnValue({
      items: [
        {
          id: 77,
          business_id: 42,
          suggestion_id: "same-mount",
          play: "featured_dish",
          title: "Library Carbonara",
          target_name: "Carbonara",
          status: "dismissed",
          image_url: "https://cdn/carbonara.jpg",
          caption: "Archived caption",
          creative_snapshot: null,
          created_by: "owner",
          created_at: "2026-07-01T00:00:00Z",
          updated_at: "2026-07-01T00:00:00Z",
        },
      ],
      total: 1,
      loading: false,
      loadingMore: false,
      error: null,
      hasMore: false,
      loadMore: jest.fn(),
      refetch: jest.fn(),
    });
    (useMarketingActivityMutations as jest.Mock).mockImplementation(
      (_businessId, _onRecordError, refreshAuthoritativeSuggestions) => {
        const [locallyHidden, setLocallyHidden] = React.useState<Set<string>>(
          new Set(),
        );
        const [restoreState, setRestoreState] = React.useState<{
          isPending: boolean;
          variables?: string;
        }>({ isPending: false });
        const unhide = (suggestionId: string) =>
          setLocallyHidden((current) => {
            const next = new Set(current);
            next.delete(suggestionId);
            return next;
          });
        return {
          record: {
            isPending: false,
            mutate: ({ suggestion: hidden }: { suggestion: { id: string } }) =>
              setLocallyHidden((current) => new Set(current).add(hidden.id)),
          },
          restore: {
            isPending: restoreState.isPending,
            isError: false,
            variables: restoreState.variables,
            mutate: (suggestionId: string) => {
              setRestoreState({ isPending: true, variables: suggestionId });
              if (typeof refreshAuthoritativeSuggestions !== "function") return;
              void refreshAuthoritativeSuggestions().then(
                (authoritative: { id: string }[]) => {
                  if (authoritative.some((item) => item.id === suggestionId)) {
                    unhide(suggestionId);
                  }
                  setRestoreState({ isPending: false });
                },
              );
            },
          },
          locallyHidden,
          unhide,
        };
      },
    );

    render(<MarketingDashboard business={business} />);
    expect(screen.getByText("Only live feed reason")).toBeInTheDocument();

    fireEvent.click(
      screen.getByRole("button", {
        name: "marketingDashboard.card.dismiss",
      }),
    );
    await waitFor(() =>
      expect(
        screen.queryByText("Only live feed reason"),
      ).not.toBeInTheDocument(),
    );

    fireEvent.click(
      screen.getByRole("button", {
        name: "marketingDashboard.library.restore",
      }),
    );
    expect(refreshSuggestions).toHaveBeenCalledTimes(1);
    expect(screen.queryByText("Only live feed reason")).not.toBeInTheDocument();

    await act(async () => {
      resolveRefresh([suggestion]);
    });
    expect(
      await screen.findByText("Only live feed reason"),
    ).toBeInTheDocument();
  });

  it("shows the locked view when an administrator suspended the business", () => {
    tier({ hasAccess: false, isSuspended: true, lockState: "suspended" });
    render(<MarketingDashboard business={business} />);
    expect(screen.getByTestId("locked-view")).toBeInTheDocument();
  });

  it("renders the feed shell + studio entry when the business is operational", () => {
    tier({});
    render(<MarketingDashboard business={business} />);
    expect(screen.queryByTestId("locked-view")).not.toBeInTheDocument();
    // The header owns the single studio CTA; the duplicate bottom promo tile is
    // gone (no-marketing-grid dashboard rule).
    expect(screen.getByText("marketingDashboard.hero.cta")).toBeInTheDocument();
    expect(
      screen.queryByText("marketingDashboard.manualStudio.cta"),
    ).not.toBeInTheDocument();
  });

  it("seeds example posts when there are no real suggestions", () => {
    tier({});
    render(<MarketingDashboard business={business} />);
    expect(
      screen.getByText("marketingDashboard.example.heading"),
    ).toBeInTheDocument();
    expect(
      screen.getAllByRole("button", { name: /example\.cta/ }).length,
    ).toBeGreaterThan(0);
  });

  // #831: an example card's "Make it yours" must open a BLANK editor. Prefilling
  // the drawer with the fake demo dish nearly let owners publish a dish they do
  // not serve.
  it("opens a blank editor from an example card instead of prefilling the fake dish", async () => {
    tier({});
    render(<MarketingDashboard business={business} />);
    const [firstCta] = screen.getAllByRole("button", { name: /example\.cta/ });
    fireEvent.click(firstCta);
    await screen.findByLabelText("marketingDashboard.editor.caption");
    // No field may carry example-card copy (identity-t returns the i18n keys).
    expect(screen.queryByDisplayValue(/example\.cards/)).not.toBeInTheDocument();
  });

  it("renders a distinct loading state without demo examples", () => {
    tier({});
    (useMarketingSuggestions as jest.Mock).mockReturnValue({
      suggestions: [],
      paused: false,
      emptyReason: "",
      loading: true,
      error: null,
      refetch: jest.fn(),
    });
    render(<MarketingDashboard business={business} />);

    expect(screen.getByRole("status")).toHaveAttribute("aria-busy", "true");
    expect(screen.getByText("marketingDashboard.loading")).toBeInTheDocument();
    expect(
      screen.queryByText("marketingDashboard.example.heading"),
    ).not.toBeInTheDocument();
  });

  it("renders a retryable failure without masking it with examples", () => {
    tier({});
    (useMarketingSuggestions as jest.Mock).mockReturnValue({
      suggestions: [],
      paused: false,
      emptyReason: "",
      loading: false,
      error: "offline",
      refetch: jest.fn(),
    });
    render(<MarketingDashboard business={business} />);

    expect(screen.getByRole("alert")).toHaveTextContent(
      "marketingDashboard.errors.suggestions_failed",
    );
    expect(
      screen.getByRole("button", { name: "marketingDashboard.feed.retry" }),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("marketingDashboard.example.heading"),
    ).not.toBeInTheDocument();
  });

  it("renders all-handled and no-enabled-play states without examples", () => {
    tier({});
    const base = {
      suggestions: [],
      paused: false,
      loading: false,
      error: null,
      refetch: jest.fn(),
    };
    (useMarketingSuggestions as jest.Mock).mockReturnValue({
      ...base,
      emptyReason: "all_handled",
    });
    const view = render(<MarketingDashboard business={business} />);
    expect(
      screen.getByText("marketingDashboard.feed.allHandled"),
    ).toBeInTheDocument();
    expect(
      screen.queryByText("marketingDashboard.example.heading"),
    ).not.toBeInTheDocument();

    (useMarketingSuggestions as jest.Mock).mockReturnValue({
      ...base,
      emptyReason: "no_enabled_plays",
    });
    view.rerender(<MarketingDashboard business={business} />);
    expect(
      screen.getByText("marketingDashboard.feed.noEnabledPlays"),
    ).toBeInTheDocument();
  });

  it("renders the paused empty state when suggestions are paused", () => {
    tier({});
    (useMarketingSuggestions as jest.Mock).mockReturnValue({
      suggestions: [],
      paused: true,
      emptyReason: "paused",
      loading: false,
      error: null,
      refetch: jest.fn(),
    });
    render(<MarketingDashboard business={business} />);
    expect(
      screen.getByText("marketingDashboard.paused.heading"),
    ).toBeInTheDocument();
  });

  it("keeps captions read-only when staff lacks marketing write permission", () => {
    tier({});
    mockStaffData = { role: "viewer" };
    mockPermissions = ["marketing:read"];
    (useMarketingSuggestions as jest.Mock).mockReturnValue({
      suggestions: [
        {
          id: "one",
          play: "featured_dish",
          title: "Feature Carbonara",
          why_data: "Menu signal",
          source: "menu_engineering",
          copy_angle: "Simple",
          rank: 1,
          target_name: "Carbonara",
        },
      ],
      paused: false,
      emptyReason: "",
      loading: false,
      error: null,
      refetch: jest.fn(),
    });

    render(<MarketingDashboard business={business} />);

    expect(useSuggestionCaption).toHaveBeenCalledWith(
      expect.objectContaining({
        enabled: false,
        creativeProfile: expect.objectContaining({ avoid_phrases: [] }),
        priority: true,
      }),
    );
    expect(
      screen.queryByRole("button", { name: "marketingDashboard.hero.cta" }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", {
        name: "marketingDashboard.card.addPhoto",
      }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "marketingDashboard.card.tweak" }),
    ).not.toBeInTheDocument();
    expect(
      screen.getByText("marketingDashboard.feed.readOnly"),
    ).toBeInTheDocument();
  });

  it("counts only complete, settled cards as ready", async () => {
    tier({});
    const suggestions = [
      { id: "ready", image_url: "https://cdn/ready.jpg" },
      { id: "pending", image_url: "https://cdn/pending.jpg" },
      { id: "failed", image_url: "https://cdn/failed.jpg" },
      { id: "incomplete" },
    ].map((item, index) => ({
      ...item,
      play: "featured_dish",
      title: `Idea ${index}`,
      why_data: "Signal",
      source: "menu_engineering",
      copy_angle: "Simple",
      rank: 10 - index,
      target_name: `Dish ${index}`,
    }));
    (useMarketingSuggestions as jest.Mock).mockReturnValue({
      suggestions,
      paused: false,
      emptyReason: "",
      loading: false,
      error: null,
      refetch: jest.fn(),
    });
    (useSuggestionCaption as jest.Mock).mockImplementation(
      ({ suggestion }: { suggestion: { id: string } }) => ({
        caption: suggestion.id === "incomplete" ? "" : "Fallback caption",
        source: "fallback",
        tone: "warm",
        generating: suggestion.id === "pending",
        error: suggestion.id === "failed" ? new Error("failed") : null,
        retry: jest.fn(),
        selectTone: jest.fn(),
        observeRef: jest.fn(),
        canGenerate: false,
      }),
    );

    render(<MarketingDashboard business={business} />);

    await waitFor(() =>
      expect(screen.getByTestId("marketing-ready-count")).toHaveTextContent(
        "1",
      ),
    );
  });

  it("shows the recent handled activity count in the pipeline", () => {
    tier({});
    (useMarketingActivity as jest.Mock).mockReturnValue({
      items: [],
      total: 6,
      loading: false,
      error: null,
      hasMore: false,
      loadMore: jest.fn(),
      refetch: jest.fn(),
    });

    render(<MarketingDashboard business={business} />);

    expect(
      screen.getByText("marketingDashboard.hero.handledRecently"),
    ).toBeInTheDocument();
    expect(screen.getByTestId("marketing-handled-count")).toHaveTextContent(
      "6",
    );
    expect(useMarketingActivity).toHaveBeenCalledWith(
      42,
      expect.objectContaining({ handled_from: expect.any(String) }),
      true,
      1,
    );
  });

  it("puts only ready cards under the Ready to post group", async () => {
    tier({});
    (useMarketingSuggestions as jest.Mock).mockReturnValue({
      suggestions: [
        {
          id: "ready-one",
          play: "featured_dish",
          title: "Ready Carbonara",
          why_data: "Signal",
          source: "menu_engineering",
          copy_angle: "Simple",
          rank: 2,
          target_name: "Carbonara",
          image_url: "https://cdn/ready.jpg",
          image_source: "menu",
        },
        {
          id: "needs-photo",
          play: "happy_hour",
          title: "Needs a photo",
          why_data: "Slow window",
          source: "slow_dayparts",
          copy_angle: "Simple",
          rank: 1,
          target_name: "After-Work Special",
          discount_type: "percentage",
          discount_value: 15,
          metrics: { suggested_offer: "After-Work Special" },
          // no image_url → never becomes ready
        },
      ],
      paused: false,
      emptyReason: "",
      loading: false,
      error: null,
      refetch: jest.fn(),
    });
    (useSuggestionCaption as jest.Mock).mockImplementation(
      ({ suggestion }: { suggestion: { id: string } }) => ({
        caption:
          suggestion.id === "ready-one"
            ? "Ready caption"
            : "Incomplete caption",
        source: "fallback",
        tone: "warm",
        generating: false,
        error: null,
        retry: jest.fn(),
        selectTone: jest.fn(),
        observeRef: jest.fn(),
        canGenerate: false,
      }),
    );

    render(<MarketingDashboard business={business} />);

    await waitFor(() =>
      expect(screen.getByTestId("marketing-ready-group")).toBeInTheDocument(),
    );

    expect(screen.getByTestId("marketing-ready-group")).toHaveTextContent(
      "marketingDashboard.feed.readyGroup",
    );
    expect(screen.getByTestId("marketing-needs-work-group")).toHaveTextContent(
      "marketingDashboard.feed.needsWorkGroup",
    );

    // Group membership is data-export-ready (CSS order is visual only — React
    // tree order stays fixed so preview state is never remount-thrashed).
    await waitFor(() =>
      expect(screen.getByTestId("marketing-card-ready-one")).toHaveAttribute(
        "data-export-ready",
        "true",
      ),
    );
    expect(screen.getByTestId("marketing-card-needs-photo")).toHaveAttribute(
      "data-export-ready",
      "false",
    );

    // Ready cards expose Review and export; Share lives in overflow (#207).
    expect(
      within(screen.getByTestId("marketing-card-ready-one")).getByRole(
        "button",
        { name: "marketingDashboard.card.reviewExport" },
      ),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", {
        name: "marketingDashboard.card.addPhoto",
      }),
    ).toBeInTheDocument();
    expect(
      within(screen.getByTestId("marketing-card-ready-one")).queryByRole(
        "button",
        { name: "marketingDashboard.card.sharePost" },
      ),
    ).not.toBeInTheDocument();
    expect(
      within(screen.getByTestId("marketing-card-needs-photo")).queryByRole(
        "button",
        { name: "marketingDashboard.card.sharePost" },
      ),
    ).not.toBeInTheDocument();
  });

  it("retries creative settings without closing the drawer", async () => {
    tier({});
    const retryLoad = jest.fn();
    (useMarketingSettings as jest.Mock).mockReturnValue({
      settings: {
        enabled: true,
        disabled_plays: [],
        creative_profile: { avoid_phrases: [] },
      },
      hasSnapshot: false,
      loading: false,
      saving: false,
      loadError: "load failed",
      saveError: null,
      rollbackOccurred: false,
      saved: false,
      save: jest.fn(),
      retryLoad,
      retrySave: jest.fn(),
    });
    render(<MarketingDashboard business={business} />);
    fireEvent.click(
      screen.getByRole("button", { name: "marketingDashboard.automation.button" }),
    );
    expect(
      await screen.findByTestId("marketing-settings-drawer"),
    ).toBeInTheDocument();
    // Opening the drawer over a latched load error re-drives the fetch (#693).
    expect(retryLoad).toHaveBeenCalledTimes(1);
    fireEvent.click(
      screen.getByRole("button", {
        name: "marketingDashboard.settings.loadError.retry",
      }),
    );
    expect(retryLoad).toHaveBeenCalledTimes(2);
    expect(
      screen.getByTestId("marketing-settings-drawer"),
    ).toBeInTheDocument();
  });

  it("names dishes blocked by inventory instead of campaigning them", async () => {
    tier({});
    (useMarketingSuggestions as jest.Mock).mockReturnValue({
      suggestions: [
        {
          id: "salad",
          play: "move_item",
          title: "Give Garden Salad a push",
          why_data: "Signal",
          source: "menu_engineering",
          copy_angle: "x",
          rank: 1,
          target_name: "Garden Salad",
        },
      ],
      paused: false,
      emptyReason: "",
      inventoryBlocked: ["Steak Plate"],
      loading: false,
      error: null,
      refetch: jest.fn(),
    });
    render(<MarketingDashboard business={business} />);
    const banner = await screen.findByTestId("marketing-inventory-blocked");
    expect(banner).toHaveTextContent(
      "marketingDashboard.feed.inventoryBlockedTitle",
    );
    expect(banner).toHaveTextContent(
      "marketingDashboard.feed.inventoryBlockedOne",
    );
  });

  it("records the exact snapshot only after explicit published confirmation", async () => {
    tier({});
    (useMarketingSuggestions as jest.Mock).mockReturnValue({
      suggestions: [
        {
          id: "one",
          play: "featured_dish",
          title: "Feature Carbonara",
          why_data: "Menu signal",
          source: "menu_engineering",
          copy_angle: "Simple",
          rank: 1,
          target_name: "Carbonara",
          image_url: "https://cdn/carbonara.jpg",
          image_source: "menu",
        },
      ],
      paused: false,
      emptyReason: "",
      loading: false,
      error: null,
      refetch: jest.fn(),
    });
    render(<MarketingDashboard business={business} />);

    fireEvent.click(screen.getByTestId("post-card-more-actions"));
    fireEvent.click(screen.getByText("marketingDashboard.card.sharePost"));
    await screen.findByRole("dialog", {
      name: "marketingDashboard.outcome.title",
    });
    expect(mockRecordMutate).not.toHaveBeenCalled();
    fireEvent.click(
      screen.getByRole("button", {
        name: "marketingDashboard.outcome.notYet",
      }),
    );
    expect(mockRecordMutate).not.toHaveBeenCalled();
    expect(
      screen.getByText("marketingDashboard.playTitles.featured_dish"),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByTestId("post-card-more-actions"));
    fireEvent.click(screen.getByText("marketingDashboard.card.sharePost"));
    await screen.findByRole("dialog", {
      name: "marketingDashboard.outcome.title",
    });
    fireEvent.click(
      screen.getByRole("button", {
        name: "marketingDashboard.outcome.confirmPublished",
      }),
    );
    expect(mockRecordMutate).toHaveBeenCalledWith(
      expect.objectContaining({
        action: "post",
        suggestion: expect.objectContaining({ id: "one" }),
        creative_snapshot: expect.objectContaining({
          caption: "Fallback caption",
          image_url: "https://cdn/carbonara.jpg",
          image_source: "menu",
          template: "editorial",
          aspect: "4:5",
          crop: { x: 0.5, y: 0.5, zoom: 1 },
        }),
      }),
      expect.any(Object),
    );
  });

  it("does not open confirmation or record activity when sharing is dismissed", async () => {
    const { shareOrDownloadPostPack } = jest.requireMock("../postContent") as {
      shareOrDownloadPostPack: jest.Mock;
    };
    shareOrDownloadPostPack.mockRejectedValueOnce(
      new DOMException("cancel", "AbortError"),
    );
    tier({});
    (useMarketingSuggestions as jest.Mock).mockReturnValue({
      suggestions: [
        {
          id: "dismissed-share",
          play: "featured_dish",
          title: "Feature Carbonara",
          why_data: "Menu signal",
          source: "menu_engineering",
          copy_angle: "Simple",
          rank: 1,
          target_name: "Carbonara",
          image_url: "https://cdn/carbonara.jpg",
          image_source: "menu",
        },
      ],
      paused: false,
      emptyReason: "",
      loading: false,
      error: null,
      refetch: jest.fn(),
    });
    render(<MarketingDashboard business={business} />);

    fireEvent.click(screen.getByTestId("post-card-more-actions"));
    fireEvent.click(screen.getByText("marketingDashboard.card.sharePost"));
    await waitFor(() => expect(shareOrDownloadPostPack).toHaveBeenCalled());

    expect(
      screen.queryByRole("dialog", {
        name: "marketingDashboard.outcome.title",
      }),
    ).not.toBeInTheDocument();
    expect(mockRecordMutate).not.toHaveBeenCalled();
  });

  it("confirms the exact edited creative handed off by the primary editor", async () => {
    tier({});
    (useMarketingSuggestions as jest.Mock).mockReturnValue({
      suggestions: [
        {
          id: "editor-one",
          play: "featured_dish",
          title: "Feature Carbonara",
          why_data: "Menu signal",
          source: "menu_engineering",
          copy_angle: "Simple",
          rank: 1,
          target_name: "Carbonara",
          image_url: "https://cdn/carbonara.jpg",
          image_source: "menu",
        },
      ],
      paused: false,
      emptyReason: "",
      loading: false,
      error: null,
      refetch: jest.fn(),
    });
    render(<MarketingDashboard business={business} />);

    fireEvent.click(
      screen.getByRole("button", {
        name: "marketingDashboard.card.reviewExport",
      }),
    );
    await screen.findByLabelText("marketingDashboard.editor.caption");
    // Default editor mode is simple (feed formats only). Craft unlocks the full
    // format registry + kit picker the creative_snapshot assertion depends on.
    fireEvent.click(
      screen.getByRole("button", {
        name: "marketingDashboard.editor.mode.craft",
      }),
    );
    fireEvent.click(
      screen.getByRole("button", {
        name: "marketingDashboard.formats.9:16.name",
      }),
    );
    // The editor's art-direction picker replaced the template picker; the kit
    // is what the renderer reads and what the snapshot below has to carry.
    fireEvent.click(
      screen.getByRole("button", { name: "marketingDashboard.kits.bold" }),
    );
    fireEvent.change(screen.getByLabelText("marketingDashboard.crop.focalX"), {
      target: { value: "0.2" },
    });
    fireEvent.change(screen.getByLabelText("marketingDashboard.crop.focalY"), {
      target: { value: "0.8" },
    });
    fireEvent.change(screen.getByLabelText("marketingDashboard.crop.zoom"), {
      target: { value: "1.6" },
    });
    fireEvent.change(
      screen.getByLabelText("marketingDashboard.slots.dishName"),
      { target: { value: "Edited Carbonara" } },
    );
    fireEvent.change(
      screen.getByLabelText("marketingDashboard.editor.caption"),
      { target: { value: "Exact editor activity caption" } },
    );
    fireEvent.click(
      await screen.findByRole("button", {
        name: "marketingDashboard.editor.downloadPack",
      }),
    );

    await screen.findByRole("dialog", {
      name: "marketingDashboard.outcome.title",
    });
    expect(mockRecordMutate).not.toHaveBeenCalled();
    fireEvent.click(
      screen.getByRole("button", {
        name: "marketingDashboard.outcome.confirmPublished",
      }),
    );
    expect(mockRecordMutate).toHaveBeenCalledWith(
      expect.objectContaining({
        action: "post",
        suggestion: expect.objectContaining({ id: "editor-one" }),
        creative_snapshot: expect.objectContaining({
          aspect: "9:16",
          // The composer's seed, not an operator choice — the drawer no longer
          // offers a template picker.
          template: "editorial",
          kit: "bold",
          composition: "photoBottomStack",
          crop: { x: 0.2, y: 0.8, zoom: 1.6 },
          slots: expect.objectContaining({ dishName: "Edited Carbonara" }),
          caption: "Exact editor activity caption",
          image_url: "https://cdn/carbonara.jpg",
          image_source: "menu",
          // Kit typeface token (art-direction Sans), not the legacy Inter default.
          font_family: "Sans",
          // Still export — media_kind image, no motion_preset.
          media_kind: "image",
        }),
      }),
      expect.any(Object),
    );
  });

  it("keeps a failed confirmation retryable", async () => {
    tier({});
    (useMarketingSuggestions as jest.Mock).mockReturnValue({
      suggestions: [
        {
          id: "one",
          play: "featured_dish",
          title: "Feature Carbonara",
          why_data: "Menu signal",
          source: "menu_engineering",
          copy_angle: "Simple",
          rank: 1,
          target_name: "Carbonara",
          image_url: "https://cdn/carbonara.jpg",
          image_source: "menu",
        },
      ],
      paused: false,
      emptyReason: "",
      loading: false,
      error: null,
      refetch: jest.fn(),
    });
    mockRecordMutate.mockImplementation(
      (_vars, options?: { onError?: (error: Error) => void }) =>
        options?.onError?.(new Error("record failed")),
    );
    render(<MarketingDashboard business={business} />);
    fireEvent.click(screen.getByTestId("post-card-more-actions"));
    fireEvent.click(screen.getByText("marketingDashboard.card.sharePost"));
    await screen.findByRole("dialog", {
      name: "marketingDashboard.outcome.title",
    });
    fireEvent.click(
      screen.getByRole("button", {
        name: "marketingDashboard.outcome.confirmPublished",
      }),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "marketingDashboard.outcome.error",
    );
    fireEvent.click(
      screen.getByRole("button", {
        name: "marketingDashboard.outcome.retry",
      }),
    );
    expect(mockRecordMutate).toHaveBeenCalledTimes(2);
  });

  it("gates captions while creative-profile settings are being persisted", () => {
    tier({});
    (useMarketingSuggestions as jest.Mock).mockReturnValue({
      suggestions: [
        {
          id: "one",
          play: "featured_dish",
          title: "Feature Carbonara",
          why_data: "Menu signal",
          source: "menu_engineering",
          copy_angle: "Simple",
          rank: 1,
          target_name: "Carbonara",
        },
      ],
      paused: false,
      emptyReason: "",
      loading: false,
      error: null,
      refetch: jest.fn(),
    });
    (useMarketingSettings as jest.Mock).mockReturnValue({
      settings: {
        enabled: true,
        disabled_plays: [],
        creative_profile: { audience: "tourists", avoid_phrases: [] },
      },
      hasSnapshot: true,
      loading: false,
      saving: true,
      loadError: null,
      saveError: null,
      rollbackOccurred: false,
      saved: false,
      save: jest.fn(),
      retryLoad: jest.fn(),
      retrySave: jest.fn(),
    });

    render(<MarketingDashboard business={business} />);

    expect(useSuggestionCaption).toHaveBeenCalledWith(
      expect.objectContaining({
        creativeProfile: expect.objectContaining({ audience: "tourists" }),
        enabled: false,
      }),
    );
  });
});
