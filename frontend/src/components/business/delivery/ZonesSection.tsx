import React, { useState } from "react";
import {
  Card,
  CardHeader,
  CardBody,
  Button,
  Chip,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
} from "@nextui-org/react";
import { MapPin, Plus, ChevronDown, ChevronUp, Trash2 } from "lucide-react";
import type { DeliveryZoneDto } from "@/api/delivery";
import { asDollars } from "@/types/money";
import { ZoneEditor } from "./ZoneEditor";

interface ZonesSectionProps {
  zones: DeliveryZoneDto[];
  onChange: (zones: DeliveryZoneDto[]) => void;
  tString: (key: string) => string;
  // Forwarded to ZoneEditor so per-zone fee inputs show the business's
  // currency instead of a hardcoded "$".
  currencyPrefix?: string;
}

let zoneIdCounter = -1;
let zoneKeyCounter = 0;

/** Fresh client-only zone identity (L3-38). Opaque and never sent to the API. */
function newZoneClientKey(): string {
  zoneKeyCounter += 1;
  return `zone-${Date.now().toString(36)}-${zoneKeyCounter}`;
}

export function newZone(existing: DeliveryZoneDto[]): DeliveryZoneDto {
  // Default the priority to max(existing)+1 so active-zone priority collisions
  // don't fail save (DEL-OP-3). L3-40: born inactive — activation is explicit.
  const nextPriority =
    existing.reduce((max, z) => Math.max(max, z.priority || 0), 0) + 1;
  return {
    id: zoneIdCounter--,
    _client_key: newZoneClientKey(),
    name: "",
    delivery_fee: asDollars(0),
    minimum_order_amount: asDollars(0),
    estimated_time: 30,
    priority: nextPriority,
    cutoff_buffer_minutes: 0,
    is_active: false,
    boundaries: {},
  };
}

/**
 * Stamps a client key on any zone that lacks one (server-loaded rows).
 * Idempotent — zones that already carry a key are returned untouched, so the
 * identity does not churn on every render.
 */
export function ensureZoneClientKeys(
  zones: DeliveryZoneDto[],
): DeliveryZoneDto[] {
  return zones.map((zone) =>
    zone._client_key ? zone : { ...zone, _client_key: newZoneClientKey() },
  );
}

/**
 * L3-38 — carries client keys from the pre-save zones onto the zones the PUT
 * returned.
 *
 * Matching is by server id first (zones that already existed keep theirs).
 * Newly created zones have no id to match on, so the leftovers are paired in
 * order: `syncDeliveryZones` creates rows in payload order, so the nth unmatched
 * saved zone is the nth zone that was new locally — even though the response is
 * re-sorted by `priority DESC, estimated_time ASC, id ASC`. Anything still
 * unmatched (a row the client never had) gets a fresh key.
 */
export function reconcileZoneClientKeys(
  previous: DeliveryZoneDto[],
  saved: DeliveryZoneDto[],
): DeliveryZoneDto[] {
  const byID = new Map<number, string>();
  const unsavedKeys: string[] = [];
  for (const zone of previous) {
    const key = zone._client_key;
    if (!key) continue;
    if (zone.id != null && zone.id > 0) byID.set(zone.id, key);
    else unsavedKeys.push(key);
  }

  const matched = saved.map((zone) => ({
    zone,
    key: zone.id != null && zone.id > 0 ? byID.get(zone.id) : undefined,
  }));
  // Assign leftovers in ascending server id — that is creation order.
  const leftovers = matched
    .filter((entry) => !entry.key)
    .sort((a, b) => (a.zone.id ?? 0) - (b.zone.id ?? 0));
  leftovers.forEach((entry, i) => {
    entry.key = unsavedKeys[i];
  });

  return matched.map(({ zone, key }) => ({
    ...zone,
    _client_key: key || newZoneClientKey(),
  }));
}

/** Stable expansion key for a zone (L3-38): client key > id > list index. */
export function zoneExpansionKey(zone: DeliveryZoneDto, index: number): string {
  if (zone._client_key) return zone._client_key;
  if (zone.id != null) return `id:${zone.id}`;
  return `idx:${index}`;
}

