/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type {
  MarketingActivity,
  MarketingCreativeSnapshot,
} from "@/api/marketing";
import { MarketingLibrary } from "./MarketingLibrary";
import * as activityHook from "./hooks/useMarketingActivity";

jest.mock("./hooks/useMarketingActivity");
jest.mock("./PostPreview", () => ({
  LIBRARY_THUMBNAIL_WIDTH: 256,
  PostPreview: ({
    renderInput,
    lazy,
    targetWidth,
    cacheKey,
  }: {
    renderInput: Record<string, any>;
    lazy?: boolean;
    targetWidth?: number;
    cacheKey?: string;
  }) => (
    <div
      data-testid="library-preview"
      data-aspect={renderInput.aspect}
      data-template={renderInput.template.id}
      data-photo={renderInput.photoUrl}
      data-slots={JSON.stringify(renderInput.slots)}
      data-crop={JSON.stringify(renderInput.crop)}
      data-font={renderInput.palette.fontFamily}
      data-lazy={lazy ? "true" : "false"}
      data-target-width={targetWidth != null ? String(targetWidth) : ""}
      data-cache-key={cacheKey ?? ""}
    />
  ),
}));

// DatePicker from NextUI is heavy in unit tests; stub a labeled text control
// that still exercises the from/to → hook wiring.
jest.mock("@nextui-org/react", () => {
  const actual = jest.requireActual("@nextui-org/react");
  return {
    ...actual,
    DatePicker: ({
      "aria-label": ariaLabel,
      value,
      onChange,
    }: {
      "aria-label"?: string;
      value?: { toString: () => string } | null;
      onChange?: (value: { toString: () => string } | null) => void;
    }) => (
      <input
        aria-label={ariaLabel}
        value={value?.toString() ?? ""}
        onChange={(event) => {
          const next = event.target.value;
          onChange?.(
            next
              ? {
                  toString: () => next,
                }
              : null,
          );
        }}
      />
    ),
  };
});

const t = (key: string, params?: Record<string, string | number>) =>
  params && "count" in params ? `${key}:${params.count}` : key;

const business = {
  id: 42,
  name: "Trattoria",
  default_language: "en",
  design_settings: {
    primary_color: "#123456",
    secondary_color: "#654321",
    font_family: "Sans",
  },
} as any;

const exactCreative: MarketingCreativeSnapshot = {
  image_url: "https://cdn/exact.jpg",
  image_source: "generated",
  caption: "The caption that was actually posted",
  aspect: "9:16",
  template: "bold",
  crop: { x: 0.21, y: 0.73, zoom: 1.8 },
  slots: {
    dishName: "Exact carbonara",
    price: "$24",
    badge: "TONIGHT",
    cta: "BOOK NOW",
    handle: "@trattoria",
  },
  font_family: "Serif",
};

function activity(
  overrides: Partial<MarketingActivity> = {},
): MarketingActivity {
  return {
    id: 1,
    business_id: 42,
    suggestion_id: "1:offer:o1",
    play: "offer",
    title: "Offer post",
    status: "dismissed",
    image_url: "",
    caption: "",
    creative_snapshot: exactCreative,
    created_by: "owner",
    created_at: "2026-07-01T00:00:00Z",
    updated_at: "2026-07-01T00:00:00Z",
    target_name: "Brunch",
    ...overrides,
  };
}

const restoreActivity = jest.fn();
const duplicateLibraryRestore = jest.fn();

beforeEach(() => {
  jest.clearAllMocks();
  (activityHook.useMarketingActivityMutations as jest.Mock).mockReturnValue({
    restore: { mutate: duplicateLibraryRestore, isPending: false },
  });
});

function mockList(
  overrides: Partial<ReturnType<typeof activityHook.useMarketingActivity>> = {},
) {
  (activityHook.useMarketingActivity as jest.Mock).mockReturnValue({
    items: [activity()],
    total: 1,
    loading: false,
    loadingMore: false,
    error: null,
    hasMore: false,
    loadMore: jest.fn(),
    refetch: jest.fn(),
    ...overrides,
  });
}

function renderLibrary(
  overrides: Partial<React.ComponentProps<typeof MarketingLibrary>> = {},
) {
  return render(
    <MarketingLibrary
      business={business}
      businessId={42}
      canEdit
      onRestore={restoreActivity}
      t={t}
      {...overrides}
    />,
  );
}

