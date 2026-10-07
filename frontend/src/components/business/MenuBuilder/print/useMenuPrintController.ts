import { useMemo, useReducer, useRef } from "react";

import { getMenuDesignFamily } from "@/lib/menuPrint/families";
import {
  isPrintSafePalette,
  resolvePrintPalette,
} from "@/lib/menuPrint/palette";
import {
  MENU_DESIGN_FAMILY_IDS,
  PAPER_DIMENSIONS_MM,
  PRINT_CONTACT_PLACEMENTS,
  PRINT_TYPOGRAPHY_BY_FAMILY,
  type FeaturedImageSelection,
  type ImageFocalPoint,
  type ImageRole,
  type MenuArtDirection,
  type MenuDesignFamilyId,
  type MenuDirectionRecommendation,
  type MenuOutputFormat,
  type MenuTreatment,
  type PaperFormat,
  type PrintContactPlacement,
  type PrintTypographyPersonality,
  type RegisteredMenuDesignFamily,
  type ResolvedPrintPalette,
} from "@/lib/menuPrint/types";

/**
 * The visible flow is deliberately two moments: choose a look, then make it
 * yours (and print). Every advanced control lives behind the fine-tune surface
 * rather than adding another stage.
 */
export const PRINT_MENU_STAGES = ["look", "personalize"] as const;

export type PrintMenuStage = (typeof PRINT_MENU_STAGES)[number];

interface PrintMenuControllerDirection extends MenuArtDirection {
  reasonKey: string;
  score: number;
}

export interface PrintMenuControllerState {
  stage: PrintMenuStage;
  direction: PrintMenuControllerDirection;
  selectedCategoryIds: string[] | "all";
}

type PrintMenuControllerAction =
  | { type: "go-to"; stage: PrintMenuStage }
  | { type: "select-family"; familyId: MenuDesignFamilyId }
  | { type: "select-treatment"; treatment: MenuTreatment }
  | { type: "select-format"; outputFormat: MenuOutputFormat }
  | { type: "select-paper"; paperFormat: PaperFormat }
  | { type: "set-categories"; categoryIds: string[] | "all" }
  | { type: "set-images"; selections: FeaturedImageSelection[] }
  | { type: "set-focal-point"; url: string; point: ImageFocalPoint }
  | { type: "set-palette"; palette: ResolvedPrintPalette }
  | {
      type: "set-typography-personality";
      personality: PrintTypographyPersonality;
    }
  | { type: "set-contact-placement"; placement: PrintContactPlacement }
  | { type: "reset"; initial: PrintMenuControllerState };

export interface UseMenuPrintControllerInput {
  recommendations: readonly MenuDirectionRecommendation[];
  defaultPaper: PaperFormat;
}

export interface PrintMenuControllerActions {
  goTo: (stage: PrintMenuStage) => void;
  selectFamily: (familyId: MenuDesignFamilyId) => void;
  selectTreatment: (treatment: MenuTreatment) => void;
  selectFormat: (outputFormat: MenuOutputFormat) => void;
  selectPaper: (paperFormat: PaperFormat) => void;
  setCategories: (categoryIds: string[] | "all") => void;
  setImages: (selections: FeaturedImageSelection[]) => void;
  setFocalPoint: (url: string, point: ImageFocalPoint) => void;
  setPalette: (palette: ResolvedPrintPalette) => void;
  setTypographyPersonality: (personality: PrintTypographyPersonality) => void;
  setContactPlacement: (placement: PrintContactPlacement) => void;
  reset: () => void;
}

const FALLBACK_RECOMMENDATION: MenuDirectionRecommendation = {
  familyId: "atelier",
  treatment: "type-led",
  outputFormat: "single-sheet",
  reasonKey: "print.recommendation.versatileRestaurant",
  score: 0,
};

const IMAGE_ROLE_FALLBACKS: Readonly<Record<ImageRole, readonly ImageRole[]>> =
  {
    cover: ["cover"],
    "full-width": [
      "full-width",
      "half-page",
      "editorial-crop",
      "category-opener",
    ],
    "half-page": [
      "half-page",
      "editorial-crop",
      "full-width",
      "category-opener",
    ],
    "editorial-crop": [
      "editorial-crop",
      "half-page",
      "full-width",
      "category-opener",
    ],
    "category-opener": [
      "category-opener",
      "full-width",
      "half-page",
      "editorial-crop",
    ],
    paired: ["paired", "compact-tile"],
    "compact-tile": ["compact-tile", "paired"],
  };

function isMenuDesignFamilyId(value: unknown): value is MenuDesignFamilyId {
  return MENU_DESIGN_FAMILY_IDS.includes(value as MenuDesignFamilyId);
}

function isPaperFormat(value: unknown): value is PaperFormat {
  return (
    typeof value === "string" &&
    Object.prototype.hasOwnProperty.call(PAPER_DIMENSIONS_MM, value)
  );
}

