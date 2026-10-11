"use client";

import { getSafeApiErrorMessage } from "@/utils/apiError";

import React, { useState, useRef } from "react";
import { uploadFile } from "@/api/uploads";
import { Button, Card, CardBody, Progress, Image } from "@nextui-org/react";
import { ImagePlus } from "lucide-react";
import { useToast } from "@/contexts/ToastContext";
import {
  getTranslation,
  useSimpleLocale,
} from "@/i18n/SimpleTranslationProvider";

interface SimpleImageUploadProps {
  onImageUploaded: (imageUrl: string) => void;
  currentImage?: string;
  isLoading?: boolean;
  businessId?: number;
  type?: "business-logo" | "menu-item" | "offer" | "bundle" | "banner" | "gallery";
  title?: string;
  description?: string;
  maxSize?: number; // in MB
}

export default function SimpleImageUpload({
  onImageUploaded,
  currentImage,
  isLoading = false,
  businessId,
  type = "business-logo",
  title,
  description,
  maxSize = 5,
}: SimpleImageUploadProps) {
  const [uploading, setUploading] = useState(false);
  const [uploadProgress, setUploadProgress] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const { showSuccess, showError } = useToast();
  const { locale: currentLocale } = useSimpleLocale();

  const tString = (
    key: string,
    params?: Record<string, string | number>,
  ): string => {
    const result = getTranslation(`imageUpload.${key}`, currentLocale, params);
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  const handleFileSelect = () => {
    fileInputRef.current?.click();
  };

  const handleFileChange = async (
    event: React.ChangeEvent<HTMLInputElement>,
  ) => {
    const file = event.target.files?.[0];
    if (!file) return;

    // Validate file type
    if (!file.type.startsWith("image/")) {
      setError(tString("errors.selectImageFile"));
      return;
    }

    // Validate file size
    if (file.size > maxSize * 1024 * 1024) {
      setError(tString("errors.imageSizeLimit", { maxSize }));
      return;
    }

    // Validate business ID for protected uploads
    if (!businessId) {
      setError(tString("errors.businessIdRequired"));
      return;
    }

    try {
      setUploading(true);
      setError(null);
      setUploadProgress(0);

      // Upload to backend S3 endpoint with proper type
      const result = await uploadFile(file, type, businessId);
      const imageUrl = result.location;
      onImageUploaded(imageUrl);
      setUploadProgress(100);

      // Show success toast
      showSuccess(
        tString("success.uploaded"),
        tString("success.uploadedDescription", {
          title: title || tString("labels.defaultImage"),
        }),
        3000,
      );
    } catch (err) {
      const errorMessage =
        getSafeApiErrorMessage(err, tString("errors.uploadFailed"));
      setError(errorMessage);

      // Show error toast
      showError(tString("errors.uploadFailedTitle"), errorMessage, 5000);
    } finally {
      setUploading(false);
      if (fileInputRef.current) {
        fileInputRef.current.value = "";
      }
    }
  };

  const handleRemoveImage = () => {
    onImageUploaded("");
  };

  return (
    <div className="space-y-3">
      {title && (
        <div>
          <h4 className="text-sm font-semibold text-ink-800">{title}</h4>
          {description && (
            <p className="mt-1 text-xs text-ink-500">{description}</p>
          )}
        </div>
      )}

      <input
        ref={fileInputRef}
        type="file"
        accept="image/*"
        onChange={handleFileChange}
        className="hidden"
      />

      {currentImage ? (
        <Card className="rounded-3xl border border-warm-200 bg-white shadow-sm shadow-warm-900/5">
          <CardBody className="p-4">
            <div className="flex items-center gap-4">
              <Image
                src={currentImage}
                alt={title || tString("labels.uploadedImage")}
                width={120}
                height={120}
                className="rounded-2xl border border-warm-200 object-cover"
              />
              <div className="min-w-0 flex-1">
                <p className="text-sm font-semibold text-ink-700">
                  {tString("labels.currentImage")}
                </p>
                <p className="mt-1 text-xs text-ink-500">
                  {tString("labels.maxSize")}: {maxSize}MB •{" "}
                  {tString("labels.formats")}: JPG, PNG, WebP
                </p>
                <div className="mt-3 flex flex-wrap gap-2">
                  <Button
                    size="sm"
                    variant="flat"
                    onPress={handleFileSelect}
                    isDisabled={uploading || isLoading}
                    className="bg-brand/10 font-semibold text-brand-dark hover:bg-brand/15"
                  >
                    {tString("buttons.changeImage")}
                  </Button>
                  <Button
                    size="sm"
                    variant="light"
                    onPress={handleRemoveImage}
                    isDisabled={uploading || isLoading}
                    className="font-semibold text-rose-600 hover:bg-rose-50"
                  >
                    {tString("buttons.remove")}
                  </Button>
                </div>
              </div>
            </div>
          </CardBody>
        </Card>
      ) : (
        <Card className="rounded-3xl border border-dashed border-warm-300 bg-warm-50/60 shadow-none transition-colors hover:border-brand/50 hover:bg-brand/5">
          <CardBody className="text-center py-6">
            <div className="space-y-3 flex flex-col items-center justify-center">
              <div className="flex h-12 w-12 items-center justify-center rounded-2xl border border-brand/15 bg-brand/10 text-brand shadow-sm shadow-brand/5">
                <ImagePlus className="h-6 w-6" strokeWidth={1.75} />
              </div>
              <div>
                <p className="text-sm font-semibold text-ink-700">
                  {title
                    ? tString("labels.uploadTitle", {
                        title: title.toLowerCase(),
                      })
                    : tString("labels.uploadImage")}
                </p>
                <p className="text-xs text-ink-500">
                  PNG, JPG, WebP {tString("labels.upTo")} {maxSize}MB
                </p>
              </div>
              <Button
                variant="flat"
                onPress={handleFileSelect}
                isDisabled={uploading || isLoading}
                size="sm"
                className="bg-brand/10 font-semibold text-brand-dark hover:bg-brand/15"
              >
                {tString("buttons.chooseFile")}
              </Button>
            </div>
          </CardBody>
        </Card>
      )}

      {uploading && (
        <div className="space-y-2">
          <Progress
            value={uploadProgress}
            className="max-w-md"
            aria-label={tString("status.uploading")}
            classNames={{
              indicator: "bg-brand",
            }}
          />
          <p className="text-sm font-medium text-ink-600">
            {tString("status.uploading")}
          </p>
        </div>
      )}

      {error && (
        <Card className="rounded-2xl border border-rose-200 bg-rose-50 shadow-none">
          <CardBody className="gap-2 p-3">
            <p className="text-sm font-medium text-rose-700">{error}</p>
            <Button
              size="sm"
              variant="light"
              onPress={() => setError(null)}
              className="self-start font-semibold text-rose-700 hover:bg-rose-100"
            >
              {tString("buttons.dismiss")}
            </Button>
          </CardBody>
        </Card>
      )}
    </div>
  );
}
