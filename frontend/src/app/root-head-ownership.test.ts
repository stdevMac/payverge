import { readFileSync } from "node:fs";
import { join } from "node:path";

describe("root metadata head ownership", () => {
  const source = readFileSync(join(__dirname, "layout.tsx"), "utf8");

  it("leaves <head> exclusively to the Next Metadata API", () => {
    expect(source).not.toContain("<head>");
    expect(source).not.toContain("</head>");
  });

  it("keeps all five inline scripts nonce-protected in the document body", () => {
    const body = source.slice(source.indexOf("<body"));

    expect(body.match(/nonce=\{nonce\}/g)).toHaveLength(5);
    expect(body).toContain(
      "dangerouslySetInnerHTML={{ __html: instanceScript }}",
    );
    expect(body).toContain('type="application/ld+json"');
    expect(body).toContain("dangerouslySetInnerHTML={{ __html: buildInfo }}");
  });

  it("emits the runtime public config as the first body script", () => {
    const body = source.slice(source.indexOf("<body"));
    const firstScript = body.slice(body.indexOf("<script"));

    expect(firstScript.slice(0, firstScript.indexOf("/>"))).toContain(
      "dangerouslySetInnerHTML={{ __html: publicEnvScript }}",
    );
  });
});
