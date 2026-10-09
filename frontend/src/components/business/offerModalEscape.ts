/**
 * #665 — offer edit/create modal Escape.
 *
 * NextUI's overlay can swallow Escape (Select/Autocomplete preventDefault
 * without closing the dialog). useDisclosure.onClose also does not always
 * tear down the portaled overlay — Cancel works because it calls the
 * ModalContent render-prop `close`. Bind Escape to that same function.
 *
 * Capture listeners must not steal Escape from a stacked overlay, and must
 * not no-op just because some other combobox on the page is expanded.
 * One Escape, one layer: close an in-modal listbox first; a later Escape
 * closes the offer modal when it is the topmost dialog.
 */

export const OFFER_EDIT_MODAL_TEST_ID = "offer-edit-modal";

const OVERLAY_SELECTOR =
  '[role="dialog"], [role="alertdialog"], [aria-modal="true"]';

function isDismissedOverlay(el: Element): boolean {
  return (
    el.hasAttribute("hidden") ||
    el.getAttribute("aria-hidden") === "true" ||
    el.getAttribute("data-hidden") === "true"
  );
}

function escapeAttrSelector(value: string): string {
  if (typeof CSS !== "undefined" && typeof CSS.escape === "function") {
    return CSS.escape(value);
  }
  return value.replace(/\\/g, "\\\\").replace(/"/g, '\\"');
}

/** Open dialogs / alertdialogs / aria-modal roots, in document order. */
function visibleOverlayRoots(): HTMLElement[] {
  return Array.from(
    document.querySelectorAll<HTMLElement>(OVERLAY_SELECTOR),
  ).filter((el) => !isDismissedOverlay(el));
}

function resolveOfferModalRoot(offerRoot: Element | null): HTMLElement | null {
  const start =
    offerRoot ??
    document.querySelector<HTMLElement>(
      `[data-testid="${OFFER_EDIT_MODAL_TEST_ID}"]`,
    );
  if (!start) return null;
  const overlay =
    (start.matches(OVERLAY_SELECTOR) ? start : null) ??
    start.querySelector(OVERLAY_SELECTOR) ??
    start.closest(OVERLAY_SELECTOR) ??
    start;
  return overlay instanceof HTMLElement ? overlay : null;
}

/**
 * True when no other overlay sits on top of the offer dialog.
 * A wrapper that contains the offer (or a nested node inside it) is the
 * same layer — only a sibling/portaled dialog counts as stacked.
 */
export function isTopmostOfferDialog(offerRoot: Element): boolean {
  const offer = resolveOfferModalRoot(offerRoot);
  if (!offer) return false;
  return !visibleOverlayRoots().some(
    (el) => el !== offer && !offer.contains(el) && !el.contains(offer),
  );
}

/**
 * Expanded Select/Autocomplete owned by the offer modal — including a
 * listbox NextUI portaled to document.body and wired via aria-controls.
 * Page-level comboboxes are ignored.
 */
export function hasExpandedPickerOwnedBy(
  offerRoot: Element,
  target: EventTarget | null,
): boolean {
  const offer = resolveOfferModalRoot(offerRoot);
  if (!offer) return false;

  if (offer.querySelector('[aria-expanded="true"]')) return true;

  if (!(target instanceof Element)) return false;
  const listbox = target.closest("[role='listbox']");
  if (!listbox) return false;
  if (offer.contains(listbox)) return true;
  const id = listbox.id;
  if (
    id &&
    offer.querySelector(
      `[aria-controls="${escapeAttrSelector(id)}"][aria-expanded="true"]`,
    )
  ) {
    return true;
  }
  return false;
}

export function shouldCloseOfferModalOnEscape(
  event: Pick<KeyboardEvent, "key" | "target"> & { defaultPrevented?: boolean },
  offerRoot: Element | null,
): boolean {
  if (event.key !== "Escape") return false;
  // NextUI / React-aria close an open listbox with preventDefault. That
  // keypress must not also tear down the offer modal.
  if (event.defaultPrevented) return false;
  const offer = resolveOfferModalRoot(offerRoot);
  if (!offer) return false;
  if (!isTopmostOfferDialog(offer)) return false;
  if (hasExpandedPickerOwnedBy(offer, event.target)) return false;
  return true;
}

/**
 * Collapse an in-modal Select/Autocomplete so the first Escape closes that
 * layer. Never click the trigger — NextUI Select treats a trigger click as
 * `onChange("")` and wipes committed Applies-to / Target (#743).
 */
export function dismissOwnedPicker(offerRoot: Element): boolean {
  const offer = resolveOfferModalRoot(offerRoot);
  if (!offer) return false;
  const expanded = offer.querySelector<HTMLElement>('[aria-expanded="true"]');
  if (!expanded) return false;

  // Interact-outside on a neutral in-dialog surface, then blur. React Aria
  // closes the listbox without changing the selected key.
  const surface =
    offer.querySelector<HTMLElement>("[data-slot='header']") ??
    offer.querySelector<HTMLElement>("header") ??
    offer;
  if (surface !== expanded) {
    surface.dispatchEvent(
      new MouseEvent("pointerdown", { bubbles: true, cancelable: true }),
    );
  }
  expanded.blur();
  return true;
}

export function bindOfferModalEscape(
  close: () => void,
  getOfferRoot: () => Element | null,
): () => void {
  let closed = false;
  const onKeyDown = (event: KeyboardEvent) => {
    if (event.key !== "Escape") return;
    const offer = resolveOfferModalRoot(getOfferRoot());
    if (!offer) return;

    // Stacked overlay on top: do not steal Escape.
    if (!isTopmostOfferDialog(offer)) return;

    // In-modal listbox: close that layer only. Stop so the NextUI modal
    // dismiss does not run on the same keypress.
    if (hasExpandedPickerOwnedBy(offer, event.target)) {
      dismissOwnedPicker(offer);
      event.preventDefault();
      event.stopPropagation();
      return;
    }

    if (!shouldCloseOfferModalOnEscape(event, offer)) return;
    event.preventDefault();
    event.stopPropagation();
    if (closed) return;
    closed = true;
    close();
  };
  // Window capture runs before document / React-aria overlay listeners.
  // Document capture covers jsdom tests that dispatch on `document`.
  window.addEventListener("keydown", onKeyDown, true);
  document.addEventListener("keydown", onKeyDown, true);
  return () => {
    window.removeEventListener("keydown", onKeyDown, true);
    document.removeEventListener("keydown", onKeyDown, true);
  };
}
