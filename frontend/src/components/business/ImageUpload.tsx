"use client";

import { getSafeApiErrorMessage } from "@/utils/apiError";

import React, { useState, useRef } from "react";
import { uploadFile } from "@/api/uploads";
import { Button, Card, CardBody, Progress, Image } from "@nextui-org/react";
import { ImagePlus } from "lucide-react";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";

interface ImageUploadProps {
  onImageUploaded: (imageUrl: string) => void;
  currentImage?: string;
  isLoading?: boolean;
  businessId?: number;
  type?: "business-logo" | "menu-item" | "offer" | "bundle" | "banner" | "gallery";
  title?: string;
  description?: string;
  aspectRatio?: "square" | "banner" | "auto";
  maxSize?: number; // in MB
}

export default function ImageUpload({
  onImageUploaded,
  currentImage,
  isLoading = false,
  businessId,
  type = "business-logo",
  title,
  description,
  aspectRatio = "auto",
  maxSize = 5,
}: ImageUploadProps) {
  const { locale: currentLocale } = useSimpleLocale();

  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `imageUpload.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const handleFileSelect = () => {
    if (fileInputRef.current) {
      fileInputRef.current.click();
    } else {
      console.error("ImageUpload: File input ref is null");
    }
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
      setError(
        tString("errors.imageSizeLimit").replace(
          "{maxSize}",
          maxSize.toString(),
        ),
      );
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

      // Upload to backend S3 endpoint with proper type
      const result = await uploadFile(file, type, businessId);
      const imageUrl = result.location;
      onImageUploaded(imageUrl);
    } catch (err) {
      setError(
        getSafeApiErrorMessage(err, tString("errors.uploadFailed")),
      );
    } finally {
      setUploading(false);
      // Reset file input
      if (fileInputRef.current) {
        fileInputRef.current.value = "";
      }
    }
  };

  const handleRemoveImage = () => {
    onImageUploaded("");
  };

  const getImageDimensions = () => {
    switch (aspectRatio) {
      case "square":
        return { width: 120, height: 120 };
      case "banner":
        return { width: 200, height: 80 };
      default:
        return { width: 120, height: 80 };
    }
  };

  const getPlaceholderHeight = () => {
    switch (aspectRatio) {
      case "square":
        return "h-32";
      case "banner":
        return "h-24";
      default:
        return "h-28";
    }
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
                alt={title || "Uploaded image"}
                {...getImageDimensions()}
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
        <Card
          // The empty-state card contains its own "Choose file" button, so it
          // is no longer pressable itself — having both fired handleFileSelect
          // twice per tap (button onPress + card onPress).
          className="rounded-3xl border border-dashed border-warm-300 bg-warm-50/60 shadow-none transition-colors hover:border-brand/50 hover:bg-brand/5"
        >
          <CardBody className={`text-center py-6 ${getPlaceholderHeight()}`}>
            <div className="space-y-3 flex flex-col items-center justify-center h-full">
              <div className="flex h-12 w-12 items-center justify-center rounded-2xl border border-brand/15 bg-brand/10 text-brand shadow-sm shadow-brand/5">
                <ImagePlus className="h-6 w-6" strokeWidth={1.75} />
              </div>
              <div>
                <p className="text-sm font-semibold text-ink-700">
                  {title
                    ? tString("labels.uploadTitle").replace(
                        "{title}",
                        title.toLowerCase(),
                      )
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
          {/* uploadFile has no onUploadProgress wiring, so uploadProgress only
              ever jumps 0 -> 100. Use an indeterminate bar (like the sibling
              MultipleImageUpload / BannerImageUploader) so it doesn't look
              frozen at 0% on slow connections. */}
          <Progress
            isIndeterminate
            aria-label={tString("status.uploading")}
            className="max-w-md"
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
