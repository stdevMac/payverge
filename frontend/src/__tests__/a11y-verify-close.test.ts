/**
 * Verify-and-close regression gates for PG-7, L1-18, F-cand-6.
 * GREEN without production change for these findings (already fixed on main).
 */

import fs from "fs";
import path from "path";

const SRC = path.resolve(__dirname, "..");

function read(rel: string): string {
  return fs.readFileSync(path.join(SRC, rel), "utf8");
}

describe("PG-7 /b menu cards interactive + dietary chips named", () => {
  const src = read("components/business-page/PublicMenuDisplay.tsx");

  it("menu cards are buttons with openItem aria-label", () => {
    expect(src).toMatch(/type=["']button["']/);
    expect(src).toMatch(/accessibility\.openItem/);
  });

  it("dietary chips render translated text content", () => {
    expect(src).toMatch(/DIETARY_TAGS/);
    expect(src).toMatch(/t\(["']menu\./);
  });
});

describe("L1-18 Manage Menu hit target", () => {
  const src = read("components/business/BusinessOverview.tsx");

  it("quick actions are full-width buttons with py-3.5 padding", () => {
    expect(src).toMatch(/type=["']button["']/);
    expect(src).toMatch(/py-3\.5/);
    expect(src).toMatch(/w-full/);
  });
});

describe("F-cand-6 sidebar label-in-name / tooltip", () => {
  const src = read("components/business/sidebar/SidebarNavRow.tsx");

  // M5 wrapped every row in a SidebarTooltip and appended the description when
  // the row was expanded. #726 kept the accessible-name guarantee but narrowed
  // where the bubble may appear: an expanded row already prints its label, so
  // repeating it in a bubble only covered the neighbouring rows. The bubble now
  // survives only on the collapsed desktop rail, where the label is
  // width/opacity-zeroed and the icon alone is not a name.
  it("names the collapsed rail's icon-only row with its label", () => {
    expect(src).toMatch(/SidebarTooltip/);
    expect(src).toMatch(/label=\{title\}/);
  });

  it("shows no hover bubble once the row prints its own label", () => {
    expect(src).toMatch(/if \(!collapsed\) return row;/);
    // The description prop is accepted (tab config still passes it) but must
    // not reach the bubble — that was the duplicated copy operators reported.
    expect(src).not.toMatch(/description=\{/);
  });
});
