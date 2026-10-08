"use client";

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import {
  Button,
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from "@nextui-org/react";
import Link from "next/link";
import QRCode from "qrcode";

import type { Business, MenuCategory } from "@/api/business";
import { getMenu } from "@/api/business";
import type { BusinessLanguage, SupportedLanguage } from "@/api/currency";
import {
  auditMenuForPrint,
  type ImageMeasurement,
} from "@/lib/menuPrint/audit";
import { getMenuDesignFamily } from "@/lib/menuPrint/families";
import { selectDefaultPrintImages } from "@/lib/menuPrint/directionDefaults";
import { normalizeMenuForPrint } from "@/lib/menuPrint/normalize";
import { resolvePrintPalette } from "@/lib/menuPrint/palette";
import { planMenuDocument } from "@/lib/menuPrint/planner/planDocument";
import { recommendMenuDirections } from "@/lib/menuPrint/recommend";
import { renderMenuPrintHtml } from "@/lib/menuPrint/renderHtml";
import type {
  CoverMode,
  LogoTreatment,
  MenuArtDirection,
  MenuDesignFamilyId,
  MenuOutputFormat,
  MenuPrintDiagnostic,
  MenuPrintOptions,
  OrnamentIntensity,
  PaperFormat,
  MenuTreatment,
} from "@/lib/menuPrint/types";
import { parseMenuCategories } from "@/utils/businessDataParsers";
import { getBusinessPageEditorPath } from "@/utils/businessUrl";
import {
  btnPrimaryNextUI,
  btnSecondaryNextUI,
} from "@/components/ui/buttonStyles";
import { PrintDiagnostics } from "./PrintDiagnostics";
import { PrintFineTunePanel } from "./PrintFineTunePanel";
import { PrintIdentityControls } from "./PrintIdentityControls";
import { PrintLookPicker } from "./PrintLookPicker";
import { PrintMenuPreview } from "./PrintMenuPreview";
import { PrintPhotoControls } from "./PrintPhotoControls";
import { PrintShapeControls } from "./PrintShapeControls";
import {
  PRINT_MENU_STAGES,
  useMenuPrintController,
  type PrintMenuStage,
} from "./useMenuPrintController";
import { useImageMeasurements } from "./useImageMeasurements";
import {
  useMenuPrintWindow,
  type MenuPrintWindowStatus,
} from "./useMenuPrintWindow";

export interface PrintMenuStudioProps {
  isOpen: boolean;
  onClose: () => void;
  business: Business;
  categories: MenuCategory[];
  defaultCurrency: string;
  languages: BusinessLanguage[];
  supportedLanguages?: SupportedLanguage[];
  currentViewLanguage: string;
  defaultLanguage: string;
  businessId: number;
  tString: (key: string) => string;
}

/**
 * Diagnostics that point at settings live behind the fine-tune surface; page
 * diagnostics belong to the live preview. Nothing else needs a destination.
 */
type FocusSurface = "fine-tune" | "preview";

interface PendingFocus {
  surface: FocusSurface;
  kind: MenuPrintDiagnostic["target"]["kind"];
  id: string;
}

const FORMAT_KEY_PARTS: Record<MenuOutputFormat, string> = {
  "single-sheet": "singleSheet",
  "two-page-spread": "twoPageSpread",
  "folded-booklet": "foldedBooklet",
  "takeaway-trifold": "takeawayTrifold",
  "drinks-card": "drinksCard",
  "counter-menu": "counterMenu",
};

const TREATMENT_KEY_PARTS: Record<MenuTreatment, string> = {
  "type-led": "typeLed",
  balanced: "balanced",
  "photo-led": "photoLed",
  compact: "compact",
};

function isUsCountry(country: string | undefined | null): boolean {
  if (!country) return true;
  const normalized = country.trim().toLowerCase();
  return (
    normalized === "us" ||
    normalized === "usa" ||
    normalized === "united states"
  );
}

function defaultPaperForBusiness(business: Business): PaperFormat {
  return isUsCountry(business.address?.country) ? "letter" : "a4";
}

function formatBusinessAddress(
  address: Business["address"],
): string | undefined {
  if (!address) return undefined;
  const value = [
    address.street,
    address.city,
    address.state,
    address.postal_code,
    address.country,
  ]
    .map((part) => (typeof part === "string" ? part.trim() : ""))
    .filter(Boolean)
    .join(", ");
  return value || undefined;
}

function surfaceForDiagnostic(
  target: MenuPrintDiagnostic["target"],
): FocusSurface {
  return target.kind === "page" ? "preview" : "fine-tune";
}

function stageIndex(stage: PrintMenuStage): number {
  return PRINT_MENU_STAGES.indexOf(stage);
}

export function PrintMenuStudio({
  isOpen,
  onClose,
  business,
  categories,
  defaultCurrency: _defaultCurrency,
  languages,
  supportedLanguages = [],
  currentViewLanguage,
  defaultLanguage,
  businessId,
  tString,
}: PrintMenuStudioProps) {
  const initialLanguage = currentViewLanguage || defaultLanguage || "en";
  const [language, setLanguage] = useState(initialLanguage);
  const [fetchedCategories, setFetchedCategories] = useState<
    MenuCategory[] | null
  >(null);
  const [lastValidCategories, setLastValidCategories] =
    useState<MenuCategory[]>(categories);
  const [isLoadingLanguage, setIsLoadingLanguage] = useState(false);
  const [languageLoadFailed, setLanguageLoadFailed] = useState(false);
  const [languageLoadAttempt, setLanguageLoadAttempt] = useState(0);
  const [includeQr, setIncludeQr] = useState(false);
  const [qrDataUrl, setQrDataUrl] = useState<string>();
  const [logoTreatment, setLogoTreatment] =
    useState<LogoTreatment>("contained");
  const [coverMode, setCoverMode] = useState<CoverMode>("none");
  const [ornamentIntensity, setOrnamentIntensity] =
    useState<OrnamentIntensity>("restrained");
  const [selectedPage, setSelectedPage] = useState(0);
  const [printing, setPrinting] = useState(false);
  const [printStatus, setPrintStatus] = useState<MenuPrintWindowStatus>();
  const [pendingFocus, setPendingFocus] = useState<PendingFocus>();
  const [fineTuneOpen, setFineTuneOpen] = useState(false);
  const stagePanelRef = useRef<HTMLDivElement>(null);
  const stageScrollRef = useRef<HTMLDivElement>(null);
  const fineTunePanelRef = useRef<HTMLDivElement>(null);
  const wasOpen = useRef(false);
  const defaultsAppliedForOpen = useRef(false);
  const languageImageDefaultsPending = useRef(false);
  const { print, cancel } = useMenuPrintWindow();
  const handleClose = useCallback(() => {
    cancel();
    setPrinting(false);
    onClose();
  }, [cancel, onClose]);

  const languageOptions = useMemo(() => {
    const codes = new Set<string>();
    if (defaultLanguage) codes.add(defaultLanguage);
    languages.forEach(({ language_code }) => {
      if (language_code) codes.add(language_code);
    });
    if (currentViewLanguage) codes.add(currentViewLanguage);
    return [...codes].map((value) => {
      const supported = supportedLanguages.find(({ code }) => code === value);
      return {
        value,
        label: supported?.native_name || supported?.name || value.toUpperCase(),
      };
    });
  }, [currentViewLanguage, defaultLanguage, languages, supportedLanguages]);

  useEffect(() => {
    if (!isOpen) return;
    const viewLanguage = currentViewLanguage || defaultLanguage || "en";
    if (language === viewLanguage) {
      setFetchedCategories(null);
      setLastValidCategories(categories);
      setIsLoadingLanguage(false);
      setLanguageLoadFailed(false);
      return;
    }
    let cancelled = false;
    setIsLoadingLanguage(true);
    setLanguageLoadFailed(false);
    getMenu(businessId, language)
      .then((menuData) => {
        if (!cancelled) {
          const translatedCategories = parseMenuCategories(menuData);
          setFetchedCategories(translatedCategories);
          setLastValidCategories(translatedCategories);
        }
      })
      .catch(() => {
        if (!cancelled) setLanguageLoadFailed(true);
      })
      .finally(() => {
        if (!cancelled) setIsLoadingLanguage(false);
      });
    return () => {
      cancelled = true;
    };
  }, [
    businessId,
    categories,
    currentViewLanguage,
    defaultLanguage,
    isOpen,
    language,
    languageLoadAttempt,
  ]);

  useEffect(() => {
    if (!isOpen || !includeQr || !business.custom_url) {
      setQrDataUrl(undefined);
      return;
    }
    let cancelled = false;
    const origin = typeof window === "undefined" ? "" : window.location.origin;
    const qrUrl = `${origin}/business/${business.custom_url}`;
    /* eslint-disable no-restricted-syntax -- qrcode library accepts print colors as hex */
    const dark = business.default_qr_foreground_color || "#000000";
    const light = business.default_qr_background_color || "#FFFFFF";
    /* eslint-enable no-restricted-syntax */
    QRCode.toDataURL(qrUrl, {
      width: 256,
      margin: 1,
      color: { dark, light },
    })
      .then((url) => {
        if (!cancelled) setQrDataUrl(url);
      })
      .catch(() => {
        if (!cancelled) setQrDataUrl(undefined);
      });
    return () => {
      cancelled = true;
    };
  }, [
    business.custom_url,
    business.default_qr_background_color,
    business.default_qr_foreground_color,
    includeQr,
    isOpen,
  ]);

  const categoriesForLanguage = useMemo(() => {
    const viewLanguage = currentViewLanguage || defaultLanguage || "en";
    return language === viewLanguage
      ? categories
      : (fetchedCategories ?? lastValidCategories);
  }, [
    categories,
    currentViewLanguage,
    defaultLanguage,
    fetchedCategories,
    language,
    lastValidCategories,
  ]);

  const businessInput = useMemo(
    () => ({
      name: business.name,
      logoUrl: business.logo || undefined,
      address: formatBusinessAddress(business.address),
      customUrl: business.custom_url,
      businessType: business.business_type,
      primaryColor: business.design_settings?.primary_color,
      secondaryColor: business.design_settings?.secondary_color,
    }),
    [business],
  );
  const baseOptions = useMemo<MenuPrintOptions>(
    () => ({
      paperFormat: defaultPaperForBusiness(business),
      language,
      showDescriptions: true,
      showImages: true,
      showTags: true,
      currencySymbol: true,
      includeQr,
      selectedCategoryIds: "all",
    }),
    [business, includeQr, language],
  );
  const sourceModel = useMemo(
    () =>
      normalizeMenuForPrint(categoriesForLanguage, businessInput, baseOptions),
    [baseOptions, businessInput, categoriesForLanguage],
  );
  const imageUrls = useMemo(
    () =>
      sourceModel.sections.flatMap((section) =>
        section.items.flatMap((item) => item.imageCandidates ?? []),
      ),
    [sourceModel],
  );
  const imageMeasurements = useImageMeasurements(isOpen ? imageUrls : []);
  const auditMeasurements = useMemo<Record<string, ImageMeasurement>>(
    () =>
      Object.fromEntries(
        Object.entries(imageMeasurements.measurements).flatMap(
          ([url, measurement]) =>
            measurement.status === "loading"
              ? []
              : [[url, { ...measurement, url }] as const],
        ),
      ),
    [imageMeasurements.measurements],
  );
  const sourceAudit = useMemo(
    () => auditMenuForPrint(sourceModel, auditMeasurements),
    [auditMeasurements, sourceModel],
  );
  const recommendations = useMemo(
    () =>
      recommendMenuDirections({
        audit: sourceAudit,
        businessType: sourceModel.business.businessType,
      }),
    [sourceAudit, sourceModel.business.businessType],
  );
  const controller = useMenuPrintController({
    recommendations,
    defaultPaper: defaultPaperForBusiness(business),
  });

  useEffect(() => {
    if (
      !isOpen ||
      !wasOpen.current ||
      defaultsAppliedForOpen.current ||
      controller.state.stage !== "look" ||
      imageMeasurements.status !== "complete"
    ) {
      return;
    }
    defaultsAppliedForOpen.current = true;
    const selectedFamily = getMenuDesignFamily(
      controller.state.direction.familyId,
    );
    const palette = resolvePrintPalette({
      familyId: selectedFamily.id,
      primaryColor: sourceModel.business.primaryColor,
      secondaryColor: sourceModel.business.secondaryColor,
    });
    const currentPalette = controller.state.direction.palette;
    if (
      palette.ground !== currentPalette.ground ||
      palette.ink !== currentPalette.ink ||
      palette.accent !== currentPalette.accent ||
      palette.muted !== currentPalette.muted ||
      palette.source !== currentPalette.source
    ) {
      controller.actions.setPalette(palette);
    }
    if (controller.state.direction.imageSelections.length === 0) {
      const selections = selectDefaultPrintImages({
        family: selectedFamily,
        model: sourceModel,
        audit: sourceAudit,
        treatment: controller.state.direction.treatment,
      });
      if (selections.length) controller.actions.setImages(selections);
    }
  }, [
    controller.actions,
    controller.state.direction.familyId,
    controller.state.direction.imageSelections.length,
    controller.state.direction.palette,
    controller.state.direction.treatment,
    controller.state.stage,
    imageMeasurements.status,
    isOpen,
    sourceAudit,
    sourceModel,
  ]);

  useEffect(() => {
    if (isOpen && !wasOpen.current) {
      defaultsAppliedForOpen.current = false;
      controller.actions.reset();
      setLanguage(initialLanguage);
      setFetchedCategories(null);
      setLastValidCategories(categories);
      setLanguageLoadFailed(false);
      setLanguageLoadAttempt(0);
      setIncludeQr(false);
      setLogoTreatment("contained");
      setCoverMode("none");
      setOrnamentIntensity("restrained");
      setSelectedPage(0);
      setPrinting(false);
      setPrintStatus(undefined);
      setPendingFocus(undefined);
      setFineTuneOpen(false);
      languageImageDefaultsPending.current = false;
    }
    if (!isOpen) {
      defaultsAppliedForOpen.current = false;
      languageImageDefaultsPending.current = false;
    }
    wasOpen.current = isOpen;
  }, [categories, controller.actions, initialLanguage, isOpen]);

  const normalizedOptions = useMemo<MenuPrintOptions>(
    () => ({
      ...baseOptions,
      paperFormat: controller.state.direction.paperFormat,
      selectedCategoryIds: controller.state.selectedCategoryIds,
    }),
    [
      baseOptions,
      controller.state.direction.paperFormat,
      controller.state.selectedCategoryIds,
    ],
  );
  const model = useMemo(
    () =>
      normalizeMenuForPrint(
        categoriesForLanguage,
        businessInput,
        normalizedOptions,
      ),
    [businessInput, categoriesForLanguage, normalizedOptions],
  );
  const audit = useMemo(
    () => auditMenuForPrint(model, auditMeasurements),
    [auditMeasurements, model],
  );
  const validImageSelections = useMemo(() => {
    const validSelectionKeys = new Set<string>();
    model.sections.forEach((section) => {
      section.items.forEach((item) => {
        const urls = new Set([
          ...(item.imageCandidates ?? []),
          ...(item.imageUrl ? [item.imageUrl] : []),
        ]);
        urls.forEach((url) => validSelectionKeys.add(`${item.id}\u0000${url}`));
      });
    });
    return controller.state.direction.imageSelections.filter((selection) =>
      validSelectionKeys.has(`${selection.itemId}\u0000${selection.url}`),
    );
  }, [controller.state.direction.imageSelections, model.sections]);
  const direction = useMemo<MenuArtDirection>(
    () => ({
      ...controller.state.direction,
      imageSelections: validImageSelections,
      logoTreatment,
      coverMode,
      ornamentIntensity,
    }),
    [
      controller.state.direction,
      coverMode,
      logoTreatment,
      ornamentIntensity,
      validImageSelections,
    ],
  );
  useEffect(() => {
    if (
      validImageSelections.length !==
      controller.state.direction.imageSelections.length
    ) {
      controller.actions.setImages(validImageSelections);
    }
  }, [
    controller.actions,
    controller.state.direction.imageSelections.length,
    validImageSelections,
  ]);
  useEffect(() => {
    if (
      !isOpen ||
      !languageImageDefaultsPending.current ||
      isLoadingLanguage ||
      languageLoadFailed ||
      imageMeasurements.status !== "complete"
    ) {
      return;
    }
    languageImageDefaultsPending.current = false;
    const family = getMenuDesignFamily(controller.state.direction.familyId);
    controller.actions.setImages(
      selectDefaultPrintImages({
        family,
        model,
        audit,
        treatment: controller.state.direction.treatment,
      }),
    );
  }, [
    audit,
    controller.actions,
    controller.state.direction.familyId,
    controller.state.direction.treatment,
    imageMeasurements.status,
    isLoadingLanguage,
    isOpen,
    languageLoadFailed,
    model,
  ]);
  const plannedDocument = useMemo(
    () => planMenuDocument({ model, audit, direction }),
    [audit, direction, model],
  );
  const origin = typeof window === "undefined" ? "" : window.location.origin;
  const html = useMemo(
    () =>
      renderMenuPrintHtml(model, plannedDocument, {
        origin,
        qrDataUrl: includeQr ? qrDataUrl : undefined,
        qrCaption: tString("print.qr.caption"),
      }),
    [includeQr, model, origin, plannedDocument, qrDataUrl, tString],
  );
  const family = getMenuDesignFamily(direction.familyId);

  const pageEstimates = useMemo<
    Partial<Record<MenuOutputFormat, number>>
  >(() => {
    const estimates: Partial<Record<MenuOutputFormat, number>> = {};
    family.supportedFormats.forEach((outputFormat) => {
      try {
        estimates[outputFormat] = planMenuDocument({
          model,
          audit,
          direction: { ...direction, outputFormat },
        }).pages.length;
      } catch {
        // Unsupported combinations are already described by the family controls.
      }
    });
    return estimates;
  }, [audit, direction, family.supportedFormats, model]);

  const goToStage = useCallback(
    (stage: PrintMenuStage) => {
      controller.actions.goTo(stage);
      if (stageScrollRef.current) stageScrollRef.current.scrollTop = 0;
      setPrintStatus(undefined);
    },
    [controller.actions],
  );
  const handleNext = () => {
    const next = PRINT_MENU_STAGES[stageIndex(controller.state.stage) + 1];
    if (next) goToStage(next);
  };
  const handleBack = () => {
    const previous = PRINT_MENU_STAGES[stageIndex(controller.state.stage) - 1];
    if (previous) goToStage(previous);
  };
  const handleLanguageChange = (nextLanguage: string) => {
    languageImageDefaultsPending.current = true;
    controller.actions.setImages([]);
    setLanguageLoadFailed(false);
    setIsLoadingLanguage(true);
    setLanguage(nextLanguage);
  };
  const retryLanguageLoad = () => {
    setLanguageLoadAttempt((attempt) => attempt + 1);
  };
  const handleLookSelect = useCallback(
    (
      familyId: MenuDesignFamilyId,
      treatment: MenuTreatment,
      outputFormat: MenuOutputFormat,
    ) => {
      const selectedFamily = getMenuDesignFamily(familyId);
      // Format must be applied after the family switch: `select-family`
      // preserves the current format only when the new family supports it,
      // so a later `select-format` dispatch is what actually lands the
      // recommendation's own format instead of silently falling back to the
      // family's first supported format.
      controller.actions.selectFamily(familyId);
      controller.actions.selectTreatment(treatment);
      controller.actions.selectFormat(outputFormat);
      controller.actions.setPalette(
        resolvePrintPalette({
          familyId,
          primaryColor: model.business.primaryColor,
          secondaryColor: model.business.secondaryColor,
        }),
      );
      controller.actions.setImages(
        selectDefaultPrintImages({
          family: selectedFamily,
          model,
          audit,
          treatment,
        }),
      );
    },
    [audit, controller.actions, model],
  );

  const handleDiagnosticNavigate = useCallback(
    (target: MenuPrintDiagnostic["target"]) => {
      const surface = surfaceForDiagnostic(target);
      if (target.kind === "page") {
        const parsed = Number.parseInt(target.id, 10);
        setSelectedPage(Number.isFinite(parsed) ? Math.max(0, parsed) : 0);
      }
      setFineTuneOpen(surface === "fine-tune");
      setPendingFocus({ surface, kind: target.kind, id: target.id });
      goToStage("personalize");
    },
    [goToStage],
  );

  useEffect(() => {
    if (!pendingFocus || controller.state.stage !== "personalize") return;
    const timer = window.setTimeout(() => {
      const panel =
        pendingFocus.surface === "fine-tune"
          ? fineTunePanelRef.current
          : stagePanelRef.current;
      if (!panel) return;
      let focusTarget: HTMLElement | undefined;
      if (pendingFocus.kind === "image") {
        const itemName = model.sections
          .flatMap((section) => section.items)
          .find((item) =>
            (item.imageCandidates ?? []).includes(pendingFocus.id),
          )?.name;
        focusTarget = [...panel.querySelectorAll<HTMLElement>("h4")].find(
          (element) => element.textContent?.trim() === itemName,
        );
      } else if (pendingFocus.kind === "category") {
        const categoryName = model.sections.find(
          ({ id }) => id === pendingFocus.id,
        )?.name;
        focusTarget =
          [...panel.querySelectorAll<HTMLElement>("label")]
            .find((element) => element.textContent?.trim() === categoryName)
            ?.querySelector<HTMLElement>("input") ?? undefined;
      } else if (pendingFocus.kind === "setting") {
        focusTarget =
          panel.querySelector<HTMLElement>(
            "button:not(:disabled), input:not(:disabled)",
          ) ?? undefined;
      } else {
        focusTarget = panel.querySelector<HTMLElement>("iframe") ?? undefined;
      }
      if (focusTarget) {
        if (focusTarget.tabIndex < 0 && !focusTarget.hasAttribute("tabindex")) {
          // Non-interactive targets (photo-card headings) need a transient
          // tabindex to accept focus; drop it on blur so the fine-tune trap's
          // tab order never permanently skips a tagged element.
          focusTarget.setAttribute("tabindex", "-1");
          focusTarget.addEventListener(
            "blur",
            () => focusTarget?.removeAttribute("tabindex"),
            { once: true },
          );
        }
        focusTarget.focus();
      }
      setPendingFocus(undefined);
    }, 0);
    return () => window.clearTimeout(timer);
  }, [controller.state.stage, model.sections, pendingFocus]);

  const canPrint =
    model.sections.length > 0 &&
    plannedDocument.readiness !== "blocked" &&
    imageMeasurements.status === "complete" &&
    !isLoadingLanguage &&
    !languageLoadFailed &&
    !printing;
  const handlePrint = useCallback(async () => {
    if (!canPrint) return;
    setPrinting(true);
    setPrintStatus(undefined);
    try {
      const result = await print(html);
      if (result.status !== "complete") setPrintStatus(result.status);
    } catch {
      setPrintStatus("error");
    } finally {
      setPrinting(false);
    }
  }, [canPrint, html, print]);

  // Shared by the fine-tune drawer's QR checkbox and the always-visible
  // side-rail toggle so both controls agree on when a QR code can render.
  const canIncludeQr = Boolean(business.custom_url);

  const fineTuneSections = [
    {
      id: "layout",
      label: tString("print.fineTune.layout"),
      content: (
        <PrintShapeControls
          family={family}
          outputFormat={direction.outputFormat}
          paperFormat={direction.paperFormat}
          language={language}
          categories={model.sections}
          selectedCategoryIds={controller.state.selectedCategoryIds}
          pageEstimates={pageEstimates}
          languageChoices={languageOptions}
          tString={tString}
          onFormatChange={controller.actions.selectFormat}
          onPaperChange={controller.actions.selectPaper}
          onLanguageChange={handleLanguageChange}
          onCategoryChange={controller.actions.setCategories}
        />
      ),
    },
    {
      id: "photos",
      label: tString("print.fineTune.photos"),
      content: (
        <PrintPhotoControls
          family={family}
          treatment={direction.treatment}
          model={model}
          audit={audit}
          measurements={imageMeasurements.measurements}
          selections={direction.imageSelections}
          focalPoints={direction.imageFocalPoints}
          tString={tString}
          onTreatmentChange={controller.actions.selectTreatment}
          onSelectionChange={controller.actions.setImages}
          onFocalPointChange={controller.actions.setFocalPoint}
        />
      ),
    },
    {
      id: "identity",
      label: tString("print.fineTune.identity"),
      content: (
        <PrintIdentityControls
          family={family}
          direction={direction}
          paletteSource={direction.palette.source}
          includeQr={includeQr}
          canIncludeQr={canIncludeQr}
          tString={tString}
          onLogoTreatmentChange={setLogoTreatment}
          onPaletteChange={controller.actions.setPalette}
          onTypographyChange={controller.actions.setTypographyPersonality}
          onOrnamentChange={setOrnamentIntensity}
          onCoverChange={setCoverMode}
          onQrChange={setIncludeQr}
          onContactPlacementChange={controller.actions.setContactPlacement}
        />
      ),
    },
  ];

  const isPersonalizing = controller.state.stage === "personalize";

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      size="full"
      scrollBehavior="inside"
      classNames={{
        base: "bg-warm-50",
        body: "p-0",
        header: "border-b border-warm-200 bg-white",
        footer: "border-t border-warm-200 bg-white",
      }}
    >
      <ModalContent>
        {() => (
          <>
            <ModalHeader className="flex flex-col gap-0.5">
              <h2 className="text-lg font-semibold text-ink-950">
                {tString("print.modalTitle")}
              </h2>
              <p className="text-sm font-normal text-ink-600">
                {tString("print.modalSubtitle")}
              </p>
            </ModalHeader>
            <ModalBody>
              <div
                data-testid="print-workflow-shell"
                className={[
                  "relative grid h-full min-h-[36rem] grid-cols-1",
                  isPersonalizing
                    ? "md:grid-cols-[minmax(0,1fr)_20rem]"
                    : "md:grid-cols-1",
                ].join(" ")}
              >
                <div
                  ref={stageScrollRef}
                  data-testid="print-stage-scroll-region"
                  data-print-workflow-stage={controller.state.stage}
                  className="min-w-0 overflow-y-auto bg-white p-5 md:p-7"
                >
                  <div
                    ref={stagePanelRef}
                    data-testid="print-stage-panel"
                    className="mx-auto max-w-6xl space-y-6"
                  >
                    <div className="space-y-1 border-b border-warm-200 pb-4">
                      <p className="text-xs font-semibold uppercase tracking-wide text-brand">
                        {`${stageIndex(controller.state.stage) + 1} / ${PRINT_MENU_STAGES.length}`}
                      </p>
                      <h3
                        className="text-xl font-semibold text-ink-950"
                        aria-label={tString(
                          `print.stage.${controller.state.stage}`,
                        )}
                      >
                        {tString(`print.stage.${controller.state.stage}`)}
                      </h3>
                    </div>
                    {isLoadingLanguage ? (
                      <p
                        role="status"
                        className="rounded-lg bg-warm-100 px-4 py-3 text-sm text-ink-700"
                      >
                        {tString("print.language.loading")}
                      </p>
                    ) : null}
                    {languageLoadFailed ? (
                      <div
                        role="alert"
                        className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-rose-200 bg-rose-50 px-4 py-3"
                      >
                        <p className="text-sm text-rose-800">
                          {tString("print.language.error")}
                        </p>
                        <Button
                          size="sm"
                          className={btnSecondaryNextUI}
                          onPress={retryLanguageLoad}
                        >
                          {tString("print.language.retry")}
                        </Button>
                      </div>
                    ) : null}
                    {isPersonalizing ? (
                      <div data-testid="print-live-preview">
                        <PrintMenuPreview
                          document={plannedDocument}
                          html={html}
                          selectedPage={selectedPage}
                          tString={tString}
                          onSelectedPageChange={setSelectedPage}
                        />
                      </div>
                    ) : (
                      <PrintLookPicker
                        model={model}
                        audit={audit}
                        selectedFamilyId={direction.familyId}
                        recommendations={recommendations}
                        paperFormat={direction.paperFormat}
                        tString={tString}
                        onSelect={handleLookSelect}
                      />
                    )}
                  </div>
                </div>
                {isPersonalizing ? (
                  <aside
                    data-testid="print-side-rail"
                    className="space-y-4 border-t border-warm-200 bg-warm-50 p-4 md:overflow-y-auto md:border-l md:border-t-0"
                  >
                    <div className="space-y-1 rounded-xl border border-warm-200 bg-white p-4">
                      <p className="text-xs font-semibold uppercase tracking-wide text-ink-500">
                        {tString("print.personalize.currentLook")}
                      </p>
                      <p
                        data-testid="print-current-look"
                        className="text-base font-semibold text-ink-950"
                      >
                        {tString(family.nameKey)}
                      </p>
                      <p className="text-xs leading-relaxed text-ink-600">
                        {tString(
                          `print.treatment.${TREATMENT_KEY_PARTS[direction.treatment]}`,
                        )}
                        {" · "}
                        {tString(
                          `print.format.${FORMAT_KEY_PARTS[direction.outputFormat]}`,
                        )}
                      </p>
                    </div>
                    <div className="space-y-3 rounded-xl border border-warm-200 bg-white p-4">
                      <p className="text-xs font-semibold uppercase tracking-wide text-ink-500">
                        {tString("print.qr.label")}
                      </p>
                      <label
                        data-testid="print-side-rail-qr"
                        className={[
                          "flex min-h-11 items-center justify-between gap-4 rounded-lg border px-3 py-2 text-sm font-medium",
                          canIncludeQr
                            ? "cursor-pointer border-warm-200 bg-warm-50 text-ink-800 hover:bg-warm-100"
                            : "cursor-not-allowed border-warm-200 bg-warm-100 text-ink-500",
                        ].join(" ")}
                      >
                        <span>{tString("print.qr.include")}</span>
                        <input
                          type="checkbox"
                          aria-label={tString("print.qr.include")}
                          checked={includeQr}
                          disabled={!canIncludeQr}
                          onChange={(event) =>
                            setIncludeQr(event.target.checked)
                          }
                          className="h-4 w-4 rounded border-warm-300 text-brand focus:ring-brand/40 disabled:cursor-not-allowed"
                        />
                      </label>
                      {!canIncludeQr ? (
                        <p className="text-xs leading-relaxed text-ink-600">
                          {tString("print.qr.unavailable")}{" "}
                          <Link
                            href={getBusinessPageEditorPath(business)}
                            className="font-semibold text-brand underline underline-offset-2 hover:text-brand/80"
                          >
                            {tString("print.qr.manageCta")}
                          </Link>
                        </p>
                      ) : null}
                    </div>
                    <Button
                      className={`${btnSecondaryNextUI} w-full`}
                      onPress={() => setFineTuneOpen(true)}
                    >
                      {tString("print.fineTune.open")}
                    </Button>
                    <PrintDiagnostics
                      document={plannedDocument}
                      tString={tString}
                      onNavigate={handleDiagnosticNavigate}
                    />
                    <div className="space-y-1 text-xs leading-relaxed text-ink-600">
                      <p>{tString("print.guidance.review")}</p>
                      <p>{tString("print.guidance.savePdf")}</p>
                    </div>
                  </aside>
                ) : null}
                <PrintFineTunePanel
                  isOpen={isPersonalizing && fineTuneOpen}
                  sections={fineTuneSections}
                  tString={tString}
                  panelRef={fineTunePanelRef}
                  onClose={() => setFineTuneOpen(false)}
                />
              </div>
            </ModalBody>
            <ModalFooter className="flex flex-wrap items-center justify-between gap-3">
              <div className="min-h-6 flex-1">
                {imageMeasurements.status === "loading" ? (
                  <p role="status" className="text-sm text-ink-600">
                    {tString("print.images.measuring")}
                  </p>
                ) : printStatus ? (
                  <p
                    role="status"
                    className="text-sm font-medium text-rose-700"
                  >
                    {tString(`print.printStatus.${printStatus}`)}
                  </p>
                ) : null}
              </div>
              <div className="flex flex-wrap items-center justify-end gap-2">
                <Button className={btnSecondaryNextUI} onPress={handleClose}>
                  {tString("print.close")}
                </Button>
                {controller.canGoBack ? (
                  <Button className={btnSecondaryNextUI} onPress={handleBack}>
                    {tString("print.back")}
                  </Button>
                ) : null}
                {controller.canGoNext ? (
                  <Button
                    className={btnPrimaryNextUI}
                    onPress={handleNext}
                    isDisabled={isLoadingLanguage || languageLoadFailed}
                  >
                    {tString("print.next")}
                  </Button>
                ) : (
                  <Button
                    className={btnPrimaryNextUI}
                    onPress={handlePrint}
                    isDisabled={!canPrint}
                    isLoading={printing}
                    aria-label={tString("print.print")}
                  >
                    {tString("print.print")}
                  </Button>
                )}
              </div>
            </ModalFooter>
          </>
        )}
      </ModalContent>
    </Modal>
  );
}
