/** @jest-environment jsdom */
import { render, waitFor } from "@testing-library/react";
import QRCodeWithText from "../QRCodeWithText";

const mockToDataURL = jest.fn();
jest.mock("qrcode", () => ({
  __esModule: true,
  default: {
    toDataURL: (...args: unknown[]) => mockToDataURL(...args),
  },
}));

const mockContext = {
  drawImage: jest.fn(),
  fillRect: jest.fn(),
  fillText: jest.fn(),
  fillStyle: "",
  textAlign: "",
  font: "",
};

class MockImage {
  onload: (() => void) | null = null;
  crossOrigin = "";

  set src(_value: string) {
    queueMicrotask(() => this.onload?.());
  }
}

describe("QRCodeWithText first-value callback", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    jest
      .spyOn(HTMLCanvasElement.prototype, "getContext")
      .mockReturnValue(mockContext as unknown as CanvasRenderingContext2D);
    Object.defineProperty(globalThis, "Image", {
      configurable: true,
      writable: true,
      value: MockImage,
    });
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  it("fires onRendered only after a QR image is drawn to the canvas", async () => {
    mockToDataURL.mockImplementation(
      (
        _value: string,
        _options: unknown,
        callback: (error: Error | null, url: string) => void,
      ) => callback(null, "data:image/png;base64,valid"),
    );
    const onRendered = jest.fn();

    render(
      <QRCodeWithText
        tableCode="TABLE-01"
        tableName="Table 1"
        poweredByText="Powered by Payverge"
        onRendered={onRendered}
      />,
    );

    expect(onRendered).not.toHaveBeenCalled();
    await waitFor(() => expect(mockContext.drawImage).toHaveBeenCalled());
    expect(onRendered).toHaveBeenCalledTimes(1);
  });

  it("does not fire onRendered when QR generation fails", async () => {
    const error = new Error("invalid QR");
    mockToDataURL.mockImplementation(
      (
        _value: string,
        _options: unknown,
        callback: (error: Error | null, url?: string) => void,
      ) => callback(error),
    );
    const consoleError = jest
      .spyOn(console, "error")
      .mockImplementation(() => {});
    const onRendered = jest.fn();

    render(
      <QRCodeWithText
        tableCode="TABLE-01"
        tableName="Table 1"
        poweredByText="Powered by Payverge"
        onRendered={onRendered}
      />,
    );

    await waitFor(() =>
      expect(consoleError).toHaveBeenCalledWith(
        "QR Code generation error:",
        error,
      ),
    );
    expect(mockContext.drawImage).not.toHaveBeenCalled();
    expect(onRendered).not.toHaveBeenCalled();
  });
});
