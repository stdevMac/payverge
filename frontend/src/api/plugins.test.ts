import {
  adminPluginAPI,
  businessPluginAPI,
  mercadoPagoPointAPI,
  mercadoPagoQRAPI,
  paymentPluginAPI,
  pluginUtils,
  type Plugin,
} from "@/api/plugins";
import { axiosInstance } from "@/api/tools/instance";
import { asDollars } from "@/types/money";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    get: jest.fn(),
    post: jest.fn(),
    patch: jest.fn(),
  },
}));

describe("plugin api", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("creates guest plugin payments with the opaque bill number route", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { payment_id: "pay_123", status: "pending" },
    });

    await paymentPluginAPI.createPluginPayment("B42-opaque", {
      plugin_id: "paypal",
      amount: asDollars(33.5),
      currency: "USD",
      tip_amount: asDollars(3.5),
    });

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/guest/bill/B42-opaque/plugin-payment",
      expect.objectContaining({
        plugin_id: "paypal",
        amount: 33.5,
        currency: "USD",
        tip_amount: 3.5,
      }),
    );
  });

  it("checks guest plugin payment status with the opaque bill number route", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { status: "paid" },
    });

    await paymentPluginAPI.getPluginPaymentStatus(
      "B42-opaque",
      "pay_123",
      "paypal",
    );

    // Must bypass the GET cache: settlement is a server-side webhook with no
    // client mutation to invalidate the cached first "pending", so a cached
    // status would replay for the full 5-min TTL and never observe completion.
    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/guest/bill/B42-opaque/plugin-payment/pay_123/status",
      {
        params: { plugin: "paypal" },
        _useCache: false,
      },
    );
  });

  it("requests business payment plugins with optional lang param", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { plugins: [{ name: "paypal", display_name: "PayPal" }] },
    });

    await expect(
      paymentPluginAPI.getBusinessPaymentPlugins(42),
    ).resolves.toEqual({
      plugins: [{ name: "paypal", display_name: "PayPal" }],
    });
    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/businesses/42/payment-plugins",
      { params: {} },
    );

    await paymentPluginAPI.getBusinessPaymentPlugins(42, "es");
    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/businesses/42/payment-plugins",
      { params: { lang: "es" } },
    );
  });

  it("deactivates admin plugins through the supported route", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { message: "Plugin deactivated successfully" },
    });

    await adminPluginAPI.deletePlugin(9);

    expect(axiosInstance.post).toHaveBeenCalledWith("/admin/plugins/9/deactivate");
  });

  it("starts Mercado Pago OAuth and returns the authorization URL", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: {
        authorization_url:
          "https://auth.mercadopago.com/authorization?client_id=app&state=xyz",
      },
    });

    await expect(businessPluginAPI.startMercadoPagoOAuth("42")).resolves.toEqual({
      authorization_url:
        "https://auth.mercadopago.com/authorization?client_id=app&state=xyz",
    });

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/42/plugins/mercadopago/oauth/start",
    );
  });

  it("starts Stripe Connect OAuth and returns the authorization URL", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: {
        authorization_url:
          "https://connect.stripe.com/oauth/authorize?response_type=code&client_id=ca_x&state=xyz",
      },
    });

    await expect(businessPluginAPI.startStripeOAuth("42")).resolves.toEqual({
      authorization_url:
        "https://connect.stripe.com/oauth/authorize?response_type=code&client_id=ca_x&state=xyz",
    });

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/42/plugins/stripe/oauth/start",
    );
  });
});

