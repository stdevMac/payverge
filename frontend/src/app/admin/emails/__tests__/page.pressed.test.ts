import fs from "fs";
import path from "path";

const SOURCE = fs.readFileSync(
  path.join(__dirname, "../page.tsx"),
  "utf8",
);

describe("admin emails type buttons (#452)", () => {
  it("exposes aria-pressed for the selected email type", () => {
    expect(SOURCE).toMatch(/aria-pressed=\{emailType === "operational"\}/);
    expect(SOURCE).toMatch(/aria-pressed=\{emailType === "platform"\}/);
    expect(SOURCE).toMatch(/role="group"/);
    expect(SOURCE).toMatch(/aria-label="Email type"/);
  });
});
