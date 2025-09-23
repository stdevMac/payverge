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
  Switch,
  Select,
  SelectItem,
  Accordion,
  AccordionItem,
} from '@nextui-org/react';
import { businessApi, MenuItem, MenuCategory, MenuItemOption } from '../../api/business';
import { PrimarySpinner } from '../ui/spinners/PrimarySpinner';
import ImageUpload from './ImageUpload';
import MultipleImageUpload from './MultipleImageUpload';
import { Plus, Edit, Trash2, Image as ImageIcon, Tag, AlertTriangle, DollarSign } from 'lucide-react';

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
  const { isOpen: isEditCategoryOpen, onOpen: onEditCategoryOpen, onOpenChange: onEditCategoryOpenChange } = useDisclosure();
  const { isOpen: isAddItemOpen, onOpen: onAddItemOpen, onOpenChange: onAddItemOpenChange } = useDisclosure();
  const { isOpen: isEditItemOpen, onOpen: onEditItemOpen, onOpenChange: onEditItemOpenChange } = useDisclosure();
  
  // Selection states
  const [selectedCategoryIndex, setSelectedCategoryIndex] = useState<number | null>(null);
  const [selectedItemIndex, setSelectedItemIndex] = useState<number | null>(null);
  const [editingCategory, setEditingCategory] = useState<MenuCategory | null>(null);
  const [editingItem, setEditingItem] = useState<MenuItem | null>(null);

  // Category form states
  const [categoryName, setCategoryName] = useState('');
  const [categoryDescription, setCategoryDescription] = useState('');
  
  // Item form states
  const [itemName, setItemName] = useState('');
  const [itemDescription, setItemDescription] = useState('');
  const [itemPrice, setItemPrice] = useState('');
  const [itemCurrency, setItemCurrency] = useState('USD');
  const [itemImage, setItemImage] = useState('');
  const [itemImages, setItemImages] = useState<string[]>([]);
  const [itemAvailable, setItemAvailable] = useState(true);
  const [itemOptions, setItemOptions] = useState<MenuItemOption[]>([]);
  const [itemAllergens, setItemAllergens] = useState<string[]>([]);
  const [itemDietaryTags, setItemDietaryTags] = useState<string[]>([]);
  const [itemSortOrder, setItemSortOrder] = useState(0);
  
  // Temporary input states
  const [newAllergen, setNewAllergen] = useState('');
  const [newDietaryTag, setNewDietaryTag] = useState('');
  const [newOptionName, setNewOptionName] = useState('');
  const [newOptionPrice, setNewOptionPrice] = useState('');

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

  // Helper functions
  const resetCategoryForm = () => {
    setCategoryName('');
    setCategoryDescription('');
    setEditingCategory(null);
  };

  const resetItemForm = () => {
    setItemName('');
    setItemDescription('');
    setItemPrice('');
    setItemCurrency('USD');
    setItemImage('');
    setItemImages([]);
    setItemAvailable(true);
    setItemOptions([]);
    setItemAllergens([]);
    setItemDietaryTags([]);
    setItemSortOrder(0);
    setEditingItem(null);
    setNewAllergen('');
    setNewDietaryTag('');
    setNewOptionName('');
    setNewOptionPrice('');
  };

  const populateItemForm = (item: MenuItem) => {
    setItemName(item.name);
    setItemDescription(item.description);
    setItemPrice(item.price.toString());
    setItemCurrency(item.currency || 'USD');
    setItemImage(item.image || '');
    setItemImages(item.images || []);
    setItemAvailable(item.is_available);
    setItemOptions(item.options || []);
    setItemAllergens(item.allergens || []);
    setItemDietaryTags(item.dietary_tags || []);
    setItemSortOrder(item.sort_order || 0);
  };

  const populateCategoryForm = (category: MenuCategory) => {
    setCategoryName(category.name);
    setCategoryDescription(category.description);
  };

  // Category CRUD operations
  const handleAddCategory = async () => {
    if (!categoryName.trim()) return;
    
    try {
      await businessApi.addMenuCategory(businessId, { 
        name: categoryName.trim(), 
        description: categoryDescription.trim(), 
        items: [] 
      });
      await loadMenu();
      resetCategoryForm();
      onAddCategoryOpenChange();
    } catch (error) {
      console.error('Failed to add category:', error);
      setError('Failed to add category');
    }
  };

  const handleEditCategory = (categoryIndex: number) => {
    const category = menu[categoryIndex];
    setEditingCategory(category);
    setSelectedCategoryIndex(categoryIndex);
    populateCategoryForm(category);
    onEditCategoryOpen();
  };

  const handleUpdateCategory = async () => {
    if (!categoryName.trim() || selectedCategoryIndex === null) return;
    
    try {
      const updatedCategory: MenuCategory = {
        name: categoryName.trim(),
        description: categoryDescription.trim(),
        items: editingCategory?.items || []
      };
      
      await businessApi.updateMenuCategory(businessId, selectedCategoryIndex, updatedCategory);
      await loadMenu();
      resetCategoryForm();
      onEditCategoryOpenChange();
    } catch (error) {
      console.error('Failed to update category:', error);
      setError('Failed to update category');
    }
  };

  const handleDeleteCategory = async (categoryIndex: number) => {
    if (!confirm('Are you sure you want to delete this category and all its items?')) return;
    
    try {
      await businessApi.deleteMenuCategory(businessId, categoryIndex);
      await loadMenu();
    } catch (error) {
      console.error('Failed to delete category:', error);
      setError('Failed to delete category');
    }
  };

  // Item CRUD operations
  const handleAddItem = async () => {
    if (!itemName.trim() || !itemPrice || selectedCategoryIndex === null) return;
    
    try {
      const newItem: MenuItem = {
        name: itemName,
        description: itemDescription,
        price: parseFloat(itemPrice),
        currency: itemCurrency,
        image: itemImage,
        images: itemImages,
        is_available: itemAvailable,
        options: itemOptions,
        allergens: itemAllergens,
        dietary_tags: itemDietaryTags,
        sort_order: itemSortOrder,
      };
      
      await businessApi.addMenuItem(businessId, selectedCategoryIndex, newItem);
      await loadMenu();
      resetItemForm();
      onAddItemOpenChange();
    } catch (error) {
      console.error('Failed to add item:', error);
      setError('Failed to add item');
    }
  };

  const handleEditItem = (categoryIndex: number, itemIndex: number) => {
    const item = menu[categoryIndex].items[itemIndex];
    setEditingItem(item);
    setSelectedCategoryIndex(categoryIndex);
    setSelectedItemIndex(itemIndex);
    populateItemForm(item);
    onEditItemOpen();
  };

  const handleUpdateItem = async () => {
    if (!itemName.trim() || !itemPrice || selectedCategoryIndex === null || selectedItemIndex === null) return;
    
    try {
      const updatedItem: MenuItem = {
        ...editingItem,
        name: itemName,
        description: itemDescription,
        price: parseFloat(itemPrice),
        currency: itemCurrency,
        image: itemImage,
        images: itemImages,
        is_available: itemAvailable,
        options: itemOptions,
        allergens: itemAllergens,
        dietary_tags: itemDietaryTags,
        sort_order: itemSortOrder,
      };
      
      await businessApi.updateMenuItem(businessId, selectedCategoryIndex, selectedItemIndex, updatedItem);
      await loadMenu();
      resetItemForm();
      onEditItemOpenChange();
    } catch (error) {
      console.error('Failed to update item:', error);
      setError('Failed to update item');
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

  // Helper functions for managing item properties
  const addOption = () => {
    if (!newOptionName.trim() || !newOptionPrice) return;
    
    const price = parseFloat(newOptionPrice);
    if (isNaN(price) || price < 0) return;
    
    const option: MenuItemOption = {
      name: newOptionName.trim(),
      price_change: price
    };
    
    setItemOptions([...itemOptions, option]);
    setNewOptionName('');
    setNewOptionPrice('');
  };

  const removeOption = (index: number) => {
    setItemOptions(itemOptions.filter((_, i) => i !== index));
  };

  const addAllergen = () => {
    if (!newAllergen.trim()) return;
    
    const allergen = newAllergen.trim();
    if (!itemAllergens.includes(allergen)) {
      setItemAllergens([...itemAllergens, allergen]);
    }
    setNewAllergen('');
  };

  const removeAllergen = (allergen: string) => {
    setItemAllergens(itemAllergens.filter(a => a !== allergen));
  };

  const addDietaryTag = () => {
    if (!newDietaryTag.trim()) return;
    
    const tag = newDietaryTag.trim();
    if (!itemDietaryTags.includes(tag)) {
      setItemDietaryTags([...itemDietaryTags, tag]);
    }
    setNewDietaryTag('');
  };

  const removeDietaryTag = (tag: string) => {
    setItemDietaryTags(itemDietaryTags.filter(t => t !== tag));
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
      {/* Header */}
      <div className="flex justify-between items-center">
        <div>
          <h2 className="text-3xl font-light text-gray-900 tracking-wide">Menu Builder</h2>
          <p className="text-gray-600 font-light mt-2">Create and manage your restaurant menu</p>
        </div>
        <Button
          onPress={onAddCategoryOpen}
          color="primary"
          size="lg"
          startContent={<Plus className="w-5 h-5" />}
          className="font-semibold shadow-lg"
        >
          Add Category
        </Button>
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
            <Card key={categoryIndex} className="shadow-lg border-0">
              <CardHeader className="pb-4">
                <div className="flex justify-between items-start w-full">
                  <div className="flex-1">
                    <h3 className="text-2xl font-light text-gray-900 tracking-wide mb-2">{category.name}</h3>
                    {category.description && (
                      <p className="text-gray-600 font-light leading-relaxed">{category.description}</p>
                    )}
                    <Chip size="sm" variant="flat" color="primary" className="mt-2">
                      {category.items.length} {category.items.length === 1 ? 'item' : 'items'}
                    </Chip>
                  </div>
                  <div className="flex gap-2">
                    <Button
                      size="sm"
                      variant="flat"
                      color="primary"
                      startContent={<Plus className="w-4 h-4" />}
                      onPress={() => {
                        setSelectedCategoryIndex(categoryIndex);
                        resetItemForm();
                        onAddItemOpen();
                      }}
                    >
                      Add Item
                    </Button>
                    <Button
                      size="sm"
                      variant="flat"
                      color="default"
                      startContent={<Edit className="w-4 h-4" />}
                      onPress={() => handleEditCategory(categoryIndex)}
                    >
                      Edit
                    </Button>
                    <Button
                      size="sm"
                      variant="flat"
                      color="danger"
                      startContent={<Trash2 className="w-4 h-4" />}
                      onPress={() => handleDeleteCategory(categoryIndex)}
                    >
                      Delete
                    </Button>
                  </div>
                </div>
              </CardHeader>
              <CardBody className="pt-0">
                {category.items.length === 0 ? (
                  <div className="bg-gray-50 border border-gray-200 rounded-xl p-8 text-center">
                    <div className="w-12 h-12 bg-gray-200 rounded-xl flex items-center justify-center mx-auto mb-4">
                      <Plus className="w-6 h-6 text-gray-500" />
                    </div>
                    <p className="text-gray-500 font-light mb-4">No items in this category yet</p>
                    <Button
                      size="sm"
                      color="primary"
                      variant="flat"
                      onPress={() => {
                        setSelectedCategoryIndex(categoryIndex);
                        resetItemForm();
                        onAddItemOpen();
                      }}
                    >
                      Add First Item
                    </Button>
                  </div>
                ) : (
                  <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                    {category.items.map((item, itemIndex) => (
                      <Card key={itemIndex} className="group hover:shadow-md transition-all duration-200 border-gray-200">
                        <CardBody className="p-4">
                          <div className="flex justify-between items-start mb-3">
                            <h4 className="font-semibold text-gray-900 tracking-wide text-lg">{item.name}</h4>
                            <Chip
                              size="sm"
                              color={item.is_available ? "success" : "default"}
                              variant="flat"
                            >
                              {item.is_available ? 'Available' : 'Unavailable'}
                            </Chip>
                          </div>
                          
                          {item.description && (
                            <p className="text-sm text-gray-600 font-light mb-3 leading-relaxed">{item.description}</p>
                          )}
                          
                          <div className="flex items-center gap-2 mb-3">
                            <DollarSign className="w-4 h-4 text-green-600" />
                            <span className="text-xl font-semibold text-gray-900">{(item.price || 0).toFixed(2)}</span>
                            {item.currency && item.currency !== 'USD' && (
                              <span className="text-sm text-gray-500">{item.currency}</span>
                            )}
                          </div>

                          {/* Additional item details */}
                          <div className="space-y-2 mb-4">
                            {item.allergens && item.allergens.length > 0 && (
                              <div className="flex items-center gap-2">
                                <AlertTriangle className="w-4 h-4 text-orange-500" />
                                <div className="flex flex-wrap gap-1">
                                  {item.allergens.map((allergen, idx) => (
                                    <Chip key={idx} size="sm" color="warning" variant="flat" className="text-xs">
                                      {allergen}
                                    </Chip>
                                  ))}
                                </div>
                              </div>
                            )}
                            
                            {item.dietary_tags && item.dietary_tags.length > 0 && (
                              <div className="flex items-center gap-2">
                                <Tag className="w-4 h-4 text-green-500" />
                                <div className="flex flex-wrap gap-1">
                                  {item.dietary_tags.map((tag, idx) => (
                                    <Chip key={idx} size="sm" color="success" variant="flat" className="text-xs">
                                      {tag}
                                    </Chip>
                                  ))}
                                </div>
                              </div>
                            )}
                          </div>

                          <div className="flex gap-2 opacity-0 group-hover:opacity-100 transition-opacity duration-200">
                            <Button
                              size="sm"
                              variant="flat"
                              color="default"
                              startContent={<Edit className="w-3 h-3" />}
                              onPress={() => handleEditItem(categoryIndex, itemIndex)}
                              className="flex-1"
                            >
                              Edit
                            </Button>
                            <Button
                              size="sm"
                              variant="flat"
                              color="danger"
                              startContent={<Trash2 className="w-3 h-3" />}
                              onPress={() => handleDeleteItem(categoryIndex, itemIndex)}
                              className="flex-1"
                            >
                              Delete
                            </Button>
                          </div>
                        </CardBody>
                      </Card>
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
                <Button color="danger" variant="light" onPress={() => {
                  resetCategoryForm();
                  onClose();
                }}>
                  Cancel
                </Button>
                <Button color="primary" onPress={handleAddCategory} isDisabled={!categoryName.trim()}>
                  Add Category
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      {/* Edit Category Modal */}
      <Modal isOpen={isEditCategoryOpen} onOpenChange={onEditCategoryOpenChange}>
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex items-center gap-2">
                <Edit className="w-5 h-5" />
                Edit Category
              </ModalHeader>
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
                <Button color="danger" variant="light" onPress={() => {
                  resetCategoryForm();
                  onClose();
                }}>
                  Cancel
                </Button>
                <Button color="primary" onPress={handleUpdateCategory} isDisabled={!categoryName.trim()}>
                  Update Category
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      {/* Add Item Modal */}
      <Modal isOpen={isAddItemOpen} onOpenChange={onAddItemOpenChange} size="2xl" scrollBehavior="inside">
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex items-center gap-2">
                <Plus className="w-5 h-5" />
                Add New Item
                {selectedCategoryIndex !== null && (
                  <Chip size="sm" variant="flat" color="primary">
                    {menu[selectedCategoryIndex]?.name}
                  </Chip>
                )}
              </ModalHeader>
              <ModalBody className="space-y-6">
                {/* Basic Information */}
                <div className="space-y-4">
                  <h4 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
                    <ImageIcon className="w-5 h-5" />
                    Basic Information
                  </h4>
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <Input
                      label="Item Name"
                      placeholder="e.g., Caesar Salad"
                      value={itemName}
                      onValueChange={setItemName}
                      isRequired
                    />
                    <div className="flex gap-2">
                      <Input
                        label="Price"
                        placeholder="0.00"
                        value={itemPrice}
                        onValueChange={setItemPrice}
                        type="number"
                        step="0.01"
                        min="0"
                        startContent={<DollarSign className="w-4 h-4 text-gray-400" />}
                        isRequired
                      />
                      <Select
                        label="Currency"
                        selectedKeys={[itemCurrency]}
                        onSelectionChange={(keys) => setItemCurrency(Array.from(keys)[0] as string)}
                        className="w-32"
                      >
                        <SelectItem key="USD" value="USD">USD</SelectItem>
                        <SelectItem key="EUR" value="EUR">EUR</SelectItem>
                        <SelectItem key="GBP" value="GBP">GBP</SelectItem>
                      </Select>
                    </div>
                  </div>
                  <Textarea
                    label="Description"
                    placeholder="Describe this menu item..."
                    value={itemDescription}
                    onValueChange={setItemDescription}
                    minRows={2}
                  />
                  <div className="flex items-center gap-4">
                    <Switch
                      isSelected={itemAvailable}
                      onValueChange={setItemAvailable}
                      color="success"
                    >
                      Available for ordering
                    </Switch>
                    <Input
                      label="Sort Order"
                      placeholder="0"
                      value={itemSortOrder.toString()}
                      onValueChange={(value) => setItemSortOrder(parseInt(value) || 0)}
                      type="number"
                      className="w-32"
                    />
                  </div>
                </div>

                <Divider />

                {/* Multiple Image Upload */}
                <MultipleImageUpload
                  images={itemImages}
                  onImagesChange={setItemImages}
                  maxImages={5}
                  businessId={businessId}
                />

                <Divider />

                {/* Options */}
                <div className="space-y-4">
                  <h4 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
                    <DollarSign className="w-5 h-5" />
                    Options & Add-ons
                  </h4>
                  <div className="flex gap-2">
                    <Input
                      label="Option Name"
                      placeholder="e.g., Extra Cheese"
                      value={newOptionName}
                      onValueChange={setNewOptionName}
                    />
                    <Input
                      label="Price Change"
                      placeholder="0.00"
                      value={newOptionPrice}
                      onValueChange={setNewOptionPrice}
                      type="number"
                      step="0.01"
                      startContent={<DollarSign className="w-4 h-4 text-gray-400" />}
                      className="w-32"
                    />
                    <Button
                      color="primary"
                      variant="flat"
                      onPress={addOption}
                      isDisabled={!newOptionName.trim() || !newOptionPrice}
                    >
                      Add
                    </Button>
                  </div>
                  {itemOptions.length > 0 && (
                    <div className="flex flex-wrap gap-2">
                      {itemOptions.map((option, index) => (
                        <Chip
                          key={`allergen-${option.name}-${index}`}
                          onClose={() => removeOption(index)}
                          variant="flat"
                          color="primary"
                        >
                          {option.name} (+${(option.price_change || 0).toFixed(2)})
                        </Chip>
                      ))}
                    </div>
                  )}
                </div>

                <Divider />

                {/* Allergens */}
                <div className="space-y-4">
                  <h4 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
                    <AlertTriangle className="w-5 h-5" />
                    Allergens
                  </h4>
                  <div className="flex gap-2">
                    <Input
                      label="Allergen"
                      placeholder="e.g., Nuts, Dairy, Gluten"
                      value={newAllergen}
                      onValueChange={setNewAllergen}
                    onKeyPress={(e) => {
                        if (e.key === 'Enter') {
                          e.preventDefault();
                          addAllergen();
                        }
                      }}
                      />
                    <Button
                      color="warning"
                      variant="flat"
                      onPress={addAllergen}
                      isDisabled={!newAllergen.trim()}
                    >
                      Add
                    </Button>
                  </div>
                  {itemAllergens.length > 0 && (
                    <div className="flex flex-wrap gap-2">
                      {itemAllergens.map((allergen, index) => (
                        <Chip
                          key={`allergen-${allergen}-${index}`}
                          onClose={() => removeAllergen(allergen)}
                          variant="flat"
                          color="warning"
                        >
                          {allergen}
                        </Chip>
                      ))}
                    </div>
                  )}
                </div>

                <Divider />

                {/* Dietary Tags */}
                <div className="space-y-4">
                  <h4 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
                    <Tag className="w-5 h-5" />
                    Dietary Tags
                  </h4>
                  <div className="flex gap-2">
                    <Input
                      label="Dietary Tag"
                      placeholder="e.g., Vegetarian, Vegan, Gluten-Free"
                      value={newDietaryTag}
                      onValueChange={setNewDietaryTag}
                    onKeyPress={(e) => {
                        if (e.key === 'Enter') {
                          e.preventDefault();
                          addDietaryTag();
                        }
                      }}
                      />
                    <Button
                      color="success"
                      variant="flat"
                      onPress={addDietaryTag}
                      isDisabled={!newDietaryTag.trim()}
                    >
                      Add
                    </Button>
                  </div>
                  {itemDietaryTags.length > 0 && (
                    <div className="flex flex-wrap gap-2">
                      {itemDietaryTags.map((tag, index) => (
                        <Chip
                          key={`dietary-${tag}-${index}`}
                          onClose={() => removeDietaryTag(tag)}
                          variant="flat"
                          color="success"
                        >
                          {tag}
                        </Chip>
                      ))}
                    </div>
                  )}
                </div>
              </ModalBody>
              <ModalFooter>
                <Button color="danger" variant="light" onPress={() => {
                  resetItemForm();
                  onClose();
                }}>
                  Cancel
                </Button>
                <Button 
                  color="primary" 
                  onPress={handleAddItem}
                  isDisabled={!itemName.trim() || !itemPrice}
                >
                  Add Item
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      {/* Edit Item Modal */}
      <Modal isOpen={isEditItemOpen} onOpenChange={onEditItemOpenChange} size="2xl" scrollBehavior="inside">
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex items-center gap-2">
                <Edit className="w-5 h-5" />
                Edit Item
                {selectedCategoryIndex !== null && (
                  <Chip size="sm" variant="flat" color="primary">
                    {menu[selectedCategoryIndex]?.name}
                  </Chip>
                )}
              </ModalHeader>
              <ModalBody className="space-y-6">
                {/* Basic Information */}
                <div className="space-y-4">
                  <h4 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
                    <ImageIcon className="w-5 h-5" />
                    Basic Information
                  </h4>
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <Input
                      label="Item Name"
                      placeholder="e.g., Caesar Salad"
                      value={itemName}
                      onValueChange={setItemName}
                      isRequired
                    />
                    <div className="flex gap-2">
                      <Input
                        label="Price"
                        placeholder="0.00"
                        value={itemPrice}
                        onValueChange={setItemPrice}
                        type="number"
                        step="0.01"
                        min="0"
                        startContent={<DollarSign className="w-4 h-4 text-gray-400" />}
                        isRequired
                      />
                      <Select
                        label="Currency"
                        selectedKeys={[itemCurrency]}
                        onSelectionChange={(keys) => setItemCurrency(Array.from(keys)[0] as string)}
                        className="w-32"
                      >
                        <SelectItem key="USD" value="USD">USD</SelectItem>
                        <SelectItem key="EUR" value="EUR">EUR</SelectItem>
                        <SelectItem key="GBP" value="GBP">GBP</SelectItem>
                      </Select>
                    </div>
                  </div>
                  <Textarea
                    label="Description"
                    placeholder="Describe this menu item..."
                    value={itemDescription}
                    onValueChange={setItemDescription}
                    minRows={2}
                  />
                  <div className="flex items-center gap-4">
                    <Switch
                      isSelected={itemAvailable}
                      onValueChange={setItemAvailable}
                      color="success"
                    >
                      Available for ordering
                    </Switch>
                    <Input
                      label="Sort Order"
                      placeholder="0"
                      value={itemSortOrder.toString()}
                      onValueChange={(value) => setItemSortOrder(parseInt(value) || 0)}
                      type="number"
                      className="w-32"
                    />
                  </div>
                </div>

                <Divider />

                {/* Multiple Image Upload */}
                <MultipleImageUpload
                  images={itemImages}
                  onImagesChange={setItemImages}
                  maxImages={5}
                  businessId={businessId}
                />

                <Divider />

                {/* Options */}
                <div className="space-y-4">
                  <h4 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
                    <DollarSign className="w-5 h-5" />
                    Options & Add-ons
                  </h4>
                  <div className="flex gap-2">
                    <Input
                      label="Option Name"
                      placeholder="e.g., Extra Cheese"
                      value={newOptionName}
                      onValueChange={setNewOptionName}
                    />
                    <Input
                      label="Price Change"
                      placeholder="0.00"
                      value={newOptionPrice}
                      onValueChange={setNewOptionPrice}
                      type="number"
                      step="0.01"
                      startContent={<DollarSign className="w-4 h-4 text-gray-400" />}
                      className="w-32"
                    />
                    <Button
                      color="primary"
                      variant="flat"
                      onPress={addOption}
                      isDisabled={!newOptionName.trim() || !newOptionPrice}
                    >
                      Add
                    </Button>
                  </div>
                  {itemOptions.length > 0 && (
                    <div className="flex flex-wrap gap-2">
                      {itemOptions.map((option, index) => (
                        <Chip
                          key={`allergen-${option.name}-${index}`}
                          onClose={() => removeOption(index)}
                          variant="flat"
                          color="primary"
                        >
                          {option.name} (+${(option.price_change || 0).toFixed(2)})
                        </Chip>
                      ))}
                    </div>
                  )}
                </div>

                <Divider />

                {/* Allergens */}
                <div className="space-y-4">
                  <h4 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
                    <AlertTriangle className="w-5 h-5" />
                    Allergens
                  </h4>
                  <div className="flex gap-2">
                    <Input
                      label="Allergen"
                      placeholder="e.g., Nuts, Dairy, Gluten"
                      value={newAllergen}
                      onValueChange={setNewAllergen}
                    onKeyPress={(e) => {
                        if (e.key === 'Enter') {
                          e.preventDefault();
                          addAllergen();
                        }
                      }}
                      />
                    <Button
                      color="warning"
                      variant="flat"
                      onPress={addAllergen}
                      isDisabled={!newAllergen.trim()}
                    >
                      Add
                    </Button>
                  </div>
                  {itemAllergens.length > 0 && (
                    <div className="flex flex-wrap gap-2">
                      {itemAllergens.map((allergen, index) => (
                        <Chip
                          key={`allergen-${allergen}-${index}`}
                          onClose={() => removeAllergen(allergen)}
                          variant="flat"
                          color="warning"
                        >
                          {allergen}
                        </Chip>
                      ))}
                    </div>
                  )}
                </div>

                <Divider />

                {/* Dietary Tags */}
                <div className="space-y-4">
                  <h4 className="text-lg font-semibold text-gray-900 flex items-center gap-2">
                    <Tag className="w-5 h-5" />
                    Dietary Tags
                  </h4>
                  <div className="flex gap-2">
                    <Input
                      label="Dietary Tag"
                      placeholder="e.g., Vegetarian, Vegan, Gluten-Free"
                      value={newDietaryTag}
                      onValueChange={setNewDietaryTag}
                    onKeyPress={(e) => {
                        if (e.key === 'Enter') {
                          e.preventDefault();
                          addDietaryTag();
                        }
                      }}
                      />
                    <Button
                      color="success"
                      variant="flat"
                      onPress={addDietaryTag}
                      isDisabled={!newDietaryTag.trim()}
                    >
                      Add
                    </Button>
                  </div>
                  {itemDietaryTags.length > 0 && (
                    <div className="flex flex-wrap gap-2">
                      {itemDietaryTags.map((tag, index) => (
                        <Chip
                          key={`dietary-${tag}-${index}`}
                          onClose={() => removeDietaryTag(tag)}
                          variant="flat"
                          color="success"
                        >
                          {tag}
                        </Chip>
                      ))}
                    </div>
                  )}
                </div>
              </ModalBody>
              <ModalFooter>
                <Button color="danger" variant="light" onPress={() => {
                  resetItemForm();
                  onClose();
                }}>
                  Cancel
                </Button>
                <Button 
                  color="primary" 
                  onPress={handleUpdateItem}
                  isDisabled={!itemName.trim() || !itemPrice}
                >
                  Update Item
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </div>
  );
}
