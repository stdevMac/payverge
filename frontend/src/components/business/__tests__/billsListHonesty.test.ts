import { resolveBillsListHonesty } from "../billsListHonesty";

const base = {
  listHydrated: false,
  listLoadFailed: false,
  globalBillsFailed: false,
  resultCount: 0,
  visibleValue: 0,
  queryIsScoped: false,
};

describe("resolveBillsListHonesty (#647)", () => {
  it("hides money before the first successful list hydrate", () => {
    expect(resolveBillsListHonesty(base)).toEqual({
      showFailureState: false,
      showMoneyStats: false,
      keepLastKnownList: false,
    });
  });

  it("does not paint $0 when the list fetch fails with no last-known totals", () => {
    expect(
      resolveBillsListHonesty({
        ...base,
        listLoadFailed: true,
      }),
    ).toEqual({
      showFailureState: true,
      showMoneyStats: false,
      keepLastKnownList: false,
    });
  });

  it("does not paint $0 when the unfiltered live feed failed and the list is empty", () => {
    expect(
      resolveBillsListHonesty({
        ...base,
        listHydrated: true,
        globalBillsFailed: true,
      }),
    ).toEqual({
      showFailureState: true,
      showMoneyStats: false,
      keepLastKnownList: false,
    });
  });

  it("does not treat a later search or history empty as a live-feed outage", () => {
    expect(
      resolveBillsListHonesty({
        ...base,
        listHydrated: true,
        globalBillsFailed: true,
        queryIsScoped: true,
      }),
    ).toEqual({
      showFailureState: false,
      showMoneyStats: true,
      keepLastKnownList: false,
    });
  });

  it("keeps last-known totals when a later list fetch fails", () => {
    expect(
      resolveBillsListHonesty({
        ...base,
        listHydrated: true,
        listLoadFailed: true,
        globalBillsFailed: true,
        resultCount: 3,
        visibleValue: 73.9,
      }),
    ).toEqual({
      showFailureState: true,
      showMoneyStats: true,
      keepLastKnownList: true,
    });
  });

  it("still paints a genuine empty till after both feeds succeed empty", () => {
    expect(
      resolveBillsListHonesty({
        ...base,
        listHydrated: true,
      }),
    ).toEqual({
      showFailureState: false,
      showMoneyStats: true,
      keepLastKnownList: false,
    });
  });

  it("shows money when the list hydrated with bills even if the live feed missed", () => {
    expect(
      resolveBillsListHonesty({
        ...base,
        listHydrated: true,
        globalBillsFailed: true,
        resultCount: 3,
        visibleValue: 73.9,
      }),
    ).toEqual({
      showFailureState: false,
      showMoneyStats: true,
      keepLastKnownList: false,
    });
  });
});
