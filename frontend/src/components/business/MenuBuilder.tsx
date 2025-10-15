'use client';

import React, { useState, useEffect, useCallback, useMemo } from 'react';
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
  Image,
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
import { businessApi, MenuItem, MenuCategory, MenuItemOption, Business } from '../../api/business';
import { getSupportedLanguages, getBusinessLanguages, updateBusinessLanguages, SupportedLanguage, BusinessLanguage } from '../../api/currency';
import { PrimarySpinner } from '../ui/spinners/PrimarySpinner';
import ImageUpload from './ImageUpload';
import MultipleImageUpload from './MultipleImageUpload';
import { Plus, Edit, Trash2, Image as ImageIcon, Tag, AlertTriangle, DollarSign, Search, X, Globe, Languages } from 'lucide-react';

interface MenuBuilderProps {
  businessId: number;
  initialMenu?: MenuCategory[];
  onMenuUpdate?: (menu: MenuCategory[]) => void;
}

export default function MenuBuilder({ businessId, initialMenu = [], onMenuUpdate }: MenuBuilderProps) {
  const [menu, setMenu] = useState<MenuCategory[]>(initialMenu);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  
  // Business state for currency settings
  const [business, setBusiness] = useState<Business | null>(null);
  const [defaultCurrency, setDefaultCurrency] = useState('USD');
  
  // Language states
  const [supportedLanguages, setSupportedLanguages] = useState<SupportedLanguage[]>([]);
  const [businessLanguages, setBusinessLanguages] = useState<BusinessLanguage[]>([]);
  const [selectedLanguages, setSelectedLanguages] = useState<string[]>([]);
  const [defaultLanguage, setDefaultLanguage] = useState<string>('en');
  const [isLanguageLoading, setIsLanguageLoading] = useState(false);
  const [currentViewLanguage, setCurrentViewLanguage] = useState<string>('en');
  
  // Modal states
  const { isOpen: isAddCategoryOpen, onOpen: onAddCategoryOpen, onOpenChange: onAddCategoryOpenChange } = useDisclosure();
  const { isOpen: isEditCategoryOpen, onOpen: onEditCategoryOpen, onOpenChange: onEditCategoryOpenChange } = useDisclosure();
  const { isOpen: isAddItemOpen, onOpen: onAddItemOpen, onOpenChange: onAddItemOpenChange } = useDisclosure();
  const { isOpen: isEditItemOpen, onOpen: onEditItemOpen, onOpenChange: onEditItemOpenChange } = useDisclosure();
  
  // Translation states
  const [isTranslating, setIsTranslating] = useState(false);
  
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

  // Search functionality
  const [searchQuery, setSearchQuery] = useState('');
  const [searchFilter, setSearchFilter] = useState<'all' | 'available' | 'unavailable'>('all');

  const loadBusiness = useCallback(async () => {
    try {
      const businessData = await businessApi.getBusiness(businessId);
      setBusiness(businessData);
      setDefaultCurrency(businessData.default_currency || 'USD');
    } catch (error) {
      console.error('Failed to load business data:', error);
    }
  }, [businessId]);

  const loadMenu = useCallback(async (language?: string) => {
    try {
      setIsLoading(true);
      setError(null);
      const menuData = await businessApi.getMenu(businessId, language);
      
      if (menuData) {
        let categories;
        
        // Use parsed_categories if available (contains translations), otherwise fall back to categories
        if (menuData.parsed_categories && Array.isArray(menuData.parsed_categories)) {
          categories = menuData.parsed_categories;
        } else if (menuData.categories) {
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
        } else {
          categories = [];
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

  // Simple translate menu function
  const handleTranslateMenu = useCallback(async (languageCode: string) => {
    try {
      setIsTranslating(true);
      setError(null);
      
      // Call backend to translate entire menu
      await businessApi.translateMenu(businessId, languageCode);
      
      // Reload menu with translated content
      await loadMenu(languageCode);
      setCurrentViewLanguage(languageCode);
      
    } catch (error) {
      console.error('Failed to translate menu:', error);
      setError('Failed to translate menu');
    } finally {
      setIsTranslating(false);
    }
  }, [businessId, loadMenu]);

  // Load business and menu data
  useEffect(() => {
    loadBusiness();
    loadMenu();
  }, [loadBusiness, loadMenu]);

  // Load language data
  const loadLanguages = useCallback(async () => {
    try {
      setIsLanguageLoading(true);
      
      // Load supported languages
      const supported = await getSupportedLanguages();
      setSupportedLanguages(supported);
      
      // Load business languages
      const business = await getBusinessLanguages(businessId);
      setBusinessLanguages(business);
      
      // Set selected languages and default
      const selectedCodes = business.map(bl => bl.language_code);
      setSelectedLanguages(selectedCodes);
      
      const defaultLang = business.find(bl => bl.is_default);
      if (defaultLang) {
        setDefaultLanguage(defaultLang.language_code);
        setCurrentViewLanguage(defaultLang.language_code);
      }
    } catch (error) {
      console.error('Failed to load languages:', error);
    } finally {
      setIsLanguageLoading(false);
    }
  }, [businessId]);

  useEffect(() => {
    loadLanguages();
  }, [loadLanguages]);

  // Handle language selection changes
  // Check if there are unsaved language changes
  const hasLanguageChanges = useMemo(() => {
    const currentCodes = businessLanguages.map(bl => bl.language_code).sort();
    const selectedCodes = selectedLanguages.slice().sort();
    const currentDefault = businessLanguages.find(bl => bl.is_default)?.language_code;
    
    return JSON.stringify(currentCodes) !== JSON.stringify(selectedCodes) || 
           currentDefault !== defaultLanguage;
  }, [businessLanguages, selectedLanguages, defaultLanguage]);

  const handleLanguageUpdate = useCallback(async (languageCodes: string[], defaultCode: string) => {
    try {
      setIsLanguageLoading(true);
      await updateBusinessLanguages(businessId, {
        language_codes: languageCodes,
        default_code: defaultCode
      });
      
      // Reload languages to get updated data
      await loadLanguages();
    } catch (error) {
      console.error('Failed to update languages:', error);
      setError('Failed to update languages');
    } finally {
      setIsLanguageLoading(false);
    }
  }, [businessId, loadLanguages]);

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
    // Currency now comes from business default
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
    // Currency is now set from business default, no longer per-item
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
        currency: defaultCurrency,
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
        currency: defaultCurrency,
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

  // Search and filter functionality
  const filteredMenu = React.useMemo(() => {
    if (!searchQuery.trim() && searchFilter === 'all') {
      return menu;
    }

    const query = searchQuery.toLowerCase().trim();
    
    return menu.map(category => {
      // Check if category matches search
      const categoryMatches = category.name.toLowerCase().includes(query) ||
                             category.description.toLowerCase().includes(query);

      // Filter items within category
      const filteredItems = category.items.filter(item => {
        // Text search
        const itemMatches = !query || 
          item.name.toLowerCase().includes(query) ||
          item.description.toLowerCase().includes(query) ||
          (item.allergens && item.allergens.some(allergen => allergen.toLowerCase().includes(query))) ||
          (item.dietary_tags && item.dietary_tags.some(tag => tag.toLowerCase().includes(query))) ||
          (item.options && item.options.some(option => option.name.toLowerCase().includes(query)));

        // Availability filter
        const availabilityMatches = searchFilter === 'all' ||
          (searchFilter === 'available' && item.is_available) ||
          (searchFilter === 'unavailable' && !item.is_available);

        return itemMatches && availabilityMatches;
      });

      // Include category if it matches search or has matching items
      if (categoryMatches || filteredItems.length > 0) {
        return {
          ...category,
          items: filteredItems
        };
      }

      return null;
    }).filter(Boolean) as MenuCategory[];
  }, [menu, searchQuery, searchFilter]);

  const clearSearch = () => {
    setSearchQuery('');
    setSearchFilter('all');
  };

  const getSearchResultsCount = () => {
    const totalItems = filteredMenu.reduce((total, category) => total + category.items.length, 0);
    const totalCategories = filteredMenu.length;
    return { totalItems, totalCategories };
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
      <div className="space-y-6">
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

        {/* Unified Language Management Section */}
        <Card className="border-gray-200">
          <CardBody className="p-6">
            <div className="space-y-6">
              {/* Header */}
              <div className="flex items-center gap-3">
                <div className="p-2 bg-blue-50 rounded-lg">
                  <Globe className="w-5 h-5 text-blue-600" />
                </div>
                <div>
                  <h3 className="text-lg font-semibold text-gray-900">Menu Languages</h3>
                  <p className="text-sm text-gray-600">Manage languages and translations for your menu</p>
                </div>
              </div>

              {/* Language Selection Row */}
              <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 items-end">
                {/* Select Languages */}
                <div className="space-y-2">
                  <label className="text-sm font-medium text-gray-700">Supported Languages</label>
                  <Select
                    placeholder="Select languages"
                    selectionMode="multiple"
                    selectedKeys={new Set(selectedLanguages)}
                    onSelectionChange={(keys) => {
                      const newSelection = Array.from(keys) as string[];
                      setSelectedLanguages(newSelection);
                      if (newSelection.length > 0 && !newSelection.includes(defaultLanguage)) {
                        setDefaultLanguage(newSelection[0]);
                      }
                    }}
                    isLoading={isLanguageLoading}
                    size="sm"
                  >
                    {supportedLanguages.map((lang) => (
                      <SelectItem key={lang.code} value={lang.code}>
                        {lang.native_name}
                      </SelectItem>
                    ))}
                  </Select>
                </div>

                {/* Default Language */}
                {selectedLanguages.length > 1 && (
                  <div className="space-y-2">
                    <label className="text-sm font-medium text-gray-700">Default Language</label>
                    <Select
                      placeholder="Choose default"
                      selectedKeys={new Set([defaultLanguage])}
                      onSelectionChange={(keys) => {
                        const newDefault = Array.from(keys)[0] as string;
                        if (newDefault) setDefaultLanguage(newDefault);
                      }}
                      isLoading={isLanguageLoading}
                      size="sm"
                    >
                      {selectedLanguages.map((langCode) => {
                        const lang = supportedLanguages.find(l => l.code === langCode);
                        return lang ? (
                          <SelectItem key={lang.code} value={lang.code}>
                            {lang.native_name}
                          </SelectItem>
                        ) : null;
                      })}
                    </Select>
                  </div>
                )}

                {/* Actions */}
                <div className="flex gap-2">
                  {hasLanguageChanges && (
                    <Button
                      color="primary"
                      size="sm"
                      onPress={() => handleLanguageUpdate(selectedLanguages, defaultLanguage)}
                      isLoading={isLanguageLoading}
                      isDisabled={selectedLanguages.length === 0}
                    >
                      Save Languages
                    </Button>
                  )}
                </div>
              </div>

              {/* View & Translate Row */}
              {businessLanguages.length > 1 && (
                <div className="space-y-3 pt-4 border-t border-gray-200">
                  <div className="flex flex-col sm:flex-row gap-4 items-start sm:items-center justify-between">
                    {/* View Language Switcher */}
                    <div className="space-y-2">
                      <label className="text-sm font-medium text-gray-700">View Menu As:</label>
                      <div className="flex flex-wrap gap-2">
                        {businessLanguages.map((bl) => {
                          const lang = supportedLanguages.find(l => l.code === bl.language_code);
                          if (!lang) return null;
                          
                          const isActive = currentViewLanguage === bl.language_code;
                          
                          return (
                            <Button
                              key={bl.language_code}
                              size="sm"
                              variant={isActive ? "solid" : "bordered"}
                              color={isActive ? "primary" : "default"}
                              onPress={() => {
                                setCurrentViewLanguage(bl.language_code);
                                loadMenu(bl.language_code);
                              }}
                            >
                              {lang.native_name}
                              {bl.is_default && " (Default)"}
                            </Button>
                          );
                        })}
                      </div>
                    </div>

                    {/* Translate Button */}
                    {currentViewLanguage !== 'en' && (
                      <Button
                        color="secondary"
                        size="sm"
                        variant="flat"
                        onPress={() => handleTranslateMenu(currentViewLanguage)}
                        startContent={<Languages className="w-4 h-4" />}
                        isLoading={isTranslating}
                        className="min-w-[140px]"
                      >
                        {isTranslating ? 'Translating...' : `Translate to ${supportedLanguages.find(l => l.code === currentViewLanguage)?.native_name}`}
                      </Button>
                    )}
                  </div>

                  {/* Status */}
                  <div className="text-xs text-gray-500">
                    {currentViewLanguage === defaultLanguage 
                      ? "Showing original content" 
                      : "Showing translated content"}
                  </div>
                </div>
              )}
            </div>
          </CardBody>
        </Card>

        {/* Search and Filter Section */}
        <Card className="border-gray-200">
          <CardBody className="p-4">
            <div className="flex flex-col sm:flex-row gap-4 items-start sm:items-center">
              <div className="flex-1 relative">
                <Input
                  placeholder="Search menu items, categories, allergens, or dietary tags..."
                  value={searchQuery}
                  onValueChange={setSearchQuery}
                  startContent={<Search className="w-4 h-4 text-gray-400" />}
                  endContent={
                    searchQuery && (
                      <Button
                        isIconOnly
                        size="sm"
                        variant="light"
                        onPress={clearSearch}
                        className="min-w-0 w-6 h-6"
                      >
                        <X className="w-3 h-3" />
                      </Button>
                    )
                  }
                  className="w-full"
                />
              </div>
              
              <div className="flex items-center gap-3">
                <Select
                  label="Filter by availability"
                  selectedKeys={[searchFilter]}
                  onSelectionChange={(keys) => setSearchFilter(Array.from(keys)[0] as 'all' | 'available' | 'unavailable')}
                  className="w-48"
                  size="sm"
                >
                  <SelectItem key="all" value="all">All Items</SelectItem>
                  <SelectItem key="available" value="available">Available Only</SelectItem>
                  <SelectItem key="unavailable" value="unavailable">Unavailable Only</SelectItem>
                </Select>
                
                {(searchQuery || searchFilter !== 'all') && (
                  <Button
                    size="sm"
                    variant="flat"
                    onPress={clearSearch}
                    startContent={<X className="w-3 h-3" />}
                  >
                    Clear
                  </Button>
                )}
              </div>
            </div>

            {/* Search Results Summary */}
            {(searchQuery || searchFilter !== 'all') && (
              <div className="mt-3 pt-3 border-t border-gray-200">
                <div className="flex items-center justify-between">
                  <div className="text-sm text-gray-600">
                    {(() => {
                      const { totalItems, totalCategories } = getSearchResultsCount();
                      return (
                        <span>
                          Found <strong>{totalItems}</strong> items in <strong>{totalCategories}</strong> categories
                          {searchQuery && (
                            <span> matching &quot;<strong>{searchQuery}</strong>&quot;</span>
                          )}
                          {searchFilter !== 'all' && (
                            <span> ({searchFilter} items)</span>
                          )}
                        </span>
                      );
                    })()}
                  </div>
                  
                  {searchQuery && (
                    <Chip size="sm" variant="flat" color="primary">
                      Search active
                    </Chip>
                  )}
                </div>
              </div>
            )}
          </CardBody>
        </Card>
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
      ) : filteredMenu.length === 0 ? (
        <div className="bg-white border border-gray-200 rounded-2xl p-16 text-center shadow-sm">
          <div className="w-20 h-20 bg-gray-50 rounded-2xl flex items-center justify-center mx-auto mb-8 border border-gray-100">
            <Search className="h-10 w-10 text-gray-400" />
          </div>
          <h3 className="text-2xl font-light text-gray-900 tracking-wide mb-4">No results found</h3>
          <p className="text-gray-600 font-light leading-relaxed mb-8 max-w-md mx-auto">
            No menu items match your search criteria. Try adjusting your search terms or filters.
          </p>
          <Button
            onPress={clearSearch}
            color="primary"
            variant="flat"
            startContent={<X className="w-4 h-4" />}
          >
            Clear Search
          </Button>
        </div>
      ) : (
        <div className="space-y-8">
          {Array.isArray(filteredMenu) && filteredMenu.map((category, categoryIndex) => {
            // Find the original category index for edit/delete operations
            const originalCategoryIndex = menu.findIndex(originalCategory => 
              originalCategory.name === category.name
            );
            
            return (
            <Card key={categoryIndex} className="shadow-lg border-0">
              <CardHeader className="pb-4">
                <div className="flex justify-between items-start w-full">
                  <div className="flex-1">
                    <h3 className="text-2xl font-light text-gray-900 tracking-wide mb-2">
                      {category.name}
                    </h3>
                    {category.description && (
                      <p className="text-gray-600 font-light leading-relaxed">
                        {category.description}
                      </p>
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
                      onPress={() => handleEditCategory(originalCategoryIndex)}
                    >
                      Edit
                    </Button>
                    <Button
                      size="sm"
                      variant="flat"
                      color="danger"
                      startContent={<Trash2 className="w-4 h-4" />}
                      onPress={() => handleDeleteCategory(originalCategoryIndex)}
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
                        setSelectedCategoryIndex(originalCategoryIndex);
                        resetItemForm();
                        onAddItemOpen();
                      }}
                    >
                      Add First Item
                    </Button>
                  </div>
                ) : (
                  <div className="grid grid-cols-1 lg:grid-cols-2 xl:grid-cols-3 gap-6">
                    {category.items.map((item, itemIndex) => {
                      // Find the original item index for edit/delete operations
                      const originalItemIndex = menu[originalCategoryIndex]?.items.findIndex(originalItem => 
                        originalItem.name === item.name && originalItem.price === item.price
                      ) || itemIndex;
                      
                      return (
                      <Card key={itemIndex} className="group hover:shadow-lg transition-all duration-300 border-gray-200 overflow-hidden h-full">
                        <CardBody className="p-0 flex flex-col h-full">
                          {/* Image Section */}
                          <div className="relative h-48 bg-gray-100 overflow-hidden">
                            {item.images && item.images.length > 0 ? (
                              <div className="relative w-full h-full">
                                <Image
                                  src={item.images[0]}
                                  alt={item.name}
                                  className="w-full h-full object-cover transition-transform duration-300 group-hover:scale-105"
                                  radius="lg"
                                />
                                {item.images.length > 1 && (
                                  <div className="absolute bottom-2 right-2 bg-black/70 text-white text-xs px-2 py-1 rounded-full">
                                    +{item.images.length - 1} more
                                  </div>
                                )}
                                {/* Availability overlay */}
                                {!item.is_available && (
                                  <div className="absolute inset-0 bg-black/50 flex items-center justify-center">
                                    <Chip color="default" variant="solid" className="text-white">
                                      Unavailable
                                    </Chip>
                                  </div>
                                )}
                              </div>
                            ) : item.image ? (
                              <div className="relative w-full h-full">
                                <Image
                                  src={item.image}
                                  alt={item.name}
                                  className="w-full h-full object-cover transition-transform duration-300 group-hover:scale-105"
                                  radius="lg"
                                />
                                {!item.is_available && (
                                  <div className="absolute inset-0 bg-black/50 flex items-center justify-center">
                                    <Chip color="default" variant="solid" className="text-white">
                                      Unavailable
                                    </Chip>
                                  </div>
                                )}
                              </div>
                            ) : (
                              <div className="w-full h-full flex items-center justify-center bg-gradient-to-br from-gray-100 to-gray-200">
                                <div className="text-center">
                                  <ImageIcon className="w-12 h-12 text-gray-400 mx-auto mb-2" />
                                  <p className="text-sm text-gray-500">No image</p>
                                </div>
                                {!item.is_available && (
                                  <div className="absolute inset-0 bg-black/50 flex items-center justify-center">
                                    <Chip color="default" variant="solid" className="text-white">
                                      Unavailable
                                    </Chip>
                                  </div>
                                )}
                              </div>
                            )}
                            
                            {/* Availability badge for available items */}
                            {item.is_available && (
                              <div className="absolute top-2 left-2">
                                <Chip size="sm" color="success" variant="solid" className="text-white">
                                  Available
                                </Chip>
                              </div>
                            )}

                          </div>

                          {/* Content Section */}
                          <div className="p-4 space-y-3 flex-grow flex flex-col">
                            {/* Header with name and price */}
                            <div className="flex justify-between items-start">
                              <h4 className="font-semibold text-gray-900 tracking-wide text-lg leading-tight flex-1 pr-2">
                                {item.name}
                              </h4>
                              <div className="flex items-center gap-1 text-green-600 font-bold text-lg">
                                <DollarSign className="w-4 h-4" />
                                <span>{(item.price || 0).toFixed(2)}</span>
                                {item.currency && item.currency !== 'USD' && (
                                  <span className="text-sm text-gray-500 ml-1">{item.currency}</span>
                                )}
                              </div>
                            </div>
                            
                            {/* Description */}
                            {item.description && (
                              <p className="text-sm text-gray-600 font-light leading-relaxed" style={{
                                display: '-webkit-box',
                                WebkitLineClamp: 2,
                                WebkitBoxOrient: 'vertical',
                                overflow: 'hidden'
                              }}>
                                {item.description}
                              </p>
                            )}

                            {/* Options preview */}
                            {item.options && item.options.length > 0 && (
                              <div className="flex items-center gap-2">
                                <span className="text-xs text-gray-500">Options:</span>
                                <Chip size="sm" variant="flat" color="primary" className="text-xs">
                                  +{item.options.length} add-ons
                                </Chip>
                              </div>
                            )}

                            {/* Tags and allergens */}
                            <div className="space-y-2">
                              {item.dietary_tags && item.dietary_tags.length > 0 && (
                                <div className="flex flex-wrap gap-1">
                                  {item.dietary_tags.slice(0, 3).map((tag, idx) => (
                                    <Chip key={idx} size="sm" color="success" variant="flat" className="text-xs">
                                      {tag}
                                    </Chip>
                                  ))}
                                  {item.dietary_tags.length > 3 && (
                                    <Chip size="sm" variant="flat" color="default" className="text-xs">
                                      +{item.dietary_tags.length - 3} more
                                    </Chip>
                                  )}
                                </div>
                              )}
                              
                              {item.allergens && item.allergens.length > 0 && (
                                <div className="flex items-center gap-2">
                                  <AlertTriangle className="w-3 h-3 text-orange-500" />
                                  <div className="flex flex-wrap gap-1">
                                    {item.allergens.slice(0, 2).map((allergen, idx) => (
                                      <Chip key={idx} size="sm" color="warning" variant="flat" className="text-xs">
                                        {allergen}
                                      </Chip>
                                    ))}
                                    {item.allergens.length > 2 && (
                                      <Chip size="sm" variant="flat" color="warning" className="text-xs">
                                        +{item.allergens.length - 2} more
                                      </Chip>
                                    )}
                                  </div>
                                </div>
                              )}
                            </div>

                            {/* Spacer to push action buttons to bottom */}
                            <div className="flex-grow"></div>

                            {/* Action buttons - always at bottom */}
                            <div className="flex gap-2 pt-4 mt-auto border-t border-gray-100">
                              <Button
                                size="sm"
                                variant="flat"
                                color="default"
                                startContent={<Edit className="w-3 h-3" />}
                                onPress={() => handleEditItem(originalCategoryIndex, originalItemIndex)}
                                className="flex-1"
                              >
                                Edit
                              </Button>
                              <Button
                                size="sm"
                                variant="flat"
                                color="danger"
                                startContent={<Trash2 className="w-3 h-3" />}
                                onPress={() => handleDeleteItem(originalCategoryIndex, originalItemIndex)}
                                className="flex-1"
                              >
                                Delete
                              </Button>
                            </div>

                          </div>
                        </CardBody>
                      </Card>
                      );
                    })}
                  </div>
                )}
              </CardBody>
            </Card>
            );
          })}
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
                      <div className="w-32">
                        <label className="text-sm text-gray-600 mb-1 block">Currency</label>
                        <div className="flex items-center h-10 px-3 bg-gray-50 rounded-lg border">
                          <span className="text-sm font-medium text-gray-700">{defaultCurrency}</span>
                        </div>
                        <p className="text-xs text-gray-500 mt-1">From business settings</p>
                      </div>
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
                      <div className="w-32">
                        <label className="text-sm text-gray-600 mb-1 block">Currency</label>
                        <div className="flex items-center h-10 px-3 bg-gray-50 rounded-lg border">
                          <span className="text-sm font-medium text-gray-700">{defaultCurrency}</span>
                        </div>
                        <p className="text-xs text-gray-500 mt-1">From business settings</p>
                      </div>
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
