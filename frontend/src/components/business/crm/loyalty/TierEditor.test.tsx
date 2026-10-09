/** @jest-environment jsdom */
import { render, screen, fireEvent } from "@testing-library/react";
import type { LoyaltyTier } from "@/api/loyalty";
import TierEditor, { validateTiers, validateTiersOnCommit } from "./TierEditor";

// Return the raw key (with {name} placeholder intact) so the test can assert
// the editor picked the right validation message + interpolated params.
jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => {
    const leaf = key.split(".").pop() ?? key;
    const stub: Record<string, string> = {
      newTierName: "New tier",
      addTier: "Add tier",
      tierName: "Tier name",
      deleteTier: "Delete tier",
      tierColor: "Color",
      tierFallback: "tier",
      colorForTier: "Color for {tier}",
      errorBlankName: "Every tier needs a name.",
      errorDuplicateName: 'Two tiers can\'t share the name "{name}".',
      errorDuplicateThreshold: "Two tiers can't share the same threshold.",
      errorThresholdOrder: "Tier thresholds must increase.",
    };
    return stub[leaf] ?? key;
  },
}));

const tier = (over: Partial<LoyaltyTier> = {}): LoyaltyTier => ({
  name: "Bronze",
  min_lifetime_spent: 0,
  sort_order: 0,
  ...over,
});

describe("validateTiers (threshold-only, per-keystroke)", () => {
  it("accepts a strictly-increasing ladder", () => {
    expect(
      validateTiers([
        tier({ name: "Bronze", min_lifetime_spent: 0 }),
        tier({ name: "Silver", min_lifetime_spent: 25000 }),
        tier({ name: "Gold", min_lifetime_spent: 75000 }),
      ]),
    ).toBeNull();
  });

  it("does NOT reject duplicate names (deferred to commit)", () => {
    // A transient duplicate name mid-typing must pass the per-keystroke check.
    expect(
      validateTiers([
        tier({ name: "Bronze", min_lifetime_spent: 0 }),
        tier({ name: "bronze", min_lifetime_spent: 25000 }),
      ]),
    ).toBeNull();
  });

  it("rejects two tiers sharing the same threshold", () => {
    const err = validateTiers([
      tier({ name: "Bronze", min_lifetime_spent: 0 }),
      tier({ name: "Silver", min_lifetime_spent: 0 }),
    ]);
    expect(err?.key).toBe("errorDuplicateThreshold");
  });

  it("rejects thresholds that descend through the displayed ladder", () => {
    const err = validateTiers([
      tier({ name: "Bronze", min_lifetime_spent: 250, sort_order: 0 }),
      tier({ name: "Silver", min_lifetime_spent: 100, sort_order: 1 }),
    ]);
    expect(err?.key).toBe("errorThresholdOrder");
  });
});

describe("validateTiersOnCommit (names + thresholds)", () => {
  it("rejects duplicate names case-insensitively", () => {
    const err = validateTiersOnCommit([
      tier({ name: "Bronze", min_lifetime_spent: 0 }),
      tier({ name: "bronze", min_lifetime_spent: 25000 }),
    ]);
    expect(err?.key).toBe("errorDuplicateName");
    expect(err?.params?.name).toBe("bronze");
  });

  it("still rejects duplicate thresholds", () => {
    const err = validateTiersOnCommit([
      tier({ name: "Bronze", min_lifetime_spent: 0 }),
      tier({ name: "Silver", min_lifetime_spent: 0 }),
    ]);
    expect(err?.key).toBe("errorDuplicateThreshold");
  });

  it("rejects blank tier names on commit", () => {
    const err = validateTiersOnCommit([
      tier({ name: "  ", min_lifetime_spent: 0 }),
      tier({ name: "Silver", min_lifetime_spent: 25000, sort_order: 1 }),
    ]);
    expect(err?.key).toBe("errorBlankName");
  });

  it.each([
    ["Go-trimmed Unicode whitespace", "\u0085Gold\u0085", "Gold"],
    ["simple lowercase for dotted I", "İ", "i"],
    ["simple lowercase without Greek final sigma context", "ΟΣ", "οσ"],
  ])("matches the API for %s", (_caseName, left, right) => {
    const err = validateTiersOnCommit([
      tier({ name: left, min_lifetime_spent: 0 }),
      tier({ name: right, min_lifetime_spent: 25000 }),
    ]);
    expect(err?.key).toBe("errorDuplicateName");
  });

  it("does not over-normalize FEFF that the API treats as content", () => {
    const err = validateTiersOnCommit([
      tier({ name: "\ufeffGold", min_lifetime_spent: 0 }),
      tier({ name: "Gold", min_lifetime_spent: 25000 }),
    ]);
    expect(err).toBeNull();
  });
});

