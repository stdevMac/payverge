/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";
import StaffLogin from "./StaffLogin";
import { SimpleTranslationProvider } from "@/i18n/SimpleTranslationProvider";

jest.mock("../../api/staff", () => ({
  requestLoginCode: jest.fn(),
  verifyLoginCode: jest.fn(),
  getStaffGoogleAuthURL: jest.fn(),
  isMembershipSelectionResponse: () => false,
}));
jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: { error: jest.fn(), success: jest.fn() },
  toast: { error: jest.fn(), success: jest.fn() },
}));
jest.mock("@/utils/staffAuth", () => ({
  setStaffData: jest.fn(),
}));

describe("StaffLogin field accessible names (#394)", () => {
  it("announces the email field once in English", () => {
    render(
      <SimpleTranslationProvider initialLocale="en">
        <StaffLogin />
      </SimpleTranslationProvider>,
    );
    expect(
      screen.getByRole("textbox", { name: /^Enter your email$/ }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("textbox", { name: /Enter your email Enter your email/ }),
    ).toBeNull();
  });

  it("announces the email field once in Spanish", () => {
    render(
      <SimpleTranslationProvider initialLocale="es">
        <StaffLogin />
      </SimpleTranslationProvider>,
    );
    expect(
      screen.getByRole("textbox", { name: /^Ingresa tu correo electrónico$/ }),
    ).toBeInTheDocument();
  });
});
