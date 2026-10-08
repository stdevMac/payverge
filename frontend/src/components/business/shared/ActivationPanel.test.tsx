/** @jest-environment jsdom */
import React from "react";
import { render, screen } from "@testing-library/react";
import { Boxes } from "lucide-react";
import ActivationPanel from "./ActivationPanel";

describe("ActivationPanel", () => {
  it("renders title, description, plain feature lines, CTA, and footnote", () => {
    render(
      <ActivationPanel
        icon={Boxes}
        title="Activate inventory"
        description="Track stock levels and get alerts before you run out."
        features={[
          { title: "Stock tracking" },
          { title: "Recipe mapping", description: "auto-deduct on order approval" },
        ]}
        action={<button type="button">Activate</button>}
        footnote="You can disable this anytime from settings."
      />,
    );

    expect(
      screen.getByRole("heading", { name: "Activate inventory" }),
    ).toBeInTheDocument();
    expect(
      screen.getByText("Track stock levels and get alerts before you run out."),
    ).toBeInTheDocument();
    expect(screen.getByText("Stock tracking")).toBeInTheDocument();
    expect(screen.getByText(/Recipe mapping/)).toBeInTheDocument();
    expect(screen.getByText(/auto-deduct on order approval/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Activate" })).toBeInTheDocument();
    expect(
      screen.getByText("You can disable this anytime from settings."),
    ).toBeInTheDocument();
    // Feature lines are plain checklist items, not boxed mini-cards.
    expect(screen.getAllByRole("listitem")).toHaveLength(2);
  });

  it("omits the feature list when none are provided", () => {
    render(
      <ActivationPanel
        icon={Boxes}
        title="Activate"
        description="Turn it on."
        action={<button type="button">Go</button>}
      />,
    );
    expect(screen.queryByRole("list")).not.toBeInTheDocument();
  });
});
