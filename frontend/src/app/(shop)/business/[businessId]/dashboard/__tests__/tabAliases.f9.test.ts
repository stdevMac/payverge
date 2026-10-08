/** Gate F-9 (audit §18): ?tab=ai_waiter / director_console / waiter all normalize. */
import fs from "node:fs";
import path from "node:path";
import { resolveTabAlias, TAB_ALIASES } from "../tabParams";

describe("F-9 tab alias normalization (L4-1)", () => {
  it("normalizes ?tab=ai_waiter → ai-waiter", () => {
    expect(resolveTabAlias("ai_waiter")).toBe("ai-waiter");
    expect(TAB_ALIASES.ai_waiter).toBe("ai-waiter");
  });

  it("normalizes ?tab=waiter → ai-waiter", () => {
    expect(resolveTabAlias("waiter")).toBe("ai-waiter");
  });

  it("normalizes ?tab=director_console → director-console", () => {
    expect(resolveTabAlias("director_console")).toBe("director-console");
  });

  it("still normalizes the pre-existing singular aliases", () => {
    expect(resolveTabAlias("director")).toBe("director-console");
    expect(resolveTabAlias("bill")).toBe("bills");
    expect(resolveTabAlias("table")).toBe("tables");
    expect(resolveTabAlias("plugin")).toBe("plugins");
    expect(resolveTabAlias("reservation")).toBe("reservations");
    // #193: plural operator wording aliases to canonical TabKey `counter`.
    expect(resolveTabAlias("counters")).toBe("counter");
  });

  it("dashboard staff resolve aliases sub=tips onto people (issue 354)", () => {
    const source = fs.readFileSync(
      path.resolve(__dirname, "../page.tsx"),
      "utf-8",
    );
    expect(source).toMatch(/tips:\s*"people"/);
    expect(source).toMatch(/TEAM_SUB_ALIASES/);
    expect(source).toMatch(/resolveSubTab\(\s*\{ sub: rawSub \},\s*TEAM_SUB_TABS,\s*"people",\s*TEAM_SUB_ALIASES/);
  });

  it("returns null for genuinely unknown tabs (not-found path)", () => {
    expect(resolveTabAlias("totally_fake_tab")).toBeNull();
    expect(resolveTabAlias(null)).toBeNull();
    expect(resolveTabAlias("")).toBeNull();
  });

  it("dashboard page imports TAB_ALIASES from tabParams (single registry)", () => {
    const source = fs.readFileSync(
      path.resolve(__dirname, "../page.tsx"),
      "utf-8",
    );
    expect(source).toMatch(/import\s*\{[^}]*TAB_ALIASES[^}]*\}\s*from\s*["']\.\/tabParams["']/);
    // No local re-declaration of the alias map
    expect(source).not.toMatch(/const\s+TAB_ALIASES\s*[:=]/);
  });
});

describe("L1-19 commitTabChange uses push for user-initiated tab changes", () => {
  const source = fs.readFileSync(
    path.resolve(__dirname, "../page.tsx"),
    "utf-8",
  );

  it("commitTabChange navigates with router.push (not replace)", () => {
    // Extract the commitTabChange body roughly and assert push is used for
    // the URL write that follows nextTabSearchParams.
    const start = source.indexOf("const commitTabChange = useCallback");
    expect(start).toBeGreaterThan(-1);
    const end = source.indexOf("const handleSetActiveTab", start);
    expect(end).toBeGreaterThan(start);
    const body = source.slice(start, end);
    expect(body).toMatch(/router\.push\(/);
    expect(body).not.toMatch(/router\.replace\(/);
    // Policy comment present so future edits do not silently flip back
    expect(body).toMatch(/L1-19/);
  });

  it("programmatic normalize/cleanup sites still use replace", () => {
    // Permission gate strip, alias rewrite, default-sub write, onboarding
    // cleanup — all must remain replace so they do not spam the back stack.
    const replaceCount = (source.match(/router\.replace\(/g) || []).length;
    expect(replaceCount).toBeGreaterThanOrEqual(5);
  });
});