function isPrintMenuStage(value: unknown): value is PrintMenuStage {
  return PRINT_MENU_STAGES.includes(value as PrintMenuStage);
}

function canonicalizeCategoryIds(categoryIds: readonly string[]): string[] {
  const seen = new Set<string>();
  const canonical: string[] = [];
  for (const rawId of categoryIds) {
    const id = rawId.trim();
    if (!id || seen.has(id)) continue;
    seen.add(id);
    canonical.push(id);
  }
  return canonical;
}

function resolveCompatibleImageRole(
  role: ImageRole,
  family: RegisteredMenuDesignFamily,
  allowRemap: boolean,
): ImageRole | undefined {
  if (family.photography.roles.includes(role)) return role;
  if (!allowRemap) return undefined;
  return IMAGE_ROLE_FALLBACKS[role]?.find((candidate) =>
    family.photography.roles.includes(candidate),
  );
}

function normalizeImageSelections(
  selections: readonly FeaturedImageSelection[],
  family: RegisteredMenuDesignFamily,
  allowRemap: boolean,
): FeaturedImageSelection[] {
  const itemIds = new Set<string>();
  const urls = new Set<string>();
  const normalized: FeaturedImageSelection[] = [];

  for (const selection of selections) {
    const role = resolveCompatibleImageRole(selection.role, family, allowRemap);
    if (!role || itemIds.has(selection.itemId) || urls.has(selection.url)) {
      continue;
    }
    itemIds.add(selection.itemId);
    urls.add(selection.url);
    normalized.push({ ...selection, role });
  }

  return normalized;
}

function isValidFocalPoint(point: ImageFocalPoint): boolean {
  return (
    Number.isFinite(point.x) &&
    Number.isFinite(point.y) &&
    point.x >= 0 &&
    point.x <= 1 &&
    point.y >= 0 &&
    point.y <= 1
  );
}

function isResolvedPrintPalette(
  palette: ResolvedPrintPalette,
): palette is ResolvedPrintPalette {
  return (
    (palette.source === "restaurant" || palette.source === "family-fallback") &&
    isPrintSafePalette(palette)
  );
}

function isValidRecommendation(
  recommendation: MenuDirectionRecommendation,
): boolean {
  if (!isMenuDesignFamilyId(recommendation.familyId)) return false;
  const family = getMenuDesignFamily(recommendation.familyId);
  return (
    family.supportedTreatments.includes(recommendation.treatment) &&
    family.supportedFormats.includes(recommendation.outputFormat)
  );
}

function cloneState(state: PrintMenuControllerState): PrintMenuControllerState {
  return {
    stage: state.stage,
    selectedCategoryIds:
      state.selectedCategoryIds === "all"
        ? "all"
        : [...state.selectedCategoryIds],
    direction: {
      ...state.direction,
      palette: { ...state.direction.palette },
      imageSelections: state.direction.imageSelections.map((selection) => ({
        ...selection,
      })),
      imageFocalPoints: Object.fromEntries(
        Object.entries(state.direction.imageFocalPoints).map(([url, point]) => [
          url,
          { ...point },
        ]),
      ),
    },
  };
}

function createInitialState({
  recommendations,
  defaultPaper,
}: UseMenuPrintControllerInput): PrintMenuControllerState {
  const recommendation =
    recommendations.find(isValidRecommendation) ?? FALLBACK_RECOMMENDATION;
  const family = getMenuDesignFamily(recommendation.familyId);

  return {
    stage: "look",
    selectedCategoryIds: "all",
    direction: {
      ...recommendation,
      paperFormat: isPaperFormat(defaultPaper) ? defaultPaper : "a4",
      palette: resolvePrintPalette({ familyId: family.id }),
      imageSelections: [],
      imageFocalPoints: {},
      logoTreatment: "contained",
      coverMode: "none",
      ornamentIntensity: "restrained",
      typographyPersonality: PRINT_TYPOGRAPHY_BY_FAMILY[family.id][0],
      contactPlacement: "footer",
    },
  };
}

