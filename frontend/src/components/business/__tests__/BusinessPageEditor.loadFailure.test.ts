/** @jest-environment node */
import fs from "node:fs";
import path from "node:path";

const SOURCE = fs.readFileSync(
  path.resolve(__dirname, "../BusinessPageEditor.tsx"),
  "utf-8",
);

describe("BusinessPageEditor — load failure disarms Save (P3)", () => {
  test("tracks a loadFailed state", () => {
    expect(SOURCE).toContain("const [loadFailed, setLoadFailed] = useState(false)");
  });

  test("the load catch flags the failure (not just a toast)", () => {
    const catchBlock = SOURCE.match(
      /catch \(error\) \{[\s\S]{0,400}?loadPageError[\s\S]{0,200}?\}/,
    );
    expect(catchBlock?.[0]).toContain("setLoadFailed(true)");
  });

  test("handleSave refuses to run after a failed load", () => {
    const save = SOURCE.match(/const handleSave = async \(\) => \{[\s\S]{0,600}/);
    expect(save?.[0]).toContain("if (loadFailed)");
  });

  test("the SaveBar dirty flag is gated on a successful load", () => {
    expect(SOURCE).toContain("dirty={isDirty && !loadFailed}");
  });

  test("a retry affordance re-runs loadData", () => {
    expect(SOURCE).toContain('tStringSettings("businessPage.retryLoad")');
  });
});
