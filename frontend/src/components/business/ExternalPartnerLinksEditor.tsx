import React, { useEffect, useRef, useState } from "react";
import Image from "next/image";
import { canOptimizeImageSrc } from "@/config/imageOrigins";
import {
  Button,
  Card,
  CardBody,
  Input,
  Select,
  SelectItem,
} from "@nextui-org/react";
import { Plus, Trash2, Upload } from "lucide-react";
import { uploadFile } from "@/api/uploads";
import {
  PROVIDER_CATALOG,
  PROVIDER_CATALOG_BY_KEY,
  type ProviderCatalogItem,
} from "@/constants/providerCatalog";

export interface ExternalPartnerLink {
  name: string;
  url: string;
  provider_key?: string;
  icon_url?: string;
}

interface ExternalPartnerLinksEditorLabels {
  title: string;
  description: string;
  addProvider: string;
  nameLabel: string;
  urlLabel: string;
  iconSourceLabel: string;
  sourcePredefined: string;
  sourceCustom: string;
  providerLabel: string;
  iconUrlLabel: string;
  uploadIcon: string;
  removeProvider: string;
  maxReached: string;
  uploadError?: string;
  imageOnlyError?: string;
}

interface ExternalPartnerLinksEditorProps {
  businessId: number;
  links: ExternalPartnerLink[];
  onChange: (links: ExternalPartnerLink[]) => void;
  labels: ExternalPartnerLinksEditorLabels;
  /**
   * Providers offered in the predefined picker. Defaults to the full catalog;
   * the delivery surface passes DELIVERY_PROVIDER_CATALOG so reservation
   * platforms (OpenTable, Resy) are not selectable as couriers (#829).
   */
  catalog?: ProviderCatalogItem[];
}

const MAX_PARTNER_LINKS = 5;

