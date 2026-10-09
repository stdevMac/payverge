/** @jest-environment jsdom */

import {
  editorModeStorageKey,
  isEditorMode,
  parseEditorMode,
  readEditorMode,
  writeEditorMode,
} from "./editorMode";

describe("editorMode", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("accepts only simple and craft", () => {
    expect(isEditorMode("simple")).toBe(true);
    expect(isEditorMode("craft")).toBe(true);
    expect(isEditorMode("advanced")).toBe(false);
    expect(isEditorMode("")).toBe(false);
    expect(isEditorMode(null)).toBe(false);
  });

  it("defaults unknown and empty values to simple", () => {
    expect(parseEditorMode(null)).toBe("simple");
    expect(parseEditorMode(undefined)).toBe("simple");
    expect(parseEditorMode("")).toBe("simple");
    expect(parseEditorMode("CANVA")).toBe("simple");
    expect(parseEditorMode(" Craft ")).toBe("craft");
  });

  it("keys storage by business id", () => {
    expect(editorModeStorageKey(42)).toBe(
      "payverge:marketing:editorMode:42",
    );
    expect(editorModeStorageKey("99")).toBe(
      "payverge:marketing:editorMode:99",
    );
  });

  it("round-trips preference per business without clobbering others", () => {
    writeEditorMode(1, "craft");
    writeEditorMode(2, "simple");
    expect(readEditorMode(1)).toBe("craft");
    expect(readEditorMode(2)).toBe("simple");
    expect(readEditorMode(3)).toBe("simple");
  });

  it("overwrites the same business key when mode changes", () => {
    writeEditorMode(7, "simple");
    writeEditorMode(7, "craft");
    expect(readEditorMode(7)).toBe("craft");
    writeEditorMode(7, "simple");
    expect(readEditorMode(7)).toBe("simple");
  });

  it("survives corrupt localStorage values by falling back to simple", () => {
    localStorage.setItem(editorModeStorageKey(5), "CANVA_PRO");
    expect(readEditorMode(5)).toBe("simple");
    localStorage.setItem(editorModeStorageKey(5), JSON.stringify({ mode: "craft" }));
    expect(readEditorMode(5)).toBe("simple");
  });

  it("does not throw when localStorage is unavailable", () => {
    const original = window.localStorage;
    Object.defineProperty(window, "localStorage", {
      configurable: true,
      value: {
        getItem: () => {
          throw new Error("quota");
        },
        setItem: () => {
          throw new Error("quota");
        },
      },
    });
    expect(() => readEditorMode(1)).not.toThrow();
    expect(readEditorMode(1)).toBe("simple");
    expect(() => writeEditorMode(1, "craft")).not.toThrow();
    Object.defineProperty(window, "localStorage", {
      configurable: true,
      value: original,
    });
  });
});
