import { sanitizeName, buildFilename } from "../filename.js";

describe("sanitizeName", () => {
  test("lowercases and keeps safe characters", () => {
    expect(sanitizeName("Analytics-Tab_01")).toBe("analytics-tab_01");
  });

  test("replaces unsafe characters with dashes and collapses repeats", () => {
    expect(sanitizeName("Bills / Detail!!")).toBe("bills-detail");
  });

  test("trims leading and trailing dashes", () => {
    expect(sanitizeName("  hero  ")).toBe("hero");
  });
});

describe("buildFilename", () => {
  test("nests a sanitized name under a dated folder", () => {
    expect(buildFilename("Analytics Tab", "2026-07-03"))
      .toBe("payverge-shots/2026-07-03/analytics-tab.png");
  });
});
