/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { Pencil, Trash2 } from "lucide-react";

import RowActionsMenu, { type RowActionItem } from "./RowActionsMenu";

const items: RowActionItem[] = [
  { key: "edit", label: "Edit", icon: <Pencil data-testid="icon-edit" /> },
  {
    key: "delete",
    label: "Delete",
    icon: <Trash2 data-testid="icon-delete" />,
    tone: "danger",
  },
  { key: "archive", label: "Archive", disabled: true },
];

describe("RowActionsMenu", () => {
  it("opens on trigger and renders action items", async () => {
    render(
      <RowActionsMenu
        aria-label="Row actions"
        items={items}
        onAction={jest.fn()}
      />,
    );

    const trigger = screen.getByRole("button", { name: "Row actions" });
    fireEvent.click(trigger);

    expect(await screen.findByRole("menuitem", { name: "Edit" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Delete" })).toBeInTheDocument();
    expect(screen.getByRole("menuitem", { name: "Archive" })).toBeInTheDocument();
    expect(screen.getByTestId("icon-edit")).toBeInTheDocument();
    expect(screen.getByTestId("icon-delete")).toBeInTheDocument();
  });

  it("fires onAction with the selected item key", async () => {
    const onAction = jest.fn();
    render(
      <RowActionsMenu
        aria-label="Row actions"
        items={items}
        onAction={onAction}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Row actions" }));
    fireEvent.click(await screen.findByRole("menuitem", { name: "Edit" }));

    expect(onAction).toHaveBeenCalledTimes(1);
    expect(onAction).toHaveBeenCalledWith("edit");
  });

  it("applies danger classes only to danger-tone items", async () => {
    render(
      <RowActionsMenu
        aria-label="Row actions"
        items={items}
        onAction={jest.fn()}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Row actions" }));

    const deleteItem = await screen.findByRole("menuitem", { name: "Delete" });
    const editItem = screen.getByRole("menuitem", { name: "Edit" });

    expect(deleteItem.className).toMatch(/text-rose/);
    expect(editItem.className).not.toMatch(/text-rose/);
  });

  it("stops propagation so parent row click handlers do not fire", () => {
    const onRowClick = jest.fn();
    const onAction = jest.fn();

    render(
      <div
        onClick={onRowClick}
        onKeyDown={() => undefined}
        role="button"
        tabIndex={0}
        data-testid="row"
      >
        <RowActionsMenu
          aria-label="Row actions"
          items={items}
          onAction={onAction}
        />
      </div>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Row actions" }));
    expect(onRowClick).not.toHaveBeenCalled();
  });

  it("accepts bottom-end placement without crashing", () => {
    render(
      <RowActionsMenu
        aria-label="Row actions"
        items={items}
        placement="bottom-end"
        onAction={jest.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "Row actions" })).toBeInTheDocument();
  });

  it("portals the menu with a high z-index so overflow parents cannot hide it", async () => {
    render(
      <div className="overflow-hidden">
        <RowActionsMenu
          aria-label="Row actions"
          items={items}
          onAction={jest.fn()}
        />
      </div>,
    );

    fireEvent.click(screen.getByRole("button", { name: "Row actions" }));
    const edit = await screen.findByRole("menuitem", { name: "Edit" });
    const menu = edit.closest("[role='menu']");
    expect(menu).not.toBeNull();
    expect(menu).toHaveAttribute("data-testid", "row-actions-menu");
    expect(menu?.className).toMatch(/z-\[100\]/);
  });

  it("opens a menu wide enough for long action labels", async () => {
    render(
      <RowActionsMenu
        aria-label="Row actions"
        items={[
          { key: "close", label: "Close without payment" },
          { key: "print", label: "Print" },
        ]}
        placement="bottom-end"
        onAction={jest.fn()}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Row actions" }));
    const closeItem = await screen.findByRole("menuitem", {
      name: "Close without payment",
    });
    expect(closeItem).toHaveTextContent("Close without payment");
    expect(screen.getByRole("menuitem", { name: "Print" })).toBeInTheDocument();
  });

  it("closes on Escape", async () => {
    render(
      <RowActionsMenu
        aria-label="Row actions"
        items={items}
        onAction={jest.fn()}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Row actions" }));
    expect(await screen.findByRole("menuitem", { name: "Edit" })).toBeInTheDocument();

    fireEvent.keyDown(document, { key: "Escape" });

    expect(screen.queryByRole("menuitem", { name: "Edit" })).not.toBeInTheDocument();
  });

  it("closes on Escape when the trigger still has focus", async () => {
    render(
      <RowActionsMenu
        aria-label="Row actions"
        items={items}
        onAction={jest.fn()}
      />,
    );

    const trigger = screen.getByRole("button", { name: "Row actions" });
    fireEvent.click(trigger);
    expect(await screen.findByRole("menuitem", { name: "Edit" })).toBeInTheDocument();

    fireEvent.keyDown(trigger, { key: "Escape" });

    expect(screen.queryByRole("menuitem", { name: "Edit" })).not.toBeInTheDocument();
  });

  it("positions the portaled menu below the trigger", async () => {
    render(
      <RowActionsMenu
        aria-label="Row actions"
        items={items}
        placement="bottom-end"
        onAction={jest.fn()}
      />,
    );

    const trigger = screen.getByRole("button", { name: "Row actions" });
    const triggerWrap = trigger.parentElement as HTMLElement;
    jest.spyOn(triggerWrap, "getBoundingClientRect").mockReturnValue({
      x: 700,
      y: 180,
      top: 180,
      left: 700,
      bottom: 212,
      right: 732,
      width: 32,
      height: 32,
      toJSON: () => ({}),
    });

    fireEvent.click(trigger);
    const edit = await screen.findByRole("menuitem", { name: "Edit" });
    const menu = edit.closest("[role='menu']") as HTMLElement;
    expect(menu).toHaveStyle({ top: "216px" });
    expect(Number.parseFloat(menu.style.top)).toBeGreaterThanOrEqual(212);
  });

  it("does not fire onAction for disabled items", async () => {
    const onAction = jest.fn();
    render(
      <RowActionsMenu
        aria-label="Row actions"
        items={items}
        onAction={onAction}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Row actions" }));
    const archive = await screen.findByRole("menuitem", { name: "Archive" });
    fireEvent.click(archive);

    expect(onAction).not.toHaveBeenCalled();
  });
});
