import fs from "node:fs";
import path from "node:path";

import { messages } from "@/i18n/getTranslation";

function at(root: Record<string, any>, dottedPath: string): string {
  const value = dottedPath.split(".").reduce<any>((node, key) => node?.[key], root);
  expect(typeof value).toBe("string");
  return value as string;
}

describe("advertised outcome claims stay evidence-scoped", () => {
  const locales = ["en", "es", "es-AR"] as const;

  test("operator copy avoids unsupported outcome percentages and speed guarantees", () => {
    const claims = [
      "aiMenuOnboarding.pdfDigitizer.processing.extractingSubtitle",
      "businessRegister.trustMessages.0",
      "businessDashboard.dashboard.menuBuilder.ai.emptyState.pdf.description",
      "businessDashboard.dashboard.pluginManager.config.paypal.connectAccountMessage",
      "businessDashboard.reservations.toggle.features.bookingDesc",
    ];

    for (const locale of locales) {
      for (const key of claims) {
        const copy = at(messages[locale], key);
        expect({ locale, key, copy }).toEqual(
          expect.objectContaining({
            copy: expect.not.stringMatching(
              /(?:\b24\/7\b|\b(?:up to|hasta)\s+(?:un\s+)?\d+%|\+\d+%|\b\d+\s*[–-]\s*\d+%|\b(?:under|menos de)\s+60\s+(?:seconds|segundos)|\b60s\b|\b(?:15\s*[–-]\s*30)\s+(?:seconds|segundos)|(?:ready to sell|list[oa] para vender|listo para vender)\s+(?:in|en)\s+seconds|(?:increase revenue|aumenta(?:r)? (?:los )?ingresos)[^.]*\d+%|\b(?:instant(?:ly)?|al instante)\b|\b(?:in|en)\s+minutes\b|\b(?:live|ready) before (?:the )?next (?:service|lunch)|\ben vivo antes del pr[oó]ximo (?:servicio|almuerzo)|(?:approximately|cerca del)\s+60%[^.]*restaurants|language is never a barrier|idioma nunca es una barrera|never sleeps|nunca duerme|no double bookings|sin reservas duplicadas|always accepting|siempre aceptando|unlimited languages|idiomas ilimitados|for less than what you pay|por menos de lo que pagas)/i,
            ),
          }),
        );
      }
    }

    // File-level sweep: hardcoded component copy must not smuggle banned
    // claims past the i18n-key checks above.
    const sweptFiles = [
      "src/app/(shop)/business/register/page.tsx",
      "src/components/business/BusinessOverview.tsx",
    ];
    for (const relativePath of sweptFiles) {
      const source = fs.readFileSync(path.join(process.cwd(), relativePath), "utf8");
      expect({ relativePath, source }).toEqual(
        expect.objectContaining({
          source: expect.not.stringMatching(
            /increase revenue by 25%|set up in under 60 seconds|\b60s\b/i,
          ),
        }),
      );
    }
  });


  test("crypto settlement copy qualifies fees and network confirmation", () => {
    const claims = [
      "businessDashboard.dashboard.pluginManager.config.usdcPayment.features.instantSettlement",
      "businessDashboard.dashboard.pluginManager.config.usdcPayment.features.instantSettlementDesc",
      "businessDashboard.overview.quickActions.enableUsdc.description",
      "businessDashboard.overview.quickActions.enableCrossChain.description",
      "businessDashboard.dashboard.pluginManager.config.crossChainPayment.howItWorksDesc",
      "businessDashboard.dashboard.pluginManager.config.crossChainPayment.enablePaymentsDesc",
      "businessDashboard.dashboard.pluginManager.config.crossChainPayment.features.anyToken",
      "businessDashboard.dashboard.pluginManager.config.crossChainPayment.features.anyTokenDesc",
      "businessDashboard.dashboard.pluginManager.config.crossChainPayment.features.autoConversionDesc",
    ];

    for (const locale of locales) {
      for (const key of claims) {
        const copy = at(messages[locale], key);
        expect(copy).not.toMatch(
          /instant settlement|liquidaci[oó]n instant[aá]nea|settled immediately|se liquidan inmediatamente|accept instant|acept[aá] pagos instant[aá]neos|any (?:crypto )?token|cualquier token|any blockchain|cualquier blockchain|thousands more|miles m[aá]s|always receive USDC|siempre recib|fee-free USDC settlement on all volume|liquidaci[oó]n en USDC sin comisiones en todo el volumen/i,
        );
      }
    }

    const guestDir = path.join(process.cwd(), "src/i18n/guest-messages");
    for (const filename of fs.readdirSync(guestDir)) {
      if (!filename.endsWith(".json")) continue;
      const tree = JSON.parse(
        fs.readFileSync(path.join(guestDir, filename), "utf8"),
      ) as Record<string, any>;
      const primer = tree.paymentProcessor?.noWalletPrimer;
      const advertisedCrypto = [
        primer,
        tree.cryptoDescription,
        tree.businessPage?.info?.cryptoPaymentsDesc,
      ].filter((value): value is string => typeof value === "string");
      for (const copy of advertisedCrypto) expect({ filename, copy }).toEqual(
        expect.objectContaining({
          copy: expect.not.stringMatching(
            /zero fees|sin comisiones|instant(?: settlement| payments?)?|instantáne[oa]s?|al instante|liquidaci[oó]n instant[aá]nea/i,
          ),
        }),
      );
      if (typeof primer === "string") {
        expect(primer).toContain("Payverge");
        expect(primer).toContain("Base");
      }
      const landingCrypto = tree.businessPage?.info?.cryptoPaymentsDesc;
      if (typeof landingCrypto === "string") expect(landingCrypto).toContain("Base");
    }
  });
});