describe("mercadoPagoPointAPI", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("lists Point terminals for a business", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: {
        terminals: [
          {
            id: "NEWLAND_N950__1",
            operating_mode: "PDV",
            external_pos_id: "POS1",
          },
        ],
      },
    });

    await expect(mercadoPagoPointAPI.listTerminals("42")).resolves.toEqual({
      terminals: [
        {
          id: "NEWLAND_N950__1",
          operating_mode: "PDV",
          external_pos_id: "POS1",
        },
      ],
    });

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/mercadopago/terminals",
      { _useCache: false },
    );
  });

  it("sets terminal operating mode via PATCH", async () => {
    (axiosInstance.patch as jest.Mock).mockResolvedValue({
      data: { terminal_id: "T1", operating_mode: "PDV" },
    });

    await expect(
      mercadoPagoPointAPI.setTerminalMode("42", "T1", "PDV"),
    ).resolves.toEqual({ terminal_id: "T1", operating_mode: "PDV" });

    expect(axiosInstance.patch).toHaveBeenCalledWith(
      "/inside/businesses/42/mercadopago/terminals/T1/mode",
      { mode: "PDV" },
    );
  });

  it("charges a bill on a Point terminal", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { order_id: "ord_1", status: "pending" },
    });

    await expect(
      mercadoPagoPointAPI.charge("42", 99, {
        terminal_id: "T1",
        amount_cents: 1500,
      }),
    ).resolves.toEqual({ order_id: "ord_1", status: "pending" });

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/42/bills/99/mercadopago/point/charge",
      { terminal_id: "T1", amount_cents: 1500 },
    );
  });

  it("polls Point order status without GET cache", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { order_id: "ord_1", status: "pending" },
    });

    await expect(
      mercadoPagoPointAPI.orderStatus("42", "ord_1"),
    ).resolves.toEqual({ order_id: "ord_1", status: "pending" });

    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/mercadopago/orders/ord_1",
      { _useCache: false },
    );
  });

  it("cancels a Point order", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { status: "cancelled" },
    });

    await expect(
      mercadoPagoPointAPI.cancelOrder("42", "ord_1"),
    ).resolves.toEqual({ status: "cancelled" });

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/42/mercadopago/orders/ord_1/cancel",
    );
  });
});

describe("mercadoPagoQRAPI", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("charges a bill with a dynamic QR order", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: {
        order_id: "ORD_QR_1",
        qr_data: "00020101...",
        qr_png_base64: "iVBORw0KGgo=",
        expires_at: "2026-08-09T12:10:00Z",
        status: "pending",
      },
    });

    await expect(
      mercadoPagoQRAPI.charge("42", 99, { amount_cents: 0 }),
    ).resolves.toEqual(
      expect.objectContaining({
        order_id: "ORD_QR_1",
        qr_png_base64: "iVBORw0KGgo=",
      }),
    );

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/42/bills/99/mercadopago/qr/charge",
      { amount_cents: 0 },
    );
  });

  it("reuses order status and cancel endpoints", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({
      data: { order_id: "ORD_QR_1", status: "pending" },
    });
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { status: "cancelled" },
    });

    await expect(
      mercadoPagoQRAPI.orderStatus("42", "ORD_QR_1"),
    ).resolves.toEqual({ order_id: "ORD_QR_1", status: "pending" });
    expect(axiosInstance.get).toHaveBeenCalledWith(
      "/inside/businesses/42/mercadopago/orders/ORD_QR_1",
      { _useCache: false },
    );

    await expect(mercadoPagoQRAPI.cancelOrder("42", "ORD_QR_1")).resolves.toEqual(
      { status: "cancelled" },
    );
    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/inside/businesses/42/mercadopago/orders/ORD_QR_1/cancel",
    );
  });
});

describe("pluginUtils locale translation lookup", () => {
  test("uses exact regional locale translation when present", () => {
    const plugin = {
      display_name: "Stripe",
      description: "Accept card payments",
      features: "[]",
      translations: [
        {
          id: 1,
          plugin_id: 1,
          language_code: "es-AR",
          field_name: "description",
          content: "Aceptá pagos con tarjeta",
          created_at: "",
          updated_at: "",
        },
      ],
    } as Plugin;

    expect(
      pluginUtils.getTranslatedContent(plugin, "description", "es-AR"),
    ).toBe("Aceptá pagos con tarjeta");
  });
});
