/**
 * @jest-environment jsdom
 */
import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import axios from "axios";

import { MercadoPagoQRCharge } from "./MercadoPagoQRCharge";

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/api/currency", () => ({
  formatCurrency: (value: number) => `$${value.toFixed(2)}`,
}));

const mockCharge = jest.fn();
const mockOrderStatus = jest.fn();
const mockCancelOrder = jest.fn();

jest.mock("@/api/plugins", () => ({
  PLUGIN: { mercadopago: "mercadopago" },
  mercadoPagoQRAPI: {
    charge: (...args: unknown[]) => mockCharge(...args),
    // Component polls orderStatus (not getOrder) — mock the real method.
    orderStatus: (...args: unknown[]) => mockOrderStatus(...args),
    cancelOrder: (...args: unknown[]) => mockCancelOrder(...args),
  },
  paymentPluginAPI: {
    getBillPaymentStatus: jest.fn(),
    getBusinessPaymentPlugins: jest.fn().mockResolvedValue({
      plugins: [{ name: "mercadopago", is_active: true }],
    }),
  },
}));

jest.mock("@/constants/plugins", () => ({
  PLUGIN: { mercadopago: "mercadopago" },
}));

function renderQR() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <MercadoPagoQRCharge
        businessId={1}
        billId={42}
        remainingAmount={25}
        currency="ARS"
      />
    </QueryClientProvider>,
  );
}

describe("MercadoPagoQRCharge", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    mockOrderStatus.mockResolvedValue({
      order_id: "ORD_EXISTING",
      status: "pending",
    });
  });

  // F10 + P6d: 409 with order_id must enter polling via orderStatus API and
  // show other-device copy when no QR image is available yet.
  it("enters polling phase on 409 conflict with order_id and polls orderStatus", async () => {
    const err = new axios.AxiosError("conflict");
    err.response = {
      status: 409,
      data: { order_id: "ORD_EXISTING", error: "pending" },
      statusText: "Conflict",
      headers: {},
      config: {} as never,
    };
    mockCharge.mockRejectedValue(err);

    renderQR();

    const openBtn = await screen.findByTestId("mp-qr-charge-open");
    fireEvent.click(openBtn);

    const confirm = await screen.findByTestId("mp-qr-confirm");
    fireEvent.click(confirm);

    await waitFor(() => {
      expect(screen.getByTestId("mp-qr-status")).toBeInTheDocument();
    });
    expect(screen.getByTestId("mp-qr-cancel-order")).toBeInTheDocument();
    expect(screen.getByText("ORD_EXISTING")).toBeInTheDocument();
    expect(screen.queryByTestId("mp-qr-confirm")).not.toBeInTheDocument();

    await waitFor(() => {
      expect(mockOrderStatus).toHaveBeenCalledWith("1", "ORD_EXISTING");
    });
    expect(screen.getByTestId("mp-qr-other-device")).toBeInTheDocument();
  });

  it("renders QR image when orderStatus returns qr_png_base64 after 409", async () => {
    const err = new axios.AxiosError("conflict");
    err.response = {
      status: 409,
      data: { order_id: "ORD_WITH_QR", error: "pending" },
      statusText: "Conflict",
      headers: {},
      config: {} as never,
    };
    mockCharge.mockRejectedValue(err);
    mockOrderStatus.mockResolvedValue({
      order_id: "ORD_WITH_QR",
      status: "pending",
      qr_data: "00020126...",
      qr_png_base64: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
    });

    renderQR();
    fireEvent.click(await screen.findByTestId("mp-qr-charge-open"));
    fireEvent.click(await screen.findByTestId("mp-qr-confirm"));

    await waitFor(() => {
      expect(screen.getByTestId("mp-qr-image")).toBeInTheDocument();
    });
    expect(screen.queryByTestId("mp-qr-other-device")).not.toBeInTheDocument();
  });
});
