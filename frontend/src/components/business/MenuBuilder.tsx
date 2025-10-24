'use client';

import React, { useState, useEffect, useCallback, useMemo } from 'react';
import { useSimpleLocale, getTranslation } from '@/i18n/SimpleTranslationProvider';
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
import AddCategoryModal from './modals/AddCategoryModal';
import EditCategoryModal from './modals/EditCategoryModal';
import AddItemModal from './modals/AddItemModal';
import EditItemModal from './modals/EditItemModal';

interface MenuBuilderProps {
  businessId: number;
  initialMenu?: MenuCategory[];
  onMenuUpdate?: (menu: MenuCategory[]) => void;
}

export default function MenuBuilder({ businessId, initialMenu = [], onMenuUpdate }: MenuBuilderProps) {
  // Translation setup
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);
  
  // Update translations when locale changes
  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);
  
  // Translation helper
  const tString = useCallback((key: string): string => {
    const fullKey = `businessDashboard.dashboard.menuBuilder.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : result as string;
  }, [currentLocale]);

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
      setError(tString('error'));
      setMenu([]); // Ensure menu is always an array even on error
    } finally {
      setIsLoading(false);
    }
  }, [businessId, tString]);

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
      setError(tString('languages.translating'));
    } finally {
      setIsTranslating(false);
    }
  }, [businessId, loadMenu, tString]);

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
      setError(tString('languages.saveFirst'));
    } finally {
      setIsLanguageLoading(false);
    }
  }, [businessId, loadLanguages, tString]);

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
    setItemOptions((item.options || []).map(option => ({
      ...option,
      id: option.id || `option-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`,
      is_required: option.is_required ?? false
    })));
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
      id: `option-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`,
      name: newOptionName.trim(),
      price_change: price,
      is_required: false
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
          <p className="text-gray-600 font-light tracking-wide">{tString('loading')}</p>
        </div>
      </div>
    );
  }

  return (
    <div className="p-4 max-w-6xl mx-auto">
      {/* Compact Header */}
      <div className="flex justify-between items-center mb-6">
        <div>
          <h1 className="text-2xl font-light text-gray-900 tracking-wide">{tString('title')}</h1>
          <p className="text-gray-600 font-light text-sm mt-1">{tString('subtitle')}</p>
        </div>
        <button
          onClick={onAddCategoryOpen}
          className="bg-gray-900 text-white px-4 py-2 rounded-lg font-medium hover:bg-gray-800 transition-colors flex items-center gap-2"
        >
          <Plus className="w-4 h-4" />
          {tString('categories.addCategory')}
        </button>
      </div>

        {/* Language Management Section */}
        <div className="bg-white border border-gray-200 rounded-lg p-4 mb-6">
          <div className="space-y-4">
            {/* Header */}
            <div className="flex items-center gap-3">
              <div className="w-8 h-8 bg-gray-50 rounded-lg flex items-center justify-center">
                <Globe className="w-4 h-4 text-gray-600" />
              </div>
              <div>
                <h3 className="text-sm font-medium text-gray-900">{tString('languages.title')}</h3>
                <p className="text-xs text-gray-600">{tString('languages.subtitle')}</p>
              </div>
            </div>

            {/* Language Selection Row */}
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-3">
              {/* Select Languages */}
              <div>
                <label className="text-xs font-medium text-gray-700 mb-1 block">{tString('languages.supportedLanguages')}</label>
                <Select
                  placeholder={tString('languages.selectLanguages')}
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
                <div>
                  <label className="text-xs font-medium text-gray-700 mb-1 block">{tString('languages.defaultLanguage')}</label>
                  <Select
                    placeholder={tString('languages.setAsDefault')}
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
              <div className="flex items-end">
                {hasLanguageChanges && (
                  <Button
                    color="primary"
                    size="sm"
                    onPress={() => handleLanguageUpdate(selectedLanguages, defaultLanguage)}
                    isLoading={isLanguageLoading}
                    isDisabled={selectedLanguages.length === 0}
                    className="w-full"
                  >
                    {tString('languages.saveLanguages')}
                  </Button>
                )}
              </div>
            </div>

            {/* View & Translate Row */}
            {businessLanguages.length > 1 && (
              <div className="pt-3 border-t border-gray-200">
                <div className="flex flex-col sm:flex-row gap-3 items-start sm:items-center justify-between">
                  {/* View Language Switcher */}
                  <div>
                    <label className="text-xs font-medium text-gray-700 mb-2 block">{tString('languages.currentView')}:</label>
                    <div className="flex flex-wrap gap-2">
                      {businessLanguages.map((bl) => {
                        const lang = supportedLanguages.find(l => l.code === bl.language_code);
                        if (!lang) return null;
                        
                        const isActive = currentViewLanguage === bl.language_code;
                        
                        return (
                          <button
                            key={bl.language_code}
                            onClick={() => {
                              setCurrentViewLanguage(bl.language_code);
                              loadMenu(bl.language_code);
                            }}
                            className={`px-3 py-1 text-xs rounded-lg font-medium transition-colors ${
                              isActive 
                                ? 'bg-gray-900 text-white' 
                                : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
                            }`}
                          >
                            {lang.native_name}
                            {bl.is_default && ' (Default)'}
                          </button>
                        );
                      })}
                    </div>
                  </div>

                  {/* Translate Button */}
                  {currentViewLanguage !== 'en' && (
                    <button
                      onClick={() => handleTranslateMenu(currentViewLanguage)}
                      disabled={isTranslating}
                      className="bg-gray-100 text-gray-700 hover:bg-gray-200 px-3 py-1 text-xs rounded-lg font-medium transition-colors flex items-center gap-2 disabled:opacity-50"
                    >
                      <Languages className="w-3 h-3" />
                      {isTranslating ? tString('languages.translating') : `Translate to ${supportedLanguages.find(l => l.code === currentViewLanguage)?.native_name}`}
                    </button>
                  )}
                </div>

                {/* Status */}
                <div className="text-xs text-gray-500 mt-2">
                  {currentViewLanguage === defaultLanguage 
                    ? "Showing original content" 
                    : "Showing translated content"}
                </div>
              </div>
            )}
          </div>
        </div>

      {/* Compact Search Bar */}
      <div className="bg-white border border-gray-200 rounded-lg p-4 mb-6">
        <div className="flex flex-col sm:flex-row gap-3">
          <div className="flex-1">
            <Input
              placeholder={tString('search.placeholder')}
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
              size="sm"
            />
          </div>
          
          <div className="flex items-center gap-2">
            <Select
              placeholder="Filter by availability"
              selectedKeys={[searchFilter]}
              onSelectionChange={(keys) => setSearchFilter(Array.from(keys)[0] as 'all' | 'available' | 'unavailable')}
              className="w-40"
              size="sm"
            >
              <SelectItem key="all" value="all">{tString('search.filters.all')}</SelectItem>
              <SelectItem key="available" value="available">{tString('search.filters.available')}</SelectItem>
              <SelectItem key="unavailable" value="unavailable">{tString('search.filters.unavailable')}</SelectItem>
            </Select>
            
            {(searchQuery || searchFilter !== 'all') && (
              <Button
                size="sm"
                variant="flat"
                onPress={clearSearch}
                startContent={<X className="w-3 h-3" />}
              >
                {tString('buttons.clear')}
              </Button>
            )}
          </div>
        </div>

        {/* Compact Search Results */}
        {(searchQuery || searchFilter !== 'all') && (
          <div className="mt-3 pt-3 border-t border-gray-200">
            <div className="text-sm text-gray-600">
              {(() => {
                const { totalItems, totalCategories } = getSearchResultsCount();
                return (
                  <>
                    Found <strong>{totalItems}</strong> items in <strong>{totalCategories}</strong> categories
                    {searchQuery && <span> matching "{searchQuery}"</span>}
                    {searchFilter !== 'all' && <span> ({searchFilter})</span>}
                  </>
                );
              })()}
            </div>
          </div>
        )}
      </div>

      {error && (
        <div className="bg-red-50 border border-red-200 rounded-lg p-3 mb-4">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2">
              <div className="w-5 h-5 bg-red-100 rounded flex items-center justify-center">
                <X className="w-3 h-3 text-red-600" />
              </div>
              <p className="text-red-700 text-sm">{error}</p>
            </div>
            <button
              onClick={() => setError(null)}
              className="text-red-600 hover:text-red-800 text-sm"
            >
              ×
            </button>
          </div>
        </div>
      )}

      {!Array.isArray(menu) || menu.length === 0 ? (
        <div className="bg-white border border-gray-200 rounded-lg p-8 text-center">
          <div className="w-12 h-12 bg-gray-50 rounded-lg flex items-center justify-center mx-auto mb-4">
            <svg className="h-6 w-6 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.746 0 3.332.477 4.5 1.253v13C19.832 18.477 18.246 18 16.5 18c-1.746 0-3.332.477-4.5 1.253" />
            </svg>
          </div>
          <h3 className="text-lg font-light text-gray-900 mb-2">{tString('noCategories')}</h3>
          <p className="text-gray-600 font-light leading-relaxed mb-8 max-w-md mx-auto">
            {tString('categories.createFirstDescription')}
          </p>
          <button
            onClick={onAddCategoryOpen}
            className="bg-gray-900 text-white px-8 py-3 text-base font-medium hover:bg-gray-800 transition-all duration-200 tracking-wide rounded-lg shadow-lg hover:shadow-xl"
          >
            {tString('categories.addCategory')}
          </button>
        </div>
      ) : filteredMenu.length === 0 ? (
        <div className="bg-white border border-gray-200 rounded-lg p-8 text-center">
          <div className="w-12 h-12 bg-gray-50 rounded-lg flex items-center justify-center mx-auto mb-4">
            <Search className="h-6 w-6 text-gray-400" />
          </div>
          <h3 className="text-lg font-light text-gray-900 mb-2">{tString('search.noResults')}</h3>
          <p className="text-gray-600 text-sm mb-4">
            {tString('search.noResultsDescription')}
          </p>
          <Button
            onPress={clearSearch}
            color="primary"
            variant="flat"
            size="sm"
            startContent={<X className="w-4 h-4" />}
          >
            {tString('buttons.clear')}
          </Button>
        </div>
      ) : (
        <div className="space-y-6">
          {Array.isArray(filteredMenu) && filteredMenu.map((category, categoryIndex) => {
            // Find the original category index for edit/delete operations
            const originalCategoryIndex = menu.findIndex(originalCategory => 
              originalCategory.name === category.name
            );
            
            return (
            <div key={categoryIndex} className="bg-white border border-gray-200 rounded-lg p-6">
              {/* Category Header */}
              <div className="flex justify-between items-start mb-4">
                <div className="flex-1">
                  <h3 className="text-lg font-medium text-gray-900 mb-1">
                    {category.name}
                  </h3>
                  {category.description && (
                    <p className="text-sm text-gray-600 mb-2">
                      {category.description}
                    </p>
                  )}
                  <div className={`inline-flex px-2 py-1 rounded text-xs font-medium ${
                    category.items.length > 0 
                      ? 'bg-green-100 text-green-700' 
                      : 'bg-gray-100 text-gray-700'
                  }`}>
                    {category.items.length} {category.items.length === 1 ? 'item' : 'items'}
                  </div>
                </div>
                <div className="flex gap-2">
                  <button
                    onClick={() => {
                      setSelectedCategoryIndex(originalCategoryIndex);
                      resetItemForm();
                      onAddItemOpen();
                    }}
                    className="bg-gray-900 text-white px-3 py-1 text-xs rounded-lg font-medium hover:bg-gray-800 transition-colors flex items-center gap-1"
                  >
                    <Plus className="w-3 h-3" />
                    Add Item
                  </button>
                  <button
                    onClick={() => handleEditCategory(originalCategoryIndex)}
                    className="bg-gray-100 text-gray-700 hover:bg-gray-200 px-3 py-1 text-xs rounded-lg font-medium transition-colors flex items-center gap-1"
                  >
                    <Edit className="w-3 h-3" />
                    Edit
                  </button>
                  <button
                    onClick={() => handleDeleteCategory(originalCategoryIndex)}
                    className="text-red-600 hover:bg-red-50 px-3 py-1 text-xs rounded-lg font-medium transition-colors flex items-center gap-1"
                  >
                    <Trash2 className="w-3 h-3" />
                    Delete
                  </button>
                </div>
              </div>
              {/* Items Section */}
              {category.items.length === 0 ? (
                <div className="bg-gray-50 rounded-lg p-6 text-center">
                  <div className="w-10 h-10 bg-gray-100 rounded-lg flex items-center justify-center mx-auto mb-3">
                    <Plus className="w-5 h-5 text-gray-400" />
                  </div>
                  <p className="text-gray-500 text-sm mb-3">{tString('noItems')}</p>
                  <button
                    onClick={() => {
                      setSelectedCategoryIndex(originalCategoryIndex);
                      resetItemForm();
                      onAddItemOpen();
                    }}
                    className="bg-gray-900 text-white px-3 py-1 text-xs font-medium hover:bg-gray-800 transition-colors rounded-lg"
                  >
                    {tString('items.addItem')}
                  </button>
                </div>
              ) : (
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                  {category.items.map((item, itemIndex) => {
                    // Find the original item index for edit/delete operations
                    const originalItemIndex = menu[originalCategoryIndex]?.items.findIndex(originalItem => 
                      originalItem.name === item.name && originalItem.price === item.price
                    ) || itemIndex;
                    
                    return (
                    <div key={itemIndex} className="bg-white border border-gray-200 rounded-lg overflow-hidden hover:shadow-md hover:border-gray-300 transition-all duration-200">
                      {/* Item Image */}
                      <div className="relative h-32 bg-gray-100">
                        {item.images && item.images.length > 0 ? (
                          <>
                            <Image
                              src={item.images[0]}
                              alt={item.name}
                              className="w-full h-full object-cover"
                              radius="none"
                            />
                            {item.images.length > 1 && (
                              <div className="absolute bottom-2 right-2 bg-black/70 text-white text-xs px-2 py-1 rounded">
                                +{item.images.length - 1} more
                              </div>
                            )}
                          </>
                        ) : item.image ? (
                          <Image
                            src={item.image}
                            alt={item.name}
                            className="w-full h-full object-cover"
                            radius="none"
                          />
                        ) : (
                          <div className="w-full h-full flex items-center justify-center bg-gray-100">
                            <div className="text-center">
                              <ImageIcon className="w-8 h-8 text-gray-400 mx-auto mb-1" />
                              <p className="text-xs text-gray-500">No Image</p>
                            </div>
                          </div>
                        )}
                        
                        {/* Availability overlay */}
                        {!item.is_available && (
                          <div className="absolute inset-0 bg-black/50 flex items-center justify-center">
                            <span className="bg-gray-800 text-white text-xs px-2 py-1 rounded">
                              Unavailable
                            </span>
                          </div>
                        )}
                        
                        {/* Availability badge for available items */}
                        {item.is_available && (
                          <div className="absolute top-2 left-2">
                            <span className="bg-green-500 text-white text-xs px-2 py-1 rounded">
                              Available
                            </span>
                          </div>
                        )}
                      </div>

                      {/* Item Content */}
                      <div className="p-4">
                        {/* Item Header */}
                        <div className="flex justify-between items-start mb-3">
                          <div className="flex-1">
                            <h4 className="text-sm font-medium text-gray-900 leading-tight">
                              {item.name}
                            </h4>
                            <div className="flex items-center gap-1 text-green-600 font-medium text-sm mt-1">
                              <DollarSign className="w-3 h-3" />
                              <span>{(item.price || 0).toFixed(2)}</span>
                              {item.currency && item.currency !== 'USD' && (
                                <span className="text-xs text-gray-500 ml-1">{item.currency}</span>
                              )}
                            </div>
                          </div>
                        </div>

                      {/* Description */}
                      {item.description && (
                        <p className="text-xs text-gray-600 mb-3 line-clamp-2">
                          {item.description}
                        </p>
                      )}

                      {/* Options and Tags */}
                      <div className="space-y-2 mb-4">
                        {item.options && item.options.length > 0 && (
                          <div className="flex items-center gap-2">
                            <span className="text-xs text-gray-500">Options:</span>
                            <span className="text-xs bg-blue-100 text-blue-700 px-2 py-1 rounded">
                              +{item.options.length}
                            </span>
                          </div>
                        )}
                        
                        {item.dietary_tags && item.dietary_tags.length > 0 && (
                          <div className="flex flex-wrap gap-1">
                            {item.dietary_tags.slice(0, 2).map((tag, idx) => (
                              <span key={idx} className="text-xs bg-green-100 text-green-700 px-2 py-1 rounded">
                                {tag}
                              </span>
                            ))}
                            {item.dietary_tags.length > 2 && (
                              <span className="text-xs bg-gray-100 text-gray-700 px-2 py-1 rounded">
                                +{item.dietary_tags.length - 2}
                              </span>
                            )}
                          </div>
                        )}
                        
                        {item.allergens && item.allergens.length > 0 && (
                          <div className="flex items-center gap-2">
                            <AlertTriangle className="w-3 h-3 text-orange-500" />
                            <div className="flex flex-wrap gap-1">
                              {item.allergens.slice(0, 2).map((allergen, idx) => (
                                <span key={idx} className="text-xs bg-orange-100 text-orange-700 px-2 py-1 rounded">
                                  {allergen}
                                </span>
                              ))}
                              {item.allergens.length > 2 && (
                                <span className="text-xs bg-gray-100 text-gray-700 px-2 py-1 rounded">
                                  +{item.allergens.length - 2}
                                </span>
                              )}
                            </div>
                          </div>
                        )}
                      </div>

                        {/* Action Buttons */}
                        <div className="flex gap-2">
                          <button
                            onClick={() => handleEditItem(originalCategoryIndex, originalItemIndex)}
                            className="flex-1 bg-gray-900 text-white px-3 py-1 text-xs font-medium hover:bg-gray-800 transition-colors rounded-lg flex items-center justify-center gap-1"
                          >
                            <Edit className="w-3 h-3" />
                            Edit
                          </button>
                          <button
                            onClick={() => handleDeleteItem(originalCategoryIndex, originalItemIndex)}
                            className="text-red-600 hover:bg-red-50 px-3 py-1 text-xs font-medium transition-colors rounded-lg flex items-center justify-center gap-1"
                          >
                            <Trash2 className="w-3 h-3" />
                            Delete
                          </button>
                        </div>
                      </div>
                    </div>
                    );
                  })}
                </div>
              )}
            </div>
            );
          })}
        </div>
      )}

      <AddCategoryModal
        isOpen={isAddCategoryOpen}
        onOpenChange={onAddCategoryOpenChange}
        categoryName={categoryName}
        setCategoryName={setCategoryName}
        categoryDescription={categoryDescription}
        setCategoryDescription={setCategoryDescription}
        onAddCategory={handleAddCategory}
        onResetForm={resetCategoryForm}
        tString={tString}
      />

      <EditCategoryModal
        isOpen={isEditCategoryOpen}
        onOpenChange={onEditCategoryOpenChange}
        categoryName={categoryName}
        setCategoryName={setCategoryName}
        categoryDescription={categoryDescription}
        setCategoryDescription={setCategoryDescription}
        onUpdateCategory={handleUpdateCategory}
        onResetForm={resetCategoryForm}
        tString={tString}
      />

      <AddItemModal
        isOpen={isAddItemOpen}
        onOpenChange={onAddItemOpenChange}
        selectedCategoryIndex={selectedCategoryIndex}
        menu={menu}
        itemName={itemName}
        setItemName={setItemName}
        itemDescription={itemDescription}
        setItemDescription={setItemDescription}
        itemPrice={itemPrice}
        setItemPrice={setItemPrice}
        defaultCurrency={defaultCurrency}
        itemImages={itemImages}
        setItemImages={setItemImages}
        itemAvailable={itemAvailable}
        setItemAvailable={setItemAvailable}
        itemSortOrder={itemSortOrder}
        setItemSortOrder={setItemSortOrder}
        businessId={businessId}
        itemOptions={itemOptions}
        newOptionName={newOptionName}
        setNewOptionName={setNewOptionName}
        newOptionPrice={newOptionPrice}
        setNewOptionPrice={setNewOptionPrice}
        onAddOption={addOption}
        onRemoveOption={removeOption}
        itemAllergens={itemAllergens}
        newAllergen={newAllergen}
        setNewAllergen={setNewAllergen}
        onAddAllergen={addAllergen}
        onRemoveAllergen={removeAllergen}
        itemDietaryTags={itemDietaryTags}
        newDietaryTag={newDietaryTag}
        setNewDietaryTag={setNewDietaryTag}
        onAddDietaryTag={addDietaryTag}
        onRemoveDietaryTag={removeDietaryTag}
        onAddItem={handleAddItem}
        onResetForm={resetItemForm}
        tString={tString}
      />

      <EditItemModal
        isOpen={isEditItemOpen}
        onOpenChange={onEditItemOpenChange}
        selectedCategoryIndex={selectedCategoryIndex}
        menu={menu}
        itemName={itemName}
        setItemName={setItemName}
        itemDescription={itemDescription}
        setItemDescription={setItemDescription}
        itemPrice={itemPrice}
        setItemPrice={setItemPrice}
        defaultCurrency={defaultCurrency}
        itemImages={itemImages}
        setItemImages={setItemImages}
        itemAvailable={itemAvailable}
        setItemAvailable={setItemAvailable}
        itemSortOrder={itemSortOrder}
        setItemSortOrder={setItemSortOrder}
        businessId={businessId}
        itemOptions={itemOptions}
        newOptionName={newOptionName}
        setNewOptionName={setNewOptionName}
        newOptionPrice={newOptionPrice}
        setNewOptionPrice={setNewOptionPrice}
        onAddOption={addOption}
        onRemoveOption={removeOption}
        itemAllergens={itemAllergens}
        newAllergen={newAllergen}
        setNewAllergen={setNewAllergen}
        onAddAllergen={addAllergen}
        onRemoveAllergen={removeAllergen}
        itemDietaryTags={itemDietaryTags}
        newDietaryTag={newDietaryTag}
        setNewDietaryTag={setNewDietaryTag}
        onAddDietaryTag={addDietaryTag}
        onRemoveDietaryTag={removeDietaryTag}
        onUpdateItem={handleUpdateItem}
        onResetForm={resetItemForm}
        tString={tString}
      />

    </div>
  );
}
