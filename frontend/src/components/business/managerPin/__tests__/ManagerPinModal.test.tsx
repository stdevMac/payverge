/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  // Echo key (with no param substitution) so we can assert keys are used
  // instead of hardcoded English literals.
  getTranslation: (key: string) => key,
}));

import ManagerPinModal from "../ManagerPinModal";

it("renders labels through the translation helper, not hardcoded English (F18)", () => {
  render(
    <ManagerPinModal isOpen onSubmit={jest.fn()} onCancel={jest.fn()} />,
  );

  // Default title + buttons resolve to their i18n keys (echoed by the mock),
  // proving the literals are gone.
  expect(screen.getByText("managerPin.title")).toBeInTheDocument();
  expect(screen.getByText("managerPin.confirm")).toBeInTheDocument();
  expect(screen.getByText("managerPin.clear")).toBeInTheDocument();
  expect(screen.getByText("managerPin.cancel")).toBeInTheDocument();
  // No leftover hardcoded English.
  expect(screen.queryByText("Manager PIN required")).toBeNull();
  expect(screen.queryByText("Confirm")).toBeNull();
});

it("accepts hardware-keyboard digits and confirms on Enter (F18)", async () => {
  const onSubmit = jest.fn().mockResolvedValue(undefined);
  render(
    <ManagerPinModal isOpen onSubmit={onSubmit} onCancel={jest.fn()} />,
  );

  fireEvent.keyDown(window, { key: "1" });
  fireEvent.keyDown(window, { key: "2" });
  fireEvent.keyDown(window, { key: "3" });
  fireEvent.keyDown(window, { key: "4" });
  fireEvent.keyDown(window, { key: "Enter" });

  await waitFor(() => expect(onSubmit).toHaveBeenCalledWith("1234"));
});

it("does not confirm on Enter while the PIN is too short", () => {
  const onSubmit = jest.fn().mockResolvedValue(undefined);
  render(
    <ManagerPinModal isOpen onSubmit={onSubmit} onCancel={jest.fn()} />,
  );

  fireEvent.keyDown(window, { key: "1" });
  fireEvent.keyDown(window, { key: "2" });
  fireEvent.keyDown(window, { key: "Enter" });

  expect(onSubmit).not.toHaveBeenCalled();
});

it("cancels on Escape (F18)", () => {
  const onCancel = jest.fn();
  render(
    <ManagerPinModal isOpen onSubmit={jest.fn()} onCancel={onCancel} />,
  );

  fireEvent.keyDown(window, { key: "Escape" });
  expect(onCancel).toHaveBeenCalled();
});
