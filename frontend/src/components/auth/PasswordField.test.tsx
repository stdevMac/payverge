/** @jest-environment jsdom */
import { fireEvent, render, screen } from "@testing-library/react";
import PasswordField from "./PasswordField";

describe("PasswordField", () => {
  it("has an accessible reveal control and preserves password-manager metadata", () => {
    render(
      <PasswordField
        label="Password"
        value="secret12"
        onChange={jest.fn()}
        autoComplete="current-password"
        showLabel="Show password"
        hideLabel="Hide password"
      />,
    );

    const input = screen.getByLabelText("Password");
    expect(input).toHaveAttribute("type", "password");
    expect(input).toHaveAttribute("autocomplete", "current-password");
    const reveal = screen.getByRole("button", { name: "Show password" });
    fireEvent.click(reveal);
    expect(input).toHaveAttribute("type", "text");
    expect(reveal).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "Hide password" })).toBe(reveal);
  });

  it("allows paste and announces the live minimum-length requirement", () => {
    const onChange = jest.fn();
    render(
      <PasswordField
        label="Create password"
        value="short"
        onChange={onChange}
        autoComplete="new-password"
        showLabel="Show password"
        hideLabel="Hide password"
        minimumLength={8}
        minimumLengthLabel="Use at least 8 characters"
      />,
    );

    const input = screen.getByLabelText("Create password");
    const paste = new Event("paste", { bubbles: true, cancelable: true });
    expect(input.dispatchEvent(paste)).toBe(true);
    expect(paste.defaultPrevented).toBe(false);
    expect(screen.getByRole("status")).toHaveTextContent(
      "Use at least 8 characters",
    );
  });
});
