/** @jest-environment jsdom */
import React from "react";
import { readFileSync } from "fs";
import { join } from "path";
import { render, screen } from "@testing-library/react";
import NotFoundClient from "./NotFoundClient";

jest.mock("next/image", () => ({
  __esModule: true,
  default: (props: { alt: string }) => (
    // eslint-disable-next-line @next/next/no-img-element
    <img alt={props.alt} />
  ),
}));

jest.mock("next/link", () => ({
  __esModule: true,
  default: ({
    children,
    href,
  }: {
    children: React.ReactNode;
    href: string;
  }) => <a href={href}>{children}</a>,
}));

const mockNotFoundCopy: Record<string, string> = {
  "notFound.title": "Page not found",
  "notFound.body": "The page you're looking for doesn't exist or has moved.",
  "notFound.backToHome": "Back to home",
  "notFound.contactSupport": "Contact support",
};

// NotFoundClient reads the slim operator-chrome catalog.
jest.mock("@/i18n/OperatorLocaleProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
}));

jest.mock("@/i18n/operatorChromeCatalog", () => ({
  getChromeTranslation: (key: string) => mockNotFoundCopy[key] ?? key,
}));

const FOCUSABLE_MAIN_SKIP_TARGET =
  /<main[\s\S]{0,240}id="main-content"[\s\S]{0,120}tabIndex=\{-1\}/;

describe("root not-found skip-link target", () => {
  it("declares a focusable main skip target in not-found.tsx or NotFoundClient", () => {
    const source = [
      readFileSync(join(__dirname, "not-found.tsx"), "utf8"),
      readFileSync(join(__dirname, "NotFoundClient.tsx"), "utf8"),
    ].join("\n");
    expect(source).toMatch(FOCUSABLE_MAIN_SKIP_TARGET);
  });

  it("renders one main landmark at #main-content that can receive skip focus", () => {
    render(<NotFoundClient />);

    const main = screen.getByRole("main");
    expect(main).toHaveAttribute("id", "main-content");
    expect(main).toHaveAttribute("tabindex", "-1");
    expect(screen.getByRole("heading", { name: "Page not found" })).toBe(
      main.querySelector("h1"),
    );
    expect(document.querySelectorAll("main#main-content")).toHaveLength(1);
  });
});
