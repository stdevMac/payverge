/** @jest-environment jsdom */

import {
  buildKeyframeUploadPayload,
  deriveCaptureGuidance,
  hasCameraApi,
  isActiveScanStatus,
  isMobileUserAgent,
  isTerminalScanStatus,
  type CapturedKeyframe,
} from "../scanStatus";

describe("scanStatus helpers", () => {
  it("classifies active and terminal statuses", () => {
    expect(isActiveScanStatus("waiting_for_phone")).toBe(true);
    expect(isActiveScanStatus("processing")).toBe(true);
    expect(isTerminalScanStatus("review_ready")).toBe(true);
    expect(isTerminalScanStatus("failed")).toBe(true);
    expect(isActiveScanStatus("cancelled")).toBe(false);
  });

  it("detects mobile UA", () => {
    expect(isMobileUserAgent("Mozilla/5.0 (iPhone; CPU iPhone OS 17_0)")).toBe(
      true,
    );
    expect(isMobileUserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X)")).toBe(
      false,
    );
  });

  it("buildKeyframeUploadPayload marks approximate and omits image blobs", () => {
    const frames: CapturedKeyframe[] = [
      {
        index: 0,
        metric: false,
        pose_position: { x: 0, y: 0 },
        timestamp: 1,
        dataUrl: "data:image/jpeg;base64,xxx",
      },
    ];
    const payload = buildKeyframeUploadPayload(frames, {
      room_width_mm: 8000,
      room_height_mm: 6000,
    });
    expect(payload.metric).toBe(false);
    expect((payload.notes as string[]).join(" ")).toMatch(/not RoomPlan/i);
    expect((payload.frames as Array<Record<string, unknown>>)[0].dataUrl).toBeUndefined();
  });

  it("deriveCaptureGuidance prefers honest messages", () => {
    expect(deriveCaptureGuidance([], 24, false)).toBe("move_closer");
    expect(
      deriveCaptureGuidance(
        Array.from({ length: 24 }, (_, i) => ({
          index: i,
          metric: false,
          timestamp: i * 500,
        })),
        24,
        false,
      ),
    ).toBe("quality_sufficient");
    expect(
      deriveCaptureGuidance(
        [
          { index: 0, metric: false, timestamp: 0, luma: 10 },
          { index: 1, metric: false, timestamp: 500, luma: 12 },
        ],
        24,
        false,
      ),
    ).toBe("low_light");
  });

  it("hasCameraApi reflects mediaDevices", () => {
    const original = navigator.mediaDevices;
    Object.defineProperty(navigator, "mediaDevices", {
      value: { getUserMedia: jest.fn() },
      configurable: true,
    });
    expect(hasCameraApi()).toBe(true);
    Object.defineProperty(navigator, "mediaDevices", {
      value: undefined,
      configurable: true,
    });
    expect(hasCameraApi()).toBe(false);
    Object.defineProperty(navigator, "mediaDevices", {
      value: original,
      configurable: true,
    });
  });
});
