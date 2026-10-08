/** @jest-environment jsdom */
import React from "react";
import { render } from "@testing-library/react";
import PageHeader from "./PageHeader";

it("default (non-dense) renders stats on their own row (unchanged layout)", () => {
  const { container } = render(
    <PageHeader
      title="Marketing"
      subtitle="Ready to post"
      stats={[{ label: "credits", value: 10 }]}
    />,
  );
  // The stat line is a sibling <p> after the title row — the historical shape.
  const statLine = container.querySelector("header > p");
  expect(statLine).not.toBeNull();
  expect(statLine?.textContent).toContain("credits");
  // Not marked dense.
  expect(container.querySelector("[data-dense='true']")).toBeNull();
});

it("dense renders title + stats + actions in a single top row", () => {
  const { container, getByText } = render(
    <PageHeader
      dense
      title="Marketing"
      stats={[{ label: "credits", value: 10 }]}
      actions={<button>Studio</button>}
    />,
  );
  expect(container.querySelector("[data-dense='true']")).not.toBeNull();
  // In dense mode the stats live inside the top flex row, not a trailing <p>.
  expect(container.querySelector("header > p")).toBeNull();
  expect(getByText("Studio")).toBeInTheDocument();
  expect(getByText("credits")).toBeInTheDocument();
});
