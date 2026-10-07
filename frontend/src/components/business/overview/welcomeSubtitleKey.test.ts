import { welcomeSubtitleKey } from "./welcomeSubtitleKey";

import en from "@/i18n/messages/en/businessDashboard.json";
import es from "@/i18n/messages/es/businessDashboard.json";
import esAR from "@/i18n/messages/es-ar/businessDashboard.json";

// Issue #795: "Your business is ready to accept payments" must be backed by
// backend plugin flags, never hardcoded.
describe("welcomeSubtitleKey", () => {
  const owner = { isStaffUser: false };

  it("claims payment-readiness only with a verified enabled rail", () => {
    expect(
      welcomeSubtitleKey({
        ...owner,
        paymentRailsKnown: true,
        anyPaymentRailEnabled: true,
      }),
    ).toBe("welcome.subtitle");
  });

  it("stays neutral when every payment rail is off (#795 demo venue)", () => {
    expect(
      welcomeSubtitleKey({
        ...owner,
        paymentRailsKnown: true,
        anyPaymentRailEnabled: false,
      }),
    ).toBe("welcome.subtitleNoPaymentRail");
  });

  it("stays neutral while plugin flags have not loaded", () => {
    expect(
      welcomeSubtitleKey({
        ...owner,
        paymentRailsKnown: false,
        anyPaymentRailEnabled: false,
      }),
    ).toBe("welcome.subtitleNoPaymentRail");
    // Stale "enabled" without loaded flags is not evidence either.
    expect(
      welcomeSubtitleKey({
        ...owner,
        paymentRailsKnown: false,
        anyPaymentRailEnabled: true,
      }),
    ).toBe("welcome.subtitleNoPaymentRail");
  });

  it("keeps role-specific staff subtitles untouched", () => {
    for (const role of ["kitchen", "host", "server", "manager"]) {
      expect(
        welcomeSubtitleKey({
          isStaffUser: true,
          staffRole: role,
          paymentRailsKnown: false,
          anyPaymentRailEnabled: false,
        }),
      ).toBe(`roleSpecific.${role}.welcome.subtitle`);
    }
  });

  it("has the neutral subtitle in every operator locale", () => {
    for (const [name, bundle] of Object.entries({ en, es, "es-ar": esAR })) {
      const welcome = (
        bundle as { overview?: { welcome?: Record<string, string> } }
      ).overview?.welcome;
      expect(welcome?.subtitleNoPaymentRail).toBeTruthy();
      // The neutral copy must not itself claim readiness.
      expect(welcome?.subtitleNoPaymentRail ?? "").not.toMatch(
        /ready to accept payments|listo para aceptar pagos/i,
      );
      expect(name).toBeTruthy();
    }
  });
});
