/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import DesignCustomization from "@/components/business/DesignCustomization";
import type { BusinessDesignSettings } from "@/api/business";

// logError is fire-and-forget (void) and POSTs to the API; unmocked, its
// console fallback lands after this file's teardown and fails the run.
jest.mock("@/utils/errorLogger", () => ({
  logError: jest.fn(),
}));

jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({ showSuccess: jest.fn(), showError: jest.fn() }),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/getTranslation");
  return {
    useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
    getTranslation: actual.getTranslation,
  };
});

const baseSettings: BusinessDesignSettings = {
  primary_color: "#111111",
  secondary_color: "#abcdef",
  font_family: "Serif",
  corner_radius: "medium",
  shadow_intensity: "subtle",
  background_pattern: "none",
  pattern_opacity: 0.2,
  show_images: true,
  show_descriptions: true,
  show_special_features: true,
} as unknown as BusinessDesignSettings;

function renderPreview(overrides: Partial<BusinessDesignSettings> = {}) {
  return render(
    <DesignCustomization
      businessId={42}
      designSettings={{ ...baseSettings, ...overrides }}
      onDesignSettingsChange={jest.fn()}
      customUrl="demo"
    />,
  );
}

/**
 * #600: the preview tree now lives inside the StorefrontPreviewFrame iframe
 * (a real 380px viewport), so preview markup is read from the frame document
 * plus the shared head (styled-jsx theme tags), not from `container`.
 */
function getPreviewFrame(container: HTMLElement): HTMLIFrameElement {
  const frame = container.querySelector<HTMLIFrameElement>(
    '[data-testid="storefront-preview-frame"]',
  );
  if (!frame) throw new Error("storefront preview frame not rendered");
  return frame;
}

function previewHtml(container: HTMLElement): string {
  const doc = getPreviewFrame(container).contentDocument;
  const headStyles = Array.from(document.querySelectorAll("head style"))
    .map((el) => el.outerHTML)
    .join("\n");
  return `${doc?.documentElement.innerHTML || ""}\n${headStyles}`;
}

describe("DesignCustomization — preview accuracy", () => {
  it("maps the font_family enum to the real CSS variable, not the raw enum string", async () => {
    const { container } = renderPreview({ font_family: "Serif" });
    await waitFor(() => {
      const html = previewHtml(container);
      // Serif must resolve to the live page's --font-title variable.
      expect(html).toContain("font-title");
      // The raw enum string must never be used as a fontFamily value.
      expect(html).not.toContain("font-family: Serif");
    });
  });

  it("maps the Sans/Inter enum to the DM Sans CSS variable", async () => {
    const { container } = renderPreview({ font_family: "Inter" });
    await waitFor(() => {
      expect(previewHtml(container)).toContain("font-sans");
    });
  });

  it("applies the secondary_color somewhere in the preview", async () => {
    const { container } = renderPreview({ secondary_color: "#abcdef" });
    // secondary color is now used for the item price / add-to-order accent.
    await waitFor(() => {
      expect(previewHtml(container).toLowerCase()).toContain("#abcdef");
    });
  });

  it("renders the preview inside a real 380px iframe viewport (#600)", async () => {
    const { container } = renderPreview();
    const frame = getPreviewFrame(container);
    // A genuine viewport: CSS media queries inside the frame resolve against
    // 380px, so the phone frame renders mobile breakpoints for real.
    expect(frame.style.width).toBe("380px");
    expect(frame.getAttribute("src")).toBeNull();
    expect(frame).toHaveAttribute("aria-hidden", "true");
    expect(frame.getAttribute("title")).toBeTruthy();
    // The #591 storefront tree — data-storefront-preview hook included —
    // mounts INSIDE the frame document, not in the dashboard document.
    await waitFor(() => {
      expect(
        frame.contentDocument?.querySelector(
          '[data-storefront-preview="true"]',
        ),
      ).not.toBeNull();
    });
    expect(
      container.querySelector('[data-storefront-preview="true"]'),
    ).toBeNull();
  });

  it("does not render an open-page button that falls back to the dashboard", () => {
    const { container } = render(
      <DesignCustomization
        businessId={1}
        designSettings={{} as any}
        onDesignSettingsChange={() => {}}
        customUrl=""
      />,
    );
    // When no customUrl is set the old button pointed href to /business/1/dashboard.
    // That fallback must no longer exist in the rendered output.
    expect(container.innerHTML).not.toContain("/business/1/dashboard");
    // The open-page button itself should also be gone.
    const openPageBtn = screen.queryByRole("button", { name: /open.*(live )?page/i });
    expect(openPageBtn).toBeNull();
  });
});
