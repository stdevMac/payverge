"use client";

import React from "react";
import {
  Button,
  Popover,
  PopoverTrigger,
  PopoverContent,
  Modal,
  ModalContent,
  ModalHeader,
  ModalBody,
  ModalFooter,
} from "@nextui-org/react";
import { Sparkles, Wand2, Info, Image as ImageIcon } from "lucide-react";
import { useInstance } from "@/hooks/useInstance";
import { resetWindowCopy } from "@/lib/dailyLimitReset";
import type { DailyLimitInfo } from "../hooks/useMenuMutations";

type ToolKey = "breakdown" | "generate" | "enhance";

interface AIImageToolsProps {
  tString: (key: string, params?: Record<string, string | number>) => string;
  itemName: string;
  itemDescription: string;
  images: string[];
  isGeneratingBreakdown: boolean;
  isGeneratingPhoto: boolean;
  isEnhancing: boolean;
  onBreakdown: () => Promise<void> | void;
  onGenerate: () => Promise<void> | void;
  onEnhance: (imageUrl: string) => Promise<void> | void;
  dailyLimitReached?: DailyLimitInfo | null;
}

/**
 * AI image tools panel — three distinct tools, each with an info popover:
 *  - breakdown: exploded-view ingredient diagram from name + description.
 *  - generate:  realistic food photo from name + description (text-to-image).
 *  - enhance:   improves an existing uploaded photo (image-to-image). When the
 *               item has more than one photo a picker modal asks which one.
 *
 * Not rendered when this install has no LLM provider (GET /instance
 * features.ai).
 */
export function AIImageTools({
  tString,
  itemName,
  itemDescription,
  images,
  isGeneratingBreakdown,
  isGeneratingPhoto,
  isEnhancing,
  onBreakdown,
  onGenerate,
  onEnhance,
  dailyLimitReached,
}: AIImageToolsProps) {
  const aiOff = useInstance().isOff("ai");
  const [pickerOpen, setPickerOpen] = React.useState(false);
  const [selected, setSelected] = React.useState<string | null>(null);

  const namedAndDescribed =
    itemName.trim().length > 0 && itemDescription.trim().length > 0;
  const hasImage = images.length > 0;
  // At the 5-photo cap the generate/breakdown/enhance handlers no-op (they'd
  // exceed maxImages), so disable the buttons instead of letting them
  // dead-click.
  const atImageCap = images.length >= 5;

  const generatingLabel = tString("items.aiImageTools.generating");
  const resetWindow = dailyLimitReached
    ? resetWindowCopy(dailyLimitReached.resetsInSeconds)
    : null;

  const handleEnhancePress = () => {
    if (images.length === 0) return;
    if (images.length === 1) {
      void onEnhance(images[0]);
      return;
    }
    setSelected(images[0]);
    setPickerOpen(true);
  };

  const confirmPicker = () => {
    if (selected) void onEnhance(selected);
    setSelected(null);
    setPickerOpen(false);
  };

  if (aiOff) return null;

  return (
    <div className="rounded-3xl border border-warm-200 bg-white p-4 shadow-sm shadow-warm-900/5">
      <div className="mb-3">
        <div className="min-w-0">
          <h5 className="inline-flex items-center gap-1.5 text-sm font-semibold text-ink-950">
            <Sparkles className="w-4 h-4 text-brand" />
            {tString("items.aiImageTools.title")}
          </h5>
          <p className="mt-0.5 text-xs text-ink-600">
            {tString("items.aiImageTools.subtitle")}
          </p>
        </div>
      </div>

      {dailyLimitReached && resetWindow ? (
        <div
          role="status"
          className="rounded-lg border border-warm-200 bg-warm-50 p-3 text-sm text-warm-700 mb-3"
        >
          {tString("items.aiImageTools.dailyLimit.title", {
            limit: String(dailyLimitReached.dailyLimit),
          })}{" "}
          {resetWindow.key === "resets"
            ? tString("items.aiImageTools.dailyLimit.resets", {
                hours: String(resetWindow.hours),
              })
            : tString(`items.aiImageTools.dailyLimit.${resetWindow.key}`)}{" "}
          {tString("items.aiImageTools.dailyLimit.contact")}
        </div>
      ) : null}

      <div className="grid grid-cols-1 sm:grid-cols-3 gap-2.5">
        <ToolCard
          toolKey="breakdown"
          tString={tString}
          icon={<Sparkles className="w-3.5 h-3.5" />}
          loading={isGeneratingBreakdown}
          disabled={!namedAndDescribed || atImageCap}
          subtitleKey={
            namedAndDescribed
              ? "items.aiImageTools.breakdown.subtitle"
              : "items.aiImageTools.breakdown.subtitleEmpty"
          }
          cta={
            isGeneratingBreakdown
              ? generatingLabel
              : tString("items.aiImageTools.breakdown.cta")
          }
          onPress={() => onBreakdown()}
        />

        <ToolCard
          toolKey="generate"
          tString={tString}
          icon={<ImageIcon className="w-3.5 h-3.5" />}
          loading={isGeneratingPhoto}
          disabled={!namedAndDescribed || atImageCap}
          subtitleKey={
            namedAndDescribed
              ? "items.aiImageTools.generate.subtitle"
              : "items.aiImageTools.generate.subtitleEmpty"
          }
          cta={
            isGeneratingPhoto
              ? generatingLabel
              : tString("items.aiImageTools.generate.cta")
          }
          onPress={() => onGenerate()}
        />

        <ToolCard
          toolKey="enhance"
          tString={tString}
          icon={<Wand2 className="w-3.5 h-3.5" />}
          loading={isEnhancing}
          disabled={!hasImage || atImageCap}
          subtitleKey={
            hasImage
              ? "items.aiImageTools.enhance.subtitle"
              : "items.aiImageTools.enhance.subtitleDisabled"
          }
          cta={
            isEnhancing
              ? generatingLabel
              : tString("items.aiImageTools.enhance.cta")
          }
          onPress={handleEnhancePress}
        />
      </div>

      {/* Enhance picker — shown only when the item has more than one photo. */}
      <Modal isOpen={pickerOpen} onOpenChange={setPickerOpen} size="lg">
        <ModalContent className="overflow-hidden rounded-3xl border border-warm-200 bg-white shadow-2xl shadow-warm-900/15">
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-0.5 border-b border-warm-200/80 bg-warm-50/70 px-6 py-5 text-ink-950">
                <span>
                  {tString("items.aiImageTools.enhance.picker.title")}
                </span>
                <span className="text-xs font-normal text-ink-500">
                  {tString("items.aiImageTools.enhance.picker.subtitle")}
                </span>
              </ModalHeader>
              <ModalBody className="py-5">
                <div className="grid grid-cols-3 gap-3">
                  {images.map((url, i) => {
                    const isSel = selected === url;
                    return (
                      <button
                        key={url}
                        type="button"
                        aria-label={`${tString("items.aiImageTools.enhance.picker.option")} ${i + 1}`}
                        aria-pressed={isSel}
                        onClick={() => setSelected(url)}
                        className={`relative aspect-square overflow-hidden rounded-lg border-2 transition ${
                          isSel
                            ? "border-brand ring-2 ring-brand/30"
                            : "border-warm-200 hover:border-brand/40"
                        }`}
                      >
                        {/* eslint-disable-next-line @next/next/no-img-element */}
                        <img
                          src={url}
                          alt=""
                          className="w-full h-full object-cover"
                        />
                      </button>
                    );
                  })}
                </div>
              </ModalBody>
              <ModalFooter className="border-t border-warm-200/80 bg-warm-50/70 px-6 py-4">
                <Button
                  variant="light"
                  onPress={onClose}
                  className="font-semibold text-ink-700 hover:bg-warm-100"
                >
                  {tString("items.aiImageTools.enhance.picker.cancel")}
                </Button>
                <Button
                  isDisabled={!selected}
                  onPress={confirmPicker}
                  className="bg-brand font-semibold text-white shadow-sm shadow-brand/20 hover:bg-brand-dark"
                >
                  {tString("items.aiImageTools.enhance.picker.confirm")}
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </div>
  );
}

