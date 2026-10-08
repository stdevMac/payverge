import fs from "fs";
import path from "path";

import {
  presentGuestPaymentError,
  presentSplitHoldError,
} from "./guestPaymentErrors";

const guestMessagesDir = path.resolve(__dirname, "../i18n/guest-messages");

function guestMessage(locale: string, key: string): unknown {
  const messages = JSON.parse(
    fs.readFileSync(path.join(guestMessagesDir, `${locale}.json`), "utf8"),
  ) as Record<string, unknown>;
  return key
    .split(".")
    .reduce<unknown>(
      (node, part) =>
        node && typeof node === "object"
          ? (node as Record<string, unknown>)[part]
          : undefined,
      messages,
    );
}

describe("presentGuestPaymentError", () => {
  it("maps awaiting_confirmations to guest key", () => {
    const p = presentGuestPaymentError({
      code: "awaiting_confirmations",
      isNetwork: false,
    });
    expect(p.messageKey).toBe("payment.errors.awaitingConfirmations");
  });

  it("maps verification_unavailable to guest key", () => {
    const p = presentGuestPaymentError({
      code: "verification_unavailable",
      isNetwork: false,
    });
    expect(p.messageKey).toBe("payment.errors.verificationUnavailable");
    expect(p.preserveCart).toBe(true);
  });

  it("maps split_share_conflict to guest key", () => {
    const p = presentGuestPaymentError({
      code: "split_share_conflict",
      isNetwork: false,
    });
    expect(p.messageKey).toBe("payment.errors.splitShareConflict");
  });

  it("maps an admin-locked venue to the localized ordering-disabled message", () => {
    const p = presentGuestPaymentError({
      code: "business_unavailable",
      isNetwork: false,
    });
    expect(p.messageKey).toBe("menu.orderingDisabled");
  });

  it("network failure uses generic key", () => {
    const p = presentGuestPaymentError({ isNetwork: true });
    expect(p.messageKey).toBe("payment.errors.generic");
  });

  it("unknown code uses generic, never raw message", () => {
    const p = presentGuestPaymentError({
      code: "SOME_NEW_CODE",
      message: "Table split failed",
      isNetwork: false,
    });
    expect(p.messageKey).toBe("payment.errors.generic");
  });

  it("maps cashier request failure codes to specific keys", () => {
    expect(
      presentGuestPaymentError({
        code: "payment_request_failed",
        isNetwork: false,
      }).messageKey,
    ).toBe("payment.errors.cashierRequestFailed");
    expect(
      presentGuestPaymentError({
        code: "bill_not_payable",
        isNetwork: false,
      }).messageKey,
    ).toBe("payment.errors.billNotPayable");
    expect(
      presentGuestPaymentError({
        code: "idempotency_conflict",
        isNetwork: false,
      }).messageKey,
    ).toBe("payment.errors.idempotencyConflict");
    expect(
      presentGuestPaymentError({
        code: "CONFLICT",
        isNetwork: false,
      }).messageKey,
    ).toBe("payment.errors.billNotPayable");
    expect(
      presentGuestPaymentError({
        code: "payment_request_pending",
        isNetwork: false,
      }).messageKey,
    ).toBe("bill.cashierRequestSentNote");
  });
});

describe("presentGuestPaymentError USDC payer binding", () => {
  const cases: Record<string, string> = {
    amount_mismatch: "payment.errors.cryptoTransferNotForQuote",
    transfer_predates_quote: "payment.errors.cryptoTransferNotForQuote",
    crypto_tx_already_recorded: "payment.errors.cryptoTransferNotForQuote",
    crypto_quote_used: "payment.errors.cryptoQuoteUsed",
    crypto_quote_expired: "payment.errors.cryptoQuoteExpired",
    crypto_quote_limit: "payment.errors.generic",
  };

  it.each(Object.entries(cases))("maps %s to %s", (code, key) => {
    const p = presentGuestPaymentError({ code, isNetwork: false });
    expect(p.messageKey).toBe(key);
    expect(p.preserveCart).toBe(true);
  });

  it("ships every payer-binding message in all guest locales", () => {
    const locales = fs
      .readdirSync(guestMessagesDir)
      .filter((file) => file.endsWith(".json") && !file.startsWith("."))
      .map((file) => file.replace(/\.json$/, ""));
    expect(locales).toHaveLength(21);
    const keys = [
      ...new Set(Object.values(cases)),
      "paymentProcessor.exactUsdcAmount",
      "paymentProcessor.exactUsdcHint",
    ];
    for (const locale of locales) {
      for (const key of keys) {
        const value = guestMessage(locale, key);
        expect({
          locale,
          key,
          ok: typeof value === "string" && value.trim() !== "",
        }).toEqual({
          locale,
          key,
          ok: true,
        });
      }
    }
  });
});

describe("presentSplitHoldError", () => {
  it("maps the expired-hold sentinel to a specific message", () => {
    expect(
      presentSplitHoldError({
        code: "split_share_conflict",
        message: "split hold has expired",
        isNetwork: false,
      }).messageKey,
    ).toBe("payment.errors.splitHoldExpired");
  });

  it("maps the over-balance sentinel to a specific message", () => {
    expect(
      presentSplitHoldError({
        code: "split_share_conflict",
        message: "split amount exceeds available balance",
        isNetwork: false,
      }).messageKey,
    ).toBe("payment.errors.splitHoldOverBalance");
  });

  it("maps the item-taken sentinel to a specific message", () => {
    expect(
      presentSplitHoldError({
        code: "split_share_conflict",
        message: "split item fraction is unavailable",
        isNetwork: false,
      }).messageKey,
    ).toBe("payment.errors.splitHoldItemTaken");
  });

  it("keeps the generic conflict message for unknown conflict shapes", () => {
    expect(
      presentSplitHoldError({
        code: "split_share_conflict",
        message: "split share is already finalized",
        isNetwork: false,
      }).messageKey,
    ).toBe("payment.errors.splitShareConflict");
  });

  it("maps split_not_open and falls back generically", () => {
    expect(
      presentSplitHoldError({ code: "split_not_open", isNetwork: false })
        .messageKey,
    ).toBe("payment.errors.splitNotOpen");
    expect(presentSplitHoldError({ isNetwork: true }).messageKey).toBe(
      "bill.splitHoldFailed",
    );
    expect(
      presentSplitHoldError({ code: "weird", isNetwork: false }).messageKey,
    ).toBe("bill.splitHoldFailed");
  });
});
