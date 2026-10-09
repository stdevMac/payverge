import fs from "fs";
import path from "path";

describe("guest bill table label", () => {
  const billSource = fs.readFileSync(
    path.resolve(__dirname, "page.tsx"),
    "utf8",
  );
  const menuSource = fs.readFileSync(
    path.resolve(__dirname, "../menu/page.tsx"),
    "utf8",
  );

  it("uses formatEntityName so Table names are not double-prefixed", () => {
    expect(billSource).toContain('from "@/lib/tableLabel"');
    expect(billSource).toMatch(
      /formatEntityName\(\s*t\("bill\.tableLabel"\),\s*tableData\.table\.name/,
    );
    // No bare "label · name" concatenation that doubles "Table Table N"
    expect(billSource).not.toMatch(
      /t\("bill\.tableLabel"\)\} \{tableData\.table\.name\}/,
    );
    expect(billSource).not.toMatch(
      /t\("bill\.tableLabel"\)\} \{tableData\.table\.name/,
    );
  });

  it("formats guest order notes with formatEntityName", () => {
    expect(menuSource).toMatch(
      /formatEntityName\(t\("menu\.tableLabel"\)[^)]*tableData\?\.table\.name/,
    );
    expect(menuSource).not.toContain(
      "`Table ${tableData?.table.name || \"Unknown\"}",
    );
  });
});
