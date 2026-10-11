"use client";

import React, { useState, useEffect } from "react";
import { Card, CardBody, Button } from "@nextui-org/react";
import { CheckCircle, X, DollarSign } from "lucide-react";
import { useGuestTranslation } from "@/i18n/GuestTranslationProvider";
import { formatCurrency } from "@/api/currency";

interface PaymentNotificationProps {
  payment: {
    amount: number;
    currency: string;
    billNumber: string;
    timestamp: string;
  } | null;
  onDismiss: () => void;
  autoHide?: boolean;
  hideDelay?: number;
}

const PaymentNotification: React.FC<PaymentNotificationProps> = ({
  payment,
  onDismiss,
  autoHide = true,
  hideDelay = 5000,
}) => {
  const [isVisible, setIsVisible] = useState(false);
  const { t, currentLanguage } = useGuestTranslation();

  useEffect(() => {
    if (payment) {
      setIsVisible(true);

      if (autoHide) {
        const timer = setTimeout(() => {
          setIsVisible(false);
          setTimeout(onDismiss, 300); // Wait for animation
        }, hideDelay);

        return () => clearTimeout(timer);
      }
    }
  }, [payment, autoHide, hideDelay, onDismiss]);

  if (!payment || !isVisible) return null;

  return (
    <div className="fixed top-4 right-4 z-50 animate-in slide-in-from-right duration-300">
      <Card className="max-w-sm shadow-lg border border-success-200 bg-success-50">
        <CardBody className="p-4">
          <div className="flex items-start justify-between gap-3">
            <div className="flex items-start gap-3">
              <div className="p-2 bg-success-100 rounded-full">
                <CheckCircle className="w-5 h-5 text-success-600" />
              </div>
              <div className="flex-1">
                <h4 className="font-semibold text-success-800 mb-1">
                  {t("notifications.payment.title")}
                </h4>
                <p className="text-sm text-success-700 mb-2">
                  {t("notifications.payment.bill", { number: payment.billNumber })}
                </p>
                <div className="flex items-center gap-2">
                  <DollarSign className="w-4 h-4 text-success-600" />
                  <span className="font-bold text-success-800">
                    {formatCurrency(payment.amount, payment.currency)}
                  </span>
                </div>
                <p className="text-xs text-success-600 mt-1">
                  {new Date(payment.timestamp).toLocaleTimeString(currentLanguage)}
                </p>
              </div>
            </div>
            <Button
              isIconOnly
              size="sm"
              variant="light"
              aria-label={t("common.close")}
              onPress={() => {
                setIsVisible(false);
                setTimeout(onDismiss, 300);
              }}
            >
              <X className="w-4 h-4" />
            </Button>
          </div>
        </CardBody>
      </Card>
    </div>
  );
};

export default PaymentNotification;
