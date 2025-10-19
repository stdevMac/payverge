'use client';

import React, { useState } from 'react';
import { uploadFile } from '@/api/uploads';
import {
  Button,
  Card,
  CardBody,
  Progress,
  Image,
} from '@nextui-org/react';
import { Upload, X, Plus, ImageIcon } from 'lucide-react';
import { useToast } from '@/contexts/ToastContext';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';

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
  maxSize = 10 // Higher quality for banners
}: BannerImageUploaderProps) {
  const { locale: currentLocale } = useSimpleLocale();
  
  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `bannerImageUploader.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  };

  const [uploading, setUploading] = useState<number | null>(null);
  const [uploadProgress, setUploadProgress] = useState(0);
  const [error, setError] = useState<string | null>(null);
  const { showSuccess, showError } = useToast();

  const handleFileUpload = async (file: File, index: number) => {
    // Validate file type
    if (!file.type.startsWith('image/')) {
      setError(tString('errors.selectImageFile'));
      return;
    }

    // Validate file size (higher limit for banners)
    if (file.size > maxSize * 1024 * 1024) {
      setError(tString('errors.imageSizeLimit').replace('{maxSize}', maxSize.toString()));
      return;
    }

    try {
      setUploading(index);
      setError(null);
      setUploadProgress(0);

      // Upload to backend S3 endpoint with banner type
      const result = await uploadFile(file, 'banner', businessId);
      const imageUrl = result.url || result.location;
      
      // Update banner images array
      const newBanners = [...bannerImages];
      while (newBanners.length <= index) {
        newBanners.push('');
      }
      newBanners[index] = imageUrl;
      onBannersChange(newBanners);
      
      setUploadProgress(100);
      
      // Show success toast
      showSuccess(
        tString('success.uploaded'),
        tString('success.uploadedDescription').replace('{index}', (index + 1).toString()),
        3000
      );
    } catch (err) {
      const errorMessage = err instanceof Error ? err.message : tString('errors.uploadFailed');
      setError(errorMessage);
      
      // Show error toast
      showError(
        tString('errors.uploadFailedTitle'),
        errorMessage,
        5000
      );
    } finally {
      setUploading(null);
      setUploadProgress(0);
    }
  };

  const handleFileSelect = (index: number) => {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = 'image/*';
    input.onchange = (e) => {
      const file = (e.target as HTMLInputElement).files?.[0];
      if (file) {
        handleFileUpload(file, index);
      }
    };
    input.click();
  };

  const removeBanner = (index: number) => {
    const newBanners = [...bannerImages];
    newBanners[index] = '';
    onBannersChange(newBanners);
  };

  const BannerSlot = ({ index }: { index: number }) => {
    const currentImage = bannerImages[index];
    const isUploading = uploading === index;

    if (currentImage) {
      return (
        <Card className="shadow-sm hover:shadow-lg transition-all duration-300 group">
          <CardBody className="p-0 relative overflow-hidden">
            <div className="relative aspect-[3/1] w-full">
              <Image
                src={currentImage}
                alt={`Banner ${index + 1}`}
                className="w-full h-full object-cover"
                classNames={{
                  wrapper: "w-full h-full",
                  img: "w-full h-full object-cover"
                }}
              />
              
              {/* Overlay with actions */}
              <div className="absolute inset-0 bg-black/40 opacity-0 group-hover:opacity-100 transition-opacity duration-300 flex items-center justify-center">
                <div className="flex gap-2">
                  <Button
                    isIconOnly
                    size="sm"
                    color="primary"
                    variant="solid"
                    onPress={() => handleFileSelect(index)}
                    className="bg-white/90 text-primary hover:bg-white"
                  >
                    <Upload className="w-4 h-4" />
                  </Button>
                  <Button
                    isIconOnly
                    size="sm"
                    color="danger"
                    variant="solid"
                    onPress={() => removeBanner(index)}
                    className="bg-white/90 text-danger hover:bg-white"
                  >
                    <X className="w-4 h-4" />
                  </Button>
                </div>
              </div>
            </div>
            
            {/* Banner info */}
            <div className="p-3 bg-default-50">
              <p className="text-xs text-default-600 text-center font-medium">
                {tString('bannerInfo').replace('{index}', (index + 1).toString()).replace('{maxSize}', maxSize.toString())}
              </p>
            </div>
          </CardBody>
        </Card>
      );
    }

    return (
      <Card 
        className="border-dashed border-2 border-default-300 hover:border-primary-300 transition-all duration-300 cursor-pointer hover:shadow-md"
        isPressable
        onPress={() => !isUploading && handleFileSelect(index)}
      >
        <CardBody className="p-6">
          <div className="aspect-[3/1] flex flex-col items-center justify-center space-y-4">
            {isUploading ? (
              <>
                <div className="w-12 h-12 rounded-full bg-primary/10 flex items-center justify-center">
                  <div className="w-6 h-6 border-2 border-primary border-t-transparent rounded-full animate-spin"></div>
                </div>
                <div className="w-full max-w-xs">
                  <Progress
                    value={uploadProgress}
                    color="primary"
                    className="w-full"
                    size="sm"
                  />
                </div>
                <p className="text-sm text-default-600 font-medium">{tString('status.uploading')}</p>
              </>
            ) : (
              <>
                <div className="w-16 h-16 rounded-full bg-default-100 flex items-center justify-center">
                  <Plus className="w-8 h-8 text-default-400" />
                </div>
                <div className="text-center">
                  <p className="text-lg font-semibold text-default-700 mb-1">
                    {tString('addBanner').replace('{index}', (index + 1).toString())}
                  </p>
                  <p className="text-sm text-default-500">
                    {tString('highQualityBanner')}
                  </p>
                  <p className="text-xs text-default-400 mt-1">
                    PNG, JPG, WebP {tString('upTo')} {maxSize}MB
                  </p>
                </div>
              </>
            )}
          </div>
        </CardBody>
      </Card>
    );
  };

  return (
    <div className="space-y-6">
      {/* Banner Grid */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {Array.from({ length: maxImages }, (_, index) => (
          <BannerSlot key={index} index={index} />
        ))}
      </div>

      {/* Error Display */}
      {error && (
        <Card className="border-danger-200 bg-danger-50 shadow-sm">
          <CardBody className="p-4">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <div className="w-8 h-8 rounded-full bg-danger-100 flex items-center justify-center">
                  <X className="w-4 h-4 text-danger-600" />
                </div>
                <p className="text-danger-700 font-medium">{error}</p>
              </div>
              <Button
                size="sm"
                color="danger"
                variant="light"
                onPress={() => setError(null)}
              >
                {tString('buttons.dismiss')}
              </Button>
            </div>
          </CardBody>
        </Card>
      )}

      {/* Guidelines */}
      <Card className="bg-gradient-to-r from-blue-50 to-indigo-50 border border-blue-200 shadow-sm">
        <CardBody className="p-6">
          <div className="flex items-start gap-4">
            <div className="w-12 h-12 rounded-full bg-blue-100 flex items-center justify-center flex-shrink-0">
              <ImageIcon className="w-6 h-6 text-blue-600" />
            </div>
            <div>
              <h4 className="font-semibold text-blue-900 mb-3">{tString('guidelines.title')}</h4>
              <ul className="text-blue-800 text-sm space-y-2">
                <li className="flex items-center gap-2">
                  <div className="w-1.5 h-1.5 rounded-full bg-blue-600"></div>
                  <span><strong>{tString('guidelines.recommendedSize')}:</strong> 1200x400 pixels (3:1 aspect ratio)</span>
                </li>
                <li className="flex items-center gap-2">
                  <div className="w-1.5 h-1.5 rounded-full bg-blue-600"></div>
                  <span><strong>{tString('guidelines.fileSize')}:</strong> {tString('guidelines.fileSizeDescription').replace('{maxSize}', maxSize.toString())}</span>
                </li>
                <li className="flex items-center gap-2">
                  <div className="w-1.5 h-1.5 rounded-full bg-blue-600"></div>
                  <span><strong>{tString('guidelines.content')}:</strong> {tString('guidelines.contentDescription')}</span>
                </li>
                <li className="flex items-center gap-2">
                  <div className="w-1.5 h-1.5 rounded-full bg-blue-600"></div>
                  <span><strong>{tString('guidelines.quality')}:</strong> {tString('guidelines.qualityDescription')}</span>
                </li>
              </ul>
            </div>
          </div>
        </CardBody>
      </Card>
    </div>
  );
});

export default BannerImageUploader;
