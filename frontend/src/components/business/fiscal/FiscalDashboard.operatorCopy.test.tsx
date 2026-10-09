/** @jest-environment jsdom */
/**
 * #939 — fiscal settings chrome operator copy.
 *
 * Two things shipped wrong in the operator catalogs:
 *   1. `fiscal.setup.placeholders.taxCondition` was the raw backend enum
 *      ("ej. responsable_inscripto") — a database value, not operator copy —
 *      and the tax ID example was a persona CUIT (20-…) on a field that asks
 *      for the *restaurant's* CUIT.
 *   2. `help.fiscalSettings` advertised "Argentina (ARCA) and UAE guidelines".
 *      Only AR/arca is buildable: internal/fiscal/providers/registry.go builds
 *      exactly `country == "AR" && provider == "arca"` and returns a permanent
 *      "unsupported country/provider combination" for everything else, and this
 *      component already ships AE as `available: false`.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";

import * as fiscalApi from "@/api/fiscal";
import enFiscal from "@/i18n/messages/en/fiscal.json";
import esFiscal from "@/i18n/messages/es/fiscal.json";
import esArFiscal from "@/i18n/messages/es-ar/fiscal.json";
import enDashboard from "@/i18n/messages/en/businessDashboard.json";
import esDashboard from "@/i18n/messages/es/businessDashboard.json";
import esArDashboard from "@/i18n/messages/es-ar/businessDashboard.json";
import FiscalDashboard from "./FiscalDashboard";

const mockLocale = { current: "es-AR" };

jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const actual = jest.requireActual("@/i18n/SimpleTranslationProvider");
  return {
    ...actual,
    useSimpleLocale: () => ({
      locale: mockLocale.current,
      setLocale: jest.fn(),
    }),
  };
});

jest.mock("@/api/fiscal");

// AR persona CUIT prefixes. A restaurant registers as a sociedad (30-), so a
// 20-/23-/24-/27- example on the business tax-ID field points operators at the
// owner's personal CUIT.
const PERSONA_CUIT_PREFIXES = ["20", "23", "24", "27"];

// A raw backend enum leaking into operator copy: two or more lowercase ASCII
// words joined by underscores (responsable_inscripto, monotributo_social, …).
const RAW_ENUM = /\b[a-z]+(?:_[a-z]+)+\b/;

function arSettings(overrides: Partial<fiscalApi.FiscalSettings> = {}) {
  return {
    id: 1,
    business_id: 142,
    country: "AR",
    provider: "arca",
    mode: "manual" as const,
    environment: "sandbox" as const,
    tax_id: "30123456789",
    tax_condition: "responsable_inscripto",
    point_of_sale: 1,
    setup_status: "validated",
    ...overrides,
  };
}

describe("FiscalDashboard operator copy (#939)", () => {
  beforeEach(() => {
    mockLocale.current = "es-AR";
    jest.clearAllMocks();
    (fiscalApi.listReceiptDelivery as jest.Mock).mockResolvedValue([]);
    (fiscalApi.listReceipts as jest.Mock).mockResolvedValue([]);
    (fiscalApi.getSettings as jest.Mock).mockResolvedValue(arSettings());
  });

  it("asks for the restaurant's own CUIT, not the owner's persona CUIT", async () => {
    render(<FiscalDashboard businessId={142} />);

    await userEvent.click(
      await screen.findByRole("button", { name: /editar configuración/i }),
    );

    const taxIdField = screen
      .getByText("ID fiscal")
      .closest("label")
      ?.querySelector("input");
    expect(taxIdField).toBeTruthy();

    const placeholder = taxIdField?.getAttribute("placeholder") ?? "";
    const cuitPrefix = placeholder.match(/\b(\d{2})-\d{8}-\d\b/)?.[1] ?? "";
    expect(cuitPrefix).not.toBe("");
    expect(PERSONA_CUIT_PREFIXES).not.toContain(cuitPrefix);
  });

  it("never renders a raw backend enum in the setup form", async () => {
    render(<FiscalDashboard businessId={142} />);

    await userEvent.click(
      await screen.findByRole("button", { name: /editar configuración/i }),
    );

    expect(screen.queryByText(/responsable_inscripto/)).not.toBeInTheDocument();
  });
});

describe("fiscal operator catalog copy (#939)", () => {
  const catalogs: Array<[string, Record<string, unknown>]> = [
    ["en", enFiscal as Record<string, unknown>],
    ["es", esFiscal as Record<string, unknown>],
    ["es-ar", esArFiscal as Record<string, unknown>],
  ];

  it.each(catalogs)(
    "%s ships no raw backend enum in fiscal.setup.placeholders",
    (_locale, catalog) => {
      const setup = (catalog.setup ?? {}) as Record<string, unknown>;
      const placeholders = (setup.placeholders ?? {}) as Record<string, string>;

      Object.entries(placeholders).forEach(([key, value]) => {
        expect(`${key}=${value}`).not.toMatch(RAW_ENUM);
      });
    },
  );

  it.each([
    ["en", enDashboard as Record<string, unknown>],
    ["es", esDashboard as Record<string, unknown>],
    ["es-ar", esArDashboard as Record<string, unknown>],
  ] as Array<[string, Record<string, unknown>]>)(
    "%s does not advertise a fiscal country the backend cannot build",
    (_locale, catalog) => {
      const help = (catalog.help ?? {}) as Record<string, string>;
      const copy = help.fiscalSettings;
      if (copy === undefined) {
        // es-AR is a thin override layer; an absent key inherits es, which is
        // asserted on its own row.
        return;
      }

      expect(copy).toMatch(/Argentina/);
      // AR/arca is the only combination CredentialAwareFactory can build.
      expect(copy).not.toMatch(/\bUAE\b|\bEAU\b|Emirat/i);
    },
  );

  it("keeps the es-AR fiscal help override in voseo when it exists", () => {
    const help = (esArDashboard as Record<string, unknown>).help as
      | Record<string, string>
      | undefined;
    const copy = help?.fiscalSettings;
    if (copy === undefined) return;

    // es-AR is the voseo layer: the tuteo imperative must not survive into it.
    // (\b is ASCII-only in JS, so anchor on the accent itself.)
    expect(copy).not.toMatch(/\bConfigura\s/);
    expect(copy).toMatch(/\bConfigurá\s/);
  });
});
