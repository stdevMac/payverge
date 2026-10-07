/**
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import ProactiveNudge from "../ProactiveNudge";

jest.mock("framer-motion", () => {
  const React = require("react");
  const Strip = ({ children, ...props }: any) => {
    // Drop animation-only props so React does not warn in jsdom.
    const {
      initial,
      animate,
      exit,
      transition,
      layout,
      ...rest
    } = props;
    return <div {...rest}>{children}</div>;
  };
  const StripSpan = ({ children, ...props }: any) => {
    const {
      initial,
      animate,
      exit,
      transition,
      layout,
      ...rest
    } = props;
    return <span {...rest}>{children}</span>;
  };
  return {
    motion: { div: Strip, span: StripSpan },
    AnimatePresence: ({ children }: any) => <>{children}</>,
    useReducedMotion: () => true,
  };
});

afterEach(() => {
  cleanup();
});

describe("ProactiveNudge", () => {
  it("renders the message and fires onEngage when the card body is clicked", () => {
    const onEngage = jest.fn();
    const onDismiss = jest.fn();
    render(
      <ProactiveNudge
        message="Need help deciding?"
        dismissLabel="Dismiss"
        onEngage={onEngage}
        onDismiss={onDismiss}
      />,
    );

    fireEvent.click(screen.getByText("Need help deciding?").closest("button")!);

    expect(onEngage).toHaveBeenCalledTimes(1);
    expect(onDismiss).not.toHaveBeenCalled();
  });

  it("fires onDismiss (and not onEngage) when the close button is clicked", () => {
    const onEngage = jest.fn();
    const onDismiss = jest.fn();
    render(
      <ProactiveNudge
        message="Need help deciding?"
        dismissLabel="Dismiss"
        onEngage={onEngage}
        onDismiss={onDismiss}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "Dismiss" }));

    expect(onDismiss).toHaveBeenCalledTimes(1);
    expect(onEngage).not.toHaveBeenCalled();
  });

  it("has role=status", () => {
    render(
      <ProactiveNudge
        message="Need help deciding?"
        dismissLabel="Dismiss"
        onEngage={jest.fn()}
        onDismiss={jest.fn()}
      />,
    );

    expect(screen.getByRole("status")).toBeInTheDocument();
  });
});
