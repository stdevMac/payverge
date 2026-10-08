/** @jest-environment jsdom */
import { act, renderHook, waitFor } from "@testing-library/react";
import type { RenderPostInput } from "../templates/renderPost";
import {
  narrativeZipMembership,
  type NarrativeRoleId,
} from "../formats/narrativeRoles";
import { useCampaignKitExport } from "./useCampaignKitExport";

const mockRenderOrder: string[] = [];
let mockInFlight = 0;
let mockMaxInFlight = 0;

jest.mock("../templates/renderPost", () => ({
  renderPostToBlob: jest.fn(async (input: RenderPostInput) => {
    mockInFlight += 1;
    mockMaxInFlight = Math.max(mockMaxInFlight, mockInFlight);
    mockRenderOrder.push(input.aspect);
    await new Promise((resolve) => setTimeout(resolve, 1));
    mockInFlight -= 1;
    return new Blob([new Uint8Array([1, 2, 3])], { type: "image/png" });
  }),
  renderPostScene: jest.fn(async () => ({
    width: 100,
    height: 100,
    nodes: [],
  })),
}));

jest.mock("../formats/printWalker", () => ({
  renderScenePrintHtml: jest.fn(() => "<!doctype html><p>tent</p>"),
}));

const mockDownloads: Array<{ name: string; size: number }> = [];

const BASE: RenderPostInput = {
  kit: "editorial",
  composition: "photoBottomStack",
  aspect: "4:5",
  photoUrl: "https://cdn.example.com/dish.jpg",
  slots: { dishName: "Milanesa" },
  palette: { primary: "#1a6b6a", secondary: "#0f3d3c" },
};

beforeEach(() => {
  mockRenderOrder.length = 0;
  mockDownloads.length = 0;
  mockInFlight = 0;
  mockMaxInFlight = 0;
  const { renderPostToBlob } = jest.requireMock("../templates/renderPost");
  (renderPostToBlob as jest.Mock).mockImplementation(
    async (input: RenderPostInput) => {
      mockInFlight += 1;
      mockMaxInFlight = Math.max(mockMaxInFlight, mockInFlight);
      mockRenderOrder.push(input.aspect);
      await new Promise((resolve) => setTimeout(resolve, 1));
      mockInFlight -= 1;
      return new Blob([new Uint8Array([1, 2, 3])], { type: "image/png" });
    },
  );
  // jsdom's URL has no createObjectURL / revokeObjectURL.
  Object.defineProperty(URL, "createObjectURL", {
    configurable: true,
    writable: true,
    value: jest.fn(() => "blob:pv-kit"),
  });
  Object.defineProperty(URL, "revokeObjectURL", {
    configurable: true,
    writable: true,
    value: jest.fn(),
  });
  jest
    .spyOn(HTMLAnchorElement.prototype, "click")
    .mockImplementation(function (this: HTMLAnchorElement) {
      mockDownloads.push({ name: this.download, size: 0 });
    });
});

afterEach(() => {
  jest.restoreAllMocks();
});

