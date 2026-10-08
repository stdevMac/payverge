import fs from "fs";
import path from "path";

/**
 * L1-12 secondary source sweep across toggle modals.
 * Primary D1 DOM proof: KitchenOrdersToggle.l1-12-opaque.test.tsx.
 */
const FILES = [
  "src/components/business/ReservationToggle.tsx",
  "src/components/business/KitchenOrdersToggle.tsx",
  "src/components/business/CRMToggle.tsx",
  "src/components/business/DeliveryToggle.tsx",
  "src/components/business/AiWaiter/AiWaiterToggle.tsx",
  "src/components/business/fiscal/FiscalDashboard.tsx",
  "src/components/business/MenuBuilder/AIMenuOnboarding/MenuReviewEditor.tsx",
];

describe("L1-12 — opaque ModalContent surfaces (secondary source sweep)", () => {
  for (const rel of FILES) {
    it(`${rel} has no ModalContent bg-white/9x translucency`, () => {
      const src = fs.readFileSync(
        path.join(__dirname, "..", "..", rel),
        "utf8",
      );
      expect(src).not.toMatch(/ModalContent className="[^"]*bg-white\/9[05]/);
      if (rel.includes("MenuReviewEditor")) {
        expect(src).not.toMatch(
          /base: "mb-4 border border-warm-200\/90 bg-white\/85/,
        );
      }
    });
  }
});
