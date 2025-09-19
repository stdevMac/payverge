'use client';

import React, { useState, useEffect, useCallback } from 'react';
import {
  Card,
  CardHeader,
  CardBody,
  Button,
  Input,
  Textarea,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  useDisclosure,
  Chip,
  Divider,
} from '@nextui-org/react';
import { businessApi } from '../../api/business';
import { PrimarySpinner } from '../ui/spinners/PrimarySpinner';
import ImageUpload from './ImageUpload';

interface MenuItem {
  name: string;
  description: string;
  price: number;
  image?: string;
  isAvailable: boolean;
}

interface MenuCategory {
  name: string;
  description: string;
  items: MenuItem[];
}

interface MenuBuilderProps {
  businessId: number;
  initialMenu?: MenuCategory[];
  onMenuUpdate?: (menu: MenuCategory[]) => void;
}

export default function MenuBuilder({ businessId, initialMenu = [], onMenuUpdate }: MenuBuilderProps) {
  const [menu, setMenu] = useState<MenuCategory[]>(initialMenu);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  
  // Modal states
  const { isOpen: isAddCategoryOpen, onOpen: onAddCategoryOpen, onOpenChange: onAddCategoryOpenChange } = useDisclosure();
  const { isOpen: isAddItemOpen, onOpen: onAddItemOpen, onOpenChange: onAddItemOpenChange } = useDisclosure();
  const [selectedCategoryIndex, setSelectedCategoryIndex] = useState<number | null>(null);

  // Form states
  const [categoryName, setCategoryName] = useState('');
  const [categoryDescription, setCategoryDescription] = useState('');
  const [itemName, setItemName] = useState('');
  const [itemDescription, setItemDescription] = useState('');
  const [itemPrice, setItemPrice] = useState('');
  const [itemImage, setItemImage] = useState('');
  const [itemAvailable, setItemAvailable] = useState(true);

  const loadMenu = useCallback(async () => {
    try {
      setIsLoading(true);
      const menuData = await businessApi.getMenu(businessId);
      if (menuData && menuData.categories) {
        setMenu(typeof menuData.categories === 'string' ? JSON.parse(menuData.categories) : menuData.categories);
      }
    } catch (error) {
      console.error('Failed to load menu:', error);
      setError('Failed to load menu data');
    } finally {
      setIsLoading(false);
    }
  }, [businessId]);

  // Load menu data
  useEffect(() => {
    loadMenu();
  }, [loadMenu]);

  const handleAddCategory = async () => {
    if (!categoryName.trim()) return;
    
    try {
      await businessApi.addMenuCategory(businessId, { 
        name: categoryName.trim(), 
        description: categoryDescription.trim(), 
        items: [] 
      });
      await loadMenu();
      setCategoryName('');
      setCategoryDescription('');
      onAddCategoryOpenChange();
    } catch (error) {
      console.error('Failed to add category:', error);
      setError('Failed to add category');
    }
  };

  const handleAddItem = async () => {
    if (!itemName.trim() || !itemPrice || selectedCategoryIndex === null) return;
    
    try {
      const item: MenuItem = {
        name: itemName.trim(),
        description: itemDescription.trim(),
        price: parseFloat(itemPrice),
        image: itemImage || undefined,
        isAvailable: itemAvailable,
      };
      
      await businessApi.addMenuItem(businessId, selectedCategoryIndex, item);
      await loadMenu();
      setItemName('');
      setItemDescription('');
      setItemPrice('');
      setItemImage('');
      setItemAvailable(true);
      onAddItemOpenChange();
    } catch (error) {
      console.error('Failed to add item:', error);
      setError('Failed to add item');
    }
  };

  const handleDeleteCategory = async (categoryIndex: number) => {
    if (!confirm('Are you sure you want to delete this category?')) return;
    
    try {
      await businessApi.deleteMenuCategory(businessId, categoryIndex);
      await loadMenu();
    } catch (error) {
      console.error('Failed to delete category:', error);
      setError('Failed to delete category');
    }
  };

  const handleDeleteItem = async (categoryIndex: number, itemIndex: number) => {
    if (!confirm('Are you sure you want to delete this item?')) return;
    
    try {
      await businessApi.deleteMenuItem(businessId, categoryIndex, itemIndex);
      await loadMenu();
    } catch (error) {
      console.error('Failed to delete item:', error);
      setError('Failed to delete item');
    }
  };

  if (isLoading) {
    return <PrimarySpinner />;
  }

  return (
    <div className="space-y-6">
      <div className="flex justify-between items-center">
        <h2 className="text-2xl font-bold">Menu Builder</h2>
        <Button color="primary" onPress={onAddCategoryOpen}>
          Add Category
        </Button>
      </div>

      {error && (
        <Card className="border-danger">
          <CardBody>
            <p className="text-danger">{error}</p>
            <Button size="sm" color="danger" variant="light" onPress={() => setError(null)}>
              Dismiss
            </Button>
          </CardBody>
        </Card>
      )}

      {menu.length === 0 ? (
        <Card>
          <CardBody className="text-center py-8">
            <p className="text-default-500 mb-4">No categories yet. Add your first category to get started!</p>
          </CardBody>
        </Card>
      ) : (
        <div className="space-y-4">
          {menu.map((category, categoryIndex) => (
            <Card key={categoryIndex}>
              <CardHeader>
                <div className="flex justify-between items-start w-full">
                  <div>
                    <h3 className="text-lg font-semibold">{category.name}</h3>
                    {category.description && (
                      <p className="text-default-600 text-sm">{category.description}</p>
                    )}
                  </div>
                  <div className="flex gap-2">
                    <Button
                      size="sm"
                      color="primary"
                      variant="light"
                      onPress={() => {
                        setSelectedCategoryIndex(categoryIndex);
                        onAddItemOpen();
                      }}
                    >
                      Add Item
                    </Button>
                    <Button
                      size="sm"
                      color="danger"
                      variant="light"
                      onPress={() => handleDeleteCategory(categoryIndex)}
                    >
                      Delete
                    </Button>
                  </div>
                </div>
              </CardHeader>
              <CardBody>
                {category.items.length === 0 ? (
                  <p className="text-default-500 text-center py-4">No items in this category yet.</p>
                ) : (
                  <div className="space-y-3">
                    {category.items.map((item, itemIndex) => (
                      <div key={itemIndex} className="border rounded-lg p-4 bg-default-50">
                        <div className="flex justify-between items-start">
                          <div className="flex-1">
                            <div className="flex items-center gap-2 mb-1">
                              <h4 className="font-medium">{item.name}</h4>
                              <Chip color="success" size="sm">
                                ${item.price.toFixed(2)}
                              </Chip>
                              {!item.isAvailable && (
                                <Chip color="danger" size="sm" variant="flat">
                                  Unavailable
                                </Chip>
                              )}
                            </div>
                            {item.description && (
                              <p className="text-default-600 text-sm">{item.description}</p>
                            )}
                          </div>
                          <Button
                            size="sm"
                            color="danger"
                            variant="light"
                            onPress={() => handleDeleteItem(categoryIndex, itemIndex)}
                          >
                            Delete
                          </Button>
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </CardBody>
            </Card>
          ))}
        </div>
      )}

      {/* Add Category Modal */}
      <Modal isOpen={isAddCategoryOpen} onOpenChange={onAddCategoryOpenChange}>
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader>Add New Category</ModalHeader>
              <ModalBody>
                <Input
                  label="Category Name"
                  placeholder="e.g., Appetizers, Main Courses"
                  value={categoryName}
                  onValueChange={setCategoryName}
                  isRequired
                />
                <Textarea
                  label="Description"
                  placeholder="Brief description of this category"
                  value={categoryDescription}
                  onValueChange={setCategoryDescription}
                />
              </ModalBody>
              <ModalFooter>
                <Button color="danger" variant="light" onPress={onClose}>
                  Cancel
                </Button>
                <Button color="primary" onPress={handleAddCategory}>
                  Add Category
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      {/* Add Item Modal */}
      <Modal isOpen={isAddItemOpen} onOpenChange={onAddItemOpenChange}>
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader>Add New Item</ModalHeader>
              <ModalBody>
                <Input
                  label="Item Name"
                  placeholder="e.g., Caesar Salad"
                  value={itemName}
                  onValueChange={setItemName}
                  isRequired
                />
                <Textarea
                  label="Description"
                  placeholder="Brief description of the item"
                  value={itemDescription}
                  onValueChange={setItemDescription}
                />
                <Input
                  label="Price ($)"
                  type="number"
                  placeholder="0.00"
                  value={itemPrice}
                  onValueChange={setItemPrice}
                  isRequired
                />
                <ImageUpload
                  onImageUploaded={(url) => setItemImage(url)}
                  currentImage={itemImage}
                />
                <div className="flex items-center gap-2">
                  <input
                    type="checkbox"
                    id="item-available"
                    checked={itemAvailable}
                    onChange={(e) => setItemAvailable(e.target.checked)}
                    className="rounded"
                  />
                  <label htmlFor="item-available" className="text-sm">
                    Available for ordering
                  </label>
                </div>
              </ModalBody>
              <ModalFooter>
                <Button color="danger" variant="light" onPress={onClose}>
                  Cancel
                </Button>
                <Button color="primary" onPress={handleAddItem}>
                  Add Item
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </div>
  );
}
