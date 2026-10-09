/**
 * @jest-environment jsdom
 */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { PropertiesPanel } from "../PropertiesPanel";
import type { EditorDocument } from "../types";

jest.mock("@nextui-org/react", () => {
  const React = require("react");
  return {
    Input: ({
      label,
      value,
      onValueChange,
      type,
      isInvalid,
      errorMessage,
      ...rest
    }: {
      label?: string;
      value?: string;
      onValueChange?: (v: string) => void;
      type?: string;
      isInvalid?: boolean;
      errorMessage?: React.ReactNode;
    } & Record<string, unknown>) => {
      const testId = (rest as { "data-testid"?: string })["data-testid"];
      return (
        <label>
          {label}
          <input
            aria-label={label}
            type={type || "text"}
            value={value ?? ""}
            aria-invalid={isInvalid ? "true" : undefined}
            data-testid={testId}
            onChange={(e) => onValueChange?.(e.target.value)}
          />
          {errorMessage ? (
            <span role="alert" data-testid={`${testId ?? label}-error`}>
              {errorMessage}
            </span>
          ) : null}
        </label>
      );
    },
    Switch: () => null,
    Select: ({
      label,
      children,
      onSelectionChange,
      selectedKeys,
    }: {
      label?: string;
      children?: React.ReactNode;
      onSelectionChange?: (keys: Set<string>) => void;
      selectedKeys?: string[];
    }) => (
      <label>
        {label}
        <select
          aria-label={label}
          value={selectedKeys?.[0] ?? ""}
          onChange={(e) =>
            onSelectionChange?.(new Set([e.target.value]))
          }
        >
          {children}
        </select>
      </label>
    ),
    SelectItem: ({
      children,
      value,
      ...rest
    }: {
      children?: React.ReactNode;
      value?: string;
      // SelectItem also receives `key` from React
    } & Record<string, unknown>) => (
      <option value={(rest as { key?: string }).key ?? value}>{children}</option>
    ),
  };
});

const t = (key: string) => key;

const baseDoc: EditorDocument = {
  schema_version: 1,
  width_mm: 10000,
  height_mm: 8000,
  measurement_unit: "m",
  tables: [
    {
      clientKey: "t1",
      table_id: 42,
      name: "T1",
      x_mm: 1000,
      y_mm: 1000,
      width_mm: 1000,
      height_mm: 1000,
      shape: "square",
      max_capacity: 4,
      min_capacity: 2,
      visible_seat_count: 4,
      rotation_deg: 0,
    },
  ],
  elements: [],
  regions: [],
};

describe("PropertiesPanel table edit", () => {
  it("shows empty state when nothing selected", () => {
    render(
      <PropertiesPanel
        t={t}
        doc={baseDoc}
        selection={[]}
        onUpdateTable={jest.fn()}
        onUpdateElement={jest.fn()}
        onUpdateRegion={jest.fn()}
      />,
    );
    expect(screen.getByTestId("properties-panel")).toBeInTheDocument();
    expect(
      screen.getByText("editor.properties.empty"),
    ).toBeInTheDocument();
  });

  it("edits table name and capacity", () => {
    const onUpdateTable = jest.fn();
    render(
      <PropertiesPanel
        t={t}
        doc={baseDoc}
        selection={[{ kind: "table", key: "t1" }]}
        onUpdateTable={onUpdateTable}
        onUpdateElement={jest.fn()}
        onUpdateRegion={jest.fn()}
        tableQrLinks={{ 42: "https://payverge.io/t/abc" }}
      />,
    );

    expect(screen.getByTestId("properties-panel")).toBeInTheDocument();
    const nameInput = screen.getByLabelText("editor.properties.name");
    fireEvent.change(nameInput, { target: { value: "Patio-1" } });
    expect(onUpdateTable).toHaveBeenCalledWith("t1", { name: "Patio-1" });

    const maxSeats = screen.getByLabelText("editor.properties.maxSeats");
    fireEvent.change(maxSeats, { target: { value: "8" } });
    expect(onUpdateTable).toHaveBeenCalledWith(
      "t1",
      expect.objectContaining({ max_capacity: 8 }),
    );

    const rotation = screen.getByLabelText("editor.properties.rotation");
    fireEvent.change(rotation, { target: { value: "15" } });
    expect(onUpdateTable).toHaveBeenCalledWith("t1", { rotation_deg: 15 });
  });

  // L3-24: legacy/server documents can carry out-of-range capacities the
  // clamped onValueChange never produces. Both seat inputs must explain why
  // they are red, not just turn red.
  const renderWithCapacity = (
    capacity: { min_capacity?: number; max_capacity?: number },
  ) =>
    render(
      <PropertiesPanel
        t={t}
        doc={{
          ...baseDoc,
          tables: [{ ...baseDoc.tables[0], ...capacity }],
        }}
        selection={[{ kind: "table", key: "t1" }]}
        onUpdateTable={jest.fn()}
        onUpdateElement={jest.fn()}
        onUpdateRegion={jest.fn()}
      />,
    );

  it("explains an out-of-range max seats value (L3-24)", () => {
    renderWithCapacity({ min_capacity: 2, max_capacity: 0 });

    expect(screen.getByTestId("props-max-capacity")).toHaveAttribute(
      "aria-invalid",
      "true",
    );
    expect(screen.getByTestId("props-max-capacity-error")).toHaveTextContent(
      "editor.properties.capacityRange",
    );
    // min (2) now exceeds max (0) — that is a different failure mode.
    expect(screen.getByTestId("props-min-capacity-error")).toHaveTextContent(
      "editor.properties.capacityMinMax",
    );
  });

  it("explains an out-of-range min seats value (L3-24)", () => {
    renderWithCapacity({ min_capacity: 0, max_capacity: 4 });

    expect(screen.getByTestId("props-min-capacity")).toHaveAttribute(
      "aria-invalid",
      "true",
    );
    expect(screen.getByTestId("props-min-capacity-error")).toHaveTextContent(
      "editor.properties.capacityRange",
    );
    expect(
      screen.queryByTestId("props-max-capacity-error"),
    ).not.toBeInTheDocument();
  });
});
