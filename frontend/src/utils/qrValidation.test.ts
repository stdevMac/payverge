import { validateTableCode, extractTableCodeFromURL, isValidTableCodeFormat } from "./qrValidation";
import { getTableByCode } from "../api/bills";

jest.mock("../api/bills", () => ({
  getTableByCode: jest.fn(),
}));

const mockedGetTableByCode = getTableByCode as jest.Mock;

// The public /guest/table/:code payload (buildPublicGuestTableResponse) carries
// ONLY these fields — no `status`. The validator must accept it as valid.
const realPayload = {
  table: {
    table_code: "ABC123",
    name: "Table 5",
    capacity: 4,
    is_active: true,
  },
  business: { id: 42 },
  menu: { categories: "[]" },
  categories: [],
};

describe("isValidTableCodeFormat", () => {
  it("accepts legacy short codes and demo slug codes", () => {
    expect(isValidTableCodeFormat("ABC123")).toBe(true);
    expect(isValidTableCodeFormat("T1")).toBe(true);
    expect(isValidTableCodeFormat("demo-50-core-table-01")).toBe(true);
    expect(isValidTableCodeFormat("QR-003")).toBe(true);
  });

  it("rejects empty and punctuation-only garbage", () => {
    expect(isValidTableCodeFormat("")).toBe(false);
    expect(isValidTableCodeFormat("!!")).toBe(false);
    expect(isValidTableCodeFormat("ab")).toBe(true); // 2+ chars ok
    expect(isValidTableCodeFormat("a")).toBe(false);
  });
});

describe("validateTableCode", () => {
  beforeEach(() => jest.clearAllMocks());

  it("accepts a valid code against the REAL backend payload shape (no `status` field)", async () => {
    mockedGetTableByCode.mockResolvedValue(realPayload);

    const result = await validateTableCode("abc123");

    // Regression: the old code checked `table.status !== 'active'` against a
    // field the backend never sends, so undefined !== 'active' rejected every
    // valid code and the entire in-app scan / manual-entry flow was a dead end.
    expect(result.isValid).toBe(true);
    expect(result.tableCode).toBe("ABC123");
  });

  it("accepts demo slug table codes with hyphens (live /scan entry)", async () => {
    mockedGetTableByCode.mockResolvedValue({
      ...realPayload,
      table: {
        ...realPayload.table,
        table_code: "demo-50-core-table-01",
      },
    });

    const result = await validateTableCode("demo-50-core-table-01");
    expect(result.isValid).toBe(true);
    expect(result.tableCode).toBe("demo-50-core-table-01");
    expect(mockedGetTableByCode).toHaveBeenCalledWith("DEMO-50-CORE-TABLE-01");
  });

  it("rejects a table the backend explicitly marks inactive", async () => {
    mockedGetTableByCode.mockResolvedValue({
      ...realPayload,
      table: { ...realPayload.table, is_active: false },
    });

    const result = await validateTableCode("ABC123");
    expect(result.isValid).toBe(false);
    // A stable, locale-independent code so /scan can translate the message
    // instead of surfacing the English `error` string on a 21-locale screen.
    expect(result.errorCode).toBe("inactive");
  });

  it("rejects malformed codes without hitting the backend", async () => {
    const result = await validateTableCode("!!");
    expect(result.isValid).toBe(false);
    expect(result.errorCode).toBe("format");
    expect(mockedGetTableByCode).not.toHaveBeenCalled();
  });

  it("rejects when the backend has no such table", async () => {
    mockedGetTableByCode.mockRejectedValue(new Error("404"));
    const result = await validateTableCode("ZZZ999");
    expect(result.isValid).toBe(false);
    expect(result.errorCode).toBe("notFound");
  });
});

describe("extractTableCodeFromURL", () => {
  it("pulls and upper-cases the code from a /t/ deep link", () => {
    expect(extractTableCodeFromURL("https://payverge.io/t/abc123")).toBe("ABC123");
  });
  it("pulls slug demo codes from /t/ deep links", () => {
    expect(extractTableCodeFromURL("https://payverge.io/t/demo-50-core-table-01")).toBe(
      "DEMO-50-CORE-TABLE-01",
    );
    expect(extractTableCodeFromURL("/t/demo-51-ai-pro-table-01/menu")).toBe(
      "DEMO-51-AI-PRO-TABLE-01",
    );
  });
  it("returns null for a non-table URL", () => {
    expect(extractTableCodeFromURL("https://payverge.io/pricing")).toBeNull();
  });
});
