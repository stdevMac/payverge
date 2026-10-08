/** @jest-environment jsdom */
import fs from "fs";
import path from "path";

describe("Kitchen special-request contrast", () => {
  const source = fs.readFileSync(
    path.join(__dirname, "Kitchen.tsx"),
    "utf8",
  );

  it("paints the special_requests line in text-amber-800", () => {
    const match = source.match(
      /item\.special_requests && \(\s*<div className="([^"]*)"/,
    );
    expect(match?.[1]).toEqual(expect.stringContaining("text-amber-800"));
    expect(source).not.toContain("italic text-amber-600");
  });
});
