import fs from "fs";
import path from "path";

// A-3: real body/label/small text must use *-500 (#6b6358, 5.6:1 AA), not
// *-400 (#857d6e, 3.9:1 AA-large-only). gray/ink/warm all alias the same
// neutralScale (tailwind.config.ts), so any of the three at -400 is the bug.
// Icon tints (text-*-400 on lucide glyphs) are EXEMPT and must NOT be matched.
const root = path.resolve(__dirname, "../../..");
const read = (rel: string) => fs.readFileSync(path.resolve(root, rel), "utf8");

describe("A-3 contrast: real text uses *-500, not *-400", () => {
  it("PublicMenuDisplay fulfillment <dt> labels are not text-*-400", () => {
    const src = read("components/business-page/PublicMenuDisplay.tsx");
    // The four <dt> labels: `text-[11px] uppercase tracking-[0.2em] text-<n>-400`
    expect(src).not.toMatch(
      /text-\[11px\] uppercase tracking-\[0\.2em\] text-(?:gray|ink|warm)-400/,
    );
  });

  it("PublicMenuDisplay bundle-refs <p> is not text-*-400", () => {
    const src = read("components/business-page/PublicMenuDisplay.tsx");
    // The refs line: `text-[11px] text-<n>-400 mt-1 truncate`
    expect(src).not.toMatch(
      /text-\[11px\] text-(?:gray|ink|warm)-400 mt-1 truncate/,
    );
  });

  it("AiWaiter char-count + cart disclaimer text is not text-*-400", () => {
    const src = read("components/guest/AiWaiter.tsx");
    // char-count: `text-right text-[10px] text-<n>-400 mt-1`
    expect(src).not.toMatch(
      /text-right text-\[10px\] text-(?:gray|ink|warm)-400/,
    );
    // cart disclaimer: `text-center text-[10px] sm:text-[10px] text-<n>-400`
    expect(src).not.toMatch(
      /text-center text-\[10px\] sm:text-\[10px\] text-(?:gray|ink|warm)-400/,
    );
  });

  it("GuestTableView powered-by <p> label is not text-*-400", () => {
    const src = read("components/guest/GuestTableView.tsx");
    // `text-label text-<n>-400` (12px, not WCAG-large)
    expect(src).not.toMatch(/text-label text-(?:gray|ink|warm)-400/);
  });

  it("BusinessContactTab closed-hours <span> is not text-*-400", () => {
    const src = read("components/business-page/BusinessContactTab.tsx");
    // closed-hours ternary: `hours.is_closed ? "text-<n>-400" : "text-<n>-900"`
    expect(src).not.toMatch(/is_closed \? "text-(?:gray|ink|warm)-400"/);
  });

  it("MenuItemNoMediaHeader label is ink-700+ (survives closed-menu opacity composites)", () => {
    const src = read("components/business-page/MenuItemNoMediaHeader.tsx");
    // Public-menu release smoke: "No image" must stay ≥4.5:1. ink-500 on
    // bg-ink-100 falls to ~3.5:1 when any ancestor applies opacity-80 over cream.
    expect(src).toMatch(/text-ink-700/);
    expect(src).not.toMatch(
      /<span className="[^"]*text-ink-(?:400|500)[^"]*">/,
    );
  });

  it("PublicMenuDisplay does not opacity-dim whole menu cards (AA contrast)", () => {
    const src = read("components/business-page/PublicMenuDisplay.tsx");
    // Whole-card opacity-80 was muting closed-mode items and dropping mid-tone
    // labels (No image) below WCAG AA. Closed state is already announced by the
    // menu-closed-hint banner; do not reintroduce card-level opacity muting.
    expect(src).not.toMatch(/opacity-80/);
  });
});
