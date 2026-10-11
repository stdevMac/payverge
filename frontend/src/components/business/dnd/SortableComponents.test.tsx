/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";

// Mock @dnd-kit/sortable so the component can render outside a DndContext.
// `useSortable` returns identifiable listeners/attributes we can assert landed
// on the drag grip (and only the grip).
const mockListeners = { onPointerDown: jest.fn() };
jest.mock("@dnd-kit/sortable", () => ({
  useSortable: () => ({
    attributes: { "data-dnd-attr": "on", tabIndex: 0 },
    listeners: mockListeners,
    setNodeRef: jest.fn(),
    transform: null,
    transition: undefined,
    isDragging: false,
  }),
}));

import { SortableCategoryCard } from "./SortableComponents";

describe("SortableCategoryCard", () => {
  it("makes the root wrapper a positioning context (relative) so the absolute grip anchors on-card", () => {
    const { container } = render(
      <SortableCategoryCard id="cat-1" ariaLabel="Drag to reorder category" className="mb-8">
        <div>Category body</div>
      </SortableCategoryCard>,
    );
    const wrapper = container.firstElementChild as HTMLElement;
    // `relative` must be present (the bug: without it the grip drifts to a
    // distant ancestor and the category is undraggable). The incoming className
    // is preserved alongside it.
    expect(wrapper.className).toContain("relative");
    expect(wrapper.className).toContain("mb-8");
  });

  it("renders the drag handle with dnd-kit listeners/attributes and an accessible name", () => {
    render(
      <SortableCategoryCard id="cat-1" ariaLabel="Drag to reorder category">
        <div>Category body</div>
      </SortableCategoryCard>,
    );
    // The grip is the only element carrying the accessible name; querying by it
    // proves the aria-label + listeners/attributes are spread onto the handle.
    const grip = screen.getByLabelText("Drag to reorder category");
    expect(grip).toBeTruthy();
    // dnd-kit attributes were spread (from the mocked useSortable).
    expect(grip.getAttribute("data-dnd-attr")).toBe("on");
    // It's the drag handle, styled draggable.
    expect(grip.className).toContain("cursor-grab");
    // The children render inside the offset content column.
    expect(screen.getByText("Category body")).toBeTruthy();
  });
});
