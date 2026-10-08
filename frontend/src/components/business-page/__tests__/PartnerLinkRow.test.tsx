/** @jest-environment jsdom */
import { render, screen } from "@testing-library/react";
import PartnerLinkRow from "../PartnerLinkRow";

describe("PartnerLinkRow", () => {
   
  const baseDesign = { primary_color: "#1a6b6a", corner_radius: "medium" };
   

  it("renders the partner name and host", () => {
    render(
      <PartnerLinkRow
        link={{ name: "Uber Eats", url: "https://www.ubereats.com/store/foo" }}
        designSettings={baseDesign}
        ctaLabel="Open partner"
      />,
    );
    expect(screen.getByText("Uber Eats")).toBeInTheDocument();
    expect(screen.getByText(/ubereats\.com/)).toBeInTheDocument();
  });

  it("does not wrap content in a Card or use bg-brand button", () => {
    const { container } = render(
      <PartnerLinkRow
        link={{ name: "OpenTable", url: "https://www.opentable.com/r/foo" }}
        designSettings={baseDesign}
        ctaLabel="Open partner"
      />,
    );
    expect(container.querySelector('[role="presentation"]')).toBeNull();
    expect(container.innerHTML).not.toMatch(/\bbg-brand\b/);
    expect(container.innerHTML).not.toMatch(/\bbg-brand-dark\b/);
  });

  it("renders provider icon when icon_url is present", () => {
    render(
      <PartnerLinkRow
        link={{
          name: "Uber Eats",
          url: "https://www.ubereats.com/foo",
          icon_url: "https://picsum.photos/seed/uber/40/40",
        }}
        designSettings={baseDesign}
        ctaLabel="Open"
      />,
    );
    const img = screen.getByAltText("Uber Eats");
    expect(img.getAttribute("src")).toContain("picsum.photos");
  });
});
