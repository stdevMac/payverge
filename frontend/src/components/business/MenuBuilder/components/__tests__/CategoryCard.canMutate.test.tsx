/** @jest-environment jsdom */
/**
 * L3-7: canMutate must reach category edit/more (and add-item) buttons.
 */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { CategoryCard } from "../CategoryCard";
import type { MenuCategory, MenuItem } from "../../../../../api/business";

const tString = (key: string) => key;

const category = {
  name: "Mains",
  description: "House favourites",
  items: [],
} as unknown as MenuCategory & { items: MenuItem[] };

const baseProps = {
  category,
  categoryIndex: 0,
  originalCategoryIndex: 0,
  tString,
  onAddItemOpen: jest.fn(),
  handleEditCategory: jest.fn(),
  handleDeleteCategory: jest.fn(),
  handleEditItem: jest.fn(),
  handleDeleteItem: jest.fn(),
  isDraggable: false,
  menu: [category],
  inventoryStatuses: {},
};

describe("L3-7 CategoryCard canMutate", () => {
  it("disables category edit/more/add when canMutate is false", () => {
    render(
      <CategoryCard {...baseProps} viewMode="list" canMutate={false} />,
    );
    const edit = screen.getByLabelText("buttons.edit Mains");
    const more = screen.getByLabelText("buttons.moreActions Mains");
    const adds = screen.getAllByRole("button", { name: /buttons.addItem/i });
    expect(edit).toBeDisabled();
    expect(more).toBeDisabled();
    expect(adds.every((b) => (b as HTMLButtonElement).disabled)).toBe(true);
    expect(edit).toHaveAttribute("title", "translatedEditBlocked");
    fireEvent.click(edit);
    expect(baseProps.handleEditCategory).not.toHaveBeenCalled();
  });

  // L3-41: identity stubs hide the placeholder bug — use the real template.
  it("interpolates {language} into the blocked tooltip (L3-41)", () => {
    const realT = (key: string) =>
      key === "translatedEditBlocked"
        ? "Switch to {language} to edit"
        : key;
    render(
      <CategoryCard
        {...baseProps}
        tString={realT}
        viewMode="list"
        canMutate={false}
        editLanguageName="Español"
      />,
    );
    expect(screen.getByLabelText("buttons.edit Mains")).toHaveAttribute(
      "title",
      "Switch to Español to edit",
    );
    expect(document.body.innerHTML).not.toContain("{language}");
  });
});
