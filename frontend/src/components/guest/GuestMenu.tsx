'use client';

import React, { useState } from 'react';
import {
  Card,
  CardBody,
  CardHeader,
  Button,
  Chip,
  Image,
  Divider,
  Badge,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  useDisclosure,
} from '@nextui-org/react';
import { ShoppingCart, Plus, Eye, Check, Info, Minus, X } from 'lucide-react';
import { useRouter } from 'next/navigation';
import { MenuCategory, MenuItem, Business } from '../../api/business';
import { BillResponse } from '../../api/bills';
import { ImageCarousel } from './ImageCarousel';

interface GuestMenuProps {
  categories: MenuCategory[];
  business: Business;
  tableCode: string;
  currentBill: BillResponse | null;
  onAddToBill: (itemName: string, price: number, quantity?: number) => void;
  onAddToCart?: (itemName: string, price: number, specialRequests?: string) => void;
}

export const GuestMenu: React.FC<GuestMenuProps> = ({
  categories,
  business,
  tableCode,
  currentBill,
  onAddToBill,
  onAddToCart,
}) => {
  const router = useRouter();
  const [selectedCategory, setSelectedCategory] = useState<number>(0);
  const [selectedItem, setSelectedItem] = useState<MenuItem | null>(null);
  const [addedItems, setAddedItems] = useState<Set<string>>(new Set());
  const [itemQuantities, setItemQuantities] = useState<Record<string, number>>({});
  const [animatingItems, setAnimatingItems] = useState<Set<string>>(new Set());
  const [selectedOptions, setSelectedOptions] = useState<Set<string>>(new Set());
  const { isOpen: isItemModalOpen, onOpen: onItemModalOpen, onClose: onItemModalClose } = useDisclosure();

  const formatCurrency = (amount: number | undefined | null) => {
    return `$${(amount || 0).toFixed(2)}`;
  };

  const getItemImages = (item: MenuItem): string[] => {
    const images: string[] = [];
    
    // Add images from the new images array
    if (item.images && Array.isArray(item.images)) {
      images.push(...item.images.filter(img => img && img.trim() !== ''));
    }
    
    // Add the single image for backward compatibility (if not already in images array)
    if (item.image && item.image.trim() !== '' && !images.includes(item.image)) {
      images.unshift(item.image); // Add to beginning for primary image
    }
    
    return images;
  };

  const getTotalItems = () => {
    if (!currentBill) return 0;
    return currentBill.items.reduce((total, item) => total + item.quantity, 0);
  };

  const handleViewBillDetails = () => {
    router.push(`/t/${tableCode}/bill`);
  };

  const toggleOption = (optionId: string) => {
    setSelectedOptions(prev => {
      const newSet = new Set(prev);
      if (newSet.has(optionId)) {
        newSet.delete(optionId);
      } else {
        newSet.add(optionId);
      }
      return newSet;
    });
  };

  const calculateItemTotalPrice = (item: MenuItem) => {
    let totalPrice = item.price || 0;
    
    if (item.options) {
      item.options.forEach((option, index) => {
        const optionId = `${item.name}-option-${index}`;
        if (selectedOptions.has(optionId)) {
          totalPrice += option.price_change || 0;
        }
      });
    }
    
    return totalPrice;
  };

  const getSelectedOptionsForItem = (item: MenuItem) => {
    if (!item.options) return [];
    
    return item.options.filter((option, index) => {
      const optionId = `${item.name}-option-${index}`;
      return selectedOptions.has(optionId);
    });
  };

  const resetModalState = () => {
    setSelectedOptions(new Set());
  };

  const handleItemClick = (item: MenuItem) => {
    setSelectedItem(item);
    resetModalState(); // Clear any previous selections
    onItemModalOpen();
  };

  const getItemQuantity = (itemName: string) => {
    return itemQuantities[itemName] || 1;
  };

  const setItemQuantity = (itemName: string, quantity: number) => {
    setItemQuantities(prev => ({
      ...prev,
      [itemName]: Math.max(1, quantity)
    }));
  };

  const handleAddToBill = (item: MenuItem, quantity?: number) => {
    const finalQuantity = quantity || getItemQuantity(item.name);
    const totalPrice = calculateItemTotalPrice(item);
    const selectedItemOptions = getSelectedOptionsForItem(item);
    
    // Create item name with options for uniqueness
    const itemNameWithOptions = selectedItemOptions.length > 0 
      ? `${item.name} (${selectedItemOptions.map(opt => opt.name).join(', ')})`
      : item.name;
    
    onAddToBill(itemNameWithOptions, totalPrice, finalQuantity);
    
    // Stripe-like success animation
    setAnimatingItems(prev => new Set(Array.from(prev).concat([item.name])));
    setAddedItems(prev => new Set(Array.from(prev).concat([item.name])));
    
    // Reset modal state and close
    resetModalState();
    onItemModalClose();
    
    setTimeout(() => {
      setAnimatingItems(prev => {
        const newSet = new Set(prev);
        newSet.delete(item.name);
        return newSet;
      });
    }, 2000);
  };

  const isItemAdded = (itemName: string) => {
    return addedItems.has(itemName);
  };

  const isItemAnimating = (itemName: string) => {
    return animatingItems.has(itemName);
  };

  if (!categories || categories.length === 0) {
    return (
      <Card>
        <CardBody className="text-center py-8">
          <ShoppingCart className="w-12 h-12 mx-auto text-default-300 mb-4" />
          <h3 className="text-lg font-medium text-default-500 mb-2">Menu Coming Soon</h3>
          <p className="text-default-400">
            The menu for this restaurant is being prepared.
          </p>
        </CardBody>
      </Card>
    );
  }

  return (
    <div className="space-y-8">
      {/* Category Navigation */}
      <div className="bg-white border border-gray-200 rounded-2xl p-6 shadow-sm">
        <div className="flex gap-3 overflow-x-auto pb-2">
          {Array.isArray(categories) && categories.map((category, index) => (
            <button
              key={index}
              onClick={() => setSelectedCategory(index)}
              className={`flex-shrink-0 px-6 py-3 rounded-xl text-sm font-medium tracking-wide transition-all duration-200 ${
                selectedCategory === index
                  ? 'bg-gray-900 text-white shadow-md'
                  : 'bg-gray-50 text-gray-600 hover:bg-gray-100 hover:text-gray-900'
              }`}
            >
              {category.name}
            </button>
          ))}
        </div>
      </div>

      {/* Current Bill Summary or Help Card */}
      {currentBill ? (
        <div className="bg-green-50 border border-green-200 rounded-2xl p-6 shadow-sm">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-4">
              <div className="relative">
                <div className="w-12 h-12 bg-green-100 rounded-xl flex items-center justify-center">
                  <ShoppingCart className="w-6 h-6 text-green-600" />
                </div>
                <div className="absolute -top-2 -right-2 w-6 h-6 bg-green-500 rounded-full flex items-center justify-center">
                  <span className="text-xs text-white font-bold">{getTotalItems()}</span>
                </div>
              </div>
              <div>
                <p className="font-medium text-green-900 tracking-wide">Current Bill</p>
                <p className="text-sm text-green-700 font-light">
                  {getTotalItems()} items • {formatCurrency(currentBill.bill.total_amount)}
                </p>
              </div>
            </div>
            <button 
              onClick={handleViewBillDetails}
              className="group border border-green-300 text-green-700 px-4 py-2 text-sm font-medium hover:border-green-400 hover:text-green-800 transition-all duration-200 tracking-wide rounded-lg"
            >
              <div className="flex items-center gap-2">
                <Eye className="w-4 h-4 group-hover:scale-110 transition-transform duration-200" />
                <span>View Details</span>
              </div>
            </button>
          </div>
        </div>
      ) : (
        <div className="bg-blue-50 border border-blue-200 rounded-2xl p-6 shadow-sm">
          <div className="text-center">
            <h3 className="text-lg font-light text-gray-900 tracking-wide mb-3">Ready to Order?</h3>
            <p className="text-gray-600 font-light leading-relaxed mb-4 max-w-md mx-auto">
              Ask your server to create a bill for Table {tableCode} to start adding items to your order.
            </p>
            <div className="text-xs text-gray-400 font-light tracking-wide">
              Powered by Payverge • Secure USDC Payments
            </div>
          </div>
        </div>
      )}

      {/* Menu Items */}
      <div className="space-y-6">
        {categories[selectedCategory] && (
          <>
            <div className="text-center">
              <h2 className="text-3xl font-light text-gray-900 tracking-wide mb-3">
                {categories[selectedCategory].name}
              </h2>
              {categories[selectedCategory].description && (
                <p className="text-gray-600 font-light leading-relaxed max-w-2xl mx-auto">
                  {categories[selectedCategory].description}
                </p>
              )}
            </div>

            <div className="grid grid-cols-1 gap-6">
              {Array.isArray(categories[selectedCategory]?.items) && categories[selectedCategory].items.map((item, itemIndex) => (
                <div
                  key={itemIndex}
                  className={`group bg-white border border-gray-200 rounded-2xl p-6 hover:shadow-xl hover:border-gray-300 transition-all duration-300 hover:-translate-y-1 cursor-pointer relative ${!item.is_available ? 'opacity-60' : ''}`}
                  onClick={() => handleItemClick(item)}
                >
                  {!item.is_available && (
                    <div className="absolute top-4 right-4 bg-red-500 text-white text-xs px-2 py-1 rounded-full font-medium">
                      Unavailable
                    </div>
                  )}
                  <div className="flex gap-6">
                    <div className="relative">
                      <ImageCarousel
                        images={getItemImages(item)}
                        itemName={item.name}
                        size="md"
                      />
                      {getItemImages(item).length > 1 && (
                        <div className="absolute bottom-2 right-2 bg-black/60 backdrop-blur-sm text-white text-xs px-1.5 py-0.5 rounded-full">
                          {getItemImages(item).length}
                        </div>
                      )}
                    </div>
                    <div className="flex-1">
                      <div className="flex justify-between items-start mb-4">
                        <div className="flex-1">
                          <div className="flex items-center gap-3 mb-2">
                            <h3 className="text-xl font-light text-gray-900 tracking-wide">
                              {item.name}
                            </h3>
                            <Button
                              isIconOnly
                              size="sm"
                              variant="light"
                              className="text-gray-400 hover:text-gray-600"
                              onClick={(e) => {
                                e.stopPropagation();
                                handleItemClick(item);
                              }}
                            >
                              <Info className="w-4 h-4" />
                            </Button>
                          </div>
                          {item.description && (
                            <p className="text-gray-600 font-light leading-relaxed mb-3">
                              {item.description}
                            </p>
                          )}
                          
                          {/* Subtle Allergens and Dietary Tags */}
                          <div className="flex flex-wrap gap-2 mt-3">
                            {item.allergens && item.allergens.length > 0 && (
                              <div className="flex items-center gap-1">
                                <div className="w-2 h-2 bg-orange-400 rounded-full"></div>
                                <span className="text-xs text-orange-600 font-medium">
                                  Contains allergens
                                </span>
                              </div>
                            )}
                            {item.dietary_tags && item.dietary_tags.length > 0 && (
                              <div className="flex flex-wrap gap-1">
                                {item.dietary_tags.slice(0, 2).map((tag, index) => (
                                  <Chip
                                    key={index}
                                    size="sm"
                                    variant="flat"
                                    color="success"
                                    className="text-xs h-5 px-2"
                                  >
                                    {tag}
                                  </Chip>
                                ))}
                                {item.dietary_tags.length > 2 && (
                                  <Chip
                                    size="sm"
                                    variant="flat"
                                    color="default"
                                    className="text-xs h-5 px-2"
                                  >
                                    +{item.dietary_tags.length - 2} more
                                  </Chip>
                                )}
                              </div>
                            )}
                          </div>
                        </div>
                        <div className="text-right ml-6">
                          <p className="text-2xl font-light text-gray-900 tracking-wide">
                            {formatCurrency(item.price)}
                          </p>
                        </div>
                      </div>

                      {/* Quantity selector and Add to order button */}
                      <div className="flex items-center justify-between">
                        {/* Quantity Selector */}
                        {currentBill && (
                          <div className="flex items-center gap-3">
                            <span className="text-sm text-gray-600 font-medium">Quantity:</span>
                            <div className="flex items-center gap-2 bg-gray-50 rounded-lg p-1">
                              <button
                                onClick={(e) => {
                                  e.stopPropagation();
                                  setItemQuantity(item.name, getItemQuantity(item.name) - 1);
                                }}
                                disabled={getItemQuantity(item.name) <= 1}
                                className="w-8 h-8 flex items-center justify-center rounded-md bg-white border border-gray-200 text-gray-600 hover:text-gray-900 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                              >
                                <Minus className="w-3 h-3" />
                              </button>
                              <span className="w-8 text-center text-sm font-medium text-gray-900">
                                {getItemQuantity(item.name)}
                              </span>
                              <button
                                onClick={(e) => {
                                  e.stopPropagation();
                                  setItemQuantity(item.name, getItemQuantity(item.name) + 1);
                                }}
                                className="w-8 h-8 flex items-center justify-center rounded-md bg-white border border-gray-200 text-gray-600 hover:text-gray-900 transition-colors"
                              >
                                <Plus className="w-3 h-3" />
                              </button>
                            </div>
                          </div>
                        )}

                        {/* Add to Bill Button with Stripe-like Animation */}
                        <div className="relative">
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              handleAddToBill(item);
                            }}
                            disabled={!currentBill}
                            className={`group flex items-center gap-3 px-6 py-3 text-sm font-medium transition-all duration-300 tracking-wide rounded-lg relative overflow-hidden ${
                              isItemAdded(item.name)
                                ? 'bg-green-500 text-white animate-success-pulse'
                                : currentBill
                                ? 'bg-gray-900 text-white hover:bg-gray-800 shadow-md hover:shadow-lg'
                                : 'bg-gray-100 text-gray-400 cursor-not-allowed'
                            }`}
                          >
                            {/* Stripe-like success animation overlay */}
                            {isItemAnimating(item.name) && (
                              <div className="absolute inset-0 bg-gradient-to-r from-green-400 via-green-500 to-green-600 animate-pulse">
                                <div className="absolute inset-0 bg-gradient-to-r from-transparent via-white/20 to-transparent animate-shimmer"></div>
                              </div>
                            )}
                            
                            <div className="relative z-10 flex items-center gap-3">
                              {isItemAdded(item.name) ? (
                                <>
                                  <Check className="w-4 h-4" />
                                  <span>Added {getItemQuantity(item.name)}!</span>
                                </>
                              ) : (
                                <>
                                  <Plus className="w-4 h-4 group-hover:scale-110 transition-transform duration-200" />
                                  <span>
                                    {currentBill 
                                      ? `Add ${getItemQuantity(item.name)} to Bill` 
                                      : 'No Active Bill'
                                    }
                                  </span>
                                </>
                              )}
                            </div>
                          </button>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </>
        )}
      </div>

      {/* Item Details Modal */}
      <Modal 
        isOpen={isItemModalOpen} 
        onClose={onItemModalClose}
        size="2xl"
        scrollBehavior="inside"
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1">
                <h2 className="text-2xl font-light text-gray-900 tracking-wide">
                  {selectedItem?.name}
                </h2>
                <p className="text-lg font-light text-gray-900 tracking-wide">
                  {selectedItem && formatCurrency(selectedItem.price)}
                </p>
              </ModalHeader>
              <ModalBody>
                <div className="space-y-6">
                  {selectedItem && getItemImages(selectedItem).length > 0 && (
                    <div className="flex justify-center">
                      <ImageCarousel
                        images={getItemImages(selectedItem)}
                        itemName={selectedItem.name}
                        size="lg"
                        className="w-full max-w-lg"
                      />
                      {/* Debug info - remove this after testing */}
                      {process.env.NODE_ENV === 'development' && (
                        <div className="absolute top-4 left-4 bg-blue-500 text-white text-xs px-2 py-1 rounded">
                          Debug: {getItemImages(selectedItem).length} images
                        </div>
                      )}
                    </div>
                  )}
                  
                  {selectedItem?.description && (
                    <div>
                      <h3 className="text-lg font-medium text-gray-900 mb-3">Description</h3>
                      <p className="text-gray-600 font-light leading-relaxed">
                        {selectedItem.description}
                      </p>
                    </div>
                  )}

                  {/* Item Details */}
                  {selectedItem && (
                    <div className="space-y-6">
                      {/* Options & Add-ons */}
                      {selectedItem.options && selectedItem.options.length > 0 && (
                        <div>
                          <h3 className="text-lg font-medium text-gray-900 mb-3 flex items-center gap-2">
                            <Plus className="w-5 h-5" />
                            Options & Add-ons
                          </h3>
                          <div className="space-y-2">
                            {selectedItem.options.map((option, index) => {
                              const optionId = `${selectedItem.name}-option-${index}`;
                              const isSelected = selectedOptions.has(optionId);
                              
                              return (
                                <div 
                                  key={index} 
                                  className={`flex justify-between items-center p-3 rounded-lg border-2 cursor-pointer transition-all duration-200 ${
                                    isSelected 
                                      ? 'bg-primary-50 border-primary-200 shadow-sm' 
                                      : 'bg-gray-50 border-gray-200 hover:bg-gray-100'
                                  }`}
                                  onClick={() => toggleOption(optionId)}
                                >
                                  <div className="flex items-center gap-3">
                                    <div className={`w-5 h-5 rounded border-2 flex items-center justify-center transition-colors ${
                                      isSelected 
                                        ? 'bg-primary border-primary text-white' 
                                        : 'border-gray-300 bg-white'
                                    }`}>
                                      {isSelected && <Check className="w-3 h-3" />}
                                    </div>
                                    <span className="text-gray-700 font-medium">{option.name}</span>
                                  </div>
                                  <span className={`font-semibold ${isSelected ? 'text-primary' : 'text-gray-600'}`}>
                                    +${option.price_change ? option.price_change.toFixed(2) : '0.00'}
                                  </span>
                                </div>
                              );
                            })}
                          </div>
                        </div>
                      )}

                      {/* Allergens */}
                      {selectedItem.allergens && selectedItem.allergens.length > 0 && (
                        <div>
                          <h3 className="text-lg font-medium text-gray-900 mb-3 flex items-center gap-2">
                            <div className="w-2 h-2 bg-orange-500 rounded-full"></div>
                            Allergen Information
                          </h3>
                          <div className="bg-orange-50 border border-orange-200 rounded-xl p-4">
                            <p className="text-sm text-orange-800 mb-3 font-medium">
                              ⚠️ This item contains or may contain:
                            </p>
                            <div className="flex flex-wrap gap-2">
                              {selectedItem.allergens.map((allergen, index) => (
                                <Chip
                                  key={index}
                                  size="sm"
                                  color="warning"
                                  variant="solid"
                                  className="text-xs font-medium"
                                >
                                  {allergen}
                                </Chip>
                              ))}
                            </div>
                          </div>
                        </div>
                      )}

                      {/* Dietary Tags */}
                      {selectedItem.dietary_tags && selectedItem.dietary_tags.length > 0 && (
                        <div>
                          <h3 className="text-lg font-medium text-gray-900 mb-3 flex items-center gap-2">
                            <div className="w-2 h-2 bg-green-500 rounded-full"></div>
                            Dietary Information
                          </h3>
                          <div className="bg-green-50 border border-green-200 rounded-xl p-4">
                            <div className="flex flex-wrap gap-2">
                              {selectedItem.dietary_tags.map((tag, index) => (
                                <Chip
                                  key={index}
                                  size="sm"
                                  color="success"
                                  variant="solid"
                                  className="text-xs font-medium"
                                >
                                  {tag}
                                </Chip>
                              ))}
                            </div>
                          </div>
                        </div>
                      )}

                      {/* Currency Info */}
                      {selectedItem.currency && selectedItem.currency !== 'USD' && (
                        <div className="bg-blue-50 border border-blue-200 rounded-lg p-3">
                          <p className="text-sm text-blue-800">
                            <Info className="w-4 h-4 inline mr-1" />
                            Price shown in {selectedItem.currency}
                          </p>
                        </div>
                      )}

                      {/* Availability Status */}
                      <div className="flex items-center justify-center">
                        <Chip
                          size="lg"
                          color={selectedItem.is_available ? "success" : "default"}
                          variant="flat"
                          startContent={selectedItem.is_available ? <Check className="w-4 h-4" /> : <X className="w-4 h-4" />}
                        >
                          {selectedItem.is_available ? 'Available Now' : 'Currently Unavailable'}
                        </Chip>
                      </div>
                    </div>
                  )}
                </div>
              </ModalBody>
              <ModalFooter>
                <div className="flex flex-col gap-4 w-full">
                  {/* Quantity Selector in Modal */}
                  {currentBill && selectedItem && (
                    <div className="flex items-center justify-center gap-4">
                      <span className="text-sm text-gray-600 font-medium">Quantity:</span>
                      <div className="flex items-center gap-3 bg-gray-50 rounded-lg p-2">
                        <button
                          onClick={() => setItemQuantity(selectedItem.name, getItemQuantity(selectedItem.name) - 1)}
                          disabled={getItemQuantity(selectedItem.name) <= 1}
                          className="w-10 h-10 flex items-center justify-center rounded-md bg-white border border-gray-200 text-gray-600 hover:text-gray-900 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
                        >
                          <Minus className="w-4 h-4" />
                        </button>
                        <span className="w-12 text-center text-lg font-medium text-gray-900">
                          {getItemQuantity(selectedItem.name)}
                        </span>
                        <button
                          onClick={() => setItemQuantity(selectedItem.name, getItemQuantity(selectedItem.name) + 1)}
                          className="w-10 h-10 flex items-center justify-center rounded-md bg-white border border-gray-200 text-gray-600 hover:text-gray-900 transition-colors"
                        >
                          <Plus className="w-4 h-4" />
                        </button>
                      </div>
                      <span className="text-sm text-gray-500">
                        Total: {selectedItem && formatCurrency(calculateItemTotalPrice(selectedItem) * getItemQuantity(selectedItem.name))}
                      </span>
                    </div>
                  )}

                  {/* Action Buttons */}
                  <div className="flex gap-3 w-full">
                    <Button 
                      variant="light" 
                      onPress={onClose}
                      className="flex-1"
                    >
                      Close
                    </Button>
                    {currentBill && selectedItem && (
                      <div className="flex-1 relative">
                        <Button
                          color="primary"
                          onPress={() => {
                            handleAddToBill(selectedItem);
                            onClose();
                          }}
                          className={`w-full relative overflow-hidden ${
                            isItemAdded(selectedItem.name)
                              ? 'bg-green-500 text-white'
                              : 'bg-gray-900 text-white'
                          }`}
                          startContent={
                            isItemAdded(selectedItem.name) ? (
                              <Check className="w-4 h-4" />
                            ) : (
                              <Plus className="w-4 h-4" />
                            )
                          }
                        >
                          {/* Stripe-like animation overlay for modal button */}
                          {isItemAnimating(selectedItem.name) && (
                            <div className="absolute inset-0 bg-gradient-to-r from-green-400 via-green-500 to-green-600 animate-pulse">
                              <div className="absolute inset-0 bg-gradient-to-r from-transparent via-white/20 to-transparent animate-shimmer"></div>
                            </div>
                          )}
                          <span className="relative z-10">
                            {isItemAdded(selectedItem.name) 
                              ? `Added ${getItemQuantity(selectedItem.name)} to Bill!` 
                              : `Add ${getItemQuantity(selectedItem.name)} to Bill - ${formatCurrency(calculateItemTotalPrice(selectedItem) * getItemQuantity(selectedItem.name))}`
                            }
                          </span>
                        </Button>
                      </div>
                    )}
                  </div>
                </div>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

    </div>
  );
};

export default GuestMenu;
