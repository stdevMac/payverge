import fs from "fs";
import path from "path";

const SRC = path.resolve(__dirname, "GuestBill.tsx");

describe("GuestBill PG-8 / PG-20 CurrencyConverter locale", () => {
  const src = fs.readFileSync(SRC, "utf8");

  it("passes moneyLocale on the total CurrencyConverter block", () => {
    // Extract the total_amount CurrencyConverter props block.
    const m = src.match(
      /amount=\{bill\.bill\.total_amount\}[\s\S]*?showUSDCConversion=\{[^}]*\}[\s\S]*?\/>/,
    );
    expect(m).not.toBeNull();
    expect(m![0]).toContain("locale={moneyLocale}");
  });

  it("formats tax and service-fee rates via formatGuestRate, not raw JS numbers", () => {
    expect(src).toMatch(/formatGuestRate/);
    expect(src).not.toMatch(
      /t\("bill\.tax",\s*\{\s*rate:\s*business\.tax_rate/,
    );
    expect(src).not.toMatch(
      /t\("bill\.serviceFee",\s*\{\s*rate:\s*business\.service_fee_rate/,
    );
  });
});
