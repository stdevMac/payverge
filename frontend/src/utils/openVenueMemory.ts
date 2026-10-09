const STORAGE_KEY = "payverge:last-open-venue";

type RememberedVenue = {
  key: string;
  name: string;
};

export function rememberOpenVenue(business: {
  business_id?: string;
  id: number;
  name?: string;
}): void {
  if (typeof sessionStorage === "undefined") return;
  const name = (business.name || "").trim();
  if (!name) return;
  const remembered: RememberedVenue = {
    key: business.business_id || String(business.id),
    name,
  };
  try {
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(remembered));
  } catch {
    // Private mode / quota — the 403 card can still fall back to the slug.
  }
}

export function recalledVenueName(identifier: string): string | null {
  if (typeof sessionStorage === "undefined" || !identifier) return null;
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as RememberedVenue;
    if (!parsed?.name || parsed.key !== identifier) return null;
    return parsed.name;
  } catch {
    return null;
  }
}
