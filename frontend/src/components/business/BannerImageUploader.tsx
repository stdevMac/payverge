"use client";

import { getSafeApiErrorMessage } from "@/utils/apiError";

import React, { useState } from "react";
import { uploadFile } from "@/api/uploads";
import { Button, Image, Progress } from "@nextui-org/react";
import { Upload, X, Plus } from "lucide-react";
import { useToast } from "@/contexts/ToastContext";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";

interface BannerImageUploaderProps {
  bannerImages: string[];
  onBannersChange: (banners: string[]) => void;
  businessId: number;
  maxImages?: number;
  maxSize?: number; // in MB
}

const BannerImageUploader = React.memo(function BannerImageUploader({
  bannerImages,
  onBannersChange,
  businessId,
  maxImages = 3,
  maxSize = 10,
}: BannerImageUploaderProps) {
  const { locale: currentLocale } = useSimpleLocale();

  const tString = (key: string): string => {
    const fullKey = `bannerImageUploader.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  const [uploading, setUploading] = useState<number | null>(null);
  const [uploadProgress, setUploadProgress] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const { showSuccess, showError } = useToast();

  const handleFileUpload = async (file: File, index: number) => {
    if (!file.type.startsWith("image/")) {
      setError(tString("errors.selectImageFile"));
      return;
    }

    if (file.size > maxSize * 1024 * 1024) {
      setError(
        tString("errors.imageSizeLimit").replace(
          "{maxSize}",
          maxSize.toString(),
        ),
      );
      return;
    }

    try {
      setUploading(index);
      setError(null);
      setUploadProgress(0);

      const result = await uploadFile(file, "banner", businessId);
      const imageUrl = result.location;

      const newBanners = [...bannerImages];
      while (newBanners.length <= index) {
        newBanners.push("");
      }
      newBanners[index] = imageUrl;
      onBannersChange(newBanners);

      setUploadProgress(100);

      showSuccess(
        tString("success.uploaded"),
        tString("success.uploadedDescription").replace(
          "{index}",
          (index + 1).toString(),
        ),
        3000,
      );
    } catch (err) {
      const errorMessage =
        getSafeApiErrorMessage(err, tString("errors.uploadFailed"));
      setError(errorMessage);
      showError(tString("errors.uploadFailedTitle"), errorMessage, 5000);
    } finally {
      setUploading(null);
      setUploadProgress(0);
    }
  };

  const handleFileSelect = (index: number) => {
    const input = document.createElement("input");
    input.type = "file";
    input.accept = "image/*";
    input.onchange = (e) => {
      const file = (e.target as HTMLInputElement).files?.[0];
      if (file) {
        void handleFileUpload(file, index);
      }
    };
    input.click();
  };

  const removeBanner = (index: number) => {
    // INT-7: splice the entry out (don't leave an empty-string hole that then
    // serializes into banner_images and shifts downstream banner indexing).
    const newBanners = bannerImages.filter((_, i) => i !== index);
    onBannersChange(newBanners);
  };

  const BannerSlot = ({ index }: { index: number }) => {
    const currentImage = bannerImages[index];
    const isUploading = uploading === index;

    if (currentImage) {
      return (
        <div className="relative group overflow-hidden rounded-2xl border border-warm-200 bg-warm-50 shadow-sm shadow-warm-900/5">
          <div className="relative aspect-[3/1] w-full">
            <Image
              src={currentImage}
              alt={`Banner ${index + 1}`}
              removeWrapper
              className="absolute inset-0 w-full h-full object-cover"
            />

            <Button
              isIconOnly
              size="sm"
              variant="flat"
              onPress={() => removeBanner(index)}
              className="absolute top-2 right-2 z-20 h-7 w-7 min-w-7 bg-white/90 text-rose-600 backdrop-blur-sm hover:bg-rose-50"
              aria-label={tString("buttons.remove") || "Remove"}
            >
              <X className="w-3.5 h-3.5" />
            </Button>

            <div className="absolute inset-0 bg-black/40 opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 transition-opacity flex items-center justify-center z-10">
              <Button
                size="sm"
                variant="flat"
                onPress={() => handleFileSelect(index)}
                startContent={<Upload className="w-3.5 h-3.5" />}
                className="bg-white/95 font-semibold text-ink-950 backdrop-blur-sm"
              >
                {tString("buttons.replace") || "Replace"}
              </Button>
            </div>

            <span className="absolute bottom-2 left-2 z-10 bg-black/60 text-white text-[10px] font-medium px-1.5 py-0.5 rounded">
              {tString("bannerInfo")
                .replace("{index}", (index + 1).toString())
                .replace("{maxSize}", maxSize.toString())}
            </span>
          </div>
        </div>
      );
    }

    return (
      <button
        type="button"
        onClick={() => !isUploading && handleFileSelect(index)}
        disabled={isUploading}
        className={`relative flex aspect-[3/1] flex-col items-center justify-center rounded-2xl border border-dashed px-4 text-center transition-colors ${
          isUploading
            ? "cursor-wait border-warm-300 bg-warm-50/60 opacity-70"
            : "cursor-pointer border-warm-300 bg-warm-50/60 hover:border-brand/50 hover:bg-brand/5"
        }`}
        aria-label={tString("addBanner").replace(
          "{index}",
          (index + 1).toString(),
        )}
      >
        {isUploading ? (
          <>
            <Progress
              isIndeterminate
              size="sm"
              aria-label={tString("status.uploading")}
              classNames={{
                base: "max-w-[80%]",
                indicator: "bg-brand",
              }}
            />
            <span className="mt-2 text-xs font-medium text-ink-600">
              {tString("status.uploading")}
            </span>
          </>
        ) : (
          <>
            <div className="mb-1.5 flex h-9 w-9 items-center justify-center rounded-full border border-warm-200 bg-white shadow-sm shadow-warm-900/5">
              <Plus className="h-4 w-4 text-brand" strokeWidth={1.75} />
            </div>
            <p className="text-sm font-semibold leading-tight text-ink-700">
              {tString("addBanner").replace(
                "{index}",
                (index + 1).toString(),
              )}
            </p>
            <p className="mt-0.5 text-[11px] text-ink-500">
              {tString("hint") || `1200×400 · PNG, JPG, WebP up to ${maxSize}MB`}
            </p>
          </>
        )}
      </button>
    );
  };

  return (
    <div className="space-y-3">
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        {Array.from({ length: maxImages }, (_, index) => (
          <BannerSlot key={index} index={index} />
        ))}
      </div>

      {error && (
        <div className="flex items-center justify-between gap-2 rounded-xl border border-rose-200 bg-rose-50 px-3 py-2">
          <p className="text-xs font-medium text-rose-700">{error}</p>
          <Button
            isIconOnly
            size="sm"
            variant="light"
            onPress={() => setError(null)}
            className="h-5 w-5 min-w-0 text-rose-700"
            aria-label={tString("buttons.dismiss")}
          >
            <X className="w-3 h-3" />
          </Button>
        </div>
      )}
    </div>
  );
});

export default BannerImageUploader;
