import * as Sentry from "@sentry/nextjs";
import {
  captureClientError,
  clearSentryIdentity,
  setSentryIdentity,
  toError,
} from "@/lib/sentry/reporting";

const mockScopeSetContext = jest.fn();
const mockScopeSetTag = jest.fn();

jest.mock("@sentry/nextjs", () => ({
  captureException: jest.fn(),
  setContext: jest.fn(),
  setTag: jest.fn(),
  setUser: jest.fn(),
  withScope: jest.fn((callback: (scope: unknown) => void) =>
    callback({
      setContext: mockScopeSetContext,
      setTag: mockScopeSetTag,
    }),
  ),
}));

const originalEnv = process.env;

describe("frontend sentry reporting", () => {
  beforeEach(() => {
    process.env = {
      ...originalEnv,
      NEXT_PUBLIC_SENTRY_DSN: "https://public@example.ingest.sentry.io/1",
      NEXT_PUBLIC_SENTRY_ENVIRONMENT: "production",
      NEXT_PUBLIC_SENTRY_ENABLED: "true",
    };
    jest.clearAllMocks();
    clearSentryIdentity();
    jest.clearAllMocks();
  });

  afterAll(() => {
    process.env = originalEnv;
  });

  it("normalizes strings to Error objects", () => {
    const error = toError("boom");

    expect(error).toBeInstanceOf(Error);
    expect(error.message).toBe("boom");
  });

  it("captures client errors with tags and sanitized context", () => {
    const rawError = new Error("checkout failed");

    captureClientError({
      error: rawError,
      component: "CheckoutButton",
      functionName: "submit",
      requestId: "req-1",
      additionalInfo: {
        business_id: 42,
        token: "secret",
        email: "guest@example.com",
      },
    });

    expect(Sentry.withScope).toHaveBeenCalledTimes(1);
    expect(Sentry.captureException).toHaveBeenCalledWith(rawError);
    expect(mockScopeSetTag).toHaveBeenCalledWith("component", "CheckoutButton");
    expect(mockScopeSetTag).toHaveBeenCalledWith("function", "submit");
    expect(mockScopeSetTag).toHaveBeenCalledWith("request_id", "req-1");
    expect(mockScopeSetContext).toHaveBeenCalledWith("payverge", {
      business_id: 42,
      token: "[Filtered]",
      email: "[Filtered]",
    });
  });

  it("does not capture client errors when disabled", () => {
    process.env.NEXT_PUBLIC_SENTRY_ENABLED = "false";

    captureClientError({
      error: "boom",
      component: "CheckoutButton",
      functionName: "submit",
    });

    expect(Sentry.withScope).not.toHaveBeenCalled();
    expect(Sentry.captureException).not.toHaveBeenCalled();
  });

  it("sets staff Sentry identity without sensitive fields", () => {
    setSentryIdentity({
      authSource: "staff",
      staffId: 7,
      businessId: 42,
    });

    expect(Sentry.setUser).toHaveBeenCalledWith({ id: "staff:7" });
    expect(Sentry.setTag).toHaveBeenCalledWith("auth_source", "staff");
    expect(Sentry.setTag).toHaveBeenCalledWith("business_id", "42");
    expect(Sentry.setContext).toHaveBeenCalledWith("payverge_identity", {
      auth_source: "staff",
      staff_id: 7,
      business_id: 42,
    });
  });

  it("sets Web3 Sentry identity with a stable non-wallet user id", () => {
    const walletAddress = "0x1111111111111111111111111111111111111111";

    setSentryIdentity({
      authSource: "web3",
      walletAddress,
    });

    expect(Sentry.setUser).toHaveBeenCalledWith({
      id: expect.stringMatching(/^web3:[a-z0-9]+$/),
    });

    const sentryUser = (Sentry.setUser as jest.Mock).mock.calls[0][0];
    expect(sentryUser.id).not.toContain(walletAddress);
    expect(sentryUser.id).not.toContain(walletAddress.toLowerCase());
    expect(Sentry.setContext).toHaveBeenCalledWith("payverge_identity", {
      auth_source: "web3",
    });
  });

  it("uses the same Web3 Sentry id for wallet addresses with different casing", () => {
    const walletAddress = "0xAbCdEf0000000000000000000000000000000000";

    setSentryIdentity({
      authSource: "web3",
      walletAddress,
    });
    const firstId = (Sentry.setUser as jest.Mock).mock.calls[0][0].id;

    clearSentryIdentity();
    jest.clearAllMocks();

    setSentryIdentity({
      authSource: "web3",
      walletAddress: walletAddress.toUpperCase(),
    });
    const secondId = (Sentry.setUser as jest.Mock).mock.calls[0][0].id;

    expect(secondId).toBe(firstId);
  });

  it("clears Sentry identity", () => {
    clearSentryIdentity();

    expect(Sentry.setUser).toHaveBeenCalledWith(null);
    expect(Sentry.setTag).toHaveBeenCalledWith("auth_source", undefined);
    expect(Sentry.setTag).toHaveBeenCalledWith("business_id", undefined);
    expect(Sentry.setContext).toHaveBeenCalledWith("payverge_identity", null);
  });

  it("clears stale business tags when setting an identity without a business", () => {
    setSentryIdentity({
      authSource: "staff",
      staffId: 7,
      businessId: 42,
    });
    jest.clearAllMocks();

    setSentryIdentity({
      authSource: "user",
      userId: 11,
    });

    expect(Sentry.setUser).toHaveBeenCalledWith({ id: "user:11" });
    expect(Sentry.setTag).toHaveBeenCalledWith("auth_source", "user");
    expect(Sentry.setTag).toHaveBeenCalledWith("business_id", undefined);
    expect(Sentry.setContext).toHaveBeenCalledWith("payverge_identity", {
      auth_source: "user",
      user_id: 11,
    });
  });

  it("updates the active identity when its safe metadata changes", () => {
    setSentryIdentity({
      authSource: "staff",
      staffId: 7,
      businessId: 42,
    });
    jest.clearAllMocks();

    setSentryIdentity({
      authSource: "staff",
      staffId: 7,
    });

    expect(Sentry.setUser).toHaveBeenCalledWith({ id: "staff:7" });
    expect(Sentry.setTag).toHaveBeenCalledWith("auth_source", "staff");
    expect(Sentry.setTag).toHaveBeenCalledWith("business_id", undefined);
    expect(Sentry.setContext).toHaveBeenCalledWith("payverge_identity", {
      auth_source: "staff",
      staff_id: 7,
    });
  });

  it("does not clear Sentry identity for a different auth source", () => {
    setSentryIdentity({
      authSource: "staff",
      staffId: 7,
      businessId: 42,
    });
    jest.clearAllMocks();

    clearSentryIdentity("customer");

    expect(Sentry.setUser).not.toHaveBeenCalled();
    expect(Sentry.setTag).not.toHaveBeenCalled();
    expect(Sentry.setContext).not.toHaveBeenCalled();
  });

  it("does not let customer identity overwrite an active staff identity", () => {
    setSentryIdentity({
      authSource: "staff",
      staffId: 7,
      businessId: 42,
    });
    jest.clearAllMocks();

    setSentryIdentity({
      authSource: "customer",
      customerId: 21,
    });

    expect(Sentry.setUser).not.toHaveBeenCalled();
    expect(Sentry.setTag).not.toHaveBeenCalled();
    expect(Sentry.setContext).not.toHaveBeenCalled();
  });

  it("applies staff identity over a stored customer identity", () => {
    setSentryIdentity({
      authSource: "customer",
      customerId: 21,
    });
    jest.clearAllMocks();

    setSentryIdentity({
      authSource: "staff",
      staffId: 7,
      businessId: 42,
    });

    expect(Sentry.setUser).toHaveBeenCalledWith({ id: "staff:7" });
    expect(Sentry.setTag).toHaveBeenCalledWith("auth_source", "staff");
    expect(Sentry.setTag).toHaveBeenCalledWith("business_id", "42");
    expect(Sentry.setContext).toHaveBeenCalledWith("payverge_identity", {
      auth_source: "staff",
      staff_id: 7,
      business_id: 42,
    });
  });

  it("re-applies stored customer identity after clearing staff identity", () => {
    setSentryIdentity({
      authSource: "customer",
      customerId: 21,
    });
    setSentryIdentity({
      authSource: "staff",
      staffId: 7,
      businessId: 42,
    });
    jest.clearAllMocks();

    clearSentryIdentity("staff");

    expect(Sentry.setUser).toHaveBeenCalledWith({ id: "customer:21" });
    expect(Sentry.setTag).toHaveBeenCalledWith("auth_source", "customer");
    expect(Sentry.setTag).toHaveBeenCalledWith("business_id", undefined);
    expect(Sentry.setContext).toHaveBeenCalledWith("payverge_identity", {
      auth_source: "customer",
      customer_id: 21,
    });
  });

  it("does not downgrade a specific customer identity to a generic customer identity", () => {
    setSentryIdentity({
      authSource: "customer",
      customerId: 21,
    });
    jest.clearAllMocks();

    setSentryIdentity({
      authSource: "customer",
    });

    expect(Sentry.setUser).not.toHaveBeenCalled();
    expect(Sentry.setTag).not.toHaveBeenCalled();
    expect(Sentry.setContext).not.toHaveBeenCalled();
  });
});
