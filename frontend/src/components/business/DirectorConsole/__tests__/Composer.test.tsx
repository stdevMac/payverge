/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import Composer from "../Composer";

jest.mock("@/i18n/SimpleTranslationProvider", () => ({
  useSimpleLocale: () => ({ locale: "en", setLocale: jest.fn() }),
  getTranslation: (key: string) => key,
}));

describe("Composer", () => {
  function renderComposer(
    overrides: Partial<React.ComponentProps<typeof Composer>> = {},
  ) {
    const props = {
      value: "",
      onChange: jest.fn(),
      onSend: jest.fn(),
      onAbort: jest.fn(),
      sending: false,
      assistantName: "Sage",
      placeholder: "Ask...",
      sendLabel: "Ask Sage",
      sendingLabel: "Thinking...",
      hint: "⌘↵ to send",
      ...overrides,
    };
    return { ...render(<Composer {...props} />), props };
  }

  it("returns focus to the textarea after onSend resolves", async () => {
    const onSend = jest.fn().mockResolvedValue(undefined);
    const { props } = renderComposer({ onSend, value: "hello" });
    const textarea = screen.getByRole("textbox");
    textarea.focus();
    fireEvent.click(screen.getByRole("button", { name: /ask sage/i }));
    await waitFor(() => {
      expect(document.activeElement).toBe(textarea);
    });
    expect(onSend).toHaveBeenCalled();
    void props;
  });

  it("sends on Cmd+Enter and Ctrl+Enter, inserts newline on plain Enter", async () => {
    const onSend = jest.fn().mockResolvedValue(undefined);
    renderComposer({ onSend, value: "hi" });
    const textarea = screen.getByRole("textbox");
    fireEvent.keyDown(textarea, { key: "Enter", metaKey: true });
    expect(onSend).toHaveBeenCalledTimes(1);
    fireEvent.keyDown(textarea, { key: "Enter", ctrlKey: true });
    expect(onSend).toHaveBeenCalledTimes(2);
    // plain Enter must NOT call onSend
    fireEvent.keyDown(textarea, { key: "Enter" });
    expect(onSend).toHaveBeenCalledTimes(2);
  });

  it("calls onAbort and shows Stop label while sending", () => {
    const onAbort = jest.fn();
    renderComposer({ onAbort, sending: true });
    const stop = screen.getByRole("button", { name: /stop/i });
    fireEvent.click(stop);
    expect(onAbort).toHaveBeenCalled();
  });

  it("shows the ⌘↵ hint", () => {
    renderComposer();
    expect(screen.getByText(/⌘↵/)).toBeInTheDocument();
  });

  it("renders an unclipped bordered field wrapper", () => {
    renderComposer();
    const root = screen.getByTestId("dc-composer");
    expect(root.className).toMatch(/px-2|px-3/);
    const wrappers = root.querySelectorAll("[class*='border']");
    expect(wrappers.length).toBeGreaterThan(0);
    expect(root.className).toMatch(/border-t/);
  });

  it("Esc dismiss stays closed while the same slash token remains (L4-12a)", () => {
    const onChange = jest.fn();
    const { rerender } = render(
      <Composer
        value="/plan"
        onChange={onChange}
        onSend={jest.fn()}
        onAbort={jest.fn()}
        sending={false}
        assistantName="Sage"
        placeholder="Ask..."
        sendLabel="Ask Sage"
        sendingLabel="Thinking..."
        hint="⌘↵ to send"
        paletteItems={["Plan 7-day AOV", "Why was Tuesday slow?"]}
      />,
    );
    expect(screen.getByTestId("dc-slash-palette")).toBeInTheDocument();
    fireEvent.keyDown(screen.getByTestId("dc-composer"), { key: "Escape" });
    expect(screen.queryByTestId("dc-slash-palette")).not.toBeInTheDocument();

    // Same token after value-effect — must stay dismissed.
    rerender(
      <Composer
        value="/plan"
        onChange={onChange}
        onSend={jest.fn()}
        onAbort={jest.fn()}
        sending={false}
        assistantName="Sage"
        placeholder="Ask..."
        sendLabel="Ask Sage"
        sendingLabel="Thinking..."
        hint="⌘↵ to send"
        paletteItems={["Plan 7-day AOV", "Why was Tuesday slow?"]}
      />,
    );
    expect(screen.queryByTestId("dc-slash-palette")).not.toBeInTheDocument();
  });

  it("filters palette items by text after / (L4-12b)", () => {
    renderComposer({
      value: "/plan",
      paletteItems: ["Plan 7-day AOV", "Why was Tuesday slow?", "Retention"],
    });
    expect(screen.getByText("Plan 7-day AOV")).toBeInTheDocument();
    expect(screen.queryByText("Retention")).not.toBeInTheDocument();
  });

  it("inserts palette selection into the input without calling onSend (L4-12c)", async () => {
    const onChange = jest.fn();
    const onSend = jest.fn();
    const onPaletteSelect = jest.fn();
    renderComposer({
      value: "/plan",
      onChange,
      onSend,
      onPaletteSelect,
      paletteItems: ["Plan 7-day AOV growth"],
    });
    fireEvent.click(screen.getByText("Plan 7-day AOV growth"));
    expect(onChange).toHaveBeenCalledWith("Plan 7-day AOV growth");
    expect(onSend).not.toHaveBeenCalled();
    await waitFor(() => {
      expect(onPaletteSelect).toHaveBeenCalledWith("Plan 7-day AOV growth");
    });
  });

  describe("⌘↵ with the slash palette open must fire exactly one action", () => {
    // The palette's native keydown listener runs on the composer root, before
    // React's delegated handler. It used to intercept ANY Enter, so ⌘↵ both
    // inserted the highlighted item AND sent the raw "/token" draft.
    const paletteItems = ["Plan 7-day AOV growth"];

    it("does not insert the palette item on ⌘↵ (send wins)", () => {
      const onChange = jest.fn();
      const onSend = jest.fn().mockResolvedValue(undefined);
      renderComposer({ value: "/plan", onChange, onSend, paletteItems });
      const textarea = screen.getByRole("textbox");

      fireEvent.keyDown(textarea, { key: "Enter", metaKey: true });

      expect(onChange).not.toHaveBeenCalled();
      expect(onSend).toHaveBeenCalledTimes(1);
    });

    it("does not insert the palette item on ⌃↵ either", () => {
      const onChange = jest.fn();
      const onSend = jest.fn().mockResolvedValue(undefined);
      renderComposer({ value: "/plan", onChange, onSend, paletteItems });

      fireEvent.keyDown(screen.getByRole("textbox"), {
        key: "Enter",
        ctrlKey: true,
      });

      expect(onChange).not.toHaveBeenCalled();
      expect(onSend).toHaveBeenCalledTimes(1);
    });

    it("still inserts (and never sends) on unmodified Enter", () => {
      const onChange = jest.fn();
      const onSend = jest.fn();
      renderComposer({ value: "/plan", onChange, onSend, paletteItems });

      fireEvent.keyDown(screen.getByRole("textbox"), { key: "Enter" });

      expect(onChange).toHaveBeenCalledWith("Plan 7-day AOV growth");
      expect(onSend).not.toHaveBeenCalled();
    });

    it("does not send when another handler already consumed the ⌘↵", () => {
      // defaultPrevented guard: the composer must not act on a key event that
      // something upstream already handled.
      const onSend = jest.fn();
      renderComposer({ value: "hello", onSend });
      const textarea = screen.getByRole("textbox");
      const consume = (e: Event) => e.preventDefault();
      textarea.addEventListener("keydown", consume);

      fireEvent.keyDown(textarea, { key: "Enter", metaKey: true });

      expect(onSend).not.toHaveBeenCalled();
      textarea.removeEventListener("keydown", consume);
    });
  });
});