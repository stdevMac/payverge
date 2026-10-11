/** @jest-environment jsdom */
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import SegmentsTab from "../SegmentsTab";
import { getSegments } from "@/api/crm";

jest.mock("@/api/crm", () => ({
  getSegments: jest.fn(),
}));

const getSegmentsMock = getSegments as jest.Mock;

describe("SegmentsTab", () => {
  beforeEach(() => {
    jest.clearAllMocks();
    getSegmentsMock.mockResolvedValue({ lapsed: 3, vip: 5, new: 8, atRisk: 2 });
  });

  it("renders built-in segments with counts", async () => {
    render(<SegmentsTab businessId={1} onJumpToCustomers={jest.fn()} />);
    expect(await screen.findByText(/Lapsed/i)).toBeInTheDocument();
    expect(await screen.findByText(/VIP/i)).toBeInTheDocument();
    expect(await screen.findByText("5")).toBeInTheDocument();
  });

  it("drills into the customer list with the backend segment param on card click", async () => {
    const onJump = jest.fn();
    render(<SegmentsTab businessId={1} onJumpToCustomers={onJump} />);
    // The atRisk card must map to the backend param "at-risk".
    const card = await screen.findByRole("button", { name: /At Risk — 2/i });
    fireEvent.click(card);
    expect(onJump).toHaveBeenCalledWith("at-risk");
  });

  it("renders a retryable error card when the fetch fails", async () => {
    getSegmentsMock.mockRejectedValueOnce(new Error("503"));
    render(<SegmentsTab businessId={1} onJumpToCustomers={jest.fn()} />);

    // The retryable error card renders (not a false "no segments yet").
    const retry = await screen.findByRole("button", { name: /Retry/i });
    expect(retry).toBeInTheDocument();
    expect(screen.getByText(/Couldn't load segments/i)).toBeInTheDocument();

    // Retrying refetches; the success path then renders the counts.
    getSegmentsMock.mockResolvedValueOnce({ lapsed: 3, vip: 5, new: 8, atRisk: 2 });
    fireEvent.click(retry);
    await waitFor(() =>
      expect(getSegmentsMock).toHaveBeenCalledTimes(2),
    );
    expect(await screen.findByText(/VIP/i)).toBeInTheDocument();
  });

  it("labels VIP as above-average spend, not a top-10% cut (#695)", async () => {
    render(<SegmentsTab businessId={1} onJumpToCustomers={jest.fn()} />);
    expect(await screen.findByText(/VIP/i)).toBeInTheDocument();
    expect(screen.queryByText(/10%/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Top 10%/i)).not.toBeInTheDocument();
    expect(
      screen.getByText(/5\+ visits and above-average lifetime spend/i),
    ).toBeInTheDocument();
  });
});
