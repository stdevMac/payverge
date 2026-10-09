/** @jest-environment jsdom */
/**
 * L5-13: real LoyaltyTab rate field must accept 2.5 / 2,5 without separator
 * erasure or mid-keystroke clamp-to-100. Rule 3: deleting LoyaltyTab or
 * reverting rate to number state must fail this suite.
 */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import LoyaltyTab from "../LoyaltyTab";

const okLoyalty = () =>
  Promise.resolve({
    program: {
      id: 1,
      business_id: 1,
      enabled: true,
      points_per_dollar: 1,
      redemption_points_per_dollar: 100,
    },
    tiers: [{ id: 1, name: "Bronze", min_lifetime_spent: 0, sort_order: 0 }],
  });

const mockPutLoyalty = jest.fn((..._args: unknown[]) =>
  Promise.resolve({ ok: true }),
);
const mockPreviewLoyalty = jest.fn((..._args: unknown[]) =>
  Promise.resolve({
    total_customers: 10,
    tier_distribution: { Bronze: 10 },
  }),
);

jest.mock("@/api/loyalty", () => ({
  getLoyalty: () => okLoyalty(),
  putLoyalty: (...args: unknown[]) => mockPutLoyalty(...args),
  previewLoyalty: (...args: unknown[]) => mockPreviewLoyalty(...args),
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

function typeChars(el: HTMLElement, chars: string) {
  let acc = (el as HTMLInputElement).value || "";
  for (const ch of chars) {
    acc = acc + ch;
    fireEvent.change(el, { target: { value: acc } });
  }
}

beforeEach(() => {
  mockPutLoyalty.mockClear();
  mockPreviewLoyalty.mockClear();
});

describe("L5-13 LoyaltyTab rate decimals (real component)", () => {
  it("keeps 2.5 while typing (separator not erased)", async () => {
    render(<LoyaltyTab businessId={1} />);
    const input = await screen.findByTestId("loyalty-points-rate");
    fireEvent.change(input, { target: { value: "" } });
    typeChars(input, "2.5");
    expect((input as HTMLInputElement).value).toBe("2.5");
  });

  it("keeps 2,5 while typing", async () => {
    render(<LoyaltyTab businessId={1} />);
    const input = await screen.findByTestId("loyalty-points-rate");
    fireEvent.change(input, { target: { value: "" } });
    typeChars(input, "2,5");
    expect((input as HTMLInputElement).value).toBe("2,5");
  });

  it("does not clamp 1.25 mid-keystroke to 100", async () => {
    render(<LoyaltyTab businessId={1} />);
    const input = await screen.findByTestId("loyalty-points-rate");
    fireEvent.change(input, { target: { value: "" } });
    typeChars(input, "1.25");
    expect((input as HTMLInputElement).value).toBe("1.25");
    fireEvent.blur(input);
    // Save must persist 1.25, not 100 or 125.
    fireEvent.click(screen.getByRole("button", { name: /save changes/i }));
    await waitFor(() =>
      expect(mockPutLoyalty).toHaveBeenCalledWith(
        1,
        expect.objectContaining({ points_per_dollar: 1.25 }),
      ),
    );
  });
});
