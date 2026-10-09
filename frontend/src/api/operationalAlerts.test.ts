export {};

const originalFetch = global.fetch;
const originalApiUrl = process.env.API_URL;

const API_URL = "https://api.example.test/api/v1";

const loadClient = async () => {
  jest.resetModules();
  process.env.API_URL = API_URL;
  return import("./operationalAlerts");
};

beforeEach(() => {
  global.fetch = jest.fn();
  process.env.API_URL = API_URL;
});

afterEach(() => {
  process.env.API_URL = originalApiUrl;
});

afterAll(() => {
  global.fetch = originalFetch;
});

describe("operational alerts api client", () => {
  it("GETs active alerts with comma-separated filters", async () => {
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ alerts: [] }),
    });
    const { fetchOperationalAlerts } = await loadClient();

    await fetchOperationalAlerts(42, {
      status: ["open", "claimed"],
      types: ["order_new"],
    });

    expect(global.fetch).toHaveBeenCalledWith(
      `${API_URL}/inside/businesses/42/alerts?status=open%2Cclaimed&types=order_new`,
      expect.objectContaining({
        credentials: "include",
        method: "GET",
      }),
    );
  });

  it("POSTs a claim source", async () => {
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ id: 7 }),
    });
    const { claimOperationalAlert } = await loadClient();

    await claimOperationalAlert(42, 7, "row_click");

    expect(global.fetch).toHaveBeenCalledWith(
      `${API_URL}/inside/businesses/42/alerts/7/claim`,
      expect.objectContaining({
        credentials: "include",
        method: "POST",
        body: JSON.stringify({ source: "row_click" }),
      }),
    );
  });

  it("POSTs an optional assignee staff_id when routing a service call", async () => {
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ id: 7 }),
    });
    const { claimOperationalAlert } = await loadClient();

    await claimOperationalAlert(42, 7, "tables_queue", 19);

    expect(global.fetch).toHaveBeenCalledWith(
      `${API_URL}/inside/businesses/42/alerts/7/claim`,
      expect.objectContaining({
        credentials: "include",
        method: "POST",
        body: JSON.stringify({ source: "tables_queue", staff_id: 19 }),
      }),
    );
  });

  it("throws OperationalAlertConflictError with claimed_by_name on a 409 claim", async () => {
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: false,
      status: 409,
      text: async () =>
        JSON.stringify({ claimed_by_name: "Sam", status: "claimed" }),
    });
    const { claimOperationalAlert, OperationalAlertConflictError } =
      await loadClient();

    const err = await claimOperationalAlert(42, 7, "row_click").catch((e) => e);

    expect(err).toBeInstanceOf(OperationalAlertConflictError);
    expect(err.status).toBe(409);
    expect(err.claimedByName).toBe("Sam");
    expect(err.alertStatus).toBe("claimed");
  });

  it("409 with an unparseable body still yields a conflict error with null fields", async () => {
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: false,
      status: 409,
      text: async () => "conflict",
    });
    const { resolveOperationalAlert, OperationalAlertConflictError } =
      await loadClient();

    const err = await resolveOperationalAlert(42, 7, "done").catch((e) => e);

    expect(err).toBeInstanceOf(OperationalAlertConflictError);
    expect(err.claimedByName).toBeNull();
    expect(err.alertStatus).toBeNull();
  });

  it("non-409 failures still throw a generic HTTP error", async () => {
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: false,
      status: 500,
      text: async () => "boom",
    });
    const { claimOperationalAlert, OperationalAlertConflictError } =
      await loadClient();

    const err = await claimOperationalAlert(42, 7, "row_click").catch((e) => e);

    expect(err).toBeInstanceOf(Error);
    expect(err).not.toBeInstanceOf(OperationalAlertConflictError);
    expect(String(err.message)).toContain("HTTP 500");
  });

  it("POSTs a resolve reason", async () => {
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ id: 7 }),
    });
    const { resolveOperationalAlert } = await loadClient();

    await resolveOperationalAlert(42, 7, "status_changed");

    expect(global.fetch).toHaveBeenCalledWith(
      `${API_URL}/inside/businesses/42/alerts/7/resolve`,
      expect.objectContaining({
        credentials: "include",
        method: "POST",
        body: JSON.stringify({ reason: "status_changed" }),
      }),
    );
  });

  it("POSTs a snooze deadline", async () => {
    const until = "2026-04-21T15:30:00.000Z";
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ id: 7 }),
    });
    const { snoozeOperationalAlert } = await loadClient();

    await snoozeOperationalAlert(42, 7, until);

    expect(global.fetch).toHaveBeenCalledWith(
      `${API_URL}/inside/businesses/42/alerts/7/snooze`,
      expect.objectContaining({
        credentials: "include",
        method: "POST",
        body: JSON.stringify({ until }),
      }),
    );
  });

  it("GETs alert events", async () => {
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ events: [] }),
    });
    const { fetchOperationalAlertEvents } = await loadClient();

    await fetchOperationalAlertEvents(42, 7);

    expect(global.fetch).toHaveBeenCalledWith(
      `${API_URL}/inside/businesses/42/alerts/7/events`,
      expect.objectContaining({
        credentials: "include",
        method: "GET",
      }),
    );
  });

  it("GETs alert settings", async () => {
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: true,
      json: async () => ({ enabled: true }),
    });
    const { fetchOperationalAlertSettings } = await loadClient();

    await fetchOperationalAlertSettings(42);

    expect(global.fetch).toHaveBeenCalledWith(
      `${API_URL}/inside/businesses/42/alerts/settings`,
      expect.objectContaining({
        credentials: "include",
        method: "GET",
      }),
    );
  });

  it("PUTs alert settings", async () => {
    const settings: import("./operationalAlerts").OperationalAlertSettings = {
      enabled: true,
      browser_notifications_enabled: true,
      sound_enabled: true,
      volume: 0.75,
      repeat_interval_seconds: 10,
      event_settings: {
        order_new: {
          enabled: true,
          repeating: true,
          priority: "high",
        },
        kitchen_order_ready: {
          enabled: true,
          repeating: true,
          priority: "urgent",
        },
        service_call: { enabled: true, repeating: true, priority: "urgent" },
        reservation_new: {
          enabled: true,
          repeating: false,
          priority: "normal",
        },
        reservation_approval: {
          enabled: true,
          repeating: true,
          priority: "urgent",
        },
        delivery_new: { enabled: true, repeating: true, priority: "urgent" },
        bill_new: { enabled: true, repeating: false, priority: "normal" },
        payment_requested: {
          enabled: true,
          repeating: false,
          priority: "high",
        },
        payment_received: {
          enabled: true,
          repeating: false,
          priority: "normal",
        },
        payment_refund_review: {
          enabled: true,
          repeating: false,
          priority: "high",
        },
        ai_takeover: { enabled: true, repeating: true, priority: "urgent" },
      },
    };
    (global.fetch as jest.Mock).mockResolvedValueOnce({
      ok: true,
      json: async () => settings,
    });
    const { updateOperationalAlertSettings } = await loadClient();

    await updateOperationalAlertSettings(42, settings);

    expect(global.fetch).toHaveBeenCalledWith(
      `${API_URL}/inside/businesses/42/alerts/settings`,
      expect.objectContaining({
        credentials: "include",
        method: "PUT",
        body: JSON.stringify(settings),
      }),
    );
  });
});

export {};
