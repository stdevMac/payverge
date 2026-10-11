/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { PhotoReadinessPanel } from "./PhotoReadinessPanel";
import * as businessApi from "@/api/business";
import * as photoCache from "./photo/cache";
import type { Business } from "@/api/business";
import type { PhotoAnalysis } from "./photo/types";

jest.mock("@/api/business", () => ({
  ...jest.requireActual("@/api/business"),
  getMenu: jest.fn(),
}));
jest.mock("@/utils/errorLogger", () => ({ logError: jest.fn() }));

const business = { id: 42, name: "Casa Sur" } as unknown as Business;
const t = (key: string, params?: Record<string, string | number>) =>
  params ? `${key}:${Object.values(params).join(",")}` : key;

function analysis(blurScore: number): PhotoAnalysis {
  return {
    luma: { size: 1, values: [128] },
    edges: { size: 1, values: [0] },
    blurScore,
    exposure: {
      histogram: new Array(16).fill(0),
      meanLuma: 128,
      shadowClipping: 0,
      highlightClipping: 0,
      channelMeans: { r: 128, g: 128, b: 128 },
    },
    subject: { x: 0, y: 0, w: 1, h: 1 },
    focal: { x: 0.5, y: 0.5 },
    negativeSpace: "bottom",
    busy: false,
    dominantColors: [],
  };
}

beforeEach(() => {
  (global as unknown as { Image: unknown }).Image = class {
    crossOrigin = "";
    naturalWidth = 800;
    naturalHeight = 600;
    onload: (() => void) | null = null;
    onerror: (() => void) | null = null;
    set src(_value: string) {
      setTimeout(() => this.onload?.(), 0);
    }
  };
});

afterEach(() => {
  jest.restoreAllMocks();
  jest.clearAllMocks();
});

describe("PhotoReadinessPanel", () => {
  it("scores every menu photo and summarizes them", async () => {
    (businessApi.getMenu as jest.Mock).mockResolvedValue({
      categories: [
        {
          name: "Mains",
          items: [
            {
              name: "Milanesa",
              image: "https://cdn/a.png",
              is_available: true,
            },
            {
              name: "Ñoquis",
              image: "https://cdn/b.png",
              is_available: true,
            },
          ],
        },
      ],
    });
    jest
      .spyOn(photoCache, "photoAnalysisFor")
      .mockImplementation((url) =>
        url.endsWith("a.png") ? analysis(500) : analysis(20),
      );

    render(<PhotoReadinessPanel business={business} t={t} />);

    await waitFor(() =>
      expect(
        screen.getByText("photoReadiness.summary:1,2"),
      ).toBeInTheDocument(),
    );
    expect(screen.getByText("Milanesa")).toBeInTheDocument();
    expect(
      screen.getByText("photoReadiness.verdicts.ready"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("photoReadiness.verdicts.reshoot"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("photoReadiness.reasons.verySoft"),
    ).toBeInTheDocument();
    expect(screen.getAllByTestId("photo-readiness-explain").length).toBe(2);
    expect(
      screen.getByText("photoReadiness.explain.ready"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("photoReadiness.explain.reshoot"),
    ).toBeInTheDocument();
  });

  it("shows the empty state when no menu item has a photo", async () => {
    (businessApi.getMenu as jest.Mock).mockResolvedValue({
      categories: [{ name: "Mains", items: [{ name: "Sopa" }] }],
    });
    render(<PhotoReadinessPanel business={business} t={t} />);
    await waitFor(() =>
      expect(screen.getByText("photoReadiness.empty")).toBeInTheDocument(),
    );
  });

  it("offers a retry when the menu cannot be loaded", async () => {
    (businessApi.getMenu as jest.Mock).mockRejectedValue(new Error("boom"));
    render(<PhotoReadinessPanel business={business} t={t} />);
    await waitFor(() =>
      expect(screen.getByText("photoReadiness.failed")).toBeInTheDocument(),
    );
    expect(
      screen.getByRole("button", { name: "photoReadiness.retry" }),
    ).toBeInTheDocument();
  });

  // A closed CORS gate is the single most likely cause of a library that cannot
  // be read, and it must show as a problem rather than as a clean bill.
  it("explains a too-dark usable grade instead of leaving a bare QA badge", async () => {
    (businessApi.getMenu as jest.Mock).mockResolvedValue({
      categories: [
        {
          name: "Drinks",
          items: [{ name: "Iced Tea", image: "https://cdn/tea.png" }],
        },
      ],
    });
    jest.spyOn(photoCache, "photoAnalysisFor").mockReturnValue({
      ...analysis(400),
      exposure: {
        histogram: new Array(16).fill(0),
        meanLuma: 20,
        shadowClipping: 0.4,
        highlightClipping: 0,
        channelMeans: { r: 20, g: 20, b: 20 },
      },
    });
    render(<PhotoReadinessPanel business={business} t={t} />);
    await waitFor(() =>
      expect(
        screen.getByTestId("photo-readiness-explain"),
      ).toBeInTheDocument(),
    );
    expect(screen.getByTestId("photo-readiness-explain").textContent).toMatch(
      /photoReadiness\.explain\.(weak|reshoot)/,
    );
    expect(screen.getByTestId("photo-readiness-explain").textContent).toMatch(
      /photoReadiness\.explain\.underexposed/,
    );
  });

  it("reports an unreadable photo instead of certifying it", async () => {
    (businessApi.getMenu as jest.Mock).mockResolvedValue({
      categories: [
        {
          name: "Mains",
          items: [{ name: "Milanesa", image: "https://cdn/a.png" }],
        },
      ],
    });
    jest.spyOn(photoCache, "photoAnalysisFor").mockReturnValue(null);
    render(<PhotoReadinessPanel business={business} t={t} />);
    await waitFor(() =>
      expect(
        screen.getByText("photoReadiness.reasons.unreadable"),
      ).toBeInTheDocument(),
    );
    expect(
      screen.getByText("photoReadiness.summary:0,1"),
    ).toBeInTheDocument();
  });
});
