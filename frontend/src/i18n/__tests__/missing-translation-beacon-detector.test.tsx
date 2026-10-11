/** @jest-environment jsdom */
import React from "react";
import { render } from "@testing-library/react";

import { getTranslation } from "../getTranslation";
import {
  GuestTranslationProvider,
  useGuestTranslation,
} from "../GuestTranslationProvider";
import { __resetReporterForTests } from "../missingTranslationReporter";
import {
  guestTelemetryKeys,
  operatorTelemetryKeys,
} from "./missing-translation-telemetry-regression.test";
import type { Locale } from "../localeRegistry";

const operatorLocales: Locale[] = ["en", "es", "es-AR"];

function GuestLookupProbe({ keys }: { keys: string[] }) {
  const { t } = useGuestTranslation();
  keys.forEach((key) => {
    t(key);
  });
  return null;
}

describe("production missing-translation beacon detector", () => {
  const originalSendBeacon = navigator.sendBeacon;
  let warnSpy: jest.SpyInstance;

  beforeEach(() => {
    __resetReporterForTests();
    warnSpy = jest.spyOn(console, "warn").mockImplementation(() => {});
    Object.defineProperty(navigator, "sendBeacon", {
      configurable: true,
      value: jest.fn(() => true),
    });
  });

  afterEach(() => {
    warnSpy.mockRestore();
    Object.defineProperty(navigator, "sendBeacon", {
      configurable: true,
      value: originalSendBeacon,
    });
  });

  it("does not emit prod telemetry beacons for the observed operator keys", () => {
    operatorLocales.forEach((locale) => {
      operatorTelemetryKeys.forEach((key) => {
        getTranslation(key, locale);
      });
    });

    expect(navigator.sendBeacon).not.toHaveBeenCalled();
  });

  it("does not emit prod telemetry beacons for the observed English guest keys", () => {
    render(
      <GuestTranslationProvider initialLanguage="en">
        <GuestLookupProbe keys={guestTelemetryKeys} />
      </GuestTranslationProvider>,
    );

    expect(navigator.sendBeacon).not.toHaveBeenCalled();
  });

  it("proves the detector fails when a lookup would hit the prod beacon path", () => {
    getTranslation("definitely.missing.operator.key", "en");

    expect(navigator.sendBeacon).toHaveBeenCalledTimes(1);
    expect(navigator.sendBeacon).toHaveBeenCalledWith(
      expect.stringContaining("/analytics/missing_translation"),
      expect.any(Blob),
    );
  });
});
