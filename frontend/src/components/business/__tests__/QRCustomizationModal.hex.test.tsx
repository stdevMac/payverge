/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";

const mockToastError = jest.fn();

jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: {
    success: jest.fn(),
    error: (...a: unknown[]) => mockToastError(...a),
  },
}));

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => key,
}));

jest.mock("@/api/uploads", () => ({
  uploadFile: jest.fn(),
}));

// Capture the colors the preview is actually rendered with.
const previewProps: Array<Record<string, unknown>> = [];
jest.mock("../QRCodeWithText", () => ({
  __esModule: true,
  default: (props: Record<string, unknown>) => {
    previewProps.push(props);
    return <div data-testid="qr-preview" />;
  },
}));

import QRCustomizationModal from "../QRCustomizationModal";

const NS = "businessDashboard.dashboard.tableManager.qrCustomization";

const table = {
  id: 1,
  name: "Table 1",
  table_code: "abc",
  qr_code: "",
  is_active: true,
  qr_foreground_color: "#000000",
  qr_background_color: "#FFFFFF",
};

const setup = (onSave = jest.fn(), onClose = jest.fn()) => {
  previewProps.length = 0;
  render(
    <QRCustomizationModal
      isOpen
      onClose={onClose}
      table={table}
      businessId={1}
      businessName="Acme"
      onSave={onSave}
    />,
  );
  return onSave;
};

beforeEach(() => jest.clearAllMocks());

it("feeds the preview a valid fallback instead of an invalid hex (F29)", () => {
  setup();
  const fgInput = screen.getByPlaceholderText("#000000");
  fireEvent.change(fgInput, { target: { value: "not-a-hex" } });

  const last = previewProps[previewProps.length - 1];
  // Preview must not receive the garbage value — it falls back to a valid hex.
  expect(last.foregroundColor).toBe("#000000");
});

it("blocks save and toasts when a hex is invalid (F29)", () => {
  const onSave = setup();
  const bgInput = screen.getByPlaceholderText("#FFFFFF");
  fireEvent.change(bgInput, { target: { value: "###" } });

  // The Save button must reject the invalid value rather than persist it.
  const saveBtn = screen.getByText(`${NS}.save`).closest("button");
  expect(saveBtn).toBeDisabled();
});

it("saves a valid hex normally (F29 regression)", () => {
  const onSave = setup();
  const fgInput = screen.getByPlaceholderText("#000000");
  fireEvent.change(fgInput, { target: { value: "#1a6b6a" } });

  const saveBtn = screen.getByText(`${NS}.save`).closest("button");
  expect(saveBtn).not.toBeDisabled();
  fireEvent.click(saveBtn!);
  expect(onSave).toHaveBeenCalledWith(
    expect.objectContaining({ qr_foreground_color: "#1a6b6a" }),
    false,
  );
});

it("does NOT close itself on save — the parent owns closing on success (R3-OK)", () => {
  const onSave = jest.fn();
  const onClose = jest.fn();
  setup(onSave, onClose);

  const saveBtn = screen.getByText(`${NS}.save`).closest("button");
  fireEvent.click(saveBtn!);

  // handleSave used to call onClose() synchronously, killing isSaving and the
  // partial-failure retry. The modal must stay open until the parent closes it.
  expect(onSave).toHaveBeenCalledTimes(1);
  expect(onClose).not.toHaveBeenCalled();
});
