import { KITS } from "../artDirection/kits";
import { harmonizePalette } from "../artDirection/palette";
import { COMPOSITIONS } from "../composition/compositions";
import type { MeasureFn } from "../composition/solver";
import { buildScene } from "../scene/buildScene";
import { FORMATS } from "./formats";
import { renderScenePrintHtml } from "./printWalker";

/**
 * The join this whole wave exists to make: ONE buildScene call, consumed by the
 * print walker instead of the canvas walker, with no format-specific branch
 * anywhere between the concept and the page.
 */
const measure: MeasureFn = (text, cssFont) => {
  const px = Number.parseFloat(cssFont.match(/([\d.]+)px/)?.[1] ?? "20");
  return text.length * px * 0.52;
};

function tentHtml(): string {
  const format = FORMATS["5:7"];
  const scene = buildScene({
    kit: KITS.editorial,
    composition: COMPOSITIONS.photoBottomStack,
    format,
    canvasW: format.px.w,
    canvasH: format.px.h,
    photoUrl: "https://cdn.example.com/milanesa.jpg",
    logoUrl: "https://cdn.example.com/logo.png",
    crop: { x: 0.5, y: 0.4, zoom: 1 },
    slots: {
      badge: "chef's pick",
      dishName: "Milanesa napolitana",
      price: "$12.00",
      cta: "Order tonight",
      handle: "@casasur",
    },
    palette: harmonizePalette({ primary: "#1a6b6a", secondary: "#0f3d3c" }, []),
    measure,
  });
  return renderScenePrintHtml(scene, format, {
    origin: "https://payverge.io",
    title: "Milanesa napolitana",
    lang: "en",
  });
}

describe("a table tent, end to end", () => {
  it("carries every filled slot onto the page", () => {
    const out = tentHtml();
    // Apostrophe is not entity-encoded by shared escapeHtml — only & < > ".
    expect(out).toContain("chef's pick");
    expect(out).toContain("Milanesa napolitana");
    expect(out).toContain("$12.00");
    expect(out).toContain("Order tonight");
    expect(out).toContain("@casasur");
  });

  it("carries the photo, the mark and the kit's grade", () => {
    const out = tentHtml();
    expect(out).toContain("cdn.example.com/milanesa.jpg");
    expect(out).toContain("cdn.example.com/logo.png");
    expect(out).toContain("filter:saturate(1.04) contrast(1.03)");
  });

  it("keeps the page fixed at the tent's size with marks", () => {
    const out = tentHtml();
    expect(out).toContain("size: 143.35mm 194.15mm;");
    expect(out.match(/class="pv-mark"/g) ?? []).toHaveLength(8);
  });

  it("never lets a node escape the artwork box", () => {
    const format = FORMATS["5:7"];
    const scene = buildScene({
      kit: KITS.editorial,
      composition: COMPOSITIONS.photoBottomStack,
      format,
      canvasW: format.px.w,
      canvasH: format.px.h,
      photoUrl: "https://cdn.example.com/milanesa.jpg",
      logoUrl: "",
      slots: { dishName: "Milanesa napolitana" },
      palette: harmonizePalette({ primary: "#1a6b6a", secondary: "#0f3d3c" }, []),
      measure,
    });
    scene.nodes.forEach((node) => {
      expect(node.rect.x).toBeGreaterThanOrEqual(-1e-9);
      expect(node.rect.y).toBeGreaterThanOrEqual(-1e-9);
      expect(node.rect.x + node.rect.w).toBeLessThanOrEqual(1 + 1e-9);
      // LogoRect has no h — diameter is width-relative on both axes.
      const h = "h" in node.rect ? node.rect.h : node.rect.w * (format.px.w / format.px.h);
      expect(node.rect.y + h).toBeLessThanOrEqual(1 + 1e-9);
    });
  });

  it("produces a document that parses as one element tree", () => {
    // Cheap structural sanity: every div/p/img this walker opens is closed or
    // self-closing, so `doc.write` in the print iframe cannot land in an
    // unbalanced state.
    const out = tentHtml();
    const opens = (out.match(/<div\b/g) ?? []).length;
    const closes = (out.match(/<\/div>/g) ?? []).length;
    expect(opens).toBe(closes);
    expect((out.match(/<p\b/g) ?? []).length).toBe(
      (out.match(/<\/p>/g) ?? []).length,
    );
    expect(out.match(/<img\b[^>]*[^/]>/g)).toBeNull();
  });
});
