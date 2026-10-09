import fs from "fs";
import path from "path";
import { shouldShowAiWaiter } from "@/app/t/[tableCode]/menu/aiGate";

describe("storefront AiWaiter entitlement gate (#944)", () => {
  const source = fs.readFileSync(
    path.resolve(__dirname, "ConvertingBusinessLandingPage.tsx"),
    "utf8",
  );

  it("mounts AiWaiter only via shouldShowAiWaiter, not leftover ai_settings toggles", () => {
    expect(source).toContain("shouldShowAiWaiter");
    expect(source).toMatch(/shouldShowAiWaiter\(business\)/);
    expect(source).not.toMatch(
      /business\.ai_settings\?\.ai_enabled\s*!==\s*false/,
    );
  });

  it("hides Mozo for a Core venue even when page-AI toggles were left on", () => {
    expect(
      shouldShowAiWaiter({
        id: 141,
        ai_available: false,
        ai_settings: {
          ai_enabled: true,
          ai_name: "Mozo",
          ai_priority: "balanced",
          business_page_ai_enabled: true,
        },
      } as Parameters<typeof shouldShowAiWaiter>[0]),
    ).toBe(false);
  });
});