describe("TierEditor (component)", () => {
  it("adds a tier above the ceiling on the default $0 base ladder (R3-ML-1)", () => {
    const onChange = jest.fn();
    // Default Bronze=0 / Silver / Gold ladder. Adding must WORK — the new row
    // is seeded at max(threshold)+1 so it never collides with the base $0 tier.
    render(
      <TierEditor
        tiers={[
          tier({ name: "Bronze", min_lifetime_spent: 0 }),
          tier({ name: "Silver", min_lifetime_spent: 25000, sort_order: 1 }),
          tier({ name: "Gold", min_lifetime_spent: 75000, sort_order: 2 }),
        ]}
        onChange={onChange}
      />,
    );

    fireEvent.click(screen.getByText("Add tier"));

    expect(onChange).toHaveBeenCalledTimes(1);
    const next = onChange.mock.calls[0][0] as LoyaltyTier[];
    expect(next).toHaveLength(4);
    // Seeded strictly above the current ceiling (75000).
    expect(next[3].min_lifetime_spent).toBe(75001);
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("adds a tier even when only the $0 base tier exists", () => {
    const onChange = jest.fn();
    render(
      <TierEditor
        tiers={[tier({ name: "Bronze", min_lifetime_spent: 0 })]}
        onChange={onChange}
      />,
    );

    fireEvent.click(screen.getByText("Add tier"));

    expect(onChange).toHaveBeenCalledTimes(1);
    const next = onChange.mock.calls[0][0] as LoyaltyTier[];
    expect(next[1].min_lifetime_spent).toBe(1);
  });

  it("assigns a new tier after the current maximum sort order", () => {
    const onChange = jest.fn();
    render(
      <TierEditor
        tiers={[
          tier({ name: "Bronze", min_lifetime_spent: 0, sort_order: 1 }),
          tier({ name: "Silver", min_lifetime_spent: 25000, sort_order: 2 }),
          tier({ name: "Gold", min_lifetime_spent: 75000, sort_order: 4 }),
        ]}
        onChange={onChange}
      />,
    );

    fireEvent.click(screen.getByText("Add tier"));

    const next = onChange.mock.calls[0][0] as LoyaltyTier[];
    expect(next[3].sort_order).toBe(5);
  });

  it("does not swallow keystrokes when a name passes through an existing one (R3-ML-3)", () => {
    const onChange = jest.fn();
    // Renaming "Gol" → "Gold" → "Golden" — the intermediate "Gold" collides
    // with the existing Gold tier's name, but per-keystroke edits must not drop.
    render(
      <TierEditor
        tiers={[
          tier({ name: "Gold", min_lifetime_spent: 0 }),
          tier({ name: "Gol", min_lifetime_spent: 25000, sort_order: 1 }),
        ]}
        onChange={onChange}
      />,
    );

    const nameInputs = screen.getAllByPlaceholderText("Tier name");
    fireEvent.change(nameInputs[1], { target: { value: "Gold" } });

    // The keystroke propagated despite the transient duplicate name.
    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange.mock.calls[0][0][1].name).toBe("Gold");
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("surfaces a duplicate-name error on blur", () => {
    const onChange = jest.fn();
    render(
      <TierEditor
        tiers={[
          tier({ name: "Gold", min_lifetime_spent: 0 }),
          tier({ name: "Gold", min_lifetime_spent: 25000, sort_order: 1 }),
        ]}
        onChange={onChange}
      />,
    );

    const nameInputs = screen.getAllByPlaceholderText("Tier name");
    fireEvent.blur(nameInputs[1]);

    expect(screen.getByRole("alert")).toHaveTextContent(
      'Two tiers can\'t share the name "Gold".',
    );
  });

  it("surfaces a localized blank-name error on blur", () => {
    const onChange = jest.fn();
    render(
      <TierEditor
        tiers={[
          tier({ name: " ", min_lifetime_spent: 0 }),
          tier({ name: "Silver", min_lifetime_spent: 25000, sort_order: 1 }),
        ]}
        onChange={onChange}
      />,
    );

    fireEvent.blur(screen.getAllByPlaceholderText("Tier name")[0]);

    expect(screen.getByRole("alert")).toHaveTextContent(
      "Every tier needs a name.",
    );
  });

  it("does not call onChange when an edit collides on threshold", () => {
    const onChange = jest.fn();
    render(
      <TierEditor
        tiers={[
          tier({ name: "Bronze", min_lifetime_spent: 0 }),
          tier({ name: "Silver", min_lifetime_spent: 25000, sort_order: 1 }),
        ]}
        onChange={onChange}
      />,
    );

    // A3b: DecimalInput (type=text) commits threshold on blur, not every keystroke.
    const threshold = screen.getByTestId("tier-threshold-1");
    fireEvent.change(threshold, { target: { value: "0" } });
    fireEvent.blur(threshold);

    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Two tiers can't share the same threshold.",
    );
  });

  it("propagates a valid edit and clears any prior error", () => {
    const onChange = jest.fn();
    render(
      <TierEditor
        tiers={[
          tier({ name: "Bronze", min_lifetime_spent: 0 }),
          tier({ name: "Silver", min_lifetime_spent: 25000, sort_order: 1 }),
        ]}
        onChange={onChange}
      />,
    );

    const threshold = screen.getByTestId("tier-threshold-1");
    fireEvent.change(threshold, { target: { value: "50000" } });
    fireEvent.blur(threshold);

    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange.mock.calls[0][0][1].min_lifetime_spent).toBe(50000);
    expect(screen.queryByRole("alert")).toBeNull();
  });
});
