import fs from "node:fs";
import path from "node:path";

// #525: Next also emits HSTS for non-Caddy paths. Keep max-age +
// includeSubDomains, but do not advertise preload until the apex is
// accepted on hstspreload.org.
describe("HSTS does not advertise preload", () => {
  it("keeps max-age and includeSubDomains without preload in next.config", () => {
    const source = fs.readFileSync(
      path.join(process.cwd(), "next.config.mjs"),
      "utf8",
    );
    const match = source.match(
      /key:\s*"Strict-Transport-Security"[\s\S]*?value:\s*"([^"]+)"/,
    );
    expect(match).not.toBeNull();
    const value = match?.[1] ?? "";
    expect(value).toContain("max-age=31536000");
    expect(value).toContain("includeSubDomains");
    expect(value.toLowerCase()).not.toMatch(/\bpreload\b/);
  });
});
