import fs from "node:fs";
import path from "node:path";

describe("Next image deviceSizes (#666)", () => {
  it("does not request w=2048 fallbacks that fail to decode feature assets", () => {
    const src = fs.readFileSync(
      path.join(__dirname, "../../../next.config.mjs"),
      "utf8",
    );
    const match = src.match(/deviceSizes:\s*\[([^\]]+)\]/);
    expect(match).toBeTruthy();
    const sizes = match![1].split(",").map((part) => Number(part.trim()));
    expect(sizes).toContain(1200);
    expect(sizes).not.toContain(2048);
    expect(sizes).not.toContain(1920);
    expect(sizes).not.toContain(3840);
  });

  it("registers route alias redirects from the shared helper", () => {
    const src = fs.readFileSync(
      path.join(__dirname, "../../../next.config.mjs"),
      "utf8",
    );
    expect(src).toContain("routeAliasRedirects");
  });
});
