"use client";

import React, { useCallback } from "react";
import {
  Button,
  Input,
  Textarea,
  Select,
  SelectItem,
  Switch,
} from "@nextui-org/react";
import {
  Award,
  Plus,
  X,
  Wifi,
  Car,
  CreditCard,
  Coffee,
  Music,
  Utensils,
  Shield,
  Heart,
  Star,
  Gift,
  Zap,
  Users,
  Clock,
  MapPin,
  Phone,
} from "lucide-react";
import { BusinessSpecialFeature } from "@/api/business";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";

interface SpecialFeaturesEditorProps {
  businessId: number;
  features: BusinessSpecialFeature[];
  onFeaturesChange: (features: BusinessSpecialFeature[]) => void;
}

interface DraftFeature {
  id?: number;
  business_id?: number;
  title: string;
  description: string;
  icon: string;
  display_order: number;
  is_active: boolean;
  created_at?: string;
  updated_at?: string;
}

export default function SpecialFeaturesEditor({
  businessId,
  features,
  onFeaturesChange,
}: SpecialFeaturesEditorProps) {
  const { locale } = useSimpleLocale();

  const t = useCallback(
    (key: string): string => {
      const fullKey = `businessSettings.specialFeaturesEditor.${key}`;
      const result = getTranslation(fullKey, locale);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [locale],
  );

  const ICON_OPTIONS = [
    { key: "wifi", label: t("iconOptions.wifi"), icon: Wifi },
    { key: "car", label: t("iconOptions.car"), icon: Car },
    {
      key: "credit-card",
      label: t("iconOptions.credit-card"),
      icon: CreditCard,
    },
    { key: "coffee", label: t("iconOptions.coffee"), icon: Coffee },
    { key: "music", label: t("iconOptions.music"), icon: Music },
    { key: "utensils", label: t("iconOptions.utensils"), icon: Utensils },
    { key: "shield", label: t("iconOptions.shield"), icon: Shield },
    { key: "heart", label: t("iconOptions.heart"), icon: Heart },
    { key: "star", label: t("iconOptions.star"), icon: Star },
    { key: "gift", label: t("iconOptions.gift"), icon: Gift },
    { key: "zap", label: t("iconOptions.zap"), icon: Zap },
    { key: "users", label: t("iconOptions.users"), icon: Users },
    { key: "clock", label: t("iconOptions.clock"), icon: Clock },
    { key: "map-pin", label: t("iconOptions.map-pin"), icon: MapPin },
    { key: "phone", label: t("iconOptions.phone"), icon: Phone },
    { key: "award", label: t("iconOptions.award"), icon: Award },
  ];

  const getIconComponent = (iconKey: string) => {
    const iconOption = ICON_OPTIONS.find((option) => option.key === iconKey);
    return iconOption ? iconOption.icon : Award;
  };

  const addFeature = useCallback(() => {
    const next: DraftFeature = {
      title: "",
      description: "",
      icon: "award",
      display_order: features.length,
      is_active: true,
      business_id: businessId,
    };
    onFeaturesChange([...features, next as BusinessSpecialFeature]);
  }, [features, onFeaturesChange, businessId]);

  const updateFeature = useCallback(
    (index: number, patch: Partial<DraftFeature>) => {
      const updated = features.map((f, i) =>
        i === index ? ({ ...f, ...patch } as BusinessSpecialFeature) : f,
      );
      onFeaturesChange(updated);
    },
    [features, onFeaturesChange],
  );

  const removeFeature = useCallback(
    (index: number) => {
      const updated = features
        .filter((_, i) => i !== index)
        .map((f, i) => ({ ...f, display_order: i }) as BusinessSpecialFeature);
      onFeaturesChange(updated);
    },
    [features, onFeaturesChange],
  );

  const applyTemplate = useCallback(
    (kind: "restaurant" | "cafe") => {
      const restaurantTemplate: DraftFeature[] = [
        {
          title: t("templates.restaurant.freeWifi.title"),
          description: t("templates.restaurant.freeWifi.description"),
          icon: "wifi",
          display_order: 0,
          is_active: true,
          business_id: businessId,
        },
        {
          title: t("templates.restaurant.cardPayments.title"),
          description: t("templates.restaurant.cardPayments.description"),
          icon: "credit-card",
          display_order: 1,
          is_active: true,
          business_id: businessId,
        },
        {
          title: t("templates.restaurant.familyFriendly.title"),
          description: t("templates.restaurant.familyFriendly.description"),
          icon: "heart",
          display_order: 2,
          is_active: true,
          business_id: businessId,
        },
        {
          title: t("templates.restaurant.parkingAvailable.title"),
          description: t("templates.restaurant.parkingAvailable.description"),
          icon: "car",
          display_order: 3,
          is_active: true,
          business_id: businessId,
        },
      ];

      const cafeTemplate: DraftFeature[] = [
        {
          title: t("templates.cafe.freeWifi.title"),
          description: t("templates.cafe.freeWifi.description"),
          icon: "wifi",
          display_order: 0,
          is_active: true,
          business_id: businessId,
        },
        {
          title: t("templates.cafe.premiumCoffee.title"),
          description: t("templates.cafe.premiumCoffee.description"),
          icon: "coffee",
          display_order: 1,
          is_active: true,
          business_id: businessId,
        },
        {
          title: t("templates.cafe.quietEnvironment.title"),
          description: t("templates.cafe.quietEnvironment.description"),
          icon: "shield",
          display_order: 2,
          is_active: true,
          business_id: businessId,
        },
      ];

      onFeaturesChange(
        (kind === "restaurant"
          ? restaurantTemplate
          : cafeTemplate) as BusinessSpecialFeature[],
      );
    },
    [businessId, onFeaturesChange, t],
  );

  if (features.length === 0) {
    return (
      <div className="space-y-4 rounded-xl border border-dashed border-brand/30 bg-brand/5 p-8 text-center">
        <Award className="mx-auto h-10 w-10 text-brand" />
        <p className="text-sm text-ink-600">{t("noFeaturesYet")}</p>
        <div className="flex flex-wrap items-center justify-center gap-2">
          <Button
            variant="flat"
            size="sm"
            className="bg-brand text-white shadow-sm shadow-brand/20"
            startContent={<Plus className="w-4 h-4" />}
            onPress={addFeature}
          >
            {t("addFirstFeature")}
          </Button>
          <span className="text-xs text-ink-500">{t("quickTemplates")}</span>
          <Button
            size="sm"
            variant="flat"
            className="bg-white text-ink-700 shadow-sm shadow-warm-900/5"
            onPress={() => applyTemplate("restaurant")}
          >
            {t("restaurantTemplate")}
          </Button>
          <Button
            size="sm"
            variant="flat"
            className="bg-white text-ink-700 shadow-sm shadow-warm-900/5"
            onPress={() => applyTemplate("cafe")}
          >
            {t("cafeTemplate")}
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <div className="overflow-hidden rounded-xl border border-warm-200 bg-white shadow-sm shadow-warm-900/5">
        {features.map((feature, index) => {
          const IconComponent = getIconComponent(feature.icon || "award");
          return (
            <div
              key={feature.id ?? `draft-${index}`}
              className={`flex items-start gap-3 px-4 py-3 transition-colors hover:bg-warm-50/50 ${
                index !== features.length - 1 ? "border-b border-warm-100" : ""
              }`}
            >
              <div className="w-20 flex-shrink-0">
                <Select
                  aria-label={t("iconAria")}
                  selectedKeys={[feature.icon || "award"]}
                  onSelectionChange={(keys) => {
                    const selectedIcon = Array.from(keys)[0] as string;
                    if (selectedIcon && selectedIcon.trim() !== "") {
                      updateFeature(index, { icon: selectedIcon });
                    }
                  }}
                  size="sm"
                  variant="bordered"
                  disallowEmptySelection
                  renderValue={() => (
                    <div className="flex items-center gap-1" data-testid={`feature-icon-${feature.icon || "award"}`}>
                      <IconComponent className="h-4 w-4 text-ink-700" data-icon={feature.icon || "award"} />
                    </div>
                  )}
                  classNames={{ trigger: "h-9 min-h-9" }}
                >
                  {ICON_OPTIONS.map((option) => (
                    <SelectItem
                      key={option.key}
                      value={option.key}
                      textValue={option.label}
                    >
                      <div className="flex items-center gap-2">
                        <option.icon className="w-4 h-4" />
                        <span>{option.label}</span>
                      </div>
                    </SelectItem>
                  ))}
                </Select>
              </div>

              <div className="flex-1 space-y-2">
                <Input
                  aria-label={t("titlePlaceholder")}
                  placeholder={t("titlePlaceholder")}
                  value={feature.title}
                  onValueChange={(value) =>
                    updateFeature(index, { title: value })
                  }
                  size="sm"
                  variant="bordered"
                  classNames={{ inputWrapper: "h-9 min-h-9" }}
                />
                <Textarea
                  aria-label={t("descriptionPlaceholder")}
                  placeholder={t("descriptionPlaceholder")}
                  value={feature.description}
                  onValueChange={(value) =>
                    updateFeature(index, { description: value })
                  }
                  size="sm"
                  variant="bordered"
                  minRows={2}
                  maxRows={3}
                />
              </div>

              <div className="flex flex-col items-center justify-center pt-1">
                <Switch
                  size="sm"
                  aria-label={t("activeAria")}
                  isSelected={feature.is_active !== false}
                  onValueChange={(value) =>
                    updateFeature(index, { is_active: value })
                  }
                />
              </div>

              <Button
                isIconOnly
                size="sm"
                variant="light"
                onPress={() => removeFeature(index)}
                className="text-rose-700 hover:bg-rose-50"
                aria-label={t("removeFeature") || "Remove"}
              >
                <X className="w-4 h-4" />
              </Button>
            </div>
          );
        })}
      </div>

      <Button
        size="sm"
        variant="flat"
        className="bg-brand/10 text-brand-dark"
        startContent={<Plus className="w-4 h-4" />}
        onPress={addFeature}
      >
        {t("addFeature")}
      </Button>
    </div>
  );
}
