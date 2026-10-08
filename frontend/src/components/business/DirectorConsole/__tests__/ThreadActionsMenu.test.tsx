/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import ThreadActionsMenu from "../ThreadActionsMenu";

describe("ThreadActionsMenu", () => {
  it("opens menu and triggers pin", async () => {
    const onPin = jest.fn();
    render(
      <ThreadActionsMenu
        title="hi"
        pinned={false}
        onRename={jest.fn()}
        onPinToggle={onPin}
        onArchive={jest.fn()}
        onExport={jest.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /thread actions/i }));
    const pinItem = await screen.findByText(/^Pin$/);
    fireEvent.click(pinItem);
    await waitFor(() => {
      expect(onPin).toHaveBeenCalled();
    });
  });

  it("rename modal updates title", async () => {
    const onRename = jest.fn();
    render(
      <ThreadActionsMenu
        title="old"
        pinned={false}
        onRename={onRename}
        onPinToggle={jest.fn()}
        onArchive={jest.fn()}
        onExport={jest.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /thread actions/i }));
    const renameItem = await screen.findByText(/^Rename$/);
    fireEvent.click(renameItem);
    const input = await screen.findByDisplayValue("old");
    fireEvent.change(input, { target: { value: "new" } });
    fireEvent.click(screen.getByText(/^Save$/));
    await waitFor(() => {
      expect(onRename).toHaveBeenCalledWith("new");
    });
  });

  it("archive fires immediately (reversible, no confirm)", async () => {
    const onArchive = jest.fn();
    render(
      <ThreadActionsMenu
        title="hi"
        pinned={false}
        onRename={jest.fn()}
        onPinToggle={jest.fn()}
        onArchive={onArchive}
        onDelete={jest.fn()}
        onExport={jest.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /thread actions/i }));
    const archiveItem = await screen.findByText(/^Archive$/);
    fireEvent.click(archiveItem);
    await waitFor(() => {
      expect(onArchive).toHaveBeenCalled();
    });
  });

  it("delete requires confirmation before firing onDelete", async () => {
    const onDelete = jest.fn();
    render(
      <ThreadActionsMenu
        title="hi"
        pinned={false}
        onRename={jest.fn()}
        onPinToggle={jest.fn()}
        onArchive={jest.fn()}
        onDelete={onDelete}
        onExport={jest.fn()}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: /thread actions/i }));
    const deleteItem = await screen.findByText(/Delete permanently/i);
    fireEvent.click(deleteItem);
    // onDelete must NOT fire on the menu click — a confirmation must appear first.
    expect(onDelete).not.toHaveBeenCalled();
    const confirmBtn = await screen.findByRole("button", { name: /^Delete$/i });
    fireEvent.click(confirmBtn);
    await waitFor(() => {
      expect(onDelete).toHaveBeenCalled();
    });
  });
});
