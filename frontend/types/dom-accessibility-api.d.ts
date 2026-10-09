declare module "dom-accessibility-api" {
  export function computeAccessibleName(el: Element): string;
  export function computeAccessibleDescription(el: Element): string;
}
