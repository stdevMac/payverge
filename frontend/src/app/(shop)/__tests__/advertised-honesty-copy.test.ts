import fs from "node:fs";
import path from "node:path";

describe("unsupported advertised capabilities stay honestly scoped", () => {
  test("guest checkout does not advertise unimplemented wallets in any locale", () => {
    const guestMessages = path.join(process.cwd(), "src/i18n/guest-messages");
    for (const filename of fs.readdirSync(guestMessages)) {
      if (!filename.endsWith(".json")) continue;
      const copy = fs.readFileSync(path.join(guestMessages, filename), "utf8");
      expect(copy).not.toMatch(/Apple Pay|Google Pay/);
    }
  });
});
