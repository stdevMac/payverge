import { readFileSync } from "fs";
import { join } from "path";

// WCAG 2.4.1: the root layout's "Skip to main content" link points at
// #main-content. Every shell rendered under that root layout — including
// routes outside (shop) — must provide a focusable <main id="main-content">.
// Checking only known-good shells lets 404 /t leave the skip link a no-op.
const FOCUSABLE_MAIN_SKIP_TARGET =
  /<main[\s\S]{0,240}id="main-content"[\s\S]{0,120}tabIndex=\{-1\}/;

function readApp(relPath: string): string {
  return readFileSync(join(__dirname, relPath), "utf8");
}

describe("skip-link target exists on every shell", () => {
  const shellLayout = readApp("(shop)/layout.tsx");
  const adminLayout = readApp("admin/AdminLayoutClient.tsx");
  const publicBusinessPage = readFileSync(
    join(
      __dirname,
      "..",
      "components",
      "business-page",
      "ConvertingBusinessLandingPage.tsx",
    ),
    "utf8",
  );
  const notFound = `${readApp("not-found.tsx")}\n${readApp("NotFoundClient.tsx")}`;
  const tableLayout = readApp("t/[tableCode]/layout.tsx");

  it("public (shop) shell <main> has id=main-content + tabIndex", () => {
    expect(shellLayout).toContain('id="main-content"');
    expect(shellLayout).toContain("tabIndex={-1}");
  });

  it("admin shell <main> has id=main-content + tabIndex", () => {
    expect(adminLayout).toContain('id="main-content"');
    expect(adminLayout).toContain("tabIndex={-1}");
  });

  it("public business pages provide the global skip link's real focusable main target", () => {
    expect(publicBusinessPage).toMatch(FOCUSABLE_MAIN_SKIP_TARGET);
  });

  it("root 404 (outside (shop)) provides the skip link's focusable main target", () => {
    expect(notFound).toMatch(FOCUSABLE_MAIN_SKIP_TARGET);
  });

  it("tableCode shell provides the skip link's focusable main target", () => {
    expect(tableLayout).toMatch(FOCUSABLE_MAIN_SKIP_TARGET);
  });

  it("public business pages do not render a second Skip to main content link", () => {
    expect(publicBusinessPage).not.toContain('href="#tab-content"');
  });

  it("public tab content does not force a second full viewport below the hero", () => {
    expect(publicBusinessPage).not.toMatch(
      /className="min-h-screen"\s+id="tab-content"/,
    );
  });
});
