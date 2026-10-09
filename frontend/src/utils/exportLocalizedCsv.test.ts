import { exportLocalizedCsv } from "./exportLocalizedCsv";
import * as csvExport from "./csvExport";

jest.mock("./csvExport", () => {
  const actual = jest.requireActual("./csvExport");
  return {
    ...actual,
    downloadCsv: jest.fn(),
  };
});

describe("exportLocalizedCsv (S-14)", () => {
  beforeEach(() => {
    jest.clearAllMocks();
  });

  it("resolves headers via t() and includes all columns", () => {
    const t = (key: string) =>
      ({ "csv.date": "Fecha", "csv.notes": "Notas", "csv.amount": "Monto" }[key] ??
      key);
    const csv = exportLocalizedCsv({
      columns: [
        { key: "date", headerKey: "csv.date" },
        { key: "notes", headerKey: "csv.notes" },
        { key: "amount", headerKey: "csv.amount" },
      ],
      rows: [{ date: "2026-05-01", notes: "hola", amount: "10.00" }],
      filename: "entries.csv",
      t,
    });
    expect(csv.split("\n")[0]).toBe("Fecha,Notas,Monto");
    expect(csv).toContain("hola");
    expect(csvExport.downloadCsv).toHaveBeenCalledWith(
      "entries.csv",
      expect.stringContaining("Fecha"),
    );
  });

  it("guards formula injection in cells", () => {
    const t = (k: string) => k;
    const csv = exportLocalizedCsv({
      columns: [{ key: "notes", headerKey: "notes" }],
      rows: [{ notes: "=cmd()" }],
      filename: "x.csv",
      t,
    });
    expect(csv).toMatch(/'=cmd\(\)/);
  });
});
