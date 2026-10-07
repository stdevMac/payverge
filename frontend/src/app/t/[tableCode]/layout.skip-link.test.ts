/**
 * /t/<tableCode> sits outside (shop), so the marketing shell's
 * <main id="main-content"> never wraps it. The table layout must own the
 * skip-link target; page-level <main>s would nest once the shell does.
 */
import { readFileSync } from "fs";
import { join } from "path";

const FOCUSABLE_MAIN_SKIP_TARGET =
  /<main[\s\S]{0,240}id="main-content"[\s\S]{0,120}tabIndex=\{-1\}/;

const layout = readFileSync(join(__dirname, "layout.tsx"), "utf8");
const billPage = readFileSync(join(__dirname, "bill/page.tsx"), "utf8");
const billError = readFileSync(join(__dirname, "bill/error.tsx"), "utf8");
const tableError = readFileSync(join(__dirname, "error.tsx"), "utf8");
const guestTableView = readFileSync(
  join(
    __dirname,
    "..",
    "..",
    "..",
    "components",
    "guest",
    "GuestTableView.tsx",
  ),
  "utf8",
);

describe("tableCode layout skip-link target", () => {
  it("wraps children in a focusable main#main-content landmark", () => {
    expect(layout).toMatch(FOCUSABLE_MAIN_SKIP_TARGET);
  });

  it("does not nest a second <main> on bill / landing / error surfaces", () => {
    expect(billPage).not.toMatch(/<main[\s>]/);
    expect(billError).not.toMatch(/<main[\s>]/);
    expect(tableError).not.toMatch(/<main[\s>]/);
    expect(guestTableView).not.toMatch(/<main[\s>]/);
  });

  it("does not put a second skip-target id on bill (layout owns it)", () => {
    expect(billPage).not.toContain('id="main-content"');
  });
});
