/** @jest-environment jsdom */
jest.mock("@/api/tools/instance", () => ({ axiosInstance: { get: jest.fn(() => new Promise(() => {})) } }));

import { render, screen } from "@testing-library/react";
import { GuestDemoPersonalDataNotice } from "../DemoPersonalDataNotice";
import { GuestTranslationProvider } from "@/i18n/GuestTranslationProvider";
import { resetInstanceCacheForTests, setInstanceForTests } from "@/hooks/useInstance";
import { parseInstanceInfo } from "@/lib/instance/instanceInfo";
import enGuest from "@/i18n/guest-messages/en.json";
import esGuest from "@/i18n/guest-messages/es.json";

function instance(mode: boolean) {
  return parseInstanceInfo({
    registration_mode: "closed",
    features: {},
    demo: { enabled: true, mode },
  });
}

afterEach(() => resetInstanceCacheForTests());

describe("DemoPersonalDataNotice", () => {
  it("has the notice copy in the guest message files", () => {
    expect(enGuest.common.demoPersonalDataNotice).toBe(
      "Public demo: don't enter real personal data — anyone can see it until the nightly reset.",
    );
    expect(esGuest.common.demoPersonalDataNotice).toMatch(/^Demo pública/);
  });

  it("renders nothing on a normal install", () => {
    setInstanceForTests(instance(false));
    const { container } = render(
      <GuestTranslationProvider>
        <GuestDemoPersonalDataNotice />
      </GuestTranslationProvider>,
    );
    expect(container).toBeEmptyDOMElement();
  });

  it("warns guests on the public demo", () => {
    setInstanceForTests(instance(true));
    render(
      <GuestTranslationProvider>
        <GuestDemoPersonalDataNotice />
      </GuestTranslationProvider>,
    );
    expect(screen.getByTestId("demo-personal-data-notice")).toHaveTextContent(
      "anyone can see it until the nightly reset",
    );
  });
});
