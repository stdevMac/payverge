/** @jest-environment jsdom */
import React from "react";
import { render, fireEvent, cleanup } from "@testing-library/react";
import { Modal, ModalBody, ModalContent } from "@nextui-org/react";

/**
 * Root B regression guard — pins the @nextui-org/modal 2.2.7 DOM contract
 * that the tailwind.config.ts `:has()` shield depends on:
 *
 *   '[data-slot="wrapper"]:has(> [aria-modal="true"][data-dismissable="true"])'
 *     { pointer-events: none }
 *
 * Premises verified here against the REAL rendered component:
 *  1. getDialogProps stamps `data-dismissable="true"` on the dialog when
 *     `isDismissable` and OMITS the attribute when `isDismissable={false}`
 *     (dataAttr → undefined) — never "false".
 *  2. The dialog is a DIRECT child of the `data-slot="wrapper"` element
 *     (the selector uses the `>` combinator).
 *  3. The dialog always carries `aria-modal="true"` (this is what keeps the
 *     rule off NextUI Pagination, the only other `data-slot="wrapper"`).
 *  4. CANARY: getBackdropProps wires an UNCONDITIONAL
 *     `onClick: () => state.close()` on the backdrop (a sibling of the
 *     wrapper). jsdom applies no CSS, so clicking the backdrop here closes
 *     even an `isDismissable={false}` modal — proving the CSS wrapper shield
 *     is the ONLY thing preventing that in production. A behavioral
 *     "onClose NOT called" test is therefore impossible in jsdom; if this
 *     canary ever fails, NextUI made the backdrop close conditional upstream
 *     and the tailwind scoping can be revisited.
 */

function renderModal(isDismissable: boolean, onClose: () => void) {
  return render(
    <Modal
      isOpen
      isDismissable={isDismissable}
      onClose={onClose}
      disableAnimation
    >
      <ModalContent>
        <ModalBody>
          <p>body</p>
        </ModalBody>
      </ModalContent>
    </Modal>,
  );
}

function getParts() {
  const wrapper = document.querySelector<HTMLElement>('[data-slot="wrapper"]');
  if (!wrapper) throw new Error("modal wrapper not rendered");
  const dialog = document.querySelector<HTMLElement>('section[role="dialog"]');
  if (!dialog) throw new Error("modal dialog not rendered");
  const backdrop = wrapper.previousElementSibling as HTMLElement | null;
  if (!backdrop) throw new Error("modal backdrop not rendered");
  return { wrapper, dialog, backdrop };
}

afterEach(cleanup);

describe("NextUI modal data-dismissable contract (Root B)", () => {
  it("stamps data-dismissable=\"true\" on the dialog, as a DIRECT wrapper child, when dismissable", () => {
    renderModal(true, jest.fn());
    const { wrapper, dialog } = getParts();
    expect(dialog.getAttribute("data-dismissable")).toBe("true");
    expect(dialog.getAttribute("aria-modal")).toBe("true");
    // `:has(> …)` direct-child combinator premise.
    expect(dialog.parentElement).toBe(wrapper);
  });

  it("OMITS data-dismissable entirely (not \"false\") when isDismissable={false}", () => {
    renderModal(false, jest.fn());
    const { wrapper, dialog } = getParts();
    expect(dialog.hasAttribute("data-dismissable")).toBe(false);
    expect(dialog.getAttribute("aria-modal")).toBe("true");
    expect(dialog.parentElement).toBe(wrapper);
  });

  it("CANARY: backdrop click closes even a non-dismissable modal without the CSS shield (jsdom has no CSS)", () => {
    const onClose = jest.fn();
    renderModal(false, onClose);
    const { backdrop } = getParts();
    fireEvent.click(backdrop);
    // NextUI 2.2.7's backdrop onClick is unconditional — only the tailwind
    // wrapper shield (pointer-events kept on non-dismissable wrappers) stops
    // this in the browser. If this assertion starts failing, upstream fixed
    // the backdrop and the Root B scoping can be simplified.
    expect(onClose).toHaveBeenCalled();
  });

  it("backdrop click closes a dismissable modal (expected product behavior)", () => {
    const onClose = jest.fn();
    renderModal(true, onClose);
    const { backdrop } = getParts();
    fireEvent.click(backdrop);
    expect(onClose).toHaveBeenCalled();
  });
});
