/** @jest-environment jsdom */
/**
 * Money-safety coverage for StripeConfig (Round 4 audit, Task 3).
 *
 * Two gaps this suite locks in:
 *   1. Pasting an `sk_live_` key must NOT silently flip the integration into
 *      live mode. The component must keep the user-chosen mode and surface an
 *      explicit warning banner instead.
 *   2. The secret key input must have a working show/hide toggle so operators
 *      can verify keys without screen-share exposure.
 */

import React from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

// Mock translation provider — return the key so assertions stay predictable.
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

// Keep next/image simple: render a plain <img> in tests.
jest.mock("next/image", () => ({
  __esModule: true,
  default: (props: Record<string, unknown>) => {
    // eslint-disable-next-line @next/next/no-img-element, jsx-a11y/alt-text
    return <img {...(props as any)} />;
  },
}));

import StripeConfig from "../StripeConfig";

const basePlugin = {
  id: 1,
  name: "stripe",
  display_name: "Stripe",
  description: "Stripe payments",
  image: "/images/plugins/stripe-logo.png",
  category: "payments",
  version: "1.0.0",
  features: "",
  is_enabled: false,
  config: "{}",
};

function renderStripeConfig(initialConfig: Record<string, any> = {}) {
  const onConfigChange = jest.fn();
  const onSave = jest.fn();
  const onCancel = jest.fn();
  const utils = render(
    <StripeConfig
      plugin={basePlugin}
      config={initialConfig}
      onConfigChange={onConfigChange}
      onSave={onSave}
      onCancel={onCancel}
    />,
  );
  return { ...utils, onConfigChange, onSave, onCancel };
}

const LIVE_SECRET = "sk_live_examplelivekey1234567890";

describe("StripeConfig live-key UX hardening", () => {
  it("does not auto-switch to live mode when a live secret key is typed", async () => {
    const user = userEvent.setup();
    renderStripeConfig();

    const secretInput = screen.getByPlaceholderText(
      /secretKeyPlaceholder/i,
    ) as HTMLInputElement;

    await user.type(secretInput, LIVE_SECRET);

    // The NextUI Switch is an accessible switch role; it must remain in the
    // default "test mode" position (not selected = test mode in this UI).
    const modeSwitch = screen.getByRole("switch");
    expect(modeSwitch).not.toBeChecked();
  });

  it("renders an alert banner when a live key is pasted while still in test mode", async () => {
    const user = userEvent.setup();
    renderStripeConfig();

    const secretInput = screen.getByPlaceholderText(
      /secretKeyPlaceholder/i,
    ) as HTMLInputElement;

    await user.type(secretInput, LIVE_SECRET);

    const alert = screen.getByRole("alert");
    expect(alert).toBeInTheDocument();
    expect(alert).toHaveTextContent(/live key detected/i);
  });

  it("toggles the secret input between password and text when the eye button is clicked", async () => {
    const user = userEvent.setup();
    renderStripeConfig({ secret_key: LIVE_SECRET });

    const secretInput = screen.getByPlaceholderText(
      /secretKeyPlaceholder/i,
    ) as HTMLInputElement;
    expect(secretInput.type).toBe("password");

    // aria-label now routes through t() (translation mock returns the key),
    // so match the i18n key suffix instead of the literal English.
    const toggle = screen.getByRole("button", { name: /showSecretKey/i });
    await user.click(toggle);

    expect(secretInput.type).toBe("text");
    // After toggling, the button's aria-label flips to the hide state.
    expect(
      screen.getByRole("button", { name: /hideSecretKey/i }),
    ).toBeInTheDocument();
  });

  // Round-5 D-C3: `isTestMode` used to be local-only state. It ignored the
  // persisted `mode` on mount and never propagated flips back through
  // `onConfigChange`, so the UI could silently disagree with the saved
  // config. The two tests below lock that down.
  it("hydrates the mode switch from initialConfig.mode = 'live' on mount", () => {
    renderStripeConfig({ mode: "live" });

    const modeSwitch = screen.getByRole("switch");
    // In this UI, "selected" = live mode. A persisted live config must
    // render the switch in the selected position, not the default test
    // position.
    expect(modeSwitch).toBeChecked();
  });

  it("propagates mode flips back through onConfigChange", async () => {
    const user = userEvent.setup();
    const { onConfigChange } = renderStripeConfig({ mode: "test" });

    const modeSwitch = screen.getByRole("switch");
    expect(modeSwitch).not.toBeChecked();

    await user.click(modeSwitch);

    // The last call to onConfigChange must include the updated `mode`
    // field so the save path persists the flip. Without this propagation
    // the toggle would be purely cosmetic.
    expect(onConfigChange).toHaveBeenCalled();
    const lastCallArgs =
      onConfigChange.mock.calls[onConfigChange.mock.calls.length - 1][0];
    expect(lastCallArgs).toEqual(
      expect.objectContaining({ mode: "live" }),
    );
  });

  it("uses the shipped stripe logo when plugin.image is empty", () => {
    render(
      <StripeConfig
        plugin={{ ...basePlugin, image: "" }}
        config={{}}
        onConfigChange={jest.fn()}
        onSave={jest.fn()}
        onCancel={jest.fn()}
      />,
    );

    expect(screen.getByRole("img", { name: "Stripe" })).toHaveAttribute(
      "src",
      expect.stringContaining("stripe-logo.png"),
    );
  });
});
