/** @jest-environment jsdom */

import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import MenuSanitizationReview from "../MenuSanitizationReview";
import type { MenuSanitizeReport } from "@/api/business";

const report: MenuSanitizeReport = {
  dropped_allergens: 1,
  dropped_dietary_tags: 1,
  dropped_items: 1,
  retained: [
    {
      category_index: 0,
      category_name: "Mains",
      item_index: 0,
      item_name: "Taco",
      field: "allergens",
      value: "peanuts",
      canonical_value: "peanut",
      reason: "alias_normalized",
    },
  ],
  dropped: [
    {
      category_index: 0,
      category_name: "Mains",
      item_index: 0,
      item_name: "Taco",
      field: "allergens",
      value: "unicorn_dust",
      reason: "unknown_enum_value",
    },
    {
      category_index: 0,
      category_name: "Mains",
      item_index: 0,
      item_name: "Taco",
      field: "dietary_tags",
      value: "keto",
      reason: "unknown_enum_value",
    },
    {
      category_index: 0,
      category_name: "Mains",
      item_index: 1,
      item_name: "Free sample",
      field: "price",
      value: "0",
      reason: "price_out_of_range",
    },
  ],
};

const translate = (key: string) =>
  ({
    "sanitization.title": "Review menu changes",
    "sanitization.description": "Some values cannot be imported.",
    "sanitization.dropped": "Removed",
    "sanitization.normalized": "Kept after normalization",
    "sanitization.fields.allergens": "Allergen",
    "sanitization.fields.dietary_tags": "Dietary tag",
    "sanitization.fields.price": "Item",
    "sanitization.reasons.unknown_enum_value": "Unsupported value",
    "sanitization.reasons.price_out_of_range": "Price outside the allowed range",
    "sanitization.reasons.alias_normalized": "Matched a supported value",
    "sanitization.confirm": "Confirm and import",
    "sanitization.back": "Back to editing",
  })[key] ?? key;

describe("MenuSanitizationReview", () => {
  it("shows dropped allergen, tag, item, and retained canonical value before confirmation", () => {
    render(
      <MenuSanitizationReview
        report={report}
        translate={translate}
        onConfirm={jest.fn()}
        onBack={jest.fn()}
        isConfirming={false}
      />,
    );

    expect(screen.getByText("unicorn_dust")).toBeInTheDocument();
    expect(screen.getByText("keto")).toBeInTheDocument();
    expect(screen.getByText("Free sample")).toBeInTheDocument();
    expect(screen.getByText(/peanuts/)).toHaveTextContent("peanuts → peanut");
    expect(screen.getByRole("button", { name: "Confirm and import" })).toBeEnabled();
  });

  it("only confirms through the explicit confirmation action", () => {
    const onConfirm = jest.fn();
    render(
      <MenuSanitizationReview
        report={report}
        translate={translate}
        onConfirm={onConfirm}
        onBack={jest.fn()}
        isConfirming={false}
      />,
    );

    expect(onConfirm).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Confirm and import" }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });
});
