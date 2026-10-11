"use client";

import React, { useCallback, useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { Input, Button, Chip, useDisclosure } from "@nextui-org/react";
import { toast } from "react-hot-toast";
import { positionsApi, type Position } from "@/api/positions";
import { SkeletonList } from "@/components/ui/skeletons";
import ConfirmationModal from "@/components/business/modals/ConfirmationModal";

export interface PositionsManagerLabels {
  title: string;
  subtitle: string;
  addButton: string;
  namePlaceholder: string;
  departmentPlaceholder: string;
  /** Explains what departments do (chat channels) under the add form. */
  departmentHint: string;
  /** Intro for the quick-add row shown while the list is empty. */
  suggestionsLabel: string;
  /** Comma-separated localized role names for the quick-add chips. */
  suggestionNames: string;
  empty: string;
  retire: string;
  /** ConfirmationModal title — archive/reactivable, not permanent destroy. */
  retireConfirmTitle: string;
  /** Soft-archive body; may include {name}. */
  retireConfirmDescription: string;
  retireConfirmAction: string;
  saved: string;
  saveError: string;
  /** L5-18: shown when the name collides case-insensitively. */
  duplicateName?: string;
  retired: string;
  retireError: string;
  loadError: string;
  retry: string;
  loading: string;
}

export interface PositionsManagerProps {
  businessId: string;
  labels: PositionsManagerLabels;
}

export default function PositionsManager({ businessId, labels }: PositionsManagerProps) {
  const queryClient = useQueryClient();
  const [positions, setPositions] = useState<Position[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const [name, setName] = useState("");
  const [department, setDepartment] = useState("");
  const [saving, setSaving] = useState(false);
  // Soft-archive (BE sets is_active=false). Confirm first so list-hide doesn't read as destroy.
  const [positionToRetire, setPositionToRetire] = useState<Position | null>(null);
  const {
    isOpen: isRetireOpen,
    onOpen: onRetireOpen,
    onOpenChange: onRetireOpenChange,
  } = useDisclosure();

  const load = useCallback(async () => {
    setLoading(true);
    setLoadError(false);
    try {
      setPositions(await positionsApi.list(businessId));
    } catch (e) {
      // 403 (non-manager lacking schedule:read) → render empty silently.
      // Anything else is a real failure worth surfacing to the console + UI.
      const status = (e as { response?: { status?: number } })?.response?.status;
      if (status !== 403) {
        console.error("[positions] load failed", e);
        setLoadError(true);
      }
    } finally {
      setLoading(false);
    }
  }, [businessId]);

  useEffect(() => {
    void load();
  }, [load]);

  const [nameInvalid, setNameInvalid] = useState(false);

  const isDuplicateName = useCallback(
    (posName: string) => {
      const needle = posName.trim().toLowerCase();
      if (!needle) return false;
      return positions.some((p) => p.name.trim().toLowerCase() === needle);
    },
    [positions],
  );

  const createPosition = useCallback(
    async (posName: string, posDepartment: string) => {
      // L5-18: case-insensitive dupe guard covers form + quick-add chips.
      if (isDuplicateName(posName)) {
        setNameInvalid(true);
        toast.error(labels.duplicateName ?? labels.saveError);
        return;
      }
      setNameInvalid(false);
      setSaving(true);
      try {
        await positionsApi.create(businessId, {
          name: posName,
          department: posDepartment,
          sort_order: positions.length,
        });
        setName("");
        setDepartment("");
        toast.success(labels.saved);
        await load();
        // Keep StaffManagement header / schedule consumers in sync (L5-20).
        void queryClient.invalidateQueries({ queryKey: ["positions", businessId] });
      } catch {
        toast.error(labels.saveError);
      } finally {
        setSaving(false);
      }
    },
    [businessId, positions.length, labels, load, queryClient, isDuplicateName],
  );

  const handleAdd = useCallback(async () => {
    if (!name.trim()) return;
    await createPosition(name.trim(), department.trim());
  }, [name, department, createPosition]);

  const suggestions = labels.suggestionNames
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);

  const requestRetire = useCallback(
    (position: Position) => {
      setPositionToRetire(position);
      onRetireOpen();
    },
    [onRetireOpen],
  );

  const confirmRetire = useCallback(async () => {
    if (!positionToRetire) return;
    try {
      await positionsApi.remove(businessId, positionToRetire.id);
      toast.success(labels.retired);
      setPositionToRetire(null);
      await load();
      // Keep StaffManagement header / schedule consumers in sync (L5-20).
      void queryClient.invalidateQueries({ queryKey: ["positions", businessId] });
    } catch {
      toast.error(labels.retireError);
    }
  }, [businessId, labels, load, positionToRetire, queryClient]);

  const retireDescription = labels.retireConfirmDescription.replace(
    /\{name\}/g,
    positionToRetire?.name ?? "",
  );

  return (
    <div className="mt-4 rounded-xl border border-gray-200 p-4">
      <h2 className="text-base font-semibold text-gray-900">{labels.title}</h2>
      <p className="text-[11px] text-gray-500">{labels.subtitle}</p>

      <form
        onSubmit={(e) => {
          e.preventDefault();
          void handleAdd();
        }}
        className="mt-3 flex flex-col gap-2 sm:flex-row"
      >
        <Input
          size="sm"
          className="min-w-0 sm:flex-1"
          aria-label={labels.namePlaceholder}
          placeholder={labels.namePlaceholder}
          value={name}
          onValueChange={(v) => {
            setName(v);
            if (nameInvalid) setNameInvalid(false);
          }}
          isInvalid={nameInvalid}
          errorMessage={
            nameInvalid
              ? labels.duplicateName ?? labels.saveError
              : undefined
          }
          data-testid="position-name-input"
        />
        <Input
          size="sm"
          className="min-w-0 sm:flex-1"
          aria-label={labels.departmentPlaceholder}
          placeholder={labels.departmentPlaceholder}
          value={department}
          onValueChange={setDepartment}
        />
        {/* shrink-0 + nowrap: full-width sibling inputs otherwise squeeze the
            button until its label clips ("dd position"). */}
        <Button size="sm" type="submit" color="primary" className="shrink-0 whitespace-nowrap self-start sm:self-auto" isLoading={saving} isDisabled={!name.trim()}>
          {labels.addButton}
        </Button>
      </form>
      <p className="mt-1.5 text-[11px] text-gray-500">{labels.departmentHint}</p>

      {/* Quick-add for the cold start: common restaurant roles, one tap each.
          Disappears as soon as the first position exists. */}
      {!loading && !loadError && positions.length === 0 && suggestions.length > 0 ? (
        <div className="mt-3 flex flex-wrap items-center gap-1.5">
          <span className="text-xs text-gray-500">{labels.suggestionsLabel}</span>
          {suggestions.map((role) => (
            <button
              key={role}
              type="button"
              disabled={saving}
              onClick={() => void createPosition(role, "")}
              className="rounded-full border border-gray-200 bg-white px-2.5 py-1 text-xs font-medium text-gray-700 transition hover:border-gray-300 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50"
            >
              {role}
            </button>
          ))}
        </div>
      ) : null}

      <div className="mt-4">
        {loading ? (
          <SkeletonList rows={3} bordered={false} ariaLabel={labels.loading} />
        ) : loadError ? (
          <div className="flex items-center justify-between rounded-lg bg-gray-50 px-3 py-2">
            <p className="text-sm text-gray-500">{labels.loadError}</p>
            <Button size="sm" variant="light" onPress={() => void load()}>
              {labels.retry}
            </Button>
          </div>
        ) : positions.length === 0 ? (
          <p className="text-sm text-gray-500">{labels.empty}</p>
        ) : (
          <ul className="flex flex-col gap-2">
            {positions.map((p) => (
              <li
                key={p.id}
                className="flex items-center justify-between rounded-lg bg-gray-50 px-3 py-2"
              >
                <span className="flex items-center gap-2 text-sm text-gray-900">
                  {p.name}
                  {p.department ? (
                    <Chip size="sm" variant="flat">
                      {p.department}
                    </Chip>
                  ) : null}
                </span>
                <Button
                  size="sm"
                  variant="light"
                  aria-label={`${labels.retire} ${p.name}`}
                  onPress={() => requestRetire(p)}
                >
                  {labels.retire}
                </Button>
              </li>
            ))}
          </ul>
        )}
      </div>

      <ConfirmationModal
        isOpen={isRetireOpen}
        onOpenChange={() => {
          setPositionToRetire(null);
          onRetireOpenChange();
        }}
        title={labels.retireConfirmTitle}
        description={retireDescription}
        confirmLabel={labels.retireConfirmAction}
        isDanger
        onConfirm={() => confirmRetire()}
      />
    </div>
  );
}
