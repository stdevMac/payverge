/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import enMessages from "@/i18n/guest-messages/en.json";
import esMessages from "@/i18n/guest-messages/es.json";
import esARMessages from "@/i18n/guest-messages/es-AR.json";

import QRCodeScanner, { extractTableCode } from "./QRCodeScanner";

/** Nested guest-messages → dotted-key lookup (matches GuestTranslationProvider). */
function mockNestedT(messages: Record<string, unknown>, key: string): string {
  const parts = key.split(".");
  let cur: unknown = messages;
  for (const part of parts) {
    if (!cur || typeof cur !== "object" || !(part in (cur as object))) {
      return key;
    }
    cur = (cur as Record<string, unknown>)[part];
  }
  return typeof cur === "string" ? cur : key;
}

let mockActiveMessages: Record<string, unknown> = enMessages as Record<
  string,
  unknown
>;

jest.mock("@/i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({
    t: (key: string) => mockNestedT(mockActiveMessages, key),
  }),
}));

jest.mock("@nextui-org/react", () => {
  const ReactActual = require("react");
  return {
    Card: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
    CardBody: ({ children }: { children: React.ReactNode }) => (
      <div>{children}</div>
    ),
    Spinner: () => <div data-testid="spinner" />,
    Button: ({
      children,
      onPress,
      "aria-label": ariaLabel,
    }: {
      children?: React.ReactNode;
      onPress?: () => void;
      "aria-label"?: string;
    }) =>
      ReactActual.createElement(
        "button",
        { type: "button", onClick: onPress, "aria-label": ariaLabel },
        children,
      ),
  };
});

// jsQR mock: decodes any frame to a payload with a table-code URL.
const mockJsqr = jest.fn((..._args: unknown[]) => ({
  data: "https://payverge.io/t/abc123",
}));
jest.mock("jsqr", () => ({
  __esModule: true,
  default: (...args: unknown[]) => mockJsqr(...args),
}));

describe("extractTableCode", () => {
  it("extracts and upper-cases a code from a /t/ URL (case-insensitive)", () => {
    expect(extractTableCode("https://payverge.io/t/abc123")).toBe("ABC123");
    expect(extractTableCode("https://payverge.io/T/AbC123")).toBe("ABC123");
  });

  it("extracts a bare alphanumeric code", () => {
    expect(extractTableCode("xyz789")).toBe("XYZ789");
  });

  it("extracts demo slug codes with hyphens from /t/ URLs", () => {
    expect(extractTableCode("https://payverge.io/t/demo-50-core-table-01")).toBe(
      "DEMO-50-CORE-TABLE-01",
    );
  });

  it("returns null for an unrelated payload", () => {
    expect(extractTableCode("https://example.com/menu")).toBeNull();
    expect(extractTableCode("")).toBeNull();
  });
});

describe("production guest-messages scan keys exist", () => {
  it("ships cameraDeniedError and scanner.closeAria in en, es, and es-AR", () => {
    for (const [locale, messages] of [
      ["en", enMessages],
      ["es", esMessages],
      ["es-AR", esARMessages],
    ] as const) {
      const denied = mockNestedT(
        messages as Record<string, unknown>,
        "scan.cameraDeniedError",
      );
      const closeAria = mockNestedT(
        messages as Record<string, unknown>,
        "scan.scanner.closeAria",
      );
      expect(denied).not.toBe("scan.cameraDeniedError");
      expect(denied.toLowerCase()).not.toContain("camera denied error");
      expect(closeAria).not.toBe("scan.scanner.closeAria");
      expect(closeAria.toLowerCase()).not.toContain("close aria");
      // Locale-specific sanity (not English leaf fallback)
      if (locale === "en") {
        expect(denied).toMatch(/camera/i);
        expect(closeAria).toMatch(/close/i);
      } else {
        expect(denied.toLowerCase()).not.toBe(
          (enMessages as { scan: { cameraDeniedError: string } }).scan
            .cameraDeniedError.toLowerCase(),
        );
      }
    }
  });
});

