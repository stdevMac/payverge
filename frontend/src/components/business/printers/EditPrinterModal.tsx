"use client";

import { useCallback, useEffect, useState } from "react";
import {
  Button,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Select,
  SelectItem,
  Switch,
} from "@nextui-org/react";

import { updatePrinter } from "@/api/print";
import type { Printer } from "@/api/print";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";

interface Props {
  businessId: number;
  printer: Printer | null;
  open: boolean;
  onClose: () => void;
  onSaved: () => void;
}

export default function EditPrinterModal({
  businessId,
  printer,
  open,
  onClose,
  onSaved,
}: Props) {
  const { locale } = useSimpleLocale();
  const t = useCallback(
    (key: string): string => {
      const result = getTranslation(`printers.${key}`, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  const [name, setName] = useState("");
  const [paperWidth, setPaperWidth] = useState("80");
  const [role, setRole] = useState("bill");
  const [enabled, setEnabled] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!printer || !open) return;
    setName(printer.name);
    setPaperWidth(String(printer.paper_width_mm || 80));
    setRole(printer.role || "bill");
    setEnabled(printer.enabled);
    setError(null);
  }, [printer, open]);

  async function handleSave() {
    if (!printer) return;
    const trimmedName = name.trim();
    if (!trimmedName) return;
    setSubmitting(true);
    setError(null);
    try {
      await updatePrinter(businessId, printer.id, {
        name: trimmedName,
        paper_width_mm: Number(paperWidth),
        role,
        enabled,
      });
      onSaved();
      onClose();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <Modal isOpen={open} onClose={onClose}>
      <ModalContent>
        <ModalHeader>{t("edit.title")}</ModalHeader>
        <ModalBody className="space-y-4">
          <Input
            label={t("wizard.nameLabel")}
            value={name}
            onValueChange={setName}
            isRequired
          />
          <Select
            label={t("wizard.paperWidthLabel")}
            selectedKeys={[paperWidth]}
            onSelectionChange={(keys) => {
              const next = Array.from(keys)[0];
              if (next != null) setPaperWidth(String(next));
            }}
            // Uniform mapped children avoid NextUI Select TS2322 on mixed static+map.
          >
            {[
              { key: "80", label: t("wizard.paper80") },
              { key: "58", label: t("wizard.paper58") },
            ].map((opt) => (
              <SelectItem key={opt.key} textValue={opt.label}>
                {opt.label}
              </SelectItem>
            ))}
          </Select>
          {/* L3-33: role drives routing — same options as creation wizard. */}
          <Select
            label={t("wizard.roleLabel")}
            selectedKeys={[role]}
            onSelectionChange={(keys) => {
              const next = Array.from(keys)[0];
              if (next != null) setRole(String(next));
            }}
            data-testid="edit-printer-role"
          >
            {[
              { key: "bill", label: t("wizard.roleBillReceipt") },
              { key: "kitchen", label: t("roleLabels.kitchen") || "Kitchen" },
              { key: "bar", label: t("roleLabels.bar") || "Bar" },
            ].map((opt) => (
              <SelectItem key={opt.key} textValue={opt.label}>
                {opt.label}
              </SelectItem>
            ))}
          </Select>
          <Switch isSelected={enabled} onValueChange={setEnabled}>
            {t("edit.enabledLabel")}
          </Switch>
          <p className="text-xs text-ink-500">{t("edit.enabledHint")}</p>
          {error && (
            <div role="alert" className="text-sm text-rose-700">
              {error}
            </div>
          )}
        </ModalBody>
        <ModalFooter>
          <Button variant="light" onPress={onClose}>
            {t("wizard.cancel")}
          </Button>
          <Button
            color="primary"
            onPress={() => void handleSave()}
            isLoading={submitting}
            isDisabled={!name.trim()}
          >
            {t("edit.save")}
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}
