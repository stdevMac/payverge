/** @jest-environment jsdom */
/**
 * #377 — this operator money route lives outside the dashboard tab tree, but
 * every outbound destination must stay on operator paths. `/business/:id` is
 * the public custom-URL protocol shim (308 → /b/:slug or 404), not a
 * dashboard.
 */
import React from "react";
import { render, screen } from "@testing-library/react";

jest.mock("next/navigation", () => ({
  useParams: () => ({
    businessId: "demo-admin-8-ai-pro",
    billId: "387",
  }),
}));

jest.mock("@/components/business/AlternativePaymentManager", () => ({
  __esModule: true,
  default: () => <div data-testid="alt-pay-manager" />,
}));

jest.mock("@/api/business", () => ({
  getBusiness: jest.fn().mockResolvedValue({ default_currency: "USD" }),
}));

jest.mock("@/utils/resolveBusinessId", () => ({
  resolveNumericBusinessId: jest.fn().mockResolvedValue(8),
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: () => {} }),
  getTranslation: (key: string) => {
    if (key.endsWith("backToBills")) return "Back to Bills";
    if (key.endsWith("breadcrumbs.business")) return "Business";
    if (key.endsWith("breadcrumbs.bills")) return "Bills";
    if (key.endsWith("breadcrumbs.alternativePayments")) {
      return "Alternative Payments";
    }
    if (key.endsWith("paymentManagement")) return "Payment Management";
    if (key.endsWith("page.title") || key.endsWith(".title")) {
      return "Alternative Payment Management";
    }
    return key;
  },
}));

import AlternativePaymentsPage from "../page";
import {
  getOperatorDashboardPath,
  isPublicBusinessProtocolPath,
} from "@/utils/businessUrl";

describe("alternative-payments operator destinations (#377)", () => {
  const slug = "demo-admin-8-ai-pro";
  const publicProtocol = `/business/${slug}`;
  const dashboard = getOperatorDashboardPath(slug);
  const bills = getOperatorDashboardPath(slug, "bills");

  it("distinguishes the public custom-URL protocol from operator destinations", () => {
    expect(isPublicBusinessProtocolPath(publicProtocol)).toBe(true);
    expect(isPublicBusinessProtocolPath(dashboard)).toBe(false);
    expect(isPublicBusinessProtocolPath(bills)).toBe(false);
    expect(
      isPublicBusinessProtocolPath(
        `/business/${slug}/bills/387/alternative-payments`,
      ),
    ).toBe(false);
  });

  function hrefsNamed(label: string): string[] {
    return screen.getAllByRole("link", { name: label }).flatMap((node) => {
      const href =
        node.getAttribute("href") ||
        node.querySelector("a")?.getAttribute("href");
      return href ? [href] : [];
    });
  }

  it("points Business at the operator dashboard, not the public custom-URL shim", () => {
    render(<AlternativePaymentsPage />);
    const hrefs = hrefsNamed("Business");
    expect(hrefs.length).toBeGreaterThan(0);
    expect(hrefs).toEqual(hrefs.map(() => dashboard));
    expect(hrefs).not.toContain(publicProtocol);
    for (const href of hrefs) {
      expect(isPublicBusinessProtocolPath(href)).toBe(false);
    }
  });

  it("keeps Back to Bills and Bills on the operator bills tab", () => {
    render(<AlternativePaymentsPage />);
    expect(hrefsNamed("Bills")).toEqual(hrefsNamed("Bills").map(() => bills));
    expect(hrefsNamed("Back to Bills")).toEqual(
      hrefsNamed("Back to Bills").map(() => bills),
    );
  });
});
