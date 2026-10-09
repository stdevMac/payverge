/**
 * @jest-environment jsdom
 *
 * The axios instance and the wagmi config must not freeze public config at
 * module evaluation: the runtime config script (window.__PAYVERGE_ENV__) can
 * run after the client chunk that imports them.
 */
import MockAdapter from "axios-mock-adapter";

jest.mock("wagmi", () => ({
  WagmiProvider: ({ children }: { children: unknown }) => children,
  createConfig: (opts: unknown) => ({ opts }),
  http: (url: string) => ({ url }),
  injected: () => ({ id: "injected" }),
  useAccount: () => ({}),
  useConnect: () => ({ connectors: [], connect: jest.fn() }),
  useDisconnect: () => ({ disconnect: jest.fn() }),
}));

describe("late runtime public config", () => {
  afterEach(() => {
    delete (window as { __PAYVERGE_ENV__?: unknown }).__PAYVERGE_ENV__;
  });

  it("axios requests use an API_URL injected after the module loaded", async () => {
    delete (window as { __PAYVERGE_ENV__?: unknown }).__PAYVERGE_ENV__;
    const { axiosInstance } = await import("../instance");
    const mock = new MockAdapter(axiosInstance);
    let seenUrl = "";
    mock.onGet().reply((config) => {
      seenUrl = `${config.baseURL}${config.url}`;
      return [200, {}];
    });

    window.__PAYVERGE_ENV__ = Object.freeze({
      API_URL: "https://api.late.example/api/v1",
    });
    await axiosInstance.get("/health/live");
    expect(seenUrl).toBe("https://api.late.example/api/v1/health/live");
    mock.restore();
  });

  it("fiscal receipt PDF links use the late API_URL", async () => {
    const { guestFiscalReceiptPdfUrl } = await import("../../bills");
    window.__PAYVERGE_ENV__ = Object.freeze({
      API_URL: "https://api.late.example/api/v1",
    });
    expect(guestFiscalReceiptPdfUrl("tok")).toBe(
      "https://api.late.example/api/v1/guest/bill/tok/fiscal-receipt/pdf",
    );
  });

  it("wagmi transports use an RPC_URL injected after the module loaded", async () => {
    const provider = await import("@/providers/DynamicProvider");
    provider.__resetWagmiConfigForTests();
    window.__PAYVERGE_ENV__ = Object.freeze({
      NETWORK: "base",
      RPC_URL: "https://rpc.late.example",
    });
    const config = provider.getWagmiConfig() as unknown as {
      opts: { transports: Record<number, { url: string }> };
    };
    expect(config.opts.transports[8453].url).toBe("https://rpc.late.example");
    expect(provider.getWagmiConfig()).toBe(config);
  });
});
