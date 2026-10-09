import fs from "fs";
import path from "path";
import { formatMoneyAmount, formatMoneyFromCents } from "./moneyFormat";

describe("formatMoneyAmount / formatMoneyFromCents (shared convention)", () => {
  it("guest surface formats with the diner locale (es → comma decimals)", () => {
    const es = formatMoneyAmount(67.27, "USD", {
      surface: "guest",
      locale: "es",
    });
    // es locales typically use "67,27" and a USD code/symbol placement.
    expect(es).toMatch(/67[,.]27/);
    expect(es.toUpperCase()).toMatch(/US|\$|USD/);
  });

  it("guest and operator surfaces do not mix bare toFixed with currency Intl", () => {
    const guest = formatMoneyAmount(68, "USD", {
      surface: "guest",
      locale: "en",
    });
    const operator = formatMoneyAmount(68, "USD", {
      surface: "operator",
      locale: "en",
    });
    // Both must include currency presentation — never bare "68.00".
    expect(guest).not.toBe("68.00");
    expect(operator).not.toBe("68.00");
    expect(guest).toMatch(/68/);
    expect(operator).toMatch(/68/);
  });

  it("formatMoneyFromCents divides cents once and formats as dollars", () => {
    const s = formatMoneyFromCents(6727, "USD", {
      surface: "operator",
      locale: "en-US",
    });
    expect(s).toMatch(/67\.27/);
  });

  it("operator es uses locale-aware grouping for larger amounts", () => {
    const s = formatMoneyFromCents(199000, "USD", {
      surface: "operator",
      locale: "es",
    });
    // 1990.00 USD — es often uses thin space or period thousands; must not be bare toFixed.
    expect(s).not.toBe("1990.00");
    expect(s).toMatch(/1[.\s\u00a0]?990/);
  });

  it("Root D / PG-15.2: FormatMoneyOptions requires locale on guest surface (type contract)", () => {
    // Compile-time: guest without locale is not assignable. Runtime smoke:
    // guest with locale uses diner formatting (not a bare "18.50 USD" third form).
    const guest = formatMoneyAmount(18.5, "USD", {
      surface: "guest",
      locale: "es",
    });
    expect(guest).not.toBe("18.50 USD");
    expect(guest).toMatch(/18[,.]50|18[,.]5/);
    // Source-level: the options type is a discriminated union.
    const src = fs.readFileSync(
      path.join(process.cwd(), "src/lib/moneyFormat.ts"),
      "utf8",
    );
    expect(src).toMatch(/surface:\s*"guest"/);
    expect(src).toMatch(/locale:\s*string/);
    expect(src).toMatch(/FormatMoneyOptions\s*=/);
  });
});

describe("PG-20 / PG-8 owned call sites use shared money paths", () => {
  it("PaymentSection passes guest locale into every CurrencyPrice (no en-US mix)", () => {
    const src = fs.readFileSync(
      path.join(process.cwd(), "src/components/guest/PaymentSection.tsx"),
      "utf8",
    );
    expect(src).toMatch(/normalizeGuestLocale/);
    expect(src).toMatch(/moneyLocale/);
    const blocks = src.match(/<CurrencyPrice[\s\S]*?\/>/g) ?? [];
    expect(blocks.length).toBeGreaterThan(0);
    for (const block of blocks) {
      expect(block).toMatch(/locale=\{moneyLocale\}/);
    }
  });

  it("guest money surfaces all pass locale into CurrencyPrice (PG-8 sweep)", () => {
    const files = [
      "src/app/t/[tableCode]/bill/page.tsx",
      "src/app/t/[tableCode]/menu/_components/CartModal.tsx",
      "src/app/t/[tableCode]/menu/page.tsx",
      "src/components/guest/GuestMenuViews.tsx",
      "src/components/guest/GuestTableView.tsx",
      "src/components/guest/MobileMenuItem.tsx",
      "src/components/navigation/PersistentGuestNav.tsx",
      "src/components/business-page/PublicMenuDisplay.tsx",
      "src/components/guest/GuestBill.tsx",
      "src/components/guest/PaymentSection.tsx",
      "src/components/splitting/GuestBillSplitPanel.tsx",
    ];
    for (const rel of files) {
      const src = fs.readFileSync(path.join(process.cwd(), rel), "utf8");
      const blocks = src.match(/<Currency(?:Price|Converter)[\s\S]*?\/>/g) ?? [];
      expect(blocks.length).toBeGreaterThan(0);
      for (const block of blocks) {
        expect(block).toMatch(/locale=\{/);
      }
    }
  });
});
