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

interface ImageUploadProps {
  onImageUploaded: (imageUrl: string) => void;
  currentImage?: string;
  isLoading?: boolean;
  businessId?: number;
}

export default function ImageUpload({ onImageUploaded, currentImage, isLoading = false, businessId }: ImageUploadProps) {
  const [uploading, setUploading] = useState(false);
  const [uploadProgress, setUploadProgress] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

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

    // Validate file size (max 5MB)
    if (file.size > 5 * 1024 * 1024) {
      setError('Image size must be less than 5MB');
      return;
    }

    try {
      setUploading(true);
      setError(null);
      setUploadProgress(0);

      // Upload to backend S3 endpoint
      const result = await uploadFile(file, 'menu-item', businessId);
      const imageUrl = result.url || result.location; // Use url if available, otherwise location
      onImageUploaded(imageUrl);
      setUploadProgress(100);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to upload image');
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

  return (
    <div className="space-y-3">
      <input
        ref={fileInputRef}
        type="file"
        accept="image/*"
        onChange={handleFileChange}
        className="hidden"
      />

      {currentImage ? (
        <Card>
          <CardBody className="p-4">
            <div className="flex items-center gap-4">
              <Image
                src={currentImage}
                alt="Menu item"
                width={80}
                height={80}
                className="object-cover rounded-lg"
              />
              <div className="flex-1">
                <p className="text-sm text-default-600">Current image</p>
                <div className="flex gap-2 mt-2">
                  <Button
                    size="sm"
                    color="primary"
                    variant="light"
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
        <Card className="border-dashed border-2 border-default-300">
          <CardBody className="text-center py-8">
            <div className="space-y-3">
              <div className="text-default-400">
                <svg
                  className="mx-auto h-12 w-12"
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
                <p className="text-default-600">Upload an image for this menu item</p>
                <p className="text-sm text-default-400">PNG, JPG, GIF up to 5MB</p>
              </div>
              <Button
                color="primary"
                variant="bordered"
                onPress={handleFileSelect}
                isDisabled={uploading || isLoading}
              >
                Select Image
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
