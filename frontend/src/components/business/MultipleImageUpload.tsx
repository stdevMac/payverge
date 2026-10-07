"use client";

import { getSafeApiErrorMessage } from "@/utils/apiError";

import React, { useRef, useState } from "react";
import {
    Button,
    Image,
    Modal,
    ModalBody,
    ModalContent,
    ModalFooter,
    ModalHeader,
    Progress,
    useDisclosure,
} from "@nextui-org/react";
import { Eye, ImageIcon, Trash2, Upload, X } from "lucide-react";
import { uploadFile } from "@/api/uploads";
import {
    getTranslation,
    useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";

interface MultipleImageUploadProps {
    images: string[];
    onImagesChange: (images: string[]) => void;
    maxImages?: number;
    businessId: number;
}

const MAX_FILE_MB = 5;

export default function MultipleImageUpload({
    images,
    onImagesChange,
    maxImages = 5,
    businessId,
}: MultipleImageUploadProps) {
    const { locale } = useSimpleLocale();
    const t = (key: string): string => {
        const result = getTranslation(`imageUpload.${key}`, locale);
        return Array.isArray(result) ? result[0] || key : (result as string);
    };

    const fileInputRef = useRef<HTMLInputElement>(null);
    const [uploading, setUploading] = useState(false);
    const [uploadProgress, setUploadProgress] = useState(0);
    const [error, setError] = useState<string | null>(null);
    const [isDragOver, setIsDragOver] = useState(false);
    const [previewIndex, setPreviewIndex] = useState<number | null>(null);
    const {
        isOpen: isPreviewOpen,
        onOpen: onPreviewOpen,
        onClose: onPreviewClose,
    } = useDisclosure();

    const canAddMore = images.length < maxImages;

    const validateAndUpload = async (file: File) => {
        if (!file) return;

        if (!file.type.startsWith("image/")) {
            setError(t("errors.selectImageFile"));
            return;
        }
        if (file.size > MAX_FILE_MB * 1024 * 1024) {
            setError(
                t("errors.imageSizeLimit").replace(
                    "{maxSize}",
                    String(MAX_FILE_MB),
                ),
            );
            return;
        }
        if (!businessId) {
            setError(t("errors.businessIdRequired"));
            return;
        }

        try {
            setUploading(true);
            setError(null);
            setUploadProgress(0);
            const result = await uploadFile(file, "menu-item", businessId);
            const url = result.location;
            if (url) {
                onImagesChange([...images, url]);
            }
            setUploadProgress(100);
        } catch (err) {
            setError(
                getSafeApiErrorMessage(err, t("errors.uploadFailed")),
            );
        } finally {
            setUploading(false);
            if (fileInputRef.current) fileInputRef.current.value = "";
        }
    };

    const openFilePicker = () => {
        if (!canAddMore || uploading) return;
        fileInputRef.current?.click();
    };

    const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        const file = e.target.files?.[0];
        if (file) void validateAndUpload(file);
    };

    const handleDrop = (e: React.DragEvent) => {
        e.preventDefault();
        setIsDragOver(false);
        if (!canAddMore || uploading) return;
        const file = e.dataTransfer.files?.[0];
        if (file) void validateAndUpload(file);
    };

    const handleRemove = (index: number) => {
        onImagesChange(images.filter((_, i) => i !== index));
    };

    const previewImageUrl =
        previewIndex !== null ? images[previewIndex] : null;

    return (
        <div className="space-y-3">
            <div className="flex items-center justify-between">
                <div>
                    <h4 className="text-sm font-semibold text-ink-950 inline-flex items-center gap-1.5">
                        <Upload className="w-4 h-4 text-ink-600" />
                        {t("labels.images") || "Images"}{" "}
                        <span className="text-ink-500 font-medium">
                            ({images.length}/{maxImages})
                        </span>
                    </h4>
                    <p className="text-xs text-ink-600 mt-0.5">
                        {t("labels.dragOrClick") ||
                            "Drag and drop or click to upload. PNG, JPG, WebP up to 5MB."}
                    </p>
                </div>
            </div>

            <input
                ref={fileInputRef}
                type="file"
                accept="image/*"
                onChange={handleFileChange}
                className="hidden"
            />

            <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-3">
                {images.map((url, index) => (
                    <div
                        key={`${url}-${index}`}
                        // Inline aspectRatio: Tailwind's aspect-square isn't
                        // surviving build here (computed: auto), so the grid
                        // would collapse the row to content height. Inline
                        // style is bulletproof.
                        style={{ aspectRatio: "1 / 1" }}
                        className="relative group overflow-hidden rounded-2xl border border-warm-200 bg-warm-50 shadow-sm shadow-warm-900/5"
                    >
                        <Image
                            src={url}
                            alt={`Image ${index + 1}`}
                            removeWrapper
                            className="absolute inset-0 w-full h-full object-cover"
                        />

                        {index === 0 && (
                            <span className="absolute top-1.5 left-1.5 z-20 rounded-full border border-warm-200/80 bg-white/95 px-1.5 py-0.5 text-[10px] font-semibold text-ink-700 shadow-sm">
                                {t("labels.primary") || "Primary"}
                            </span>
                        )}

                        <div className="absolute inset-0 bg-black/40 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center gap-2 z-10">
                            <Button
                                isIconOnly
                                size="sm"
                                variant="flat"
                                className="h-7 w-7 min-w-7 bg-white/90 text-ink-950 backdrop-blur-sm"
                                onPress={() => {
                                    setPreviewIndex(index);
                                    onPreviewOpen();
                                }}
                                aria-label={t("buttons.preview") || "Preview"}
                            >
                                <Eye className="w-3.5 h-3.5" />
                            </Button>
                            <Button
                                isIconOnly
                                size="sm"
                                variant="flat"
                                className="h-7 w-7 min-w-7 bg-white/90 text-rose-600 backdrop-blur-sm"
                                onPress={() => handleRemove(index)}
                                aria-label={t("buttons.remove") || "Remove"}
                            >
                                <X className="w-3.5 h-3.5" />
                            </Button>
                        </div>
                    </div>
                ))}

                {canAddMore && (
                    <button
                        type="button"
                        onClick={openFilePicker}
                        onDragOver={(e) => {
                            e.preventDefault();
                            setIsDragOver(true);
                        }}
                        onDragLeave={() => setIsDragOver(false)}
                        onDrop={handleDrop}
                        disabled={uploading}
                        style={{ aspectRatio: "1 / 1" }}
                        className={`relative rounded-2xl border-2 border-dashed flex flex-col items-center justify-center text-center px-2 transition-colors ${
                            isDragOver
                                ? "border-brand bg-brand/5"
                                : "border-warm-300 bg-warm-50/70 hover:border-brand/50 hover:bg-brand/5"
                        } ${uploading ? "opacity-60 cursor-wait" : "cursor-pointer"}`}
                        aria-label={t("buttons.addImage") || "Add image"}
                    >
                        {uploading ? (
                            <>
                                <Progress
                                    isIndeterminate
                                    size="sm"
                                    aria-label={t("status.uploading") || "Uploading"}
                                    classNames={{
                                        base: "max-w-[80%]",
                                        indicator: "bg-brand",
                                    }}
                                />
                                <span className="text-[11px] text-ink-600 mt-2">
                                    {t("status.uploading") || "Uploading…"}
                                </span>
                            </>
                        ) : (
                            <>
                                <div className="w-8 h-8 rounded-full bg-white border border-warm-200 flex items-center justify-center mb-1.5 group-hover:border-brand/40 transition-colors shadow-sm shadow-warm-900/5">
                                    <ImageIcon className="w-4 h-4 text-ink-600" strokeWidth={1.75} />
                                </div>
                                <span className="text-xs font-semibold text-ink-700 leading-tight">
                                    {images.length === 0
                                        ? t("buttons.addFirstImage") || "Add image"
                                        : t("buttons.addAnother") || "Add another"}
                                </span>
                                <span className="text-[10px] text-ink-500 mt-0.5">
                                    {t("labels.orDrop") || "or drop here"}
                                </span>
                            </>
                        )}
                    </button>
                )}
            </div>

            {error && (
                <div className="rounded-2xl border border-rose-200 bg-rose-50 px-3 py-2 flex items-center justify-between gap-2">
                    <p className="text-xs text-rose-700">{error}</p>
                    <Button
                        isIconOnly
                        size="sm"
                        variant="light"
                        onPress={() => setError(null)}
                        className="min-w-0 w-5 h-5 text-rose-700"
                        aria-label={t("buttons.dismiss") || "Dismiss"}
                    >
                        <X className="w-3 h-3" />
                    </Button>
                </div>
            )}

            <Modal
                isOpen={isPreviewOpen}
                onClose={() => {
                    onPreviewClose();
                    setPreviewIndex(null);
                }}
                size="2xl"
            >
                <ModalContent className="overflow-hidden rounded-3xl border border-warm-200 bg-white shadow-2xl shadow-warm-900/15">
                    <ModalHeader className="border-b border-warm-200/80 bg-warm-50/70 px-6 py-5 text-lg font-semibold text-ink-950">
                        {t("labels.preview") || "Preview"}
                    </ModalHeader>
                    <ModalBody className="px-6 py-5">
                        {previewImageUrl && (
                            <div className="relative w-full min-h-[400px] flex items-center justify-center bg-warm-50 rounded-2xl overflow-hidden border border-warm-200/80">
                                <Image
                                    src={previewImageUrl}
                                    alt="Preview"
                                    removeWrapper
                                    className="max-w-full max-h-[70vh] w-auto h-auto object-contain"
                                />
                            </div>
                        )}
                    </ModalBody>
                    <ModalFooter className="gap-2 border-t border-warm-200/80 bg-warm-50/70 px-6 py-4">
                        {previewIndex !== null && (
                            <Button
                                variant="flat"
                                startContent={<Trash2 className="w-3.5 h-3.5" />}
                                onPress={() => {
                                    handleRemove(previewIndex);
                                    onPreviewClose();
                                    setPreviewIndex(null);
                                }}
                                className="border border-rose-200 bg-rose-50 font-semibold text-rose-700 hover:bg-rose-100"
                            >
                                {t("buttons.remove") || "Remove"}
                            </Button>
                        )}
                        <Button
                            onPress={() => {
                                onPreviewClose();
                                setPreviewIndex(null);
                            }}
                            className="font-semibold text-ink-700 hover:bg-white"
                        >
                            {t("buttons.close") || "Close"}
                        </Button>
                    </ModalFooter>
                </ModalContent>
            </Modal>
        </div>
    );
}
