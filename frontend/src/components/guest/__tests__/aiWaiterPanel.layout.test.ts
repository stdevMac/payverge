import fs from "fs";
import path from "path";
/**
 * Guest Sage panel geometry against NextUI's real `modal()` slots (#724).
 *
 * A hand-rolled slot mock already shipped the phone card: it never ran
 * tailwind-merge, so `max-w-none` and `max-w-md` could coexist and the
 * test still passed. These assertions use `@nextui-org/theme` the same
 * way `useModal` does, then evaluate the merged class list at a phone
 * width (390) and a desktop width (1280).
 */
import { modal } from "@nextui-org/theme";
import { AI_WAITER_PANEL_MODAL } from "@/components/guest/aiWaiterPanel";

const PHONE_WIDTH = 390;
const DESKTOP_WIDTH = 1280;
const SM = 640;

const resolvePanelSlots = () => {
  const { size, placement, scrollBehavior, backdrop, classNames } =
    AI_WAITER_PANEL_MODAL;
  const slots = modal({ size, placement, scrollBehavior, backdrop });
  return {
    base: slots.base({ class: classNames.base }),
    wrapper: slots.wrapper({ class: classNames.wrapper }),
    backdrop: slots.backdrop({ class: classNames.backdrop }),
  };
};

/** Classes that actually apply at `viewportWidth` after NextUI's merge. */
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

const maxWidthTokens = (tokens: string[]): string[] =>
  tokens
    .map(stripImportant)
    .filter((token) => token.startsWith("max-w-"));

const marginYTokens = (tokens: string[]): string[] =>
  tokens
    .map(stripImportant)
    .filter((token) => token.startsWith("my-") || token === "m-0");

const itemsTokens = (tokens: string[]): string[] =>
  tokens.map(stripImportant).filter((token) => token.startsWith("items-"));

describe("Guest Sage panel NextUI slots (#724)", () => {
  const slots = resolvePanelSlots();

  it("is a full-width sheet at a phone viewport, not max-w-md", () => {
    const base = tokensAt(slots.base, PHONE_WIDTH);
    expect(hasToken(base, "w-full")).toBe(true);
    expect(maxWidthTokens(base)).toEqual(
      expect.arrayContaining(["max-w-none"]),
    );
    expect(maxWidthTokens(base)).not.toContain("max-w-md");
    expect(hasToken(tokensAt(slots.wrapper, PHONE_WIDTH), "items-end")).toBe(
      true,
    );
  });

  it("docks as a full-height rail at a desktop viewport, not a floating phone card", () => {
    const base = tokensAt(slots.base, DESKTOP_WIDTH);
    const wrapper = tokensAt(slots.wrapper, DESKTOP_WIDTH);

    expect(hasToken(base, "h-[100dvh]")).toBe(true);
    expect(hasToken(base, "w-[26rem]")).toBe(true);
    expect(marginYTokens(base)).not.toContain("my-16");
    expect(hasToken(base, "mx-6")).toBe(false);

    expect(itemsTokens(wrapper)).toContain("items-stretch");
    expect(itemsTokens(wrapper)).not.toContain("items-center");
    expect(hasToken(wrapper, "justify-end")).toBe(true);
  });

  it("dims the menu instead of blurring it away", () => {
    expect(slots.backdrop).not.toMatch(/backdrop-blur/);
  });

  it("is the object AiWaiter spreads onto the real Modal", () => {
    const src = fs.readFileSync(
      path.join(__dirname, "../AiWaiter.tsx"),
      "utf8",
    );
    expect(src).toContain("{...AI_WAITER_PANEL_MODAL}");
  });

  it("fails the NextUI defaults that painted the live phone card", () => {
    const def = modal({});
    const defaultBase = def.base();
    const defaultWrapper = def.wrapper();
    expect(tokensAt(defaultBase, PHONE_WIDTH).some((t) => stripImportant(t) === "max-w-md")).toBe(
      true,
    );
    expect(
      tokensAt(defaultWrapper, DESKTOP_WIDTH).some(
        (t) => stripImportant(t) === "items-center",
      ),
    ).toBe(true);
    expect(tokensAt(defaultBase, DESKTOP_WIDTH).some((t) => stripImportant(t) === "my-16")).toBe(
      true,
    );
  });
});
