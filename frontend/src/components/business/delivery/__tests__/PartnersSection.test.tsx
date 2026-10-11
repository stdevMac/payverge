/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { PartnersSection } from "../PartnersSection";

const t = (k: string) => k;

// Mock ExternalPartnerLinksEditor to avoid complex deps
jest.mock("../../ExternalPartnerLinksEditor", () => ({
  __esModule: true,
  default: ({
    links,
    onChange,
    labels,
    catalog,
  }: {
    links: unknown[];
    onChange: (l: unknown[]) => void;
    labels: Record<string, string>;
    catalog?: Array<{ key: string }>;
  }) => (
    <div data-testid="external-partner-links-editor">
      <span data-testid="link-count">{links.length}</span>
      <button onClick={() => onChange([])}>clear</button>
      <span data-testid="label-title">{labels.title}</span>
      <span data-testid="catalog-keys">
        {(catalog ?? []).map((p) => p.key).join(",")}
      </span>
    </div>
  ),
}));

describe("PartnersSection", () => {
  const links = [
    { name: "DoorDash", url: "https://doordash.com" },
    { name: "UberEats", url: "https://ubereats.com" },
  ];

  it("renders section header", () => {
    render(
      <PartnersSection
        businessId={1}
        links={links}
        onChange={jest.fn()}
        tString={t}
      />,
    );
    expect(
      screen.getByText("focused.partners.cardTitle"),
    ).toBeInTheDocument();
    expect(
      screen.getByText("focused.partners.cardSubtitle"),
    ).toBeInTheDocument();
  });

  it("passes links and businessId to ExternalPartnerLinksEditor", () => {
    render(
      <PartnersSection
        businessId={42}
        links={links}
        onChange={jest.fn()}
        tString={t}
      />,
    );
    expect(screen.getByTestId("link-count").textContent).toBe("2");
  });

  it("forwards onChange callback", () => {
    const onChange = jest.fn();
    render(
      <PartnersSection
        businessId={1}
        links={links}
        onChange={onChange}
        tString={t}
      />,
    );
    screen.getByText("clear").click();
    expect(onChange).toHaveBeenCalledWith([]);
  });

  // #829: the delivery picker must only offer couriers — reservation
  // platforms (OpenTable, Resy) are not delivery partners.
  it("hands the editor a courier-only catalog without OpenTable/Resy", () => {
    render(
      <PartnersSection
        businessId={1}
        links={links}
        onChange={jest.fn()}
        tString={t}
      />,
    );
    const keys = (screen.getByTestId("catalog-keys").textContent ?? "").split(",");
    expect(keys.length).toBeGreaterThan(0);
    expect(keys).not.toContain("opentable");
    expect(keys).not.toContain("resy");
    expect(keys).toEqual(
      expect.arrayContaining([
        "talabat",
        "careem",
        "deliveroo",
        "ubereats",
        "doordash",
        "zomato",
      ]),
    );
  });

  it("passes translated labels to editor", () => {
    render(
      <PartnersSection
        businessId={1}
        links={[]}
        onChange={jest.fn()}
        tString={t}
      />,
    );
    expect(screen.getByTestId("label-title").textContent).toBe(
      "focused.providers.connectedTitle",
    );
  });
});
