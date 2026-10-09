/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import MenuSanitizationReview from "../MenuSanitizationReview";
import type { MenuSanitizeReport } from "@/api/business";

const report: MenuSanitizeReport = {
  dropped_allergens: 1,
  dropped_dietary_tags: 0,
  dropped_items: 0,
  dropped: [
    {
      category_index: 0,
      category_name: "Mains",
      field: "name",
      item_index: 0,
      value: "x",
      item_name: "x",
      reason: "unknown_enum_value",
    },
  ],
  retained: [],
};

describe("MenuSanitizationReview L3-3", () => {
  it("closes on document Escape via useDialogKeyboard (Root C)", () => {
    const onBack = jest.fn();
    render(
      <MenuSanitizationReview
        report={report}
        translate={(k) => k}
        onConfirm={jest.fn()}
        onBack={onBack}
        isConfirming={false}
      />,
    );
    expect(screen.getByRole("alertdialog")).toBeInTheDocument();
    (document.activeElement as HTMLElement | null)?.blur?.();
    document.dispatchEvent(
      new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
    );
    expect(onBack).toHaveBeenCalledTimes(1);
  });

  // R2-6: while the confirm request is in flight the visible Back button is
  // disabled, but Escape still reached onBack — unwinding the review out from
  // under a write the server had already accepted.
  it("ignores Escape while the confirm request is in flight", () => {
    const onBack = jest.fn();
    render(
      <MenuSanitizationReview
        report={report}
        translate={(k) => k}
        onConfirm={jest.fn()}
        onBack={onBack}
        isConfirming
      />,
    );
    expect(
      screen.getByRole("button", { name: "sanitization.back" }),
    ).toBeDisabled();

    (document.activeElement as HTMLElement | null)?.blur?.();
    document.dispatchEvent(
      new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
    );
    expect(onBack).not.toHaveBeenCalled();
  });
});
