import { paymentMethodLabel, splitTenderLabel } from "./paymentMethodLabels";

const t = (key: string) => `[${key}]`;

describe("paymentMethodLabel", () => {
  it.each([
    ["stripe", "[paymentMethods.card]"],
    ["paypal", "PayPal"],
    ["mercadopago", "MercadoPago"],
    ["usdc_payment", "[paymentMethods.cryptoUsdc]"],
    ["crypto", "[paymentMethods.cryptoUsdc]"],
    ["cross_chain_payment", "[paymentMethods.crossChain]"],
    ["cross-chain", "[paymentMethods.crossChain]"],
    ["some_new_plugin", "Some New Plugin"],
    ["", ""],
    [undefined, ""],
  ])("%s → %s", (method, expected) => {
    expect(paymentMethodLabel(method as string | undefined, t)).toBe(expected);
  });
});

describe("splitTenderLabel", () => {
  it.each([
    ["cash", "[paymentMethods.cash]"],
    ["crypto", "[paymentMethods.cryptoUsdc]"],
    ["cross-chain", "[paymentMethods.crossChain]"],
    ["plugin", "[paymentMethods.card]"],
    ["card", "[paymentMethods.card]"],
    ["", ""],
  ])("%s → %s", (tender, expected) => {
    expect(splitTenderLabel(tender, t)).toBe(expected);
  });
});
