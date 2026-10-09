import { MOTIF_DRAWERS } from "../scene/motifs";
import {
  logoNode,
  motifNode,
  photoNode,
  scrimNode,
  shapeNode,
  sortNodes,
  textNode,
  type Scene,
  type SceneNode,
} from "../scene/types";
import { FORMATS } from "./formats";
import {
  DROPPED_MOTIFS,
  PRINTABLE_MOTIFS,
  guestDeepLinkHtml,
  renderScenePrintHtml,
} from "./printWalker";

const ORIGIN = "https://payverge.io";

function emptyScene(): Scene {
  return {
    width: FORMATS["5:7"].px.w,
    height: FORMATS["5:7"].px.h,
    background: "#1a6b6a",
    nodes: [],
  };
}

function sceneWith(nodes: SceneNode[]): Scene {
  return {
    width: FORMATS["5:7"].px.w,
    height: FORMATS["5:7"].px.h,
    background: "#1a6b6a",
    nodes: sortNodes(nodes),
  };
}

describe("renderScenePrintHtml — document shape", () => {
  const html = () =>
    renderScenePrintHtml(emptyScene(), FORMATS["5:7"], {
      origin: ORIGIN,
      title: "Milanesa night",
      lang: "en",
    });

  it("starts with a doctype", () => {
    expect(html().trimStart().toLowerCase().startsWith("<!doctype html>")).toBe(
      true,
    );
  });

  it("carries the language on <html> and an escaped title", () => {
    expect(html()).toContain('<html lang="en">');
    expect(html()).toContain("<title>Milanesa night</title>");
  });

  it("escapes markup in the title", () => {
    const out = renderScenePrintHtml(emptyScene(), FORMATS["5:7"], {
      origin: ORIGIN,
      title: "<script>alert(1)</script>",
      lang: "en",
    });
    expect(out).not.toContain("<script>alert");
    expect(out).toContain("&lt;script&gt;");
  });

  // Chromium falls back to A4 when a fixed-format @page carries `auto`, so both
  // lengths must be explicit numbers.
  it("declares @page with two explicit millimetre lengths", () => {
    expect(html()).toContain("size: 143.35mm 194.15mm;");
    expect(html()).toContain("margin: 0;");
    expect(html()).not.toMatch(/size:[^;]*auto/);
  });

  // @page margins do not apply on screen, so the preview would not match the
  // print without this mirror. Paid for once already in menuPrint/renderHtml.ts.
  it("mirrors the page box for on-screen preview", () => {
    const out = html();
    expect(out).toContain("@media screen {");
    expect(out).toMatch(/@media screen \{[\s\S]*width: 143\.35mm;/);
    expect(out).toMatch(/@media screen \{[\s\S]*height: 194\.15mm;/);
  });

  it("inlines @font-face for both marketing faces, from the given origin", () => {
    const out = html();
    expect(out).toContain("@font-face");
    expect(out).toContain('font-family: "DM Sans"');
    expect(out).toContain('font-family: "DM Serif Display"');
    expect(out).toContain(`${ORIGIN}/fonts/dm-sans/dm-sans-latin.woff2`);
    expect(out).toContain(`${ORIGIN}/fonts/dm-sans/dm-sans-latin-ext.woff2`);
  });

  // The DM faces are Google CSS-API subsets, not full builds. That is safe here
  // ONLY because nothing in the marketing scene IR asks for an OpenType feature:
  // there is no small-caps and no oldstyle-figure request anywhere in it. If one
  // ever appears, the face has to become a full build first.
  it("asks for no OpenType feature a subset build would silently drop", () => {
    const out = html();
    expect(out).not.toContain("font-variant-caps");
    expect(out).not.toContain("font-variant-numeric");
    expect(out).not.toContain("font-feature-settings");
  });

  it("paints the scene background on the bleed box", () => {
    expect(html()).toContain("background: #1a6b6a;");
  });

  it("emits eight crop marks", () => {
    const out = html();
    expect(out.match(/class="pv-mark"/g) ?? []).toHaveLength(8);
  });

  it("forces exact colour reproduction", () => {
    const out = html();
    expect(out).toContain("-webkit-print-color-adjust: exact;");
    expect(out).toContain("print-color-adjust: exact;");
  });

  it("refuses a screen format", () => {
    expect(() =>
      renderScenePrintHtml(emptyScene(), FORMATS["4:5"], {
        origin: ORIGIN,
        title: "x",
        lang: "en",
      }),
    ).toThrow("not_a_print_format");
  });
});

describe("renderScenePrintHtml — nodes", () => {
  const render = (nodes: SceneNode[]) =>
    renderScenePrintHtml(sceneWith(nodes), FORMATS["5:7"], {
      origin: ORIGIN,
      title: "t",
      lang: "en",
    });

  it("emits a photo as a cover-fitted img positioned from its crop", () => {
    const out = render([
      photoNode({
        url: "https://cdn.example.com/dish.jpg",
        rect: { x: 0, y: 0, w: 1, h: 1 },
        crop: { x: 0.25, y: 0.75, zoom: 1 },
        filter: "saturate(1.04) contrast(1.03)",
        z: 0,
      }),
    ]);
    expect(out).toContain('src="https://cdn.example.com/dish.jpg"');
    expect(out).toContain("object-fit:cover");
    expect(out).toContain("object-position:25% 75%");
    expect(out).toContain("filter:saturate(1.04) contrast(1.03)");
    // Full bleed: the bleed box, offset by the mark gutter.
    expect(out).toContain("left:5mm");
    expect(out).toContain("width:133.35mm");
  });

  it("paints Wave 2 white-balance as a multiply layer over the photo", () => {
    const out = render([
      photoNode({
        url: "https://cdn.example.com/dish.jpg",
        rect: { x: 0, y: 0, w: 1, h: 1 },
        filter: "brightness(1.05)",
        whiteBalance: "rgb(255, 240, 230)",
        z: 0,
      }),
    ]);
    expect(out).toContain("filter:brightness(1.05)");
    expect(out).toContain("background:rgb(255, 240, 230)");
    expect(out).toContain("mix-blend-mode:multiply");
  });

  it("escapes a photo url so it cannot break out of the attribute", () => {
    const out = render([
      photoNode({
        url: 'https://cdn.example.com/a.jpg" onerror="alert(1)',
        rect: { x: 0, y: 0, w: 1, h: 1 },
        z: 0,
      }),
    ]);
    // escapeHtml (menuPrint) turns " into &quot; so the injected quote cannot
    // close the src attribute. The literal characters `onerror=` remain inside
    // the attribute value (safe); an unescaped attribute break would not.
    expect(out).toContain("&quot;");
    expect(out).not.toMatch(/src="[^"]*"[^>]*onerror=/);
  });

  it("emits a scrim as a linear gradient over its own rect", () => {
    const out = render([
      scrimNode({
        rect: { x: 0, y: 0.4, w: 1, h: 0.6 },
        from: "rgba(28,25,23,0)",
        to: "rgba(28,25,23,0.78)",
        adaptive: true,
        luminanceThreshold: 155,
        z: 10,
      }),
    ]);
    expect(out).toContain(
      "background:linear-gradient(to bottom, rgba(28,25,23,0), rgba(28,25,23,0.78))",
    );
  });

  it("emits text with the resolved face, size in points, and colour", () => {
    const out = render([
      textNode({
        key: "dishName",
        text: "Milanesa napolitana",
        rect: { x: 0.08, y: 0.6, w: 0.84, h: 0.12 },
        z: 30,
        font: "serif",
        weight: 700,
        sizePct: 0.075,
        align: "left",
        color: "#ffffff",
        maxLines: 2,
        lineHeight: 1.05,
        tracking: 0.04,
      }),
    ]);
    expect(out).toContain("Milanesa napolitana");
    expect(out).toContain('font-family:"DM Serif Display"');
    // 0.075 * 1575 px = 118.125 px; at 300 dpi that is 28.35 pt.
    expect(out).toContain("font-size:28.35pt");
    expect(out).toContain("color:#ffffff");
    expect(out).toContain("line-height:1.05");
    expect(out).toContain("letter-spacing:0.04em");
  });

  it("uses the sans face and normal weight for supporting slots", () => {
    const out = render([
      textNode({
        key: "handle",
        text: "@casasur",
        rect: { x: 0.08, y: 0.8, w: 0.84, h: 0.04 },
        z: 30,
        font: "sans",
        weight: 500,
        sizePct: 0.022,
        align: "right",
        color: "#e4e0d8",
        maxLines: 1,
        lineHeight: 1.1,
      }),
    ]);
    expect(out).toContain('font-family:"DM Sans"');
    expect(out).toContain("font-weight:500");
    expect(out).toContain("text-align:right");
  });

  it("uppercases where the node says to, in CSS rather than in the text", () => {
    const out = render([
      textNode({
        key: "badge",
        text: "chef's pick",
        rect: { x: 0.08, y: 0.5, w: 0.4, h: 0.05 },
        z: 30,
        font: "sans",
        weight: 500,
        sizePct: 0.026,
        align: "left",
        color: "#1c1917",
        maxLines: 1,
        lineHeight: 1.1,
        uppercase: true,
      }),
    ]);
    expect(out).toContain("text-transform:uppercase");
    // The original casing survives in the markup, so a copy/paste out of the
    // saved PDF gets the operator's words, not a shouted version of them.
    // escapeHtml does not entity-encode apostrophes (only & < > "); that is the
    // shared menuPrint contract and is fine inside a text node.
    expect(out).toContain("chef's pick");
  });

  it("clamps text to its declared line count", () => {
    const out = render([
      textNode({
        key: "dishName",
        text: "A very long dish name that would wrap several times over",
        rect: { x: 0.08, y: 0.6, w: 0.3, h: 0.12 },
        z: 30,
        font: "serif",
        weight: 700,
        sizePct: 0.075,
        align: "left",
        color: "#ffffff",
        maxLines: 2,
        lineHeight: 1.05,
      }),
    ]);
    expect(out).toContain("-webkit-line-clamp:2");
    expect(out).toContain("overflow:hidden");
  });

  it("escapes text content", () => {
    const out = render([
      textNode({
        key: "dishName",
        text: "<img src=x onerror=alert(1)>",
        rect: { x: 0.08, y: 0.6, w: 0.84, h: 0.12 },
        z: 30,
        font: "sans",
        weight: 700,
        sizePct: 0.05,
        align: "left",
        color: "#ffffff",
        maxLines: 1,
        lineHeight: 1.1,
      }),
    ]);
    expect(out).not.toMatch(/<img /);
    expect(out).toContain("&lt;img");
  });

  it("keeps nodes in the order the scene gives them", () => {
    const out = render([
      textNode({
        key: "dishName",
        text: "ZZTOP",
        rect: { x: 0, y: 0.5, w: 1, h: 0.1 },
        z: 30,
        font: "sans",
        weight: 700,
        sizePct: 0.05,
        align: "left",
        color: "#fff",
        maxLines: 1,
        lineHeight: 1.1,
      }),
      photoNode({
        url: "https://cdn.example.com/a.jpg",
        rect: { x: 0, y: 0, w: 1, h: 1 },
        z: 0,
      }),
    ]);
    // The scene is pre-sorted by z, and DOM order IS paint order for absolutely
    // positioned siblings without z-index, so the photo must come first.
    expect(out.indexOf("cdn.example.com/a.jpg")).toBeLessThan(out.indexOf("ZZTOP"));
  });
});

describe("renderScenePrintHtml — shapes, motifs and the mark", () => {
  const render = (nodes: SceneNode[]) =>
    renderScenePrintHtml(sceneWith(nodes), FORMATS["5:7"], {
      origin: ORIGIN,
      title: "t",
      lang: "en",
    });

  it("emits a shape as a filled box with a width-relative radius", () => {
    const out = render([
      shapeNode({
        rect: { x: 0.1, y: 0.1, w: 0.5, h: 0.2 },
        fill: "#1a6b6a",
        radiusPct: 0.02,
        z: 20,
      }),
    ]);
    expect(out).toContain("background:#1a6b6a");
    // 0.02 * 1575px = 31.5px; at 300 dpi that is 2.667mm.
    expect(out).toContain("border-radius:2.667mm");
  });

  it("emits the mark as a circular clipped image", () => {
    const out = render([
      logoNode({
        url: "https://cdn.example.com/logo.png",
        // LogoRect is origin + diameter only; no h.
        rect: { x: 0.08, y: 0.06, w: 0.11 },
        z: 50,
      }),
    ]);
    expect(out).toContain('src="https://cdn.example.com/logo.png"');
    expect(out).toContain("border-radius:50%");
    expect(out).toContain("object-fit:cover");
    // Square, sized off canvas WIDTH — 0.11 * 1575px → 14.6685mm at 300 dpi.
    expect(out).toMatch(/width:14\.6685mm;height:14\.6685mm/);
  });

  it("draws the two motifs it can express and drops the rest", () => {
    const printable = render([
      motifNode({
        motif: "rule",
        rect: { x: 0.08, y: 0.5, w: 0.84, h: 0.3 },
        color: "#c9a227",
        opacity: 0.9,
        z: 40,
      }),
    ]);
    expect(printable).toContain("pv-motif-rule");

    const brackets = render([
      motifNode({
        motif: "cornerBrackets",
        rect: { x: 0.08, y: 0.5, w: 0.84, h: 0.3 },
        color: "#c9a227",
        opacity: 0.9,
        z: 40,
      }),
    ]);
    expect(brackets.match(/pv-motif-bracket/g) ?? []).toHaveLength(4);

    (["seal", "ticketNotch", "halftone", "grain", "tape"] as const).forEach(
      (motif) => {
        const out = render([
          motifNode({
            motif,
            rect: { x: 0, y: 0, w: 1, h: 1 },
            color: "#c9a227",
            opacity: 0.9,
            z: 40,
          }),
        ]);
        expect(out).not.toContain("pv-motif");
      },
    );
  });

  // The drop list is a decision, not an oversight, and it has to stay honest as
  // MOTIF_DRAWERS grows: a motif added to the canvas walker and silently absent
  // from print is a tent that does not match its posts.
  it("accounts for every motif the canvas walker can draw", () => {
    const known = Object.keys(MOTIF_DRAWERS).sort();
    const accounted = [...PRINTABLE_MOTIFS, ...DROPPED_MOTIFS].sort();
    expect(accounted).toEqual(known);
  });
});

describe("guestDeepLinkHtml (S2-D)", () => {
  it("returns empty when url missing", () => {
    expect(guestDeepLinkHtml(undefined)).toBe("");
    expect(guestDeepLinkHtml("")).toBe("");
  });

  it("rejects non-http and signed private URLs", () => {
    expect(guestDeepLinkHtml("javascript:alert(1)")).toBe("");
    expect(
      guestDeepLinkHtml("https://cdn.example.com/x?X-Amz-Signature=abc"),
    ).toBe("");
  });

  it("renders link and optional QR data URL", () => {
    const html = guestDeepLinkHtml(
      "https://payverge.io/b/bistro?tab=menu",
      "data:image/png;base64,abc",
    );
    expect(html).toContain("pv-guest");
    expect(html).toContain("https://payverge.io/b/bistro?tab=menu");
    expect(html).toContain('src="data:image/png;base64,abc"');
  });

  it("renders promo code under the QR when provided", () => {
    const html = guestDeepLinkHtml(
      "https://payverge.io/b/bistro?pv_ref=PV-HH-ABCD",
      "data:image/png;base64,abc",
      "PV-HH-ABCD",
    );
    expect(html).toContain("pv-promo");
    expect(html).toContain("PV-HH-ABCD");
  });

  it("allows promo-only badge without URL", () => {
    const html = guestDeepLinkHtml(undefined, undefined, "PV-FD-1111");
    expect(html).toContain("PV-FD-1111");
    expect(html).not.toContain("<a ");
  });

  it("escapes markup in guest URL", () => {
    const html = guestDeepLinkHtml('https://payverge.io/b/x?q="<script>"');
    expect(html).not.toContain("<script>");
    expect(html).toContain("&lt;");
  });
});
