/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { PostCard } from "./PostCard";
import type { CampaignSuggestion } from "@/api/marketing";
import type { Business } from "@/api/business";
import type { CaptionResource } from "./hooks/useSuggestionCaptions";

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn() },
}));
const mockPostPreview = jest.fn();
/** Override per-test via `.state`: "ready" (default) | "failed" | "rendering". */
const mockPreviewControls = {
  state: "ready" as "ready" | "failed" | "rendering",
};
jest.mock("./PostPreview", () => {
  const ReactRuntime = jest.requireActual("react") as typeof React;
  return {
    PostPreview: (props: {
      onRenderStateChange?: (state: string) => void;
    }) => {
      mockPostPreview(props);
      // useLayoutEffect so isReady is true before tests click export actions.
      ReactRuntime.useLayoutEffect(() => {
        props.onRenderStateChange?.(mockPreviewControls.state);
      }, [props.onRenderStateChange]);
      return <div data-testid="post-preview" />;
    },
  };
});
jest.mock("./postContent", () => ({
  ...jest.requireActual("./postContent"),
  downloadPostPack: jest.fn(() => Promise.resolve()),
  shareOrDownloadPostPack: jest.fn(() => Promise.resolve("downloaded")),
  copyCaption: jest.fn(() => Promise.resolve()),
}));

const business = {
  id: 1,
  name: "Trattoria",
  social_media: "trattoria",
  default_currency: "USD",
  default_language: "en",
  design_settings: { font_family: "Serif" },
} as unknown as Business;
const t = (k: string) => k;

const baseSuggestion = {
  id: "a",
  play: "featured_dish",
  title: "Feature your Carbonara",
  why_data: "Top seller",
  source: "menu_engineering",
  copy_angle: "x",
  rank: 60,
  target_name: "Carbonara",
  metrics: { price: 12.99 },
} as unknown as CampaignSuggestion;

const captionResource = (
  overrides: Partial<CaptionResource> = {},
): CaptionResource => ({
  caption: "Discover Carbonara at Trattoria.",
  source: "fallback",
  tone: "warm",
  generating: false,
  error: null,
  retry: jest.fn(),
  selectTone: jest.fn(),
  observeRef: jest.fn(),
  canGenerate: true,
  ...overrides,
});

beforeEach(() => {
  jest.clearAllMocks();
  mockPreviewControls.state = "ready";
});

it("passes the exact same render input, including font and crop, to preview and export", async () => {
  const { downloadPostPack } = jest.requireMock("./postContent") as {
    downloadPostPack: jest.Mock;
  };
  const suggestion = {
    ...baseSuggestion,
    image_url: "https://cdn/c.jpg",
    image_source: "menu",
  } as CampaignSuggestion;

  render(
    <PostCard
      suggestion={suggestion}
      business={business}
      t={t}
      onTweak={() => {}}
      captionResource={captionResource()}
    />,
  );

  const previewProps = mockPostPreview.mock.calls.at(-1)?.[0] as {
    renderInput?: { palette: { fontFamily?: string }; crop?: unknown };
  };
  expect(previewProps.renderInput?.palette.fontFamily).toBe("Serif");
  // Crop is intentionally ABSENT so the renderer crops around the analysis
  // focal point. The recorded snapshot still carries DEFAULT_CROP.
  expect(previewProps.renderInput?.crop).toBeUndefined();
  expect(previewProps).toEqual(
    expect.objectContaining({
      previewErrorLabel: "preview.renderError",
      previewRetryLabel: "preview.retry",
    }),
  );

  await openMoreActions();
  fireEvent.click(screen.getByText("card.downloadPack"));
  await waitFor(() => expect(downloadPostPack).toHaveBeenCalledTimes(1));
  const exportArgs = downloadPostPack.mock.calls[0][0] as {
    renderInput?: unknown;
  };
  expect(exportArgs.renderInput).toBe(previewProps.renderInput);
});

