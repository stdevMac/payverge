import { useState, useEffect, useCallback, useRef } from "react";
import { businessApi, MenuCategory, Business } from "../../../../api/business";
import { parseMenuCategories } from "@/utils/businessDataParsers";
import {
  getSupportedLanguages,
  getBusinessLanguages,
  updateBusinessLanguages,
  translateEntireMenu,
  getTranslationStatus,
  SupportedLanguage,
  BusinessLanguage,
} from "../../../../api/currency";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";
import type { Orderability } from "@/api/orders";
import { buildBusinessLanguagesPayload } from "@/utils/businessLanguages";
import {
  readLastGoodOperatorMenu,
  rememberLastGoodOperatorMenu,
} from "./lastGoodOperatorMenu";

export function useMenuData(businessId: number) {
  const { locale } = useSimpleLocale();
  const [currentLocale, setCurrentLocale] = useState(locale);

  useEffect(() => {
    setCurrentLocale(locale);
  }, [locale]);

  const tString = useCallback(
    (key: string, params?: Record<string, string | number>): string => {
      const fullKey = `businessDashboard.dashboard.menuBuilder.${key}`;
      const result = getTranslation(fullKey, currentLocale, params);
      return Array.isArray(result) ? result[0] || key : (result as string);
    },
    [currentLocale],
  );

  const cachedMenu = readLastGoodOperatorMenu(businessId);
  const [menu, setMenu] = useState<MenuCategory[]>(() => cachedMenu?.menu ?? []);
  const [menuVersion, setMenuVersion] = useState<number>(
    () => cachedMenu?.version ?? 0,
  );
  const [itemOrderability, setItemOrderability] = useState<
    Record<string, Orderability>
  >(() => cachedMenu?.orderability ?? {});
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Distinct from mutation `error` (toasted + cleared). A catalog fetch flake
  // must keep last-good or show retry — never the empty-menu CTA (#773).
  const [loadFailed, setLoadFailed] = useState(false);

  // Business state
  const [business, setBusiness] = useState<Business | null>(null);
  const [defaultCurrency, setDefaultCurrency] = useState("USD");

  // Language states
  const [supportedLanguages, setSupportedLanguages] = useState<
    SupportedLanguage[]
  >([]);
  const [businessLanguages, setBusinessLanguages] = useState<
    BusinessLanguage[]
  >([]);
  const [selectedLanguages, setSelectedLanguages] = useState<string[]>([]);
  const [defaultLanguage, setDefaultLanguage] = useState<string>("");
  const [isLanguageLoading, setIsLanguageLoading] = useState(false);
  const [currentViewLanguage, setCurrentViewLanguage] = useState<string>("");
  const [languagesLoaded, setLanguagesLoaded] = useState(false);
  const [isTranslating, setIsTranslating] = useState(false);
  // Tracks async-job progress so the button can show a real "Syncing 240/1200…"
  // bar (progress = strings processed, total = strings to process). Null when
  // no sync is in flight.
  const [syncProgress, setSyncProgress] = useState<{
    done: number;
    total: number;
  } | null>(null);
  // Partial-failure / error message from the async translate job, surfaced on
  // the sync control. Null when the last sync succeeded or none has run.
  const [syncError, setSyncError] = useState<string | null>(null);
  // Client-side cancel flag. The batch translate runs server-side in a
  // background job with no cancel endpoint; cancelling stops the UI from
  // polling/applying (the idempotent job finishes server-side harmlessly, and
  // the unchanged-string skip makes a later re-run cheap).
  const cancelSyncRef = useRef(false);
  const loadMenuRequestIdRef = useRef(0);

  const loadBusiness = useCallback(async () => {
    try {
      const businessData = await businessApi.getBusiness(businessId);
      setBusiness(businessData);
      setDefaultCurrency(businessData.default_currency || "USD");
    } catch (error) {
      console.error("Failed to load business data:", error);
    }
  }, [businessId]);

  const loadLanguages = useCallback(async () => {
    try {
      setIsLanguageLoading(true);
      const supported = await getSupportedLanguages();
      setSupportedLanguages(supported);

      const businessLangs = await getBusinessLanguages(businessId);
      setBusinessLanguages(businessLangs);

      const selectedCodes = businessLangs.map((bl) => bl.language_code);
      setSelectedLanguages(selectedCodes);

      const defaultLang = businessLangs.find((bl) => bl.is_default);
      if (defaultLang) {
        setDefaultLanguage(defaultLang.language_code);
        setCurrentViewLanguage(defaultLang.language_code);
      } else {
        setDefaultLanguage("en");
        setCurrentViewLanguage("en");
      }
      setLanguagesLoaded(true);
    } catch (error) {
      console.error("Failed to load languages:", error);
      setDefaultLanguage("en");
      setCurrentViewLanguage("en");
      setLanguagesLoaded(true);
    } finally {
      setIsLanguageLoading(false);
    }
  }, [businessId]);

  const loadMenu = useCallback(
    async (language?: string) => {
      const requestId = ++loadMenuRequestIdRef.current;
      try {
        // Keep last-good visible during a refresh so a flake cannot flash the
        // empty-catalog CTA. Only skeleton when we have nothing to show.
        const lastGood = readLastGoodOperatorMenu(businessId);
        setIsLoading(!(lastGood && lastGood.menu.length > 0));
        setError(null);
        setLoadFailed(false);
        const menuData = await businessApi.getMenu(businessId, language);
        if (requestId !== loadMenuRequestIdRef.current) {
          return;
        }
        if (menuData?.language && language && menuData.language !== language) {
          return;
        }

        if (menuData) {
          // Track menu version for optimistic concurrency control
          if (menuData.version !== undefined) {
            setMenuVersion(menuData.version);
          }

          const nextMenu = parseMenuCategories(menuData);
          const nextOrderability = menuData.item_orderability ?? {};
          setMenu(nextMenu);
          setItemOrderability(nextOrderability);
          rememberLastGoodOperatorMenu(businessId, {
            menu: nextMenu,
            version: menuData.version ?? 0,
            orderability: nextOrderability,
          });
        } else {
          setMenu([]);
          setItemOrderability({});
        }
      } catch (error) {
        if (requestId !== loadMenuRequestIdRef.current) {
          return;
        }
        console.error("Failed to load menu:", error);
        setLoadFailed(true);
        const lastGood = readLastGoodOperatorMenu(businessId);
        if (lastGood && lastGood.menu.length > 0) {
          setMenu(lastGood.menu);
          setMenuVersion(lastGood.version);
          setItemOrderability(lastGood.orderability);
        }
        // Do not wipe the catalog and do not set mutation `error` — that path
        // toasts and then MenuList would render "No categories yet" (#773).
      } finally {
        if (requestId === loadMenuRequestIdRef.current) {
          setIsLoading(false);
        }
      }
    },
    [businessId],
  );

  /**
   * Run a batch translation into the given target languages via the async job
   * endpoint (TranslateEntireMenu) and poll its status for real progress. This
   * replaces the old synchronous per-string in-request Google loop that made
   * thousands of external calls in one HTTP request and died at menu scale
   * (§3.7 fix 1). Reports { done, total } from the job so the UI shows a real
   * progress bar, supports cancel (client-side), and surfaces partial failures.
   *
   * Returns true on completion, false on failure/cancel.
   */
  const runTranslationJob = useCallback(
    async (targets: string[], viewLanguageAfter?: string): Promise<boolean> => {
      if (targets.length === 0) return true;

      cancelSyncRef.current = false;
      setIsTranslating(true);
      setError(null);
      setSyncError(null);
      setSyncProgress({ done: 0, total: 0 });

      try {
        const { job_id } = await translateEntireMenu(businessId, targets);

        // Poll the job status until it terminates or the user cancels.
        // ~1.2s cadence keeps the bar lively without hammering the server.
        for (;;) {
          if (cancelSyncRef.current) {
            setSyncError(tString("languages.syncCancelled"));
            return false;
          }
          const status = await getTranslationStatus(job_id);
          setSyncProgress({
            done: status.progress ?? 0,
            total: status.total ?? 0,
          });

          if (status.status === "completed") {
            break;
          }
          if (status.status === "failed") {
            setSyncError(status.message || tString("languages.syncError"));
            return false;
          }
          await new Promise((r) => setTimeout(r, 1200));
        }

        // Refresh the visible menu if the active view was just translated.
        if (
          viewLanguageAfter &&
          viewLanguageAfter !== defaultLanguage &&
          targets.includes(viewLanguageAfter)
        ) {
          setCurrentViewLanguage(viewLanguageAfter);
          await loadMenu(viewLanguageAfter);
        } else if (
          currentViewLanguage &&
          currentViewLanguage !== defaultLanguage &&
          targets.includes(currentViewLanguage)
        ) {
          await loadMenu(currentViewLanguage);
        }
        return true;
      } catch (error) {
        console.error("Failed to run translation job:", error);
        setSyncError(tString("languages.syncError"));
        return false;
      } finally {
        setIsTranslating(false);
        setSyncProgress(null);
        cancelSyncRef.current = false;
      }
    },
    [businessId, currentViewLanguage, defaultLanguage, loadMenu, tString],
  );

  // Per-language sync: translate the menu into one target language via the job.
  const handleTranslateMenu = useCallback(
    async (languageCode: string) => {
      await runTranslationJob([languageCode], languageCode);
    },
    [runTranslationJob],
  );

  /**
   * Translate the default-language menu into every other configured language
   * in one click, via a single async job (all targets at once) with real
   * progress polling.
   */
  const handleSyncAllTranslations = useCallback(async () => {
    const targets = businessLanguages
      .filter((bl) => !bl.is_default && bl.language_code)
      .map((bl) => bl.language_code);
    await runTranslationJob(targets);
  }, [businessLanguages, runTranslationJob]);

  // Cancel an in-flight sync (client-side): stops polling/applying. The
  // background job finishes server-side harmlessly.
  const cancelSyncAllTranslations = useCallback(() => {
    cancelSyncRef.current = true;
  }, []);

  const handleLanguageUpdate = useCallback(
    async (languageCodes: string[], defaultCode: string) => {
      try {
        setIsLanguageLoading(true);
        await updateBusinessLanguages(
          businessId,
          buildBusinessLanguagesPayload({
            selected: languageCodes,
            defaultLanguage: defaultCode,
          }),
        );
        await loadLanguages();
      } catch (error) {
        console.error("Failed to update languages:", error);
        setError(tString("languages.saveError"));
      } finally {
        setIsLanguageLoading(false);
      }
    },
    [businessId, loadLanguages, tString],
  );

  useEffect(() => {
    loadBusiness().catch((err) => console.error("loadBusiness failed:", err));
    loadLanguages().catch((err) => console.error("loadLanguages failed:", err));
  }, [loadBusiness, loadLanguages]);

  useEffect(() => {
    if (languagesLoaded && defaultLanguage) {
      loadMenu(currentViewLanguage || defaultLanguage).catch((err) =>
        console.error("loadMenu failed:", err),
      );
    }
  }, [loadMenu, languagesLoaded, defaultLanguage, currentViewLanguage]);

  return {
    menu,
    setMenu,
    itemOrderability,
    menuVersion,
    setMenuVersion,
    isLoading,
    error,
    setError,
    loadFailed,
    business,
    defaultCurrency,
    loadMenu,
    tString,
    // Language stuff
    supportedLanguages,
    businessLanguages,
    selectedLanguages,
    setSelectedLanguages,
    defaultLanguage,
    setDefaultLanguage,
    isLanguageLoading,
    currentViewLanguage,
    setCurrentViewLanguage,
    languagesLoaded,
    handleTranslateMenu,
    handleSyncAllTranslations,
    cancelSyncAllTranslations,
    handleLanguageUpdate,
    isTranslating,
    syncProgress,
    syncError,
  };
}
