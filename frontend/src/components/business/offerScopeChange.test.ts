import {
  applyOfferScopeChange,
  applyOfferTargetSelection,
  isOfferScope,
} from "./offerScopeChange";

describe("#743 offer scope commits", () => {
  it("ignores an empty NextUI Select onChange so Escape does not wipe", () => {
    expect(
      applyOfferScopeChange(
        { applicable_to: "item", target_id: "steak-1" },
        "",
      ),
    ).toBeNull();
  });

  it("keeps the committed target when Applies-to is unchanged", () => {
    expect(
      applyOfferScopeChange(
        { applicable_to: "item", target_id: "steak-1" },
        "item",
      ),
    ).toEqual({ applicable_to: "item", target_id: "steak-1" });
  });

  it("clears Target only when Applies-to actually changes", () => {
    expect(
      applyOfferScopeChange(
        { applicable_to: "item", target_id: "steak-1" },
        "category",
      ),
    ).toEqual({ applicable_to: "category", target_id: "" });
  });

  it("accepts the all-items scope without treating it as a React Aria sentinel", () => {
    expect(isOfferScope("all")).toBe(true);
    expect(
      applyOfferScopeChange(
        { applicable_to: "item", target_id: "steak-1" },
        "all",
      ),
    ).toEqual({ applicable_to: "all", target_id: "" });
  });

  it("ignores Autocomplete null while the input still shows a label", () => {
    expect(applyOfferTargetSelection(null, "Steak Plate")).toEqual({
      kind: "ignore",
    });
  });

  it("honors a real clear when the operator emptied the field", () => {
    expect(applyOfferTargetSelection(null, "")).toEqual({ kind: "clear" });
    expect(applyOfferTargetSelection("", "   ")).toEqual({ kind: "clear" });
  });

  it("commits a selected target key", () => {
    expect(applyOfferTargetSelection("steak-1", "Steak")).toEqual({
      kind: "commit",
      target_id: "steak-1",
    });
  });
});