interface ToolCardProps {
  toolKey: ToolKey;
  tString: (key: string) => string;
  icon: React.ReactNode;
  loading: boolean;
  disabled: boolean;
  subtitleKey: string;
  cta: string;
  onPress: () => void;
}

function ToolCard({
  toolKey,
  tString,
  icon,
  loading,
  disabled,
  subtitleKey,
  cta,
  onPress,
}: ToolCardProps) {
  // Cards disable when their precondition (name+desc, or an uploaded photo)
  // is unmet.
  const btnDisabled = disabled && !loading;

  return (
    <div
      className={`relative flex flex-col rounded-2xl border p-3 shadow-sm shadow-warm-900/5 ${
        "border-warm-200 bg-white"
      }`}
    >
      <div className="flex items-start gap-2 mb-1.5">
        <div
          className={`w-7 h-7 rounded-md border flex items-center justify-center flex-shrink-0 ${
            "bg-brand/10 border-brand/20 text-brand"
          }`}
        >
          {icon}
        </div>
        <h6 className="flex-1 pt-0.5 text-xs font-semibold leading-tight text-ink-950">
          {tString(`items.aiImageTools.${toolKey}.title`)}
        </h6>
        <Popover placement="top" showArrow>
          <PopoverTrigger>
            <button
              type="button"
              aria-label={tString(
                `items.aiImageTools.${toolKey}.info.ariaLabel`,
              )}
              className="flex-shrink-0 text-ink-400 hover:text-ink-700"
            >
              <Info className="w-3.5 h-3.5" />
            </button>
          </PopoverTrigger>
          <PopoverContent>
            <div className="max-w-[16rem] p-1.5">
              <p className="mb-1 text-xs font-semibold text-ink-950">
                {tString(`items.aiImageTools.${toolKey}.info.title`)}
              </p>
              <p className="text-[11px] leading-relaxed text-ink-600">
                {tString(`items.aiImageTools.${toolKey}.info.body`)}
              </p>
              {/* The per-tool example-image placeholder was removed: a
                                dashed "coming soon" box reaching live operator surfaces
                                read as unfinished. Re-add a real example image per tool
                                here once the assets exist (2026-06-04 design spec). */}
            </div>
          </PopoverContent>
        </Popover>
      </div>
      <p className="mb-3 flex-1 text-[11px] leading-relaxed text-ink-600">
        {tString(subtitleKey)}
      </p>
      <Button
        type="button"
        size="sm"
        variant="solid"
        onPress={onPress}
        isDisabled={btnDisabled}
        isLoading={loading}
        className={`h-8 text-xs font-medium w-full ${
          "bg-brand text-white"
        }`}
        startContent={
          !loading ? icon : null
        }
      >
        {cta}
      </Button>
    </div>
  );
}
