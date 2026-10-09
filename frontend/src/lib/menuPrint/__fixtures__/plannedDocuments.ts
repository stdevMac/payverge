import type { MenuPrintAudit } from "../audit";
import { getMenuDesignFamily } from "../families";
import { resolvePrintPalette } from "../palette";
import { planMenuDocument } from "../planner/planDocument";
import type {
  MenuArtDirection,
  PlannedMenuDocument,
  PrintMenuModel,
} from "../types";
import { PRINT_TYPOGRAPHY_BY_FAMILY } from "../types";
import { sparseRestaurantMenu } from "./restaurantMenus";

export interface PlanFixtureOverrides {
  audit?: Partial<MenuPrintAudit>;
  direction?: Partial<MenuArtDirection>;
}

export interface PlanFixtureDocumentInput
  extends Partial<MenuArtDirection>, PlanFixtureOverrides {
  model?: PrintMenuModel;
  failedUrls?: string[];
  lowResolutionUrls?: string[];
  duplicateUrls?: string[];
}

export function makeAudit(overrides?: Partial<MenuPrintAudit>): MenuPrintAudit;
export function makeAudit(
  model: PrintMenuModel,
  overrides?: Partial<MenuPrintAudit>,
): MenuPrintAudit;
export function makeAudit(
  modelOrOverrides: PrintMenuModel | Partial<MenuPrintAudit> = {},
  suppliedOverrides: Partial<MenuPrintAudit> = {},
): MenuPrintAudit {
  const isModel = "sections" in modelOrOverrides;
  const model = isModel ? modelOrOverrides : sparseRestaurantMenu;
  const overrides = isModel ? suppliedOverrides : modelOrOverrides;
  const items = model.sections.flatMap(
    ({ items: sectionItems }) => sectionItems,
  );
  const described = items.filter(({ description }) => Boolean(description));
  const urls = items.flatMap(({ imageCandidates }) => imageCandidates ?? []);
  const defaults: MenuPrintAudit = {
    categoryCount: model.sections.length,
    itemCount: items.length,
    describedItemCount: described.length,
    averageDescriptionLength: described.length
      ? described.reduce(
          (sum, item) => sum + (item.description?.length ?? 0),
          0,
        ) / described.length
      : 0,
    availablePhotoCount: new Set(urls).size,
    photoCoverage: items.length
      ? items.filter(({ imageCandidates }) => imageCandidates?.length).length /
        items.length
      : 0,
    lowResolutionUrls: [],
    failedUrls: [],
    duplicateUrls: urls.filter((url, index) => urls.indexOf(url) !== index),
    measurements: {},
  };
  return {
    ...defaults,
    ...overrides,
    lowResolutionUrls: [
      ...(overrides.lowResolutionUrls ?? defaults.lowResolutionUrls),
    ],
    failedUrls: [...(overrides.failedUrls ?? defaults.failedUrls)],
    duplicateUrls: [...(overrides.duplicateUrls ?? defaults.duplicateUrls)],
    measurements: Object.fromEntries(
      Object.entries(overrides.measurements ?? defaults.measurements ?? {}).map(
        ([url, measurement]) => [url, { ...measurement }],
      ),
    ),
  };
}

export function makeDirection(
  overrides: Partial<MenuArtDirection> = {},
): MenuArtDirection {
  const familyId = overrides.familyId ?? "atelier";
  const family = getMenuDesignFamily(familyId);
  const familyChanged = overrides.familyId !== undefined;
  const treatment =
    overrides.treatment ??
    (familyChanged ? family.supportedTreatments[0] : "type-led");
  const outputFormat =
    overrides.outputFormat ??
    (familyChanged ? family.supportedFormats[0] : "single-sheet");
  if (!family.supportedTreatments.includes(treatment)) {
    throw new Error(
      `Menu family ${family.id} does not support treatment ${treatment}`,
    );
  }
  if (!family.supportedFormats.includes(outputFormat)) {
    throw new Error(
      `Menu family ${family.id} does not support format ${outputFormat}`,
    );
  }
  return {
    familyId,
    treatment,
    outputFormat,
    paperFormat: overrides.paperFormat ?? "a4",
    palette: overrides.palette
      ? { ...overrides.palette }
      : resolvePrintPalette({ familyId }),
    imageSelections: (overrides.imageSelections ?? []).map((selection) => ({
      ...selection,
    })),
    imageFocalPoints: Object.fromEntries(
      Object.entries(overrides.imageFocalPoints ?? {}).map(([url, point]) => [
        url,
        { ...point },
      ]),
    ),
    logoTreatment: overrides.logoTreatment ?? "contained",
    coverMode: overrides.coverMode ?? "none",
    ornamentIntensity: overrides.ornamentIntensity ?? "restrained",
    typographyPersonality:
      overrides.typographyPersonality ??
      PRINT_TYPOGRAPHY_BY_FAMILY[familyId][0],
    contactPlacement: overrides.contactPlacement ?? "footer",
  };
}

export function planFixtureDocument(
  input?: PlanFixtureDocumentInput,
): PlannedMenuDocument;
export function planFixtureDocument(
  model: PrintMenuModel,
  overrides?: PlanFixtureOverrides,
): PlannedMenuDocument;
export function planFixtureDocument(
  modelOrInput: PrintMenuModel | PlanFixtureDocumentInput = {},
  suppliedOverrides: PlanFixtureOverrides = {},
): PlannedMenuDocument {
  const legacyModelCall = "sections" in modelOrInput;
  const model = legacyModelCall
    ? modelOrInput
    : (modelOrInput.model ?? sparseRestaurantMenu);
  const input = (
    legacyModelCall ? suppliedOverrides : modelOrInput
  ) as PlanFixtureDocumentInput;
  const flatDirection: Partial<MenuArtDirection> = legacyModelCall
    ? {}
    : {
        familyId: input.familyId,
        treatment: input.treatment,
        outputFormat: input.outputFormat,
        paperFormat: input.paperFormat,
        palette: input.palette,
        imageSelections: input.imageSelections,
        imageFocalPoints: input.imageFocalPoints,
        logoTreatment: input.logoTreatment,
        coverMode: input.coverMode,
        ornamentIntensity: input.ornamentIntensity,
        typographyPersonality: input.typographyPersonality,
        contactPlacement: input.contactPlacement,
      };
  const auditOverrides: Partial<MenuPrintAudit> = {
    ...input.audit,
    failedUrls: legacyModelCall
      ? input.audit?.failedUrls
      : (input.failedUrls ?? input.audit?.failedUrls),
    lowResolutionUrls: legacyModelCall
      ? input.audit?.lowResolutionUrls
      : (input.lowResolutionUrls ?? input.audit?.lowResolutionUrls),
    duplicateUrls: legacyModelCall
      ? input.audit?.duplicateUrls
      : (input.duplicateUrls ?? input.audit?.duplicateUrls),
  };
  return planMenuDocument({
    model,
    audit: makeAudit(model, auditOverrides),
    direction: makeDirection({ ...flatDirection, ...input.direction }),
  });
}
