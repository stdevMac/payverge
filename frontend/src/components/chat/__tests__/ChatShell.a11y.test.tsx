/**
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import ChatShell from "../ChatShell";

jest.mock("framer-motion", () => {
  const React = require("react");
  const Strip = ({ children, ...props }: any) => {
    // Drop animation-only props so React does not warn in jsdom.
    const { initial, animate, exit, transition, layout, ...rest } = props;
    return <div {...rest}>{children}</div>;
  };
  return {
    motion: { div: Strip },
    AnimatePresence: ({ children }: any) => <>{children}</>,
    useReducedMotion: () => true,
  };
});

jest.mock("@nextui-org/react", () => {
  const React = require("react");
  return {
    Button: ({ children, onPress, ...props }: any) => (
      <button type="button" onClick={() => onPress?.()} {...props}>
        {children}
      </button>
    ),
    Input: (props: any) => <input {...props} />,
    ScrollShadow: ({ children, ...props }: any) => (
      <div {...props}>{children}</div>
    ),
  };
});

jest.mock("@/components/business/DirectorConsole/DirectorMessage", () => {
  return function MockDirectorMessage({ content }: { content: string }) {
    return <div data-testid="director-message">{content}</div>;
  };
});

beforeAll(() => {
  // jsdom does not implement scrollIntoView.
  Element.prototype.scrollIntoView = jest.fn();
  // Polyfill inert if missing in older jsdom.
  if (!("inert" in HTMLElement.prototype)) {
    Object.defineProperty(HTMLElement.prototype, "inert", {
      configurable: true,
      enumerable: true,
      get() {
        return this.hasAttribute("inert");
      },
      set(value: boolean) {
        if (value) this.setAttribute("inert", "");
        else this.removeAttribute("inert");
      },
    });
  }
});

const baseProps = {
  title: "Help",
  messages: [] as any[],
  input: "",
  onInputChange: jest.fn(),
  onSend: jest.fn(),
  onClose: jest.fn(),
  placeholder: "Type…",
  sendLabel: "Send",
  loadingLabel: "Thinking",
  dialogLabel: "Assistant dialog",
  closeLabel: "Close",
  welcomeMessage: "How can I help?",
};

describe("ChatShell accessibility", () => {
  afterEach(() => {
    cleanup();
    document.body.innerHTML = "";
  });

  it("traps Tab focus inside the dialog and restores focus on unmount", async () => {
    const opener = document.createElement("button");
    opener.textContent = "Open";
    document.body.appendChild(opener);
    opener.focus();
    expect(document.activeElement).toBe(opener);

    const sibling = document.createElement("div");
    sibling.setAttribute("data-testid", "page-sibling");
    sibling.innerHTML = '<button type="button">Outside</button>';
    document.body.appendChild(sibling);

    const { unmount } = render(
      <div data-testid="app-root">
        <ChatShell {...baseProps} portal />
      </div>,
    );

    const dialog = await screen.findByRole("dialog", {
      name: "Assistant dialog",
    });
    expect(dialog).toHaveAttribute("aria-modal", "true");
    expect(dialog).toHaveAttribute("data-chat-shell-root");
    // Portaled as a direct body child (not nested under app-root).
    expect(dialog.parentElement).toBe(document.body);

    // Background body children (sibling, RTL root) are inert while the portaled
    // dialog is open. Nested app-root inherits that through the body tree.
    expect(sibling.inert).toBe(true);
    const bodyChildren = Array.from(document.body.children) as HTMLElement[];
    const inertedRoots = bodyChildren.filter((el) => el !== dialog && el.inert);
    expect(inertedRoots.length).toBeGreaterThan(0);

    // Focus moved into the dialog.
    expect(dialog.contains(document.activeElement as Node)).toBe(true);

    // Tab cycles within dialog rather than escaping.
    fireEvent.keyDown(dialog, { key: "Tab" });
    expect(dialog.contains(document.activeElement as Node)).toBe(true);

    unmount();
    expect(sibling.inert).toBe(false);
    expect(document.activeElement).toBe(opener);
  });

  it("closes on Escape", async () => {
    const onClose = jest.fn();
    render(<ChatShell {...baseProps} onClose={onClose} portal />);
    const dialog = await screen.findByRole("dialog", {
      name: "Assistant dialog",
    });
    fireEvent.keyDown(dialog, { key: "Escape" });
    expect(onClose).toHaveBeenCalled();
  });

  it("moves initial focus to the multiline composer", async () => {
    render(<ChatShell {...baseProps} portal />);
    await screen.findByRole("dialog", { name: "Assistant dialog" });
    expect(document.activeElement).toBe(
      screen.getByRole("textbox", { name: "Type…" }),
    );
    expect(screen.getByRole("textbox", { name: "Type…" }).tagName).toBe(
      "TEXTAREA",
    );
  });

  it("keeps composer focus when the parent re-renders with a new onClose identity", async () => {
    const { rerender } = render(
      <ChatShell {...baseProps} portal onClose={() => {}} />,
    );
    await screen.findByRole("dialog", { name: "Assistant dialog" });
    const input = screen.getByPlaceholderText("Type…");
    input.focus();
    // Simulate a keystroke-driven parent re-render: new input value AND a fresh
    // inline onClose identity, exactly what ConciergeWidget/OpsAssistantWidget do.
    rerender(<ChatShell {...baseProps} portal input="a" onClose={() => {}} />);
    expect(document.activeElement).toBe(input);
  });

  it("keeps the dialog named while exposing conversation log semantics", async () => {
    render(
      <ChatShell
        {...baseProps}
        portal
        messages={[{ role: "assistant", content: "Restored answer" }]}
        conversationLabel="Conversation history"
        jumpToLatestLabel="Latest reply"
      />,
    );

    await screen.findByRole("dialog", { name: "Assistant dialog" });
    expect(
      screen.getByRole("log", { name: "Conversation history" }),
    ).toHaveTextContent("Restored answer");
  });

  it("uses the shared viewport for an error-only active conversation state", () => {
    render(
      <ChatShell
        {...baseProps}
        portal={false}
        welcomeMessage={undefined}
        errorMessage="Localized failure"
      />,
    );

    expect(screen.getByRole("log", { name: "Help" })).toHaveTextContent(
      "Localized failure",
    );
    expect(screen.getByRole("log", { name: "Help" })).toHaveAttribute(
      "aria-busy",
      "false",
    );
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });

  it("uses the shared viewport for an untouched welcome without an empty status", () => {
    render(<ChatShell {...baseProps} portal={false} />);

    expect(screen.getByRole("log", { name: "Help" })).toHaveAttribute(
      "aria-busy",
      "false",
    );
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  });
});
