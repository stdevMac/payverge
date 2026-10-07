import { getTranslation } from "./SimpleTranslationProvider";
import { reportMissingTranslation } from "./missingTranslationReporter";

jest.mock("./missingTranslationReporter", () => ({
  reportMissingTranslation: jest.fn(),
}));

describe("getTranslation param interpolation robustness", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  // A param key containing regex-special characters used to throw inside
  // applyParams (the key was interpolated raw into `new RegExp(...)`), which
  // dropped the request into the catch path and the humanized-leaf fallback.
  // applyParams now escapes the key, so a malformed param name is inert: the
  // translation resolves normally and no missing-translation telemetry fires.
  it("does not crash on regex-special param keys; resolves normally", () => {
    expect(getTranslation("navigation.home", "es", { "[": "bad" })).toBe(
      "Inicio",
    );
    expect(reportMissingTranslation).not.toHaveBeenCalled();
  });
});
