'use client';

import React, { useState } from 'react';
import {
  Card,
  CardBody,
  Button,
  Image,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  useDisclosure,
} from '@nextui-org/react';
import { Upload, X, Plus, Eye, Trash2 } from 'lucide-react';
import ImageUpload from './ImageUpload';

interface MultipleImageUploadProps {
  images: string[];
  onImagesChange: (images: string[]) => void;
  maxImages?: number;
  businessId: number;
}

export default function MultipleImageUpload({ 
  images, 
  onImagesChange, 
  maxImages = 5,
  businessId 
}: MultipleImageUploadProps) {
  const [selectedImageIndex, setSelectedImageIndex] = useState<number | null>(null);
  const { isOpen: isViewModalOpen, onOpen: onViewModalOpen, onClose: onViewModalClose } = useDisclosure();
  const { isOpen: isAddModalOpen, onOpen: onAddModalOpen, onClose: onAddModalClose } = useDisclosure();

  const handleAddModalOpen = () => {
    console.log('MultipleImageUpload: Opening modal with businessId:', businessId);
    onAddModalOpen();
  };

  const handleImageUploaded = (imageUrl: string) => {
    console.log('MultipleImageUpload: Image uploaded successfully:', imageUrl);
    const newImages = [...images, imageUrl];
    onImagesChange(newImages);
    onAddModalClose();
  };

  const handleRemoveImage = (index: number) => {
    console.log('MultipleImageUpload: Removing image at index:', index);
    console.log('MultipleImageUpload: Current images:', images);
    const newImages = images.filter((_, i) => i !== index);
    console.log('MultipleImageUpload: New images after removal:', newImages);
    onImagesChange(newImages);
  };

  const handleViewImage = (index: number) => {
    setSelectedImageIndex(index);
    onViewModalOpen();
  };

  const canAddMore = images.length < maxImages;

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h4 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
            <Upload className="w-5 h-5" />
            Images ({images.length}/{maxImages})
          </h4>
          <p className="text-sm text-gray-500">
            Add multiple images to showcase your menu item
          </p>
        </div>
        {canAddMore && (
          <Button
            color="primary"
            variant="flat"
            startContent={<Plus className="w-4 h-4" />}
            onPress={handleAddModalOpen}
            size="sm"
          >
            Add Image
          </Button>
        )}
      </div>

      {/* Images Grid */}
      {images.length > 0 ? (
        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-4">
          {images.map((imageUrl, index) => (
            <Card key={index} className="relative group">
              <CardBody className="p-0">
                <div className="relative aspect-square">
                  <Image
                    src={imageUrl}
                    alt={`Menu item image ${index + 1}`}
                    className="w-full h-full object-cover"
                    radius="lg"
                  />
                  
                  {/* Overlay with view action */}
                  <div className="absolute inset-0 bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity duration-200 rounded-lg flex items-center justify-center z-10">
                    <Button
                      isIconOnly
                      size="sm"
                      variant="flat"
                      color="default"
                      className="bg-white/20 backdrop-blur-sm z-20"
                      onPress={() => handleViewImage(index)}
                    >
                      <Eye className="w-4 h-4 text-white" />
                    </Button>
                  </div>

                  {/* Always visible delete button for better mobile experience */}
                  <Button
                    isIconOnly
                    size="sm"
                    variant="flat"
                    color="danger"
                    className="absolute top-2 right-2 bg-red-500 hover:bg-red-600 shadow-lg z-30"
                    onPress={() => handleRemoveImage(index)}
                    onClick={() => handleRemoveImage(index)}
                  >
                    <X className="w-3 h-3 text-white" />
                  </Button>

                  {/* Primary image indicator */}
                  {index === 0 && (
                    <div className="absolute top-2 left-2 z-20">
                      <div className="bg-primary text-white text-xs px-2 py-1 rounded-full">
                        Primary
                      </div>
                    </div>
                  )}
                </div>
              </CardBody>
            </Card>
          ))}
        </div>
      ) : (
        <Card className="border-2 border-dashed border-gray-300">
          <CardBody className="text-center py-8">
            <Upload className="w-12 h-12 text-gray-400 mx-auto mb-4" />
            <p className="text-gray-500 mb-4">No images added yet</p>
            <Button
              color="primary"
              variant="flat"
              startContent={<Plus className="w-4 h-4" />}
              onPress={handleAddModalOpen}
            >
              Add First Image
            </Button>
          </CardBody>
        </Card>
      )}

      {/* Add Image Modal */}
      <Modal isOpen={isAddModalOpen} onClose={onAddModalClose} size="lg">
        <ModalContent>
          <ModalHeader>Add New Image</ModalHeader>
          <ModalBody>
            <ImageUpload
              onImageUploaded={handleImageUploaded}
              currentImage=""
              businessId={businessId}
              type="menu-item"
              title="Menu Item Image"
              description="Upload an image for this menu item"
              aspectRatio="square"
              maxSize={5}
            />
          </ModalBody>
          <ModalFooter>
            <Button color="danger" variant="light" onPress={onAddModalClose}>
              Cancel
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>

      {/* View Image Modal */}
      <Modal 
        isOpen={isViewModalOpen} 
        onClose={onViewModalClose}
        size="2xl"
      >
        <ModalContent>
          <ModalHeader className="flex items-center justify-between">
            <span>Image {(selectedImageIndex || 0) + 1} of {images.length}</span>
            <Button
              isIconOnly
              color="danger"
              variant="light"
              onPress={() => {
                if (selectedImageIndex !== null) {
                  handleRemoveImage(selectedImageIndex);
                  onViewModalClose();
                }
              }}
              onClick={() => {
                if (selectedImageIndex !== null) {
                  handleRemoveImage(selectedImageIndex);
                  onViewModalClose();
                }
              }}
            >
              <Trash2 className="w-4 h-4" />
            </Button>
          </ModalHeader>
          <ModalBody>
            {selectedImageIndex !== null && (
              <div className="relative aspect-video">
                <Image
                  src={images[selectedImageIndex]}
                  alt={`Menu item image ${selectedImageIndex + 1}`}
                  className="w-full h-full object-contain"
                />
              </div>
            )}
          </ModalBody>
          <ModalFooter>
            <Button color="default" onPress={onViewModalClose}>
              Close
            </Button>
          </ModalFooter>
        </ModalContent>
      </Modal>
    </div>
  );
}
