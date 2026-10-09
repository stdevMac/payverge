import {
  exportBlockedReason,
  exportBlockedReasonKey,
  isExportReady,
} from "./exportReadiness";

describe("exportBlockedReason", () => {
  const readyBase = {
    hasPhoto: true,
    previewState: "ready",
    displayCaption: "Hello",
    captionGenerating: false,
    captionError: false,
  };

  it("is ready when photo, preview, and caption are good", () => {
    expect(exportBlockedReason(readyBase)).toBeNull();
    expect(isExportReady(readyBase)).toBe(true);
  });

  it("prefers photo absence over later blockers", () => {
    expect(
      exportBlockedReason({
        ...readyBase,
        hasPhoto: false,
        previewState: "failed",
        displayCaption: "",
      }),
    ).toBe("needs_photo");
  });

  it("reports photo_broken when the preview failed", () => {
    expect(
      exportBlockedReason({ ...readyBase, previewState: "failed" }),
    ).toBe("photo_broken");
  });

  it("reports preview_rendering while the canvas is still painting", () => {
    expect(
      exportBlockedReason({ ...readyBase, previewState: "rendering" }),
    ).toBe("preview_rendering");
  });

  it("reports caption_loading while AI caption is in flight", () => {
    expect(
      exportBlockedReason({ ...readyBase, captionGenerating: true }),
    ).toBe("caption_loading");
  });

  it("reports caption_failed after AI caption error", () => {
    expect(
      exportBlockedReason({ ...readyBase, captionError: true }),
    ).toBe("caption_failed");
  });

  it("reports needs_caption when caption is blank", () => {
    expect(
      exportBlockedReason({ ...readyBase, displayCaption: "   " }),
    ).toBe("needs_caption");
  });

  it("blocks Ready-to-post when the photo library grades the image too dark", () => {
    expect(
      exportBlockedReason({ ...readyBase, photoQuality: "too_dark" }),
    ).toBe("photo_too_dark");
    expect(isExportReady({ ...readyBase, photoQuality: "too_dark" })).toBe(
      false,
    );
  });

  it("blocks Ready-to-post when the photo must be reshot", () => {
    expect(
      exportBlockedReason({ ...readyBase, photoQuality: "reshoot" }),
    ).toBe("photo_weak");
  });

  it("maps reasons to card.blocked.* i18n keys", () => {
    expect(exportBlockedReasonKey("needs_photo")).toBe(
      "card.blocked.needs_photo",
    );
    expect(exportBlockedReasonKey("caption_loading")).toBe(
      "card.blocked.caption_loading",
    );
    expect(exportBlockedReasonKey("photo_too_dark")).toBe(
      "card.blocked.photo_too_dark",
    );
  });
});
