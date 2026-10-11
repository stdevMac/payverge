/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";

const BULLETS = [
  { title: "Director Console", description: "Owner-level decisions backed by AI." },
  {
    title: "AI Waiter",
    description: "Menu-aware guest service in English and Spanish.",
  },
  {
    title: "Payments go straight to you",
    description: "Guests pay on the rails you connect; nothing sits in between.",
  },
];

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en" }),
  getTranslation: (key: string) => {
    if (key === "dashboard.authentication.valueProp.title") return "What's inside";
    if (key === "dashboard.authentication.valueProp.imageAlt") {
      return "Payverge analytics dashboard";
    }
    if (key === "dashboard.authentication.valueProp.bullets") return BULLETS;
    return key;
  },
}));

jest.mock("next/image", () => ({
  __esModule: true,
  default: ({ fill, priority, ...props }: { fill?: boolean; priority?: boolean; alt?: string; src?: string }) =>
    require("react").createElement("img", {
      ...props,
      alt: props.alt ?? "",
    }),
}));

import { AuthValueProp, AuthValuePropCompact } from "./AuthValueProp";

describe("AuthValueProp", () => {
  it("renders a real product crop instead of an empty beige slab", () => {
    const { container } = render(<AuthValueProp />);
    const img = screen.getByRole("img", { name: "Payverge analytics dashboard" });
    expect(img).toHaveAttribute("src", "/onboarding/analytics.webp");
    expect(container.querySelector('[aria-hidden]:not(svg)')).toBeNull();
    expect(screen.getByText("Director Console")).toBeInTheDocument();
    expect(screen.getByText("AI Waiter")).toBeInTheDocument();
    expect(screen.getByText("Payments go straight to you")).toBeInTheDocument();
  });
});

describe("AuthValuePropCompact", () => {
  it("keeps one Director / AI / fees line for the 390px sign-in column", () => {
    render(<AuthValuePropCompact />);
    const line = screen.getByTestId("auth-value-prop-compact");
    expect(line).toHaveTextContent(
      "Director Console · AI Waiter · Payments go straight to you",
    );
    expect(line.className).toMatch(/md:hidden/);
  });
});