describe("useCampaignKitExport", () => {
  it("renders every format in hero-first order", async () => {
    const { result } = renderHook(() => useCampaignKitExport());
    await act(async () => {
      await result.current.exportKit({
        base: BASE,
        formats: ["4:5", "9:16", "1:1"],
        caption: "Milanesa night",
        targetName: "Milanesa",
        origin: "https://payverge.io",
        lang: "en",
      });
    });
    expect(mockRenderOrder).toEqual(["4:5", "9:16", "1:1"]);
  });

  // Six full-resolution canvases at once is roughly 40MB of backing store, and
  // Safari refuses large canvas allocations well before that. Sequential also
  // keeps the shared image cache hot without racing on it.
  it("never renders two formats at once", async () => {
    const { result } = renderHook(() => useCampaignKitExport());
    await act(async () => {
      await result.current.exportKit({
        base: BASE,
        formats: ["4:5", "9:16", "1:1", "wide", "strip"],
        caption: "c",
        targetName: "Milanesa",
        origin: "",
        lang: "en",
      });
    });
    expect(mockMaxInFlight).toBe(1);
  });

  it("triggers exactly one download for the whole kit", async () => {
    const { result } = renderHook(() => useCampaignKitExport());
    await act(async () => {
      await result.current.exportKit({
        base: BASE,
        formats: ["4:5", "9:16", "5:7"],
        caption: "Milanesa night",
        targetName: "Milanesa",
        origin: "",
        lang: "en",
      });
    });
    expect(mockDownloads).toHaveLength(1);
    expect(mockDownloads[0].name).toBe("milanesa-campaign-kit.zip");
  });

  it("does not send the print format through the canvas renderer", async () => {
    const { result } = renderHook(() => useCampaignKitExport());
    await act(async () => {
      await result.current.exportKit({
        base: BASE,
        formats: ["4:5", "5:7"],
        caption: "c",
        targetName: "Milanesa",
        origin: "",
        lang: "en",
      });
    });
    expect(mockRenderOrder).toEqual(["4:5"]);
  });

  it("reports progress as it goes and clears when done", async () => {
    const { result } = renderHook(() => useCampaignKitExport());
    let promise!: Promise<boolean>;
    act(() => {
      promise = result.current.exportKit({
        base: BASE,
        formats: ["4:5", "9:16", "1:1"],
        caption: "c",
        targetName: "Milanesa",
        origin: "",
        lang: "en",
      });
    });
    await waitFor(() => expect(result.current.progress).not.toBeNull());
    expect(result.current.progress!.total).toBe(3);
    await act(async () => {
      await promise;
    });
    expect(result.current.progress).toBeNull();
  });

  // A CORS failure on one format must not lose the other five.
  it("keeps going when one format fails and reports which", async () => {
    const { renderPostToBlob } = jest.requireMock("../templates/renderPost");
    (renderPostToBlob as jest.Mock).mockImplementationOnce(async () => {
      throw new Error("png_export_failed");
    });
    const { result } = renderHook(() => useCampaignKitExport());
    let ok = false;
    await act(async () => {
      ok = await result.current.exportKit({
        base: BASE,
        formats: ["4:5", "9:16"],
        caption: "c",
        targetName: "Milanesa",
        origin: "",
        lang: "en",
      });
    });
    expect(ok).toBe(true);
    expect(result.current.failedFormats).toEqual(["4:5"]);
    expect(mockDownloads).toHaveLength(1);
  });

  it("fails cleanly when every format fails", async () => {
    const { renderPostToBlob } = jest.requireMock("../templates/renderPost");
    (renderPostToBlob as jest.Mock).mockImplementation(async () => {
      throw new Error("png_export_failed");
    });
    const { result } = renderHook(() => useCampaignKitExport());
    let ok = true;
    await act(async () => {
      ok = await result.current.exportKit({
        base: BASE,
        formats: ["4:5", "9:16"],
        caption: "c",
        targetName: "Milanesa",
        origin: "",
        lang: "en",
      });
    });
    expect(ok).toBe(false);
    expect(mockDownloads).toHaveLength(0);
    expect(result.current.error).toBe("kit_export_failed");
  });

  it("does not revoke the zip object URL in the same turn as the download click", async () => {
    const revoke = URL.revokeObjectURL as jest.Mock;
    const { result } = renderHook(() => useCampaignKitExport());
    await act(async () => {
      await result.current.exportKit({
        base: BASE,
        formats: ["4:5"],
        caption: "c",
        targetName: "Milanesa",
        origin: "",
        lang: "en",
      });
    });
    expect(mockDownloads).toHaveLength(1);
    expect(mockDownloads[0].name).toBe("milanesa-campaign-kit.zip");
    // Immediate revoke cancels Chrome/Safari downloads of in-memory zips.
    expect(revoke).not.toHaveBeenCalled();
  });

  it("surfaces a thrown download as kit_export_failed instead of rejecting", async () => {
    jest
      .spyOn(HTMLAnchorElement.prototype, "click")
      .mockImplementation(() => {
        throw new Error("download_blocked");
      });
    const { result } = renderHook(() => useCampaignKitExport());
    let ok = true;
    await act(async () => {
      ok = await result.current.exportKit({
        base: BASE,
        formats: ["4:5"],
        caption: "c",
        targetName: "Milanesa",
        origin: "",
        lang: "en",
      });
    });
    expect(ok).toBe(false);
    expect(result.current.error).toBe("kit_export_failed");
    expect(result.current.progress).toBeNull();
  });
});

