import { atelierFamily } from "./atelier";
import { counterFamily } from "./counter";
import { fieldFamily } from "./field";
import { galleryFamily } from "./gallery";
import { maisonFamily } from "./maison";
import { nightHouseFamily } from "./nightHouse";
import { osteriaFamily } from "./osteria";
import { listMenuDesignFamilies, registerMenuDesignFamily } from "./registry";
import { streetFamily } from "./street";
import type { RegisteredMenuDesignFamily } from "../types";

[
  atelierFamily,
  maisonFamily,
  osteriaFamily,
  nightHouseFamily,
  counterFamily,
  streetFamily,
  fieldFamily,
  galleryFamily,
].forEach(registerMenuDesignFamily);

export const ALL_MENU_DESIGN_FAMILIES: readonly RegisteredMenuDesignFamily[] =
  Object.freeze(listMenuDesignFamilies());

export { atelierFamily } from "./atelier";
export { counterFamily } from "./counter";
export { fieldFamily } from "./field";
export { galleryFamily } from "./gallery";
export { maisonFamily } from "./maison";
export { nightHouseFamily } from "./nightHouse";
export { osteriaFamily } from "./osteria";
export {
  defineFamily,
  getMenuDesignFamily,
  listMenuDesignFamilies,
  registerMenuDesignFamily,
} from "./registry";
export { streetFamily } from "./street";
export type { RegisteredMenuDesignFamily } from "../types";
