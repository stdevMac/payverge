"use client";

import React, { useState, useEffect, useCallback } from "react";
import {
  Button,
  Skeleton,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
} from "@nextui-org/react";
import { UserPlus, AlertCircle } from "lucide-react";
import { useToast } from "@/contexts/ToastContext";
import { deliveryApi, DeliveryDriver } from "@/api/delivery";
import { useSimpleLocale, getTranslation } from "@/i18n/SimpleTranslationProvider";
import { DriverRow } from "./drivers/DriverRow";
import { DriverEditorModal, DriverFormValues } from "./drivers/DriverEditorModal";

interface DriversManagerProps {
  businessId: number;
}

export default function DriversManager({ businessId }: DriversManagerProps) {
  const { showSuccess, showError } = useToast();
  const { locale } = useSimpleLocale();

  const tString = useCallback(
    (key: string, vars?: Record<string, string>): string => {
      const result = getTranslation(`deliverySettings.${key}`, locale);
      let val = Array.isArray(result) ? result[0] || key : (result as string);
      if (typeof val !== "string") return key;
      if (vars) {
        Object.entries(vars).forEach(([k, v]) => {
          val = (val as string).replace(`{${k}}`, v);
        });
      }
      return val as string;
    },
    [locale]
  );

  const [drivers, setDrivers] = useState<DeliveryDriver[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);
  const [togglingDriverIds, setTogglingDriverIds] = useState<Set<number>>(new Set());

  // Editor modal
  const [editorOpen, setEditorOpen] = useState(false);
  const [editingDriver, setEditingDriver] = useState<DeliveryDriver | null>(null);

  // Remove confirm modal
  const [removeTarget, setRemoveTarget] = useState<DeliveryDriver | null>(null);
  const [removeLoading, setRemoveLoading] = useState(false);

  const fetchDrivers = useCallback(async () => {
    setLoading(true);
    setLoadError(false);
    try {
      const data = await deliveryApi.getBusinessDrivers(businessId);
      setDrivers(data);
    } catch {
      // A load failure is distinct from a save failure: show a dedicated error
      // panel with retry instead of a misleading "couldn't save" toast followed
      // by an empty "no drivers yet" state.
      setLoadError(true);
      showError(tString("drivers.toasts.loadError"));
    } finally {
      setLoading(false);
    }
  }, [businessId, showError, tString]);

  useEffect(() => {
    fetchDrivers();
  }, [fetchDrivers]);

  const openCreate = () => {
    setEditingDriver(null);
    setEditorOpen(true);
  };

  const openEdit = (driver: DeliveryDriver) => {
    setEditingDriver(driver);
    setEditorOpen(true);
  };

  const handleEditorSubmit = async (values: DriverFormValues) => {
    try {
      if (editingDriver) {
        const updated = await deliveryApi.updateDriver(businessId, editingDriver.id, values);
        setDrivers((prev) => prev.map((d) => (d.id === updated.id ? updated : d)));
      } else {
        const created = await deliveryApi.createDriver(businessId, values);
        setDrivers((prev) => [...prev, created]);
      }
      showSuccess(tString("drivers.toasts.saved"));
      setEditorOpen(false);
    } catch {
      showError(tString("drivers.toasts.saveError"));
      throw new Error("save failed");
    }
  };

  const handleToggleAvailability = async (
    driver: DeliveryDriver,
    next: boolean
  ) => {
    // Prevent duplicate in-flight calls for the same driver.
    if (togglingDriverIds.has(driver.id)) return;
    setTogglingDriverIds((prev) => new Set([...prev, driver.id]));

    // Optimistic update
    setDrivers((prev) =>
      prev.map((d) => (d.id === driver.id ? { ...d, is_available: next } : d))
    );
    try {
      await deliveryApi.updateDriver(businessId, driver.id, { is_available: next });
    } catch {
      // Roll back
      setDrivers((prev) =>
        prev.map((d) =>
          d.id === driver.id ? { ...d, is_available: driver.is_available } : d
        )
      );
      showError(tString("drivers.toasts.availabilityError"));
    } finally {
      setTogglingDriverIds((prev) => {
        const next = new Set(prev);
        next.delete(driver.id);
        return next;
      });
    }
  };

  const handleRemoveConfirm = async () => {
    if (!removeTarget) return;
    setRemoveLoading(true);
    try {
      await deliveryApi.deleteDriver(businessId, removeTarget.id);
      setDrivers((prev) => prev.filter((d) => d.id !== removeTarget.id));
      showSuccess(tString("drivers.toasts.deleted"));
      setRemoveTarget(null);
    } catch {
      showError(tString("drivers.toasts.deleteError"));
    } finally {
      setRemoveLoading(false);
    }
  };

  return (
    <div className="space-y-4">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h3 className="text-base font-semibold text-ink-950">
          {tString("drivers.title")}
        </h3>
        <Button
          size="sm"
          variant="solid"
          startContent={<UserPlus size={14} />}
          onPress={openCreate}
          className="bg-brand font-semibold text-white shadow-sm shadow-brand/20 hover:bg-brand-dark"
        >
          {tString("drivers.addDriver")}
        </Button>
      </div>

      {/* List */}
      {loading ? (
        <div className="space-y-3">
          {[1, 2, 3].map((i) => (
            <Skeleton key={i} className="rounded-xl h-16 w-full" />
          ))}
        </div>
      ) : loadError ? (
        <div
          data-testid="drivers-load-error"
          className="rounded-3xl border border-red-200 bg-red-50 p-8 flex flex-col items-center gap-3 text-center shadow-sm"
        >
          <AlertCircle className="w-8 h-8 text-red-400" />
          <p className="text-sm text-red-700">{tString("drivers.loadError")}</p>
          <Button
            size="sm"
            color="danger"
            variant="flat"
            onPress={() => void fetchDrivers()}
          >
            {tString("drivers.retry")}
          </Button>
        </div>
      ) : drivers.length === 0 ? (
        <div className="rounded-3xl border border-warm-200 bg-white p-8 text-center shadow-sm shadow-warm-900/5">
          <p className="text-sm text-ink-600">{tString("drivers.empty")}</p>
        </div>
      ) : (
        <div className="space-y-3">
          {drivers.map((driver) => (
            <DriverRow
              key={driver.id}
              driver={driver}
              onEdit={openEdit}
              onRemove={(d) => setRemoveTarget(d)}
              onToggleAvailability={handleToggleAvailability}
              isToggling={togglingDriverIds.has(driver.id)}
              tString={tString}
            />
          ))}
        </div>
      )}

      {/* Editor modal */}
      <DriverEditorModal
        isOpen={editorOpen}
        driver={editingDriver}
        onClose={() => setEditorOpen(false)}
        onSubmit={handleEditorSubmit}
        tString={tString}
      />

      {/* Remove confirm modal */}
      <Modal
        isOpen={!!removeTarget}
        onClose={() => setRemoveTarget(null)}
        placement="center"
        size="sm"
      >
        <ModalContent className="overflow-hidden rounded-3xl border border-warm-200 bg-white shadow-2xl shadow-warm-900/15">
          <ModalHeader className="border-b border-warm-200/80 bg-warm-50/70 text-base font-semibold text-ink-950">
            {tString("drivers.confirm.removeTitle")}
          </ModalHeader>
          <ModalBody className="py-5">
            <p className="text-sm text-ink-600">
              {tString("drivers.confirm.removeDescription").replace(
                "{name}",
                removeTarget?.name ?? ""
              )}
            </p>
          </ModalBody>
          <ModalFooter className="border-t border-warm-200/80 bg-warm-50/70">
            <Button
              variant="light"
              onPress={() => setRemoveTarget(null)}
              isDisabled={removeLoading}
              className="font-semibold text-ink-700 hover:bg-warm-100"
            >
              {tString("drivers.confirm.removeCancel")}
            </Button>
            <Button
              onPress={handleRemoveConfirm}
              isLoading={removeLoading}
              className="bg-rose-600 font-semibold text-white shadow-sm shadow-rose-900/20 hover:bg-rose-700"
            >
              {tString("drivers.confirm.removeConfirm")}
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
}
