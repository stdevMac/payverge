import { detectPwaPlatform } from "./platform";

const iosSafari = {
  userAgent:
    "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 Version/18.0 Mobile/15E148 Safari/604.1",
  platform: "iPhone",
  maxTouchPoints: 5,
};

describe("detectPwaPlatform", () => {
  it("treats navigator standalone as installed", () => {
    expect(
      detectPwaPlatform({
        ...iosSafari,
        standalone: true,
        displayModeStandalone: false,
      }),
    ).toBe("installed");
  });

  it("treats display-mode standalone as installed", () => {
    expect(
      detectPwaPlatform({
        ...iosSafari,
        standalone: false,
        displayModeStandalone: true,
      }),
    ).toBe("installed");
  });

  it("recognizes iPhone and touch-capable iPadOS Safari as manual install", () => {
    expect(
      detectPwaPlatform({
        ...iosSafari,
        standalone: false,
        displayModeStandalone: false,
      }),
    ).toBe("manual-install");
    expect(
      detectPwaPlatform({
        userAgent:
          "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15) AppleWebKit/605.1.15 Version/18.0 Safari/605.1.15",
        platform: "MacIntel",
        maxTouchPoints: 5,
        standalone: false,
        displayModeStandalone: false,
      }),
    ).toBe("manual-install");
  });

  it.each(["CriOS/126.0", "FxiOS/127.0", "EdgiOS/126.0", "OPiOS/2.0"])(
    "does not claim %s on iOS can install",
    (browserToken) => {
      expect(
        detectPwaPlatform({
          ...iosSafari,
          userAgent: iosSafari.userAgent.replace("Version/18.0", browserToken),
          standalone: false,
          displayModeStandalone: false,
        }),
      ).toBe("unavailable");
    },
  );

  it.each([
    ["DuckDuckGo", "Ddg/7.0"],
    ["Google app", "GSA/375.0"],
    ["Facebook in-app", "FBAN/FBIOS"],
  ])("does not claim %s on iOS can install", (_browser, appToken) => {
    expect(
      detectPwaPlatform({
        ...iosSafari,
        userAgent: `${iosSafari.userAgent} ${appToken}`,
        standalone: false,
        displayModeStandalone: false,
      }),
    ).toBe("unavailable");
  });
});