it("shows caption preview and download pack when the card has a photo", () => {
  const s = {
    ...baseSuggestion,
    image_url: "https://cdn/c.jpg",
    image_source: "menu",
  } as CampaignSuggestion;
  render(
    <PostCard
      suggestion={s}
      business={business}
      t={t}
      onTweak={() => {}}
      captionResource={captionResource({
        caption: "Fresh pasta tonight! #carbonara",
        source: "ai",
      })}
    />,
  );
  expect(
    screen.getByText("Fresh pasta tonight! #carbonara"),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: "card.reviewExport" }),
  ).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: /card\.sharePost/ }),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: /card\.downloadPack/ }),
  ).not.toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: /card\.moreActions/ }),
  ).toBeInTheDocument();
});

async function openMoreActions() {
  fireEvent.click(screen.getByTestId("post-card-more-actions"));
  await waitFor(() => {
    expect(screen.getByText("card.copyCaption")).toBeInTheDocument();
  });
}

it("uses Review and export as the sole primary action and keeps advanced controls editor-only", () => {
  const onTweak = jest.fn();
  render(
    <PostCard
      suggestion={
        {
          ...baseSuggestion,
          image_url: "https://cdn/c.jpg",
          image_source: "menu",
        } as CampaignSuggestion
      }
      business={business}
      t={t}
      onTweak={onTweak}
      captionResource={captionResource()}
    />,
  );

  fireEvent.click(screen.getByRole("button", { name: "card.reviewExport" }));
  expect(onTweak).toHaveBeenCalledWith(expect.objectContaining({ id: "a" }));
  expect(
    screen.queryByRole("button", { name: /card\.sharePost/ }),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: /card\.downloadPack/ }),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByRole("group", { name: "card.styleLabel" }),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByRole("group", { name: "card.formatLabel" }),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "tones.punchy" }),
  ).not.toBeInTheDocument();
});

it.each([
  ["share", "card.sharePost", "downloaded"],
  ["download", "card.downloadPack", "downloaded"],
  ["copy", "card.copyCaption", "copied"],
] as const)(
  "%s handoff reports exact creative without recording activity",
  async (_action, label, mode) => {
    const onHandoff = jest.fn();
    const onMarkPosted = jest.fn();
    render(
      <PostCard
        suggestion={
          {
            ...baseSuggestion,
            image_url: "https://cdn/c.jpg",
            image_source: "menu",
          } as CampaignSuggestion
        }
        business={business}
        t={t}
        onTweak={() => {}}
        onHandoff={onHandoff}
        onMarkPosted={onMarkPosted}
        captionResource={captionResource({ caption: "Exact approved caption" })}
      />,
    );

    await openMoreActions();
    fireEvent.click(screen.getByText(label));
    await waitFor(() => expect(onHandoff).toHaveBeenCalledTimes(1));
    expect(onMarkPosted).not.toHaveBeenCalled();
    expect(onHandoff).toHaveBeenCalledWith(
      expect.objectContaining({
        mode,
        suggestion: expect.objectContaining({ id: "a" }),
        creative: expect.objectContaining({
          caption: "Exact approved caption",
          image_url: "https://cdn/c.jpg",
          image_source: "menu",
          template: "editorial",
          kit: "editorial",
          aspect: "4:5",
          crop: { x: 0.5, y: 0.5, zoom: 1 },
          slots: expect.objectContaining({ dishName: "Carbonara" }),
        }),
      }),
    );
  },
);

it("shows an add-photo action instead of download when there is no photo", () => {
  const s = {
    ...baseSuggestion,
    play: "happy_hour",
    title: "Fill your slow window",
  } as CampaignSuggestion;
  render(
    <PostCard suggestion={s} business={business} t={t} onTweak={() => {}} />,
  );
  expect(screen.getByText("sourceTag.none")).toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: /card\.addPhoto/ }),
  ).toBeInTheDocument();
});

