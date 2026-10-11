/** @jest-environment node */
import {
  isCorsOrNoResponseLoginFailure,
  mapLoginRequestError,
} from "./loginRequestError";

const TRANSPORT = "Couldn't reach the server. Check your connection and try again.";
const ORDINARY = "Invalid email or password.";

describe("isCorsOrNoResponseLoginFailure", () => {
  it("treats axios Network Error / ERR_NETWORK with no response as transport failure", () => {
    expect(
      isCorsOrNoResponseLoginFailure({
        message: "Network Error",
        code: "ERR_NETWORK",
      }),
    ).toBe(true);
  });

  it("treats ERR_FAILED with no response as transport failure", () => {
    expect(
      isCorsOrNoResponseLoginFailure({
        message: "net::ERR_FAILED",
        code: "ERR_FAILED",
      }),
    ).toBe(true);
  });

  it("treats the interceptor's sanitized network copy as a transport failure", () => {
    expect(
      isCorsOrNoResponseLoginFailure({
        message:
          "Unable to connect to the server. Please check your internet connection and try again.",
      }),
    ).toBe(true);
  });

  it("treats status 0 with no response as a transport failure", () => {
    expect(
      isCorsOrNoResponseLoginFailure({
        status: 0,
        code: "ERR_NETWORK",
        message: "Network Error",
      }),
    ).toBe(true);
  });

  it("does not treat a 401 body as a transport failure", () => {
    expect(
      isCorsOrNoResponseLoginFailure({
        status: 401,
        response: { status: 401, data: { error: "invalid" } },
        message: "Request failed with status code 401",
      }),
    ).toBe(false);
  });

  it("does not treat a generic JS error as a transport failure", () => {
    expect(
      isCorsOrNoResponseLoginFailure({
        message: "Something went wrong. Please try again.",
      }),
    ).toBe(false);
  });
});

describe("mapLoginRequestError", () => {
  it("maps CORS/no-response login failures to the honest transport copy", () => {
    expect(
      mapLoginRequestError(
        { message: "Network Error", code: "ERR_NETWORK" },
        ORDINARY,
        TRANSPORT,
      ),
    ).toBe(TRANSPORT);
    expect(
      mapLoginRequestError(
        {
          message:
            "Unable to connect to the server. Please check your internet connection and try again.",
        },
        ORDINARY,
        TRANSPORT,
      ),
    ).toBe(TRANSPORT);
  });

  it("keeps ordinary auth failures", () => {
    expect(
      mapLoginRequestError(
        {
          status: 401,
          response: { status: 401, data: { error: ORDINARY } },
        },
        ORDINARY,
        TRANSPORT,
      ),
    ).toBe(ORDINARY);
  });
});
