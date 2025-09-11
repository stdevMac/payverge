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
    <div className="space-y-6">
      {/* Category Navigation */}
      <Card>
        <CardBody>
          <div className="flex gap-2 overflow-x-auto pb-2">
            {categories.map((category, index) => (
              <Button
                key={index}
                variant={selectedCategory === index ? 'solid' : 'light'}
                color={selectedCategory === index ? 'primary' : 'default'}
                size="sm"
                onPress={() => setSelectedCategory(index)}
                className="flex-shrink-0"
              >
                {category.name}
              </Button>
            ))}
          </div>
        </CardBody>
      </Card>

      {/* Current Bill Summary */}
      {currentBill && (
        <Card className="bg-primary-50 border-primary-200">
          <CardBody>
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-3">
                <Badge content={getTotalItems()} color="primary">
                  <ShoppingCart className="w-5 h-5 text-primary" />
                </Badge>
                <div>
                  <p className="font-medium text-primary">Current Bill</p>
                  <p className="text-sm text-primary-600">
                    {getTotalItems()} items • {formatCurrency(currentBill.bill.total_amount)}
                  </p>
                </div>
              </div>
              <Button
                color="primary"
                variant="flat"
                size="sm"
                startContent={<Eye className="w-4 h-4" />}
              >
                View Details
              </Button>
            </div>
          </CardBody>
        </Card>
      )}

      {/* Menu Items */}
      <div className="space-y-4">
        {categories[selectedCategory] && (
          <>
            <div className="mb-4">
              <h2 className="text-2xl font-bold">{categories[selectedCategory].name}</h2>
              {categories[selectedCategory].description && (
                <p className="text-default-600 mt-1">{categories[selectedCategory].description}</p>
              )}
            </div>

            <div className="grid grid-cols-1 gap-4">
              {categories[selectedCategory].items.map((item, itemIndex) => (
                <Card key={itemIndex} className="hover:shadow-md transition-shadow">
                  <CardBody>
                    <div className="flex gap-4">
                      {item.image && (
                        <div className="flex-shrink-0">
                          <Image
                            src={item.image}
                            alt={item.name}
                            className="w-24 h-24 object-cover rounded-lg"
                          />
                        </div>
                      )}
                      <div className="flex-1">
                        <div className="flex justify-between items-start mb-2">
                          <div>
                            <h3 className="text-lg font-semibold">{item.name}</h3>
                            {item.description && (
                              <p className="text-default-600 text-sm mt-1">{item.description}</p>
                            )}
                          </div>
                          <div className="text-right">
                            <p className="text-xl font-bold text-primary">
                              {formatCurrency(item.price)}
                            </p>
                          </div>
                        </div>


                        {/* Add to order button */}
                        <div className="flex justify-end">
                          <Button
                            color="primary"
                            size="sm"
                            startContent={<Plus className="w-4 h-4" />}
                            isDisabled={!currentBill}
                            onPress={() => onAddToBill(item.name, item.price, 1)}
                          >
                            {currentBill ? 'Add to Bill' : 'No Active Bill'}
                          </Button>
                        </div>
                      </div>
                    </div>
                  </CardBody>
                </Card>
              ))}
            </div>
          </>
        )}
      </div>

      {/* Info Card */}
      <Card className="bg-default-100">
        <CardBody>
          <div className="text-center">
            <h3 className="font-semibold mb-2">Need Help?</h3>
            <p className="text-sm text-default-600 mb-3">
              Ask your server to create a bill for your table to start ordering.
            </p>
            <div className="flex justify-center gap-4 text-xs text-default-500">
              <span>Table: {tableCode}</span>
              <span>•</span>
              <span>Powered by Payverge</span>
            </div>
          </div>
        </CardBody>
      </Card>
    </div>
  );
};

export default GuestMenu;