describe("QRCodeScanner camera + jsQR path", () => {
  const realGUM = navigator.mediaDevices;
  let stopTrack: jest.Mock;

  beforeEach(() => {
    mockActiveMessages = enMessages as Record<string, unknown>;
    mockJsqr.mockClear();
    stopTrack = jest.fn();
    Object.defineProperty(navigator, "mediaDevices", {
      configurable: true,
      value: {
        getUserMedia: jest.fn().mockResolvedValue({
          getTracks: () => [{ stop: stopTrack }],
        }),
      },
    });

    // Force the jsQR branch (no native BarcodeDetector) and a drawable canvas.
    delete (window as unknown as { BarcodeDetector?: unknown }).BarcodeDetector;
    HTMLCanvasElement.prototype.getContext = jest.fn(() => ({
      drawImage: jest.fn(),
      getImageData: jest.fn(() => ({
        data: new Uint8ClampedArray(4),
        width: 1,
        height: 1,
      })),
    })) as unknown as typeof HTMLCanvasElement.prototype.getContext;
    HTMLMediaElement.prototype.play = jest.fn().mockResolvedValue(undefined);
  });

  afterEach(() => {
    Object.defineProperty(navigator, "mediaDevices", {
      configurable: true,
      value: realGUM,
    });
    jest.useRealTimers();
  });

  it("decodes a QR via jsQR and calls onScan with the upper-cased code, then stops the camera", async () => {
    const onScan = jest.fn();
    const onError = jest.fn();
    render(
      <QRCodeScanner onScan={onScan} onError={onError} onClose={jest.fn()} />,
    );

    // Start the camera (production English copy from guest-messages/en.json).
    fireEvent.click(
      screen.getByText(
        mockNestedT(enMessages as Record<string, unknown>, "scan.scanner.startScanning"),
      ),
    );

    await waitFor(() => expect(document.querySelector("video")).not.toBeNull());
    const video = document.querySelector("video") as HTMLVideoElement;
    Object.defineProperty(video, "videoWidth", {
      configurable: true,
      value: 640,
    });
    Object.defineProperty(video, "videoHeight", {
      configurable: true,
      value: 480,
    });

    await waitFor(() => expect(onScan).toHaveBeenCalledWith("ABC123"));
    expect(mockJsqr).toHaveBeenCalled();
    expect(stopTrack).toHaveBeenCalled();
    expect(onError).not.toHaveBeenCalled();
  });

  it("surfaces production English camera-denied copy from guest-messages", async () => {
    (navigator.mediaDevices.getUserMedia as jest.Mock).mockRejectedValueOnce(
      new Error("denied"),
    );
    const onError = jest.fn();
    render(
      <QRCodeScanner onScan={jest.fn()} onError={onError} onClose={jest.fn()} />,
    );

    const expected = mockNestedT(
      enMessages as Record<string, unknown>,
      "scan.cameraDeniedError",
    );
    expect(expected).not.toBe("scan.cameraDeniedError");

    fireEvent.click(
      screen.getByText(
        mockNestedT(enMessages as Record<string, unknown>, "scan.scanner.startScanning"),
      ),
    );
    await waitFor(() => expect(onError).toHaveBeenCalledWith(expected));
    // Must not fall through to sentence-cased leaf
    expect(onError).not.toHaveBeenCalledWith("Camera denied error");
    expect(onError).not.toHaveBeenCalledWith("scan.cameraDeniedError");
  });

  it("surfaces production Spanish camera-denied copy from guest-messages", async () => {
    mockActiveMessages = esMessages as Record<string, unknown>;
    (navigator.mediaDevices.getUserMedia as jest.Mock).mockRejectedValueOnce(
      new Error("denied"),
    );
    const onError = jest.fn();
    render(
      <QRCodeScanner onScan={jest.fn()} onError={onError} onClose={jest.fn()} />,
    );

    const expected = mockNestedT(
      esMessages as Record<string, unknown>,
      "scan.cameraDeniedError",
    );
    expect(expected).toMatch(/c[aá]mara/i);

    fireEvent.click(
      screen.getByText(
        mockNestedT(esMessages as Record<string, unknown>, "scan.scanner.startScanning"),
      ),
    );
    await waitFor(() => expect(onError).toHaveBeenCalledWith(expected));
    expect(onError).not.toHaveBeenCalledWith("Camera denied error");
  });

  it("labels the close control with production scan.scanner.closeAria", () => {
    render(
      <QRCodeScanner
        onScan={jest.fn()}
        onError={jest.fn()}
        onClose={jest.fn()}
      />,
    );
    const expected = mockNestedT(
      enMessages as Record<string, unknown>,
      "scan.scanner.closeAria",
    );
    expect(screen.getByRole("button", { name: expected })).toBeInTheDocument();
  });
});

describe("QRCodeScanner dialog semantics (#391)", () => {
  beforeAll(() => {
    if (!("inert" in HTMLElement.prototype)) {
      Object.defineProperty(HTMLElement.prototype, "inert", {
        configurable: true,
        enumerable: true,
        get() {
          return this.hasAttribute("inert");
        },
        set(value: boolean) {
          if (value) this.setAttribute("inert", "");
          else this.removeAttribute("inert");
        },
      });
    }
  });

  beforeEach(() => {
    mockActiveMessages = enMessages as Record<string, unknown>;
  });

  it("exposes a named modal dialog", () => {
    render(
      <QRCodeScanner
        onScan={jest.fn()}
        onError={jest.fn()}
        onClose={jest.fn()}
      />,
    );
    const title = mockNestedT(
      enMessages as Record<string, unknown>,
      "scan.scanner.title",
    );
    const dialog = screen.getByRole("dialog", { name: title });
    expect(dialog).toHaveAttribute("aria-modal", "true");
  });

  it("moves focus into the dialog, inerts the background, traps Tab, and dismisses on Escape", () => {
    const opener = document.createElement("button");
    opener.textContent = "Scan QR code";
    document.body.appendChild(opener);
    opener.focus();
    expect(document.activeElement).toBe(opener);

    const sibling = document.createElement("div");
    sibling.innerHTML = '<button type="button">Background</button>';
    document.body.appendChild(sibling);

    const onClose = jest.fn();
    const { unmount } = render(
      <QRCodeScanner onScan={jest.fn()} onError={jest.fn()} onClose={onClose} />,
    );

    const title = mockNestedT(
      enMessages as Record<string, unknown>,
      "scan.scanner.title",
    );
    const dialog = screen.getByRole("dialog", { name: title });
    expect(dialog.contains(document.activeElement as Node)).toBe(true);
    expect(sibling.inert).toBe(true);

    fireEvent.keyDown(dialog, { key: "Tab" });
    expect(dialog.contains(document.activeElement as Node)).toBe(true);

    fireEvent.keyDown(dialog, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);

    unmount();
    expect(sibling.inert).toBe(false);
    expect(document.activeElement).toBe(opener);
    opener.remove();
    sibling.remove();
  });
});
