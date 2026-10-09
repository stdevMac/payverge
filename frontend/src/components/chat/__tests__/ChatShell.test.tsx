/** @jest-environment jsdom */

import React, { useState } from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ChatMessage } from "@/components/chat/types";
import ChatShell from "../ChatShell";

jest.mock("framer-motion", () => {
  const ReactActual = require("react");
  const Motion = new Proxy(
    {},
    {
      get: (_target, tag: string) =>
        ReactActual.forwardRef(
          ({ children, ...props }: any, ref: React.Ref<HTMLElement>) => {
            const {
              initial: _initial,
              animate: _animate,
              exit: _exit,
              transition: _transition,
              ...domProps
            } = props;
            return ReactActual.createElement(
              tag,
              { ...domProps, ref },
              children,
            );
          },
        ),
    },
  );
  return {
    motion: Motion,
    AnimatePresence: ({ children }: { children: React.ReactNode }) => children,
    useReducedMotion: () => true,
  };
});

jest.mock("@nextui-org/react", () => ({
  Button: ({ children, onPress, isDisabled }: any) => (
    <button type="button" disabled={isDisabled} onClick={onPress}>
      {children}
    </button>
  ),
}));

type MutableMetrics = {
  scrollTop: number;
  scrollHeight: number;
  clientHeight: number;
};

function installMetrics(
  element: HTMLElement,
  initial: MutableMetrics,
): MutableMetrics {
  const metrics = { ...initial };
  Object.defineProperties(element, {
    scrollTop: {
      configurable: true,
      get: () => metrics.scrollTop,
      set: (value: number) => {
        metrics.scrollTop = Math.min(
          value,
          Math.max(0, metrics.scrollHeight - metrics.clientHeight),
        );
      },
    },
    scrollHeight: {
      configurable: true,
      get: () => metrics.scrollHeight,
    },
    clientHeight: {
      configurable: true,
      get: () => metrics.clientHeight,
    },
  });
  return metrics;
}

function Harness() {
  const [input, setInput] = useState("");
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  return (
    <ChatShell
      title="Assistant"
      messages={messages}
      input={input}
      onInputChange={setInput}
      onSend={() => {
        const prompt = input.trim();
        if (!prompt) return;
        setMessages((current) => [
          ...current,
          { role: "user", content: prompt },
        ]);
        setInput("");
      }}
      onClose={jest.fn()}
      placeholder="Ask a question"
      sendLabel="Send"
      loadingLabel="Thinking"
      dialogLabel="Assistant dialog"
      closeLabel="Close"
      conversationLabel="Conversation"
      jumpToLatestLabel="Jump to latest"
      portal={false}
    />
  );
}

describe("ChatShell long-turn behavior", () => {
  it("keeps composer focus and anchors the submitted turn without scrolling the document", async () => {
    const windowScroll = jest.spyOn(window, "scrollTo").mockImplementation();
    render(<Harness />);
    const scroller = screen.getByRole("log", { name: "Conversation" });
    const metrics = installMetrics(scroller, {
      scrollTop: 0,
      scrollHeight: 1_400,
      clientHeight: 400,
    });
    const composer = screen.getByRole("textbox", { name: "Ask a question" });
    fireEvent.change(composer, { target: { value: "Explain the full menu" } });
    fireEvent.click(screen.getByRole("button", { name: "Send" }));

    expect(await screen.findByText("Explain the full menu")).toBeVisible();
    await waitFor(() => expect(composer).toHaveFocus());
    expect(composer).toHaveValue("");
    expect(metrics.scrollTop).toBe(1_000);
    expect(windowScroll).not.toHaveBeenCalled();
  });
});
