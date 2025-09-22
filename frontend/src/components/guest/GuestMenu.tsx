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
} from '@nextui-org/react';
import { ShoppingCart, Plus, Eye } from 'lucide-react';
import { MenuCategory, MenuItem, Business } from '../../api/business';
import { BillResponse } from '../../api/bills';

interface GuestMenuProps {
  categories: MenuCategory[];
  business: Business;
  tableCode: string;
  currentBill: BillResponse | null;
  onAddToBill: (itemName: string, price: number, quantity?: number) => void;
}

export const GuestMenu: React.FC<GuestMenuProps> = ({
  categories,
  business,
  tableCode,
  currentBill,
  onAddToBill,
}) => {
  const [selectedCategory, setSelectedCategory] = useState<number>(0);

  const formatCurrency = (amount: number) => {
    return `$${amount.toFixed(2)}`;
  };

  const getTotalItems = () => {
    if (!currentBill) return 0;
    return currentBill.items.reduce((total, item) => total + item.quantity, 0);
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
            <button className="group border border-green-300 text-green-700 px-4 py-2 text-sm font-medium hover:border-green-400 hover:text-green-800 transition-all duration-200 tracking-wide rounded-lg">
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
                  className="group bg-white border border-gray-200 rounded-2xl p-6 hover:shadow-xl hover:border-gray-300 transition-all duration-300 hover:-translate-y-1"
                >
                  <div className="flex gap-6">
                    {item.image && (
                      <div className="flex-shrink-0">
                        <div className="w-28 h-28 rounded-xl overflow-hidden border border-gray-100 shadow-sm">
                          <Image
                            src={item.image}
                            alt={item.name}
                            className="w-full h-full object-cover"
                          />
                        </div>
                      </div>
                    )}
                    <div className="flex-1">
                      <div className="flex justify-between items-start mb-4">
                        <div>
                          <h3 className="text-xl font-light text-gray-900 tracking-wide mb-2">
                            {item.name}
                          </h3>
                          {item.description && (
                            <p className="text-gray-600 font-light leading-relaxed">
                              {item.description}
                            </p>
                          )}
                        </div>
                        <div className="text-right ml-6">
                          <p className="text-2xl font-light text-gray-900 tracking-wide">
                            {formatCurrency(item.price)}
                          </p>
                        </div>
                      </div>

                      {/* Add to order button */}
                      <div className="flex justify-end">
                        <button
                          onClick={() => onAddToBill(item.name, item.price, 1)}
                          disabled={!currentBill}
                          className={`group flex items-center gap-3 px-6 py-3 text-sm font-medium transition-all duration-200 tracking-wide rounded-lg ${
                            currentBill
                              ? 'bg-gray-900 text-white hover:bg-gray-800 shadow-md hover:shadow-lg'
                              : 'bg-gray-100 text-gray-400 cursor-not-allowed'
                          }`}
                        >
                          <Plus className="w-4 h-4 group-hover:scale-110 transition-transform duration-200" />
                          <span>{currentBill ? 'Add to Bill' : 'No Active Bill'}</span>
                        </button>
                      </div>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          </>
        )}
      </div>

    </div>
  );
};

export default GuestMenu;
