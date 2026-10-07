/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { PluginConfigFooter } from "./PluginConfigFooter";

describe("PluginConfigFooter (L6-31)", () => {
  it("shows Enable when plugin is not yet enabled", () => {
    render(
      <PluginConfigFooter
        onSave={jest.fn()}
        onCancel={jest.fn()}
        isEnabled={false}
        cancelLabel="Cancel"
        saveLabel="Save"
        enableLabel="Enable"
      />,
    );
    expect(screen.getByRole("button", { name: "Enable" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
  });

  it("shows Save when plugin is already enabled", () => {
    render(
      <PluginConfigFooter
        onSave={jest.fn()}
        onCancel={jest.fn()}
        isEnabled
        cancelLabel="Cancel"
        saveLabel="Save"
        enableLabel="Enable"
      />,
    );
    expect(screen.getByRole("button", { name: "Save" })).toBeInTheDocument();
  });

  it("invokes onSave / onCancel", () => {
    const onSave = jest.fn();
    const onCancel = jest.fn();
    render(
      <PluginConfigFooter
        onSave={onSave}
        onCancel={onCancel}
        isEnabled={false}
        cancelLabel="Cancel"
        saveLabel="Save"
        enableLabel="Enable"
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Enable" }));
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onSave).toHaveBeenCalledTimes(1);
    expect(onCancel).toHaveBeenCalledTimes(1);
  });
});
