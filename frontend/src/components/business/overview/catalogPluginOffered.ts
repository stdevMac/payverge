import type { Plugin } from "@/api/plugins";

// A platform catalog plugin may be offered to the operator (quick actions,
// "ready to accept payments" claims) only when its row exists, is active and
// is not marked coming-soon. The backend seeds cross_chain_payment as
// coming-soon while guests cannot settle through that rail
// (services.CrossChainGuestSettlementAvailable), so an old business
// enablement of it must not count as a working payment rail either.
export function isCatalogPluginOffered(
  plugin?: Pick<Plugin, "is_active" | "coming_soon"> | null,
): boolean {
  return Boolean(plugin && plugin.is_active !== false && !plugin.coming_soon);
}
