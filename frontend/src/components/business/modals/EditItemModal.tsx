"use client";

import React, { useEffect, useState } from "react";
import {
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
  Button,
  Input,
  Textarea,
  Chip,
  Switch,
  Tabs,
  Tab,
} from "@nextui-org/react";
import {
  Edit,
  DollarSign,
  AlertTriangle,
  Tag,
} from "lucide-react";
import { MenuCategory, MenuItemOption } from "../../../api/business";
import { formatCurrency as formatCurrencyIntl } from "../../../api/currency";
import MultipleImageUpload from "../MultipleImageUpload";
import { AIImageTools } from "../MenuBuilder/components/AIImageTools";
import type { DailyLimitInfo } from "../MenuBuilder/hooks/useMenuMutations";
import TagSelector from "../TagSelector";
import { tryParseLocaleDecimal } from "@/lib/parseLocaleDecimal";
import { DecimalInput } from "@/components/ui/DecimalInput";

interface EditItemModalProps {
  isOpen: boolean;
  onOpenChange: () => void;
  selectedCategoryIndex: number | null;
  menu: MenuCategory[];
  itemName: string;
  setItemName: (name: string) => void;
  itemDescription: string;
  setItemDescription: (description: string) => void;
  itemPrice: string;
  setItemPrice: (price: string) => void;
  itemCogs: string;
  setItemCogs: (cogs: string) => void;
  defaultCurrency: string;
  itemImages: string[];
  setItemImages: (images: string[]) => void;
  itemAvailable: boolean;
  setItemAvailable: (available: boolean) => void;
  itemSortOrder: number;
  setItemSortOrder: (order: number) => void;
  businessId: number;
  itemOptions: MenuItemOption[];
  newOptionName: string;
  setNewOptionName: (name: string) => void;
  newOptionPrice: string;
  setNewOptionPrice: (price: string) => void;
  onAddOption: () => void;
  onRemoveOption: (index: number) => void;
  itemAllergens: string[];
  newAllergen: string;
  setNewAllergen: (allergen: string) => void;
  onAddAllergen: (allergen: string) => void;
  onRemoveAllergen: (allergen: string) => void;
  itemDietaryTags: string[];
  newDietaryTag: string;
  setNewDietaryTag: (tag: string) => void;
  onAddDietaryTag: (tag: string) => void;
  onRemoveDietaryTag: (tag: string) => void;
  onUpdateItem: () => void;
  onResetForm: () => void;
  tString: (key: string) => string;

  onGenerateBreakdown?: (name: string, description: string, dietaryTags?: string[]) => Promise<string | null>;
  onGeneratePhoto?: (name: string, description: string, dietaryTags?: string[]) => Promise<string | null>;
  onEnhancePhoto?: (name: string, description: string, imageUrl: string, dietaryTags?: string[]) => Promise<string | null>;
  isGeneratingBreakdown?: boolean;
  isGeneratingPhoto?: boolean;
  isEnhancing?: boolean;
  isSaving?: boolean;
  dailyLimitReached?: DailyLimitInfo | null;
}

