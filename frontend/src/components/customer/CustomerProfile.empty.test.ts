import fs from "fs";
import path from "path";

describe("CustomerProfile unsigned empty state", () => {
  const source = fs.readFileSync(
    path.resolve(__dirname, "CustomerProfile.tsx"),
    "utf8",
  );
  const en = JSON.parse(
    fs.readFileSync(
      path.resolve(__dirname, "../../i18n/guest-messages/en.json"),
      "utf8",
    ),
  );

  it("uses an h1 for the unsigned heading and honest not-signed-in copy", () => {
    expect(source).toContain('data-testid="customer-profile-unsigned"');
    expect(source).toMatch(/<h1[\s\S]*?translate\("notFound"\)/);
    expect(en.customerProfile.notFound).toMatch(/not signed in/i);
    expect(en.customerProfile.signInPrompt).toBeTruthy();
  });
});
