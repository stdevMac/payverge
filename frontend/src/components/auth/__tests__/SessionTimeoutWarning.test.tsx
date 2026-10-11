/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import SessionTimeoutWarning from "../SessionTimeoutWarning";

describe("SessionTimeoutWarning", () => {
  it("renders nothing when not expired", () => {
    const { container } = render(
      <SessionTimeoutWarning isExpired={false} onLogin={jest.fn()} />,
    );
    expect(container.firstChild).toBeNull();
  });

  it("renders expired modal when isExpired is true", () => {
    render(<SessionTimeoutWarning isExpired={true} onLogin={jest.fn()} />);
    expect(screen.getByText(/session expired/i)).toBeTruthy();
    expect(screen.getByRole("button", { name: /log in again/i })).toBeTruthy();
  });

  it("calls onLogin when login button clicked", () => {
    const onLogin = jest.fn();
    render(<SessionTimeoutWarning isExpired={true} onLogin={onLogin} />);
    fireEvent.click(screen.getByRole("button", { name: /log in again/i }));
    expect(onLogin).toHaveBeenCalledTimes(1);
  });
});
