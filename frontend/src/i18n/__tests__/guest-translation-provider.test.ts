import { translateKey } from "../GuestTranslationProvider";

describe("GuestTranslationProvider.translateKey", () => {
  describe("ICU-style {var} interpolation", () => {
    it("replaces a simple identifier with a string param", () => {
      const messages = { greeting: "Hello {name}" };
      expect(translateKey(messages, "greeting", { name: "world" })).toBe(
        "Hello world",
      );
    });

    it("replaces multiple identifiers in one string", () => {
      const messages = { tableRow: "Table {tableNumber} – {status}" };
      expect(
        translateKey(messages, "tableRow", {
          tableNumber: 7,
          status: "occupied",
        }),
      ).toBe("Table 7 – occupied");
    });

    it("resolves dotted key paths", () => {
      const messages = { menu: { priceNote: "Price shown in {currency}" } };
      expect(
        translateKey(messages, "menu.priceNote", { currency: "USD" }),
      ).toBe("Price shown in USD");
    });

    it("preserves the original token when a param is missing", () => {
      const messages = { code: "Table {tableCode} isn't active" };
      // No params at all
      expect(translateKey(messages, "code")).toBe(
        "Table {tableCode} isn't active",
      );
      // Params without the key
      expect(translateKey(messages, "code", {})).toBe(
        "Table {tableCode} isn't active",
      );
    });

    it("coerces numeric params to strings", () => {
      const messages = { total: "Tax {rate}" };
      expect(translateKey(messages, "total", { rate: 0.08 })).toBe("Tax 0.08");
    });

    it("handles zero and empty string params without falling back", () => {
      const messages = { msg: "Count is {n}" };
      expect(translateKey(messages, "msg", { n: 0 })).toBe("Count is 0");
      expect(translateKey(messages, "msg", { n: "" })).toBe("Count is ");
    });

    it("returns the key when the translation is missing", () => {
      const messages = { known: "hi" };
      expect(translateKey(messages, "not.a.real.key")).toBe("not.a.real.key");
    });
  });

  describe("ICU plural/select passthrough", () => {
    it("does not mangle plural blocks (no plural support yet)", () => {
      // When the provider lacks full ICU support, a plural block should pass
      // through unchanged. The identifier-only regex must skip `{count, plural,...}`
      // because the token after `{` contains a comma (non-word char).
      const messages = {
        items: "Total {count, plural, one {# item} other {# items}}",
      };
      // Note: nested tokens like {# item} use a non-word char (#) so they also
      // skip the regex. The whole plural expression survives intact.
      expect(translateKey(messages, "items", { count: 3 })).toBe(
        "Total {count, plural, one {# item} other {# items}}",
      );
    });

    it("replaces {count} and collapses an unsupported {{...plural...}} block to its `other` form (GUEST-4 guard)", () => {
      // The guest runtime has no ICU plural support. Per the GUEST-4 brace
      // guard, a `{{count, plural, one {item} other {items}}}` block is
      // collapsed to its brace-free `other` form BEFORE interpolation, so a
      // raw `{{...}}` literal can never leak to a guest. {count} is then
      // interpolated normally.
      const messages = {
        cartCount: "{count} {{count, plural, one {item} other {items}}} in cart",
      };
      expect(translateKey(messages, "cartCount", { count: 2 })).toBe(
        "2 items in cart",
      );
    });
  });
});