function reducer(
  state: PrintMenuControllerState,
  action: PrintMenuControllerAction,
): PrintMenuControllerState {
  switch (action.type) {
    case "go-to":
      return !isPrintMenuStage(action.stage) || state.stage === action.stage
        ? state
        : { ...state, stage: action.stage };
    case "select-family": {
      if (!isMenuDesignFamilyId(action.familyId)) return state;
      if (action.familyId === state.direction.familyId) return state;
      const family = getMenuDesignFamily(action.familyId);
      const treatment = family.supportedTreatments.includes(
        state.direction.treatment,
      )
        ? state.direction.treatment
        : family.supportedTreatments[0];
      const outputFormat = family.supportedFormats.includes(
        state.direction.outputFormat,
      )
        ? state.direction.outputFormat
        : family.supportedFormats[0];

      return {
        ...state,
        direction: {
          ...state.direction,
          familyId: family.id,
          treatment,
          outputFormat,
          palette: resolvePrintPalette({ familyId: family.id }),
          imageSelections: normalizeImageSelections(
            state.direction.imageSelections,
            family,
            true,
          ),
          typographyPersonality:
            PRINT_TYPOGRAPHY_BY_FAMILY[family.id].find(
              (personality) =>
                personality === state.direction.typographyPersonality,
            ) ?? PRINT_TYPOGRAPHY_BY_FAMILY[family.id][0],
        },
      };
    }
    case "select-treatment": {
      const family = getMenuDesignFamily(state.direction.familyId);
      if (!family.supportedTreatments.includes(action.treatment)) return state;
      return {
        ...state,
        direction: { ...state.direction, treatment: action.treatment },
      };
    }
    case "select-format": {
      const family = getMenuDesignFamily(state.direction.familyId);
      if (!family.supportedFormats.includes(action.outputFormat)) return state;
      return {
        ...state,
        direction: { ...state.direction, outputFormat: action.outputFormat },
      };
    }
    case "select-paper":
      return isPaperFormat(action.paperFormat)
        ? {
            ...state,
            direction: {
              ...state.direction,
              paperFormat: action.paperFormat,
            },
          }
        : state;
    case "set-categories":
      return {
        ...state,
        selectedCategoryIds:
          action.categoryIds === "all"
            ? "all"
            : canonicalizeCategoryIds(action.categoryIds),
      };
    case "set-images": {
      const family = getMenuDesignFamily(state.direction.familyId);
      return {
        ...state,
        direction: {
          ...state.direction,
          imageSelections: normalizeImageSelections(
            action.selections,
            family,
            false,
          ),
        },
      };
    }
    case "set-focal-point":
      if (!isValidFocalPoint(action.point)) return state;
      return {
        ...state,
        direction: {
          ...state.direction,
          imageFocalPoints: {
            ...state.direction.imageFocalPoints,
            [action.url]: { ...action.point },
          },
        },
      };
    case "set-palette":
      if (!isResolvedPrintPalette(action.palette)) return state;
      return {
        ...state,
        direction: {
          ...state.direction,
          palette: { ...action.palette },
        },
      };
    case "set-typography-personality": {
      const personalities =
        PRINT_TYPOGRAPHY_BY_FAMILY[state.direction.familyId];
      if (!personalities.includes(action.personality)) return state;
      return action.personality === state.direction.typographyPersonality
        ? state
        : {
            ...state,
            direction: {
              ...state.direction,
              typographyPersonality: action.personality,
            },
          };
    }
    case "set-contact-placement":
      return !PRINT_CONTACT_PLACEMENTS.includes(action.placement) ||
        action.placement === state.direction.contactPlacement
        ? state
        : {
            ...state,
            direction: {
              ...state.direction,
              contactPlacement: action.placement,
            },
          };
    case "reset":
      return cloneState(action.initial);
  }
}

export function useMenuPrintController({
  recommendations,
  defaultPaper,
}: UseMenuPrintControllerInput) {
  const initialState = useMemo(
    () => createInitialState({ recommendations, defaultPaper }),
    [defaultPaper, recommendations],
  );
  const latestInitialState = useRef(initialState);
  latestInitialState.current = initialState;
  const [state, dispatch] = useReducer(reducer, initialState);
  const actions = useMemo<PrintMenuControllerActions>(
    () => ({
      goTo: (stage) => dispatch({ type: "go-to", stage }),
      selectFamily: (familyId) => dispatch({ type: "select-family", familyId }),
      selectTreatment: (treatment) =>
        dispatch({ type: "select-treatment", treatment }),
      selectFormat: (outputFormat) =>
        dispatch({ type: "select-format", outputFormat }),
      selectPaper: (paperFormat) =>
        dispatch({ type: "select-paper", paperFormat }),
      setCategories: (categoryIds) =>
        dispatch({ type: "set-categories", categoryIds }),
      setImages: (selections) => dispatch({ type: "set-images", selections }),
      setFocalPoint: (url, point) =>
        dispatch({ type: "set-focal-point", url, point }),
      setPalette: (palette) => dispatch({ type: "set-palette", palette }),
      setTypographyPersonality: (personality) =>
        dispatch({ type: "set-typography-personality", personality }),
      setContactPlacement: (placement) =>
        dispatch({ type: "set-contact-placement", placement }),
      reset: () =>
        dispatch({ type: "reset", initial: latestInitialState.current }),
    }),
    [],
  );
  const stageIndex = PRINT_MENU_STAGES.indexOf(state.stage);

  return {
    state,
    actions,
    canGoBack: stageIndex > 0,
    canGoNext: stageIndex < PRINT_MENU_STAGES.length - 1,
  };
}
