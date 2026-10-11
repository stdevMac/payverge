"use client";

import { useCallback, useState } from "react";
import {
  Button,
  Input,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
  Radio,
  RadioGroup,
  Select,
  SelectItem,
} from "@nextui-org/react";

import { createPrinter } from "@/api/print";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";

interface Props {
  businessId: number;
  open: boolean;
  onClose: () => void;
  onCreated: () => void;
}

export default function AddPrinterWizard({
  businessId,
  open,
  onClose,
  onCreated,
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
  const [role, setRole] = useState("bill");
  const [transport, setTransport] = useState<"browser" | "cloudprnt">("browser");
  const [paperWidth, setPaperWidth] = useState("80");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function handleSave() {
    const trimmedName = name.trim();
    // Whitespace-only names previously created blank rows. Guard here as well as
    // via the disabled Save button so an Enter-key submit can't slip through.
    if (!trimmedName) return;
    setSubmitting(true);
    setError(null);
    try {
      await createPrinter(businessId, {
        name: trimmedName,
        role,
        transport,
        paper_width_mm: Number(paperWidth),
      });
      onCreated();
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
        <ModalHeader>{t("wizard.title")}</ModalHeader>
        <ModalBody className="space-y-4">
          <Input
            label={t("wizard.nameLabel")}
            value={name}
            onChange={(e) => setName(e.target.value)}
            isRequired
          />
          {/* L3-33: kitchen/bar roles must be creatable here — not only via edit. */}
          <Select
            label={t("wizard.roleLabel")}
            selectedKeys={[role]}
            onSelectionChange={(keys) => {
              const next = Array.from(keys)[0];
              if (next != null) setRole(String(next));
            }}
            data-testid="add-printer-role"
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
          <RadioGroup
            label={t("wizard.transportLabel")}
            value={transport}
            onValueChange={(v) => setTransport(v as "browser" | "cloudprnt")}
          >
            <Radio value="browser">{t("wizard.transportBrowser")}</Radio>
            <Radio value="cloudprnt" isDisabled>
              {t("wizard.transportCloudprnt")}
            </Radio>
          </RadioGroup>
          <Select
            label={t("wizard.paperWidthLabel")}
            selectedKeys={[paperWidth]}
            onSelectionChange={(keys) =>
              setPaperWidth(Array.from(keys)[0] as string)
            }
          >
            <SelectItem key="80">{t("wizard.paper80")}</SelectItem>
            <SelectItem key="58">{t("wizard.paper58")}</SelectItem>
          </Select>
          {error && (
            <div role="alert" className="text-danger text-sm">
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
            onPress={handleSave}
            isLoading={submitting}
            isDisabled={!name.trim()}
          >
            {t("wizard.save")}
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
}
