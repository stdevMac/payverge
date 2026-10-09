import { displayBillHistoryActor } from "./billHistoryActor";

describe("displayBillHistoryActor", () => {
  it.each([
    "demo+admin1-business2-staff3@demo.example.test",
    "DEMO+admin12-business40-staff1@example.org",
    "demo+admin7-cafe@example.org",
    "demo",
    "demo-seed",
    " Demo-Seed ",
    "",
    "   ",
    null,
    undefined,
  ])("hides seeded or empty actor %p", (actor) => {
    expect(displayBillHistoryActor(actor)).toBeNull();
  });

  it.each([
    ["maria@restaurant.test", "maria@restaurant.test"],
    ["demo@restaurant.test", "demo@restaurant.test"],
    [
      "demo+reservation-1-20260101@example.org",
      "demo+reservation-1-20260101@example.org",
    ],
    ["system", "system"],
    [" Ana ", "Ana"],
  ])("keeps real actor %p", (actor, expected) => {
    expect(displayBillHistoryActor(actor)).toBe(expected);
  });
});