export default function EditItemModal({
  isOpen,
  onOpenChange,
  selectedCategoryIndex,
  menu,
  itemName,
  setItemName,
  itemDescription,
  setItemDescription,
  itemPrice,
  setItemPrice,
  itemCogs,
  setItemCogs,
  defaultCurrency,
  itemImages,
  setItemImages,
  itemAvailable,
  setItemAvailable,
  itemSortOrder,
  setItemSortOrder,
  businessId,
  itemOptions,
  newOptionName,
  setNewOptionName,
  newOptionPrice,
  setNewOptionPrice,
  onAddOption,
  onRemoveOption,
  itemAllergens,
  newAllergen: _newAllergen,
  setNewAllergen: _setNewAllergen,
  onAddAllergen,
  onRemoveAllergen: _onRemoveAllergen,
  itemDietaryTags,
  newDietaryTag: _newDietaryTag,
  setNewDietaryTag: _setNewDietaryTag,
  onAddDietaryTag,
  onRemoveDietaryTag: _onRemoveDietaryTag,
  onUpdateItem,
  onResetForm,
  tString,
  onGenerateBreakdown,
  onGeneratePhoto,
  onEnhancePhoto,
  isGeneratingBreakdown = false,
  isGeneratingPhoto = false,
  isEnhancing = false,
  isSaving = false,
  dailyLimitReached,
}: EditItemModalProps) {
  // L3-2 / L3-5: tryParse returns null for garbage; never treat garbage as $0.
  const parsedPrice = tryParseLocaleDecimal(itemPrice);
  const priceValid = parsedPrice !== null && parsedPrice >= 0;
  // A3b: optional COGS — empty OK; garbage / negative → error slot + Save off.
  const parsedCogs = tryParseLocaleDecimal(itemCogs);
  const cogsInvalid =
    itemCogs.trim() !== "" &&
    (parsedCogs === null || parsedCogs < 0);
  // A raw "-5"/"5,50" option price is a truthy string, so the old
  // `!newOptionPrice` check left the Add button enabled and the shared handler
  // then silently discarded the option with no feedback. Gate on a finite,
  // non-negative parse instead, and only surface inline feedback once the
  // operator has actually typed something invalid (not on a pristine field).
  const parsedOptionPrice = tryParseLocaleDecimal(newOptionPrice);
  const optionPriceValid =
    parsedOptionPrice !== null && parsedOptionPrice >= 0;
  const optionPriceInvalid =
    newOptionPrice.trim() !== "" && !optionPriceValid;
  // #117: progressive disclosure — ops fields first; enrichment behind a tab.
  const [section, setSection] = useState<"basics" | "details">("basics");
  useEffect(() => {
    if (isOpen) setSection("basics");
  }, [isOpen]);
  return (
    <Modal
      isOpen={isOpen}
      onOpenChange={(next) => {
        // Parent onOpenChange is a close hook (or a useDisclosure toggle that
        // ignores the boolean). Only propagate close so an open/remount signal
        // cannot flip the dialog back open (#388).
        if (next === false) onOpenChange();
      }}
      size="3xl"
      scrollBehavior="inside"
      // NextUI 2.6.11: Tabs layoutId cursor + Modal AnimatePresence leave a
      // blocking overlay after the section changes (heroui#4561). Operators
      // then cannot Close / Cancel / Escape without a full reload.
      disableAnimation
    >
      <ModalContent className="overflow-hidden rounded-3xl border border-warm-200 bg-white shadow-2xl shadow-warm-900/15">
        {(onClose) => (
          <>
            <ModalHeader className="flex flex-col gap-1 border-b border-warm-200/80 bg-warm-50/70 px-6 py-5">
              <div className="flex min-w-0 items-center gap-3">
                <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-brand/15 bg-brand/10 text-brand-dark shadow-sm shadow-brand/10">
                  <Edit className="h-4 w-4" />
                </span>
                <div className="flex min-w-0 flex-wrap items-center gap-2">
                  <span className="text-lg font-semibold tracking-normal text-ink-950">
                    {tString("items.editItem")}
                  </span>
                  {menu && selectedCategoryIndex !== null && menu[selectedCategoryIndex] && (
                    <span className="rounded-full border border-brand/15 bg-brand/10 px-2.5 py-1 text-xs font-semibold text-brand-dark">
                      {menu[selectedCategoryIndex].name}
                    </span>
                  )}
                </div>
              </div>
            </ModalHeader>
            <ModalBody className="gap-4 bg-white px-6 py-5">
              <Tabs
                aria-label={tString("items.editItem")}
                selectedKey={section}
                onSelectionChange={(key) => {
                  const next = String(key);
                  if (next === "basics" || next === "details") setSection(next);
                }}
                size="sm"
                variant="underlined"
                disableAnimation
                classNames={{
                  tabList: "gap-4 border-b border-warm-200 w-full",
                  tab: "px-0 h-8 text-sm font-medium text-ink-500",
                  cursor: "bg-brand",
                }}
                data-testid="edit-item-sections"
              >
                <Tab
                  key="basics"
                  title={tString("items.editSectionBasics") || "Basics"}
                  data-testid="edit-item-section-basics"
                />
                <Tab
                  key="details"
                  title={tString("items.editSectionDetails") || "Photos & tags"}
                  data-testid="edit-item-section-details"
                />
              </Tabs>

              {section === "basics" ? (
                <div className="space-y-4" data-testid="edit-item-basics-panel">
                  {/* Name + price — operationally urgent controls up top. */}
                  <div className="grid grid-cols-1 gap-3 md:grid-cols-[minmax(0,1fr)_160px]">
                    <Input
                      label={tString("items.itemName")}
                      placeholder={tString("items.itemNamePlaceholder")}
                      value={itemName}
                      onValueChange={setItemName}
                      isRequired
                      variant="bordered"
                    />
                    <DecimalInput
                      label={tString("items.itemPrice")}
                      placeholder={tString("items.itemPricePlaceholder")}
                      value={itemPrice}
                      onValueChange={setItemPrice}
                      isRequired
                      min={0}
                      isInvalid={itemPrice.trim() !== "" && !priceValid}
                      errorMessage={
                        itemPrice.trim() !== "" && !priceValid
                          ? tString("validation.invalidPrice")
                          : undefined
                      }
                      variant="bordered"
                      startContent={
                        <DollarSign className="w-4 h-4 text-ink-400" />
                      }
                      endContent={
                        <span className="text-xs font-medium text-ink-500">
                          {defaultCurrency}
                        </span>
                      }
                      data-testid="edit-item-price"
                    />
                  </div>
                  <Textarea
                    label={tString("items.itemDescription")}
                    placeholder={tString("items.itemDescriptionPlaceholder")}
                    value={itemDescription}
                    onValueChange={setItemDescription}
                    minRows={2}
                    variant="bordered"
                  />
                  <div className="flex items-center justify-between gap-4 rounded-2xl border border-warm-200/80 bg-warm-50/70 px-3 py-2.5 shadow-sm shadow-warm-900/5">
                    <Switch
                      isSelected={itemAvailable}
                      onValueChange={setItemAvailable}
                      color="success"
                      size="sm"
                    >
                      <span className="text-sm font-medium text-ink-700">
                        {tString("items.available")}
                      </span>
                    </Switch>
                    <div className="flex items-center gap-2">
                      <span className="text-xs font-medium text-ink-500">
                        {tString("items.sortOrder")}
                      </span>
                      <Input
                        aria-label={tString("items.sortOrder")}
                        placeholder="0"
                        value={itemSortOrder.toString()}
                        onValueChange={(value) =>
                          setItemSortOrder(parseInt(value, 10) || 0)
                        }
                        type="number"
                        size="sm"
                        classNames={{
                          base: "w-20",
                          inputWrapper: "h-8 min-h-8 border border-warm-200 bg-white",
                        }}
                      />
                    </div>
                  </div>

                  {/* Options stay on Basics — common mid-shift tweak. */}
                  <section className="space-y-3 rounded-2xl border border-warm-200/80 bg-warm-50/50 p-3 shadow-sm shadow-warm-900/5">
                    <header className="flex items-center gap-2">
                      <DollarSign className="w-4 h-4 text-ink-600" />
                      <h4 className="text-sm font-semibold text-ink-950">
                        {tString("items.options")}
                      </h4>
                      {itemOptions.length > 0 && (
                        <span className="text-xs font-medium text-ink-500">
                          · {itemOptions.length}
                        </span>
                      )}
                    </header>
                    <div className="flex flex-col sm:flex-row gap-2">
                      <Input
                        aria-label={tString("items.optionName")}
                        placeholder={tString("items.optionNamePlaceholder")}
                        value={newOptionName}
                        onValueChange={setNewOptionName}
                        variant="bordered"
                        size="sm"
                        classNames={{ inputWrapper: "h-9 min-h-9" }}
                      />
                      <DecimalInput
                        aria-label={tString("items.optionPrice")}
                        placeholder={tString("items.optionPricePlaceholder")}
                        value={newOptionPrice}
                        onValueChange={setNewOptionPrice}
                        min={0}
                        variant="bordered"
                        size="sm"
                        isInvalid={optionPriceInvalid}
                        errorMessage={
                          optionPriceInvalid
                            ? tString("validation.invalidPrice")
                            : undefined
                        }
                        startContent={
                          <DollarSign className="w-3.5 h-3.5 text-ink-400" />
                        }
                        classNames={{
                          base: "w-full sm:w-32",
                          inputWrapper: "h-9 min-h-9",
                        }}
                        data-testid="edit-item-option-price"
                      />
                      <Button
                        type="button"
                        variant="flat"
                        size="sm"
                        onPress={onAddOption}
                        isDisabled={!newOptionName.trim() || !optionPriceValid}
                        className="h-9 self-stretch border border-brand/15 bg-brand/10 px-4 text-sm font-semibold text-brand-dark shadow-sm shadow-brand/10 transition-colors hover:bg-brand/15 sm:self-auto"
                      >
                        {tString("buttons.add")}
                      </Button>
                    </div>
                    {itemOptions.length > 0 && (
                      <div className="flex flex-wrap gap-1.5">
                        {itemOptions.map((option, index) => (
                          <Chip
                            key={`option-${option.name}-${index}`}
                            onClose={() => onRemoveOption(index)}
                            variant="flat"
                            size="sm"
                            className="border border-brand/15 bg-brand/10 font-medium text-brand-dark"
                          >
                            {option.name} +{formatCurrencyIntl(option.price_change || 0, defaultCurrency || "USD")}
                          </Chip>
                        ))}
                      </div>
                    )}
                  </section>
                </div>
              ) : (
                <div className="space-y-4" data-testid="edit-item-details-panel">
                  {/* Full-width food-cost field so the helper is not clipped (#130/#150). */}
                  <div className="space-y-1.5">
                    <DecimalInput
                      label={tString("items.itemCogs") || "Food cost (per plate)"}
                      placeholder={tString("items.itemCogsPlaceholder") || "0.00"}
                      value={itemCogs}
                      onValueChange={setItemCogs}
                      min={0}
                      isInvalid={cogsInvalid}
                      errorMessage={
                        cogsInvalid
                          ? tString("validation.invalidPrice")
                          : undefined
                      }
                      variant="bordered"
                      startContent={
                        <DollarSign className="w-4 h-4 text-ink-400" />
                      }
                      endContent={
                        <span className="text-xs font-medium text-ink-500">
                          {defaultCurrency}
                        </span>
                      }
                      data-testid="edit-item-cogs"
                    />
                    <p
                      className="text-xs leading-relaxed text-ink-500"
                      data-testid="edit-item-cogs-hint"
                    >
                      {tString("items.itemCogsHint") ||
                        "Optional plate cost used when no Inventory recipe is linked."}
                    </p>
                  </div>

                  <MultipleImageUpload
                    images={itemImages}
                    onImagesChange={setItemImages}
                    maxImages={5}
                    businessId={businessId}
                  />

                  {(onGenerateBreakdown || onGeneratePhoto || onEnhancePhoto) && (
                    <div data-testid="edit-item-ai-tools">
                      <AIImageTools
                        tString={tString}
                        itemName={itemName}
                        itemDescription={itemDescription}
                        images={itemImages}
                        isGeneratingBreakdown={!!isGeneratingBreakdown}
                        isGeneratingPhoto={!!isGeneratingPhoto}
                        isEnhancing={!!isEnhancing}
                        onBreakdown={async () => {
                          if (!onGenerateBreakdown || itemImages.length >= 5) return;
                          const url = await onGenerateBreakdown(itemName, itemDescription, itemDietaryTags);
                          if (url) setItemImages([...itemImages, url]);
                        }}
                        onGenerate={async () => {
                          if (!onGeneratePhoto || itemImages.length >= 5) return;
                          const url = await onGeneratePhoto(itemName, itemDescription, itemDietaryTags);
                          if (url) setItemImages([...itemImages, url]);
                        }}
                        onEnhance={async (imageUrl) => {
                          if (!onEnhancePhoto || itemImages.length >= 5) return;
                          const url = await onEnhancePhoto(itemName, itemDescription, imageUrl, itemDietaryTags);
                          if (url) setItemImages([...itemImages, url]);
                        }}
                        dailyLimitReached={dailyLimitReached}
                      />
                    </div>
                  )}

                  <section className="space-y-2 rounded-2xl border border-warm-200/80 bg-white p-3 shadow-sm shadow-warm-900/5">
                    <header className="flex items-center gap-2">
                      <AlertTriangle className="w-4 h-4 text-amber-500" />
                      <h4 className="text-sm font-semibold text-ink-950">
                        {tString("items.allergens")}
                      </h4>
                      {itemAllergens.length > 0 && (
                        <span className="text-xs font-medium text-ink-500">
                          · {itemAllergens.length}
                        </span>
                      )}
                    </header>
                    <TagSelector
                      type="allergens"
                      selectedTags={itemAllergens}
                      onToggleTag={onAddAllergen}
                    />
                  </section>

                  <section className="space-y-2 rounded-2xl border border-warm-200/80 bg-white p-3 shadow-sm shadow-warm-900/5">
                    <header className="flex items-center gap-2">
                      <Tag className="w-4 h-4 text-emerald-500" />
                      <h4 className="text-sm font-semibold text-ink-950">
                        {tString("items.dietaryTags")}
                      </h4>
                      {itemDietaryTags.length > 0 && (
                        <span className="text-xs font-medium text-ink-500">
                          · {itemDietaryTags.length}
                        </span>
                      )}
                    </header>
                    <TagSelector
                      type="dietary"
                      selectedTags={itemDietaryTags}
                      onToggleTag={onAddDietaryTag}
                    />
                  </section>
                </div>
              )}
            </ModalBody>
            <ModalFooter className="gap-2 border-t border-warm-200/80 bg-warm-50/70 px-6 py-4">
              <button
                type="button"
                onClick={() => {
                  onResetForm();
                  onClose();
                }}
                className="rounded-xl px-4 py-2 text-sm font-semibold text-ink-600 transition-colors hover:bg-white hover:text-ink-950"
              >
                {tString("buttons.cancel")}
              </button>
              <button
                type="button"
                onClick={onUpdateItem}
                disabled={
                  !itemName.trim() || !priceValid || cogsInvalid || isSaving
                }
                className="rounded-xl bg-brand px-4 py-2 text-sm font-semibold text-white shadow-sm shadow-brand/20 transition-colors hover:bg-brand-dark disabled:cursor-not-allowed disabled:opacity-50"
              >
                {isSaving
                  ? tString("buttons.saving")
                  : tString("buttons.updateItem")}
              </button>
            </ModalFooter>
          </>
        )}
      </ModalContent>
    </Modal>
  );
}