it("shows a dedicated initial loading state", () => {
  mockList({ items: [], total: 0, loading: true });

  renderLibrary();

  expect(screen.getByRole("status")).toHaveTextContent("library.loading");
  expect(screen.queryByText("library.empty")).not.toBeInTheDocument();
});

it("shows a retryable failure without calling an empty response activity", () => {
  const refetch = jest.fn();
  mockList({ items: [], total: 0, error: "network_failed", refetch });

  renderLibrary();
  expect(screen.getByRole("alert")).toHaveTextContent("library.loadFailed");

  fireEvent.click(screen.getByRole("button", { name: "library.retry" }));
  expect(refetch).toHaveBeenCalledTimes(1);
  expect(screen.queryByText("library.empty")).not.toBeInTheDocument();
});

it("distinguishes a genuinely empty Library from a filtered empty result", () => {
  mockList({ items: [], total: 0 });
  renderLibrary();
  expect(screen.getByText("library.empty")).toBeInTheDocument();

  fireEvent.click(screen.getByRole("button", { name: "library.filterPosted" }));
  expect(screen.getByText("library.filteredEmpty")).toBeInTheDocument();
});

it("lists inventory-hidden ideas in Historial Ocultos without a snapshot", () => {
  mockList({
    items: [
      activity({
        id: 9,
        suggestion_id: "86:featured_dish:demo-steak",
        play: "featured_dish",
        title: "Feature your star: Steak Plate",
        target_name: "Steak Plate",
        status: "dismissed",
        created_by: "inventory",
        creative_snapshot: null,
        caption: "",
        image_url: "",
      }),
    ],
    total: 1,
  });
  renderLibrary();
  fireEvent.click(screen.getByRole("button", { name: "library.filterDismissed" }));
  expect(screen.getByText("Feature your star: Steak Plate")).toBeInTheDocument();
  expect(screen.getAllByText(/Steak Plate/).length).toBeGreaterThan(0);
  expect(screen.queryByText("library.filteredEmpty")).not.toBeInTheDocument();
  expect(screen.getByRole("button", { name: "library.restore" })).toBeInTheDocument();
});

it("switching the status filter re-invokes the hook with the new status", () => {
  mockList();
  renderLibrary();
  fireEvent.click(screen.getByRole("button", { name: "library.filterPosted" }));
  const lastCall = (
    activityHook.useMarketingActivity as jest.Mock
  ).mock.calls.at(-1);
  expect(lastCall?.[1]).toEqual(expect.objectContaining({ status: "posted" }));
});

function playFilterTrigger(): HTMLElement {
  // NextUI Select renders a button trigger (aria-haspopup="listbox") over a
  // hidden native <select>; the visible control the operator interacts with is
  // the trigger, not a bare native select.
  const trigger = screen
    .getAllByLabelText("library.filterPlay")
    .find((el) => el.getAttribute("aria-haspopup") === "listbox");
  if (!trigger) throw new Error("play filter trigger not found");
  return trigger;
}

it("offers the play filter as an accessible NextUI Select trigger", () => {
  mockList();
  renderLibrary();

  const trigger = playFilterTrigger();
  expect(trigger.tagName).toBe("BUTTON");
  expect(trigger).toHaveAttribute("aria-haspopup", "listbox");
});

it("selecting a play from the listbox re-invokes the hook with that play", async () => {
  const user = userEvent.setup();
  mockList();
  renderLibrary();

  await user.click(playFilterTrigger());
  const listbox = await screen.findByRole("listbox");
  await user.click(within(listbox).getByRole("option", { name: "plays.offer" }));

  await waitFor(() => {
    const lastCall = (
      activityHook.useMarketingActivity as jest.Mock
    ).mock.calls.at(-1);
    expect(lastCall?.[1]).toEqual(expect.objectContaining({ play: "offer" }));
  });
});

it("wires date-range controls to from/to activity filters", () => {
  mockList();
  renderLibrary();

  // Initial call has empty date filters
  expect(activityHook.useMarketingActivity).toHaveBeenCalledWith(
    42,
    expect.objectContaining({ from: "", to: "" }),
  );

  const fromInput = screen.getByLabelText("library.filterFrom");
  const toInput = screen.getByLabelText("library.filterTo");
  fireEvent.change(fromInput, { target: { value: "2026-01-01" } });
  fireEvent.change(toInput, { target: { value: "2026-01-31" } });

  const lastCall = (
    activityHook.useMarketingActivity as jest.Mock
  ).mock.calls.at(-1);
  expect(lastCall?.[1]).toEqual(
    expect.objectContaining({ from: "2026-01-01", to: "2026-01-31" }),
  );
});

