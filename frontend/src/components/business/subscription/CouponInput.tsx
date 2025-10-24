'use client';

import React from 'react';
import { Card, CardBody, Input, Checkbox, Chip, Spinner } from '@nextui-org/react';
import { Tag, CheckCircle, XCircle } from 'lucide-react';

interface CouponInputProps {
  useCoupon: boolean;
  onToggleCoupon: (use: boolean) => void;
  couponCode: string;
  onCouponCodeChange: (code: string) => void;
  isCouponValid?: boolean;
  couponDiscount?: bigint;
  isCouponLoading?: boolean;
  tString: (key: string) => string;
}

export const CouponInput: React.FC<CouponInputProps> = ({
  useCoupon,
  onToggleCoupon,
  couponCode,
  onCouponCodeChange,
  isCouponValid,
  couponDiscount,
  isCouponLoading,
  tString,
}) => {
  const formatDiscountAmount = (discount?: bigint) => {
    if (!discount) return '0.00';
    return (Number(discount) / 1000000).toFixed(2); // Convert from wei to USDC
  };

  return (
    <Card className="w-full">
      <CardBody className="p-4">
        <div className="space-y-3">
          <div className="flex items-center justify-between">
            <div className="flex items-center space-x-2">
              <Tag className="h-4 w-4 text-green-600" />
              <h5 className="font-semibold">{tString('renewalModal.couponCode')}</h5>
            </div>
            <Checkbox
              isSelected={useCoupon}
              onValueChange={onToggleCoupon}
              color="success"
            />
          </div>
          
          {useCoupon && (
            <div className="space-y-3">
              <Input
                label={tString('renewalModal.enterCouponCode')}
                placeholder={tString('renewalModal.couponPlaceholder')}
                value={couponCode}
                onValueChange={onCouponCodeChange}
                startContent={<Tag size={16} className="text-gray-400" />}
                endContent={
                  isCouponLoading ? (
                    <Spinner size="sm" />
                  ) : couponCode && couponCode.length > 0 ? (
                    isCouponValid ? (
                      <CheckCircle size={16} className="text-green-500" />
                    ) : (
                      <XCircle size={16} className="text-red-500" />
                    )
                  ) : null
                }
                color={
                  !couponCode || couponCode.length === 0 
                    ? 'default'
                    : isCouponValid 
                      ? 'success' 
                      : 'danger'
                }
                description={
                  couponCode && couponCode.length > 0 && !isCouponLoading
                    ? isCouponValid
                      ? tString('renewalModal.validCoupon')
                      : tString('renewalModal.invalidCoupon')
                    : undefined
                }
              />
              
              {isCouponValid && couponDiscount && (
                <div className="bg-green-50 border border-green-200 rounded-lg p-3">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center space-x-2">
                      <CheckCircle className="h-4 w-4 text-green-600" />
                      <span className="text-sm text-green-800 font-medium">
                        {tString('renewalModal.discountApplied')}
                      </span>
                    </div>
                    <Chip
                      color="success"
                      variant="flat"
                      size="sm"
                    >
                      -${formatDiscountAmount(couponDiscount)}
                    </Chip>
                  </div>
                </div>
              )}
            </div>
          )}
        </div>
      </CardBody>
    </Card>
  );
};
