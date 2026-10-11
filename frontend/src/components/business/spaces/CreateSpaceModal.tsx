"use client";

import React, { useCallback, useEffect, useState } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Input,
  Select,
  SelectItem,
} from "@nextui-org/react";
import { Pencil, Smartphone } from "lucide-react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import type {
  CreateSpaceInput,
  SpaceMeasurementUnit,
  SpaceType,
} from "@/api/spaces";

export type CreateSpacePath = "scan" | "draw";

export interface CreateSpaceModalProps {
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (
    input: CreateSpaceInput,
    path: CreateSpacePath,
  ) => Promise<void>;
  /** When set, skip the form and open directly on path chooser with prefilled meta (e.g. empty-state CTAs). */
  initialPath?: CreateSpacePath | null;
  isSubmitting?: boolean;
}

const SPACE_TYPES: SpaceType[] = [
  "indoor",
  "outdoor",
  "patio",
  "rooftop",
  "bar",
  "private",
  "terrace",
];

const UNITS: SpaceMeasurementUnit[] = ["m", "ft"];

type Step = "details" | "path";

export default function CreateSpaceModal({
  isOpen,
  onOpenChange,
  onSubmit,
  initialPath = null,
  isSubmitting = false,
}: CreateSpaceModalProps) {
  const { locale } = useSimpleLocale();
  const t = useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      const result = getTranslation(`spacesTables.${key}`, locale, params);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  const [step, setStep] = useState<Step>("details");
  const [name, setName] = useState("");
  const [spaceType, setSpaceType] = useState<SpaceType>("indoor");
  const [floorLevel, setFloorLevel] = useState(0);
  const [unit, setUnit] = useState<SpaceMeasurementUnit>("m");
  const [nameError, setNameError] = useState<string | null>(null);
  const [pendingPath, setPendingPath] = useState<CreateSpacePath | null>(
    initialPath,
  );

  useEffect(() => {
    if (!isOpen) {
      setStep("details");
      setName("");
      setSpaceType("indoor");
      setFloorLevel(0);
      setUnit("m");
      setNameError(null);
      setPendingPath(initialPath);
      return;
    }
    if (initialPath) {
      setPendingPath(initialPath);
    }
  }, [isOpen, initialPath]);

  const goPath = () => {
    if (!name.trim()) {
      setNameError(t("create.nameRequired"));
      return;
    }
    setNameError(null);
    setStep("path");
  };

  const handleCreate = async (path: CreateSpacePath) => {
    if (!name.trim()) {
      setNameError(t("create.nameRequired"));
      setStep("details");
      return;
    }
    await onSubmit(
      {
        name: name.trim(),
        space_type: spaceType,
        floor_level: floorLevel,
        measurement_unit: unit,
      },
      path,
    );
  };

  return (
    <Modal
      isOpen={isOpen}
      onOpenChange={onOpenChange}
      size="lg"
      scrollBehavior="inside"
      aria-label={t("a11y.createDialog")}
    >
      <ModalContent>
        {(onClose) => (
          <>
            <ModalHeader className="flex flex-col gap-1">
              <span className="font-title text-xl text-ink-900">
                {step === "details" ? t("create.title") : t("create.pathTitle")}
              </span>
              <span className="text-sm font-normal text-ink-500">
                {step === "details"
                  ? t("create.subtitle")
                  : t("create.pathSubtitle")}
              </span>
            </ModalHeader>
            <ModalBody>
              {step === "details" ? (
                <div className="flex flex-col gap-4">
                  <Input
                    label={t("create.nameLabel")}
                    placeholder={t("create.namePlaceholder")}
                    value={name}
                    onValueChange={(v) => {
                      setName(v);
                      if (nameError) setNameError(null);
                    }}
                    isInvalid={Boolean(nameError)}
                    errorMessage={nameError ?? undefined}
                    variant="bordered"
                    autoFocus
                    classNames={{
                      inputWrapper:
                        "border-warm-200 data-[hover=true]:border-brand/40",
                    }}
                  />
                  <Select
                    label={t("create.typeLabel")}
                    selectedKeys={[spaceType]}
                    onSelectionChange={(keys) => {
                      const next = Array.from(keys)[0] as SpaceType | undefined;
                      if (next) setSpaceType(next);
                    }}
                    variant="bordered"
                    classNames={{
                      trigger:
                        "border-warm-200 data-[hover=true]:border-brand/40",
                    }}
                    items={SPACE_TYPES.map((key) => ({
                      key,
                      label: t(`spaceTypes.${key}`),
                    }))}
                  >
                    {(item) => (
                      <SelectItem key={item.key}>{item.label}</SelectItem>
                    )}
                  </Select>
                  <Input
                    type="number"
                    label={t("create.floorLabel")}
                    description={t("create.floorHint")}
                    value={String(floorLevel)}
                    onValueChange={(v) => {
                      const n = parseInt(v, 10);
                      setFloorLevel(Number.isFinite(n) ? n : 0);
                    }}
                    variant="bordered"
                    classNames={{
                      inputWrapper:
                        "border-warm-200 data-[hover=true]:border-brand/40",
                    }}
                  />
                  <Select
                    label={t("create.unitLabel")}
                    selectedKeys={[unit]}
                    onSelectionChange={(keys) => {
                      const next = Array.from(keys)[0] as
                        | SpaceMeasurementUnit
                        | undefined;
                      if (next) setUnit(next);
                    }}
                    variant="bordered"
                    classNames={{
                      trigger:
                        "border-warm-200 data-[hover=true]:border-brand/40",
                    }}
                    items={UNITS.map((key) => ({
                      key,
                      label: t(`units.${key}`),
                    }))}
                  >
                    {(item) => (
                      <SelectItem key={item.key}>{item.label}</SelectItem>
                    )}
                  </Select>
                </div>
              ) : (
                <div
                  className="grid gap-3 sm:grid-cols-2"
                  role="group"
                  aria-label={t("a11y.pathChooser")}
                >
                  <button
                    type="button"
                    onClick={() => void handleCreate("scan")}
                    disabled={isSubmitting}
                    className="flex flex-col items-start gap-3 rounded-2xl border border-warm-200 bg-white p-4 text-left transition-colors hover:border-brand/40 hover:bg-brand/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand disabled:opacity-60"
                    data-testid="create-path-scan"
                  >
                    <span className="flex h-10 w-10 items-center justify-center rounded-xl bg-brand/10 text-brand">
                      <Smartphone className="h-5 w-5" />
                    </span>
                    <span className="text-sm font-semibold text-ink-900">
                      {t("create.pathScanTitle")}
                    </span>
                    <span className="text-sm text-ink-500">
                      {t("create.pathScanBody")}
                    </span>
                  </button>
                  <button
                    type="button"
                    onClick={() => void handleCreate("draw")}
                    disabled={isSubmitting}
                    className="flex flex-col items-start gap-3 rounded-2xl border border-warm-200 bg-white p-4 text-left transition-colors hover:border-brand/40 hover:bg-brand/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand disabled:opacity-60"
                    data-testid="create-path-draw"
                  >
                    <span className="flex h-10 w-10 items-center justify-center rounded-xl bg-brand/10 text-brand">
                      <Pencil className="h-5 w-5" />
                    </span>
                    <span className="text-sm font-semibold text-ink-900">
                      {t("create.pathDrawTitle")}
                    </span>
                    <span className="text-sm text-ink-500">
                      {t("create.pathDrawBody")}
                    </span>
                  </button>
                </div>
              )}
            </ModalBody>
            <ModalFooter>
              {step === "details" ? (
                <>
                  <Button variant="light" onPress={onClose}>
                    {t("create.cancel")}
                  </Button>
                  <Button
                    className="bg-brand text-white font-medium"
                    onPress={goPath}
                    data-testid="create-space-continue"
                  >
                    {t("create.continue")}
                  </Button>
                </>
              ) : (
                <>
                  <Button
                    variant="light"
                    onPress={() => setStep("details")}
                    isDisabled={isSubmitting}
                  >
                    {t("create.back")}
                  </Button>
                  {pendingPath === "scan" ? (
                    <Button
                      className="bg-brand text-white font-medium"
                      isLoading={isSubmitting}
                      onPress={() => void handleCreate("scan")}
                    >
                      {t("create.createAndScan")}
                    </Button>
                  ) : pendingPath === "draw" ? (
                    <Button
                      className="bg-brand text-white font-medium"
                      isLoading={isSubmitting}
                      onPress={() => void handleCreate("draw")}
                    >
                      {t("create.createAndDraw")}
                    </Button>
                  ) : null}
                </>
              )}
            </ModalFooter>
          </>
        )}
      </ModalContent>
    </Modal>
  );
}