it("shows loading-more progress and prevents duplicate pagination requests", () => {
  const loadMore = jest.fn();
  mockList({ hasMore: true, loadMore, loadingMore: true });
  renderLibrary();

  const button = screen.getByRole("button", { name: "library.loadingMore" });
  expect(button).toBeDisabled();
  fireEvent.click(button);
  expect(loadMore).not.toHaveBeenCalled();
});

it("labels ready activity rows as ready — never as dismissed/hidden", () => {
  mockList({
    items: [
      activity({
        id: 9,
        status: "ready",
        title: "Owner review queue item",
      }),
    ],
  });
  renderLibrary();

  expect(screen.getAllByText("library.statusReady").length).toBeGreaterThan(0);
  expect(screen.queryByText("library.statusDismissed")).not.toBeInTheDocument();
  expect(screen.queryByText("library.statusPosted")).not.toBeInTheDocument();
});

it("wraps library titles instead of single-line mid-word truncate", () => {
  mockList({
    items: [
      activity({
        title:
          "Supercalifragilisticexpialidocious weekend brunch featuring seasonal truffle carbonara",
      }),
    ],
  });
  renderLibrary();

  const title = screen.getByText(
    /Supercalifragilisticexpialidocious weekend brunch/,
  );
  expect(title.className).toMatch(/line-clamp-2|break-words|text-balance/);
  expect(title.className).not.toMatch(/(?:^|\s)truncate(?:\s|$)/);
});

it("renders the exact captured snapshot rather than top-level fallback fields", () => {
  mockList({
    items: [
      activity({
        status: "posted",
        image_url: "https://cdn/stale-top-level.jpg",
        caption: "Stale top-level caption",
      }),
    ],
  });

  renderLibrary();

  const preview = screen.getByTestId("library-preview");
  expect(preview).toHaveAttribute("data-aspect", "9:16");
  expect(preview).toHaveAttribute("data-template", "bold");
  expect(preview).toHaveAttribute("data-photo", exactCreative.image_url);
  expect(preview).toHaveAttribute(
    "data-slots",
    JSON.stringify(exactCreative.slots),
  );
  expect(preview).toHaveAttribute(
    "data-crop",
    JSON.stringify(exactCreative.crop),
  );
  expect(preview).toHaveAttribute("data-font", "Serif");
  expect(
    screen.getByText("The caption that was actually posted"),
  ).toBeInTheDocument();
  expect(screen.getByText("library.capturedPreview")).toBeInTheDocument();
  expect(screen.queryByText("Stale top-level caption")).not.toBeInTheDocument();
});

it("uses a lazy thumbnail render path (not full-res) for library rows", () => {
  mockList({ items: [activity({ id: 77, status: "posted" })] });
  renderLibrary();

  const preview = screen.getByTestId("library-preview");
  expect(preview).toHaveAttribute("data-lazy", "true");
  expect(preview).toHaveAttribute("data-target-width", "256");
  expect(preview).toHaveAttribute("data-cache-key", "activity-77");
});

it("Reuse returns every captured field to the local editor callback", () => {
  const onReuse = jest.fn();
  mockList({ items: [activity({ status: "posted" })] });

  renderLibrary({ onReuse });
  fireEvent.click(screen.getByRole("button", { name: "library.reuse" }));

  expect(onReuse).toHaveBeenCalledWith(
    expect.objectContaining({ suggestion_id: "1:offer:o1" }),
    exactCreative,
  );
});

it("reconstructs legacy rows from top-level content and labels defaults honestly", () => {
  const onReuse = jest.fn();
  mockList({
    items: [
      activity({
        status: "posted",
        image_url: "https://cdn/legacy.jpg",
        caption: "Legacy caption",
        creative_snapshot: null,
      }),
    ],
  });

  renderLibrary({ onReuse });

  const preview = screen.getByTestId("library-preview");
  expect(preview).toHaveAttribute("data-photo", "https://cdn/legacy.jpg");
  expect(preview).toHaveAttribute("data-aspect", "4:5");
  expect(preview).toHaveAttribute("data-template", "editorial");
  expect(preview).toHaveAttribute(
    "data-crop",
    JSON.stringify({ x: 0.5, y: 0.5, zoom: 1 }),
  );
  // Reconstructed rows have no kit; wire default is Sans (kit typefaces own faces).
  expect(preview).toHaveAttribute("data-font", "Sans");
  expect(screen.getByText("Legacy caption")).toBeInTheDocument();
  expect(screen.getByText("library.reconstructedPreview")).toBeInTheDocument();
  expect(screen.queryByText("library.capturedPreview")).not.toBeInTheDocument();

  fireEvent.click(screen.getByRole("button", { name: "library.reuse" }));
  expect(onReuse).toHaveBeenCalledWith(
    expect.anything(),
    expect.objectContaining({
      image_url: "https://cdn/legacy.jpg",
      caption: "Legacy caption",
      aspect: "4:5",
      template: "editorial",
      crop: { x: 0.5, y: 0.5, zoom: 1 },
      font_family: "Sans",
    }),
  );
});

