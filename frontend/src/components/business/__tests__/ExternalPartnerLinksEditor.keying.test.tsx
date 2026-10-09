/** @jest-environment jsdom */
import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import ExternalPartnerLinksEditor, {
  type ExternalPartnerLink,
} from "@/components/business/ExternalPartnerLinksEditor";

jest.mock("@/api/uploads", () => ({ uploadFile: jest.fn() }));

const labels = {
  title: "Partners",
  description: "desc",
  addProvider: "Add",
  nameLabel: "Name",
  urlLabel: "URL",
  iconSourceLabel: "Icon source",
  sourcePredefined: "Predefined",
  sourceCustom: "Custom",
  providerLabel: "Provider",
  iconUrlLabel: "Icon URL",
  uploadIcon: "Upload",
  removeProvider: "Remove",
  maxReached: "Max {max}",
};

function Harness({ initial }: { initial: ExternalPartnerLink[] }) {
  const [links, setLinks] = React.useState(initial);
  return (
    <ExternalPartnerLinksEditor
      businessId={1}
      links={links}
      onChange={setLinks}
      labels={labels}
    />
  );
}

describe("ExternalPartnerLinksEditor — stable row keys", () => {
  it("keeps focus on the correct row after removing a middle row", () => {
    render(
      <Harness
        initial={[
          { name: "First", url: "https://a.test", provider_key: "p" },
          { name: "Second", url: "https://b.test", provider_key: "p" },
          { name: "Third", url: "https://c.test", provider_key: "p" },
        ]}
      />,
    );

    const rows = screen.getAllByLabelText("URL") as HTMLInputElement[];
    expect(rows).toHaveLength(3);
    expect(rows.map((r) => r.value)).toEqual([
      "https://a.test",
      "https://b.test",
      "https://c.test",
    ]);

    // Capture the THIRD row's actual DOM node before removal.
    const thirdRowNode = rows[2];

    // Remove the MIDDLE row.
    const removeButtons = screen.getAllByRole("button", { name: "Remove" });
    fireEvent.click(removeButtons[1]);

    const remaining = screen.getAllByLabelText("URL") as HTMLInputElement[];
    expect(remaining).toHaveLength(2);
    expect(remaining.map((r) => r.value)).toEqual([
      "https://a.test",
      "https://c.test",
    ]);

    // Stable keys preserve row identity: the SAME DOM node that held the third
    // row still holds it (now at index 1). With index keys React would reuse
    // the node at position 1 for "https://c.test" and discard thirdRowNode,
    // so transient UI state (focus / IME / upload target) would attach wrong.
    expect(remaining[1]).toBe(thirdRowNode);
    expect(remaining).not.toContain(rows[1]); // the removed middle node is gone
  });
});
