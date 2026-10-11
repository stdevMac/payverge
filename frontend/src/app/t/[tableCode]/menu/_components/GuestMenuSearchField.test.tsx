/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { GuestMenuSearchField } from "./GuestMenuSearchField";

jest.mock("@nextui-org/react", () => ({
  Input: ({
    "aria-label": ariaLabel,
    placeholder,
    value,
    onChange,
  }: {
    "aria-label"?: string;
    placeholder?: string;
    value?: string;
    onChange?: (event: React.ChangeEvent<HTMLInputElement>) => void;
  }) => (
    <input
      aria-label={ariaLabel}
      placeholder={placeholder}
      value={value}
      onChange={onChange}
    />
  ),
}));

jest.mock("lucide-react", () => ({
  Search: () => <span />,
}));

describe("GuestMenuSearchField", () => {
  it("gives the search textbox an accessible name", () => {
    render(
      <GuestMenuSearchField
        value=""
        onChange={() => undefined}
        label="Search menu items..."
      />,
    );

    expect(
      screen.getByRole("textbox", { name: "Search menu items..." }),
    ).toBeInTheDocument();
  });
});
