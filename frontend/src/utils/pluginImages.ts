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

export function resolvePluginImageSrc(plugin: PluginImageSource): string | null {
  const image = plugin.image?.trim();
  if (image && !image.includes("dummyimage.com")) {
    return image;
  }

  const pluginName = plugin.name?.trim();
  if (isKnownPluginName(pluginName)) {
    return PLUGIN_IMAGE_BY_NAME[pluginName];
  }

  return image || null;
}
