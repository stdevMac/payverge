/**
 * Live Bills card: bills with the table_id = 0 sentinel (delivery/counter)
 * come back from the analytics endpoint with an empty table_name — the card
 * rendered a blank name next to the icon (a dangling "$ "). The label helper
 * must fall back to Counter/Delivery via counter_id.
 */
import { liveBillTableLabel } from "./LiveBills";

const tString = (key: string) =>
  ({ counter: "Counter", delivery: "Delivery" })[key] ?? key;

describe("liveBillTableLabel", () => {
  it("prefers the real table name when present", () => {
    expect(
      liveBillTableLabel({ table_name: "Patio 2", counter_id: null }, tString),
    ).toBe("Patio 2");
  });

  it("labels tableless counter bills 'Counter'", () => {
    expect(liveBillTableLabel({ table_name: "", counter_id: 4 }, tString)).toBe(
      "Counter",
    );
  });

  it("labels tableless bills without a counter 'Delivery'", () => {
    expect(
      liveBillTableLabel({ table_name: "", counter_id: null }, tString),
    ).toBe("Delivery");
  });

  it("treats a missing counter_id (older API) as delivery", () => {
    expect(liveBillTableLabel({ table_name: "" }, tString)).toBe("Delivery");
  });
});
