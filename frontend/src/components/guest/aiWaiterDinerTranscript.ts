import { isTrustedEntityImageUrl } from "./aiImageOrigins";
import { isMenuItemOrderable, isVenueClosedState } from "./menuItemAvailability";
import type { AssistantEntity, AssistantResponse } from "@/types/assistant";

/**
 * Item-level 86s. Guest `ResolveOrderability` remaps inventory_out →
 * `business_closed`, and the live catalog row has no `orderability_state`.
 * Deny a card only from a signal that survives that overwrite: catalog
 * `inventory_status` (stamped out_of_stock, hours do not wipe it), catalog
 * `orderability_state` when a projector left one, or the map when it still
 * says inventory_out / manual_disabled (#723).
 */
const itemUnavailableStates = new Set([
  "manual_disabled",
  "inventory_out",
  "out_of_stock",
]);

const venueBrowseableStates = new Set([
  "business_closed",
  "ordering_disabled",
]);

/** Most dish cards a single turn may show before the transcript gets noisy. */
const MAX_MEDIA_ENTITIES = 3;

type DinerOrderabilityEntry = {
  state?: string | null;
  orderable?: boolean;
};

export type DinerOrderabilityMap = Record<
  string,
  DinerOrderabilityEntry | undefined
>;

export type DinerTranscript = {
  response: AssistantResponse;
  media: Map<AssistantEntity, string[]>;
};

/**
 * Reshape one assistant turn for a diner, who came for a waiter's answer and
 * not for the assistant protocol. Dropped here:
 *  - executable actions (the guest surface never runs them),
 *  - retrieval sources (they surface as a "3 sources used" debug disclosure),
 *  - every suggested dish we cannot show a photo for,
 *  - the grounded-list bullets that would duplicate cards or leak an 86
 *    (`- **Harvest Bowl** — available`, `- **Steak** — unavailable`).
 *
 * Restore (#724) runs the same reshape, so a refresh cannot resurrect
 * Complete / Availability / source-count chrome from a raw stored envelope.
 */
export function forDinerTranscript(
  response: AssistantResponse,
  menuData: readonly unknown[],
  orderability: DinerOrderabilityMap = {},
): DinerTranscript {
  const media = new Map<AssistantEntity, string[]>();
  for (const entity of response.entities) {
    if (media.size === MAX_MEDIA_ENTITIES) break;
    const images = dinerEntityImages(entity, menuData, orderability);
    if (images.length > 0) media.set(entity, images);
  }
  const shownIDs = new Set(Array.from(media.keys(), (entity) => entity.id));
  // Every named entity is a decision — card, 86 omit, or unmatched — so its
  // protocol bullet must not survive into the diner transcript.
  const decidedNames = new Set(
    response.entities.map((entity) => entity.display_name),
  );
  return {
    response: dinerProtocolChrome({
      ...response,
      answer: {
        ...response.answer,
        content: stripDuplicatedDishList(response.answer.content, decidedNames),
      },
      entities: response.entities.filter((entity) => media.has(entity)),
      sections: response.sections.map((section) => ({
        ...section,
        answer: stripDuplicatedDishList(section.answer, decidedNames),
        entity_ids: section.entity_ids.filter((id) => shownIDs.has(id)),
      })),
    }),
    media,
  };
}

/**
 * Protocol chrome that must never reach a diner, including on the first
 * paint of a restored turn before menu photos resolve. Sources and actions
 * are dropped; availability bullets for known entities are stripped. Status
 * stays on the envelope — `hideProtocolChrome` owns the Complete chip.
 */
export function dinerProtocolChrome(
  response: AssistantResponse,
): AssistantResponse {
  const names = new Set(response.entities.map((entity) => entity.display_name));
  return {
    ...response,
    answer: {
      ...response.answer,
      content: stripDuplicatedDishList(response.answer.content, names),
    },
    actions: [],
    sources: [],
    sections: response.sections.map((section) => ({
      ...section,
      answer: stripDuplicatedDishList(section.answer, names),
      action_ids: [],
      source_ids: [],
    })),
  };
}

/**
 * Drop the server-owned grounded list once cards/omits are decided. The
 * finalizer writes `- **Name** — available` / `— unavailable` (every guest
 * locale uses an em-dash + availability word). An 86 omit must lose its
 * bullet the same way a painted card loses its "available" twin (#723).
 *
 * Lines that do not name a decided dish stay put, so a waiter sentence is
 * never eaten just because it mentions a dish.
 */
export function stripDuplicatedDishList(
  content: string,
  decidedNames: ReadonlySet<string>,
): string {
  if (!content || decidedNames.size === 0) return content;
  const names = new Set(
    Array.from(decidedNames, (name) => normalizeDishName(name)),
  );
  const kept: string[] = [];
  for (const line of content.split(/\r?\n/)) {
    const listed = listedDishName(line);
    if (listed !== null && names.has(normalizeDishName(listed))) {
      continue;
    }
    kept.push(line);
  }
  return kept.join("\n").replace(/\n{3,}/g, "\n\n").trim();
}

