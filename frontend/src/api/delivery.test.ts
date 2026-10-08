import { deliveryApi, guestDeliveryApi, UpdateDeliverySettingsInput } from "@/api/delivery";
import { axiosInstance } from "@/api/tools/instance";
import { asDollars } from "@/types/money";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: {
    post: jest.fn(),
    get: jest.fn(),
    put: jest.fn(),
    delete: jest.fn(),
  },
}));

describe("deliveryApi", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("quotes delivery through the public quote endpoint", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({ data: { eligible: true } });

    await guestDeliveryApi.quote(42, {
      order_subtotal: asDollars(25),
      delivery_address: {
        street: "1 Main St",
        city: "Dubai",
        country: "AE",
      },
    });

    expect(axiosInstance.post).toHaveBeenCalledWith("/businesses/42/delivery/quote", {
      order_subtotal: 25,
      delivery_address: {
        street: "1 Main St",
        city: "Dubai",
        country: "AE",
      },
    });
  });

  it("submits guest delivery checkout through the transactional checkout endpoint", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({ data: { tracking_url: "/delivery/DEL-1/track" } });

    await guestDeliveryApi.checkout(42, {
      customer_name: "Guest",
      customer_phone: "5551112222",
      customer_email: "guest@example.com",
      delivery_address: {
        street: "1 Main St",
        city: "Dubai",
        country: "AE",
      },
      items: [
        {
          menu_item_name: "Burger",
          quantity: 1,
          price: asDollars(12),
        },
      ],
    });

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/businesses/42/delivery/orders",
      {
        customer_name: "Guest",
        customer_phone: "5551112222",
        customer_email: "guest@example.com",
        delivery_address: {
          street: "1 Main St",
          city: "Dubai",
          country: "AE",
        },
        items: [
          {
            menu_item_name: "Burger",
            quantity: 1,
            price: 12,
          },
        ],
      },
      undefined,
    );
  });

  it("forwards the idempotency key as X-Request-Id on checkout", async () => {
    (axiosInstance.post as jest.Mock).mockResolvedValue({
      data: { tracking_url: "/delivery/DEL-2/track" },
    });

    await guestDeliveryApi.checkout(
      42,
      {
        customer_name: "Guest",
        customer_phone: "5551112222",
        customer_email: "guest@example.com",
        delivery_address: { street: "1 Main St", city: "Dubai", country: "AE" },
        items: [{ menu_item_name: "Burger", quantity: 1, price: asDollars(12) }],
      },
      { idempotencyKey: "delivery-retry-abc123" },
    );

    expect(axiosInstance.post).toHaveBeenCalledWith(
      "/businesses/42/delivery/orders",
      expect.any(Object),
      { headers: { "X-Request-Id": "delivery-retry-abc123" } },
    );
  });

  it("loads public delivery tracking by delivery number", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: { delivery_number: "DEL-1", status: "assigned" } });

    await guestDeliveryApi.track("DEL-1");

    expect(axiosInstance.get).toHaveBeenCalledWith("/delivery/DEL-1/track");
  });

  it("surfaces backend quote errors instead of a generic axios message", async () => {
    (axiosInstance.post as jest.Mock).mockRejectedValue({
      response: { data: { error: "Delivery is outside active hours" } },
    });

    await expect(
      guestDeliveryApi.quote(42, {
        order_subtotal: asDollars(25),
        delivery_address: {
          street: "1 Main St",
          city: "Dubai",
          country: "AE",
        },
      }),
    ).rejects.toThrow("Delivery is outside active hours");
  });

  it("attaches the HTTP status to tracking errors so guest pages can localize them", async () => {
    (axiosInstance.get as jest.Mock).mockRejectedValue({
      response: { status: 404, data: { error: "Delivery not found" } },
    });

    await expect(guestDeliveryApi.track("DEL-404")).rejects.toMatchObject({
      status: 404,
    });
  });

  it("updates delivery settings with zone inputs that do not require ids", async () => {
    const payload: UpdateDeliverySettingsInput = {
      delivery_enabled: true,
      zones: [
        {
          name: "Downtown",
          delivery_fee: asDollars(7),
          minimum_order_amount: asDollars(20),
          estimated_time: 18,
          priority: 1,
          cutoff_buffer_minutes: 0,
          is_active: true,
        },
      ],
    };
    (axiosInstance.put as jest.Mock).mockResolvedValue({ data: { business_id: 42 } });

    await deliveryApi.updateDeliverySettings(42, payload);

    expect(axiosInstance.put).toHaveBeenCalledWith(
      "/inside/businesses/42/delivery-settings",
      payload,
    );
  });

  it("loads business drivers from the protected delivery driver endpoint", async () => {
    (axiosInstance.get as jest.Mock).mockResolvedValue({ data: [{ id: 7, name: "Rami" }] });

    await deliveryApi.getBusinessDrivers(42);

    expect(axiosInstance.get).toHaveBeenCalledWith("/inside/businesses/42/drivers");
  });

  it("rejects invalid outgoing delivery status updates before hitting the backend", async () => {
    await expect(
      deliveryApi.updateDeliveryOrderStatus(42, 9, "lost" as any),
    ).rejects.toThrow("Invalid delivery status: lost");

    expect(axiosInstance.put).not.toHaveBeenCalled();
  });

  it("sends valid delivery status updates to the protected status route", async () => {
    (axiosInstance.put as jest.Mock).mockResolvedValue({ data: {} });

    await deliveryApi.updateDeliveryOrderStatus(42, 9, "in_transit");

    expect(axiosInstance.put).toHaveBeenCalledWith(
      "/inside/businesses/42/deliveries/9/status",
      { status: "in_transit" },
    );
  });
});

describe("updateDeliverySettings — partner-only stopgap UI", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("does not force in_house_delivery_enabled=false on every PUT", async () => {
    (axiosInstance.put as jest.Mock).mockResolvedValue({ data: {} });

    await deliveryApi.updateDeliverySettings(1, {
      delivery_enabled: true,
      in_house_delivery_enabled: true,
      third_party_enabled: false,
      external_partner_links: [],
    } as any);

    const sent = (axiosInstance.put as jest.Mock).mock.calls[0][1];
    expect(sent.in_house_delivery_enabled).toBe(true);
    expect(sent.third_party_enabled).toBe(false);
  });
});
