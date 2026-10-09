/** @jest-environment jsdom */
/**
 * Audit B3 — Drivers tab raw i18n keys.
 *
 * Prior behaviour: `DriversManager` (rendered under the Delivery > Drivers
 * tab on /business/<slug>) called `getTranslation("deliverySettings.drivers.*")`,
 * but the `deliverySettings.json` namespace nested driver labels under
 * `deliverySettings.dispatch.drivers.*`. `getTranslation` returns the raw
 * key when a path resolves to undefined, so operators saw dotted strings
 * like "deliverySettings.drivers.title" rendered as visible text.
 *
 * This test loads the *real* SimpleTranslationProvider/getTranslation
 * (no key-echo mock) and asserts that the rendered DOM contains no
 * strings that look like raw i18n keys.
 */

import React from "react";
import { render, waitFor } from "@testing-library/react";
import DriversManager from "../delivery/DriversManager";

// Delivery API: return one driver so the list path renders (status chip,
// vehicle label, availability switch, edit/remove buttons — every place
// where a missing key would have leaked).
jest.mock("@/api/delivery", () => ({
  VEHICLE_TYPES: ["bicycle", "scooter", "motorcycle", "car", "van"],
  deliveryApi: {
    getBusinessDrivers: jest.fn(() =>
      Promise.resolve([
        {
          id: 1,
          business_id: 1,
          name: "AI Driver 1",
          phone: "+1 202 555 0101",
          email: "",
          vehicle_type: "scooter",
          vehicle_plate: "",
          status: "online",
          is_available: true,
          is_active: true,
        },
      ]),
    ),
    createDriver: jest.fn(),
    updateDriver: jest.fn(),
    deleteDriver: jest.fn(),
  },
}));

jest.mock("@/contexts/ToastContext", () => {
  const showSuccess = jest.fn();
  const showError = jest.fn();
  return { useToast: () => ({ showSuccess, showError }) };
});

// Matches "foo.bar.baz" style dotted keys (at least two dots, lowercase
// camelCase segments). Anything that survives the i18n provider unresolved
// will look like this.
const I18N_KEY_RE = /^[a-z][a-zA-Z0-9]*(\.[a-zA-Z][a-zA-Z0-9_]*){2,}$/;

describe("Drivers tab i18n", () => {
  it("renders no raw deliverySettings translation keys", async () => {
    const { container, findByText } = render(<DriversManager businessId={1} />);

    // Wait for the driver row to appear so the list path actually renders
    // (status chip, vehicle label, availability switch, action buttons).
    await findByText("AI Driver 1");

    await waitFor(() => {
      const text = container.textContent || "";
      const leaks = text
        .split(/\s+/)
        .filter((s) => I18N_KEY_RE.test(s))
        .filter((s) => s.startsWith("deliverySettings."));
      expect(leaks).toEqual([]);
    });
  });
});
