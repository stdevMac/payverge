import { PLUGIN, isKnownPluginName, type PluginName } from "@/constants/plugins";

const PLUGIN_IMAGE_BY_NAME: Record<PluginName, string> = {
  [PLUGIN.usdcPayment]: "/images/plugins/usdc.png",
  [PLUGIN.crossChainPayment]: "/images/plugins/cross-chain.png",
  [PLUGIN.stripe]: "/images/plugins/stripe-logo.png",
  [PLUGIN.paypal]: "/images/plugins/paypal-logo.png",
  [PLUGIN.mercadopago]: "/images/plugins/mercadopago-logo.png",
  [PLUGIN.telegram]: "/images/plugins/telegram-logo.png",
  [PLUGIN.trustpilot]: "/images/plugins/trustpilot-logo.png",
  [PLUGIN.dailyEmailReport]: "/images/plugins/daily-report.png",
  [PLUGIN.weeklyEmailReport]: "/images/plugins/weekly-report.png",
};

interface PluginImageSource {
  name?: string;
  image?: string | null;
}

// Seeded catalog rows used dummyimage.com placeholders; treat those as missing
// so the bundled logo shows instead. Match the parsed host, not a substring,
// so a path or query that merely mentions the domain is left alone.
function isPlaceholderImage(image: string): boolean {
  let host: string;
  try {
    host = new URL(image, "http://localhost").hostname.toLowerCase();
  } catch {
    return false;
  }
  return host === "dummyimage.com" || host.endsWith(".dummyimage.com");
}

export function resolvePluginImageSrc(plugin: PluginImageSource): string | null {
  const image = plugin.image?.trim();
  if (image && !isPlaceholderImage(image)) {
    return image;
  }

  const pluginName = plugin.name?.trim();
  if (isKnownPluginName(pluginName)) {
    return PLUGIN_IMAGE_BY_NAME[pluginName];
  }

  return image || null;
}
