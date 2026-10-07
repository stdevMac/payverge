import {
  ACTIVATION_EVENT_NAMES,
  ACTIVATION_SCHEMA_VERSION,
  createActivationEmitter,
  safeActivationToken,
  validateActivationEvent,
  type ClientActivationEvent,
  type ActivationStorage,
} from "./activationEvents";

class MemoryStorage implements ActivationStorage {
  values = new Map<string, string>();
  getItem(key: string) {
    return this.values.get(key) ?? null;
  }
  setItem(key: string, value: string) {
    this.values.set(key, value);
  }
}

const dimensions = {
  locale: "es-AR",
  device_class: "mobile" as const,
  acquisition_source: "organic",
  acquisition_campaign: "launch_2026",
  elapsed_ms: 1200,
};

describe("activation event contract", () => {
  it("contains exactly the normative 14 versioned events", () => {
    expect(ACTIVATION_SCHEMA_VERSION).toBe(1);
    expect(ACTIVATION_EVENT_NAMES).toEqual([
      "registration_started",
      "registration_completed",
      "workspace_created",
      "onboarding_step_viewed",
      "onboarding_step_clicked",
      "menu_item_created",
      "table_created",
      "qr_previewed",
      "payment_configured",
      "staff_invited",
      "setup_completed",
      "test_order_completed",
      "activation_achieved",
      "first_paid_bill",
    ]);
  });

  it("rejects PII, payment data, unknown fields, and authoritative client events", () => {
    expect(() =>
      validateActivationEvent({
        name: "registration_started",
        schema_version: 1,
        funnel_id: "728a70ef-0e4f-49b7-9978-229b7bc2be59",
        idempotency_key:
          "client:728a70ef-0e4f-49b7-9978-229b7bc2be59:registration_started:global",
        dimensions,
      } as ClientActivationEvent),
    ).not.toThrow();
    expect(() =>
      validateActivationEvent({
        name: "registration_started",
        schema_version: 1,
        funnel_id: "728a70ef-0e4f-49b7-9978-229b7bc2be59",
        idempotency_key:
          "client:728a70ef-0e4f-49b7-9978-229b7bc2be59:registration_started:global",
        dimensions,
        email: "owner@example.test",
      } as never),
    ).toThrow();
    expect(() =>
      validateActivationEvent({
        name: "registration_started",
        schema_version: 1,
        funnel_id: "728a70ef-0e4f-49b7-9978-229b7bc2be59",
        idempotency_key:
          "client:728a70ef-0e4f-49b7-9978-229b7bc2be59:registration_started:global",
        dimensions: { ...dimensions, locale: "owner@example.test" },
      }),
    ).toThrow();
    expect(() =>
      validateActivationEvent({
        name: "first_paid_bill",
        schema_version: 1,
        funnel_id: "728a70ef-0e4f-49b7-9978-229b7bc2be59",
        idempotency_key:
          "client:728a70ef-0e4f-49b7-9978-229b7bc2be59:first_paid_bill:global",
        dimensions,
      } as unknown as ClientActivationEvent),
    ).toThrow();
    expect(() =>
      validateActivationEvent({
        name: "registration_started",
        schema_version: 1,
        funnel_id: "728a70ef-0e4f-49b7-9978-229b7bc2be59",
        idempotency_key:
          "client:728a70ef-0e4f-49b7-9978-229b7bc2be59:registration_started:global",
        dimensions: { ...dimensions, business_id: "999" },
      } as unknown as ClientActivationEvent),
    ).toThrow("unsupported activation dimension: business_id");
    expect(() =>
      validateActivationEvent({
        name: "registration_started",
        schema_version: 1,
        funnel_id: "728a70ef-0e4f-49b7-9978-229b7bc2be59",
        idempotency_key:
          "client:728a70ef-0e4f-49b7-9978-229b7bc2be59:registration_started:global",
        dimensions: { ...dimensions, card_last4: "4242" },
      } as unknown as ClientActivationEvent),
    ).toThrow();
  });

  it("drops optional events without consent", async () => {
    const transport = jest.fn().mockResolvedValue(undefined);
    const emitter = createActivationEmitter({
      transport,
      storage: new MemoryStorage(),
      hasConsent: () => false,
      randomUUID: () => "728a70ef-0e4f-49b7-9978-229b7bc2be59",
    });
    await expect(
      emitter.emit("registration_started", dimensions),
    ).resolves.toBe("consent_denied");
    expect(transport).not.toHaveBeenCalled();
  });

  it("does not disguise PII or payment data while normalizing dimensions", () => {
    expect(safeActivationToken("owner@example.test")).toBe("unknown");
    expect(safeActivationToken("email=owner@example.test")).toBe("unknown");
    expect(safeActivationToken("4242 4242 4242 4242")).toBe("unknown");
    expect(safeActivationToken("paid-social/launch 2026")).toBe(
      "paid-social/launch_2026",
    );
  });

  it("is exactly once across concurrent rerender, retry, and refreshed emitter", async () => {
    const storage = new MemoryStorage();
    const transport = jest
      .fn()
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValue(undefined);
    const options = {
      transport,
      storage,
      hasConsent: () => true,
      randomUUID: () => "728a70ef-0e4f-49b7-9978-229b7bc2be59",
    };
    const first = createActivationEmitter(options);
    await expect(
      first.emit("registration_started", dimensions),
    ).rejects.toThrow("offline");
    await Promise.all([
      first.emit("registration_started", dimensions),
      first.emit("registration_started", dimensions),
    ]);
    expect(transport).toHaveBeenCalledTimes(2);

    const refreshed = createActivationEmitter(options);
    await expect(
      refreshed.emit("registration_started", dimensions),
    ).resolves.toBe("already_sent");
    expect(transport).toHaveBeenCalledTimes(2);
    expect(refreshed.funnelID()).toBe("728a70ef-0e4f-49b7-9978-229b7bc2be59");
  });
});
