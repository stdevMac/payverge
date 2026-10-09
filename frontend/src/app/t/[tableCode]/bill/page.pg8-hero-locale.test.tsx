/**
 * PG-8 / PG-20 — guest bill page hero must format money with the guest locale.
 *
 * Live defect: `/t/.../bill` with lang=es rendered hero `$67.27` (en-US) while
 * GuestBill line items rendered `67,27 US$`. The hero lives in this page, not
 * GuestBill.tsx. This test asserts the **rendered DOM string**, not that a prop
 * was passed.
 *
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { CurrencyPrice } from "@/components/common/CurrencyConverter";
import { formatCurrency } from "@/api/currency";
import { normalizeGuestLocale } from "@/utils/guestCurrencyFormatter";
import fs from "fs";
import path from "path";

jest.mock("@/api/currency", () => {
  const actual = jest.requireActual("@/api/currency");
  return {
    ...actual,
    // Same currency, no live rate needed — still exercises real formatCurrency.
    convertAmount: jest.fn(async (amount: number) => ({
      converted_amount: amount,
    })),
  };
});

describe("PG-8 guest bill hero money locale (rendered DOM)", () => {
  it("CurrencyPrice with guest es locale never renders en-US $67.27 form", async () => {
    const amount = 67.27;
    const moneyLocale = normalizeGuestLocale("es");
    const expectedEs = formatCurrency(amount, "USD", undefined, moneyLocale);
    const enUsForm = formatCurrency(amount, "USD", undefined, "en-US");

    // Sanity: the two locales really differ (otherwise the assertion is weak).
    expect(expectedEs).not.toBe(enUsForm);
    expect(enUsForm).toMatch(/\$67\.27|67\.27/);
    // es typically places the code after or uses comma decimals.
    expect(expectedEs).toMatch(/67[,.]27/);

    render(
      <p data-testid="bill-hero-total" className="font-mono text-display-md">
        <CurrencyPrice
          amount={amount}
          fromCurrency="USD"
          displayCurrency="USD"
          locale={moneyLocale}
        />
      </p>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("bill-hero-total").textContent).toBe(
        expectedEs,
      );
    });
    // Explicit anti-regression: the en-US bare-dollar hero must not appear.
    expect(screen.queryByText("$67.27")).toBeNull();
    expect(screen.getByTestId("bill-hero-total").textContent).not.toBe(
      enUsForm,
    );
  });

  it("bill page source threads moneyLocale into every CurrencyPrice (incl. hero)", () => {
    const src = fs.readFileSync(
      path.join(process.cwd(), "src/app/t/[tableCode]/bill/page.tsx"),
      "utf8",
    );
    expect(src).toMatch(/normalizeGuestLocale/);
    expect(src).toMatch(/const moneyLocale = normalizeGuestLocale/);
    const blocks = src.match(/<CurrencyPrice[\s\S]*?\/>/g) ?? [];
    expect(blocks.length).toBeGreaterThanOrEqual(8);
    for (const block of blocks) {
      expect(block).toMatch(/locale=\{moneyLocale\}/);
    }
    // Hero block is the amount-due section (remainingDue / total_amount).
    expect(src).toMatch(
      /amount=\{isPayable \? remainingDue : currentBill\.bill\.total_amount\}[\s\S]*?locale=\{moneyLocale\}/,
    );
  });

  it("PersistentGuestNav bill tab threads moneyLocale (bottom dock was $67.27)", () => {
    const src = fs.readFileSync(
      path.join(
        process.cwd(),
        "src/components/navigation/PersistentGuestNav.tsx",
      ),
      "utf8",
    );
    expect(src).toMatch(/const moneyLocale = normalizeGuestLocale/);
    const blocks = src.match(/<CurrencyPrice[\s\S]*?\/>/g) ?? [];
    expect(blocks.length).toBe(1);
    expect(blocks[0]).toMatch(/locale=\{moneyLocale\}/);
  });

  it("CurrencyConverter locale prop is required (type contract in source)", () => {
    const src = fs.readFileSync(
      path.join(
        process.cwd(),
        "src/components/common/CurrencyConverter.tsx",
      ),
      "utf8",
    );
    // Must be required, not optional — optional reintroduces silent en-US.
    expect(src).toMatch(/locale:\s*string;/);
    expect(src).not.toMatch(/locale\?:\s*string;/);
  });

  it("hero + line items under es share one money convention (no $67.27 next to 10,20 US$)", async () => {
    // Mirrors the live audit: hero total + multiple line amounts must not mix
    // en-US bare-dollar with locale currency formatting on one surface.
    const moneyLocale = normalizeGuestLocale("es");
    const amounts = [67.27, 10.2, 57.8, 5.13, 4.34];
    const expected = amounts.map((a) =>
      formatCurrency(a, "USD", undefined, moneyLocale),
    );
    const enUsHero = formatCurrency(67.27, "USD", undefined, "en-US");

    render(
      <div data-testid="bill-money-surface">
        <p data-testid="hero">{/* hero */}
          <CurrencyPrice
            amount={67.27}
            fromCurrency="USD"
            displayCurrency="USD"
            locale={moneyLocale}
          />
        </p>
        {amounts.slice(1).map((a, i) => (
          <p key={i} data-testid={`line-${i}`}>
            <CurrencyPrice
              amount={a}
              fromCurrency="USD"
              displayCurrency="USD"
              locale={moneyLocale}
            />
          </p>
        ))}
      </div>,
    );

    await waitFor(() => {
      expect(screen.getByTestId("hero").textContent).toBe(expected[0]);
    });
    for (let i = 1; i < amounts.length; i++) {
      expect(screen.getByTestId(`line-${i - 1}`).textContent).toBe(
        expected[i],
      );
    }

    const surface = screen.getByTestId("bill-money-surface").textContent ?? "";
    // Single convention: every amount uses the es formatter string; no en-US hero.
    expect(surface).not.toContain(enUsHero);
    expect(surface).not.toMatch(/\$67\.27/);
    for (const s of expected) {
      expect(surface).toContain(s);
    }
  });
});
