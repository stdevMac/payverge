import React, { useEffect, useMemo, useState } from "react";
import {
  Button,
  Popover,
  PopoverContent,
  PopoverTrigger,
  Select,
  SelectItem,
  Tooltip,
} from "@nextui-org/react";
import { Globe } from "lucide-react";
import { BusinessLanguage, SupportedLanguage } from "../../../../api/currency";
import { btnGhostIcon } from "@/components/ui/buttonStyles";
import ConfirmationModal from "../../modals/ConfirmationModal";
import {
  applyDefaultLanguageDraft,
  applyLockedLanguageDraft,
  applyUnlockedLanguageDraft,
  canSaveLanguageDraft,
  languageDraftsDiffer,
  type LanguageDraft,
} from "../../../../utils/businessLanguages";

interface LanguagesPopoverProps {
  tString: (key: string) => string;
  isLocked: boolean;
  supportedLanguages: SupportedLanguage[];
  businessLanguages: BusinessLanguage[];
  selectedLanguages: string[];
  setSelectedLanguages: (langs: string[]) => void;
  defaultLanguage: string;
  setDefaultLanguage: (lang: string) => void;
  isLanguageLoading: boolean;
  hasLanguageChanges: boolean;
  handleLanguageUpdate: (
    selectedLanguages: string[],
    defaultLanguage: string,
  ) => void;
}