it("surfaces photo load failure with fix affordance — never a silent empty ready card", async () => {
  mockPreviewControls.state = "failed";
  const onTweak = jest.fn();
  const s = {
    ...baseSuggestion,
    image_url: "https://cdn/broken.jpg",
    image_source: "menu",
  } as CampaignSuggestion;
  render(
    <PostCard
      suggestion={s}
      business={business}
      t={t}
      onTweak={onTweak}
      captionResource={captionResource()}
    />,
  );
  await waitFor(() => {
    expect(screen.getByTestId("card-photo-failed")).toBeInTheDocument();
  });
  expect(screen.getByText("card.photoLoadFailed")).toBeInTheDocument();
  expect(screen.queryByText("card.readyBadge")).not.toBeInTheDocument();
  // Primary CTA is fix/regenerate path into the editor, not export.
  const fixButtons = screen.getAllByRole("button", { name: /card\.fixPhoto/ });
  expect(fixButtons.length).toBeGreaterThanOrEqual(1);
  fireEvent.click(fixButtons[0]);
  expect(onTweak).toHaveBeenCalledWith(s);
  expect(
    screen.queryByRole("button", { name: /card\.downloadPack/ }),
  ).not.toBeInTheDocument();
});

it("never shows photo_ready as ready when the preview failed", async () => {
  mockPreviewControls.state = "failed";
  const s = {
    ...baseSuggestion,
    image_url: "https://cdn/broken.jpg",
    image_source: "bundle",
    why_data: "",
    why_factors: [
      { key: "bundle_price", value: "68" },
      { key: "photo_ready", value: "1" },
    ],
  } as CampaignSuggestion;
  render(
    <PostCard
      suggestion={s}
      business={business}
      t={t}
      onTweak={() => {}}
      captionResource={captionResource()}
    />,
  );
  await waitFor(() => {
    expect(screen.getByTestId("card-photo-failed")).toBeInTheDocument();
  });
  expect(screen.getByText("why.factors.bundle_price")).toBeInTheDocument();
  expect(screen.queryByText("why.factors.photo_ready")).not.toBeInTheDocument();
  expect(screen.queryByText("card.readyBadge")).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: /card\.downloadPack/ }),
  ).not.toBeInTheDocument();
});

it("renders an Example badge and a single make-it-yours action for example cards", () => {
  const onTweak = jest.fn();
  const s = {
    id: "ex",
    play: "offer",
    title: "Turn an offer into a post",
    why_data: "demo",
    source: "example",
    target_name: "Weekend Brunch",
    copy_angle: "punchy",
    rank: 1,
    image_url: "/marketing-examples/example-offer.jpg",
    image_source: "offer",
    discount_type: "percentage",
    discount_value: 20,
  } as unknown as CampaignSuggestion;
  render(
    <PostCard
      suggestion={s}
      business={business}
      t={t}
      onTweak={onTweak}
      isExample
    />,
  );
  expect(screen.getByText("example.badge")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: /example\.cta/ }));
  expect(onTweak).toHaveBeenCalledWith(s);
});