export function ZonesSection({
  zones,
  onChange,
  tString,
  currencyPrefix,
}: ZonesSectionProps) {
  // L3-38: expand by stable zone id so BE re-sort after save keeps the same card open.
  const [expandedKey, setExpandedKey] = useState<string | null>(null);
  // A fully-configured zone (fee, minimums, boundaries) is easy to fat-finger
  // away on a tablet, so deletion goes through a confirm step matching the
  // driver-removal pattern (audit L6 #13).
  const [removeIndex, setRemoveIndex] = useState<number | null>(null);

  const addZone = () => {
    const next = [...zones, newZone(zones)];
    onChange(next);
    const created = next[next.length - 1];
    setExpandedKey(zoneExpansionKey(created, next.length - 1));
  };

  const removeZone = (index: number) => {
    const removed = zones[index];
    const removedKey = zoneExpansionKey(removed, index);
    const next = zones.filter((_, i) => i !== index);
    onChange(next);
    if (expandedKey === removedKey) {
      setExpandedKey(null);
    }
  };

  const confirmRemoveZone = () => {
    if (removeIndex === null) return;
    removeZone(removeIndex);
    setRemoveIndex(null);
  };

  const updateZone = (index: number, zone: DeliveryZoneDto) => {
    onChange(zones.map((z, i) => (i === index ? zone : z)));
  };

  const toggleExpand = (key: string) => {
    setExpandedKey((prev) => (prev === key ? null : key));
  };

  return (
    <Card className="rounded-3xl border border-warm-200 bg-white shadow-sm shadow-warm-900/5">
      <CardHeader className="flex gap-3">
        <MapPin className="h-5 w-5 text-brand" />
        <div className="flex-1">
          <p className="font-semibold text-ink-950">
            {tString("focused.zones.cardTitle")}
          </p>
          <p className="text-sm text-ink-600">
            {tString("focused.zones.cardSubtitle")}
          </p>
        </div>
      </CardHeader>
      <CardBody className="space-y-3">
        <p className="text-xs text-ink-500">
          {tString("focused.zones.overlapHint")}
        </p>

        {zones.length === 0 && (
          <p className="py-4 text-center text-sm text-ink-600">
            {tString("focused.zones.noZones")}
          </p>
        )}

        {zones.map((zone, index) => {
          const expandKey = zoneExpansionKey(zone, index);
          const isExpanded = expandedKey === expandKey;
          const displayName =
            zone.name || tString("focused.zones.zoneNamePlaceholder");
          return (
            <div
              key={expandKey}
              className="overflow-hidden rounded-2xl border border-warm-200 bg-white shadow-sm shadow-warm-900/5"
            >
              <div className="flex items-center gap-3 px-4 py-3 bg-white">
                <button
                  type="button"
                  onClick={() => toggleExpand(expandKey)}
                  className="flex-1 flex items-center gap-3 text-left min-w-0"
                  aria-expanded={isExpanded}
                >
                  <Chip
                    size="sm"
                    variant="flat"
                    className="shrink-0 border border-warm-200 bg-warm-100 font-mono text-xs text-ink-700"
                  >
                    #{zone.priority}
                  </Chip>
                  <span className="truncate text-sm font-semibold text-ink-950">
                    {displayName}
                  </span>
                  <Chip
                    size="sm"
                    variant="flat"
                    className={
                      zone.is_active
                        ? "shrink-0 border border-emerald-200 bg-emerald-50 text-emerald-700"
                        : "shrink-0 border border-warm-200 bg-warm-100 text-ink-600"
                    }
                  >
                    {zone.is_active
                      ? tString("focused.zones.active")
                      : tString("focused.zones.inactive")}
                  </Chip>
                  {isExpanded ? (
                    <ChevronUp className="ml-auto h-4 w-4 shrink-0 text-ink-400" />
                  ) : (
                    <ChevronDown className="ml-auto h-4 w-4 shrink-0 text-ink-400" />
                  )}
                </button>
                <Button
                  isIconOnly
                  size="sm"
                  variant="light"
                  aria-label={tString("focused.zones.removeZone")}
                  onPress={() => setRemoveIndex(index)}
                  className="text-rose-600 hover:bg-rose-50"
                >
                  <Trash2 className="w-4 h-4" />
                </Button>
              </div>

              {isExpanded && (
                <div className="border-t border-warm-100 bg-warm-50/60 px-4 pb-4">
                  <ZoneEditor
                    zone={zone}
                    onChange={(updated) => updateZone(index, updated)}
                    tString={tString}
                    currencyPrefix={currencyPrefix}
                  />
                </div>
              )}
            </div>
          );
        })}

        {/* Compact rounded-full primary per the button recipe — self-start
            keeps the NextUI Button from stretching full-width in CardBody. */}
        <Button
          startContent={<Plus className="w-4 h-4" />}
          onPress={addZone}
          radius="full"
          className="mt-2 self-start bg-brand font-medium text-white hover:bg-brand-dark"
        >
          {tString("focused.zones.addZone")}
        </Button>
      </CardBody>

      <Modal
        isOpen={removeIndex !== null}
        onClose={() => setRemoveIndex(null)}
        placement="center"
        size="sm"
      >
        <ModalContent className="overflow-hidden rounded-3xl border border-warm-200 bg-white shadow-2xl shadow-warm-900/15">
          <ModalHeader className="border-b border-warm-200/80 bg-warm-50/70 text-base font-semibold text-ink-950">
            {tString("focused.zones.confirm.removeTitle")}
          </ModalHeader>
          <ModalBody className="py-5">
            <p className="text-sm text-ink-600">
              {tString("focused.zones.confirm.removeDescription").replace(
                "{name}",
                removeIndex !== null
                  ? zones[removeIndex]?.name ||
                      tString("focused.zones.zoneNamePlaceholder")
                  : "",
              )}
            </p>
          </ModalBody>
          <ModalFooter className="border-t border-warm-200/80 bg-warm-50/70">
            <Button
              variant="light"
              onPress={() => setRemoveIndex(null)}
              className="font-semibold text-ink-700 hover:bg-warm-100"
            >
              {tString("focused.zones.confirm.removeCancel")}
            </Button>
            <Button
              onPress={confirmRemoveZone}
              className="bg-rose-600 font-semibold text-white shadow-sm shadow-rose-900/20 hover:bg-rose-700"
            >
              {tString("focused.zones.confirm.removeConfirm")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </Card>
  );
}
