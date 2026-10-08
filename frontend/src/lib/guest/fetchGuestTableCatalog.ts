import { getServerApiUrl } from "@/lib/serverApiUrl";
import { fetchWithTransientRetry } from "@/lib/guest/fetchWithTransientRetry";

export type GuestTableCatalog = {
  businessName: string | null;
  dishes: string[];
};

function collectDishNames(payload: unknown): string[] {
  if (!payload || typeof payload !== "object") return [];
  const data = payload as {
    categories?: Array<{
      name?: string;
      items?: Array<{ name?: string }>;
    }>;
    bundles?: Array<{ name?: string }>;
    business?: { name?: string };
  };
  const names: string[] = [];
  for (const category of data.categories ?? []) {
    if (typeof category.name === "string" && category.name.trim()) {
      names.push(category.name.trim());
    }
    for (const item of category.items ?? []) {
      if (typeof item.name === "string" && item.name.trim()) {
        names.push(item.name.trim());
      }
    }
  }
  for (const bundle of data.bundles ?? []) {
    if (typeof bundle.name === "string" && bundle.name.trim()) {
      names.push(bundle.name.trim());
    }
  }
  return names;
}

/** SSR catalog for the first HTML paint of /t/:code/menu (no-JS + crawlers). */
export async function fetchGuestTableCatalog(
  tableCode: string,
): Promise<GuestTableCatalog> {
  const apiUrl = getServerApiUrl();
  if (!apiUrl) return { businessName: null, dishes: [] };
  try {
    const res = await fetchWithTransientRetry(
      `${apiUrl}/guest/table/${encodeURIComponent(tableCode)}`,
      { cache: "no-store" },
    );
    if (!res.ok) return { businessName: null, dishes: [] };
    const data: unknown = await res.json();
    const businessName =
      data &&
      typeof data === "object" &&
      data !== null &&
      "business" in data &&
      typeof (data as { business?: { name?: string } }).business?.name ===
        "string"
        ? (data as { business: { name: string } }).business.name
        : null;
    return { businessName, dishes: collectDishNames(data) };
  } catch {
    return { businessName: null, dishes: [] };
  }
}
