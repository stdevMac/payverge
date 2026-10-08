/**
 * Compose an operator-control accessible name: visible purpose + current
 * value/state. NextUI Select triggers otherwise announce the value only;
 * empty Switch labelledby nodes otherwise announce nothing (#446).
 */
export function formatControlName(purpose: string, value: string): string {
  const p = purpose.trim();
  const v = value.trim();
  if (!v) return p;
  if (v.toLowerCase() === p.toLowerCase()) return p;
  return `${p}, ${v}`;
}

function isSwitchOn(el: HTMLElement): boolean {
  const aria = el.getAttribute("aria-checked");
  if (aria === "true") return true;
  if (aria === "false") return false;
  if (el instanceof HTMLInputElement) return el.checked;
  return el.getAttribute("data-selected") === "true";
}

export function announcedSwitchName(
  el: HTMLElement,
  onLabel = "on",
  offLabel = "off",
): string {
  return formatControlName(
    el.getAttribute("aria-label") || el.textContent || "",
    isSwitchOn(el) ? onLabel : offLabel,
  );
}

export function labelledByTargetsHaveText(el: HTMLElement): boolean {
  const labelledBy = el.getAttribute("aria-labelledby");
  if (!labelledBy) return true;
  return labelledBy.split(/\s+/).every((id) => {
    if (!id) return true;
    return Boolean(document.getElementById(id)?.textContent?.trim());
  });
}

export function applyNamedControl(
  el: HTMLElement | null,
  name: string,
  options?: {
    labelId?: string;
    descriptionId?: string;
    /** Drop labelledby so aria-label (purpose + value) wins. */
    replaceLabelledBy?: boolean;
  },
): void {
  if (!el) return;
  const labelEl = options?.labelId
    ? document.getElementById(options.labelId)
    : null;
  const labelText = labelEl?.textContent?.trim() ?? "";

  el.setAttribute("aria-label", name);

  if (options?.replaceLabelledBy) {
    el.removeAttribute("aria-labelledby");
  } else if (options?.labelId && labelText) {
    el.setAttribute("aria-labelledby", options.labelId);
  } else if (!labelledByTargetsHaveText(el)) {
    el.removeAttribute("aria-labelledby");
  }

  if (
    options?.descriptionId &&
    document.getElementById(options.descriptionId)
  ) {
    el.setAttribute("aria-describedby", options.descriptionId);
  }
}
