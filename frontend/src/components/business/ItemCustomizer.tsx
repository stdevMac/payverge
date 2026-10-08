"use client";

import React, { useState } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Card,
  CardBody,
  Checkbox,
  Input,
  Divider,
  Chip,
  Image,
} from "@nextui-org/react";
import { Plus, Minus } from "lucide-react";
import toast from "react-hot-toast";
import { MenuItem, MenuItemOption } from "../../api/business";
import { formatCurrency as formatCurrencyIntl } from "../../api/currency";
import { roundDollars } from "@/types/money";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";

interface ItemCustomizerProps {
  isOpen: boolean;
  onClose: () => void;
  item: MenuItem;
  onAddToCart: (
    item: MenuItem,
    quantity: number,
    selectedOptions: MenuItemOption[],
    specialRequests: string,
  ) => void;
  allowSpecialRequests?: boolean;
  submitLabel?: string;
  // ISO currency code for the business this item belongs to. Was
  // hardcoded "$" throughout the customize flow; AED guests adding
  // an item to a bill saw their prices in dollars.
  currency?: string;
}

interface SelectedOption extends MenuItemOption {
  selected: boolean;
}

export const ItemCustomizer: React.FC<ItemCustomizerProps> = ({
  isOpen,
  onClose,
  item,
  onAddToCart,
  allowSpecialRequests = true,
  submitLabel,
  currency = "USD",
}) => {
  const { locale: currentLocale } = useSimpleLocale();

  // Translation helper
  const tString = (key: string): string => {
    const fullKey = `itemCustomizer.${key}`;
    const result = getTranslation(fullKey, currentLocale);
    return Array.isArray(result) ? result[0] || key : (result as string);
  };

  const formatCurrency = (amount: number) =>
    formatCurrencyIntl(amount, currency);

  const [quantity, setQuantity] = useState(1);
  const [selectedOptions, setSelectedOptions] = useState<SelectedOption[]>(
    (item.options || []).map((option) => ({ ...option, selected: false })),
  );
  const [specialRequests, setSpecialRequests] = useState("");

  const handleOptionToggle = (optionIndex: number) => {
    setSelectedOptions((prev) =>
      prev.map((option, index) =>
        index === optionIndex
          ? { ...option, selected: !option.selected }
          : option,
      ),
    );
  };

  const calculateTotalPrice = () => {
    const basePrice = item.price || 0;
    const optionsPrice = selectedOptions
      .filter((option) => option.selected)
      .reduce((sum, option) => sum + (option.price_change || 0), 0);
    // Cents precision — qty×price float residue must not show on totals (FIND-043).
    return roundDollars((basePrice + optionsPrice) * quantity) as number;
  };

  // Required options that the user hasn't selected. The "Required" label is
  // otherwise purely cosmetic — nothing actually enforced it before submit.
  const missingRequiredOptions = selectedOptions.filter(
    (option) => option.is_required && !option.selected,
  );

  const handleAddToCart = () => {
    if (missingRequiredOptions.length > 0) {
      toast.error(
        tString("requiredMissing").replace(
          "{options}",
          missingRequiredOptions.map((option) => option.name).join(", "),
        ),
      );
      return;
    }

    const finalSelectedOptions = selectedOptions
      .filter((option) => option.selected)
      .map(({ selected, ...option }) => option); // Remove the 'selected' property

    onAddToCart(item, quantity, finalSelectedOptions, specialRequests);

    // Reset form
    setQuantity(1);
    setSelectedOptions(
      (item.options || []).map((option) => ({ ...option, selected: false })),
    );
    setSpecialRequests("");
    onClose();
  };

  const handleClose = () => {
    // Reset form when closing
    setQuantity(1);
    setSelectedOptions(
      (item.options || []).map((option) => ({ ...option, selected: false })),
    );
    setSpecialRequests("");
    onClose();
  };

  const totalPrice = calculateTotalPrice();
  const hasOptions = item.options && item.options.length > 0;

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      size="2xl"
      scrollBehavior="inside"
    >
      <ModalContent>
        <ModalHeader className="flex flex-col gap-1">
          <h3 className="text-xl font-semibold">{tString("title")}</h3>
        </ModalHeader>

        <ModalBody>
          {/* Item Details */}
          <Card>
            <CardBody className="p-4">
              <div className="flex gap-4">
                {item.image && (
                  <Image
                    src={item.image}
                    alt={item.name}
                    className="w-24 h-24 object-cover rounded-lg flex-shrink-0"
                  />
                )}
                <div className="flex-1">
                  <h4 className="text-lg font-semibold">{item.name}</h4>
                  <p className="text-default-600 text-sm mb-2">
                    {item.description}
                  </p>
                  <Chip color="primary" variant="flat">
                    {tString("basePrice")}: {formatCurrency(item.price || 0)}
                  </Chip>
                </div>
              </div>
            </CardBody>
          </Card>

          {/* Quantity Selector */}
          <Card>
            <CardBody className="p-4">
              <h5 className="font-medium mb-3">{tString("quantity")}</h5>
              <div className="flex items-center gap-3">
                <Button
                  isIconOnly
                  size="sm"
                  variant="flat"
                  aria-label={tString("decreaseQuantity") || "Decrease quantity"}
                  onPress={() => setQuantity(Math.max(1, quantity - 1))}
                  isDisabled={quantity <= 1}
                >
                  <Minus className="w-4 h-4" />
                </Button>
                <Input
                  size="sm"
                  className="w-20"
                  aria-label={tString("quantity") || "Quantity"}
                  value={quantity.toString()}
                  onChange={(e) => {
                    const value = parseInt(e.target.value, 10) || 1;
                    setQuantity(Math.max(1, value));
                  }}
                />
                <Button
                  isIconOnly
                  size="sm"
                  variant="flat"
                  aria-label={tString("increaseQuantity") || "Increase quantity"}
                  onPress={() => setQuantity(quantity + 1)}
                >
                  <Plus className="w-4 h-4" />
                </Button>
              </div>
            </CardBody>
          </Card>

          {/* Add-ons/Options */}
          {hasOptions && (
            <Card>
              <CardBody className="p-4">
                <h5 className="font-medium mb-3">{tString("addOnsOptions")}</h5>
                <div className="space-y-3">
                  {selectedOptions.map((option, index) => (
                    <div
                      key={index}
                      className="flex items-center justify-between"
                    >
                      <Checkbox
                        isSelected={option.selected}
                        onValueChange={() => handleOptionToggle(index)}
                      >
                        <div className="flex flex-col">
                          <span className="text-sm font-medium">
                            {option.name}
                          </span>
                          {option.is_required && (
                            <span className="text-xs text-danger">
                              {tString("required")}
                            </span>
                          )}
                        </div>
                      </Checkbox>
                      <Chip
                        size="sm"
                        color={
                          option.price_change === 0
                            ? "default"
                            : option.price_change > 0
                              ? "success"
                              : "danger"
                        }
                        variant="flat"
                      >
                        {option.price_change === 0
                          ? tString("free")
                          : option.price_change > 0
                            ? `+${formatCurrency(option.price_change)}`
                            : `-${formatCurrency(Math.abs(option.price_change))}`}
                      </Chip>
                    </div>
                  ))}
                </div>
              </CardBody>
            </Card>
          )}

          {/* Special Requests */}
          {allowSpecialRequests && (
            <Card>
              <CardBody className="p-4">
                <h5 className="font-medium mb-3">{tString("specialRequests")}</h5>
                <Input
                  placeholder={tString("specialRequestsPlaceholder")}
                  value={specialRequests}
                  onValueChange={setSpecialRequests}
                  variant="bordered"
                  maxLength={200}
                />
                <p className="text-xs text-default-500 mt-1">
                  {specialRequests.length}/200 {tString("characters")}
                </p>
              </CardBody>
            </Card>
          )}

          <Divider />

          {/* Price Summary */}
          <Card className="bg-primary-50 border-primary-200">
            <CardBody className="p-4">
              <div className="space-y-2">
                <div className="flex justify-between text-sm">
                  <span>
                    {tString("basePriceQuantity")} ({quantity}x):
                  </span>
                  <span>
                    {formatCurrency(
                      roundDollars((item.price || 0) * quantity) as number,
                    )}
                  </span>
                </div>
                {Array.isArray(selectedOptions) &&
                  selectedOptions.some((option) => option.selected) && (
                    <div className="flex justify-between text-sm">
                      <span>
                        {tString("addOnsQuantity")} ({quantity}x):
                      </span>
                      <span>
                        {formatCurrency(
                          roundDollars(
                            selectedOptions
                              .filter((option) => option.selected)
                              .reduce(
                                (sum, option) =>
                                  sum + (option.price_change || 0),
                                0,
                              ) * quantity,
                          ) as number,
                        )}
                      </span>
                    </div>
                  )}
                <Divider />
                <div className="flex justify-between font-semibold text-lg">
                  <span>{tString("total")}:</span>
                  <span className="text-primary">{formatCurrency(totalPrice)}</span>
                </div>
              </div>
            </CardBody>
          </Card>
        </ModalBody>

        <ModalFooter>
          <Button variant="light" onPress={handleClose}>
            {tString("cancel")}
          </Button>
          <Button
            color="primary"
            onPress={handleAddToCart}
            isDisabled={missingRequiredOptions.length > 0}
          >
            {submitLabel || tString("addToCart")} - {formatCurrency(totalPrice)}
          </Button>
        </ModalFooter>
      </ModalContent>
    </Modal>
  );
};
