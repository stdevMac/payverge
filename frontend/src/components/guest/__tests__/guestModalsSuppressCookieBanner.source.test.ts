/**
 * Guest modals whose footer actions sit on the bottom edge must step the
 * cookie banner aside while open (see CartModal.cookieBanner and
 * OrderSuccessModal.cookieBanner for the rendered behavior). The banner is
 * fixed at z-[10000] on that edge: left visible it covers the primary action,
 * and a tap on it counts as a click outside that closes the modal.
 *
 * This wiring check keeps every guest modal on the hook so a refactor cannot
 * silently drop it.
 */
import fs from "fs";
import path from "path";

const SRC = path.resolve(__dirname, "..", "..", "..");

const WIRED: Array<[string, string]> = [
  ["app/t/[tableCode]/menu/_components/CartModal.tsx", "isOpen"],
  ["app/t/[tableCode]/menu/_components/OrderSuccessModal.tsx", "isOpen"],
  ["components/guest/GuestBill.tsx", "isCashierModalOpen"],
  ["components/guest/GuestMenuViews.tsx", "isItemModalOpen"],
  ["components/guest/AiWaiter.tsx", "isOpen"],
  ["components/splitting/GuestBillSplitPanel.tsx", "cashierModalOpen"],
  ["components/payment/PaymentProcessor.tsx", "isOpen"],
  ["components/payment/CrossChainPayment.tsx", "isOpen"],
  ["components/payment/PaymentStatusChecker.tsx", "isOpen"],
  ["components/customer/CustomerAuthModal.tsx", "isOpen"],
];

describe("guest modals suppress the cookie banner while open", () => {
  it.each(WIRED)("%s calls useSuppressCookieBanner(%s)", (file, openVar) => {
    const source = fs.readFileSync(path.join(SRC, file), "utf8");
    expect(source).toContain(
      'import { useSuppressCookieBanner } from "@/contexts/CookieConsentContext";',
    );
    expect(source).toContain(`useSuppressCookieBanner(${openVar});`);
  });
});
