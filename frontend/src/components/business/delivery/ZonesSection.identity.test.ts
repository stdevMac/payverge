import {
  ensureZoneClientKeys,
  newZone,
  reconcileZoneClientKeys,
  zoneExpansionKey,
} from "./ZonesSection";
import type { DeliveryZoneDto } from "@/api/delivery";
import { asDollars } from "@/types/money";

describe("L3-38 zone expansion identity", () => {
  it("keys by zone id not list index", () => {
    const z = { id: 42, name: "A" } as DeliveryZoneDto;
    expect(zoneExpansionKey(z, 0)).toBe("id:42");
    expect(zoneExpansionKey(z, 7)).toBe("id:42");
  });

  it("falls back to index only when id missing", () => {
    const z = { name: "draft" } as DeliveryZoneDto;
    expect(zoneExpansionKey(z, 3)).toBe("idx:3");
  });

  it("prefers the client key over the server id", () => {
    const z = { id: 42, name: "A", _client_key: "zone-abc" } as DeliveryZoneDto;
    expect(zoneExpansionKey(z, 0)).toBe("zone-abc");
  });
});

describe("L3-38 client keys survive the save round-trip", () => {
  const mk = (
    id: number,
    name: string,
    priority: number,
  ): DeliveryZoneDto =>
    ({
      id,
      name,
      delivery_fee: asDollars(0),
      minimum_order_amount: asDollars(0),
      estimated_time: 30,
      priority,
      cutoff_buffer_minutes: 0,
      is_active: false,
      boundaries: {},
    }) as DeliveryZoneDto;

  it("stamps a stable key on every zone and never reuses one", () => {
    const keyed = ensureZoneClientKeys([mk(1, "North", 1), mk(2, "South", 2)]);
    const keys = keyed.map((z) => z._client_key);
    expect(keys.every(Boolean)).toBe(true);
    expect(new Set(keys).size).toBe(2);

    // Idempotent: re-stamping keeps the same keys (identity must not churn
    // on every render).
    expect(ensureZoneClientKeys(keyed).map((z) => z._client_key)).toEqual(keys);

    // A freshly added zone is born with a key.
    expect(newZone(keyed)._client_key).toBeTruthy();
  });

  it("carries keys onto the saved zones despite the backend re-sort", () => {
    // Operator state: two saved zones + one brand new zone (negative id).
    const before = ensureZoneClientKeys([
      mk(7, "North", 1),
      mk(9, "South", 2),
      newZone([]),
    ]);
    before[2].name = "Airport";

    // What the PUT returns: real id for the new zone, re-sorted by
    // priority DESC (Airport got the highest priority from newZone()).
    const saved = [
      mk(31, "Airport", 3),
      mk(9, "South", 2),
      mk(7, "North", 1),
    ];

    const next = reconcileZoneClientKeys(before, saved);
    const keyOf = (name: string) =>
      next.find((z) => z.name === name)?._client_key;

    expect(keyOf("North")).toBe(before[0]._client_key);
    expect(keyOf("South")).toBe(before[1]._client_key);
    // The zone the operator was editing keeps its identity, so its card
    // stays expanded across the save.
    expect(keyOf("Airport")).toBe(before[2]._client_key);
    expect(next.find((z) => z.name === "Airport")?.id).toBe(31);
  });

  it("stamps a fresh key on a zone the server returned but the client never had", () => {
    const before = ensureZoneClientKeys([mk(7, "North", 1)]);
    const saved = [mk(7, "North", 1), mk(8, "Ghost", 5)];

    const next = reconcileZoneClientKeys(before, saved);
    expect(next[0]._client_key).toBe(before[0]._client_key);
    expect(next[1]._client_key).toBeTruthy();
    expect(next[1]._client_key).not.toBe(before[0]._client_key);
  });
});

describe("L3-40 new zones are inactive by default", () => {
  it("does not flip in_house_delivery_enabled via active-by-default", () => {
    const zone = newZone([]);
    expect(zone.is_active).toBe(false);
    // Interaction: only active zones enable in-house delivery.
    const active = [{ ...zone, is_active: true, delivery_fee: asDollars(1) }];
    expect(active.some((z) => z.is_active)).toBe(true);
    expect([zone].some((z) => z.is_active)).toBe(false);
  });
});
