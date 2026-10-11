/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import PollsList, { type PollCardView, type PollsListLabels } from "@/components/staff/dashboard/PollsList";

const labels: PollsListLabels = {
  title: "Polls",
  empty: "No open polls",
  emptyHint: "Polls appear here.",
  loading: "Loading polls",
  vote: "Vote",
  voted: "Vote recorded",
  closed: "Closed",
  anonymous: "Anonymous",
  votesCount: "{n} votes",
};

const openPoll: PollCardView = {
  id: 5, question: "Pizza night?", isAnonymous: false, status: "open", myVote: null,
  options: [
    { optionId: 11, label: "Yes", votes: null },
    { optionId: 12, label: "No", votes: null },
  ],
};

const votedPoll: PollCardView = {
  id: 6, question: "Best shift?", isAnonymous: true, status: "open", myVote: 21,
  options: [
    { optionId: 21, label: "Morning", votes: 3 },
    { optionId: 22, label: "Night", votes: 1 },
  ],
};

describe("PollsList", () => {
  it("renders vote buttons for an open, un-voted poll and never shows a dollar amount", () => {
    const onVote = jest.fn();
    render(<PollsList polls={[openPoll]} labels={labels} votingPollId={null} onVote={onVote} />);
    fireEvent.click(screen.getByRole("button", { name: "Yes" }));
    expect(onVote).toHaveBeenCalledWith(5, 11);
    expect(document.body.textContent).not.toMatch(/\$\d/);
  });

  it("renders tally bars + anonymous chip once voted, with no voter list", () => {
    render(<PollsList polls={[votedPoll]} labels={labels} votingPollId={null} onVote={jest.fn()} />);
    expect(screen.getByText("Vote recorded")).toBeInTheDocument();
    expect(screen.getByText("Anonymous")).toBeInTheDocument();
    expect(screen.getByText("3 votes")).toBeInTheDocument();
    // Voted poll shows bars, not vote buttons.
    expect(screen.queryByRole("button", { name: "Morning" })).not.toBeInTheDocument();
  });

  it("renders an empty state when there are no polls", () => {
    render(<PollsList polls={[]} labels={labels} votingPollId={null} onVote={jest.fn()} />);
    expect(screen.getByText("No open polls")).toBeInTheDocument();
  });
});
