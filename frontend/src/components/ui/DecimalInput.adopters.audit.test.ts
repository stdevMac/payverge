/**
 * A3b — structural audit of every production DecimalInput adopter.
 * Each site must pass min (blur-clamp bound) so NR-1/NR-2 class regressions
 * cannot hide behind optional props.
 */
import fs from "fs";
import path from "path";

const ROOT = path.join(process.cwd(), "src");

function walk(dir: string, acc: string[] = []): string[] {
  for (const ent of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, ent.name);
    if (ent.isDirectory()) {
      if (ent.name === "node_modules" || ent.name === "__tests__") continue;
      walk(p, acc);
    } else if (
      (ent.name.endsWith(".tsx") || ent.name.endsWith(".ts")) &&
      !ent.name.includes(".test.")
    ) {
      acc.push(p);
    }
  }
  return acc;
}

/** Return the DecimalInput self-closing block that contains data-testid="tid". */
function blockForTestId(src: string, tid: string): string | null {
  const needle = `data-testid="${tid}"`;
  const idx = src.indexOf(needle);
  if (idx < 0) return null;
  // Last DecimalInput open before the testid (skip nested Input in startContent).
  const before = src.slice(0, idx);
  const openAt = before.lastIndexOf("<DecimalInput");
  if (openAt < 0) return null;
  const after = src.slice(openAt);
  const close = after.match(/^<DecimalInput\b[\s\S]*?\/>/);
  return close ? close[0] : null;
}

describe("A3b DecimalInput adopters audit", () => {
  const files = walk(ROOT).filter((f) => {
    const src = fs.readFileSync(f, "utf8");
    return src.includes("<DecimalInput");
  });

  it("finds DecimalInput production call sites", () => {
    expect(files.length).toBeGreaterThanOrEqual(6);
  });

  it("every DecimalInput block has min= (blur-clamp bound)", () => {
    const missing: string[] = [];
    for (const f of files) {
      const src = fs.readFileSync(f, "utf8");
      const blocks = src.match(/<DecimalInput\b[\s\S]*?\/>/g) ?? [];
      for (const b of blocks) {
        if (!/\bmin=\{/.test(b) && !/\bmin=/.test(b)) {
          missing.push(`${path.relative(ROOT, f)}: ${b.slice(0, 80)}…`);
        }
      }
    }
    expect(missing).toEqual([]);
  });

  it("money/qty testids use DecimalInput with isInvalid or errorMessage", () => {
    const critical = [
      "bundle-price-input",
      "add-item-price",
      "add-item-cogs",
      "add-item-option-price",
      "edit-item-price",
      "edit-item-cogs",
      "edit-item-option-price",
      "loyalty-points-rate",
      "quick-adjust-quantity",
      "inventory-current-quantity",
      "inventory-cost-per-unit",
      "shift-break-minutes",
      "zone-delivery-fee",
      "zone-minimum-order",
      "delivery-base-fee",
      "delivery-free-above",
      "delivery-minimum-order",
      "record-payment-amount",
      "record-payment-tip",
      "alt-payment-amount",
      "ledger-entry-amount",
    ];
    for (const tid of critical) {
      let found = false;
      for (const f of files) {
        const src = fs.readFileSync(f, "utf8");
        const block = blockForTestId(src, tid);
        if (!block) continue;
        expect(block.startsWith("<DecimalInput")).toBe(true);
        expect(block).toMatch(/isInvalid|errorMessage/);
        expect(block).toMatch(/\bmin=/);
        found = true;
        break;
      }
      expect({ tid, found }).toEqual({ tid, found: true });
    }
    // Dynamic testid template `tier-threshold-${i}` — match pattern in source.
    const tierSrc = fs.readFileSync(
      path.join(ROOT, "components/business/crm/loyalty/TierEditor.tsx"),
      "utf8",
    );
    expect(tierSrc).toMatch(/tier-threshold-\$\{i\}/);
    expect(tierSrc).toMatch(/<DecimalInput[\s\S]*?tier-threshold/);
    expect(tierSrc).toMatch(/isInvalid/);
    expect(tierSrc).toMatch(/\bmin=\{0\}/);
    // Schedule fields pass testId= into NumberField → data-testid={testId}.
    const scheduleSrc = fs.readFileSync(
      path.join(ROOT, "components/business/schedule/ScheduleSettingsModal.tsx"),
      "utf8",
    );
    expect(scheduleSrc).toMatch(/testId="schedule-shift-hours"/);
    expect(scheduleSrc).toMatch(/isInvalid=\{!valid\}/);
    expect(scheduleSrc).toMatch(/min=\{min\}/);
  });

  it("L3-5: add and edit option price both use DecimalInput (not type=number)", () => {
    const add = fs.readFileSync(
      path.join(ROOT, "components/business/modals/AddItemModal.tsx"),
      "utf8",
    );
    const edit = fs.readFileSync(
      path.join(ROOT, "components/business/modals/EditItemModal.tsx"),
      "utf8",
    );
    const addBlock = blockForTestId(add, "add-item-option-price");
    const editBlock = blockForTestId(edit, "edit-item-option-price");
    expect(addBlock).toBeTruthy();
    expect(editBlock).toBeTruthy();
    expect(addBlock!.startsWith("<DecimalInput")).toBe(true);
    expect(editBlock!.startsWith("<DecimalInput")).toBe(true);
    expect(addBlock).not.toMatch(/type=["']number["']/);
    expect(editBlock).not.toMatch(/type=["']number["']/);
  });
});
