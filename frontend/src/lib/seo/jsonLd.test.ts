import { serializeJsonLd } from "./jsonLd";

describe("serializeJsonLd", () => {
  it("escapes less-than characters so script tags cannot break out", () => {
    const payload = {
      name: '</script><script>window.__xss = true</script>',
    };

    const serialized = serializeJsonLd(payload);

    expect(serialized).not.toContain("</script>");
    expect(serialized).toContain("\\u003c/script>");
    expect(serialized).toContain("\\u003cscript>");
  });

  it("preserves valid JSON semantics", () => {
    const serialized = serializeJsonLd({ name: "Payverge", nested: { count: 1 } });

    expect(JSON.parse(serialized)).toEqual({ name: "Payverge", nested: { count: 1 } });
  });
});
