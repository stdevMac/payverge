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
import { useToast } from '@/contexts/ToastContext';

interface SimpleImageUploadProps {
  onImageUploaded: (imageUrl: string) => void;
  currentImage?: string;
  isLoading?: boolean;
  businessId?: number;
  type?: 'business-logo' | 'menu-item' | 'banner' | 'gallery';
  title?: string;
  description?: string;
  maxSize?: number; // in MB
}

export default function SimpleImageUpload({ 
  onImageUploaded, 
  currentImage, 
  isLoading = false, 
  businessId,
  type = 'business-logo',
  title,
  description,
  maxSize = 5
}: SimpleImageUploadProps) {
  const [uploading, setUploading] = useState(false);
  const [uploadProgress, setUploadProgress] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const { showSuccess, showError } = useToast();

  const handleFileSelect = () => {
    fileInputRef.current?.click();
  };

  const handleFileChange = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;

    // Validate file type
    if (!file.type.startsWith('image/')) {
      setError('Please select an image file');
      return;
    }

    // Validate file size
    if (file.size > maxSize * 1024 * 1024) {
      setError(`Image size must be less than ${maxSize}MB`);
      return;
    }

    // Validate business ID for protected uploads
    if (!businessId) {
      setError('Business ID is required for file upload');
      return;
    }

    try {
      setUploading(true);
      setError(null);
      setUploadProgress(0);

      // Upload to backend S3 endpoint with proper type
      const result = await uploadFile(file, type, businessId);
      const imageUrl = result.url || result.location;
      onImageUploaded(imageUrl);
      setUploadProgress(100);
      
      // Show success toast
      showSuccess(
        'Image Uploaded!',
        `${title || 'Image'} has been uploaded successfully.`,
        3000
      );
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : 'Failed to upload image';
      setError(errorMessage);
      
      // Show error toast
      showError(
        'Upload Failed',
        errorMessage,
        5000
      );
    } finally {
      setUploading(false);
      if (fileInputRef.current) {
        fileInputRef.current.value = '';
      }
    }
  };

  const handleRemoveImage = () => {
    onImageUploaded('');
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
                width={120}
                height={120}
                className="object-cover rounded-lg border-2 border-default-200"
              />
              <div className="flex-1">
                <p className="text-sm text-default-600 font-medium">Current image</p>
                <p className="text-xs text-default-400 mt-1">
                  Max size: {maxSize}MB • Formats: JPG, PNG, WebP
                </p>
                <div className="flex gap-2 mt-3">
                  <Button
                    size="sm"
                    color="primary"
                    variant="flat"
                    onPress={handleFileSelect}
                    isDisabled={uploading || isLoading}
                  >
                    Change Image
                  </Button>
                  <Button
                    size="sm"
                    color="danger"
                    variant="light"
                    onPress={handleRemoveImage}
                    isDisabled={uploading || isLoading}
                  >
                    Remove
                  </Button>
                </div>
              </div>
            </div>
          </CardBody>
        </Card>
      ) : (
        <Card className="border-dashed border-2 border-default-300 hover:border-primary-300 transition-colors">
          <CardBody className="text-center py-6">
            <div className="space-y-3 flex flex-col items-center justify-center">
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
                  {title ? `Upload ${title.toLowerCase()}` : 'Upload an image'}
                </p>
                <p className="text-xs text-default-400">
                  PNG, JPG, WebP up to {maxSize}MB
                </p>
              </div>
              <Button
                color="primary"
                variant="flat"
                onPress={handleFileSelect}
                isDisabled={uploading || isLoading}
                size="sm"
              >
                Choose File
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
          <p className="text-sm text-default-600">Uploading image...</p>
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
              Dismiss
            </Button>
          </CardBody>
        </Card>
      )}
    </div>
  );
}
