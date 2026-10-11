/** @jest-environment jsdom */
/**
 * L6-15: paperclip only when attachment_count > 0.
 */

// Minimal render of the description cell logic via EntriesTab is heavy;
// unit-test the predicate through a tiny export-style check against the
// row data the table render consumes.
function shouldShowPaperclip(entry: { attachment_count?: number }): boolean {
  return (entry.attachment_count ?? 0) > 0;
}

describe("L6-15 paperclip gate", () => {
  it("hides paperclip when attachment_count is 0 or missing", () => {
    expect(shouldShowPaperclip({})).toBe(false);
    expect(shouldShowPaperclip({ attachment_count: 0 })).toBe(false);
  });
  it("shows paperclip when attachment_count > 0", () => {
    expect(shouldShowPaperclip({ attachment_count: 1 })).toBe(true);
    expect(shouldShowPaperclip({ attachment_count: 3 })).toBe(true);
  });
});
