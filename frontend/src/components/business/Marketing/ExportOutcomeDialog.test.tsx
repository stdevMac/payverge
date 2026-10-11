/** @jest-environment jsdom */

import React from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import {
  ExportOutcomeDialog,
  type MarketingCreativeHandoff,
} from "./ExportOutcomeDialog";

const handoff: MarketingCreativeHandoff = {
  mode: "downloaded",
  suggestion: {
    id: "a",
    play: "featured_dish",
    title: "Feature Carbonara",
    target_name: "Carbonara",
    why_data: "Top seller",
    source: "menu_engineering",
    copy_angle: "Feature the dish",
    rank: 1,
  },
  creative: {
    caption: "Exact caption",
    image_url: "https://cdn/c.jpg",
    image_source: "menu",
    template: "editorial",
    aspect: "4:5",
    slots: { dishName: "Carbonara" },
    crop: { x: 0.5, y: 0.5, zoom: 1 },
    font_family: "Inter",
  },
};

const t = (key: string) => key;

it("Not yet closes without recording and leaves the creative untouched", () => {
  const onNotYet = jest.fn();
  const onConfirmPublished = jest.fn();
  render(
    <ExportOutcomeDialog
      handoff={handoff}
      onNotYet={onNotYet}
      onConfirmPublished={onConfirmPublished}
      t={t}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "outcome.notYet" }));
  expect(onNotYet).toHaveBeenCalledTimes(1);
  expect(onConfirmPublished).not.toHaveBeenCalled();
});

it("Yes, it was published returns the handoff snapshot", () => {
  const onConfirmPublished = jest.fn();
  render(
    <ExportOutcomeDialog
      handoff={handoff}
      onNotYet={() => {}}
      onConfirmPublished={onConfirmPublished}
      t={t}
    />,
  );
  fireEvent.click(
    screen.getByRole("button", { name: "outcome.confirmPublished" }),
  );
  expect(onConfirmPublished).toHaveBeenCalledWith(
    expect.objectContaining({
      mode: handoff.mode,
      suggestion: handoff.suggestion,
      creative: handoff.creative,
    }),
  );
});

it("sends freeform posted channel on confirm without implying a schedule", () => {
  const onConfirmPublished = jest.fn();
  render(
    <ExportOutcomeDialog
      handoff={handoff}
      onNotYet={() => {}}
      onConfirmPublished={onConfirmPublished}
      t={t}
    />,
  );
  const input = screen.getByTestId("outcome-posted-channel");
  fireEvent.change(input, { target: { value: "WhatsApp group" } });
  fireEvent.click(
    screen.getByRole("button", { name: "outcome.confirmPublished" }),
  );
  expect(onConfirmPublished).toHaveBeenCalledWith(
    expect.objectContaining({ postedChannel: "WhatsApp group" }),
  );
  expect(screen.getByText("outcome.channelHint")).toBeInTheDocument();
  expect(screen.queryByText(/schedul/i)).not.toBeInTheDocument();
});

it("seeds channel from destination label when present", () => {
  render(
    <ExportOutcomeDialog
      handoff={{ ...handoff, destinationLabel: "Instagram Feed" }}
      onNotYet={() => {}}
      onConfirmPublished={() => {}}
      t={t}
    />,
  );
  expect(screen.getByTestId("outcome-posted-channel")).toHaveValue(
    "Instagram Feed",
  );
});

it("keeps a failed confirmation open with an explicit retry", () => {
  const onRetry = jest.fn();
  render(
    <ExportOutcomeDialog
      handoff={handoff}
      onNotYet={() => {}}
      onConfirmPublished={() => {}}
      error="record failed"
      onRetry={onRetry}
      t={t}
    />,
  );
  expect(screen.getByRole("alert")).toHaveTextContent("outcome.error");
  fireEvent.click(screen.getByRole("button", { name: "outcome.retry" }));
  expect(onRetry).toHaveBeenCalledTimes(1);
});

it("has a visible accessible name and focuses the published decision", async () => {
  render(
    <ExportOutcomeDialog
      handoff={handoff}
      onNotYet={() => {}}
      onConfirmPublished={() => {}}
      t={t}
    />,
  );

  const dialog = screen.getByRole("dialog", { name: "outcome.title" });
  expect(screen.getByText("outcome.title")).toHaveTextContent("outcome.title");
  await waitFor(() =>
    expect(dialog).toContainElement(document.activeElement as HTMLElement),
  );
});

it("names the destination in outcome copy and never implies scheduling", () => {
  const tWithParams = (
    key: string,
    params?: Record<string, string | number>,
  ) =>
    params
      ? `${key}:${Object.entries(params)
          .map(([k, v]) => `${k}=${v}`)
          .join(",")}`
      : key;

  render(
    <ExportOutcomeDialog
      handoff={{
        ...handoff,
        mode: "downloaded",
        destinationLabel: "Instagram Feed",
      }}
      onNotYet={() => {}}
      onConfirmPublished={() => {}}
      t={tWithParams}
    />,
  );

  expect(
    screen.getByText("outcome.downloadedWithDestination:destination=Instagram Feed"),
  ).toBeInTheDocument();
  expect(screen.queryByText(/schedul/i)).not.toBeInTheDocument();
});
