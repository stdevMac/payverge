/**
 * S-4 operator: LoyaltyTab must format the unit money amount through
 * formatMoneyAmount (surface: operator + locale). Live defect under es:
 * "USD 1.00" / "USD 1.00" next to correct "18,50 US$" elsewhere on the dashboard.
 *
 * Assert rendered DOM — not helper presence. Revert the formatMoneyAmount
 * wiring (restore bare `{currency} 1.00`) and this suite must go red.
 *
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import LoyaltyTab from "../LoyaltyTab";
import { formatMoneyAmount } from "@/lib/moneyFormat";

jest.mock("@/api/loyalty", () => ({
  getLoyalty: () =>
    Promise.resolve({
      program: {
        id: 1,
        business_id: 1,
        enabled: true,
        points_per_dollar: 1.25,
        redemption_points_per_dollar: 100,
      },
      tiers: [
        { id: 1, name: "Bronze", min_lifetime_spent: 0, sort_order: 0 },
        { id: 2, name: "Silver", min_lifetime_spent: 250, sort_order: 1 },
      ],
    }),
  putLoyalty: jest.fn(() => Promise.resolve({ ok: true })),
  previewLoyalty: () =>
    Promise.resolve({
      total_customers: 10,
      tier_distribution: { Bronze: 10 },
    }),
}));

jest.mock("@/api/business", () => ({
  getBusiness: jest.fn(() =>
    Promise.resolve({ id: 1, default_currency: "USD" }),
  ),
}));

jest.mock("@/contexts/ToastContext", () => ({
  useToast: () => ({
    showSuccess: jest.fn(),
    showError: jest.fn(),
    showInfo: jest.fn(),
    showWarning: jest.fn(),
  }),
}));

const mockLocale = { current: "es" as string };

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

describe("S-4 LoyaltyTab operator money labels", () => {
  beforeEach(() => {
    mockLocale.current = "es";
  });

  it("renders earn/redeem with locale money, never bare USD 1.00", async () => {
    render(<LoyaltyTab businessId={1} />);

    const expectedUnit = formatMoneyAmount(1, "USD", {
      surface: "operator",
      locale: "es",
    });
    // es → "1,00 US$" (narrow no-break space may appear)
    expect(expectedUnit).toMatch(/1[,.]00/);
    expect(expectedUnit.toUpperCase()).toMatch(/US|\$|USD/);

    const earn = await screen.findByTestId("loyalty-earn-explanation");
    const redeem = await screen.findByTestId("loyalty-redeem-explanation");

    // Must contain the full formatted unit amount (locale money).
    expect(earn.textContent).toContain(expectedUnit);
    expect(redeem.textContent).toContain(expectedUnit);

    // Rate 1.25 under es → "1,25" (not bare "1.25" only if locale formats comma).
    const rateEs = new Intl.NumberFormat("es", {
      maximumFractionDigits: 2,
    }).format(1.25);
    expect(earn.textContent).toContain(rateEs);

    // Forbidden legacy form from the live audit.
    expect(earn.textContent).not.toMatch(/USD\s*1\.00/);
    expect(redeem.textContent).not.toMatch(/USD\s*1\.00/);
    expect(earn.textContent).not.toContain("USD 1.00");
  });

  it("en locale still includes currency presentation (not bare 1.00)", async () => {
    mockLocale.current = "en";
    render(<LoyaltyTab businessId={1} />);

    const expectedUnit = formatMoneyAmount(1, "USD", {
      surface: "operator",
      locale: "en",
    });
    const earn = await screen.findByTestId("loyalty-earn-explanation");
    expect(earn.textContent).toContain(expectedUnit);
    // en-US typically "$1.00" — must not be the audit's "USD 1.00" bare pair.
    expect(earn.textContent).not.toMatch(/USD\s+1\.00/);
  });

  it("points-rate field label embeds locale money amount", async () => {
    mockLocale.current = "es";
    render(<LoyaltyTab businessId={1} />);

    const expectedUnit = formatMoneyAmount(1, "USD", {
      surface: "operator",
      locale: "es",
    });
    await waitFor(() => {
      const wrap = screen.getByTestId("loyalty-rate-field-wrap");
      expect(wrap.textContent).toContain(expectedUnit);
    });
  });
});
