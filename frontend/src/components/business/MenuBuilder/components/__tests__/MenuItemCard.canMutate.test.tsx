/** @jest-environment jsdom */
/**
 * L3-7: canMutate must reach edit/delete on MenuItemCard.
 * L3-11: list-view grip must preserve dnd-kit onKeyDown (KeyboardSensor).
 */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { MenuItemCard } from "../MenuItemCard";
import type { MenuItem } from "../../../../../api/business";

const tString = (key: string) => key;

const item = {
  name: "Burger",
  description: "Beef",
  price: 12,
  currency: "USD",
  is_available: true,
  allergens: [],
  dietary_tags: [],
} as unknown as MenuItem;

const baseProps = {
  item,
  originalCategoryIndex: 0,
  originalItemIndex: 0,
  tString,
  handleEditItem: jest.fn(),
  handleDeleteItem: jest.fn(),
};

describe("L3-7 canMutate gates edit/delete", () => {
  it("disables edit/more when canMutate is false and surfaces translatedEditBlocked", () => {
    render(
      <MenuItemCard {...baseProps} viewMode="list" canMutate={false} />,
    );
    const edit = screen.getByRole("button", { name: "buttons.edit Burger" });
    const more = screen.getByRole("button", {
      name: "buttons.moreActions Burger",
    });
    expect(edit).toBeDisabled();
    expect(more).toBeDisabled();
    expect(edit).toHaveAttribute("title", "translatedEditBlocked");
    expect(more).toHaveAttribute("title", "translatedEditBlocked");
    fireEvent.click(edit);
    fireEvent.click(more);
    expect(baseProps.handleEditItem).not.toHaveBeenCalled();
    expect(baseProps.handleDeleteItem).not.toHaveBeenCalled();
  });

  // L3-41: the real template carries a {language} placeholder. An identity
  // `tString` stub can never catch a missing interpolation, so this test feeds
  // the REAL en string and asserts the DOM never ships the raw placeholder.
  it("interpolates {language} into the blocked tooltip (L3-41)", () => {
    const realT = (key: string) =>
      key === "translatedEditBlocked"
        ? "Switch to {language} to edit"
        : key;
    render(
      <MenuItemCard
        {...baseProps}
        tString={realT}
        viewMode="list"
        canMutate={false}
        editLanguageName="Español"
      />,
    );
    const edit = screen.getByRole("button", { name: "buttons.edit Burger" });
    const more = screen.getByRole("button", {
      name: "buttons.moreActions Burger",
    });
    expect(edit).toHaveAttribute("title", "Switch to Español to edit");
    expect(more).toHaveAttribute("title", "Switch to Español to edit");
    expect(document.body.innerHTML).not.toContain("{language}");
  });

  it("keeps edit/more enabled when canMutate is true (default)", () => {
    render(<MenuItemCard {...baseProps} viewMode="list" />);
    expect(
      screen.getByRole("button", { name: "buttons.edit Burger" }),
    ).not.toBeDisabled();
    expect(
      screen.getByRole("button", { name: "buttons.moreActions Burger" }),
    ).not.toBeDisabled();
  });
});

// D1 / L3-11: list grip must forward Space/keyboard to dnd-kit's onKeyDown.
// Revert-proof: a grip onKeyDown that only preventDefault (swallows) makes this red.
describe("L3-11 keyboard drag: merge onKeyDown with dnd-kit", () => {
  it("invokes dragHandleProps.onKeyDown on the list grip (does not swallow it)", () => {
    const onKeyDown = jest.fn();
    render(
      <MenuItemCard
        {...baseProps}
        viewMode="list"
        dragHandleProps={{ onKeyDown, tabIndex: 0 } as React.HTMLAttributes<HTMLElement>}
      />,
    );
    const grip = screen.getByRole("button", {
      name: "buttons.reorder Burger",
    });
    fireEvent.keyDown(grip, { key: " ", code: "Space" });
    expect(onKeyDown).toHaveBeenCalled();
  });
});