describe("why-this-post explainability", () => {
  it("renders structured why_factors with i18n labels (prefer over why_data)", () => {
    // L4-22: the wire carries bare numbers and the FE owns the unit word, so
    // `qty_sold` renders through `why.factors.count_value`. Money keys like
    // `margin_per_unit` still pass through as-is.
    const countAware = (key: string, params?: Record<string, string | number>) =>
      key === "why.factors.count_value" ? `${params?.count} orders` : key;
    const s = {
      ...baseSuggestion,
      why_data: "Legacy English monologue",
      why_factors: [
        { key: "qty_sold", value: "18" },
        { key: "margin_per_unit", value: "6.5" },
      ],
    } as CampaignSuggestion;
    render(
      <PostCard
        suggestion={s}
        business={business}
        t={countAware}
        captionResource={captionResource()}
      />,
    );
    const block = screen.getByTestId("why-this-post");
    expect(block).toHaveAttribute("data-why-mode", "factors");
    expect(screen.getByText("why.heading")).toBeInTheDocument();
    expect(screen.getByTestId("why-factors")).toBeInTheDocument();
    expect(screen.getByText("why.factors.qty_sold")).toBeInTheDocument();
    expect(screen.getByText("18 orders")).toBeInTheDocument();
    expect(screen.getByText("why.factors.margin_per_unit")).toBeInTheDocument();
    expect(screen.getByText("6.5")).toBeInTheDocument();
    // Structured path must not also paint legacy prose as the body.
    expect(screen.queryByTestId("why-legacy")).not.toBeInTheDocument();
    expect(
      screen.queryByText("Legacy English monologue"),
    ).not.toBeInTheDocument();
  });

  it("falls back to why_data when factors are absent", () => {
    render(
      <PostCard
        suggestion={baseSuggestion}
        business={business}
        t={t}
        captionResource={captionResource()}
      />,
    );
    const block = screen.getByTestId("why-this-post");
    expect(block).toHaveAttribute("data-why-mode", "legacy");
    expect(screen.getByTestId("why-legacy")).toHaveTextContent("Top seller");
    expect(screen.queryByTestId("why-factors")).not.toBeInTheDocument();
  });

  it("hides the why block when reason is missing", () => {
    const s = {
      ...baseSuggestion,
      why_data: "",
      why_factors: undefined,
    } as CampaignSuggestion;
    render(
      <PostCard
        suggestion={s}
        business={business}
        t={t}
        captionResource={captionResource()}
      />,
    );
    expect(screen.queryByTestId("why-this-post")).not.toBeInTheDocument();
  });

  it("expands collapsed factor list beyond the first two rows", () => {
    const s = {
      ...baseSuggestion,
      why_factors: [
        { key: "qty_sold", value: "10 orders" },
        { key: "margin_per_unit", value: "4" },
        { key: "photo_ready", value: "Has a photo ready to post" },
      ],
    } as CampaignSuggestion;
    render(
      <PostCard
        suggestion={s}
        business={business}
        t={t}
        captionResource={captionResource()}
      />,
    );
    expect(screen.getByText("why.factors.qty_sold")).toBeInTheDocument();
    expect(screen.getByText("why.factors.margin_per_unit")).toBeInTheDocument();
    expect(
      screen.queryByText("why.factors.photo_ready"),
    ).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /why\.expand/ }));
    expect(screen.getByText("why.factors.photo_ready")).toBeInTheDocument();
    expect(
      screen.getByText("Has a photo ready to post"),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /why\.collapse/ }));
    expect(
      screen.queryByText("why.factors.photo_ready"),
    ).not.toBeInTheDocument();
  });
});

it("surfaces an inline error when the share pack rejects", async () => {
  const { shareOrDownloadPostPack } = jest.requireMock("./postContent") as {
    shareOrDownloadPostPack: jest.Mock;
  };
  shareOrDownloadPostPack.mockRejectedValueOnce(new Error("tainted"));
  const s = {
    ...baseSuggestion,
    image_url: "https://cdn/c.jpg",
    image_source: "menu",
  } as CampaignSuggestion;
  render(
    <PostCard
      suggestion={s}
      business={business}
      t={t}
      onTweak={() => {}}
      captionResource={captionResource({
        caption: "Ready to post",
        source: "ai",
      })}
    />,
  );
  await openMoreActions();
  fireEvent.click(screen.getByText("card.sharePost"));
  expect(await screen.findByText("errors.download_failed")).toBeInTheDocument();
});

it("keeps the localized fallback visible but disables ready actions while an AI caption is generating", async () => {
  const s = {
    ...baseSuggestion,
    image_url: "https://cdn/c.jpg",
  } as CampaignSuggestion;
  render(
    <PostCard
      suggestion={s}
      business={business}
      t={t}
      onTweak={() => {}}
      captionResource={captionResource({ generating: true })}
    />,
  );

  expect(
    screen.getByText("Discover Carbonara at Trattoria."),
  ).toBeInTheDocument();
  expect(screen.getByText("card.captionLoading")).toBeInTheDocument();
  expect(screen.queryByText("card.readyBadge")).not.toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: "card.reviewExport" }),
  ).toBeInTheDocument();
  await openMoreActions();
  expect(document.querySelector('[data-key="share"]')).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  expect(document.querySelector('[data-key="download"]')).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  expect(
    screen.queryByRole("button", { name: "card.markPosted" }),
  ).not.toBeInTheDocument();
});