/**
 * Resolve a suggested-dish entity back to its unique live catalog row. This
 * is how the chat card learns its price and cart identity (#791) — the
 * entity envelope deliberately carries neither. Returns null on any protocol
 * mismatch or an ambiguous/missing catalog match. Browsability (86s, closed
 * hours) is NOT checked here; card rendering already decides that.
 */
export function dinerEntityMenuItem(
  entity: AssistantEntity,
  menuData: readonly unknown[],
): { item: Record<string, unknown>; rawID: string } | null {
  if (
    entity.type !== "menu_item" ||
    entity.availability !== "available" ||
    !entity.id.startsWith("menu_item:")
  ) {
    return null;
  }
  const rawID = entity.id.slice("menu_item:".length);
  if (!rawID) return null;

  const matches: Record<string, unknown>[] = [];
  for (const category of menuData) {
    if (!category || typeof category !== "object") continue;
    const items = (category as { items?: unknown }).items;
    if (!Array.isArray(items)) continue;
    for (const item of items) {
      if (
        item &&
        typeof item === "object" &&
        typeof (item as { id?: unknown }).id === "string" &&
        (item as { id: string }).id === rawID
      ) {
        matches.push(item as Record<string, unknown>);
      }
    }
  }
  if (matches.length !== 1) return null;
  return { item: matches[0], rawID };
}

function dinerEntityImages(
  entity: AssistantEntity,
  menuData: readonly unknown[],
  orderability: DinerOrderabilityMap = {},
): string[] {
  const match = dinerEntityMenuItem(entity, menuData);
  if (!match) return [];
  const { item, rawID } = match;
  if (!dinerEntityIsBrowsable(item, rawID, orderability)) {
    return [];
  }

  const candidates = [
    item.image,
    ...(Array.isArray(item.images) ? item.images : []),
  ];
  const seen = new Set<string>();
  const images: string[] = [];
  for (const candidate of candidates) {
    if (
      typeof candidate !== "string" ||
      !candidate ||
      candidate !== candidate.trim() ||
      seen.has(candidate) ||
      !isTrustedEntityImageUrl(candidate)
    ) {
      continue;
    }
    seen.add(candidate);
    images.push(candidate);
    if (images.length === 3) break;
  }
  return images;
}

function dinerEntityIsBrowsable(
  item: Record<string, unknown>,
  rawID: string,
  orderability: DinerOrderabilityMap,
): boolean {
  if (itemLevelUnavailable(item, rawID, orderability)) return false;
  const decision = orderability[rawID];
  const mapState = String(decision?.state ?? "");
  const itemState = String(item.orderability_state ?? "");
  if (
    venueBrowseableStates.has(mapState) ||
    venueBrowseableStates.has(itemState) ||
    isVenueClosedState(mapState) ||
    isVenueClosedState(itemState) ||
    isVenueClosedState(decision?.state)
  ) {
    // Closed hours flip projected `is_available` to false. That is not an 86.
    return true;
  }
  if (item.is_available !== true && item.isAvailable !== true) return false;
  return isMenuItemOrderable(item);
}

/**
 * An 86 remains an 86 after hours. The live map for an inventory-86'd dish
 * is `{ state: "business_closed" }` — identical to a merely-closed dish —
 * so venue pause on the map is not an 86. Catalog `inventory_status` is
 * the flag ResolveOrderability does not wipe.
 */
function itemLevelUnavailable(
  item: Record<string, unknown>,
  rawID: string,
  orderability: DinerOrderabilityMap,
): boolean {
  if (
    itemUnavailableStates.has(String(item.inventory_status ?? "")) ||
    itemUnavailableStates.has(String(item.orderability_state ?? ""))
  ) {
    return true;
  }
  const mapState = String(orderability[rawID]?.state ?? "");
  if (
    venueBrowseableStates.has(mapState) ||
    isVenueClosedState(mapState) ||
    mapState === ""
  ) {
    return false;
  }
  return itemUnavailableStates.has(mapState);
}

const PROTOCOL_LIST_ITEM =
  /^\s*[-*]\s+(?:\*\*)?(.+?)(?:\*\*)?\s+[—–-]\s+\S+\s*$/u;
const PROTOCOL_IDENTITY_LINE = /^\s*\*\*(.+?)\*\*\s+[—–-]\s+\S+\s*$/u;

function listedDishName(line: string): string | null {
  const listed = line.match(PROTOCOL_LIST_ITEM);
  if (listed) return listed[1].trim();
  const identity = line.match(PROTOCOL_IDENTITY_LINE);
  return identity ? identity[1].trim() : null;
}

function normalizeDishName(name: string): string {
  return name.trim().replace(/\s+/g, " ").toLowerCase();
}
