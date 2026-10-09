import { PROVIDER_CATALOG_BY_KEY } from "@/constants/providerCatalog";

export interface ExternalPartnerLink {
  name: string;
  url: string;
  provider_key?: string;
  icon_url?: string;
}

function resolvePartnerIcon(link: ExternalPartnerLink): string {
  if (link.icon_url) return link.icon_url;
  if (link.provider_key) {
    return PROVIDER_CATALOG_BY_KEY[link.provider_key]?.iconPath || "";
  }
  return "";
}

export function withResolvedIcon(link: ExternalPartnerLink): ExternalPartnerLink {
  const icon_url = resolvePartnerIcon(link);
  return icon_url ? { ...link, icon_url } : link;
}
