'use client';

import React, { useState, useEffect } from 'react';
import { Card, CardBody, Button, Chip } from '@nextui-org/react';
import { Bell, X, Plus, Minus } from 'lucide-react';

interface BillUpdateNotificationProps {
  update: {
    type: 'item_added' | 'item_removed' | 'bill_updated';
    itemName?: string;
    billNumber: string;
    timestamp: string;
  } | null;
  onDismiss: () => void;
  autoHide?: boolean;
  hideDelay?: number;
}

export const BillUpdateNotification: React.FC<BillUpdateNotificationProps> = ({
  update,
  onDismiss,
  autoHide = true,
  hideDelay = 3000
}) => {
  const [isVisible, setIsVisible] = useState(false);

  useEffect(() => {
    if (update) {
      setIsVisible(true);
      
      if (autoHide) {
        const timer = setTimeout(() => {
          setIsVisible(false);
          setTimeout(onDismiss, 300);
        }, hideDelay);
        
        return () => clearTimeout(timer);
      }
    }
  }, [update, autoHide, hideDelay, onDismiss]);

  if (!update || !isVisible) return null;

  const getIcon = () => {
    switch (update.type) {
      case 'item_added':
        return <Plus className="w-4 h-4 text-primary-600" />;
      case 'item_removed':
        return <Minus className="w-4 h-4 text-warning-600" />;
      default:
        return <Bell className="w-4 h-4 text-default-600" />;
    }
  };

  const getMessage = () => {
    switch (update.type) {
      case 'item_added':
        return `Added ${update.itemName} to bill`;
      case 'item_removed':
        return `Removed ${update.itemName} from bill`;
      default:
        return 'Bill updated';
    }
  };

  const getColorScheme = () => {
    switch (update.type) {
      case 'item_added':
        return 'primary';
      case 'item_removed':
        return 'warning';
      default:
        return 'default';
    }
  };

  const colorScheme = getColorScheme();

  return (
    <div className="fixed top-4 left-1/2 transform -translate-x-1/2 z-40 animate-in slide-in-from-top duration-300">
      <Card className={`max-w-sm shadow-lg border border-${colorScheme}-200 bg-${colorScheme}-50`}>
        <CardBody className="p-3">
          <div className="flex items-center justify-between gap-3">
            <div className="flex items-center gap-2">
              <div className={`p-1.5 bg-${colorScheme}-100 rounded-full`}>
                {getIcon()}
              </div>
              <div>
                <p className={`text-sm font-medium text-${colorScheme}-800`}>
                  {getMessage()}
                </p>
                <p className={`text-xs text-${colorScheme}-600`}>
                  Bill #{update.billNumber}
                </p>
              </div>
            </div>
            <Button
              isIconOnly
              size="sm"
              variant="light"
              onPress={() => {
                setIsVisible(false);
                setTimeout(onDismiss, 300);
              }}
            >
              <X className="w-3 h-3" />
            </Button>
          </div>
        </CardBody>
      </Card>
    </div>
  );
};

export default BillUpdateNotification;
