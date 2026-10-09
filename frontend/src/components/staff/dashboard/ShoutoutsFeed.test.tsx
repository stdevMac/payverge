/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import ShoutoutsFeed, { type ShoutoutsFeedLabels } from "@/components/staff/dashboard/ShoutoutsFeed";
import type { Shoutout } from "@/api/engagement";

const labels: ShoutoutsFeedLabels = {
  title: "Recognition",
  empty: "No shout-outs yet",
  emptyHint: "Kudos appear here.",
  loading: "Loading recognition",
  send: "Send a shout-out",
  recipient: "To",
  recipientPlaceholder: "Choose a teammate",
  message: "Message",
  messagePlaceholder: "What did they do well?",
  emoji: "Emoji",
  visibility: "Visibility",
  team: "Whole team",
  private: "Private",
  submit: "Send",
  sending: "Sending…",
};

const shoutouts: Shoutout[] = [
  {
    id: 1, business_id: 42, from_staff_id: 7, to_staff_id: 3,
    message: "Great close last night", emoji: "🙌", visibility: "team",
    created_at: "2026-06-30T18:00:00Z",
  },
];

const teammates = [
  { id: 3, name: "Mara" },
  { id: 9, name: "Diego" },
];

const nameById = (id: number) => ({ 7: "You", 3: "Mara", 9: "Diego" }[id] ?? "—");

describe("ShoutoutsFeed", () => {
  it("renders the feed with resolved names and never shows a dollar amount", () => {
    render(
      <ShoutoutsFeed
        shoutouts={shoutouts}
        teammates={teammates}
        nameById={nameById}
        labels={labels}
        onSend={jest.fn()}
      />,
    );
    expect(screen.getByText("Great close last night")).toBeInTheDocument();
    expect(screen.getByText("You → Mara")).toBeInTheDocument();
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("sends a shout-out with the chosen recipient + message", async () => {
    const onSend = jest.fn();
    render(
      <ShoutoutsFeed
        shoutouts={shoutouts}
        teammates={teammates}
        nameById={nameById}
        labels={labels}
        onSend={onSend}
      />,
    );
    // Recipient is a NextUI Select: open the dropdown and pick Diego (id 9).
    await userEvent.click(screen.getByRole("button", { name: /To/ }));
    await userEvent.click(await screen.findByRole("option", { name: "Diego" }));
    fireEvent.change(screen.getByLabelText("Message"), { target: { value: "Nailed the rush" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(onSend).toHaveBeenCalledWith({
      to_staff_id: 9,
      message: "Nailed the rush",
      emoji: undefined,
      visibility: "team",
    });
  });

  it("renders an empty state when there are no shout-outs", () => {
    render(
      <ShoutoutsFeed
        shoutouts={[]}
        teammates={teammates}
        nameById={nameById}
        labels={labels}
        onSend={jest.fn()}
      />,
    );
    expect(screen.getByText("No shout-outs yet")).toBeInTheDocument();
  });
});
