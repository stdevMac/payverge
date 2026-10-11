"use client";

import React, { useCallback, useRef, useState } from "react";
import { Button, Image, Input, Spinner } from "@nextui-org/react";
import { ImageIcon, Plus, X } from "lucide-react";
import { BusinessGalleryImage } from "@/api/business";
import { uploadFile } from "@/api/uploads";
import { useToast } from "@/contexts/ToastContext";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";
import { getSafeApiErrorMessage } from "@/utils/apiError";

interface GalleryImageUploaderProps {
  businessId: number;
  images: BusinessGalleryImage[];
  onImagesChange: (images: BusinessGalleryImage[]) => void;
}

const MAX_FILE_MB = 5;
/** Max gallery images per business (UI + upload guard). */
const MAX_GALLERY_IMAGES = 30;
/** Concurrent uploads so a multi-file drop doesn't serial-stall. */
const UPLOAD_CONCURRENCY = 3;

export default function GalleryImageUploader({
  businessId,
  images,
  onImagesChange,
}: GalleryImageUploaderProps) {
  const [isUploading, setIsUploading] = useState(false);
  const [isDragOver, setIsDragOver] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const { showSuccess, showError } = useToast();
  const { locale: currentLocale } = useSimpleLocale();
  const tString = useCallback((key: string): string => {
    const result = getTranslation(`imageUpload.${key}`, currentLocale);
    return Array.isArray(result) ? result[0] || key : (result as string);
  }, [currentLocale]);
  const tStringGallery = useCallback((key: string): string => {
    const result = getTranslation(`imageUpload.gallery.${key}`, currentLocale);
    return Array.isArray(result) ? result[0] || key : (result as string);
  }, [currentLocale]);

  const handleFileUpload = useCallback(
    async (files: FileList | File[]) => {
      const fileArr = Array.from(files);
      if (fileArr.length === 0) return;

      const slotsLeft = Math.max(0, MAX_GALLERY_IMAGES - images.length);
      if (slotsLeft === 0) {
        showError(
          tStringGallery("maxCountTitle") || "Gallery full",
          (tStringGallery("maxCountBody") || "You can upload up to {max} images.").replace(
            "{max}",
            String(MAX_GALLERY_IMAGES),
          ),
        );
        return;
      }
      const accepted = fileArr.slice(0, slotsLeft);
      if (fileArr.length > slotsLeft) {
        showError(
          tStringGallery("maxCountTitle") || "Gallery full",
          (tStringGallery("truncatedToMax") || "Only {count} more image(s) fit (max {max}).").replace(
            "{count}",
            String(slotsLeft),
          ).replace("{max}", String(MAX_GALLERY_IMAGES)),
        );
      }

      setIsUploading(true);
      const newRecords: BusinessGalleryImage[] = [];
      let uploadFailed = false;
      let lastError: unknown = null;

      try {
        // Validate first, then upload with bounded concurrency.
        const validFiles: File[] = [];
        for (const file of accepted) {
          if (!file.type.startsWith("image/")) {
            showError(tString("errors.selectImageFile"), file.name);
            continue;
          }
          if (file.size > MAX_FILE_MB * 1024 * 1024) {
            showError(
              tString("errors.imageSizeLimit").replace(
                "{maxSize}",
                String(MAX_FILE_MB),
              ),
              file.name,
            );
            continue;
          }
          validFiles.push(file);
        }

        // Chunked concurrency: up to UPLOAD_CONCURRENCY uploads in flight.
        // Use allSettled — a failed sibling in a chunk must NOT discard the
        // uploads that already succeeded (F12: those images live in S3, so
        // dropping them would lose them from the UI).
        for (let start = 0; start < validFiles.length; start += UPLOAD_CONCURRENCY) {
          const chunk = validFiles.slice(start, start + UPLOAD_CONCURRENCY);
          const settled = await Promise.allSettled(
            chunk.map(async (file) => {
              const uploadResult = await uploadFile(file, "gallery", businessId);
              return uploadResult.location;
            }),
          );
          for (const outcome of settled) {
            if (outcome.status === "fulfilled") {
              newRecords.push({
                id: -(images.length + newRecords.length + 1),
                business_id: businessId,
                image_url: outcome.value,
                caption: "",
                display_order: images.length + newRecords.length,
                is_active: true,
                created_at: "",
                updated_at: "",
              } as BusinessGalleryImage);
            } else {
              uploadFailed = true;
              lastError = outcome.reason;
            }
          }
        }

        if (uploadFailed) {
          showError(
            tString("errors.uploadFailedTitle"),
            getSafeApiErrorMessage(lastError, tString("errors.uploadFailed")),
          );
        }
        if (newRecords.length > 0) {
          showSuccess(
            tString("success.uploaded"),
            tStringGallery("uploadedCount").replace(
              "{count}",
              String(newRecords.length),
            ),
          );
        }
      } catch (error) {
        showError(
          tString("errors.uploadFailedTitle"),
          getSafeApiErrorMessage(error, tString("errors.uploadFailed")),
        );
      } finally {
        // Apply whatever uploaded successfully even if a later file in the
        // batch threw — those images already live in S3, so dropping them
        // here would lose them from the UI and force the operator to re-add.
        if (newRecords.length > 0) {
          onImagesChange([...images, ...newRecords]);
        }
        setIsUploading(false);
        if (fileInputRef.current) fileInputRef.current.value = "";
      }
    },
    [
      businessId,
      images,
      onImagesChange,
      showError,
      showSuccess,
      tString,
      tStringGallery,
    ],
  );

  const handleDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault();
      setIsDragOver(false);
      if (isUploading) return;
      const files = e.dataTransfer.files;
      if (files?.length) void handleFileUpload(files);
    },
    [handleFileUpload, isUploading],
  );

  const handleFileInput = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      if (e.target.files) {
        void handleFileUpload(e.target.files);
      }
    },
    [handleFileUpload],
  );

  const updateCaption = useCallback(
    (index: number, caption: string) => {
      const updated = images.map((img, i) =>
        i === index ? { ...img, caption } : img,
      );
      onImagesChange(updated);
    },
    [images, onImagesChange],
  );

  const removeImage = useCallback(
    (index: number) => {
      const updated = images.filter((_, i) => i !== index);
      onImagesChange(updated);
    },
    [images, onImagesChange],
  );

  return (
    <div className="space-y-3">
      <input
        ref={fileInputRef}
        type="file"
        multiple
        accept="image/*"
        onChange={handleFileInput}
        className="hidden"
      />

      <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-3">
        {images.map((image, index) => (
          <div
            key={image.id ?? `${image.image_url}-${index}`}
            style={{ aspectRatio: "1 / 1" }}
            className="relative group overflow-hidden rounded-2xl border border-warm-200/80 bg-warm-50 shadow-sm shadow-warm-900/5 transition hover:-translate-y-0.5 hover:border-brand/30 hover:shadow-md hover:shadow-brand/10"
          >
            <Image
              src={image.image_url}
              alt={image.caption || `Gallery image ${index + 1}`}
              removeWrapper
              className="absolute inset-0 w-full h-full object-cover transition duration-300 group-hover:scale-105"
            />
            <Button
              isIconOnly
              size="sm"
              variant="flat"
              className="absolute top-1.5 right-1.5 z-20 h-7 w-7 min-w-7 rounded-xl border border-rose-200 bg-white/90 text-rose-600 shadow-sm shadow-rose-900/10 backdrop-blur-sm"
              onPress={() => removeImage(index)}
              aria-label={tString("buttons.remove")}
            >
              <X className="w-3.5 h-3.5" />
            </Button>
            <div className="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/70 to-transparent p-2 z-10">
              <Input
                placeholder={tStringGallery("captionPlaceholder")}
                value={image.caption || ""}
                onValueChange={(value) => updateCaption(index, value)}
                size="sm"
                variant="flat"
                classNames={{
                  inputWrapper:
                    "rounded-xl border border-white/60 bg-white/90 backdrop-blur-sm h-7 min-h-7 shadow-none",
                  input: "text-xs text-ink-900 placeholder:text-warm-600",
                }}
              />
            </div>
          </div>
        ))}

        <button
          type="button"
          onClick={() => !isUploading && fileInputRef.current?.click()}
          onDragOver={(e) => {
            e.preventDefault();
            setIsDragOver(true);
          }}
          onDragLeave={() => setIsDragOver(false)}
          onDrop={handleDrop}
          disabled={isUploading}
          style={{ aspectRatio: "1 / 1" }}
          className={`relative rounded-2xl border-2 border-dashed flex flex-col items-center justify-center text-center px-2 shadow-sm shadow-warm-900/5 transition ${
            isDragOver
              ? "border-brand bg-brand/10"
              : "border-warm-300 bg-warm-50/70 hover:-translate-y-0.5 hover:border-brand/50 hover:bg-brand/5 hover:shadow-md hover:shadow-brand/10"
          } ${isUploading ? "opacity-60 cursor-wait" : "cursor-pointer"}`}
          aria-label={tString("buttons.addImage")}
        >
          {isUploading ? (
            <>
              <Spinner size="sm" />
              <span className="text-[11px] text-ink-700 mt-2">
                {tString("status.uploading")}
              </span>
            </>
          ) : (
            <>
              <div className="w-8 h-8 rounded-full bg-white border border-brand/20 flex items-center justify-center mb-1.5 shadow-sm shadow-brand/10">
                {images.length === 0 ? (
                  <ImageIcon className="w-4 h-4 text-brand" strokeWidth={1.75} />
                ) : (
                  <Plus className="w-4 h-4 text-brand" strokeWidth={1.75} />
                )}
              </div>
              <span className="text-xs font-semibold text-ink-800 leading-tight">
                {images.length === 0
                  ? tString("buttons.addFirstImage")
                  : tString("buttons.addAnother")}
              </span>
              <span className="text-[10px] text-warm-500 mt-0.5">
                {tString("labels.orDrop")}
              </span>
            </>
          )}
        </button>
      </div>

      <p className="text-xs text-warm-700">
        {tStringGallery("hint")}
      </p>
      <p className="text-xs text-warm-600">
        {tStringGallery("autoTranslateNotice")}
      </p>
    </div>
  );
}
