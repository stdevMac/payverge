/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { Boxes } from "lucide-react";
import DashboardTabShell from "./DashboardTabShell";

const header = { title: "Inventory", subtitle: "Track ingredients" };

describe("DashboardTabShell", () => {
  it("shrinks inside flex dashboards so wide children can scroll (#211)", () => {
    const { container } = render(
      <DashboardTabShell header={header}>
        <div>body</div>
      </DashboardTabShell>,
    );
    expect(container.firstElementChild?.className).toMatch(/min-w-0/);
    expect(container.firstElementChild?.className).toMatch(/\bw-full\b/);
  });

  it("renders header, tabs, and content in the ready state", () => {
    render(
      <DashboardTabShell
        header={{
          ...header,
          stats: [{ label: "items", value: 12 }],
          actions: <button type="button">Refresh</button>,
        }}
        tabs={{
          items: [
            { key: "a", label: "Catalog" },
            { key: "b", label: "Counts" },
          ],
          activeKey: "a",
          onChange: () => {},
        }}
      >
        <div>content-body</div>
      </DashboardTabShell>,
    );

    expect(
      screen.getByRole("heading", { name: "Inventory" }),
    ).toBeInTheDocument();
    expect(screen.getByText("12")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Catalog" })).toBeInTheDocument();
    expect(screen.getByText("content-body")).toBeInTheDocument();
    expect(screen.getByRole("tabpanel").className).toMatch(/min-w-0/);
  });

  it("fills the dashboard pane so a child scrollport can take remaining height", () => {
    const { container } = render(
      <DashboardTabShell
        header={header}
        fill
        tabs={{
          items: [
            { key: "a", label: "Catalog" },
            { key: "b", label: "Counts" },
          ],
          activeKey: "a",
          onChange: () => {},
        }}
      >
        <div>content-body</div>
      </DashboardTabShell>,
    );

    expect(container.firstElementChild?.className).toMatch(/h-full/);
    expect(container.firstElementChild?.className).toMatch(/min-h-0/);
    expect(container.firstElementChild?.className).toMatch(/flex-col/);
    expect(container.firstElementChild?.className).toMatch(/\bgap-3\b/);
    expect(container.firstElementChild?.className).not.toMatch(/\bspace-y-/);
    expect(screen.getByRole("tabpanel").className).toMatch(/flex-1/);
    expect(screen.getByRole("tabpanel").className).toMatch(/min-h-0/);
    expect(screen.getByRole("tabpanel").className).toMatch(/min-w-0/);
  });

  it("locked wins over everything else", () => {
    render(
      <DashboardTabShell
        header={header}
        locked={<div>locked-view</div>}
        loading={<div>skeleton</div>}
        activation={{
          icon: Boxes,
          title: "Activate",
          description: "Turn on.",
          action: <button type="button">Go</button>,
        }}
      >
        <div>content-body</div>
      </DashboardTabShell>,
    );

    expect(screen.getByText("locked-view")).toBeInTheDocument();
    expect(screen.queryByText("skeleton")).not.toBeInTheDocument();
    expect(screen.queryByText("content-body")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("heading", { name: "Inventory" }),
    ).not.toBeInTheDocument();
  });

  it("loading wins over activation and content", () => {
    render(
      <DashboardTabShell
        header={header}
        loading={<div>skeleton</div>}
        activation={{
          icon: Boxes,
          title: "Activate",
          description: "Turn on.",
          action: <button type="button">Go</button>,
        }}
      >
        <div>content-body</div>
      </DashboardTabShell>,
    );

    expect(screen.getByText("skeleton")).toBeInTheDocument();
    expect(screen.queryByText("content-body")).not.toBeInTheDocument();
  });

  // S-9: loading must not unmount the page header or sub-tab rail.
  // The shell contract promises "The page header never disappears" — loading
  // belongs only in the content slot so operators can still read the title
  // and switch sub-tabs while data refetches.
  it("keeps page header and sub-tabs mounted while loading (S-9)", () => {
    render(
      <DashboardTabShell
        header={header}
        loading={<div>skeleton</div>}
        tabs={{
          items: [
            { key: "a", label: "Catalog" },
            { key: "b", label: "Counts" },
          ],
          activeKey: "a",
          onChange: () => {},
        }}
      >
        <div>content-body</div>
      </DashboardTabShell>,
    );

    expect(
      screen.getByRole("heading", { name: "Inventory" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Catalog" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Counts" })).toBeInTheDocument();
    expect(screen.getByText("skeleton")).toBeInTheDocument();
    expect(screen.queryByText("content-body")).not.toBeInTheDocument();
  });

  it("activation renders header + activation panel instead of content", () => {
    render(
      <DashboardTabShell
        header={header}
        activation={{
          icon: Boxes,
          title: "Activate inventory",
          description: "Turn on tracking.",
          features: [{ title: "Stock tracking" }],
          action: <button type="button">Activate</button>,
        }}
      >
        <div>content-body</div>
      </DashboardTabShell>,
    );

    expect(
      screen.getByRole("heading", { name: "Inventory" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Activate inventory" }),
    ).toBeInTheDocument();
    expect(screen.getByText("Stock tracking")).toBeInTheDocument();
    expect(screen.queryByText("content-body")).not.toBeInTheDocument();
  });

  it("treats `condition && activation` falsy values as ready state", () => {
    render(
      <DashboardTabShell header={header} activation={false}>
        <div>content-body</div>
      </DashboardTabShell>,
    );
    expect(screen.getByText("content-body")).toBeInTheDocument();
  });

  it("wraps children in a tabpanel wired to the active tab when tabs are present", () => {
    render(
      <DashboardTabShell
        header={header}
        tabs={{
          items: [
            { key: "a", label: "Catalog" },
            { key: "b", label: "Counts" },
          ],
          activeKey: "a",
          onChange: () => {},
        }}
      >
        <div>content-body</div>
      </DashboardTabShell>,
    );

    const panel = screen.getByRole("tabpanel");
    const activeTab = screen.getByRole("tab", { name: "Catalog" });
    // The panel is labelled by the active tab, and the active tab controls it.
    expect(panel.getAttribute("aria-labelledby")).toBe(
      activeTab.getAttribute("id"),
    );
    expect(panel.getAttribute("id")).toBe(
      activeTab.getAttribute("aria-controls"),
    );
    // Content lives inside the panel.
    expect(panel).toHaveTextContent("content-body");
  });

  it("renders children unwrapped (no tabpanel) when no tabs are provided", () => {
    render(
      <DashboardTabShell header={header}>
        <div>content-body</div>
      </DashboardTabShell>,
    );
    expect(screen.queryByRole("tabpanel")).not.toBeInTheDocument();
    expect(screen.getByText("content-body")).toBeInTheDocument();
  });
});
