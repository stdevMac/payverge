/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import OfferCoverSheet, { type OfferCoverSheetLabels } from "./OfferCoverSheet";

const labels: OfferCoverSheetLabels = {
  title: "Offer this shift",
  subtitle: "Two ways to pass this shift to someone else.",
  offerCover: "Offer for a teammate to cover",
  offerCoverHint: "A teammate accepts, then your manager approves.",
  giveUp: "Give up the shift",
  giveUpHint: "Your manager takes you off and finds cover.",
  cancel: "Cancel",
  close: "Close",
};

function renderSheet(overrides?: Partial<React.ComponentProps<typeof OfferCoverSheet>>) {
  const onOffer = jest.fn();
  const onGiveUp = jest.fn();
  const onClose = jest.fn();
  render(
    <OfferCoverSheet
      shiftLabel="Mon 5:00 PM – 11:00 PM"
      onOffer={onOffer}
      onGiveUp={onGiveUp}
      onClose={onClose}
      busy={false}
      labels={labels}
      {...overrides}
    />,
  );
  return { onOffer, onGiveUp, onClose };
}

test("shows the two hand-off choices with the shift label for context", () => {
  renderSheet();
  expect(screen.getByText("Offer for a teammate to cover")).toBeInTheDocument();
  expect(screen.getByText("Give up the shift")).toBeInTheDocument();
  expect(screen.getByText("Mon 5:00 PM – 11:00 PM")).toBeInTheDocument();
});

test("offer choice fires onOffer; give-up choice fires onGiveUp", () => {
  const { onOffer, onGiveUp } = renderSheet();
  fireEvent.click(screen.getByText("Offer for a teammate to cover"));
  expect(onOffer).toHaveBeenCalledTimes(1);
  fireEvent.click(screen.getByText("Give up the shift"));
  expect(onGiveUp).toHaveBeenCalledTimes(1);
});

test("both choices are disabled while a request is in flight", () => {
  const { onOffer, onGiveUp } = renderSheet({ busy: true });
  fireEvent.click(screen.getByText("Offer for a teammate to cover"));
  fireEvent.click(screen.getByText("Give up the shift"));
  expect(onOffer).not.toHaveBeenCalled();
  expect(onGiveUp).not.toHaveBeenCalled();
});

test("close button fires onClose", () => {
  const { onClose } = renderSheet();
  fireEvent.click(screen.getByRole("button", { name: "Close" }));
  expect(onClose).toHaveBeenCalledTimes(1);
});

test("pressing Escape calls onClose", () => {
  const { onClose } = renderSheet();
  fireEvent.keyDown(document, { key: "Escape" });
  expect(onClose).toHaveBeenCalledTimes(1);
});

test("pressing Escape while busy does not call onClose", () => {
  const { onClose } = renderSheet({ busy: true });
  fireEvent.keyDown(document, { key: "Escape" });
  expect(onClose).not.toHaveBeenCalled();
});

test("on mount focus is inside the dialog", () => {
  renderSheet();
  const dialog = screen.getByRole("dialog");
  expect(dialog.contains(document.activeElement)).toBe(true);
  expect(
    screen.getByText("Offer for a teammate to cover").closest("button"),
  ).toHaveFocus();
});

test("money-free: no dollar figure anywhere", () => {
  renderSheet();
  expect(document.body.textContent).not.toMatch(/\$\d/);
});
