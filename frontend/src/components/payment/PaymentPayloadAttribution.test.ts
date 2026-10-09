import fs from "fs";
import path from "path";

const PAYMENT_COMPONENTS = [
  "PaymentProcessor.tsx",
  "CrossChainPayment.tsx",
];

describe("guest payment customer attribution", () => {
  it.each(PAYMENT_COMPONENTS)(
    "%s does not send customer identifiers from the frontend",
    (fileName) => {
      const source = fs.readFileSync(path.resolve(__dirname, fileName), "utf8");

      expect(source).not.toContain("customer_id");
      expect(source).not.toContain("crmAPI");
      expect(source).not.toContain("getProfile");
    },
  );
});
