import type { ErrorEvent, Log, Options, TransactionEvent } from "@sentry/core";
import {
  getFrontendSentryRuntimeConfig,
  parseBoolean,
  parseSampleRate,
  sanitizeObject,
  sanitizeUrl,
  scrubSentryEvent,
  scrubSentryLog,
  scrubSentryReplayRecordingEvent,
  scrubSentryTransaction,
} from "@/lib/sentry/config";
import { formatPeerStamp, PEER_STAMP_HEADER } from "@/lib/proxy/peerStamp";

const originalEnv = process.env;

describe("frontend sentry config", () => {
  beforeEach(() => {
    process.env = { ...originalEnv };
    delete process.env.NEXT_PUBLIC_SENTRY_DSN;
    delete process.env.NEXT_PUBLIC_SENTRY_ENABLED;
    delete process.env.NEXT_PUBLIC_SENTRY_ENVIRONMENT;
    delete process.env.NEXT_PUBLIC_SENTRY_RELEASE;
    delete process.env.NEXT_PUBLIC_SENTRY_TRACES_SAMPLE_RATE;
    delete process.env.NEXT_PUBLIC_SENTRY_REPLAYS_SESSION_SAMPLE_RATE;
    delete process.env.NEXT_PUBLIC_SENTRY_REPLAYS_ON_ERROR_SAMPLE_RATE;
  });

  afterAll(() => {
    process.env = originalEnv;
  });

  it("enables Sentry only for staging or production when a DSN is present", () => {
    process.env.NEXT_PUBLIC_SENTRY_DSN = "https://public@example.ingest.sentry.io/1";
    process.env.NEXT_PUBLIC_SENTRY_ENVIRONMENT = "development";
    expect(getFrontendSentryRuntimeConfig().enabled).toBe(false);

    process.env.NEXT_PUBLIC_SENTRY_ENVIRONMENT = "staging";
    expect(getFrontendSentryRuntimeConfig().enabled).toBe(true);

    process.env.NEXT_PUBLIC_SENTRY_ENVIRONMENT = "production";
    expect(getFrontendSentryRuntimeConfig().enabled).toBe(true);
  });

  it("lets explicit enable flags override environment gating", () => {
    process.env.NEXT_PUBLIC_SENTRY_DSN = "https://public@example.ingest.sentry.io/1";
    process.env.NEXT_PUBLIC_SENTRY_ENVIRONMENT = "development";
    process.env.NEXT_PUBLIC_SENTRY_ENABLED = "true";
    expect(getFrontendSentryRuntimeConfig().enabled).toBe(true);

    process.env.NEXT_PUBLIC_SENTRY_ENVIRONMENT = "production";
    process.env.NEXT_PUBLIC_SENTRY_ENABLED = "false";
    expect(getFrontendSentryRuntimeConfig().enabled).toBe(false);
  });

  it("requires a DSN even when Sentry is explicitly enabled", () => {
    process.env.NEXT_PUBLIC_SENTRY_ENVIRONMENT = "production";
    process.env.NEXT_PUBLIC_SENTRY_ENABLED = "true";

    expect(getFrontendSentryRuntimeConfig().enabled).toBe(false);
  });

  it("normalizes the prod alias to production for gating and reporting", () => {
    process.env.NEXT_PUBLIC_SENTRY_DSN = "https://public@example.ingest.sentry.io/1";
    process.env.NEXT_PUBLIC_SENTRY_ENVIRONMENT = "prod";

    const config = getFrontendSentryRuntimeConfig();
    expect(config.enabled).toBe(true);
    expect(config.environment).toBe("production");
  });

  it("parses booleans and sample rates conservatively", () => {
    expect(parseBoolean("true")).toBe(true);
    expect(parseBoolean("1")).toBe(true);
    expect(parseBoolean("yes")).toBe(true);
    expect(parseBoolean("false")).toBe(false);
    expect(parseBoolean("0")).toBe(false);
    expect(parseBoolean("")).toBeUndefined();
    expect(parseSampleRate("0.25", 0.1)).toBe(0.25);
    expect(parseSampleRate("2", 0.1)).toBe(0.1);
    expect(parseSampleRate("-1", 0.1)).toBe(0.1);
    expect(parseSampleRate("bad", 0.1)).toBe(0.1);
  });

  it("defaults replay sample rates to zero until an operator opts in", () => {
    process.env.NEXT_PUBLIC_SENTRY_DSN = "https://public@example.ingest.sentry.io/1";
    process.env.NEXT_PUBLIC_SENTRY_ENVIRONMENT = "production";

    expect(getFrontendSentryRuntimeConfig()).toMatchObject({
      replaysSessionSampleRate: 0,
      replaysOnErrorSampleRate: 0,
    });
  });

  it("reads public Sentry environment configuration", () => {
    process.env.NEXT_PUBLIC_SENTRY_DSN = " https://public@example.ingest.sentry.io/1 ";
    process.env.NEXT_PUBLIC_SENTRY_ENVIRONMENT = " staging ";
    process.env.NEXT_PUBLIC_SENTRY_RELEASE = " payverge-web@1.2.3 ";
    process.env.NEXT_PUBLIC_SENTRY_TRACES_SAMPLE_RATE = "0.2";
    process.env.NEXT_PUBLIC_SENTRY_REPLAYS_SESSION_SAMPLE_RATE = "0.3";
    process.env.NEXT_PUBLIC_SENTRY_REPLAYS_ON_ERROR_SAMPLE_RATE = "0.4";

    expect(getFrontendSentryRuntimeConfig()).toMatchObject({
      dsn: "https://public@example.ingest.sentry.io/1",
      enabled: true,
      environment: "staging",
      release: "payverge-web@1.2.3",
      tracesSampleRate: 0.2,
      replaysSessionSampleRate: 0.3,
      replaysOnErrorSampleRate: 0.4,
    });
  });

  it("strips query strings and fragments from URLs", () => {
    expect(sanitizeUrl("https://payverge.io/dashboard?token=secret#frag")).toBe(
      "https://payverge.io/dashboard",
    );
    expect(sanitizeUrl("/business/42/orders?email=a@example.com")).toBe(
      "/business/42/orders",
    );
  });

  it("filters wallet addresses from URL paths while stripping query strings and fragments", () => {
    expect(
      sanitizeUrl(
        "https://payverge.io/participants/0x1111111111111111111111111111111111111111?token=secret#frag",
      ),
    ).toBe("https://payverge.io/participants/[Filtered]");
    expect(
      sanitizeUrl(
        "/business/42/participants/0x2222222222222222222222222222222222222222/orders?email=a@example.com",
      ),
    ).toBe("/business/42/participants/[Filtered]/orders");
  });

  it("filters guest route access tokens from URL paths", () => {
    expect(sanitizeUrl("/api/v1/table/TABLE-guest-abc123?token=secret")).toBe(
      "/api/v1/table/[Filtered]",
    );
    expect(sanitizeUrl("/t/TABLE-guest-abc123/bill?token=secret")).toBe(
      "/t/[Filtered]/bill",
    );
    expect(
      sanitizeUrl("https://payverge.io/reservations/RES-CONF-abc123/confirm"),
    ).toBe("https://payverge.io/reservations/[Filtered]/confirm");
    expect(
      sanitizeUrl("https://payverge.io/delivery/DEL-guest-abc123/track?x=y"),
    ).toBe("https://payverge.io/delivery/[Filtered]/track");
  });

  it("filters guest route access tokens from path-like event fields", () => {
    expect(
      sanitizeObject({
        path: "/t/TABLE-guest-abc123/bill?token=secret",
        pathname: "/reservations/RES-CONF-abc123/confirm",
        route: "/delivery/DEL-guest-abc123/track",
      }),
    ).toEqual({
      path: "/t/[Filtered]/bill",
      pathname: "/reservations/[Filtered]/confirm",
      route: "/delivery/[Filtered]/track",
    });

    const transaction = scrubSentryTransaction({
      type: "transaction",
      transaction: "/t/TABLE-guest-abc123/bill?token=secret",
    } satisfies TransactionEvent);

    expect(transaction?.transaction).toBe("/t/[Filtered]/bill");
  });

  it("filters embedded wallet addresses and emails inside ordinary strings", () => {
    expect(
      sanitizeObject({
        payment_ref: "wallet 0x3333333333333333333333333333333333333333",
        note: "customer owner@example.com confirmed",
      }),
    ).toEqual({
      payment_ref: "wallet [Filtered]",
      note: "[Filtered]",
    });
  });

  it("scrubs sensitive request and user fields from events", () => {
    const event = scrubSentryEvent({
      type: undefined,
      user: {
        id: "user-42",
        email: "owner@example.com",
        username: "Owner Name",
        ip_address: "203.0.113.1",
      },
      request: {
        url: "https://payverge.io/checkout?token=abc",
        headers: {
          Authorization: "Bearer secret",
          "X-Request-Id": "req-1",
        },
        cookies: { session: "secret" },
        data: { password: "secret", ok: "value" },
        query_string: "token=abc",
      },
      extra: {
        payment_token: "tok_secret",
        business_id: 42,
      },
      contexts: {
        payverge: {
          customer_email: "guest@example.com",
          business_id: 42,
        },
      },
    } satisfies ErrorEvent);

    expect(event?.user).toEqual({ id: "user-42" });
    expect(event?.request?.url).toBe("https://payverge.io/checkout");
    expect(event?.request?.headers).toEqual({
      Authorization: "[Filtered]",
      "X-Request-Id": "req-1",
    });
    expect(event?.request?.cookies).toBeUndefined();
    expect(event?.request?.data).toBeUndefined();
    expect(event?.request?.query_string).toBeUndefined();
    expect(event?.extra?.payment_token).toBe("[Filtered]");
    expect(event?.extra?.business_id).toBe(42);
    expect(event?.contexts?.payverge?.customer_email).toBe("[Filtered]");
    expect(event?.contexts?.payverge?.business_id).toBe(42);
  });

  it("scrubs common PII keys from arbitrary frontend context", () => {
    const context = sanitizeObject({
      name: "Jane Guest",
      first_name: "Jane",
      lastName: "Guest",
      street: "123 Main St",
      address_line1: "Apt 4",
      postalCode: "90210",
      notes: "Guest is allergic to peanuts",
      business_id: 42,
      request_id: "req-1",
    });

    expect(context).toEqual({
      name: "[Filtered]",
      first_name: "[Filtered]",
      lastName: "[Filtered]",
      street: "[Filtered]",
      address_line1: "[Filtered]",
      postalCode: "[Filtered]",
      notes: "[Filtered]",
      business_id: 42,
      request_id: "req-1",
    });
  });

  it("scrubs proxy IP request headers", () => {
    const event = scrubSentryEvent({
      type: undefined,
      request: {
        headers: {
          "x-forwarded-for": "203.0.113.10, 198.51.100.2",
          "x-real-ip": "203.0.113.11",
          "cf-connecting-ip": "203.0.113.12",
          "true-client-ip": "203.0.113.13",
          forwarded: "for=203.0.113.14;proto=https",
          "x-request-id": "req-1",
        },
      },
    } satisfies ErrorEvent);

    expect(event?.request?.headers).toEqual({
      "x-forwarded-for": "[Filtered]",
      "x-real-ip": "[Filtered]",
      "cf-connecting-ip": "[Filtered]",
      "true-client-ip": "[Filtered]",
      forwarded: "[Filtered]",
      "x-request-id": "req-1",
    });
  });

  it("scrubs the proxy peer stamp (client IP and stamp nonce)", () => {
    const stamp = formatPeerStamp("Zk3nonce_abcdefghijklmn", "http", "203.0.113.7");
    const request = { headers: { [PEER_STAMP_HEADER]: stamp, "x-request-id": "req-2" } };

    const event = scrubSentryEvent({ type: undefined, request } satisfies ErrorEvent);
    const transaction = scrubSentryTransaction({
      type: "transaction",
      request,
    } satisfies TransactionEvent);

    for (const scrubbed of [event, transaction]) {
      expect(scrubbed?.request?.headers).toEqual({
        [PEER_STAMP_HEADER]: "[Filtered]",
        "x-request-id": "req-2",
      });
      expect(JSON.stringify(scrubbed)).not.toContain("203.0.113.7");
      expect(JSON.stringify(scrubbed)).not.toContain("Zk3nonce");
    }
  });

  it("drops console breadcrumbs from replay recordings", () => {
    expect(
      scrubSentryReplayRecordingEvent({
        type: 5,
        timestamp: Date.now(),
        data: {
          tag: "breadcrumb",
          payload: {
            timestamp: Date.now(),
            type: "default",
            category: "console",
            level: "error",
            message: "Error response: guest@example.com",
            data: {
              logger: "console",
              arguments: [
                "https://payverge.io/t/TABLE-guest-abc123/bill?token=secret",
                { name: "Jane Guest" },
              ],
            },
          },
        },
      }),
    ).toBeNull();
  });

  it("scrubs replay recording payloads that are not console breadcrumbs", () => {
    const event = scrubSentryReplayRecordingEvent({
      type: 5,
      timestamp: Date.now(),
      data: {
        tag: "breadcrumb",
        payload: {
          timestamp: Date.now(),
          type: "default",
          category: "navigation",
          message: "/t/TABLE-guest-abc123/bill?token=secret",
          data: {
            from: "/reservations/RES-CONF-abc123/confirm",
            name: "Jane Guest",
            request_id: "req-1",
          },
        },
      },
    });

    expect(event).toMatchObject({
      data: {
        payload: {
          message: "/t/[Filtered]/bill",
          data: {
            from: "/reservations/[Filtered]/confirm",
            name: "[Filtered]",
            request_id: "req-1",
          },
        },
      },
    });
  });

  it("scrubs Payverge customer, delivery, wallet, and IP fields", () => {
    const event = scrubSentryEvent({
      type: undefined,
      extra: {
        delivery_address: "123 Main St",
        customer_name: "Jane Guest",
        walletAddress: "0x1111111111111111111111111111111111111111",
        payer_address: "0x2222222222222222222222222222222222222222",
        settlement_address: "0x3333333333333333333333333333333333333333",
        client_ip: "203.0.113.10",
        external_ref: "0x4444444444444444444444444444444444444444",
        business_id: "business-42",
        user_id: "user-42",
      },
    } satisfies ErrorEvent);

    expect(event?.extra?.delivery_address).toBe("[Filtered]");
    expect(event?.extra?.customer_name).toBe("[Filtered]");
    expect(event?.extra?.walletAddress).toBe("[Filtered]");
    expect(event?.extra?.payer_address).toBe("[Filtered]");
    expect(event?.extra?.settlement_address).toBe("[Filtered]");
    expect(event?.extra?.client_ip).toBe("[Filtered]");
    expect(event?.extra?.external_ref).toBe("[Filtered]");
    expect(event?.extra?.business_id).toBe("business-42");
    expect(event?.extra?.user_id).toBe("user-42");
  });

  it("filters unsafe user IDs instead of restoring them raw", () => {
    const emailUser = scrubSentryEvent({
      type: undefined,
      user: { id: "owner@example.com" },
    } satisfies ErrorEvent);
    const walletUser = scrubSentryEvent({
      type: undefined,
      user: { id: "0x5555555555555555555555555555555555555555" },
    } satisfies ErrorEvent);

    expect(emailUser?.user).toBeUndefined();
    expect(walletUser?.user).toBeUndefined();
  });

  it("matches Sentry callback types", () => {
    const beforeSend: NonNullable<Options["beforeSend"]> = scrubSentryEvent;
    const beforeSendTransaction: NonNullable<Options["beforeSendTransaction"]> =
      scrubSentryTransaction;
    const beforeSendLog: NonNullable<Options["beforeSendLog"]> = scrubSentryLog;

    expect(beforeSend).toBe(scrubSentryEvent);
    expect(beforeSendTransaction).toBe(scrubSentryTransaction);
    expect(beforeSendLog).toBe(scrubSentryLog);
  });

  it("scrubs sensitive values from the log message body", () => {
    const scrubbed = scrubSentryLog({
      level: "info",
      message:
        "login from guest@example.com using Bearer abc.def.ghi wallet 0x5555555555555555555555555555555555555555" as Log["message"],
    });

    expect(scrubbed?.message).not.toContain("guest@example.com");
    expect(scrubbed?.message).not.toContain("Bearer abc.def.ghi");
    expect(scrubbed?.message).not.toContain(
      "0x5555555555555555555555555555555555555555",
    );
    expect(scrubbed?.message).toContain("[Filtered]");
  });

  it("filters sensitive log attributes but preserves safe ones", () => {
    const scrubbed = scrubSentryLog({
      level: "error",
      message: "checkout failed" as Log["message"],
      attributes: {
        business_id: 42,
        email: "owner@example.com",
        note: "contact guest@example.com about the order",
        authorization: "Bearer secret-token",
      },
    });

    expect(scrubbed?.attributes).toMatchObject({ business_id: 42 });
    expect(scrubbed?.attributes?.email).toBe("[Filtered]");
    expect(scrubbed?.attributes?.authorization).toBe("[Filtered]");
    expect(scrubbed?.attributes?.note).toBe("[Filtered]");
  });
});
