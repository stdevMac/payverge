/** @jest-environment jsdom */

import React from "react";
import { FORMATS, FORMAT_ORDER } from "./formats/formats";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import messages from "@/i18n/messages/en/marketingDashboard.json";
import {
  generateMarketingCaption,
  generateMarketingImage,
  ImageDailyLimitError,
} from "@/api/marketing";
import { getBusinessGalleryImages } from "@/api/business";
import { PostEditorDrawer } from "./PostEditorDrawer";
import { KIT_ORDER } from "./artDirection/kits";
import { TEMPLATES } from "./templates/templates";
import {
  resolveArtDirection,
  type RenderPostInput,
} from "./templates/renderPost";

jest.mock("./templates/renderPost", () => {
  const actual = jest.requireActual("./templates/renderPost");
  return {
    ...actual,
    loadOptionalImage: jest.fn().mockResolvedValue({
      naturalWidth: 800,
      naturalHeight: 1000,
      decode: async () => undefined,
    }),
  };
});

jest.mock("./photo/cache", () => ({
  ...jest.requireActual("./photo/cache"),
  photoAnalysisFor: jest.fn().mockReturnValue(null),
}));
import { COMPOSITIONS } from "./composition/compositions";
import {
  copyCaption,
  downloadPostPack,
  downloadPostVideo,
  shareOrDownloadPostPack,
} from "./postContent";
import { detectMotionSupport } from "./motion/capabilities";
import { writeEditorMode } from "./editorMode";
import type { NarrativeKitExportRequest } from "./hooks/useCampaignKitExport";
import toast from "react-hot-toast";

// This suite renders the full NextUI + framer-motion editor drawer tree. Per
// the documented worker-oversubscription note in jest.config.js, when several
// heavy RTL render suites co-schedule on one worker the event loop starves and
// waitFor's polling can cross the 5s wall-clock deadline even though the
// assertions themselves settle quickly. Give this render-heavy suite the same
// headroom used by src/app/admin/users/page.test.tsx rather than masking it
// with a global testTimeout bump.
jest.setTimeout(15000);

jest.mock("@/api/marketing", () => ({
  ...jest.requireActual("@/api/marketing"),
  generateMarketingCaption: jest.fn(),
  generateMarketingImage: jest.fn(),
}));

jest.mock("@/api/business", () => ({
  ...jest.requireActual("@/api/business"),
  getBusinessGalleryImages: jest.fn(),
}));

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

jest.mock("./postContent", () => ({
  ...jest.requireActual("./postContent"),
  copyCaption: jest.fn(),
  downloadPostPack: jest.fn(),
  downloadPostVideo: jest.fn(() => Promise.resolve()),
  shareOrDownloadPostPack: jest.fn(),
}));

const mockExportKit = jest.fn(async () => true);
const mockExportNarrativeKit = jest.fn(
  async (_req: NarrativeKitExportRequest): Promise<boolean> => true,
);
jest.mock("./hooks/useCampaignKitExport", () => ({
  useCampaignKitExport: () => ({
    exportKit: mockExportKit,
    exportNarrativeKit: mockExportNarrativeKit,
    exportPrintShopPack: jest.fn(async () => true),
    progress: null,
    failedFormats: [],
    error: null,
  }),
}));

jest.mock("./motion/capabilities", () => ({
  detectMotionSupport: jest.fn(() => ({
    supported: true,
    path: "webcodecs" as const,
    missing: [],
  })),
}));

/**
 * Force an exact art direction onto the render input.
 *
 * The composer now always emits a kit and never a `TemplateDef`, so the LEGACY
 * branch of `paintedSlotKeys` is the one no prop combination can reach — and
 * a specific composition still cannot be dialled in from outside, since the
 * chooser derives it from the content. Wrap the real hook and override only
 * the art-direction fields of `renderInput`, leaving every other composer
 * behaviour genuine, so both branches are exercised against a real composer
 * rather than a stub.
 */
let mockRenderInputOverride: Partial<RenderPostInput> | null = null;
jest.mock("./hooks/usePostComposer", () => {
  const actual = jest.requireActual("./hooks/usePostComposer");
  return {
    ...actual,
    usePostComposer: (args: unknown) => {
      const state = actual.usePostComposer(args);
      if (!mockRenderInputOverride) return state;
      return {
        ...state,
        renderInput: { ...state.renderInput, ...mockRenderInputOverride },
      };
    },
  };
});

/**
 * The exact object the preview, the download and the share pack all render
 * from. Captured rather than asserted through a canvas because it IS the
 * contract: `resolveArtDirection` in renderPost.ts reads `kit`/`composition`
 * off this input and prefers `template` only in their absence, so a test that
 * never looks at it cannot tell the kit path from the legacy one.
 */
let mockPreviewRenderInput: RenderPostInput | null = null;
jest.mock("./PostPreview", () => ({
  PostPreview: ({
    renderInput,
    onRenderStateChange,
  }: {
    renderInput?: unknown;
    onRenderStateChange?: (state: string) => void;
  }) => {
    mockPreviewRenderInput = (renderInput ?? null) as RenderPostInput | null;
    return (
      <div data-testid="post-preview">
        <button type="button" onClick={() => onRenderStateChange?.("failed")}>
          Fail preview
        </button>
        <button type="button" onClick={() => onRenderStateChange?.("ready")}>
          Ready preview
        </button>
      </div>
    );
  },
}));

jest.mock("../SimpleImageUpload", () => ({
  __esModule: true,
  default: ({
    onImageUploaded,
  }: {
    onImageUploaded: (url: string) => void;
  }) => (
    <button
      type="button"
      onClick={() => onImageUploaded("https://cdn/upload.jpg")}
    >
      Upload test photo
    </button>
  ),
}));

jest.mock("../modals/ConfirmationModal", () => ({
  __esModule: true,
  default: ({
    isOpen,
    title,
    description,
    confirmLabel,
    cancelLabel,
    onConfirm,
    onOpenChange,
  }: any) =>
    isOpen ? (
      <div role="dialog" aria-label={title}>
        <h2>{title}</h2>
        <p>{description}</p>
        <button
          type="button"
          onClick={() => {
            onConfirm();
            onOpenChange();
          }}
        >
          {confirmLabel}
        </button>
        <button type="button" onClick={onOpenChange}>
          {cancelLabel}
        </button>
      </div>
    ) : null,
}));

const mockGenerateImage = generateMarketingImage as jest.MockedFunction<
  typeof generateMarketingImage
>;
const mockGenerateCaption = generateMarketingCaption as jest.MockedFunction<
  typeof generateMarketingCaption
>;
const mockGetGallery = getBusinessGalleryImages as jest.MockedFunction<
  typeof getBusinessGalleryImages
>;
const mockCopyCaption = copyCaption as jest.MockedFunction<typeof copyCaption>;
const mockDownloadPostPack = downloadPostPack as jest.MockedFunction<
  typeof downloadPostPack
>;
const mockDownloadPostVideo = downloadPostVideo as jest.MockedFunction<
  typeof downloadPostVideo
>;
const mockSharePack = shareOrDownloadPostPack as jest.MockedFunction<
  typeof shareOrDownloadPostPack
>;
const mockDetectMotionSupport = detectMotionSupport as jest.MockedFunction<
  typeof detectMotionSupport
>;

const business = {
  id: 42,
  name: "Trattoria",
  default_language: "en",
  display_currency: "USD",
  design_settings: { font_family: "Serif" },
} as any;

const suggestion = {
  id: "1:featured_dish:m1",
  play: "featured_dish",
  title: "Feature Carbonara",
  why_data: "Your bestseller",
  source: "menu_engineering",
  target_name: "Carbonara",
  target_description: "Silky sauce",
  copy_angle: "Celebrate it",
  rank: 100,
  image_url: "https://cdn/menu.jpg",
  image_source: "menu",
} as const;

