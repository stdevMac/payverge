import { sanitizeErrorKey } from "@/utils/errorMessages";
import { localizedErrorMessage } from "@/utils/localizedError";

describe("sanitizeErrorKey (K-10)", () => {
  it("maps HTTP statuses to stable keys", () => {
    expect(sanitizeErrorKey({ response: { status: 409 } }).key).toBe(
      "conflict",
    );
    expect(sanitizeErrorKey({ response: { status: 500 } }).key).toBe("server");
    expect(sanitizeErrorKey({ response: { status: 403 } }).key).toBe(
      "authorization",
    );
    expect(sanitizeErrorKey({ response: { status: 429 } }).key).toBe(
      "rateLimited",
    );
    expect(sanitizeErrorKey(new TypeError("boom")).key).toBe("default");
  });

  it("maps axios transport failures (no response) to network", () => {
    const err = Object.assign(new Error("Network Error"), {
      isAxiosError: true,
    });
    expect(sanitizeErrorKey(err).key).toBe("network");
  });

  it("still surfaces status and backend code for callers that branch on them", () => {
    const out = sanitizeErrorKey({
      response: { status: 409, data: { code: "SOME_CODE" } },
    });
    expect(out.status).toBe(409);
    expect(out.code).toBe("SOME_CODE");
  });
});

describe("localizedErrorMessage (K-10)", () => {
  it("returns Spanish copy for es", () => {
    const msg = localizedErrorMessage({ response: { status: 500 } }, "es");
    expect(msg).toBe(
      "Algo salió mal de nuestro lado. Inténtalo de nuevo más tarde.",
    );
  });

  it("returns English copy for en", () => {
    const msg = localizedErrorMessage({ response: { status: 500 } }, "en");
    expect(msg).toBe(
      "Something went wrong on our end. Please try again later.",
    );
  });
});
