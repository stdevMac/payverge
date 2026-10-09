/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import BusinessContactTab from "../BusinessContactTab";

const t = (k: string) => k;

const business = {
  name: "Test Biz",
  social_media: JSON.stringify({
    instagram: "testbiz",
    tiktok: "testbiz",
  }),
  address: {},
} as any;

describe("BusinessContactTab — social links", () => {
  it("renders a TikTok link when configured", () => {
    render(
      <BusinessContactTab
        business={business}
        designSettings={{ primary_color: "#1a6b6a" }}
        operatingHours={[]}
        t={t}
      />,
    );
    const link = screen.getByRole("link", { name: /tiktok/i });
    expect(link).toHaveAttribute("href", "https://tiktok.com/@testbiz");
  });

  it("normalizes a dirty full-URL facebook value instead of double-prefixing", () => {
    const dirty = {
      ...business,
      social_media: JSON.stringify({
        facebook: "https://www.facebook.com/payverge/",
      }),
    };
    render(
      <BusinessContactTab
        business={dirty}
        designSettings={{ primary_color: "#1a6b6a" }}
        operatingHours={[]}
        t={t}
      />,
    );
    const link = screen.getByRole("link", { name: /facebook/i });
    expect(link).toHaveAttribute("href", "https://facebook.com/payverge");
    expect(link.getAttribute("href")).not.toMatch(/facebook\.com\/https/);
  });

  it("builds instagram href from a bare handle", () => {
    render(
      <BusinessContactTab
        business={business}
        designSettings={{ primary_color: "#1a6b6a" }}
        operatingHours={[]}
        t={t}
      />,
    );
    const link = screen.getByRole("link", { name: /instagram/i });
    expect(link).toHaveAttribute("href", "https://instagram.com/testbiz");
  });
});
