/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import PageHeader from "./PageHeader";

describe("PageHeader", () => {
  it("renders the serif title, subtitle, and actions without a panel wrapper", () => {
    render(
      <PageHeader
        title="Tables"
        subtitle="Create and manage your restaurant tables"
        actions={<button type="button">Create table</button>}
      />,
    );

    const heading = screen.getByRole("heading", { name: "Tables" });
    expect(heading).toBeInTheDocument();
    expect(heading.className).toContain("font-title");
    expect(
      screen.getByText("Create and manage your restaurant tables"),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Create table" }),
    ).toBeInTheDocument();
  });

  it("renders a single quiet status dot", () => {
    render(<PageHeader title="Schedule" status={{ label: "Draft", tone: "attention" }} />);
    expect(screen.getByText("Draft")).toBeInTheDocument();
  });

  it("renders inline stats as a dot-separated text line", () => {
    render(
      <PageHeader
        title="Team"
        stats={[
          { label: "active staff", value: 4 },
          { label: "pending invites", value: 1 },
        ]}
      />,
    );

    expect(screen.getByText("4")).toBeInTheDocument();
    expect(screen.getByText("active staff")).toBeInTheDocument();
    expect(screen.getByText("pending invites")).toBeInTheDocument();
    expect(screen.getByText("·")).toBeInTheDocument();
  });

  it("exposes a stat's clarifying title as a native tooltip on its wrapper", () => {
    render(
      <PageHeader
        title="Tables"
        stats={[
          {
            label: "Reserved (next 30 min)",
            value: 2,
            title: "Tables with a reservation starting in the next 30 minutes",
          },
        ]}
      />,
    );

    const label = screen.getByText("Reserved (next 30 min)");
    const wrapper = label.closest("span[title]");
    expect(wrapper).not.toBeNull();
    expect(wrapper).toHaveAttribute(
      "title",
      "Tables with a reservation starting in the next 30 minutes",
    );
  });

  it("omits the stat line entirely when no stats are passed", () => {
    const { container } = render(<PageHeader title="Bills" stats={[]} />);
    expect(container.querySelectorAll("p")).toHaveLength(0);
  });

  it("lets the action cluster wrap inside the header instead of expanding the page (#460)", () => {
    const { container } = render(
      <PageHeader
        title="Menu"
        actions={
          <>
            <button type="button">Languages</button>
            <button type="button">Print menu</button>
            <button type="button">Import</button>
            <button type="button">Add category</button>
          </>
        }
      />,
    );
    const actions = container.querySelector("[data-page-header-actions]");
    expect(actions).not.toBeNull();
    expect(actions?.className).toMatch(/\bmin-w-0\b/);
    expect(actions?.className).toMatch(/\bmax-w-full\b/);
    expect(actions?.className).toMatch(/\bflex-wrap\b/);
    expect(actions?.className).not.toMatch(/\bshrink-0\b/);
  });
});
