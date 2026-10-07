/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import type { MenuCategory } from "../../../../api/business";

jest.mock("../hooks/useMenuDragDrop", () => ({
  useMenuDragDrop: () => ({ sensors: [], handleDragEnd: jest.fn() }),
}));

jest.mock("./CategoryCard", () => ({
  CategoryCard: ({
    originalCategoryIndex,
    onAddItemOpen,
    handleEditCategory,
    handleDeleteCategory,
  }: any) => (
    <div data-testid="category" data-reorder-index={originalCategoryIndex}>
      <button onClick={() => onAddItemOpen(originalCategoryIndex)}>add</button>
      <button onClick={() => handleEditCategory(originalCategoryIndex)}>edit</button>
      <button onClick={() => handleDeleteCategory(originalCategoryIndex)}>delete</button>
    </div>
  ),
}));

import { MenuList } from "./MenuList";

it("targets the second no-id duplicate-name category for add/edit/delete/reorder", () => {
  const first: MenuCategory = { name: "Mains", description: "First", items: [] };
  const second: MenuCategory = { name: "Mains", description: "Second", items: [] };
  const projectedSecond = {
    ...second,
    sourceCategoryIndex: 1,
  } as MenuCategory & { sourceCategoryIndex: number };
  const onAdd = jest.fn();
  const onEdit = jest.fn();
  const onDelete = jest.fn();

  render(
    <MenuList
      menu={[first, second]}
      setMenu={jest.fn()}
      filteredMenu={[projectedSecond]}
      businessId={1}
      tString={(key) => key}
      searchQuery=""
      searchFilter="all"
      onAddCategoryOpen={jest.fn()}
      onAddItemOpen={onAdd}
      handleEditCategory={onEdit}
      handleDeleteCategory={onDelete}
      handleEditItem={jest.fn()}
      handleDeleteItem={jest.fn()}
      viewMode="grid"
      clearSearch={jest.fn()}
      menuVersion={1}
      setMenuVersion={jest.fn()}
      loadMenu={jest.fn()}
    />,
  );

  fireEvent.click(screen.getByText("add"));
  fireEvent.click(screen.getByText("edit"));
  fireEvent.click(screen.getByText("delete"));

  expect(onAdd).toHaveBeenCalledWith(1);
  expect(onEdit).toHaveBeenCalledWith(1);
  expect(onDelete).toHaveBeenCalledWith(1);
  expect(screen.getByTestId("category")).toHaveAttribute("data-reorder-index", "1");
});

it("shows retry — not the empty-catalog CTA — when a fetch flake leaves no categories (#773)", () => {
  const loadMenu = jest.fn();
  render(
    <MenuList
      menu={[]}
      setMenu={jest.fn()}
      filteredMenu={[]}
      businessId={86}
      tString={(key) => key}
      searchQuery=""
      searchFilter="all"
      onAddCategoryOpen={jest.fn()}
      onAddItemOpen={jest.fn()}
      handleEditCategory={jest.fn()}
      handleDeleteCategory={jest.fn()}
      handleEditItem={jest.fn()}
      handleDeleteItem={jest.fn()}
      viewMode="grid"
      clearSearch={jest.fn()}
      menuVersion={1}
      setMenuVersion={jest.fn()}
      loadMenu={loadMenu}
      loadFailed
      currentViewLanguage="es"
    />,
  );

  expect(screen.getByTestId("menu-load-failed")).toBeInTheDocument();
  expect(screen.queryByText("noCategories")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: /retryLoad/ }));
  expect(loadMenu).toHaveBeenCalledWith("es");
});