it("disabled Share/Download carry title + aria-label explaining why (not NONE)", async () => {
  const s = {
    ...baseSuggestion,
    image_url: "https://cdn/c.jpg",
  } as CampaignSuggestion;
  render(
    <PostCard
      suggestion={s}
      business={business}
      t={t}
      onTweak={() => {}}
      captionResource={captionResource({ generating: true })}
    />,
  );

  await openMoreActions();
  const share = document.querySelector('[data-key="share"]');
  const download = document.querySelector('[data-key="download"]');
  expect(share).toHaveAttribute("aria-disabled", "true");
  expect(download).toHaveAttribute("aria-disabled", "true");

  // title must explain the blocker; bare empty/missing titles fail a11y audits.
  expect(share?.getAttribute("title")).toBe("card.blocked.caption_loading");
  expect(download?.getAttribute("title")).toBe("card.blocked.caption_loading");
  expect(share?.getAttribute("aria-label")).toMatch(
    /card\.blocked\.caption_loading/,
  );
  expect(download?.getAttribute("aria-label")).toMatch(
    /card\.blocked\.caption_loading/,
  );
  expect(share?.getAttribute("title")).not.toBe("NONE");
  expect(share?.getAttribute("aria-label")).not.toBe("NONE");
});

it("wraps headlines with text-balance + word-boundary clamp (no mid-word truncate)", () => {
  const longTitle =
    "Feature your Supercalifragilisticexpialidocious-Truffle-Carbonara-with-seasonal-garnish";
  const s = {
    ...baseSuggestion,
    play_key: "featured_dish",
    title: longTitle,
    target_name:
      "Supercalifragilisticexpialidocious Truffle Carbonara with seasonal garnish",
  } as CampaignSuggestion;
  render(
    <PostCard
      suggestion={s}
      business={business}
      t={(key, params) =>
        key === "playTitles.featured_dish"
          ? `Feature your star: ${params?.name ?? ""}`
          : key
      }
      captionResource={captionResource()}
    />,
  );

  const heading = screen.getByRole("heading", { level: 3 });
  expect(heading.className).toMatch(/line-clamp-2/);
  expect(heading.className).toMatch(/text-balance|\[text-wrap:balance\]/);
  expect(heading.className).toMatch(/break-words/);
  // Must not use single-line truncate that clips mid-word.
  expect(heading.className).not.toMatch(/(?:^|\s)truncate(?:\s|$)/);
});

it("preserves the fallback and exposes a visible per-card retry after AI failure", async () => {
  const retry = jest.fn();
  const s = {
    ...baseSuggestion,
    image_url: "https://cdn/c.jpg",
  } as CampaignSuggestion;
  render(
    <PostCard
      suggestion={s}
      business={business}
      t={t}
      onTweak={() => {}}
      captionResource={captionResource({ error: new Error("failed"), retry })}
    />,
  );

  expect(
    screen.getByText("Discover Carbonara at Trattoria."),
  ).toBeInTheDocument();
  expect(screen.getByText("errors.caption_failed")).toBeInTheDocument();
  expect(screen.queryByText("card.readyBadge")).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "card.retryCaption" }));
  expect(retry).toHaveBeenCalledTimes(1);
  await openMoreActions();
  expect(document.querySelector('[data-key="share"]')).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  expect(document.querySelector('[data-key="download"]')).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  expect(
    screen.queryByRole("button", { name: "card.markPosted" }),
  ).not.toBeInTheDocument();
});

