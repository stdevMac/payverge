/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";
import { computeAccessibleName } from "dom-accessibility-api";
import { Input } from "@nextui-org/react";
import { AccessibleInput } from "./AccessibleInput";
import { AccessibleTextarea } from "./AccessibleTextarea";
import PasswordField from "@/components/auth/PasswordField";

describe("NextUI labeled fields expose a single accessible name (#394)", () => {
  it("raw NextUI Input currently doubles the visible label (library defect)", () => {
    const { container } = render(
      <Input type="email" label="Email" placeholder="Enter your email" />,
    );
    const input = container.querySelector("input");
    expect(input).toBeTruthy();
    const name = computeAccessibleName(input as HTMLElement);
    // NextUI 2.4 composes aria-label + labelledby. If a later release stops
    // doubling, this canary may start seeing "Email" — that is progress, not
    // a product regression; AccessibleInput still must emit a single name.
    expect(name === "Email" || name === "Email Email").toBe(true);
  });

  it("AccessibleTextarea keeps the visible Notes label once", () => {
    render(<AccessibleTextarea label="Notes" placeholder="Anything else" />);
    expect(screen.getByRole("textbox", { name: /^Notes$/ })).toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: /Notes Notes/ })).toBeNull();
  });

  it("AccessibleInput keeps the visible Email label once", () => {
    render(
      <AccessibleInput type="email" label="Email" placeholder="Enter your email" />,
    );
    expect(screen.getByRole("textbox", { name: /^Email$/ })).toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: /Email Email/ })).toBeNull();
  });

  it.each([
    ["Full Name", "Enter your name"],
    ["Invite code", "Enter your invite code"],
    ["Correo", "Ingresa tu correo"],
    ["Nombre completo", "Ingresá tu nombre"],
    ["Código de invitación", "Ingresá tu código de invitación"],
  ])("names %s exactly once", (label, placeholder) => {
    render(<AccessibleInput type="text" label={label} placeholder={placeholder} />);
    expect(screen.getByRole("textbox", { name: new RegExp(`^${label}$`) })).toBeInTheDocument();
  });

  it("PasswordField wiring names Password exactly once", () => {
    const { container } = render(
      <PasswordField
        label="Password"
        value=""
        onChange={jest.fn()}
        autoComplete="current-password"
        showLabel="Show password"
        hideLabel="Hide password"
        placeholder="Enter your password"
      />,
    );
    const input = container.querySelector('input[type="password"]');
    expect(input).toBeTruthy();
    expect(computeAccessibleName(input as HTMLElement)).toBe("Password");
    expect(screen.getByLabelText(/^Password$/)).toBe(input);
  });

  it("PasswordField Spanish label is announced once", () => {
    const { container } = render(
      <PasswordField
        label="Contraseña"
        value=""
        onChange={jest.fn()}
        autoComplete="current-password"
        showLabel="Mostrar contraseña"
        hideLabel="Ocultar contraseña"
        placeholder="Ingresa tu contraseña"
      />,
    );
    const input = container.querySelector('input[type="password"]');
    expect(computeAccessibleName(input as HTMLElement)).toBe("Contraseña");
  });
});