const ExternalPartnerLinksEditor = React.memo(
  function ExternalPartnerLinksEditor({
    businessId,
    links,
    onChange,
    labels,
    catalog = PROVIDER_CATALOG,
  }: ExternalPartnerLinksEditorProps) {
    const fileInputRef = useRef<HTMLInputElement>(null);
    const [uploadTargetIndex, setUploadTargetIndex] = useState<number | null>(
      null,
    );
    const [uploadingIndex, setUploadingIndex] = useState<number | null>(null);
    const [uploadError, setUploadError] = useState<string>("");

    // Stable per-row keys so React reconciles rows by identity, not array index.
    // Keying by index made focus / in-progress IME / the file-upload target jump
    // to the wrong row when a middle row was deleted (indices shift). We keep a
    // parallel id list mutated in lockstep with add/remove and reconciled to the
    // incoming links length (initial load / external changes).
    const rowIdSeq = useRef(0);
    const nextRowId = () => `row-${rowIdSeq.current++}`;
    const [rowIds, setRowIds] = useState<string[]>(() =>
      links.map(() => nextRowId()),
    );

    useEffect(() => {
      setRowIds((current) => {
        if (current.length === links.length) return current;
        if (current.length < links.length) {
          const added = Array.from(
            { length: links.length - current.length },
            () => nextRowId(),
          );
          return [...current, ...added];
        }
        return current.slice(0, links.length);
      });
    }, [links.length]);

    const updateLink = (index: number, patch: Partial<ExternalPartnerLink>) => {
      onChange(
        links.map((link, i) => (i === index ? { ...link, ...patch } : link)),
      );
    };

    const getMode = (link: ExternalPartnerLink): "predefined" | "custom" =>
      link.icon_url ? "custom" : "predefined";

    const addLink = () => {
      if (links.length >= MAX_PARTNER_LINKS) return;
      setRowIds((current) => [...current, nextRowId()]);
      onChange([
        ...links,
        {
          name: "",
          url: "",
          provider_key: catalog[0]?.key || "",
        },
      ]);
    };

    const removeLink = (index: number) => {
      // Drop the id at the SAME index so the remaining rows keep their identity
      // (and their focus / upload-target / IME state) instead of shifting.
      setRowIds((current) => current.filter((_, i) => i !== index));
      onChange(links.filter((_, i) => i !== index));
    };

    const handleModeChange = (index: number, mode: "predefined" | "custom") => {
      if (mode === "predefined") {
        updateLink(index, {
          provider_key:
            links[index].provider_key || catalog[0]?.key || "",
          icon_url: undefined,
        });
        return;
      }

      updateLink(index, {
        provider_key: undefined,
      });
    };

    const triggerUpload = (index: number) => {
      setUploadError("");
      setUploadTargetIndex(index);
      fileInputRef.current?.click();
    };

    const onIconFileSelected = async (
      event: React.ChangeEvent<HTMLInputElement>,
    ) => {
      const file = event.target.files?.[0];
      if (!file || uploadTargetIndex === null) return;

      if (!file.type.startsWith("image/")) {
        setUploadError(labels.imageOnlyError ?? "Please choose an image file.");
        return;
      }

      try {
        setUploadingIndex(uploadTargetIndex);
        const result = await uploadFile(file, "partner-icons", businessId);
        const iconUrl = result.location;
        updateLink(uploadTargetIndex, {
          icon_url: iconUrl,
          provider_key: undefined,
        });
        setUploadError("");
      } catch (error) {
        console.error("Failed to upload partner icon:", error);
        setUploadError(labels.uploadError ?? "Failed to upload icon.");
      } finally {
        setUploadingIndex(null);
        setUploadTargetIndex(null);
        if (fileInputRef.current) {
          fileInputRef.current.value = "";
        }
      }
    };

    return (
      <Card className="border border-warm-200 bg-white/95 shadow-sm shadow-warm-900/5">
        <CardBody className="space-y-4">
          <div>
            <h4 className="text-sm font-semibold text-ink-900">
              {labels.title}
            </h4>
            <p className="mt-1 text-xs leading-5 text-ink-600">
              {labels.description}
            </p>
          </div>

          <input
            ref={fileInputRef}
            type="file"
            accept="image/*"
            className="hidden"
            onChange={onIconFileSelected}
          />

          <div className="space-y-4">
            {links.map((link, index) => {
              const mode = getMode(link);
              // Links saved without a provider_key (older rows, API imports)
              // infer it from the name/URL instead of silently defaulting to
              // the first catalog entry — "Uber Eats" rendered a Talabat icon.
              const inferredProviderKey = catalog.find(
                (p) =>
                  link.name?.toLowerCase() === p.name.toLowerCase() ||
                  (link.url ?? "").toLowerCase().includes(p.key),
              )?.key;
              const selectedProviderKey =
                link.provider_key && PROVIDER_CATALOG_BY_KEY[link.provider_key]
                  ? link.provider_key
                  : inferredProviderKey || catalog[0]?.key || "";
              // A saved key outside this surface's catalog (e.g. a legacy
              // OpenTable delivery row, #829) stays honestly labeled in its
              // own row's options, but new rows never offer it.
              const rowCatalog =
                selectedProviderKey &&
                !catalog.some((p) => p.key === selectedProviderKey) &&
                PROVIDER_CATALOG_BY_KEY[selectedProviderKey]
                  ? [...catalog, PROVIDER_CATALOG_BY_KEY[selectedProviderKey]]
                  : catalog;
              const providerIcon = selectedProviderKey
                ? PROVIDER_CATALOG_BY_KEY[selectedProviderKey]?.iconPath
                : "";
              const previewIcon = link.icon_url || providerIcon;

              return (
                <div
                  key={rowIds[index] ?? `row-fallback-${index}`}
                  className="space-y-3 rounded-xl border border-warm-200 bg-warm-50/35 p-4 transition-colors hover:border-brand/30 hover:bg-white"
                >
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                    <Input
                      label={labels.nameLabel}
                      value={link.name}
                      onValueChange={(value) =>
                        updateLink(index, { name: value })
                      }
                      variant="bordered"
                    />
                    <Input
                      label={labels.urlLabel}
                      value={link.url}
                      onValueChange={(value) =>
                        updateLink(index, { url: value })
                      }
                      variant="bordered"
                    />
                  </div>

                  <div className="grid grid-cols-1 md:grid-cols-3 gap-3 items-end">
                    <Select
                      label={labels.iconSourceLabel}
                      selectedKeys={[mode]}
                      onSelectionChange={(keys) => {
                        const value = Array.from(keys)[0] as
                          | "predefined"
                          | "custom";
                        if (value) {
                          handleModeChange(index, value);
                        }
                      }}
                      variant="bordered"
                    >
                      <SelectItem
                        key="predefined"
                        value="predefined"
                        textValue={labels.sourcePredefined}
                      >
                        {labels.sourcePredefined}
                      </SelectItem>
                      <SelectItem
                        key="custom"
                        value="custom"
                        textValue={labels.sourceCustom}
                      >
                        {labels.sourceCustom}
                      </SelectItem>
                    </Select>

                    {mode === "predefined" ? (
                      <Select
                        label={labels.providerLabel}
                        selectedKeys={[selectedProviderKey]}
                        onSelectionChange={(keys) => {
                          const value = Array.from(keys)[0] as string;
                          updateLink(index, {
                            provider_key: value,
                            icon_url: undefined,
                          });
                        }}
                        variant="bordered"
                      >
                        {rowCatalog.map((provider) => (
                          <SelectItem
                            key={provider.key}
                            value={provider.key}
                            textValue={provider.name}
                          >
                            {provider.name}
                          </SelectItem>
                        ))}
                      </Select>
                    ) : (
                      <Input
                        label={labels.iconUrlLabel}
                        value={link.icon_url || ""}
                        onValueChange={(value) =>
                          updateLink(index, {
                            icon_url: value,
                            provider_key: undefined,
                          })
                        }
                        variant="bordered"
                      />
                    )}

                    <div className="flex items-center gap-2">
                      {mode === "custom" && (
                        <Button
                          variant="flat"
                          className="bg-brand/10 text-brand-dark"
                          startContent={<Upload className="w-4 h-4" />}
                          onPress={() => triggerUpload(index)}
                          isLoading={uploadingIndex === index}
                        >
                          {labels.uploadIcon}
                        </Button>
                      )}
                      <Button
                        color="danger"
                        variant="light"
                        className="text-rose-700 hover:bg-rose-50"
                        startContent={<Trash2 className="w-4 h-4" />}
                        onPress={() => removeLink(index)}
                      >
                        {labels.removeProvider}
                      </Button>
                    </div>
                  </div>

                  {previewIcon && (
                    <div className="flex items-center gap-2 pt-1">
                      <Image
                        src={previewIcon}
                        unoptimized={!canOptimizeImageSrc(previewIcon)}
                        alt={link.name || "Provider icon"}
                        width={32}
                        height={32}
                        className="rounded-md border border-warm-200 bg-white object-cover shadow-sm shadow-warm-900/5"
                      />
                      <span className="text-xs text-ink-600">
                        {link.name || "-"}
                      </span>
                    </div>
                  )}
                </div>
              );
            })}
          </div>

          {uploadError && (
            <p className="rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-xs font-medium text-rose-700">
              {uploadError}
            </p>
          )}

          <div className="flex items-center justify-between gap-3">
            <Button
              startContent={<Plus className="w-4 h-4" />}
              onPress={addLink}
              isDisabled={links.length >= MAX_PARTNER_LINKS}
              className="bg-brand text-white shadow-sm shadow-brand/20 transition-transform hover:bg-brand-dark active:scale-[0.98]"
            >
              {labels.addProvider}
            </Button>
            <p className="text-xs text-ink-500">
              {labels.maxReached.replace("{max}", String(MAX_PARTNER_LINKS))}
            </p>
          </div>
        </CardBody>
      </Card>
    );
  },
);

export default ExternalPartnerLinksEditor;
