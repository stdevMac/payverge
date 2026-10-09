/** @jest-environment jsdom */
import {
  formatControlName,
  labelledByTargetsHaveText,
} from "./namedControl";

describe("formatControlName (#446)", () => {
  it("joins purpose and current value", () => {
    expect(formatControlName("Font Family", "Sans (DM Sans)")).toBe(
      "Font Family, Sans (DM Sans)",
    );
    expect(formatControlName("Menu layout", "Grid")).toBe("Menu layout, Grid");
    expect(formatControlName("Auto-assign tables", "on")).toBe(
      "Auto-assign tables, on",
    );
    expect(formatControlName("Show Item Images", "off")).toBe(
      "Show Item Images, off",
    );
  });

  it("does not duplicate an identical value", () => {
    expect(formatControlName("Grid", "Grid")).toBe("Grid");
    expect(formatControlName("  Font Family  ", "")).toBe("Font Family");
  });
});

describe("labelledByTargetsHaveText", () => {
  it("rejects empty generated labelledby targets", () => {
    document.body.innerHTML = `<span id="empty-label"></span><button aria-labelledby="empty-label">x</button>`;
    const el = document.querySelector("button") as HTMLElement;
    expect(labelledByTargetsHaveText(el)).toBe(false);
  });

  it("accepts a labelledby target that has visible text", () => {
    document.body.innerHTML = `<span id="title">Low stock warnings</span><button aria-labelledby="title">x</button>`;
    const el = document.querySelector("button") as HTMLElement;
    expect(labelledByTargetsHaveText(el)).toBe(true);
  });
});
