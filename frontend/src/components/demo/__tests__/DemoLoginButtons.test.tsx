/** @jest-environment jsdom */
const mockDemoLogin = jest.fn();
jest.mock("@/api/demo", () => ({
  demoLogin: (...args: unknown[]) => mockDemoLogin(...args),
}));

import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import DemoLoginButtons from "../DemoLoginButtons";

afterEach(() => mockDemoLogin.mockReset());

describe("DemoLoginButtons", () => {
  it("offers owner, kitchen and waiter entries and no password field", () => {
    const { container } = render(<DemoLoginButtons onOwnerSignedIn={jest.fn()} onStaffSignedIn={jest.fn()} />);
    expect(screen.getByRole("button", { name: "Enter demo as Owner" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Enter demo as Staff (kitchen)" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Enter demo as Staff (waiter)" })).toBeInTheDocument();
    expect(container.querySelector("input")).toBeNull();
  });

  it("signs in as owner and hands over the redirect", async () => {
    mockDemoLogin.mockResolvedValue({ kind: "owner", redirect: "/dashboard" });
    const onOwner = jest.fn();
    render(<DemoLoginButtons onOwnerSignedIn={onOwner} onStaffSignedIn={jest.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "Enter demo as Owner" }));
    await waitFor(() => expect(onOwner).toHaveBeenCalledWith("/dashboard"));
    expect(mockDemoLogin).toHaveBeenCalledWith("owner");
  });

  it.each([
    ["Enter demo as Staff (kitchen)", "kitchen"],
    ["Enter demo as Staff (waiter)", "waiter"],
  ])("%s signs in the %s staff member", async (label, role) => {
    const staff = { id: 3, name: "S", email: "s@x", role: "server", business_id: 7 };
    mockDemoLogin.mockResolvedValue({ kind: "staff", staff });
    const onStaff = jest.fn();
    render(<DemoLoginButtons onOwnerSignedIn={jest.fn()} onStaffSignedIn={onStaff} />);
    fireEvent.click(screen.getByRole("button", { name: label }));
    await waitFor(() => expect(onStaff).toHaveBeenCalledWith(staff));
    expect(mockDemoLogin).toHaveBeenCalledWith(role);
  });

  it("shows the server error and re-enables the buttons", async () => {
    mockDemoLogin.mockRejectedValue(new Error("The demo is still being prepared. Try again in a minute."));
    render(<DemoLoginButtons onOwnerSignedIn={jest.fn()} onStaffSignedIn={jest.fn()} />);
    fireEvent.click(screen.getByRole("button", { name: "Enter demo as Owner" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("still being prepared");
    expect(screen.getByRole("button", { name: "Enter demo as Owner" })).not.toBeDisabled();
  });
});
