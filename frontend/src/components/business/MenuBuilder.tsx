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
      setError(null);
      const menuData = await businessApi.getMenu(businessId);
      
      if (menuData && menuData.categories) {
        let categories;
        
        // Handle string or array categories
        if (typeof menuData.categories === 'string') {
          try {
            categories = JSON.parse(menuData.categories);
          } catch (parseError) {
            console.error('Failed to parse menu categories:', parseError);
            categories = [];
          }
        } else {
          categories = menuData.categories;
        }
        
        // Ensure categories is an array
        if (Array.isArray(categories)) {
          setMenu(categories);
        } else {
          console.error('Menu categories is not an array:', categories);
          setMenu([]);
        }
      } else {
        // No menu data, set empty array
        setMenu([]);
      }
    } catch (error) {
      console.error('Failed to load menu:', error);
      setError('Failed to load menu data');
      setMenu([]); // Ensure menu is always an array even on error
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
      
      // Clear form fields after successful addition
      setItemName('');
      setItemDescription('');
      setItemPrice('');
      setItemImage(''); // This is fine to clear since the item has been saved
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
    return (
      <div className="flex items-center justify-center py-16">
        <div className="text-center">
          <div className="w-16 h-16 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-6 border border-gray-100">
            <div className="w-8 h-8 border-2 border-gray-300 border-t-gray-900 rounded-full animate-spin"></div>
          </div>
          <p className="text-gray-600 font-light tracking-wide">Loading menu...</p>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-8">
      <div className="flex justify-between items-center">
        <h2 className="text-3xl font-light text-gray-900 tracking-wide">Menu Builder</h2>
        <button
          onClick={onAddCategoryOpen}
          className="bg-gray-900 text-white px-6 py-3 text-sm font-medium hover:bg-gray-800 transition-all duration-200 tracking-wide rounded-lg shadow-md hover:shadow-lg"
        >
          Add Category
        </button>
      </div>

      {error && (
        <div className="bg-red-50 border border-red-200 rounded-2xl p-6 shadow-sm">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-4">
              <div className="w-12 h-12 bg-red-100 rounded-xl flex items-center justify-center">
                <svg className="h-6 w-6 text-red-600" viewBox="0 0 20 20" fill="currentColor">
                  <path fillRule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clipRule="evenodd" />
                </svg>
              </div>
              <div>
                <h3 className="text-lg font-medium text-red-900 tracking-wide">Error</h3>
                <p className="text-red-700 font-light">{error}</p>
              </div>
            </div>
            <button
              onClick={() => setError(null)}
              className="text-red-600 hover:text-red-800 transition-colors duration-200"
            >
              Dismiss
            </button>
          </div>
        </div>
      )}

      {!Array.isArray(menu) || menu.length === 0 ? (
        <div className="bg-white border border-gray-200 rounded-2xl p-16 text-center shadow-sm">
          <div className="w-20 h-20 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-8 border border-gray-100">
            <svg className="h-10 w-10 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.746 0 3.332.477 4.5 1.253v13C19.832 18.477 18.246 18 16.5 18c-1.746 0-3.332.477-4.5 1.253" />
            </svg>
          </div>
          <h3 className="text-2xl font-light text-gray-900 tracking-wide mb-4">No menu categories yet</h3>
          <p className="text-gray-600 font-light leading-relaxed mb-8 max-w-md mx-auto">
            Create your first menu category to start building your restaurant menu.
          </p>
          <button
            onClick={onAddCategoryOpen}
            className="bg-gray-900 text-white px-8 py-3 text-base font-medium hover:bg-gray-800 transition-all duration-200 tracking-wide rounded-lg shadow-lg hover:shadow-xl"
          >
            Add Your First Category
          </button>
        </div>
      ) : (
        <div className="space-y-8">
          {Array.isArray(menu) && menu.map((category, categoryIndex) => (
            <div key={categoryIndex} className="bg-white border border-gray-200 rounded-2xl p-8 shadow-sm">
              <div className="flex justify-between items-start mb-8">
                <div>
                  <h3 className="text-2xl font-light text-gray-900 tracking-wide mb-2">{category.name}</h3>
                  {category.description && (
                    <p className="text-gray-600 font-light leading-relaxed">{category.description}</p>
                  )}
                </div>
                <div className="flex gap-3">
                  <button
                    onClick={() => {
                      setSelectedCategoryIndex(categoryIndex);
                      onAddItemOpen();
                    }}
                    className="bg-gray-100 text-gray-700 px-4 py-2 text-sm font-medium hover:bg-gray-200 transition-all duration-200 tracking-wide rounded-lg"
                  >
                    Add Item
                  </button>
                  <button
                    onClick={() => handleDeleteCategory(categoryIndex)}
                    className="text-red-600 hover:text-red-800 px-4 py-2 text-sm font-medium transition-colors duration-200 tracking-wide"
                  >
                    Delete
                  </button>
                </div>
              </div>
              
              {category.items.length === 0 ? (
                <div className="bg-gray-50 border border-gray-100 rounded-xl p-8 text-center">
                  <p className="text-gray-500 font-light">No items in this category yet</p>
                </div>
              ) : (
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
                  {category.items.map((item, itemIndex) => (
                    <div key={itemIndex} className="group bg-gray-50 border border-gray-100 rounded-xl p-6 hover:bg-white hover:border-gray-200 hover:shadow-md transition-all duration-200">
                      <div className="flex justify-between items-start mb-4">
                        <h4 className="font-medium text-gray-900 tracking-wide">{item.name}</h4>
                        <div className={`px-3 py-1 rounded-xl text-xs font-medium tracking-wide ${
                          item.isAvailable 
                            ? 'bg-green-100 text-green-700 border border-green-200' 
                            : 'bg-gray-100 text-gray-700 border border-gray-200'
                        }`}>
                          {item.isAvailable ? 'Available' : 'Unavailable'}
                        </div>
                      </div>
                      {item.description && (
                        <p className="text-sm text-gray-600 font-light mb-4 leading-relaxed">{item.description}</p>
                      )}
                      <div className="flex justify-between items-center">
                        <span className="text-xl font-light text-gray-900 tracking-wide">${item.price.toFixed(2)}</span>
                        <button
                          onClick={() => handleDeleteItem(categoryIndex, itemIndex)}
                          className="text-red-600 hover:text-red-800 text-sm font-medium transition-colors duration-200 opacity-0 group-hover:opacity-100"
                        >
                          Delete
                        </button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
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
                  businessId={businessId}
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