it("does not hand off when the card share sheet is dismissed", async () => {
  const { shareOrDownloadPostPack } = jest.requireMock("./postContent") as {
    shareOrDownloadPostPack: jest.Mock;
  };
  shareOrDownloadPostPack.mockRejectedValueOnce(
    new DOMException("cancel", "AbortError"),
  );
  const onHandoff = jest.fn();
  render(
    <PostCard
      suggestion={
        {
          ...baseSuggestion,
          image_url: "https://cdn/c.jpg",
          image_source: "menu",
        } as CampaignSuggestion
      }
      business={business}
      t={t}
      onTweak={() => {}}
      onHandoff={onHandoff}
      captionResource={captionResource()}
    />,
  );

  await openMoreActions();
  fireEvent.click(screen.getByText("card.sharePost"));
  await waitFor(() => expect(shareOrDownloadPostPack).toHaveBeenCalled());
  expect(onHandoff).not.toHaveBeenCalled();
  expect(screen.queryByRole("alert")).not.toBeInTheDocument();
});

it("does not expose alternate tone controls on the queue card", () => {
  const selectTone = jest.fn();
  render(
    <PostCard
      suggestion={baseSuggestion}
      business={business}
      t={t}
      onTweak={() => {}}
      captionResource={captionResource({ source: "ai", selectTone })}
    />,
  );

  expect(
    screen.queryByRole("button", { name: "tones.punchy" }),
  ).not.toBeInTheDocument();
  expect(selectTone).not.toHaveBeenCalled();
});

it("hides all mutating actions for a read-only operator", () => {
  render(
    <PostCard
      suggestion={
        {
          ...baseSuggestion,
          image_url: "https://cdn/c.jpg",
        } as CampaignSuggestion
      }
      business={business}
      t={t}
      onTweak={() => {}}
      onDismiss={jest.fn()}
      onMarkPosted={jest.fn()}
      canEdit={false}
      captionResource={captionResource()}
    />,
  );
  expect(
    screen.queryByRole("button", { name: "card.reviewExport" }),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "card.dismiss" }),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: "card.moreActions" }),
  ).not.toBeInTheDocument();
});

function capturedRenderInput(suggestion: CampaignSuggestion) {
  render(
    <PostCard
      suggestion={suggestion}
      business={business}
      t={t}
      onTweak={() => {}}
      captionResource={captionResource()}
    />,
  );
  const previewProps = mockPostPreview.mock.calls.at(-1)?.[0] as {
    renderInput?: Record<string, unknown>;
  };
  return previewProps.renderInput ?? {};
}

describe("feed card art direction", () => {
  it("renders through the kit path, letting the analysis choose the layout", () => {
    const input = capturedRenderInput({
      ...baseSuggestion,
      play: "win_back",
    } as CampaignSuggestion);
    expect(input.kit).toBe("minimal");
    expect(input.composition).toBeUndefined();
    expect(input.template).toBeUndefined();
  });

  it("records the kit it rendered with, and no composition", async () => {
    const creative = await (async () => {
      const onHandoff = jest.fn();
      render(
        <PostCard
          suggestion={
            {
              ...baseSuggestion,
              play: "offer",
              image_url: "https://cdn/c.jpg",
              image_source: "menu",
              discount_type: "percentage",
              discount_value: 20,
            } as CampaignSuggestion
          }
          business={business}
          t={t}
          onTweak={() => {}}
          onHandoff={onHandoff}
          captionResource={captionResource({
            caption: "Exact approved caption",
          })}
        />,
      );
      await openMoreActions();
      fireEvent.click(screen.getByText("card.downloadPack"));
      await waitFor(() => expect(onHandoff).toHaveBeenCalledTimes(1));
      return onHandoff.mock.calls[0][0].creative as Record<string, unknown>;
    })();
    expect(creative.kit).toBe("bold");
    // Absent means "derive", on the wire and in seedArtDirection alike — so the
    // Reuse editor re-derives the SAME layout from the SAME cached analysis
    // rather than being pinned to one chosen before the photo was measured.
    expect(creative.composition).toBeUndefined();
  });
});

