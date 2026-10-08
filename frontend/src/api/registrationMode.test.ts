import {
  fetchRegistrationMode,
  isRegistrationClosedError,
  parseRegistrationMode,
  peekRegistrationMode,
  resetRegistrationModeCacheForTests,
} from "@/api/registrationMode";
import { axiosInstance } from "@/api/tools/instance";

jest.mock("@/api/tools/instance", () => ({
  axiosInstance: { get: jest.fn() },
}));

const get = axiosInstance.get as jest.Mock;

describe("registration mode api", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    resetRegistrationModeCacheForTests();
  });

  it("parses only the three backend modes", () => {
    expect(parseRegistrationMode("invite")).toBe("invite");
    expect(parseRegistrationMode(" OPEN ")).toBe("open");
    expect(parseRegistrationMode("closed")).toBe("closed");
    expect(parseRegistrationMode("public")).toBeNull();
    expect(parseRegistrationMode(undefined)).toBeNull();
    expect(parseRegistrationMode(1)).toBeNull();
  });

  it("reads the public probe once per page load, without toasts or refresh", async () => {
    get.mockResolvedValue({ data: { registration_mode: "open" } });

    const [a, b] = await Promise.all([fetchRegistrationMode(), fetchRegistrationMode()]);
    await expect(fetchRegistrationMode()).resolves.toBe("open");

    expect(a).toBe("open");
    expect(b).toBe("open");
    expect(peekRegistrationMode()).toBe("open");
    expect(get).toHaveBeenCalledTimes(1);
    expect(get).toHaveBeenCalledWith("/platform/registration-mode", {
      _skipErrorToast: true,
      _skipAuthRefresh: true,
    });
  });

  it("resolves null and retries later when the probe fails", async () => {
    get.mockRejectedValueOnce(new Error("offline"));
    await expect(fetchRegistrationMode()).resolves.toBeNull();
    expect(peekRegistrationMode()).toBeNull();

    get.mockResolvedValueOnce({ data: { registration_mode: "closed" } });
    await expect(fetchRegistrationMode()).resolves.toBe("closed");
    expect(get).toHaveBeenCalledTimes(2);
  });

  it("treats an unknown or missing value as unknown", async () => {
    get.mockResolvedValue({ data: {} });
    await expect(fetchRegistrationMode()).resolves.toBeNull();
    get.mockReturnValue(undefined);
    await expect(fetchRegistrationMode()).resolves.toBeNull();
  });

  it("recognizes the backend's closed-signup refusal", () => {
    expect(
      isRegistrationClosedError({
        error: "Registration is closed on this instance",
        params: { reason: "registration_closed" },
      }),
    ).toBe(true);
    expect(isRegistrationClosedError({ params: { reason: "invite_required" } })).toBe(false);
    expect(isRegistrationClosedError(undefined)).toBe(false);
    expect(isRegistrationClosedError("registration_closed")).toBe(false);
  });
});
