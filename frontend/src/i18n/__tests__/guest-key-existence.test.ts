/**
 * CI net for the i18n-bypass audit class: every literal t("...") key used in
 * guest-facing code must exist in guest-messages/en.json.
 *
 * Enforced post-campaign (T17): every literal t() key resolved by the lanes is
 * present in guest-messages/en.json — `runKeysCheck().missing` is empty, so this
 * net runs unskipped to catch any future i18n-bypass regression.
 */
const { runKeysCheck } = require("../../../scripts/check-guest-locales.js");

describe("guest t() key existence", () => {
  it("finds no literal t() keys missing from en.json", () => {
    const { missing } = runKeysCheck();
    expect(missing).toEqual([]);
  });
});
