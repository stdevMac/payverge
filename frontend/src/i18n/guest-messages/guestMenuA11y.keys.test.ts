import fs from "fs";
import path from "path";

const DIR = __dirname;
const REQUIRED: Record<string, string[]> = {
  "accessibility.addItemToCart": ["{name}"],
  "accessibility.addItemToOrder": ["{name}"],
  "accessibility.addNamedBundle": ["{name}"],
  "accessibility.decreaseItemQuantity": ["{name}"],
  "accessibility.increaseItemQuantity": ["{name}"],
  "accessibility.removeNamedItem": ["{name}"],
  "menu.itemAddedNamed": ["{quantity}", "{name}"],
  "menu.cartQuantityIs": ["{name}", "{quantity}"],
  "menu.cartItemRemoved": ["{name}"],
  "menu.cartCleared": [],
  "menu.cartEmptyAnnouncement": [],
  "menu.updatingTotal": [],
  "menu.totalUpdated": ["{amount}"],
  "menu.clearCartConfirmAria": [],
  "menu.bundles.added": ["{name}"],
  "menu.bundles.addedShort": [],
  "navigation.billWithSummary": ["{count}", "{amount}"],
  "navigation.billWithSummaryOne": ["{amount}"],
  "bill.paidAnnouncement": [],
  "orders.readyAnnouncement": [],
  "deliveryTracking.courierPickedUpAnnouncement": [],
};

function flatten(obj: unknown, prefix = "", out: Record<string, unknown> = {}) {
  if (!obj || typeof obj !== "object" || Array.isArray(obj)) return out;
  for (const [key, value] of Object.entries(obj as Record<string, unknown>)) {
    const next = prefix ? `${prefix}.${key}` : key;
    if (value && typeof value === "object" && !Array.isArray(value)) {
      flatten(value, next, out);
    } else {
      out[next] = value;
    }
  }
  return out;
}

const locales = fs
  .readdirSync(DIR)
  .filter((file) => file.endsWith(".json") && !file.startsWith("."));

describe("guest menu a11y catalog (#421-428)", () => {
  it.each(locales)("%s includes named-action keys and placeholders", (file) => {
    const flat = flatten(
      JSON.parse(fs.readFileSync(path.join(DIR, file), "utf8")),
    );
    for (const [key, tokens] of Object.entries(REQUIRED)) {
      expect(typeof flat[key]).toBe("string");
      for (const token of tokens) {
        expect(String(flat[key])).toContain(token);
      }
    }
  });
});
