/** @jest-environment jsdom */
import { renderHook, act, waitFor } from "@testing-library/react";

const mockPrint = jest.fn().mockResolvedValue(undefined);
jest.mock("../printers/useIframePrint", () => ({
  useIframePrint: () => ({ print: mockPrint }),
}));

jest.mock("qrcode", () => ({
  __esModule: true,
  default: {
    toDataURL: jest.fn((url: string) =>
      Promise.resolve(`data:image/png;base64,${encodeURIComponent(url)}`),
    ),
  },
}));

import QRCode from "qrcode";
import { useQrSheetPrint } from "./useQrSheetPrint";

const strings = {
  scanCaption: "Scan to order",
  documentTitle: "Biz — QR codes",
  poweredBy: "Powered by Payverge",
};

const tables = [
  { name: "Patio 1", table_code: "PATIO1", is_active: true, qr_foreground_color: "#123456" },
  { name: "Patio 2", table_code: "PATIO2", is_active: true },
  { name: "Retired", table_code: "OLD1", is_active: false },
];

describe("useQrSheetPrint (P2-16)", () => {
  beforeEach(() => jest.clearAllMocks());

  it("prints one card per ACTIVE table, honoring per-table color overrides over business defaults", async () => {
    const { result } = renderHook(() =>
      useQrSheetPrint({
        businessName: "Biz",
        tables,
        defaults: { qr_foreground_color: "#000011", qr_background_color: "#FFFFEE" },
        strings,
      }),
    );

    expect(result.current.activeTableCount).toBe(2);

    await act(async () => {
      await result.current.printAll();
    });

    // Inactive table excluded; two QR generations.
    expect((QRCode.toDataURL as jest.Mock).mock.calls).toHaveLength(2);
    // Per-table override wins for PATIO1; business default fills PATIO2.
    const [, opts1] = (QRCode.toDataURL as jest.Mock).mock.calls[0];
    const [, opts2] = (QRCode.toDataURL as jest.Mock).mock.calls[1];
    expect(opts1.color.dark).toBe("#123456");
    expect(opts2.color.dark).toBe("#000011");
    expect(opts2.color.light).toBe("#FFFFEE");

    await waitFor(() => expect(mockPrint).toHaveBeenCalledTimes(1));
    const html = mockPrint.mock.calls[0][0] as string;
    expect(html).toContain("Patio 1");
    expect(html).toContain("Patio 2");
    expect(html).not.toContain("Retired");
  });

  it("no-ops with zero active tables and reports errors through onError", async () => {
    const onError = jest.fn();
    const { result } = renderHook(() =>
      useQrSheetPrint({ businessName: "Biz", tables: [], strings, onError }),
    );
    await act(async () => {
      await result.current.printAll();
    });
    expect(mockPrint).not.toHaveBeenCalled();

    mockPrint.mockRejectedValueOnce(new Error("blocked"));
    const { result: r2 } = renderHook(() =>
      useQrSheetPrint({ businessName: "Biz", tables: [tables[0]], strings, onError }),
    );
    await act(async () => {
      await r2.current.printAll();
    });
    expect(onError).toHaveBeenCalled();
  });
});
