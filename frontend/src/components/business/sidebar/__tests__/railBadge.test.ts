import { capBadge, sumBadges } from "../railBadge";

describe("capBadge", () => {
  it("returns the number as a string up to 9", () => {
    expect(capBadge(0)).toBe("0");
    expect(capBadge(1)).toBe("1");
    expect(capBadge(9)).toBe("9");
  });
  it("caps anything above 9 at '9+'", () => {
    expect(capBadge(10)).toBe("9+");
    expect(capBadge(250)).toBe("9+");
  });
});

describe("sumBadges", () => {
  it("sums numeric badges, treating null as 0", () => {
    expect(sumBadges([{ badge: 2 }, { badge: null }, { badge: 3 }])).toBe(5);
  });
  it("returns 0 for an empty list", () => {
    expect(sumBadges([])).toBe(0);
  });
  it("returns 0 when every badge is null", () => {
    expect(sumBadges([{ badge: null }, { badge: null }])).toBe(0);
  });
});