describe("useCampaignKitExport — narrative pack", () => {
  it("renders narrative roles as their mapped formats and downloads one zip", async () => {
    const { result } = renderHook(() => useCampaignKitExport());
    await act(async () => {
      await result.current.exportNarrativeKit({
        base: BASE,
        caption: "Tonight only",
        captionAngles: [
          { id: "primary", text: "Tonight only" },
          { id: "urgency", text: "Ends at 10" },
        ],
        targetName: "Milanesa",
        origin: "",
        lang: "en",
        readmeText: "Assets ready now — you post when you want.",
        manifestNote: "Assets ready now — you post when you want.",
      });
    });
    // hero, teaser, story, tent(print), email_strip — canvas order excludes tent
    expect(mockRenderOrder).toEqual(["4:5", "1:1", "9:16", "strip"]);
    expect(mockDownloads).toHaveLength(1);
    expect(mockDownloads[0].name).toBe("milanesa-narrative-kit.zip");
  });

  it("exposes roleId on progress during narrative export", async () => {
    const { result } = renderHook(() => useCampaignKitExport());
    let promise!: Promise<boolean>;
    act(() => {
      promise = result.current.exportNarrativeKit({
        base: BASE,
        caption: "c",
        roles: ["hero", "story"] as NarrativeRoleId[],
        targetName: "Milanesa",
        origin: "",
        lang: "en",
      });
    });
    await waitFor(() => expect(result.current.progress).not.toBeNull());
    expect(result.current.progress!.roleId).toBeDefined();
    expect(["hero", "story"]).toContain(result.current.progress!.roleId);
    await act(async () => {
      await promise;
    });
    expect(result.current.progress).toBeNull();
  });

  it("membership helper lists the files a narrative zip should contain", () => {
    // Contract the hook implements: visual roles + caption angles + README +
    // schedule-free manifest. Pinning here keeps UI and zip in lockstep.
    expect(
      narrativeZipMembership({
        targetName: "Milanesa",
        caption: "Tonight only",
        captionAngles: [{ id: "urgency", text: "Ends at 10" }],
      }),
    ).toEqual([
      "milanesa-hero-4x5.png",
      "milanesa-teaser-1x1.png",
      "milanesa-story-9x16.png",
      "milanesa-tent-5x7.html",
      "milanesa-email-strip-strip.png",
      "caption.txt",
      "caption-urgency.txt",
      "README.txt",
      "manifest.json",
    ]);
  });

  it("does not put schedule tokens in the narrative archive name", async () => {
    const { result } = renderHook(() => useCampaignKitExport());
    await act(async () => {
      await result.current.exportNarrativeKit({
        base: BASE,
        caption: "",
        roles: ["hero"],
        targetName: "Dish",
        origin: "",
        lang: "en",
      });
    });
    expect(mockDownloads[0].name).not.toMatch(
      /schedule|queue|due|calendar|week/i,
    );
  });
});

describe("useCampaignKitExport — print shop pack (S3-Reach)", () => {
  it("exports table tent as print-shop zip (HTML path, not canvas)", async () => {
    const { result } = renderHook(() => useCampaignKitExport());
    await act(async () => {
      await result.current.exportPrintShopPack({
        base: BASE,
        presetId: "table_tent_5x7",
        caption: "Tonight",
        targetName: "Milanesa",
        origin: "https://payverge.io",
        lang: "en",
      });
    });
    expect(mockRenderOrder).toEqual([]); // tent is print HTML, not canvas
    expect(mockDownloads).toHaveLength(1);
    expect(mockDownloads[0].name).toBe(
      "milanesa-table-tent-5x7-print-shop.zip",
    );
  });

  it("exports window cling as high-dpi PNG via canvas", async () => {
    const { result } = renderHook(() => useCampaignKitExport());
    await act(async () => {
      await result.current.exportPrintShopPack({
        base: BASE,
        presetId: "window_clings_square",
        caption: "Visit us",
        targetName: "Milanesa",
        origin: "",
        lang: "en",
      });
    });
    expect(mockRenderOrder).toEqual(["1:1"]);
    expect(mockDownloads[0].name).toContain("window-clings-square");
    expect(mockDownloads[0].name).toContain("print-shop");
  });

  it("rejects unknown print presets", async () => {
    const { result } = renderHook(() => useCampaignKitExport());
    let ok = true;
    await act(async () => {
      ok = await result.current.exportPrintShopPack({
        base: BASE,
        // @ts-expect-error intentional invalid preset
        presetId: "billboard",
        caption: "",
        targetName: "x",
        origin: "",
        lang: "en",
      });
    });
    expect(ok).toBe(false);
    expect(result.current.error).toBe("invalid_print_preset");
    expect(mockDownloads).toHaveLength(0);
  });
});
