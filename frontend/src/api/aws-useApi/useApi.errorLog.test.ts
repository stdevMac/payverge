import { signOutFromSession } from "./useApi";

jest.mock("react-hot-toast", () => ({ __esModule: true, default: { error: jest.fn() } }));
jest.mock("@/utils/errorLogger", () => ({ logError: jest.fn().mockResolvedValue(undefined) }));

const mockPost = jest.fn();
jest.mock("../tools/instance", () => ({
  axiosInstance: { post: (...a: unknown[]) => mockPost(...a) },
}));

describe("handleApiError production logging (Q-4)", () => {
  const ORIGINAL_ENV = process.env.NODE_ENV;
  let errorSpy: jest.SpyInstance;

  beforeEach(() => {
    errorSpy = jest.spyOn(console, "error").mockImplementation(() => {});
  });
  afterEach(() => {
    errorSpy.mockRestore();
    (process.env as Record<string, string | undefined>).NODE_ENV = ORIGINAL_ENV;
  });

  it("does NOT log the raw response body object in production", async () => {
    (process.env as Record<string, string | undefined>).NODE_ENV = "production";
    const secretBody = { error: "bad", token: "leak-me", identity: { email: "a@b.c" } };
    mockPost.mockRejectedValueOnce({ response: { status: 401, data: secretBody } });

    await expect(signOutFromSession()).rejects.toMatchObject({
      response: { status: 401, data: secretBody },
    });

    const loggedArgs = errorSpy.mock.calls.flat();
    // The raw body object must never be among the logged arguments in prod.
    expect(loggedArgs).not.toContain(secretBody);
    // A stable message that includes the source + status IS logged.
    const joined = errorSpy.mock.calls.map((c) => c.map(String).join(" ")).join("\n");
    expect(joined).toMatch(/signOut/);
    expect(joined).toMatch(/401/);
  });

  it("logs the full response body object in development", async () => {
    (process.env as Record<string, string | undefined>).NODE_ENV = "development";
    const body = { error: "bad" };
    mockPost.mockRejectedValueOnce({ response: { status: 401, data: body } });

    await expect(signOutFromSession()).rejects.toMatchObject({
      response: { status: 401, data: body },
    });

    const loggedArgs = errorSpy.mock.calls.flat();
    expect(loggedArgs).toContain(body);
  });
});
