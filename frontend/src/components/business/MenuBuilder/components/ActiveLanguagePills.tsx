import React from "react";
import { Languages, RefreshCw, X, AlertTriangle } from "lucide-react";
import { Tooltip } from "@nextui-org/react";
import { BusinessLanguage, SupportedLanguage } from "../../../../api/currency";

interface ActiveLanguagePillsProps {
  tString: (key: string) => string;
  supportedLanguages: SupportedLanguage[];
  businessLanguages: BusinessLanguage[];
  currentViewLanguage: string;
  setCurrentViewLanguage: (lang: string) => void;
  loadMenu: (lang: string) => void;
  handleTranslateMenu: (lang: string) => void;
  handleSyncAllTranslations: () => void;
  cancelSyncAllTranslations: () => void;
  isTranslating: boolean;
  syncProgress: { done: number; total: number } | null;
  syncError: string | null;
  defaultLanguage: string;
}

export function ActiveLanguagePills({
  tString,
  supportedLanguages,
  businessLanguages,
  currentViewLanguage,
  setCurrentViewLanguage,
  handleTranslateMenu,
  handleSyncAllTranslations,
  cancelSyncAllTranslations,
  isTranslating,
  syncProgress,
  syncError,
  defaultLanguage,
}: ActiveLanguagePillsProps) {
  if (businessLanguages.length <= 1) return null;

  const targetLangNative = supportedLanguages.find(
    (l) => l.code === currentViewLanguage,
  )?.native_name;

  const syncBusy = isTranslating && syncProgress !== null;
  // Real job progress: done/total strings processed. total is 0 until the job
  // reports its first status, so guard the percentage.
  const pct =
    syncBusy && syncProgress!.total > 0
      ? Math.min(
          100,
          Math.round((syncProgress!.done / syncProgress!.total) * 100),
        )
      : 0;
  const syncLabel = syncBusy
    ? tString("languages.syncing")
        .replace("{done}", String(syncProgress!.done))
        .replace("{total}", String(syncProgress!.total))
    : tString("languages.syncTranslations");

  return (
    <div className="mb-4 flex flex-col gap-2 rounded-3xl border border-warm-200 bg-white p-3 shadow-sm shadow-warm-900/5">
      <div className="flex flex-wrap items-center gap-2">
        <span className="mr-1 text-[11px] font-semibold uppercase tracking-wide text-ink-600">
          {tString("languagesPopover.currentlyEditing")}
        </span>
        {businessLanguages.map((bl) => {
          const lang = supportedLanguages.find(
            (l) => l.code === bl.language_code,
          );
          if (!lang) return null;
          const isActive = currentViewLanguage === bl.language_code;
          return (
            <button
              key={bl.language_code}
              type="button"
              onClick={() => {
                setCurrentViewLanguage(bl.language_code);
              }}
              className={`inline-flex h-7 items-center rounded-full border px-3 text-xs font-semibold transition-colors ${
                isActive
                  ? "bg-brand text-white border-brand shadow-sm"
                  : "border-warm-200 bg-warm-50 text-ink-700 hover:border-brand/30 hover:bg-brand/5"
              }`}
            >
              {lang.native_name}
              {bl.is_default && (
                <span
                  className={`ml-1.5 text-[10px] ${isActive ? "text-white/80" : "text-ink-500"}`}
                >
                  · {tString("languages.defaultSuffix").toLowerCase()}
                </span>
              )}
            </button>
          );
        })}

        <div className="ml-auto flex items-center gap-2">
          {currentViewLanguage !== defaultLanguage && !syncBusy && (
            <button
              type="button"
              onClick={() =>
                currentViewLanguage && handleTranslateMenu(currentViewLanguage)
              }
              disabled={isTranslating}
              className="inline-flex h-7 items-center gap-1.5 rounded-full border border-warm-200 bg-white px-3 text-xs font-semibold text-ink-700 transition-colors hover:border-brand/30 hover:bg-brand/5 disabled:opacity-50"
            >
              <Languages className="w-3.5 h-3.5" />
              {isTranslating
                ? tString("languages.translating")
                : `${tString("languages.translateTo")} ${targetLangNative ?? ""}`}
            </button>
          )}
          <Tooltip
            content={tString("languages.syncTranslationsTooltip")}
            placement="bottom"
            delay={400}
          >
            <button
              type="button"
              onClick={handleSyncAllTranslations}
              disabled={isTranslating}
              aria-label={tString("languages.syncTranslationsAria")}
              className="inline-flex items-center gap-1.5 px-3 h-7 text-xs rounded-full font-semibold border border-brand/30 bg-brand/10 text-brand-dark hover:bg-brand/15 hover:border-brand/50 transition-colors disabled:opacity-60 disabled:cursor-wait"
            >
              <RefreshCw
                className={`w-3.5 h-3.5 ${syncBusy ? "animate-spin" : ""}`}
              />
              {syncLabel}
            </button>
          </Tooltip>
          {syncBusy && (
            <button
              type="button"
              onClick={cancelSyncAllTranslations}
              aria-label={tString("languages.syncCancelAria")}
              className="inline-flex h-7 w-7 items-center justify-center rounded-full border border-warm-200 bg-white text-ink-600 transition-colors hover:border-red-300 hover:bg-red-50 hover:text-red-600"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          )}
        </div>
      </div>

      {/* Real progress bar driven by the async translate job's done/total. */}
      {syncBusy && (
        <div
          className="h-1.5 w-full overflow-hidden rounded-full bg-warm-100"
          role="progressbar"
          aria-valuenow={pct}
          aria-valuemin={0}
          aria-valuemax={100}
        >
          <div
            className="h-full rounded-full bg-brand transition-[width] duration-300"
            style={{ width: `${pct}%` }}
          />
        </div>
      )}

      {syncError && !syncBusy && (
        <div className="flex items-center gap-1.5 text-xs font-medium text-red-600">
          <AlertTriangle className="h-3.5 w-3.5" />
          {syncError}
        </div>
      )}
    </div>
  );
}
