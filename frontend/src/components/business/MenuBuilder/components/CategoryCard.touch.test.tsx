/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { DndContext } from "@dnd-kit/core";
import { SortableContext } from "@dnd-kit/sortable";
import { CategoryCard } from "./CategoryCard";
import type { MenuCategory, MenuItem } from "../../../../api/business";

// Passthrough translator: returns the i18n key verbatim (repo modal-test convention).
const tString = (key: string) => key;

// Empty category → only the header-level edit/more render (no per-item
// MenuItemCard actions), so the queries below unambiguously target the
// category controls.
const category: MenuCategory & { items: MenuItem[] } = {
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

// Category edit/more must be ≥44px touch targets. Delete is in overflow (#116).
for (const viewMode of ["grid", "list"] as const) {
  it(`${viewMode} view: category edit/more are ≥44px touch targets`, () => {
    render(<CategoryCard {...baseProps} viewMode={viewMode} />);
    const edit = screen.getByLabelText("buttons.edit Mains");
    const more = screen.getByLabelText("buttons.moreActions Mains");
    for (const btn of [edit, more]) {
      expect(btn.className).toContain("h-11");
      expect(btn.className).toContain("w-11");
      expect(btn.className).toContain("[@media(hover:hover)_and_(pointer:fine)]:h-8");
      expect(btn.className).not.toContain("[@media(hover:hover)]:h-8");
    }
    expect(
      screen.queryByLabelText("buttons.delete Mains"),
    ).not.toBeInTheDocument();
  });
}

it("gives the category drag handle a contextual accessible name", () => {
  render(
    <DndContext>
      <SortableContext items={["cat-pos-0"]}>
        <CategoryCard {...baseProps} viewMode="grid" isDraggable />
      </SortableContext>
    </DndContext>,
  );

  expect(
    screen.getByRole("button", { name: "buttons.reorderCategory Mains" }),
  ).toBeInTheDocument();
});

it("exposes delete only via the overflow menu (#116)", async () => {
  render(<CategoryCard {...baseProps} viewMode="grid" />);
  fireEvent.click(screen.getByTestId("category-more-actions"));
  const del = await screen.findByTestId("category-delete-action");
  fireEvent.click(del);
  expect(baseProps.handleDeleteCategory).toHaveBeenCalledWith(0);
});
