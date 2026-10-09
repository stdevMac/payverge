import fs from "node:fs";
import os from "node:os";
import path from "node:path";

import { findHardcodedStrings } from "../check-hardcoded-strings";

describe("guest hardcoded-string coverage", () => {
  test("scans lowercase guest JSX copy in route and account directories", () => {
    const root = fs.mkdtempSync(path.join(os.tmpdir(), "guest-hardcoded-"));
    const files = [
      "frontend/src/app/(shop)/customer/login/page.tsx",
      "frontend/src/components/guest/cart/Empty.tsx",
    ];
    for (const relative of files) {
      const file = path.join(root, relative);
      fs.mkdirSync(path.dirname(file), { recursive: true });
      fs.writeFileSync(file, "export default () => <p>try again later</p>;\n");
    }

    const findings = findHardcodedStrings(root);
    expect(findings.map((finding) => finding.text)).toEqual([
      "try again later",
      "try again later",
    ]);
  });
});