it("offers a read-only detail view even when the operator cannot edit", () => {
  mockList({
    items: [
      activity({
        status: "posted",
        creative_snapshot: {
          ...exactCreative,
          caption: "A full caption that was clamped in the list",
        },
      }),
    ],
  });
  renderLibrary({ canEdit: false, onReuse: undefined, onRestore: undefined });

  expect(
    screen.getByRole("button", { name: "library.viewDetail" }),
  ).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "library.reuse" }),
  ).not.toBeInTheDocument();

  fireEvent.click(screen.getByRole("button", { name: "library.viewDetail" }));
  expect(screen.getByLabelText("library.detailTitle")).toBeInTheDocument();
  // List clamps the caption AND the drawer shows the full text.
  expect(
    screen.getAllByText("A full caption that was clamped in the list").length,
  ).toBeGreaterThanOrEqual(2);
  expect(screen.getByText("library.detailCaption")).toBeInTheDocument();
  expect(screen.getByText("library.detailMeta")).toBeInTheDocument();
});

it("restores dismissed activity only when the operator can edit", () => {
  mockList();
  const { rerender } = renderLibrary();
  fireEvent.click(screen.getByRole("button", { name: "library.restore" }));
  expect(restoreActivity).toHaveBeenCalledWith("1:offer:o1");
  expect(activityHook.useMarketingActivityMutations).not.toHaveBeenCalled();

  rerender(
    <MarketingLibrary
      business={business}
      businessId={42}
      canEdit={false}
      onRestore={restoreActivity}
      t={t}
    />,
  );
  expect(
    screen.queryByRole("button", { name: "library.restore" }),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "library.reuse" }),
  ).not.toBeInTheDocument();
});

it("keeps a failed restore visible and retries the same activity", () => {
  mockList();
  renderLibrary({ restoreFailedSuggestionId: "1:offer:o1" });

  expect(screen.getByRole("alert")).toHaveTextContent("library.restoreFailed");
  fireEvent.click(screen.getByRole("button", { name: "library.retryRestore" }));

  expect(restoreActivity).toHaveBeenCalledWith("1:offer:o1");
});

it("keeps future or legacy play rows visible but does not offer an invalid reuse", () => {
  mockList({
    items: [
      activity({
        id: 2,
        play: "unknown",
        raw_play: "seasonal_push",
        title: "Legacy campaign",
        status: "posted",
      }),
    ],
  });

  renderLibrary();

  expect(screen.getByText("library.unknownPlay")).toBeInTheDocument();
  expect(screen.queryByText("plays.unknown")).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "library.reuse" }),
  ).not.toBeInTheDocument();
});

describe("motion rows", () => {
  it("badges a row whose snapshot is a video", async () => {
    mockList({
      items: [
        activity({
          id: 1,
          creative_snapshot: {
            ...exactCreative,
            media_kind: "video",
            motion_preset: "pushIn",
          },
        }),
      ],
    });
    renderLibrary();
    expect(await screen.findByText("motion.libraryBadge")).toBeInTheDocument();
  });

  it("does not badge a still row", async () => {
    mockList({ items: [activity({ id: 2, creative_snapshot: exactCreative })] });
    renderLibrary();
    await screen.findByTestId("library-preview");
    expect(screen.queryByText("motion.libraryBadge")).toBeNull();
  });

  it("does not badge a legacy row with no snapshot at all", async () => {
    mockList({
      items: [activity({ id: 3, creative_snapshot: null })],
    });
    renderLibrary();
    expect(screen.queryByText("motion.libraryBadge")).toBeNull();
  });

  it("does not badge a row whose media_kind is an id this build does not know", async () => {
    mockList({
      items: [
        activity({
          id: 4,
          creative_snapshot: { ...exactCreative, media_kind: "lottie" },
        }),
      ],
    });
    renderLibrary();
    expect(screen.queryByText("motion.libraryBadge")).toBeNull();
  });
});
