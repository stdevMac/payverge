import { rowsToCsv } from "./csvExport";

describe("rowsToCsv", () => {
  it("renders a header and data rows", () => {
    const csv = rowsToCsv(
      [
        { key: "name", header: "Name" },
        { key: "amount", header: "Amount" },
      ],
      [{ name: "Coffee", amount: 2.5 }],
    );
    const lines = csv.split("\n");
    expect(lines[0]).toBe("Name,Amount");
    expect(lines[1]).toBe("Coffee,2.5");
  });

  it("quotes values containing commas or quotes", () => {
    const csv = rowsToCsv(
      [{ key: "desc", header: "Description" }],
      [{ desc: 'Say "hi", friend' }],
    );
    expect(csv.split("\n")[1]).toBe('"Say ""hi"", friend"');
  });

  it("neutralizes formula-injection characters at the start of a cell", () => {
    const injectionValues = [
      "=SUM(A1:B1)",
      "+cmd|' /C calc'!A0",
      "-2+3+cmd",
      "@SUM(1)",
      "\t=hidden",
      "\r=hidden",
    ];
    for (const val of injectionValues) {
      const csv = rowsToCsv(
        [{ key: "val", header: "Value" }],
        [{ val }],
      );
      const dataCell = csv.split("\n")[1];
      // Strip surrounding quotes if present
      const raw = dataCell.replace(/^"|"$/g, "");
      expect(raw.charAt(0)).not.toMatch(/[=+\-@\t\r]/);
    }
  });
});
