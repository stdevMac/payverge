import fs from "fs";
import path from "path";

const roots = [
  "src/components/business-page/GuestReservationForm.tsx",
  "src/components/delivery/GuestDeliveryCheckout.tsx",
  "src/components/delivery/GuestDeliveryOrder.tsx",
  "src/components/guest/PaymentSection.tsx",
];

describe("guest money/booking forms use AccessibleInput (#518)", () => {
  it.each(roots)("%s does not use raw NextUI Input/Textarea", (rel) => {
    const source = fs.readFileSync(path.join(process.cwd(), rel), "utf8");
    expect(source).toContain("AccessibleInput");
    expect(source).not.toMatch(/from "@nextui-org\/react".*\bInput\b/);
    expect(source).not.toMatch(/<Input[\s>]/);
    expect(source).not.toMatch(/<Textarea[\s>]/);
  });
});
