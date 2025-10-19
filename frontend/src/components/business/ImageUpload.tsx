'use client';

import React, { useState, useRef } from 'react';
import { uploadFile } from '@/api/uploads';
import {
  Button,
  Card,
  CardBody,
  Progress,
  Image,
} from '@nextui-org/react';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';

interface ImageUploadProps {
  onImageUploaded: (imageUrl: string) => void;
  currentImage?: string;
  isLoading?: boolean;
  businessId?: number;
  type?: 'business-logo' | 'menu-item' | 'banner' | 'gallery';
  title?: string;
  description?: string;
  aspectRatio?: 'square' | 'banner' | 'auto';
  maxSize?: number; // in MB
}

export default function ImageUpload({ 
  onImageUploaded, 
  currentImage, 
  isLoading = false, 
  businessId,
  type = 'business-logo',
  title,
  description,
  aspectRatio = 'auto',
  maxSize = 5
}: ImageUploadProps) {
  const { locale: currentLocale } = useSimpleLocale();
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `imageUpload.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };

  const [uploading, setUploading] = useState(false);
  const [uploadProgress, setUploadProgress] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const handleFileSelect = () => {
    console.log('ImageUpload: handleFileSelect called');
    console.log('ImageUpload: fileInputRef.current:', fileInputRef.current);
    if (fileInputRef.current) {
      fileInputRef.current.click();
      console.log('ImageUpload: File input clicked');
    } else {
      console.error('ImageUpload: File input ref is null');
    }
  };

  const handleFileChange = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;

    // Validate file type
    if (!file.type.startsWith('image/')) {
      setError(tString('errors.selectImageFile'));
      return;
    }

    // Validate file size
    if (file.size > maxSize * 1024 * 1024) {
      setError(tString('errors.imageSizeLimit').replace('{maxSize}', maxSize.toString()));
      return;
    }

    // Validate business ID for protected uploads
    if (!businessId) {
      setError(tString('errors.businessIdRequired'));
      return;
    }

    try {
      setUploading(true);
      setError(null);
      setUploadProgress(0);

      // Upload to backend S3 endpoint with proper type
      const result = await uploadFile(file, type, businessId);
      const imageUrl = result.url || result.location; // Use url if available, otherwise location
      onImageUploaded(imageUrl);
      setUploadProgress(100);
    } catch (err) {
      setError(err instanceof Error ? err.message : tString('errors.uploadFailed'));
    } finally {
      setUploading(false);
      // Reset file input
      if (fileInputRef.current) {
        fileInputRef.current.value = '';
      }
    }
  };

  const handleRemoveImage = () => {
    onImageUploaded('');
  };

  const getImageDimensions = () => {
    switch (aspectRatio) {
      case 'square':
        return { width: 120, height: 120 };
      case 'banner':
        return { width: 200, height: 80 };
      default:
        return { width: 120, height: 80 };
    }
  };

  const getPlaceholderHeight = () => {
    switch (aspectRatio) {
      case 'square':
        return 'h-32';
      case 'banner':
        return 'h-24';
      default:
        return 'h-28';
    }
  };

  return (
    <div className="space-y-3">
      {title && (
        <div>
          <h4 className="text-sm font-medium text-default-700">{title}</h4>
          {description && (
            <p className="text-xs text-default-500 mt-1">{description}</p>
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
        <Card className="shadow-sm">
          <CardBody className="p-4">
            <div className="flex items-center gap-4">
              <Image
                src={currentImage}
                alt={title || 'Uploaded image'}
                {...getImageDimensions()}
                className="object-cover rounded-lg border-2 border-default-200"
              />
              <div className="flex-1">
                <p className="text-sm text-default-600 font-medium">{tString('labels.currentImage')}</p>
                <p className="text-xs text-default-400 mt-1">
                  {tString('labels.maxSize')}: {maxSize}MB • {tString('labels.formats')}: JPG, PNG, WebP
                </p>
                <div className="flex gap-2 mt-3">
                  <Button
                    size="sm"
                    color="primary"
                    variant="flat"
                    onPress={handleFileSelect}
                    isDisabled={uploading || isLoading}
                  >
                    {tString('buttons.changeImage')}
                  </Button>
                  <Button
                    size="sm"
                    color="danger"
                    variant="light"
                    onPress={handleRemoveImage}
                    isDisabled={uploading || isLoading}
                  >
                    {tString('buttons.remove')}
                  </Button>
                </div>
              </div>
            </div>
          </CardBody>
        </Card>
      ) : (
        <Card 
          className="border-dashed border-2 border-default-300 hover:border-primary-300 transition-colors cursor-pointer"
          isPressable
          onPress={handleFileSelect}
        >
          <CardBody className={`text-center py-6 ${getPlaceholderHeight()}`}>
            <div className="space-y-3 flex flex-col items-center justify-center h-full">
              <div className="text-default-400">
                <svg
                  className="mx-auto h-10 w-10"
                  stroke="currentColor"
                  fill="none"
                  viewBox="0 0 48 48"
                >
                  <path
                    d="M28 8H12a4 4 0 00-4 4v20m32-12v8m0 0v8a4 4 0 01-4 4H12a4 4 0 01-4-4v-4m32-4l-3.172-3.172a4 4 0 00-5.656 0L28 28M8 32l9.172-9.172a4 4 0 015.656 0L28 28m0 0l4 4m4-24h8m-4-4v8m-12 4h.02"
                    strokeWidth={2}
                    strokeLinecap="round"
                    strokeLinejoin="round"
                  />
                </svg>
              </div>
              <div>
                <p className="text-sm font-medium text-default-600">
                  {title ? tString('labels.uploadTitle').replace('{title}', title.toLowerCase()) : tString('labels.uploadImage')}
                </p>
                <p className="text-xs text-default-400">
                  PNG, JPG, WebP {tString('labels.upTo')} {maxSize}MB
                </p>
              </div>
              <Button
                color="primary"
                variant="flat"
                onPress={handleFileSelect}
                onClick={handleFileSelect}
                isDisabled={uploading || isLoading}
                size="sm"
              >
                {tString('buttons.chooseFile')}
              </Button>
            </div>
          </CardBody>
        </Card>
      )}
      {uploading && (
        <div className="space-y-2">
          <Progress
            value={uploadProgress}
            color="primary"
            className="max-w-md"
          />
          <p className="text-sm text-default-600">{tString('status.uploading')}</p>
        </div>
      )}

      {error && (
        <Card className="border-danger">
          <CardBody>
            <p className="text-danger text-sm">{error}</p>
            <Button
              size="sm"
              color="danger"
              variant="light"
              onPress={() => setError(null)}
            >
              {tString('buttons.dismiss')}
            </Button>
          </CardBody>
        </Card>
      )}
    </div>
  );
}