const t = (key: string, params?: Record<string, string | number>): string => {
  const value = key.split(".").reduce<unknown>((current, part) => {
    if (!current || typeof current !== "object") return undefined;
    return (current as Record<string, unknown>)[part];
  }, messages);
  if (typeof value !== "string") return key;
  return Object.entries(params ?? {}).reduce(
    (copy, [name, replacement]) =>
      copy.replaceAll(`{${name}}`, String(replacement)),
    value,
  );
};

const defaultProps = {
  isOpen: true,
  onClose: jest.fn(),
  business,
  suggestion: suggestion as any,
  canEdit: true,
  canGenerate: true,
  canUpload: true,
  t,
};

function renderEditor(
  overrides: Partial<React.ComponentProps<typeof PostEditorDrawer>> = {},
) {
  return render(<PostEditorDrawer {...defaultProps} {...overrides} />);
}

function EditorTriggerHarness() {
  const [isOpen, setIsOpen] = React.useState(false);
  return (
    <>
      <button type="button" onClick={() => setIsOpen(true)}>
        Open editor
      </button>
      <PostEditorDrawer
        {...defaultProps}
        isOpen={isOpen}
        onClose={() => setIsOpen(false)}
      />
    </>
  );
}

describe("PostEditorDrawer", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    localStorage.clear();
    // Existing craft-surface assertions (kits, motion, campaign kit) seed craft
    // so the suite keeps covering those controls; simple mode has its own cases.
    writeEditorMode(business.id, "craft");
    mockRenderInputOverride = null;
    mockPreviewRenderInput = null;
    mockGetGallery.mockResolvedValue([]);
    mockGenerateCaption.mockReturnValue(new Promise(() => undefined));
    mockGenerateImage.mockResolvedValue({
      url: "https://cdn/generated.jpg",
      credit: "paid",
    });
    mockCopyCaption.mockResolvedValue();
    mockDownloadPostPack.mockResolvedValue();
    mockDownloadPostVideo.mockResolvedValue();
    mockSharePack.mockResolvedValue("shared");
    mockDetectMotionSupport.mockReturnValue({
      supported: true,
      path: "webcodecs",
      missing: [],
    });
    mockExportKit.mockReset();
    mockExportKit.mockResolvedValue(true);
    mockExportNarrativeKit.mockReset();
    mockExportNarrativeKit.mockResolvedValue(true);
    (toast.success as jest.Mock).mockClear();
    (toast.error as jest.Mock).mockClear();
  });

  it("hands off the exact edited creative after editor download, copy, and share", async () => {
    mockGenerateCaption.mockResolvedValue("AI caption");
    const onHandoff = jest.fn();
    renderEditor({ onHandoff } as any);
    await waitFor(() =>
      expect(screen.getByLabelText(t("editor.caption"))).toHaveValue(
        "AI caption",
      ),
    );

    fireEvent.click(
      screen.getByRole("button", { name: t(FORMATS["9:16"].labelKey) }),
    );
    const styles = await screen.findByRole("group", {
      name: t("editor.styleLabel"),
    });
    fireEvent.click(
      within(styles).getByRole("button", { name: t("kits.bold") }),
    );
    fireEvent.change(screen.getByLabelText(t("crop.focalX")), {
      target: { value: "0.25" },
    });
    fireEvent.change(screen.getByLabelText(t("crop.focalY")), {
      target: { value: "0.75" },
    });
    fireEvent.change(screen.getByLabelText(t("crop.zoom")), {
      target: { value: "1.5" },
    });
    fireEvent.change(screen.getByLabelText(t("slots.dishName")), {
      target: { value: "Edited Carbonara" },
    });
    fireEvent.change(screen.getByLabelText(t("editor.caption")), {
      target: { value: "Exact editor caption" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Ready preview" }));

    fireEvent.click(
      screen.getByRole("button", { name: t("editor.downloadPack") }),
    );
    await waitFor(() => expect(onHandoff).toHaveBeenCalledTimes(1));
    expect(onHandoff).toHaveBeenLastCalledWith({
      mode: "downloaded",
      destinationLabel: expect.any(String),
      suggestion: expect.objectContaining({ id: suggestion.id }),
      creative: expect.objectContaining({
        aspect: "9:16",
        // The drawer no longer offers a template picker, so this is the
        // composer's seed value rather than an operator choice. It still
        // travels: it is what seeds the kit when this snapshot is reopened by
        // a build that cannot read the `kit` field.
        template: "editorial",
        crop: { x: 0.25, y: 0.75, zoom: 1.5 },
        slots: expect.objectContaining({ dishName: "Edited Carbonara" }),
        caption: "Exact editor caption",
        image_url: "https://cdn/menu.jpg",
        image_source: "menu",
        // bold kit display face is sans (brandLock.fontFamilyFromKit).
        font_family: "Sans",
        // Without these on the outbound snapshot the art direction is lost the
        // moment the drawer closes, whatever the composer holds in memory.
        kit: "bold",
        composition: "photoBottomStack",
        destination_id: "ig_feed",
      }),
    });

    fireEvent.click(
      screen.getByRole("button", { name: t("editor.copyCaption") }),
    );
    await waitFor(() => expect(onHandoff).toHaveBeenCalledTimes(2));
    expect(onHandoff).toHaveBeenLastCalledWith(
      expect.objectContaining({ mode: "copied" }),
    );

    fireEvent.click(
      screen.getByRole("button", { name: t("editor.sharePost") }),
    );
    await waitFor(() => expect(onHandoff).toHaveBeenCalledTimes(3));
    expect(onHandoff).toHaveBeenLastCalledWith(
      expect.objectContaining({ mode: "shared" }),
    );
  });

  it("exports a reused snapshot through the same exact handoff without persisting a draft", async () => {
    const onHandoff = jest.fn();
    const initialCreative = {
      image_url: "https://cdn/reused.jpg",
      image_source: "upload" as const,
      caption: "Exact archived caption",
      aspect: "1:1" as const,
      template: "minimal" as const,
      crop: { x: 0.11, y: 0.86, zoom: 2.2 },
      slots: {
        dishName: "Archive special",
        price: "$19",
        badge: "REUSE",
        cta: "COME BY",
        handle: "@archive",
      },
      font_family: "Sans" as const,
    };

    renderEditor({ initialCreative, onHandoff });

    expect(screen.getByLabelText(t("editor.caption"))).toHaveValue(
      "Exact archived caption",
    );
    expect(screen.getByLabelText(t("slots.dishName"))).toHaveValue(
      "Archive special",
    );
    expect(screen.getByLabelText(t("crop.focalX"))).toHaveValue("0.11");
    expect(screen.getByLabelText(t("crop.focalY"))).toHaveValue("0.86");
    expect(screen.getByLabelText(t("crop.zoom"))).toHaveValue("2.2");
    expect(mockGenerateCaption).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Ready preview" }));
    fireEvent.click(
      screen.getByRole("button", { name: t("editor.downloadPack") }),
    );

    await waitFor(() =>
      expect(onHandoff).toHaveBeenCalledWith({
        mode: "downloaded",
        destinationLabel: expect.any(String),
        suggestion: expect.objectContaining({ id: suggestion.id }),
        // Byte-for-byte the archived creative, plus the art direction the
        // legacy snapshot never carried: re-exporting a pre-Wave-1 post is how
        // it acquires a kit, derived from its own template rather than guessed.
        // Absent kit_formats on the archive means a single-format post — not a
        // six-format campaign — so re-export preserves that as [hero].
        // Absent destination_id seeds the default destination pack id.
        creative: {
          ...initialCreative,
          kit: "minimal",
          composition: "photoBottomStack",
          kit_formats: ["1:1"],
          media_kind: "image",
          destination_id: "ig_feed",
          // S3-Reach: deterministic promo / correlation code on every handoff.
          promo_code: expect.stringMatching(/^PV-/),
        },
      }),
    );
  });

  it("does not hand off when the editor share sheet is dismissed", async () => {
    const onHandoff = jest.fn();
    mockGenerateCaption.mockResolvedValue("Ready caption");
    mockSharePack.mockRejectedValueOnce(
      new DOMException("cancel", "AbortError"),
    );
    renderEditor({ onHandoff } as any);
    await waitFor(() =>
      expect(screen.getByLabelText(t("editor.caption"))).toHaveValue(
        "Ready caption",
      ),
    );
    fireEvent.click(screen.getByRole("button", { name: "Ready preview" }));
    fireEvent.click(
      screen.getByRole("button", { name: t("editor.sharePost") }),
    );
    await waitFor(() => expect(mockSharePack).toHaveBeenCalledTimes(1));
    expect(onHandoff).not.toHaveBeenCalled();
  });

  it("opens an existing creative clean and prompts only after an actual edit", () => {
    const onClose = jest.fn();
    renderEditor({ onClose });

    fireEvent.click(screen.getByText(t("editor.close")).closest("button")!);
    expect(onClose).toHaveBeenCalledTimes(1);
    expect(
      screen.queryByRole("dialog", { name: t("discard.title") }),
    ).not.toBeInTheDocument();

    fireEvent.change(screen.getByLabelText(t("slots.dishName")), {
      target: { value: "Friday Carbonara" },
    });
    fireEvent.click(screen.getByText(t("editor.close")).closest("button")!);
    expect(
      screen.getByRole("dialog", { name: t("discard.title"), hidden: true }),
    ).toBeInTheDocument();
  });

  it("has an accessible name, focuses on entry, and returns focus after Escape", async () => {
    const user = userEvent.setup();
    render(<EditorTriggerHarness />);
    const trigger = screen.getByRole("button", { name: "Open editor" });
    await user.click(trigger);

    const dialog = await screen.findByRole("dialog", {
      name: new RegExp(
        `^${t("playTitles.featured_dish", { name: "Carbonara" })}`,
      ),
    });
    await waitFor(() =>
      expect(dialog).toContainElement(document.activeElement as HTMLElement),
    );

    await user.keyboard("{Escape}");
    await waitFor(() => expect(dialog).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
  });

  it("uses Escape to confirm dirty close without leaving two modal focus traps", async () => {
    const user = userEvent.setup();
    renderEditor();
    fireEvent.change(screen.getByLabelText(t("slots.dishName")), {
      target: { value: "Dirty headline" },
    });

    await user.keyboard("{Escape}");
    const confirmation = await screen.findByRole("dialog", {
      name: t("discard.title"),
      hidden: true,
    });
    await waitFor(() =>
      expect(screen.getAllByRole("dialog", { hidden: true })).toEqual([
        confirmation,
      ]),
    );

    fireEvent.click(
      within(confirmation).getByRole("button", {
        name: t("discard.cancel"),
        hidden: true,
      }),
    );
    expect(await screen.findByLabelText(t("slots.dishName"))).toHaveValue(
      "Dirty headline",
    );
  });

  it("leaves Escape to the open language selector without closing the editor", async () => {
    const user = userEvent.setup();
    const onClose = jest.fn();
    renderEditor({ onClose });
    const languageTrigger = screen
      .getAllByLabelText(t("editor.captionLanguage"))
      .find((element) => element.getAttribute("aria-haspopup") === "listbox");
    expect(languageTrigger).toBeDefined();

    await user.click(languageTrigger!);
    expect(await screen.findByRole("listbox")).toBeInTheDocument();
    await user.keyboard("{Escape}");

    await waitFor(() =>
      expect(screen.queryByRole("listbox")).not.toBeInTheDocument(),
    );
    expect(
      screen.getByRole("dialog", {
        name: new RegExp(
          `^${t("playTitles.featured_dish", { name: "Carbonara" })}`,
        ),
      }),
    ).toBeInTheDocument();
    expect(onClose).not.toHaveBeenCalled();
    expect(
      screen.queryByRole("dialog", { name: t("discard.title"), hidden: true }),
    ).not.toBeInTheDocument();
  });

  it("resets discarded work before reopening the same mounted suggestion", async () => {
    const onClose = jest.fn();
    const { rerender } = renderEditor({ onClose });
    fireEvent.change(screen.getByLabelText(t("slots.dishName")), {
      target: { value: "Discard me" },
    });
    fireEvent.click(screen.getByText(t("editor.close")).closest("button")!);
    const discard = screen.getByRole("dialog", {
      name: t("discard.title"),
      hidden: true,
    });
    fireEvent.click(
      within(discard).getByRole("button", {
        name: t("discard.confirm"),
        hidden: true,
      }),
    );

    await waitFor(() =>
      expect(screen.getByLabelText(t("slots.dishName"))).toHaveValue(
        "Carbonara",
      ),
    );
    rerender(
      <PostEditorDrawer {...defaultProps} onClose={onClose} isOpen={false} />,
    );
    await waitFor(() =>
      expect(
        screen.queryByLabelText(t("slots.dishName")),
      ).not.toBeInTheDocument(),
    );
    rerender(<PostEditorDrawer {...defaultProps} onClose={onClose} isOpen />);
    await screen.findByLabelText(t("slots.dishName"));
    fireEvent.click(screen.getByText(t("editor.close")).closest("button")!);

    expect(onClose).toHaveBeenCalledTimes(2);
    expect(
      screen.queryByRole("dialog", {
        name: t("discard.title"),
        hidden: true,
      }),
    ).not.toBeInTheDocument();
  });

  it("shows free versus AI provenance and confirms direction and ratio before generating", async () => {
    renderEditor();
    expect(
      screen.getByText(
        t("provenance.free", { source: t("provenance.sources.menu") }),
      ),
    ).toBeInTheDocument();

    fireEvent.click(screen.getByRole("tab", { name: t("photoSource.ai") }));
    fireEvent.click(screen.getByRole("button", { name: t("editor.newPhoto") }));
    expect(mockGenerateImage).not.toHaveBeenCalled();

    const confirm = screen.getByRole("dialog", {
      name: t("generationConfirm.title"),
      hidden: true,
    });
    expect(confirm).toHaveTextContent(
      t("generationConfirm.direction", { name: "Carbonara" }),
    );
    expect(confirm).toHaveTextContent(
      t("generationConfirm.ratio", { ratio: t(FORMATS["4:5"].labelKey) }),
    );
    expect(confirm).not.toHaveTextContent(/credit/i);
    fireEvent.click(
      within(confirm).getByRole("button", {
        name: t("generationConfirm.confirm"),
        hidden: true,
      }),
    );

    await waitFor(() => expect(mockGenerateImage).toHaveBeenCalledTimes(1));
    expect(await screen.findByText(t("provenance.ai"))).toBeInTheDocument();

    fireEvent.click(screen.getByRole("tab", { name: t("photoSource.ai") }));
    fireEvent.click(screen.getByRole("button", { name: t("editor.newPhoto") }));
    const retryConfirm = screen.getByRole("dialog", {
      name: t("generationConfirm.title"),
      hidden: true,
    });
    expect(retryConfirm).toHaveTextContent(t("generationConfirm.paidRetryWarning"));
    expect(retryConfirm).not.toHaveTextContent(
      t("generationConfirm.freeAlternative"),
    );
  });

  it("keeps a paid generation owned by the editor until it succeeds, then allows an explicit discard", async () => {
    let resolveGeneration!: (result: { url: string; credit: "paid" }) => void;
    mockGenerateImage.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveGeneration = resolve;
      }),
    );
    const onClose = jest.fn();
    mockGenerateCaption.mockResolvedValueOnce("AI caption");
    renderEditor({ onClose });

    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: t("editor.regenerateCaption") }),
      ).not.toBeDisabled(),
    );
    fireEvent.change(screen.getByLabelText(t("crop.focalX")), {
      target: { value: "0.2" },
    });
    fireEvent.change(screen.getByLabelText(t("crop.focalY")), {
      target: { value: "0.7" },
    });
    fireEvent.change(screen.getByLabelText(t("crop.zoom")), {
      target: { value: "1.4" },
    });

    fireEvent.click(screen.getByRole("tab", { name: t("photoSource.ai") }));
    fireEvent.click(screen.getByRole("button", { name: t("editor.newPhoto") }));
    const confirmation = screen.getByRole("dialog", {
      name: t("generationConfirm.title"),
      hidden: true,
    });
    fireEvent.click(
      within(confirmation).getByRole("button", {
        name: t("generationConfirm.confirm"),
        hidden: true,
      }),
    );

    await waitFor(() => expect(mockGenerateImage).toHaveBeenCalledTimes(1));
    const controls = screen.getByTestId("editor-controls-column");
    expect(controls).toHaveAttribute("aria-busy", "true");
    expect(controls).toHaveAttribute("aria-disabled", "true");
    for (const tab of ["thisPhoto", "gallery", "upload", "ai"] as const) {
      expect(
        screen.getByRole("tab", { name: t(`photoSource.${tab}`) }),
      ).toBeDisabled();
    }
    for (const cropControl of ["focalX", "focalY", "zoom"] as const) {
      expect(screen.getByLabelText(t(`crop.${cropControl}`))).toBeDisabled();
    }
    expect(
      screen.getByRole("button", { name: t("crop.reset") }),
    ).toBeDisabled();
    for (const formatId of FORMAT_ORDER) {
      // Destination pack chips + layout format chips share labels.
      const buttons = screen.getAllByRole("button", {
        name: t(FORMATS[formatId].labelKey),
      });
      expect(buttons.length).toBeGreaterThan(0);
      buttons.forEach((button) => expect(button).toBeDisabled());
    }
    for (const kit of KIT_ORDER) {
      expect(
        screen.getByRole("button", { name: t(`kits.${kit}`) }),
      ).toBeDisabled();
    }
    expect(
      screen.getByRole("button", { name: t("editor.reshuffle") }),
    ).toBeDisabled();
    expect(screen.getByLabelText(t("slots.dishName"))).toBeDisabled();
    for (const tone of ["warm", "playful", "elegant", "punchy"] as const) {
      expect(
        screen.getByRole("button", { name: t(`tones.${tone}`) }),
      ).toBeDisabled();
    }
    for (const languageControl of screen.getAllByLabelText(
      t("editor.captionLanguage"),
    )) {
      expect(languageControl).toBeDisabled();
    }
    expect(screen.getByLabelText(t("editor.caption"))).toBeDisabled();
    expect(
      screen.getByRole("button", { name: t("editor.regenerateCaption") }),
    ).toBeDisabled();
    expect(
      screen.getByRole("button", { name: t("editor.copyCaption") }),
    ).toBeDisabled();

    fireEvent.change(screen.getByLabelText(t("crop.focalX")), {
      target: { value: "0.8" },
    });
    fireEvent.change(screen.getByLabelText(t("crop.focalY")), {
      target: { value: "0.1" },
    });
    fireEvent.change(screen.getByLabelText(t("crop.zoom")), {
      target: { value: "2.5" },
    });
    fireEvent.click(screen.getByRole("button", { name: t("crop.reset") }));
    fireEvent.click(
      screen.getByRole("button", { name: t(FORMATS["9:16"].labelKey) }),
    );
    fireEvent.click(screen.getByRole("button", { name: t("kits.minimal") }));
    fireEvent.click(
      screen.getByRole("button", { name: t("editor.reshuffle") }),
    );
    fireEvent.change(screen.getByLabelText(t("slots.dishName")), {
      target: { value: "Changed while billed" },
    });
    fireEvent.click(screen.getByRole("button", { name: t("tones.playful") }));
    fireEvent.change(screen.getByLabelText(t("editor.caption")), {
      target: { value: "Changed copy while billed" },
    });
    fireEvent.click(
      screen.getByRole("button", { name: t("editor.regenerateCaption") }),
    );
    fireEvent.click(
      screen.getByRole("tab", { name: t("photoSource.gallery") }),
    );
    fireEvent.click(screen.getByRole("tab", { name: t("photoSource.upload") }));
    fireEvent.click(
      screen.getByRole("tab", { name: t("photoSource.thisPhoto") }),
    );

    expect(screen.getByLabelText(t("crop.focalX"))).toHaveValue("0.2");
    expect(screen.getByLabelText(t("crop.focalY"))).toHaveValue("0.7");
    expect(screen.getByLabelText(t("crop.zoom"))).toHaveValue("1.4");
    expect(screen.getByLabelText(t("slots.dishName"))).toHaveValue("Carbonara");
    // Destination pack + layout both expose the hero format chip.
    screen
      .getAllByRole("button", { name: t(FORMATS["4:5"].labelKey) })
      .forEach((button) =>
        expect(button).toHaveAttribute("aria-pressed", "true"),
      );
    expect(
      screen.getByRole("button", { name: t("kits.editorial") }),
    ).toHaveAttribute("aria-pressed", "true");
    expect(mockPreviewRenderInput?.composition).toBe("photoBottomStack");
    expect(screen.getByLabelText(t("editor.caption"))).toHaveValue(
      "AI caption",
    );
    expect(
      screen.getByRole("tab", { name: t("photoSource.ai") }),
    ).toHaveAttribute("aria-selected", "true");
    const close = screen.getByText(t("editor.close")).closest("button")!;
    expect(close).toBeDisabled();
    expect(screen.getByText(t("editor.generationBusy"))).toBeInTheDocument();
    fireEvent.click(close);
    fireEvent.keyDown(document, { key: "Escape" });
    expect(onClose).not.toHaveBeenCalled();
    expect(
      screen.queryByRole("dialog", { name: t("discard.title"), hidden: true }),
    ).not.toBeInTheDocument();

    await act(async () =>
      resolveGeneration({
        url: "https://cdn/generated-after-wait.jpg",
        credit: "paid",
      }),
    );
    await screen.findByText(t("provenance.ai"));
    expect(controls).toHaveAttribute("aria-busy", "false");
    expect(controls).toHaveAttribute("aria-disabled", "false");
    expect(close).not.toBeDisabled();
    fireEvent.click(close);
    const discard = screen.getByRole("dialog", {
      name: t("discard.title"),
      hidden: true,
    });
    fireEvent.click(
      within(discard).getByRole("button", {
        name: t("discard.confirm"),
        hidden: true,
      }),
    );
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("allows closing after a paid generation fails", async () => {
    let rejectGeneration!: (reason: unknown) => void;
    mockGenerateImage.mockReturnValueOnce(
      new Promise((_resolve, reject) => {
        rejectGeneration = reject;
      }),
    );
    const onClose = jest.fn();
    renderEditor({ onClose });

    fireEvent.click(screen.getByRole("tab", { name: t("photoSource.ai") }));
    fireEvent.click(screen.getByRole("button", { name: t("editor.newPhoto") }));
    fireEvent.click(
      within(
        screen.getByRole("dialog", {
          name: t("generationConfirm.title"),
          hidden: true,
        }),
      ).getByRole("button", {
        name: t("generationConfirm.confirm"),
        hidden: true,
      }),
    );
    await waitFor(() => expect(mockGenerateImage).toHaveBeenCalledTimes(1));

    await act(async () => rejectGeneration(new Error("offline")));
    await screen.findByRole("alert");
    const close = screen.getByText(t("editor.close")).closest("button")!;
    expect(close).not.toBeDisabled();
    fireEvent.click(close);
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("does not expose an editor without edit capability", () => {
    renderEditor({ canEdit: false } as any);

    expect(
      screen.queryByTestId("editor-controls-column"),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: t("editor.downloadPack") }),
    ).not.toBeInTheDocument();
  });

  it("hides AI and upload sources when their capabilities are absent", () => {
    renderEditor({ canGenerate: false, canUpload: false } as any);

    expect(
      screen.queryByRole("tab", { name: t("photoSource.ai") }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("tab", { name: t("photoSource.upload") }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("tab", { name: t("photoSource.gallery") }),
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: t("editor.regenerateCaption") }),
    ).not.toBeInTheDocument();
  });

  it("surfaces the daily limit panel when generation hits the fair-use ceiling", async () => {
    mockGenerateImage.mockRejectedValueOnce(
      new ImageDailyLimitError(500, 21600),
    );
    renderEditor();
    fireEvent.click(screen.getByRole("tab", { name: t("photoSource.ai") }));
    fireEvent.click(screen.getByRole("button", { name: t("editor.newPhoto") }));
    const confirm = screen.getByRole("dialog", {
      name: t("generationConfirm.title"),
      hidden: true,
    });
    fireEvent.click(
      within(confirm).getByRole("button", {
        name: t("generationConfirm.confirm"),
        hidden: true,
      }),
    );

    // Title, resets, and contact are three interpolated spans in one paragraph
    // (same layout as Menu Builder AIImageTools) — match the combined message.
    // Other role=status nodes exist in the drawer (e.g. logo-missing), so scope
    // by content then assert the live region on the panel.
    const title = await screen.findByText(
      /500 AI images today/i,
      {},
      { timeout: 5000 },
    );
    const status = title.closest('[role="status"]');
    expect(status).not.toBeNull();
    expect(status).toHaveTextContent(/resets in about 6 hours/i);
    expect(status).toHaveTextContent(/Need more today\? Get in touch/i);
    expect(
      screen.queryByRole("button", { name: /buy/i }),
    ).not.toBeInTheDocument();
    // Buttons stay live so a retry can succeed once the limit resets (or re-set
    // the panel with a fresh resets_in_seconds) — matches Menu Builder.
    // Covers both generation and cleanup entry points (M6 left cleanup undefended).
    expect(
      screen.getByRole("button", { name: t("editor.newPhoto") }),
    ).not.toBeDisabled();
    expect(
      screen.getByRole("button", { name: t("editor.cleanupPhoto") }),
    ).not.toBeDisabled();
  });

  it("distinguishes gallery loading, failure with retry, empty, and loaded states", async () => {
    let rejectGallery!: (reason?: unknown) => void;
    mockGetGallery.mockReturnValueOnce(
      new Promise((_resolve, reject) => {
        rejectGallery = reject;
      }),
    );
    renderEditor();
    fireEvent.click(
      screen.getByRole("tab", { name: t("photoSource.gallery") }),
    );
    expect(
      screen.getByText(t("photoSource.galleryLoading")),
    ).toBeInTheDocument();

    await act(async () => rejectGallery(new Error("offline")));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      t("photoSource.galleryFailed"),
    );

    mockGetGallery.mockResolvedValueOnce([]);
    fireEvent.click(
      screen.getByRole("button", { name: t("photoSource.galleryRetry") }),
    );
    expect(
      await screen.findByText(t("photoSource.galleryEmpty")),
    ).toBeInTheDocument();

    mockGetGallery.mockResolvedValueOnce([
      {
        id: 9,
        business_id: 42,
        image_url: "https://cdn/gallery.jpg",
        caption: "Dining room",
        is_active: true,
      } as any,
    ]);
    fireEvent.click(
      screen.getByRole("button", { name: t("photoSource.galleryRetry") }),
    );
    expect(
      await screen.findByRole("button", { name: "Dining room" }),
    ).toBeInTheDocument();
  });

  it("starts pending and enables export only after the current preview succeeds", () => {
    renderEditor();
    const download = screen.getByRole("button", {
      name: t("editor.downloadPack"),
    });
    expect(download).toBeDisabled();
    const checklist = screen.getByTestId("post-readiness-checklist");
    expect(checklist.querySelector('[data-step="preview"]')).toHaveAttribute(
      "data-ready",
      "false",
    );

    fireEvent.click(screen.getByRole("button", { name: "Ready preview" }));
    expect(download).not.toBeDisabled();
    expect(checklist).toHaveAttribute("data-export-ready", "true");

    fireEvent.click(screen.getByRole("button", { name: "Fail preview" }));
    expect(download).toBeDisabled();
    expect(checklist.querySelector('[data-step="preview"]')).toHaveAttribute(
      "data-ready",
      "false",
    );
    expect(checklist.querySelector('[data-step="preview"]')).toHaveAttribute(
      "data-broken",
      "true",
    );
    expect(
      screen.getByText(t("editor.checklist.previewBroken")),
    ).toBeInTheDocument();
    // Ready chip never stays green while preview is broken.
    expect(checklist.querySelector('[data-step="ready"]')).toHaveAttribute(
      "data-ready",
      "false",
    );

    fireEvent.click(screen.getByRole("button", { name: "Ready preview" }));
    expect(download).not.toBeDisabled();
    expect(checklist).toHaveAttribute("data-export-ready", "true");
  });

  it("renders preview first on mobile and makes the wide preview sticky", () => {
    renderEditor();
    const preview = screen.getByTestId("editor-preview-column");
    const controls = screen.getByTestId("editor-controls-column");
    expect(
      preview.compareDocumentPosition(controls) &
        Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy();
    expect(preview).toHaveClass("lg:sticky");
    expect(
      screen.getByRole("heading", { name: t("sections.image") }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: t("sections.layout") }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: t("sections.copy") }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: t("sections.export") }),
    ).toBeInTheDocument();
    expect(screen.getByLabelText(t("crop.focalX"))).toHaveAttribute(
      "type",
      "range",
    );
    expect(screen.getByLabelText(t("crop.focalY"))).toHaveAttribute(
      "type",
      "range",
    );
    expect(screen.getByLabelText(t("crop.zoom"))).toHaveAttribute(
      "type",
      "range",
    );
  });

  /**
   * The drawer offers a slot editor for exactly the bands the render will
   * paint. That set is a property of the composition, so these pin both
   * branches of `paintedSlotKeys`: a kit-path post has no `TemplateDef` to
   * read at all, and a legacy post must keep offering what its template
   * offered before the slot set moved to the composition.
   */
  const SLOT_KEYS = ["badge", "dishName", "price", "cta", "handle"] as const;

  const visibleSlotEditors = () =>
    SLOT_KEYS.filter(
      (key) => screen.queryAllByLabelText(t(`slots.${key}`)).length > 0,
    );

  it.each([
    ["minimal", ["dishName", "price", "handle"]],
    ["editorial", [...SLOT_KEYS]],
    ["bold", [...SLOT_KEYS]],
  ] as const)(
    "offers the %s template's own slots when the post has no kit",
    async (style, expected) => {
      // The legacy path is no longer reachable through the drawer's own
      // controls — the composer always emits a kit — but a `RenderPostInput`
      // built from a pre-Wave-1 snapshot still is, so the branch stays pinned.
      // minimal defines dishName/price/handle and nothing else, so its legacy
      // composition must not surface badge or cta. Deriving this from the
      // composer's `composition` instead would show all five: that field always
      // holds a choosable family (photoBottomStack here), never a legacy one.
      mockRenderInputOverride = {
        kit: undefined,
        composition: undefined,
        template: TEMPLATES[style],
      };
      renderEditor();
      await screen.findByRole("heading", { name: t("sections.layout") });

      await waitFor(() => expect(visibleSlotEditors()).toEqual([...expected]));
    },
  );

  it("offers the composition's bands when the post is on the kit path", async () => {
    // badgeHero lays out badge/dishName/cta/handle and deliberately no price.
    mockRenderInputOverride = { kit: "editorial", composition: "badgeHero" };
    renderEditor();
    await screen.findByRole("heading", { name: t("sections.layout") });

    await waitFor(() =>
      expect(visibleSlotEditors()).toEqual([
        "badge",
        "dishName",
        "cta",
        "handle",
      ]),
    );
    expect(screen.queryAllByLabelText(t("slots.price"))).toHaveLength(0);
  });

  it("falls back to the composer's composition when a kit-path input carries none", async () => {
    mockRenderInputOverride = { kit: "editorial", composition: undefined };
    renderEditor();
    await screen.findByRole("heading", { name: t("sections.layout") });

    // The composer seeded photoBottomStack for this suggestion, whose standard
    // band set is all five. Without the fall back this would index COMPOSITIONS
    // with undefined and offer no editors at all.
    await waitFor(() => expect(visibleSlotEditors()).toEqual([...SLOT_KEYS]));
  });

  /**
   * The drawer and the renderer must answer "which composition is this post?"
   * with the SAME function, not with two copies of the same precedence rule.
   *
   * The two tests above pin the answers this legacy input must produce; this
   * one pins that the drawer got them by asking `resolveArtDirection` rather
   * than by restating it. `legacyMinimal` is hard-coded on the left of the
   * comparison on purpose — deriving BOTH sides from the resolver would make
   * the assertion true for whatever the resolver happened to return, which is
   * exactly the tautology that lets a duplicated rule pass its own test.
   *
   * Aspect is passed as "1:1" because all three legacy families' `bandsFor`
   * ignore it (they reproduce a fixed template slot set); the drawer's own
   * aspect therefore cannot change the expected set.
   */
  it("derives its slot editors from the same resolution the renderer uses", async () => {
    mockRenderInputOverride = {
      kit: undefined,
      composition: undefined,
      template: TEMPLATES.minimal,
    };
    renderEditor();
    await screen.findByRole("heading", { name: t("sections.layout") });

    const resolved = resolveArtDirection(
      {
        template: TEMPLATES.minimal,
        aspect: "1:1",
        photoUrl: "",
        slots: { dishName: "Milanesa", price: "$12.00" },
        palette: { primary: "#1a6b6a", secondary: "#0f3d3c" },
      },
      false,
    );
    expect(resolved.compositionId).toBe("legacyMinimal");

    const painted = COMPOSITIONS[resolved.compositionId]
      .bandsFor(FORMATS["1:1"])
      .map((band) => band.key);
    await waitFor(() =>
      expect(visibleSlotEditors()).toEqual(
        SLOT_KEYS.filter((key) => painted.includes(key)),
      ),
    );
  });

  describe("art direction controls", () => {
    const styleGroup = () =>
      screen.findByRole("group", { name: t("editor.styleLabel") });

    it("offers one button per kit and presses the composer's current kit", async () => {
      renderEditor();
      const group = await styleGroup();

      const buttons = within(group).getAllByRole("button");
      expect(buttons).toHaveLength(KIT_ORDER.length);
      expect(buttons.map((button) => button.textContent)).toEqual(
        KIT_ORDER.map((id) => t(`kits.${id}`)),
      );
      expect(
        within(group).getByRole("button", { name: t("kits.editorial") }),
      ).toHaveAttribute("aria-pressed", "true");
    });

    it("renders and exports the picked kit instead of the legacy template", async () => {
      mockGenerateCaption.mockResolvedValue("AI caption");
      const onHandoff = jest.fn();
      renderEditor({ onHandoff });
      const group = await styleGroup();
      const chalkboard = within(group).getByRole("button", {
        name: t("kits.chalkboard"),
      });

      fireEvent.click(chalkboard);
      await waitFor(() =>
        expect(chalkboard).toHaveAttribute("aria-pressed", "true"),
      );

      // The load-bearing half of the wire: preview, download and share pack all
      // render from this one object. A `template` left on it would be the field
      // `resolveArtDirection` falls back to, so its absence is asserted too.
      await waitFor(() =>
        expect(mockPreviewRenderInput?.kit).toBe("chalkboard"),
      );
      expect(mockPreviewRenderInput?.template).toBeUndefined();
      expect(mockPreviewRenderInput?.composition).toBe("photoBottomStack");

      fireEvent.click(screen.getByRole("button", { name: "Ready preview" }));
      fireEvent.click(
        screen.getByRole("button", { name: t("editor.downloadPack") }),
      );
      await waitFor(() => expect(onHandoff).toHaveBeenCalledTimes(1));
      expect(onHandoff).toHaveBeenLastCalledWith(
        expect.objectContaining({
          creative: expect.objectContaining({ kit: "chalkboard" }),
        }),
      );
    });

    it("rotates the composition on reshuffle and leaves the kit alone", async () => {
      renderEditor();
      const group = await styleGroup();
      const ticket = within(group).getByRole("button", {
        name: t("kits.ticket"),
      });
      fireEvent.click(ticket);
      await waitFor(() => expect(mockPreviewRenderInput?.kit).toBe("ticket"));
      expect(mockPreviewRenderInput?.composition).toBe("photoBottomStack");

      // This fixture carries a photo, so more than one family survives the
      // usability filter and the control is live. Without that the button is
      // disabled, the click does nothing, and the kit assertion below passes
      // for the wrong reason.
      const reshuffle = screen.getByRole("button", {
        name: t("editor.reshuffle"),
      });
      expect(reshuffle).not.toBeDisabled();
      fireEvent.click(reshuffle);

      await waitFor(() =>
        expect(mockPreviewRenderInput?.composition).toBe("photoTopStack"),
      );
      expect(mockPreviewRenderInput?.kit).toBe("ticket");
      expect(ticket).toHaveAttribute("aria-pressed", "true");
    });

    it("disables reshuffle when there is no photo and the rotation is a fixed point", async () => {
      renderEditor({ suggestion: { ...suggestion, image_url: "" } as any });
      await styleGroup();

      expect(mockPreviewRenderInput?.composition).toBe("posterStack");
      expect(
        screen.getByRole("button", { name: t("editor.reshuffle") }),
      ).toBeDisabled();
    });
  });

  describe("motion export", () => {
    it("offers every motion preset plus a still option", async () => {
      renderEditor();
      expect(
        await screen.findByRole("group", { name: /motion/i }),
      ).toBeInTheDocument();
      [
        "Slow push in",
        "Drift across",
        "Line by line",
        "Badge first",
        "Grain wash",
        "Ticket print",
      ].forEach((label) =>
        expect(screen.getByRole("button", { name: label })).toBeInTheDocument(),
      );
    });

    it("downloads a video with the selected preset", async () => {
      renderEditor();
      await userEvent.click(
        screen.getByRole("button", { name: "Drift across" }),
      );
      await userEvent.click(
        screen.getByRole("button", { name: /download video/i }),
      );
      await waitFor(() =>
        expect(mockDownloadPostVideo).toHaveBeenCalledTimes(1),
      );
      expect(mockDownloadPostVideo.mock.calls[0][0]).toMatchObject({
        preset: "slowPan",
      });
    });

    it("shows a percentage while rendering and a cancel control", async () => {
      let reportProgress: ((fraction: number) => void) | undefined;
      mockDownloadPostVideo.mockImplementationOnce(
        (args: { onProgress?: (f: number) => void }) =>
          new Promise<void>(() => {
            reportProgress = args.onProgress;
          }),
      );
      renderEditor();
      await userEvent.click(
        screen.getByRole("button", { name: /download video/i }),
      );
      await waitFor(() => expect(reportProgress).toBeDefined());
      act(() => reportProgress?.(0.42));
      expect(await screen.findByText(/42%/)).toBeInTheDocument();
      expect(
        screen.getByRole("button", { name: /cancel/i }),
      ).toBeInTheDocument();
    });

    it("aborts the export when cancel is pressed", async () => {
      let seenSignal: AbortSignal | undefined;
      mockDownloadPostVideo.mockImplementationOnce(
        (args: { signal?: AbortSignal }) =>
          new Promise<void>(() => {
            seenSignal = args.signal;
          }),
      );
      renderEditor();
      await userEvent.click(
        screen.getByRole("button", { name: /download video/i }),
      );
      await waitFor(() => expect(seenSignal).toBeDefined());
      await userEvent.click(screen.getByRole("button", { name: /cancel/i }));
      expect(seenSignal?.aborted).toBe(true);
    });

    it("hides the video control and explains why on a browser without WebCodecs", async () => {
      mockDetectMotionSupport.mockReturnValue({
        supported: false,
        path: null,
        missing: ["VideoEncoder"],
      });
      renderEditor();
      expect(
        await screen.findByText(/video export needs a newer browser/i),
      ).toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: /download video/i }),
      ).toBeNull();
    });

    it("surfaces a failed export without losing the still download", async () => {
      mockGenerateCaption.mockResolvedValue("AI caption");
      mockDownloadPostVideo.mockRejectedValueOnce(new Error("encode_failed"));
      renderEditor();
      await waitFor(() =>
        expect(screen.getByLabelText(t("editor.caption"))).toHaveValue(
          "AI caption",
        ),
      );
      await userEvent.click(
        screen.getByRole("button", { name: /download video/i }),
      );
      expect(
        await screen.findByText(/video export failed/i),
      ).toBeInTheDocument();
      fireEvent.click(screen.getByRole("button", { name: "Ready preview" }));
      await waitFor(() =>
        expect(
          screen.getByRole("button", { name: t("editor.downloadPack") }),
        ).toBeEnabled(),
      );
    });
  });

  describe("campaign kit panel", () => {
    it("offers every registry format, with the hero preselected and locked on", async () => {
      renderEditor();
      const group = await screen.findByRole("group", {
        name: t("campaignKit.formatsLabel"),
      });
      const toggles = within(group).getAllByRole("checkbox");
      expect(toggles).toHaveLength(FORMAT_ORDER.length);
      const hero = within(group).getByRole("checkbox", {
        name: t(FORMATS["4:5"].labelKey),
      });
      expect(hero).toBeChecked();
      expect(hero).toBeDisabled();
    });

    it("lets the operator drop a non-hero format", async () => {
      const user = userEvent.setup();
      renderEditor();
      const group = await screen.findByRole("group", {
        name: t("campaignKit.formatsLabel"),
      });
      // Default destination pack is IG Feed → [4:5, 1:1]; square is non-hero.
      const square = within(group).getByRole("checkbox", {
        name: t(FORMATS["1:1"].labelKey),
      });
      expect(square).toBeChecked();
      await user.click(square);
      expect(square).not.toBeChecked();
    });

    it("labels the print format with its print hint", async () => {
      renderEditor();
      expect(
        await screen.findByText(t("campaignKit.printHint")),
      ).toBeInTheDocument();
    });

    it("disables the kit download until the post is export-ready", async () => {
      renderEditor({ suggestion: { ...suggestion, image_url: "" } as any });
      const button = await screen.findByRole("button", {
        name: t("campaignKit.download"),
      });
      expect(button).toBeDisabled();
    });

    // The stored concept is the hero plus the list. Six formats must not become
    // six Library rows.
    it("records one snapshot carrying the format list", async () => {
      mockGenerateCaption.mockResolvedValue("AI caption");
      const user = userEvent.setup();
      const onHandoff = jest.fn();
      renderEditor({ onHandoff });
      await waitFor(() =>
        expect(screen.getByLabelText(t("editor.caption"))).toHaveValue(
          "AI caption",
        ),
      );
      fireEvent.click(screen.getByRole("button", { name: "Ready preview" }));
      const button = await screen.findByRole("button", {
        name: t("campaignKit.download"),
      });
      await waitFor(() => expect(button).toBeEnabled());
      await user.click(button);
      await waitFor(() => expect(onHandoff).toHaveBeenCalledTimes(1));
      const creative = onHandoff.mock.calls[0][0].creative;
      expect(creative.aspect).toBe("4:5");
      // Fresh composer seeds the default destination pack (IG Feed), not the
      // full six-format campaign kit. Operators can still tick more formats.
      expect(creative.kit_formats).toEqual(["4:5", "1:1"]);
      expect(creative.destination_id).toBe("ig_feed");
    });
  });

  describe("motion snapshot round trip", () => {
    it("records media_kind image and no preset for a still download", async () => {
      mockGenerateCaption.mockResolvedValue("AI caption");
      const onHandoff = jest.fn();
      renderEditor({ onHandoff } as any);
      await waitFor(() =>
        expect(screen.getByLabelText(t("editor.caption"))).toHaveValue(
          "AI caption",
        ),
      );
      fireEvent.click(screen.getByRole("button", { name: "Ready preview" }));
      await waitFor(() =>
        expect(
          screen.getByRole("button", { name: t("editor.downloadPack") }),
        ).toBeEnabled(),
      );
      await userEvent.click(
        screen.getByRole("button", { name: t("editor.downloadPack") }),
      );
      await waitFor(() => expect(onHandoff).toHaveBeenCalled());
      expect(onHandoff.mock.calls[0][0].creative).toMatchObject({
        media_kind: "image",
      });
      expect(onHandoff.mock.calls[0][0].creative.motion_preset).toBeUndefined();
    });

    it("records media_kind video and the preset after a video download", async () => {
      const onHandoff = jest.fn();
      renderEditor({ onHandoff } as any);
      await userEvent.click(
        screen.getByRole("button", { name: "Badge first" }),
      );
      await userEvent.click(
        screen.getByRole("button", { name: /download video/i }),
      );
      await waitFor(() => expect(onHandoff).toHaveBeenCalled());
      expect(onHandoff.mock.calls[0][0].creative).toMatchObject({
        media_kind: "video",
        motion_preset: "badgePop",
      });
    });

    /**
     * The backend rejects `media_kind: "video"` with an empty preset outright
     * (marketing_activity_handlers.go). Asserting the pair here catches the
     * mismatch in jest rather than as a 400 an operator sees.
     */
    it("never emits a video media kind without a preset", async () => {
      const onHandoff = jest.fn();
      renderEditor({ onHandoff } as any);
      await userEvent.click(
        screen.getByRole("button", { name: /download video/i }),
      );
      await waitFor(() => expect(onHandoff).toHaveBeenCalled());
      const creative = onHandoff.mock.calls[0][0].creative;
      if (creative.media_kind === "video") {
        expect(typeof creative.motion_preset).toBe("string");
        expect(creative.motion_preset).not.toBe("");
      }
    });
  });

  describe("simple vs craft progressive disclosure", () => {
    it("defaults to simple when no preference is stored", () => {
      localStorage.clear();
      renderEditor();
      expect(screen.getByTestId("post-editor-drawer")).toHaveAttribute(
        "data-editor-mode",
        "simple",
      );
      expect(screen.getByTestId("editor-mode-simple")).toHaveAttribute(
        "aria-pressed",
        "true",
      );
      expect(
        screen.queryByTestId("craft-style-controls"),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByTestId("craft-motion-presets"),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByTestId("craft-campaign-kit"),
      ).not.toBeInTheDocument();
      // Narrative pack is destination craft, not craft-only disclosure.
      expect(screen.getByTestId("narrative-campaign-pack")).toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: /download video/i }),
      ).not.toBeInTheDocument();
    });

    it("shows craft controls after toggle without clearing photo or caption", () => {
      localStorage.clear();
      writeEditorMode(business.id, "simple");
      renderEditor();
      const caption = screen.getByLabelText(t("editor.caption"));
      const before = (caption as HTMLTextAreaElement).value;
      expect(before.length).toBeGreaterThan(0);

      fireEvent.click(screen.getByTestId("editor-mode-craft"));
      expect(screen.getByTestId("post-editor-drawer")).toHaveAttribute(
        "data-editor-mode",
        "craft",
      );
      expect(screen.getByTestId("craft-style-controls")).toBeInTheDocument();
      expect(screen.getByTestId("craft-motion-presets")).toBeInTheDocument();
      expect(screen.getByTestId("craft-campaign-kit")).toBeInTheDocument();
      expect(screen.getByTestId("narrative-campaign-pack")).toBeInTheDocument();
      expect((caption as HTMLTextAreaElement).value).toBe(before);
      expect(
        localStorage.getItem(`payverge:marketing:editorMode:${business.id}`),
      ).toBe("craft");
    });

    it("hides craft surfaces when switching back to simple", () => {
      renderEditor();
      expect(screen.getByTestId("craft-style-controls")).toBeInTheDocument();
      fireEvent.click(screen.getByTestId("editor-mode-simple"));
      expect(
        screen.queryByTestId("craft-style-controls"),
      ).not.toBeInTheDocument();
      expect(
        screen.queryByTestId("craft-campaign-kit"),
      ).not.toBeInTheDocument();
      expect(screen.getByTestId("narrative-campaign-pack")).toBeInTheDocument();
    });
  });

  describe("narrative campaign pack", () => {
    it("lists the five story roles and exports on click when ready", async () => {
      const user = userEvent.setup();
      renderEditor();
      const pack = await screen.findByTestId("narrative-campaign-pack");
      expect(
        within(pack).getByText(t("narrativeKit.readyNow")),
      ).toBeInTheDocument();
      expect(
        within(pack).getByText(t("narrativeKit.roles.hero.name")),
      ).toBeInTheDocument();
      expect(
        within(pack).getByText(t("narrativeKit.roles.teaser.name")),
      ).toBeInTheDocument();
      expect(
        within(pack).getByText(t("narrativeKit.roles.story.name")),
      ).toBeInTheDocument();
      expect(
        within(pack).getByText(t("narrativeKit.roles.tent.name")),
      ).toBeInTheDocument();
      expect(
        within(pack).getByText(t("narrativeKit.roles.email_strip.name")),
      ).toBeInTheDocument();

      fireEvent.click(screen.getByRole("button", { name: "Ready preview" }));
      const exportBtn = within(pack).getByTestId("export-narrative-pack");
      await waitFor(() => expect(exportBtn).toBeEnabled());
      await user.click(exportBtn);
      await waitFor(() =>
        expect(mockExportNarrativeKit).toHaveBeenCalledTimes(1),
      );
      const req = mockExportNarrativeKit.mock.calls[0]?.[0] as
        | NarrativeKitExportRequest
        | undefined;
      expect(req).toBeDefined();
      expect(req!.roles).toEqual([
        "hero",
        "teaser",
        "story",
        "tent",
        "email_strip",
      ]);
      expect(req!.readmeText).toBe(t("narrativeKit.readme"));
      // No schedule fields on the request object.
      expect(req).not.toHaveProperty("scheduled_for");
      expect(req).not.toHaveProperty("due_at");
    });
  });

  describe("honest readiness gates", () => {
    it("keeps ready and export disabled while preview is broken", () => {
      renderEditor();
      fireEvent.click(screen.getByRole("button", { name: "Ready preview" }));
      const checklist = screen.getByTestId("post-readiness-checklist");
      expect(checklist).toHaveAttribute("data-export-ready", "true");

      fireEvent.click(screen.getByRole("button", { name: "Fail preview" }));
      expect(checklist).toHaveAttribute("data-export-ready", "false");
      expect(checklist).toHaveAttribute("data-preview-broken", "true");
      const previewStep = checklist.querySelector('[data-step="preview"]');
      expect(previewStep).toHaveAttribute("data-ready", "false");
      expect(previewStep).toHaveAttribute("data-broken", "true");
      const readyStep = checklist.querySelector('[data-step="ready"]');
      expect(readyStep).toHaveAttribute("data-ready", "false");
      expect(
        screen.getByRole("button", { name: t("editor.downloadPack") }),
      ).toBeDisabled();
      expect(
        screen.getByText(t("editor.exportPreviewBroken")),
      ).toBeInTheDocument();
    });

    it("never greens ready without photo even if preview is ready", () => {
      renderEditor({
        suggestion: {
          ...suggestion,
          image_url: "",
          image_source: "none",
        } as any,
      });
      fireEvent.click(screen.getByRole("button", { name: "Ready preview" }));
      const checklist = screen.getByTestId("post-readiness-checklist");
      const photoStep = checklist.querySelector('[data-step="photo"]');
      expect(photoStep).toHaveAttribute("data-ready", "false");
      expect(checklist).toHaveAttribute("data-export-ready", "false");
      const readyStep = checklist.querySelector('[data-step="ready"]');
      expect(readyStep).toHaveAttribute("data-ready", "false");
    });
  });

  describe("manual studio export outcomes", () => {
    async function readyManualGalleryPost() {
      mockGetGallery.mockResolvedValueOnce([
        {
          id: 1,
          business_id: 42,
          image_url: "https://cdn/demo-gallery-1.jpg",
          caption: "Demo gallery 1",
          is_active: true,
        } as any,
      ]);
      const onHandoff = jest.fn();
      renderEditor({ suggestion: null, onHandoff });
      fireEvent.click(
        screen.getByRole("tab", { name: t("photoSource.gallery") }),
      );
      fireEvent.click(
        await screen.findByRole("button", { name: "Demo gallery 1" }),
      );
      fireEvent.change(screen.getByLabelText(t("editor.caption")), {
        target: { value: "Una noche simple, rica y sin vueltas." },
      });
      fireEvent.click(screen.getByRole("button", { name: "Ready preview" }));
      expect(screen.getByText(t("editor.exportReady"))).toBeInTheDocument();
      return onHandoff;
    }

    it("toasts a named feed-pack success for a gallery-backed manual post", async () => {
      const onHandoff = await readyManualGalleryPost();
      fireEvent.click(
        screen.getByRole("button", { name: t("destinations.ig_feed.export") }),
      );
      await waitFor(() => expect(mockExportKit).toHaveBeenCalledTimes(1));
      expect(mockExportKit).toHaveBeenCalledWith(
        expect.objectContaining({
          caption: "Una noche simple, rica y sin vueltas.",
          targetName: expect.any(String),
        }),
      );
      await waitFor(() =>
        expect(toast.success).toHaveBeenCalledWith(
          t("destinations.exportSuccess"),
        ),
      );
      expect(toast.error).not.toHaveBeenCalled();
      // Manual drafts have no suggestion-backed persistence path.
      expect(onHandoff).not.toHaveBeenCalled();
    });

    it("toasts and replaces the ready line when the feed pack fails", async () => {
      mockExportKit.mockResolvedValueOnce(false);
      await readyManualGalleryPost();
      fireEvent.click(
        screen.getByRole("button", { name: t("destinations.ig_feed.export") }),
      );
      await waitFor(() =>
        expect(toast.error).toHaveBeenCalledWith(t("destinations.exportFailed")),
      );
      expect(screen.getByTestId("export-outcome")).toHaveTextContent(
        t("destinations.exportFailed"),
      );
      expect(screen.queryByText(t("editor.exportReady"))).not.toBeInTheDocument();
      expect(toast.success).not.toHaveBeenCalled();
    });

    it("toasts a named single-post success for a gallery-backed manual post", async () => {
      const onHandoff = await readyManualGalleryPost();
      fireEvent.click(
        screen.getByRole("button", { name: t("editor.downloadPack") }),
      );
      await waitFor(() =>
        expect(mockDownloadPostPack).toHaveBeenCalledTimes(1),
      );
      await waitFor(() =>
        expect(toast.success).toHaveBeenCalledWith(
          t("card.downloadPackSuccess"),
        ),
      );
      expect(onHandoff).not.toHaveBeenCalled();
    });

    it("toasts and replaces the ready line when the single-post download fails", async () => {
      mockDownloadPostPack.mockRejectedValueOnce(new Error("png_export_failed"));
      await readyManualGalleryPost();
      fireEvent.click(
        screen.getByRole("button", { name: t("editor.downloadPack") }),
      );
      await waitFor(() =>
        expect(toast.error).toHaveBeenCalledWith(t("errors.download_failed")),
      );
      expect(screen.getByTestId("export-outcome")).toHaveTextContent(
        t("errors.download_failed"),
      );
      expect(screen.queryByText(t("editor.exportReady"))).not.toBeInTheDocument();
    });
  });
});
