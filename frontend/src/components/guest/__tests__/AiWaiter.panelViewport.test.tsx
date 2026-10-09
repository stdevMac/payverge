/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { Modal, ModalBody, ModalContent, NextUIProvider } from "@nextui-org/react";
import { AI_WAITER_PANEL_MODAL } from "@/components/guest/aiWaiterPanel";

const PHONE_WIDTH = 390;
const DESKTOP_WIDTH = 1280;
const SM = 640;

const tokensAt = (className: string, viewportWidth: number): string[] =>
  className
    .split(/\s+/)
    .filter(Boolean)
    .flatMap((token) => {
      if (token.startsWith("sm:")) {
        return viewportWidth >= SM ? [token.slice(3)] : [];
      }
      if (/^(md|lg|xl|2xl):/.test(token)) return [];
      return [token];
    });

const stripImportant = (token: string): string =>
  token.startsWith("!") ? token.slice(1) : token;

const hasToken = (tokens: string[], name: string): boolean =>
  tokens.some((token) => stripImportant(token) === name);

const setViewport = (width: number) => {
  Object.defineProperty(window, "innerWidth", {
    configurable: true,
    writable: true,
    value: width,
  });
  window.dispatchEvent(new Event("resize"));
};

const mountPanel = () =>
  render(
    <NextUIProvider>
      <Modal
        isOpen
        disableAnimation
        aria-label="Sage"
        {...AI_WAITER_PANEL_MODAL}
      >
        <ModalContent>
          <ModalBody>diner chat</ModalBody>
        </ModalContent>
      </Modal>
    </NextUIProvider>,
  );

describe("Guest Sage panel real NextUI Modal (#724)", () => {
  it("applies a full-width sheet at a phone viewport", () => {
    setViewport(PHONE_WIDTH);
    mountPanel();

    const dialog = screen.getByRole("dialog");
    const wrapper = document.querySelector("[data-slot='wrapper']");
    expect(wrapper).not.toBeNull();
    const base = tokensAt(dialog.className, PHONE_WIDTH);
    expect(hasToken(base, "w-full")).toBe(true);
    expect(
      base.map(stripImportant).filter((token) => token.startsWith("max-w-")),
    ).toEqual(expect.arrayContaining(["max-w-none"]));
    expect(
      base.map(stripImportant).filter((token) => token.startsWith("max-w-")),
    ).not.toContain("max-w-md");
    expect(
      hasToken(tokensAt(wrapper!.className, PHONE_WIDTH), "items-end"),
    ).toBe(true);
  });

  it("docks to the inline-end edge at a desktop viewport instead of centering a phone card", () => {
    setViewport(DESKTOP_WIDTH);
    mountPanel();

    const dialog = screen.getByRole("dialog");
    const wrapper = document.querySelector("[data-slot='wrapper']");
    expect(wrapper).not.toBeNull();
    const base = tokensAt(dialog.className, DESKTOP_WIDTH);
    const wrap = tokensAt(wrapper!.className, DESKTOP_WIDTH);
    expect(hasToken(base, "h-[100dvh]")).toBe(true);
    expect(hasToken(base, "w-[26rem]")).toBe(true);
    expect(base.map(stripImportant)).not.toContain("my-16");
    expect(wrap.map(stripImportant)).toContain("items-stretch");
    expect(wrap.map(stripImportant)).not.toContain("items-center");
    expect(hasToken(wrap, "justify-end")).toBe(true);
  });
});
