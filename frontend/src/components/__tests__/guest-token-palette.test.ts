import fs from "fs";
import path from "path";

// Guards the Wave-5 warm/ink token migration: these guest surfaces must not
// regress back to Tailwind's gray-* alias or the NextUI text-primary token.
const FILES = [
  "src/components/delivery/GuestDeliveryCheckout.tsx",
  "src/components/delivery/GuestDeliveryOrder.tsx",
  "src/components/customer/CustomerProfile.tsx",
];

describe("guest token palette", () => {
  it.each(FILES)("%s uses warm/ink/brand tokens, not gray-*/text-primary", (rel) => {
    const source = fs.readFileSync(path.join(process.cwd(), rel), "utf8");
    expect(source).not.toMatch(/\b(?:text|bg|border|divide)-gray-\d/);
    expect(source).not.toMatch(/\btext-primary\b/);
  });
});
