import { queryKeys } from "@/api/queryKeys";

describe("queryKeys", () => {
  it("normalizes order list filters into stable primitive keys", () => {
    expect(
      queryKeys.orders.list("7", { status: "open", table_id: 3 }),
    ).toEqual(
      queryKeys.orders.list("7", { table_id: 3, status: "open" }),
    );
    expect(queryKeys.orders.list("7", { status: undefined })).toEqual(
      queryKeys.orders.list("7"),
    );
  });

  it("keeps guest and inside alternative-payment keys in separate namespaces", () => {
    expect(queryKeys.payments.alternativeGuest("same-id")).toEqual([
      "payments",
      "alternative",
      "guest",
      "same-id",
    ]);
    expect(queryKeys.payments.alternativeInside("same-id")).toEqual([
      "payments",
      "alternative",
      "inside",
      "same-id",
    ]);
    expect(queryKeys.payments.alternativeGuest("same-id")).not.toEqual(
      queryKeys.payments.alternativeInside("same-id"),
    );
  });
});

describe("per-principal mine keys", () => {
  it("include the staff id so principals never share cached rows", () => {
    expect(queryKeys.availability.mine("42", 7)).toEqual(["availability", "42", "mine", "7"]);
    expect(queryKeys.timesheet.mine("42", 7)).toEqual(["timesheet", "42", "mine", "7"]);
    expect(queryKeys.coverage.mine("42", 7)).toEqual(["coverage", "42", "mine", "7"]);
    expect(queryKeys.coverage.mine("42", 7)).not.toEqual(queryKeys.coverage.mine("42", 8));
  });
});
