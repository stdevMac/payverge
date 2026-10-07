/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";

const mockSuccess = jest.fn();
const mockDismiss = jest.fn();
jest.mock("react-hot-toast", () => ({
  __esModule: true,
  default: {
    success: (...a: unknown[]) => mockSuccess(...a),
    dismiss: (...a: unknown[]) => mockDismiss(...a),
  },
}));

import { showApprovedFollowUpToast } from "./approvalFollowUpToast";

it("renders the message with an action that dismisses and navigates", () => {
  const onAction = jest.fn();
  showApprovedFollowUpToast({
    message: "Approved",
    actionLabel: "View tables",
    onAction,
  });
  expect(mockSuccess).toHaveBeenCalledTimes(1);
  const renderFn = mockSuccess.mock.calls[0][0] as (t: {
    id: string;
  }) => React.ReactElement;
  render(renderFn({ id: "toast-1" }));
  fireEvent.click(screen.getByRole("button", { name: "View tables" }));
  expect(mockDismiss).toHaveBeenCalledWith("toast-1");
  expect(onAction).toHaveBeenCalled();
});
