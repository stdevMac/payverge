import fs from "fs";
import path from "path";
import { shouldShowAiWaiter } from "@/app/t/[tableCode]/menu/aiGate";

describe("GuestTableView AI Waiter gate", () => {
  const source = fs.readFileSync(
    path.resolve(__dirname, "GuestTableView.tsx"),
    "utf8",
  );

  it("mounts AiWaiter only via shouldShowAiWaiter (not ConciergeWidget)", () => {
    expect(source).toContain("shouldShowAiWaiter");
    expect(source).toContain("<AiWaiter");
    expect(source).not.toContain("ConciergeWidget");
  });

  it("hands stable cart metadata off without storing the bearer or claiming success", () => {
    expect(source).toContain("stagePendingCartNavigation");
    expect(source).toContain('return "deferred"');
    expect(source).toContain("menuItemId: metadata?.menuItemId");
    expect(source).toContain("bundleId: metadata?.bundleId");
    expect(source).not.toContain("sessionToken:");
    expect(source).not.toMatch(/added(?:Toast|Chat)/);
  });

  it("shows for AI Pro and hides for core", () => {
    expect(shouldShowAiWaiter({ id: 51, ai_available: true })).toBe(true);
    expect(shouldShowAiWaiter({ id: 50, ai_available: false })).toBe(false);
  });
});
