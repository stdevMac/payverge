import fs from "fs";
import path from "path";

const guestDir = path.join(__dirname, "..", "guest-messages");
const slimDir = path.join(__dirname, "..", "cookie-consent");
const REGEN =
  "The slim files are copies of guest-messages cookies and must be regenerated.";

function localeJsonFiles(dir: string): string[] {
  return fs
    .readdirSync(dir)
    .filter((name) => name.endsWith(".json") && !name.startsWith("."))
    .sort();
}

describe("cookie-consent slim catalogs", () => {
  it("matches every guest-messages cookies subtree", () => {
    const guestFiles = localeJsonFiles(guestDir);
    const slimFiles = localeJsonFiles(slimDir);
    expect({ note: REGEN, slimFiles }).toEqual({
      note: REGEN,
      slimFiles: guestFiles,
    });

    for (const file of guestFiles) {
      const guest = JSON.parse(
        fs.readFileSync(path.join(guestDir, file), "utf8"),
      );
      const slim = JSON.parse(fs.readFileSync(path.join(slimDir, file), "utf8"));
      expect({ note: REGEN, file, slim }).toEqual({
        note: REGEN,
        file,
        slim: guest.cookies,
      });
    }
  });

  it("does not import from ./guest-messages/", () => {
    const source = fs.readFileSync(
      path.join(__dirname, "..", "cookieConsentCopy.ts"),
      "utf8",
    );
    expect(source).not.toMatch(/from\s+["']\.\/guest-messages\//);
    expect(source).not.toMatch(/import\(\s*["']\.\/guest-messages\//);
  });
});