export function LanguagesPopover({
  tString,
  isLocked,
  supportedLanguages,
  businessLanguages,
  selectedLanguages,
  defaultLanguage,
  isLanguageLoading,
  handleLanguageUpdate,
}: LanguagesPopoverProps) {
  const [open, setOpen] = useState(false);
  const [draftSelected, setDraftSelected] = useState(selectedLanguages);
  const [draftDefault, setDraftDefault] = useState(defaultLanguage);
  const [needsDefaultChoice, setNeedsDefaultChoice] = useState(false);
  const [relabelConfirmOpen, setRelabelConfirmOpen] = useState(false);

  const savedDraft: LanguageDraft = useMemo(
    () => ({ selected: selectedLanguages, defaultLanguage }),
    [selectedLanguages, defaultLanguage],
  );

  useEffect(() => {
    if (open) {
      setDraftSelected(selectedLanguages);
      setDraftDefault(defaultLanguage);
      setNeedsDefaultChoice(false);
      return;
    }
    setRelabelConfirmOpen(false);
    // Snapshot saved languages only when the popover opens so unsaved
    // draft edits are not overwritten by parent re-renders.
    // eslint-disable-next-line react-hooks/exhaustive-deps -- open is the reset trigger
  }, [open]);

  const applyDraft = (next: {
    selected: string[];
    defaultLanguage: string;
    needsDefaultChoice: boolean;
  }) => {
    setDraftSelected(next.selected);
    setDraftDefault(next.defaultLanguage);
    setNeedsDefaultChoice(next.needsDefaultChoice);
  };

  const draftDirty = languageDraftsDiffer(
    { selected: draftSelected, defaultLanguage: draftDefault },
    savedDraft,
  );
  const canSave = canSaveLanguageDraft({
    selected: draftSelected,
    defaultLanguage: draftDefault,
  });
  const requiresRelabelConfirm =
    isLocked && draftDefault !== defaultLanguage && canSave;

  const persistDraft = () => {
    handleLanguageUpdate(draftSelected, draftDefault);
    setOpen(false);
  };

  const handleSave = () => {
    if (!canSave) {
      setNeedsDefaultChoice(true);
      return;
    }
    if (requiresRelabelConfirm) {
      setRelabelConfirmOpen(true);
      return;
    }
    persistDraft();
  };

  const relabelLanguageName =
    supportedLanguages.find((language) => language.code === draftDefault)
      ?.native_name || draftDefault;

  const activeLanguageCount = businessLanguages.length || 1;

  return (
    <>
      <Popover
        isOpen={open}
        onOpenChange={setOpen}
        placement="bottom-end"
        offset={8}
        classNames={{ content: "p-0" }}
      >
        <PopoverTrigger>
          {/* Native button so the badge can render outside the trigger
                    bounds — NextUI's Button applies overflow-hidden which
                    was clipping the count to the inside of the icon square. */}
          <button
            type="button"
            aria-label={tString("header.languagesAria")}
            className={"relative " + btnGhostIcon}
          >
            <Globe className="w-4 h-4" />
            {activeLanguageCount > 1 && (
              <span className="absolute -top-1.5 -right-1.5 min-w-[20px] h-5 px-1.5 rounded-full bg-brand text-white text-[10px] font-semibold inline-flex items-center justify-center border-2 border-white shadow-sm leading-none">
                {activeLanguageCount}
              </span>
            )}
          </button>
        </PopoverTrigger>
        <PopoverContent className="w-[320px] overflow-hidden rounded-3xl border border-warm-200 bg-white shadow-2xl shadow-warm-900/15 sm:w-[360px]">
          <div className="w-full p-4 space-y-4">
            <div>
              <h3 className="text-sm font-semibold text-ink-950">
                {tString("languagesPopover.title")}
              </h3>
              <p className="mt-0.5 text-xs text-ink-600">
                {tString("languagesPopover.subtitle")}
              </p>
            </div>

            {/* Language selection */}
            <div>
              <label className="mb-1.5 block text-[11px] font-semibold uppercase tracking-wide text-ink-600">
                {isLocked
                  ? tString("languages.defaultLanguage")
                  : tString("languages.supportedLanguages")}
              </label>
              {isLocked ? (
                <Select
                  aria-label={tString("languages.defaultLanguage")}
                  placeholder={tString("languages.selectLanguage")}
                  selectedKeys={
                    draftDefault ? new Set([draftDefault]) : new Set()
                  }
                  onSelectionChange={(keys) => {
                    const newDefault = Array.from(keys)[0] as string;
                    if (newDefault) {
                      applyDraft(
                        applyLockedLanguageDraft(
                          {
                            selected: draftSelected,
                            defaultLanguage: draftDefault,
                          },
                          newDefault,
                        ),
                      );
                    }
                  }}
                  isLoading={isLanguageLoading}
                  size="sm"
                >
                  {supportedLanguages.map((lang) => (
                    <SelectItem key={lang.code} value={lang.code}>
                      {lang.native_name}
                    </SelectItem>
                  ))}
                </Select>
              ) : (
                <Select
                  aria-label={tString("languages.supportedLanguages")}
                  placeholder={tString("languages.selectLanguages")}
                  selectionMode="multiple"
                  selectedKeys={new Set(draftSelected)}
                  onSelectionChange={(keys) => {
                    applyDraft(
                      applyUnlockedLanguageDraft(
                        {
                          selected: draftSelected,
                          defaultLanguage: draftDefault,
                        },
                        Array.from(keys) as string[],
                      ),
                    );
                  }}
                  isLoading={isLanguageLoading}
                  size="sm"
                >
                  {supportedLanguages.map((lang) => (
                    <SelectItem key={lang.code} value={lang.code}>
                      {lang.native_name}
                    </SelectItem>
                  ))}
                </Select>
              )}
            </div>

            {!isLocked && (draftSelected.length > 1 || needsDefaultChoice) && (
              <div>
                <label className="mb-1.5 block text-[11px] font-semibold uppercase tracking-wide text-ink-600">
                  {tString("languages.defaultLanguage")}
                </label>
                <Select
                  aria-label={tString("languages.defaultLanguage")}
                  placeholder={tString("languages.setAsDefault")}
                  selectedKeys={
                    draftDefault ? new Set([draftDefault]) : new Set()
                  }
                  onSelectionChange={(keys) => {
                    const newDefault = Array.from(keys)[0] as string;
                    if (newDefault) {
                      applyDraft(
                        applyDefaultLanguageDraft(
                          {
                            selected: draftSelected,
                            defaultLanguage: draftDefault,
                          },
                          newDefault,
                        ),
                      );
                    }
                  }}
                  isLoading={isLanguageLoading}
                  size="sm"
                >
                  {draftSelected.map((langCode) => {
                    const lang = supportedLanguages.find(
                      (l) => l.code === langCode,
                    );
                    return lang ? (
                      <SelectItem key={lang.code} value={lang.code}>
                        {lang.native_name}
                      </SelectItem>
                    ) : null;
                  })}
                </Select>
                {needsDefaultChoice && (
                  <p className="mt-1.5 text-xs text-amber-800">
                    {tString("languages.chooseNewDefault")}
                  </p>
                )}
              </div>
            )}

            {/* Footer */}
            <div className="flex items-center justify-end gap-2 border-t border-warm-100 pt-2">
              {draftDirty ? (
                <Tooltip content={tString("languages.hasChanges")}>
                  <Button
                    size="sm"
                    onPress={handleSave}
                    isLoading={isLanguageLoading}
                    isDisabled={!canSave}
                    className="h-9 bg-brand font-semibold text-white shadow-sm shadow-brand/20 hover:bg-brand-dark"
                  >
                    {tString("languages.saveLanguages")}
                  </Button>
                </Tooltip>
              ) : (
                <Button
                  size="sm"
                  variant="flat"
                  onPress={() => setOpen(false)}
                  className="h-9 bg-warm-100 font-semibold text-ink-700 hover:bg-warm-200"
                >
                  {tString("languagesPopover.close")}
                </Button>
              )}
            </div>
          </div>
        </PopoverContent>
      </Popover>
      <ConfirmationModal
        isOpen={relabelConfirmOpen}
        onOpenChange={() => setRelabelConfirmOpen(false)}
        title={tString("languages.relabelConfirmTitle")}
        description={tString("languages.relabelConfirm").replace(
          "{language}",
          relabelLanguageName,
        )}
        confirmLabel={tString("languages.relabelConfirmButton")}
        onConfirm={persistDraft}
      />
    </>
  );
}
