/** @jest-environment jsdom */

import {
  OFFER_EDIT_MODAL_TEST_ID,
  bindOfferModalEscape,
  dismissOwnedPicker,
  hasExpandedPickerOwnedBy,
  isTopmostOfferDialog,
  shouldCloseOfferModalOnEscape,
} from "./offerModalEscape";

function mountOfferDialog(): HTMLElement {
  const dialog = document.createElement("div");
  dialog.setAttribute("role", "dialog");
  dialog.setAttribute("aria-modal", "true");
  dialog.setAttribute("data-testid", OFFER_EDIT_MODAL_TEST_ID);
  const input = document.createElement("input");
  dialog.appendChild(input);
  document.body.appendChild(dialog);
  return dialog;
}

function mountStackedDialog(): HTMLElement {
  const stacked = document.createElement("div");
  stacked.setAttribute("role", "dialog");
  stacked.setAttribute("aria-modal", "true");
  stacked.setAttribute("data-testid", "stacked-overlay");
  document.body.appendChild(stacked);
  return stacked;
}

afterEach(() => {
  document.body.replaceChildren();
});

describe("#665 offer modal Escape layers", () => {
  it("closes on Escape from a focused field when the offer modal is the top layer", () => {
    const offer = mountOfferDialog();
    const input = offer.querySelector("input");
    expect(
      shouldCloseOfferModalOnEscape({ key: "Escape", target: input }, offer),
    ).toBe(true);
  });

  it("does not close under a stacked overlay", () => {
    const offer = mountOfferDialog();
    const stacked = mountStackedDialog();
    expect(isTopmostOfferDialog(offer)).toBe(false);
    expect(
      shouldCloseOfferModalOnEscape(
        { key: "Escape", target: offer.querySelector("input") },
        offer,
      ),
    ).toBe(false);
    stacked.remove();
    expect(isTopmostOfferDialog(offer)).toBe(true);
    expect(
      shouldCloseOfferModalOnEscape(
        { key: "Escape", target: offer.querySelector("input") },
        offer,
      ),
    ).toBe(true);
  });

  it("defers Escape to an expanded listbox owned by the offer modal", () => {
    const offer = mountOfferDialog();
    const combo = document.createElement("div");
    combo.setAttribute("role", "combobox");
    combo.setAttribute("aria-expanded", "true");
    combo.setAttribute("aria-controls", "offer-listbox");
    offer.appendChild(combo);
    const listbox = document.createElement("div");
    listbox.id = "offer-listbox";
    listbox.setAttribute("role", "listbox");
    document.body.appendChild(listbox);

    expect(hasExpandedPickerOwnedBy(offer, listbox)).toBe(true);
    expect(
      shouldCloseOfferModalOnEscape({ key: "Escape", target: combo }, offer),
    ).toBe(false);

    combo.setAttribute("aria-expanded", "false");
    expect(
      shouldCloseOfferModalOnEscape({ key: "Escape", target: combo }, offer),
    ).toBe(true);
  });

  it("still closes when some other page combobox is expanded", () => {
    const offer = mountOfferDialog();
    const pageCombo = document.createElement("div");
    pageCombo.setAttribute("role", "combobox");
    pageCombo.setAttribute("aria-expanded", "true");
    document.body.insertBefore(pageCombo, offer);

    expect(
      shouldCloseOfferModalOnEscape(
        { key: "Escape", target: offer.querySelector("input") },
        offer,
      ),
    ).toBe(true);
  });

  it("does not close when another layer already preventDefaulted Escape", () => {
    const offer = mountOfferDialog();
    expect(
      shouldCloseOfferModalOnEscape(
        { key: "Escape", target: offer.querySelector("input"), defaultPrevented: true },
        offer,
      ),
    ).toBe(false);
  });

  it("ignores keys other than Escape", () => {
    const offer = mountOfferDialog();
    expect(
      shouldCloseOfferModalOnEscape({ key: "Enter", target: document.body }, offer),
    ).toBe(false);
  });

  it("bindOfferModalEscape closes on window-capture Escape when it is the top layer", () => {
    const offer = mountOfferDialog();
    const close = jest.fn();
    const unbind = bindOfferModalEscape(close, () => offer);
    window.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    expect(close).toHaveBeenCalledTimes(1);
    unbind();
  });

  it("bindOfferModalEscape does not steal Escape from a stacked dialog", () => {
    const offer = mountOfferDialog();
    const stacked = mountStackedDialog();
    const close = jest.fn();
    const bubble = jest.fn();
    document.addEventListener("keydown", bubble);
    const unbind = bindOfferModalEscape(close, () => offer);

    stacked.dispatchEvent(
      new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
    );

    expect(close).not.toHaveBeenCalled();
    expect(bubble).toHaveBeenCalled();
    unbind();
    document.removeEventListener("keydown", bubble);
  });

  it("bindOfferModalEscape leaves the first Escape to an in-modal listbox", () => {
    const offer = mountOfferDialog();
    const combo = document.createElement("div");
    combo.setAttribute("role", "combobox");
    combo.setAttribute("aria-expanded", "true");
    combo.tabIndex = 0;
    const onComboClick = jest.fn();
    combo.addEventListener("click", onComboClick);
    combo.addEventListener("blur", () => {
      combo.setAttribute("aria-expanded", "false");
    });
    offer.appendChild(combo);
    combo.focus();
    const close = jest.fn();
    const unbind = bindOfferModalEscape(close, () => offer);

    combo.dispatchEvent(
      new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
    );
    expect(close).not.toHaveBeenCalled();
    expect(onComboClick).not.toHaveBeenCalled();
    expect(combo.getAttribute("aria-expanded")).toBe("false");

    combo.dispatchEvent(
      new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
    );
    expect(close).toHaveBeenCalledTimes(1);
    unbind();
  });

  it("collapses a picker via in-dialog interact-outside, not a trigger click (#743)", () => {
    const offer = mountOfferDialog();
    const header = document.createElement("header");
    offer.appendChild(header);
    const combo = document.createElement("button");
    combo.setAttribute("aria-expanded", "true");
    const onClick = jest.fn();
    combo.addEventListener("click", onClick);
    header.addEventListener("pointerdown", () => {
      combo.setAttribute("aria-expanded", "false");
    });
    offer.appendChild(combo);

    expect(dismissOwnedPicker(offer)).toBe(true);
    expect(onClick).not.toHaveBeenCalled();
    expect(combo.getAttribute("aria-expanded")).toBe("false");
  });

  it("never click()s an expanded trigger to collapse it (#743)", () => {
    const offer = mountOfferDialog();
    const combo = document.createElement("button");
    combo.setAttribute("aria-expanded", "true");
    const onClick = jest.fn();
    combo.addEventListener("click", onClick);
    offer.appendChild(combo);

    expect(dismissOwnedPicker(offer)).toBe(true);
    expect(onClick).not.toHaveBeenCalled();
  });
});