it("marks externally published content with the exact current creative", async () => {
  const onMarkPosted = jest.fn();
  render(
    <PostCard
      suggestion={
        {
          ...baseSuggestion,
          image_url: "https://cdn/c.jpg",
          image_source: "menu",
        } as CampaignSuggestion
      }
      business={business}
      t={t}
      onTweak={() => {}}
      onMarkPosted={onMarkPosted}
      captionResource={captionResource({ caption: "Published elsewhere" })}
    />,
  );

  await openMoreActions();
  fireEvent.click(screen.getByText("card.markPosted"));
  expect(onMarkPosted).toHaveBeenCalledWith({
    suggestion: expect.objectContaining({ id: "a" }),
    creative: expect.objectContaining({
      caption: "Published elsewhere",
      image_url: "https://cdn/c.jpg",
      image_source: "menu",
      template: "editorial",
      kit: "editorial",
      aspect: "4:5",
      crop: { x: 0.5, y: 0.5, zoom: 1 },
    }),
  });
});

it("announces the caption disclosure state and controls the stable caption element", () => {
  render(
    <PostCard
      suggestion={baseSuggestion}
      business={business}
      t={t}
      onTweak={() => {}}
      captionResource={captionResource({ caption: "A".repeat(160) })}
    />,
  );

  const button = screen.getByRole("button", { name: "card.expandCaption" });
  const controlledId = button.getAttribute("aria-controls");
  expect(button).toHaveAttribute("aria-expanded", "false");
  expect(controlledId).toBeTruthy();
  expect(document.getElementById(controlledId!)).toHaveTextContent(
    "A".repeat(160),
  );

  fireEvent.click(button);
  expect(
    screen.getByRole("button", { name: "card.collapseCaption" }),
  ).toHaveAttribute("aria-expanded", "true");
});

import { TEMPLATES, TEMPLATE_ORDER } from "./templates/templates";
import { PLATFORM_CONTENT_BOUNDS } from "./templates/types";
import type { AspectRatio } from "./templates/types";

describe("preview frame content bounds", () => {
  // PostCard and PostEditorDrawer both pass `PLATFORM_CONTENT_BOUNDS[aspect]`
  // to PlatformPreviewFrame instead of reading it off `renderInput.template`,
  // which is optional now and absent entirely on a kit-path post. That is only
  // a safe substitution while the two are the same value, so pin it here: give
  // any template its own contentBounds and both preview frames start lying
  // about the safe area while still typechecking.
  it.each(TEMPLATE_ORDER)(
    "template %s uses the platform content bounds verbatim at every aspect",
    (style) => {
      const aspects: AspectRatio[] = ["1:1", "4:5", "9:16"];
      for (const aspect of aspects) {
        expect(TEMPLATES[style].layouts[aspect].contentBounds).toEqual(
          PLATFORM_CONTENT_BOUNDS[aspect],
        );
      }
    },
  );
});

import { localizedSuggestionTitle } from "./PostCard";

describe("localizedSuggestionTitle", () => {
  const t = (key: string, params?: Record<string, string | number>) => {
    if (key === "playTitles.featured_dish")
      return `Feature your star: ${params?.name}`;
    if (key === "playTitles.happy_hour")
      return `Fill ${params?.daypart} with a happy hour`;
    if (key === "playTitles.happy_hour_generic")
      return "Fill your slowest window";
    return key;
  };

  it("uses play_key + name for featured_dish", () => {
    expect(
      localizedSuggestionTitle(
        {
          ...baseSuggestion,
          play_key: "featured_dish",
          target_name: "Carbonara",
        } as CampaignSuggestion,
        t,
      ),
    ).toBe("Feature your star: Carbonara");
  });

  it("interpolates daypart_key for happy_hour", () => {
    expect(
      localizedSuggestionTitle(
        {
          id: "h",
          play: "happy_hour",
          play_key: "happy_hour",
          daypart_key: "Tue 5–7pm",
          title: "Fill Tue 5–7pm with a happy hour",
          why_data: "x",
          source: "slow_dayparts",
          copy_angle: "x",
          rank: 1,
        } as CampaignSuggestion,
        t,
      ),
    ).toBe("Fill Tue 5–7pm with a happy hour");
  });
});
