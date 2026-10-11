import {
  MENU_DESIGN_FAMILY_IDS,
  type MenuDesignFamily,
  type MenuDesignFamilyId,
  type RegisteredMenuDesignFamily,
} from "../types";

const FAMILIES = new Map<MenuDesignFamilyId, RegisteredMenuDesignFamily>();

export function defineFamily(
  family: MenuDesignFamily,
): RegisteredMenuDesignFamily {
  if (!family.supportedTreatments.length || !family.supportedFormats.length) {
    throw new Error(
      `Menu family ${family.id} must support a treatment and format`,
    );
  }

  const registered: RegisteredMenuDesignFamily = {
    ...family,
    supportedTreatments: Object.freeze([...family.supportedTreatments]),
    supportedFormats: Object.freeze([...family.supportedFormats]),
    typography: Object.freeze({ ...family.typography }),
    paletteFallback: Object.freeze({ ...family.paletteFallback }),
    compositions: Object.freeze([...family.compositions]),
    photography: Object.freeze({
      ...family.photography,
      roles: Object.freeze([...family.photography.roles]),
    }),
  };

  return Object.freeze(registered);
}

export function registerMenuDesignFamily(
  family: RegisteredMenuDesignFamily,
): void {
  const registered = FAMILIES.get(family.id);
  if (registered === family) {
    return;
  }
  if (registered) {
    throw new Error(`Menu design family already registered: ${family.id}`);
  }
  FAMILIES.set(family.id, family);
}

export function getMenuDesignFamily(
  id: MenuDesignFamilyId,
): RegisteredMenuDesignFamily {
  const family = FAMILIES.get(id);
  if (!family) {
    throw new Error(`Unknown menu design family: ${id}`);
  }
  return family;
}

export function listMenuDesignFamilies(): RegisteredMenuDesignFamily[] {
  return MENU_DESIGN_FAMILY_IDS.map(getMenuDesignFamily);
}
