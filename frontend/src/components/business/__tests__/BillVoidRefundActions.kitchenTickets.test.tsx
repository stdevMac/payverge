/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import enBill from "@/i18n/messages/en/billManager.json";
import esBill from "@/i18n/messages/es/billManager.json";
import esArBill from "@/i18n/messages/es-ar/billManager.json";

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { success: jest.fn(), error: jest.fn() },
}));

const mockVoidBill = jest.fn();
jest.mock("@/api/bills", () => ({
  getBillAudit: jest.fn().mockResolvedValue({ entries: [] }),
  voidBill: (...args: unknown[]) => mockVoidBill(...args),
  refundBillPayment: jest.fn(),
}));

jest.mock("@/api/currency", () => ({
  formatCurrency: (amount: number) => `$${(amount || 0).toFixed(2)}`,
}));

jest.mock("@/components/business/managerPin/ManagerPinProvider", () => ({
  useWithManagerPin: () => ({
    withManagerPin: (fn: (pin?: string) => Promise<unknown>) => fn(undefined),
  }),
}));

// es-AR is the voseo OVERRIDE tier: it deep-merges over es. Resolve against the
// real merged bundle so a missing key fails here, not on a busy pass.
function deepMerge(
  base: Record<string, unknown>,
  override: Record<string, unknown>,
): Record<string, unknown> {
  const out: Record<string, unknown> = { ...base };
  for (const [key, value] of Object.entries(override)) {
    const prev = out[key];
    out[key] =
      value && typeof value === "object" && !Array.isArray(value) &&
      prev && typeof prev === "object" && !Array.isArray(prev)
        ? deepMerge(
            prev as Record<string, unknown>,
            value as Record<string, unknown>,
          )
        : value;
  }
  return out;
}

const esArEffective = deepMerge(
  esBill as Record<string, unknown>,
  esArBill as Record<string, unknown>,
);

function resolve(tree: Record<string, unknown>, dotted: string): string {
  const value = dotted
    .split(".")
    .reduce<unknown>(
      (acc, part) =>
        acc && typeof acc === "object"
          ? (acc as Record<string, unknown>)[part]
          : undefined,
      tree,
    );
  return typeof value === "string" ? value : dotted;
}

// The factory is hoisted above the imports, so it rebuilds the merged tree
// with require() rather than closing over module-scope consts.
jest.mock("@/i18n/SimpleTranslationProvider", () => {
  const merge = (
    base: Record<string, unknown>,
    override: Record<string, unknown>,
  ): Record<string, unknown> => {
    const out: Record<string, unknown> = { ...base };
    for (const [key, value] of Object.entries(override)) {
      const prev = out[key];
      out[key] =
        value && typeof value === "object" && !Array.isArray(value) &&
        prev && typeof prev === "object" && !Array.isArray(prev)
          ? merge(
              prev as Record<string, unknown>,
              value as Record<string, unknown>,
            )
          : value;
    }
    return out;
  };
  const tree = merge(
    require("@/i18n/messages/es/billManager.json"),
    require("@/i18n/messages/es-ar/billManager.json"),
  );
  return {
    useSimpleLocale: () => ({ locale: "es-AR" }),
    getTranslation: (key: string) => {
      const value = key
        .replace(/^billManager\./, "")
        .split(".")
        .reduce<unknown>(
          (acc, part) =>
            acc && typeof acc === "object"
              ? (acc as Record<string, unknown>)[part]
              : undefined,
          tree,
        );
      return typeof value === "string" ? value : key;
    },
  };
});

import { BillVoidRefundActions } from "../BillVoidRefundActions";

const t = (key: string) => resolve(esArEffective, key);

function makeOpenBill() {
  return {
    bill: {
      id: 99,
      business_id: 7,
      bill_number: "B-1132",
      status: "open",
      paid_amount: 0,
      payments: [],
    },
  } as unknown as Parameters<typeof BillVoidRefundActions>[0]["bill"];
}

async function openVoidModalAndSubmit() {
  const label = t("voidRefund.actions.voidBill");
  fireEvent.click((await screen.findAllByText(label))[0]);

  const textarea = await screen.findByRole("textbox");
  fireEvent.change(textarea, {
    target: { value: "el host quiso liberar la mesa" },
  });

  await waitFor(() => expect(screen.getAllByText(label).length).toBe(2));
  fireEvent.click(screen.getAllByText(label)[1]);
}

// #704 twin door: the backend now refuses a void while expo still owes this
// check food (bill_void_kitchen_tickets_live). Without a code branch,
// getSafeApiErrorMessage surfaces the handler's raw English at a Spanish
// manager — the same leak b4f3c2f36 closed on the Liberar door.
it("localizes the bill_void_kitchen_tickets_live 409 instead of leaking backend English", async () => {
  const toast = (await import("react-hot-toast")).default;
  const backendEnglish =
    "The kitchen is still working this check — bump or cancel the open tickets before voiding";
  mockVoidBill.mockReset().mockRejectedValue({
    isAxiosError: true,
    response: {
      status: 409,
      data: { error: backendEnglish, code: "bill_void_kitchen_tickets_live" },
    },
  });

  render(
    <BillVoidRefundActions
      bill={makeOpenBill()}
      onRefresh={() => {}}
      onCloseParent={() => {}}
      currency="USD"
    />,
  );

  await openVoidModalAndSubmit();

  await waitFor(() => expect(mockVoidBill).toHaveBeenCalled());
  await waitFor(() =>
    expect(toast.error).toHaveBeenCalledWith(
      t("voidRefund.errors.kitchenTicketsLive"),
    ),
  );
  // The raw handler string must never reach the manager.
  expect(toast.error).not.toHaveBeenCalledWith(backendEnglish);
  // And the key must actually resolve — an unresolved path echoes itself.
  expect(t("voidRefund.errors.kitchenTicketsLive")).not.toContain("voidRefund.");
});

it("keeps the generic void failure copy for uncoded errors", async () => {
  const toast = (await import("react-hot-toast")).default;
  (toast.error as jest.Mock).mockClear();
  mockVoidBill.mockReset().mockRejectedValue({
    isAxiosError: true,
    response: { status: 500, data: {} },
  });

  render(
    <BillVoidRefundActions
      bill={makeOpenBill()}
      onRefresh={() => {}}
      onCloseParent={() => {}}
      currency="USD"
    />,
  );

  await openVoidModalAndSubmit();

  await waitFor(() =>
    expect(toast.error).toHaveBeenCalledWith(t("voidRefund.errors.voidFailed")),
  );
});

it("ships kitchenTicketsLive in every operator locale, voseo on es-AR", () => {
  const read = (tree: Record<string, unknown>) =>
    resolve(tree, "voidRefund.errors.kitchenTicketsLive");
  const en = read(enBill as Record<string, unknown>);
  const es = read(esBill as Record<string, unknown>);
  const esAr = read(esArEffective);

  [en, es, esAr].forEach((value) => {
    expect(typeof value).toBe("string");
    expect(value.length).toBeGreaterThan(0);
    expect(value).not.toContain("voidRefund.");
  });
  expect(es).not.toBe(en);
  // es-AR is the voseo override tier: tuteo imperatives must not survive.
  expect(esAr).not.toBe(es);
  expect(esAr).toMatch(/cerrá|cancelá/);
});
