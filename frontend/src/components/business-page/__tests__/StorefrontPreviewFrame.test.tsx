/** @jest-environment jsdom */
/**
 * #600 — the Business Page live preview must render at a REAL 380px viewport
 * so CSS media queries (Tailwind md:/lg:, raw @media) resolve against the
 * phone frame, not the operator's desktop browser window.
 *
 * StorefrontPreviewFrame does this with a same-origin about:blank iframe and
 * a React portal: the children stay in the parent React tree (live prop
 * updates, operator session, isolated preview QueryClient all unchanged)
 * while their DOM lives in the iframe document. These tests pin down the
 * mechanism: the iframe viewport, the portal, and the head-style mirroring
 * that makes Tailwind's breakpointed stylesheets exist inside the frame.
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import StorefrontPreviewFrame from "../StorefrontPreviewFrame";

function getFrame(): HTMLIFrameElement {
  return screen.getByTestId("storefront-preview-frame") as HTMLIFrameElement;
}

function frameDoc(): Document {
  const doc = getFrame().contentDocument;
  if (!doc) throw new Error("iframe has no contentDocument");
  return doc;
}

afterEach(() => {
  // Tests inject probe styles into the shared jsdom head — clean them up.
  document.head
    .querySelectorAll("[data-test-probe]")
    .forEach((node) => node.remove());
  document.documentElement.className = "";
  document.body.className = "";
});

describe("StorefrontPreviewFrame — real mobile viewport (#600)", () => {
  it("renders a same-origin iframe sized to the 380px phone viewport", () => {
    render(
      <StorefrontPreviewFrame title="Live Preview">
        <div>child</div>
      </StorefrontPreviewFrame>,
    );

    const frame = getFrame();
    expect(frame.tagName).toBe("IFRAME");
    // A real viewport: media queries inside resolve against 380px.
    expect(frame.style.width).toBe("380px");
    expect(frame.style.height).toBe("700px");
    // Same-origin about:blank — populated via portal, so there is no preview
    // URL that could be reached unauthenticated.
    expect(frame.getAttribute("src")).toBeNull();
    expect(frame.hasAttribute("sandbox")).toBe(false);
    // Decorative preview treatment (#591 host contract, ported to the frame).
    expect(frame).toHaveAttribute("title", "Live Preview");
    expect(frame).toHaveAttribute("aria-hidden", "true");
    expect(frame).toHaveAttribute("tabindex", "-1");
  });

  it("honors a custom viewport width", () => {
    render(
      <StorefrontPreviewFrame title="Live Preview" width={414} height={800}>
        <div>child</div>
      </StorefrontPreviewFrame>,
    );
    const frame = getFrame();
    expect(frame.style.width).toBe("414px");
    expect(frame.style.height).toBe("800px");
  });

  it("portals its children into the iframe document, not the host document", async () => {
    const { container } = render(
      <StorefrontPreviewFrame title="Live Preview">
        <div data-testid="preview-probe">storefront tree</div>
      </StorefrontPreviewFrame>,
    );

    await waitFor(() => {
      expect(
        frameDoc().body.querySelector('[data-testid="preview-probe"]'),
      ).not.toBeNull();
    });
    // The child must NOT also render in the parent document (that was the
    // pre-#600 behavior where desktop media queries applied).
    expect(
      container.querySelector('[data-testid="preview-probe"]'),
    ).toBeNull();
  });

  it("keeps portaled children updating live on re-render", async () => {
    const { rerender } = render(
      <StorefrontPreviewFrame title="Live Preview">
        <div data-testid="preview-probe">first draft</div>
      </StorefrontPreviewFrame>,
    );
    await waitFor(() => {
      expect(
        frameDoc().body.querySelector('[data-testid="preview-probe"]')
          ?.textContent,
      ).toBe("first draft");
    });

    rerender(
      <StorefrontPreviewFrame title="Live Preview">
        <div data-testid="preview-probe">edited draft</div>
      </StorefrontPreviewFrame>,
    );
    await waitFor(() => {
      expect(
        frameDoc().body.querySelector('[data-testid="preview-probe"]')
          ?.textContent,
      ).toBe("edited draft");
    });
  });

  it("copies the parent head stylesheets into the frame so breakpointed CSS exists there", async () => {
    const style = document.createElement("style");
    style.setAttribute("data-test-probe", "true");
    style.textContent =
      "@media (min-width: 768px) { .probe-md { display: none; } }";
    document.head.appendChild(style);

    render(
      <StorefrontPreviewFrame title="Live Preview">
        <div>child</div>
      </StorefrontPreviewFrame>,
    );

    await waitFor(() => {
      const copied = Array.from(frameDoc().head.querySelectorAll("style"))
        .map((el) => el.textContent || "")
        .join("\n");
      expect(copied).toContain(".probe-md");
    });
  });

  it("mirrors later parent-head style changes into the frame (live theme edits)", async () => {
    const themeTag = document.createElement("style");
    themeTag.setAttribute("data-test-probe", "true");
    themeTag.textContent = ".storefront-theme{--primary:#111111;}";
    document.head.appendChild(themeTag);

    render(
      <StorefrontPreviewFrame title="Live Preview">
        <div>child</div>
      </StorefrontPreviewFrame>,
    );
    await waitFor(() => {
      expect(frameHeadCss()).toContain("--primary:#111111");
    });

    // styled-jsx rewrites the theme tag in place on each brand-color edit —
    // the MutationObserver must propagate that into the frame.
    themeTag.textContent = ".storefront-theme{--primary:#ff5722;}";
    await waitFor(() => {
      expect(frameHeadCss()).toContain("--primary:#ff5722");
    });

    // And appending an entirely new tag (lazy chunk CSS) syncs too.
    const lateTag = document.createElement("style");
    lateTag.setAttribute("data-test-probe", "true");
    lateTag.textContent = ".late-chunk-css{color:red;}";
    document.head.appendChild(lateTag);
    await waitFor(() => {
      expect(frameHeadCss()).toContain(".late-chunk-css");
    });

    function frameHeadCss(): string {
      return Array.from(frameDoc().head.querySelectorAll("style"))
        .map((el) => el.textContent || "")
        .join("\n");
    }
  });

  it("mirrors the parent html/body classes so next/font variables resolve in the frame", async () => {
    document.documentElement.className = "__font_probe_variable";
    document.body.className = "antialiased-probe";

    render(
      <StorefrontPreviewFrame title="Live Preview">
        <div>child</div>
      </StorefrontPreviewFrame>,
    );

    await waitFor(() => {
      expect(frameDoc().documentElement.className).toBe(
        "__font_probe_variable",
      );
      expect(frameDoc().body.className).toBe("antialiased-probe");
    });
  });

  it("resets the frame body and hides scrollbars via the base stylesheet", async () => {
    render(
      <StorefrontPreviewFrame title="Live Preview">
        <div>child</div>
      </StorefrontPreviewFrame>,
    );
    await waitFor(() => {
      const base = frameDoc().head.querySelector(
        "[data-storefront-preview-base]",
      );
      expect(base).not.toBeNull();
      expect(base!.textContent).toContain("margin:0");
      expect(base!.textContent).toContain("scrollbar-width:none");
    });
  });
});
