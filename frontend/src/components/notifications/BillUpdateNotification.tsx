"use client";

import React, { useState, useEffect } from "react";
import { Card, CardBody, Button } from "@nextui-org/react";
import { Bell, X, Plus, Minus } from "lucide-react";
import { useGuestTranslation } from "@/i18n/GuestTranslationProvider";

// Static class literals per color scheme. Tailwind's JIT only emits classes it
// can see as complete strings at build time, so interpolated names like
// `bg-${scheme}-50` are purged in production (e.g. text-default-800 has zero
// other occurrences and was being dropped). Keep every class a literal here.
const SCHEME_STYLES = {
  primary: {
    card: "border-primary-200 bg-primary-50",
    iconBubble: "bg-primary-100",
    title: "text-primary-800",
    subtitle: "text-primary-600",
  },
  warning: {
    card: "border-warning-200 bg-warning-50",
    iconBubble: "bg-warning-100",
    title: "text-warning-800",
    subtitle: "text-warning-600",
  },
  default: {
    card: "border-default-200 bg-default-50",
    iconBubble: "bg-default-100",
    title: "text-default-800",
    subtitle: "text-default-600",
  },
} as const;

type SchemeKey = keyof typeof SCHEME_STYLES;

interface BillUpdateNotificationProps {
  update: {
    type: "item_added" | "item_removed" | "bill_updated";
    itemName?: string;
    billNumber: string;
    timestamp: string;
  } | null;
  onDismiss: () => void;
  autoHide?: boolean;
  hideDelay?: number;
}

const BillUpdateNotification: React.FC<BillUpdateNotificationProps> = ({
  update,
  onDismiss,
  autoHide = true,
  hideDelay = 3000,
}) => {
  const [isVisible, setIsVisible] = useState(false);
  const { t } = useGuestTranslation();

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
      case "item_added":
        return <Plus className="w-4 h-4 text-primary-600" />;
      case "item_removed":
        return <Minus className="w-4 h-4 text-warning-600" />;
      default:
        return <Bell className="w-4 h-4 text-default-600" />;
    }
  };

  const getMessage = () => {
    switch (update.type) {
      case "item_added":
        return t("notifications.billUpdate.itemAdded", {
          item: update.itemName ?? "",
        });
      case "item_removed":
        return t("notifications.billUpdate.itemRemoved", {
          item: update.itemName ?? "",
        });
      default:
        return t("notifications.billUpdate.updated");
    }
  };

  const getColorScheme = (): SchemeKey => {
    switch (update.type) {
      case "item_added":
        return "primary";
      case "item_removed":
        return "warning";
      default:
        return "default";
    }
  };

  const styles = SCHEME_STYLES[getColorScheme()];

  return (
    <div className="fixed top-4 left-1/2 transform -translate-x-1/2 z-40 animate-in slide-in-from-top duration-300">
      <Card className={`max-w-sm shadow-lg border ${styles.card}`}>
        <CardBody className="p-3">
          <div className="flex items-center justify-between gap-3">
            <div className="flex items-center gap-2">
              <div className={`p-1.5 ${styles.iconBubble} rounded-full`}>
                {getIcon()}
              </div>
              <div>
                <p className={`text-sm font-medium ${styles.title}`}>
                  {getMessage()}
                </p>
                <p className={`text-xs ${styles.subtitle}`}>
                  {t("notifications.billUpdate.billRef", { number: update.billNumber })}
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
              <X className="w-3 h-3" />
            </Button>
          </div>
        </CardBody>
      </Card>
    </div>
  );
};

export default BillUpdateNotification;
