/** @jest-environment jsdom */
import fs from "fs";
import path from "path";
import { render } from "@testing-library/react";
import { AccessibleInput } from "./AccessibleInput";

describe("AccessibleInput keeps LTR strings LTR inside an RTL page", () => {
  it.each(["email", "tel", "url"])("sets dir=ltr on type=%s", (type) => {
    const { container } = render(
      <div dir="rtl">
        <AccessibleInput type={type} label="Field" />
      </div>,
    );
    expect(container.querySelector("input")).toHaveAttribute("dir", "ltr");
  });

  it("leaves free-text inputs to inherit the page direction", () => {
    const { container } = render(
      <div dir="rtl">
        <AccessibleInput type="text" label="Name" />
      </div>,
    );
    expect(container.querySelector("input")).not.toHaveAttribute("dir");
  });

  it("respects an explicit dir", () => {
    const { container } = render(
      <AccessibleInput type="email" dir="auto" label="Email" />,
    );
    expect(container.querySelector("input")).toHaveAttribute("dir", "auto");
  });
});

describe("guest surfaces use logical direction (RTL locales)", () => {
  const src = (rel: string) =>
    fs.readFileSync(path.resolve(__dirname, "../..", rel), "utf8");

  it("customer auth email input is dir=ltr", () => {
    const modal = src("components/customer/CustomerAuthModal.tsx");
    expect(modal).toMatch(/type="email"\s+dir="ltr"/);
  });

  it("menu back arrow mirrors and the cart badge / bundle price use logical sides", () => {
    const page = src("app/t/[tableCode]/menu/page.tsx");
    expect(page).toMatch(/<ArrowLeft className="h-5 w-5 rtl:rotate-180"/);
    expect(page).not.toMatch(/-right-1\.5/);
    expect(page).not.toMatch(/className="text-right"/);
  });
});
