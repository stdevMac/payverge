/**
 * NR-1: loyalty points rate 0–100 bound.
 * Live defect: typing 1,5,0 left "150" after blur with Save enabled.
 * Clamp on blur must rewrite the field; out-of-range must show error and
 * keep Save disabled until the value is in range.
 *
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import LoyaltyTab from "../LoyaltyTab";

type PutLoyaltyArgs = Parameters<typeof import("@/api/loyalty").putLoyalty>;

const mockPutLoyalty = jest.fn((..._args: PutLoyaltyArgs) =>
  Promise.resolve({ ok: true }),
);

jest.mock("@/api/loyalty", () => ({
  getLoyalty: () =>
    Promise.resolve({
      program: {
        id: 1,
        business_id: 1,
        enabled: true,
        points_per_dollar: 1,
        redemption_points_per_dollar: 100,
      },
      tiers: [{ id: 1, name: "Bronze", min_lifetime_spent: 0, sort_order: 0 }],
    }),
  putLoyalty: (...args: PutLoyaltyArgs) => mockPutLoyalty(...args),
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

function typeChars(el: HTMLElement, chars: string) {
  let acc = (el as HTMLInputElement).value || "";
  for (const ch of chars) {
    acc = acc + ch;
    fireEvent.change(el, { target: { value: acc } });
  }
}

beforeEach(() => {
  mockPutLoyalty.mockClear();
});

describe("NR-1 LoyaltyTab points rate 0–100", () => {
  it("keeps 150 while typing, clamps to 100 on blur", async () => {
    render(<LoyaltyTab businessId={1} />);
    const input = await screen.findByTestId("loyalty-points-rate");
    fireEvent.change(input, { target: { value: "" } });
    // Per-keystroke: 1 → 15 → 150 (must not clamp mid-stream).
    typeChars(input, "1");
    expect((input as HTMLInputElement).value).toBe("1");
    typeChars(input, "5");
    expect((input as HTMLInputElement).value).toBe("15");
    typeChars(input, "0");
    expect((input as HTMLInputElement).value).toBe("150");
    fireEvent.blur(input);
    await waitFor(() => {
      expect((input as HTMLInputElement).value).toBe("100");
    });
  });

  it("marks out-of-range invalid and disables Save while field shows 150", async () => {
    render(<LoyaltyTab businessId={1} />);
    const input = await screen.findByTestId("loyalty-points-rate");
    fireEvent.change(input, { target: { value: "" } });
    // Per-keystroke (rule 4): 1 → 15 → 150, never clamp mid-stream.
    typeChars(input, "1");
    expect((input as HTMLInputElement).value).toBe("1");
    typeChars(input, "5");
    expect((input as HTMLInputElement).value).toBe("15");
    typeChars(input, "0");
    // Before blur the raw string is still 150 — must not look valid.
    expect((input as HTMLInputElement).value).toBe("150");
    await waitFor(() => {
      expect(input).toHaveAttribute("aria-invalid", "true");
    });
    // Rendered error slot (not only aria-invalid).
    expect(
      screen.getByText(/earn rate must be between 0 and 100/i),
    ).toBeInTheDocument();
    const save = screen.getByRole("button", { name: /save changes/i });
    expect(save).toBeDisabled();
    expect(mockPutLoyalty).not.toHaveBeenCalled();
  });

  it("rejects garbage with aria-invalid and does not PUT", async () => {
    render(<LoyaltyTab businessId={1} />);
    const input = await screen.findByTestId("loyalty-points-rate");
    fireEvent.change(input, { target: { value: "" } });
    typeChars(input, "abc");
    fireEvent.blur(input);
    await waitFor(() => {
      expect(input).toHaveAttribute("aria-invalid", "true");
    });
    expect((input as HTMLInputElement).value).toBe("abc");
    const save = screen.getByRole("button", { name: /save changes/i });
    expect(save).toBeDisabled();
  });
});
