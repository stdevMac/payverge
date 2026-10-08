import {
  PAYMENT_PLUGIN_ORDER,
  isCardPaymentPlugin,
  isCryptoPaymentPlugin,
  sortPluginsForOperatorMarketplace,
} from "@/utils/pluginMarketplaceOrder";
import { PLUGIN } from "@/constants/plugins";

describe("sortPluginsForOperatorMarketplace", () => {
  it("puts Mercado Pago before Stripe/PayPal and crypto last", () => {
    const sorted = sortPluginsForOperatorMarketplace([
      {
        name: PLUGIN.crossChainPayment,
        category: "payment",
        display_name: "Any Token Payment",
      },
      {
        name: PLUGIN.usdcPayment,
        category: "payment",
        display_name: "USDC Payment",
      },
      {
        name: PLUGIN.stripe,
        category: "payment",
        display_name: "Stripe",
      },
      {
        name: PLUGIN.mercadopago,
        category: "payment",
        display_name: "Mercado Pago",
      },
      {
        name: PLUGIN.paypal,
        category: "payment",
        display_name: "PayPal",
      },
    ]);

    expect(sorted.map((p) => p.name)).toEqual([...PAYMENT_PLUGIN_ORDER]);
  });

  it("classifies card processors separately from crypto rails", () => {
    expect(isCardPaymentPlugin(PLUGIN.mercadopago)).toBe(true);
    expect(isCardPaymentPlugin(PLUGIN.stripe)).toBe(true);
    expect(isCardPaymentPlugin(PLUGIN.paypal)).toBe(true);
    expect(isCardPaymentPlugin(PLUGIN.usdcPayment)).toBe(false);
    expect(isCryptoPaymentPlugin(PLUGIN.usdcPayment)).toBe(true);
    expect(isCryptoPaymentPlugin(PLUGIN.mercadopago)).toBe(false);
  });

  it("keeps Coming Soon plugins after live ones in the same category", () => {
    const sorted = sortPluginsForOperatorMarketplace([
      {
        name: "quickbooks",
        category: "integration",
        display_name: "QuickBooks",
        coming_soon: true,
      },
      {
        name: PLUGIN.telegram,
        category: "integration",
        display_name: "Telegram",
        coming_soon: false,
      },
    ]);

    expect(sorted.map((p) => p.name)).toEqual([
      PLUGIN.telegram,
      "quickbooks",
    ]);
  });
});
