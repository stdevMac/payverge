/** @jest-environment jsdom */

import React from "react";
import {
  act,
  createEvent,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import {
  AssistantComposer,
  type AssistantComposerLabels,
} from "../AssistantComposer";

const labels: AssistantComposerLabels = {
  textarea: "Escribe tu pregunta",
  send: "Enviar",
  busy: "Enviando",
  characterCount: (current, maximum) => `${current} de ${maximum} caracteres`,
};

type UncontrolledComposerOverrides = Partial<{
  onSend: React.ComponentProps<typeof AssistantComposer>["onSend"];
  busy: boolean;
  disabled: boolean;
  mobile: boolean;
  draftKey: string;
  initialValue: string;
  errorMessage: string | null;
  maxLength: number;
}>;

function renderComposer(overrides: UncontrolledComposerOverrides = {}) {
  const onSend = jest.fn().mockResolvedValue(undefined);
  render(
    <AssistantComposer
      labels={labels}
      maxLength={10}
      onSend={onSend}
      {...overrides}
    />,
  );

  return {
    onSend,
    textarea: screen.getByRole("textbox", { name: labels.textarea }),
    sendButton: screen.getByRole("button", { name: labels.send }),
  };
}

describe("AssistantComposer", () => {
  beforeEach(() => {
    window.localStorage.clear();
    jest.restoreAllMocks();
  });

  it("sends a trimmed desktop prompt with Enter, clears it, and restores focus", async () => {
    const { onSend, textarea } = renderComposer();
    fireEvent.change(textarea, { target: { value: "  Hola  " } });
    fireEvent.keyDown(textarea, { key: "Enter" });

    await waitFor(() => expect(onSend).toHaveBeenCalledWith("Hola"));
    await waitFor(() => expect(textarea).toHaveValue(""));
    expect(textarea).toHaveFocus();
  });

  it.each([
    ["Shift+Enter", { shiftKey: true }],
    ["mobile Enter", {}, true],
    ["IME Enter", { isComposing: true }],
  ])("does not send for %s", async (_name, keyOptions, mobile = false) => {
    const { onSend, textarea } = renderComposer({ mobile });
    fireEvent.change(textarea, { target: { value: "Hola" } });
    fireEvent.keyDown(textarea, { key: "Enter", ...keyOptions });

    expect(onSend).not.toHaveBeenCalled();
    expect(textarea).toHaveValue("Hola");
  });

  it("does not send legacy IME Enter events with keyCode 229", () => {
    const onSend = jest.fn(() => new Promise<void>(() => undefined));
    const { textarea } = renderComposer({ onSend });
    fireEvent.change(textarea, { target: { value: "Hola" } });
    const legacyImeEnter = createEvent.keyDown(textarea, {
      key: "Enter",
      keyCode: 229,
      which: 229,
      isComposing: false,
    }) as KeyboardEvent;

    expect(legacyImeEnter.keyCode).toBe(229);
    expect(legacyImeEnter.which).toBe(229);
    fireEvent(textarea, legacyImeEnter);

    expect(onSend).not.toHaveBeenCalled();
    expect(textarea).toHaveValue("Hola");
  });

  it("sends from the explicit localized button", async () => {
    const { onSend, textarea, sendButton } = renderComposer();
    fireEvent.change(textarea, { target: { value: "Pregunta" } });
    fireEvent.click(sendButton);

    await waitFor(() => expect(onSend).toHaveBeenCalledWith("Pregunta"));
  });

  it.each(["busy", "disabled"] as const)(
    "waits for external %s to clear before restoring composer focus",
    async (gate) => {
      let resolveSend!: () => void;
      const onSend = jest.fn(
        () =>
          new Promise<void>((resolve) => {
            resolveSend = resolve;
          }),
      );
      const tree = (gated: boolean) => (
        <>
          <AssistantComposer
            busy={gate === "busy" && gated}
            disabled={gate === "disabled" && gated}
            labels={labels}
            maxLength={20}
            onSend={onSend}
          />
          <button type="button">Fuera</button>
        </>
      );
      const { rerender } = render(tree(false));
      const textarea = screen.getByRole("textbox", { name: labels.textarea });
      fireEvent.change(textarea, { target: { value: "Pregunta" } });
      fireEvent.click(screen.getByRole("button", { name: labels.send }));
      rerender(tree(true));

      const outside = screen.getByRole("button", { name: "Fuera" });
      outside.focus();
      await act(async () => {
        resolveSend();
        await Promise.resolve();
      });

      expect(textarea).toHaveValue("");
      expect(outside).toHaveFocus();

      rerender(tree(false));
      await waitFor(() => expect(textarea).toHaveFocus());
    },
  );

  it("guards against rapid duplicate submissions before busy state renders", () => {
    const onSend = jest.fn(() => new Promise<void>(() => undefined));
    const { textarea, sendButton } = renderComposer({ onSend });
    fireEvent.change(textarea, { target: { value: "Una vez" } });

    act(() => {
      sendButton.click();
      sendButton.click();
    });

    expect(onSend).toHaveBeenCalledTimes(1);
  });

  it.each([
    ["whitespace", { initialValue: "   " }],
    ["busy", { initialValue: "Hola", busy: true }],
    ["disabled", { initialValue: "Hola", disabled: true }],
  ])("does not send when %s", async (_name, props) => {
    const onSend = jest.fn();
    render(
      <AssistantComposer
        labels={labels}
        maxLength={10}
        onSend={onSend}
        {...props}
      />,
    );

    const buttonName =
      "busy" in props && props.busy ? labels.busy : labels.send;
    fireEvent.click(screen.getByRole("button", { name: buttonName }));
    expect(onSend).not.toHaveBeenCalled();
  });

  it("uses a one-row textarea and caps growth at five lines with internal scrolling", () => {
    const { textarea } = renderComposer({ maxLength: 500 });
    expect(textarea).toHaveAttribute("rows", "1");

    Object.defineProperty(textarea, "scrollHeight", {
      configurable: true,
      value: 180,
    });
    textarea.style.lineHeight = "20px";
    textarea.style.paddingTop = "0px";
    textarea.style.paddingBottom = "0px";
    fireEvent.change(textarea, {
      target: { value: "one\ntwo\nthree\nfour\nfive\nsix" },
    });

    expect(textarea.style.height).toBe("100px");
    expect(textarea.style.overflowY).toBe("auto");

    Object.defineProperty(textarea, "scrollHeight", {
      configurable: true,
      value: 40,
    });
    fireEvent.change(textarea, { target: { value: "short" } });
    expect(textarea.style.height).toBe("40px");
    expect(textarea.style.overflowY).toBe("hidden");
  });

  it.each([
    ["false result", jest.fn().mockResolvedValue(false)],
    ["rejection", jest.fn().mockRejectedValue(new Error("offline"))],
    [
      "synchronous throw",
      jest.fn(() => {
        throw new Error("offline");
      }),
    ],
  ])("preserves the prompt after a %s", async (_name, onSend) => {
    const { textarea, sendButton } = renderComposer({ onSend });
    fireEvent.change(textarea, { target: { value: "Retry me" } });
    fireEvent.click(sendButton);

    await waitFor(() => expect(onSend).toHaveBeenCalledWith("Retry me"));
    await waitFor(() => expect(textarea).toHaveValue("Retry me"));
    expect(textarea).toHaveFocus();
  });

  it("enforces maxLength and shows the localized count at 80 percent", () => {
    const { textarea } = renderComposer();
    expect(textarea).toHaveAttribute("maxLength", "10");

    fireEvent.change(textarea, { target: { value: "1234567" } });
    expect(screen.queryByText("7 de 10 caracteres")).not.toBeInTheDocument();

    fireEvent.change(textarea, { target: { value: "12345678" } });
    expect(screen.getByText("8 de 10 caracteres")).toBeInTheDocument();
  });

  it("re-bounds the current draft instead of resetting it when maxLength changes", () => {
    const onSend = jest.fn();
    const { rerender } = render(
      <AssistantComposer labels={labels} maxLength={10} onSend={onSend} />,
    );
    const textarea = screen.getByRole("textbox", { name: labels.textarea });
    fireEvent.change(textarea, { target: { value: "12345678" } });

    rerender(
      <AssistantComposer labels={labels} maxLength={5} onSend={onSend} />,
    );

    expect(textarea).toHaveValue("12345");
    expect(textarea).toHaveAttribute("maxLength", "5");
  });

  it("uses localized textarea, send, count, and busy labels", () => {
    const { rerender } = render(
      <AssistantComposer
        busy
        initialValue="12345678"
        labels={labels}
        maxLength={10}
        onSend={jest.fn()}
      />,
    );

    expect(
      screen.getByRole("textbox", { name: labels.textarea }),
    ).toHaveAttribute("placeholder", labels.textarea);
    expect(screen.getByRole("button", { name: labels.busy })).toBeDisabled();
    expect(screen.getByText("8 de 10 caracteres")).toBeInTheDocument();

    rerender(
      <AssistantComposer
        initialValue="12345678"
        labels={labels}
        maxLength={10}
        onSend={jest.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: labels.send })).toBeEnabled();
  });

  it("uses a separate localized hint and can focus when its surface opens", () => {
    render(
      <AssistantComposer
        focusOnMount
        labels={labels}
        maxLength={20}
        onSend={jest.fn()}
        placeholder="Ask about the Harvest Bowl"
      />,
    );

    const textarea = screen.getByRole("textbox", { name: labels.textarea });
    expect(textarea).toHaveAttribute(
      "placeholder",
      "Ask about the Harvest Bowl",
    );
    expect(textarea).toHaveFocus();
  });

  it("renders a controlled value and follows parent rerenders", () => {
    const onValueChange = jest.fn();
    const onSend = jest.fn();
    const { rerender } = render(
      <AssistantComposer
        labels={labels}
        maxLength={20}
        onSend={onSend}
        onValueChange={onValueChange}
        value="Primero"
      />,
    );
    const textarea = screen.getByRole("textbox", { name: labels.textarea });
    expect(textarea).toHaveValue("Primero");

    rerender(
      <AssistantComposer
        labels={labels}
        maxLength={20}
        onSend={onSend}
        onValueChange={onValueChange}
        value="Después"
      />,
    );
    expect(textarea).toHaveValue("Después");
  });

  it("requests a bounded controlled value while leaving rendering to the parent", () => {
    const onValueChange = jest.fn();
    render(
      <AssistantComposer
        labels={labels}
        maxLength={5}
        onSend={jest.fn()}
        onValueChange={onValueChange}
        value="Old"
      />,
    );
    const textarea = screen.getByRole("textbox", { name: labels.textarea });

    fireEvent.change(textarea, { target: { value: "123456" } });

    expect(onValueChange).toHaveBeenCalledWith("12345");
    expect(textarea).toHaveValue("Old");
  });

  it("asks the parent to clear a successful controlled send and restores focus", async () => {
    const onValueChange = jest.fn();
    const onSend = jest.fn().mockResolvedValue(undefined);
    function ControlledHarness() {
      const [value, setValue] = React.useState("Pregunta");
      return (
        <AssistantComposer
          labels={labels}
          maxLength={20}
          onSend={onSend}
          onValueChange={(nextValue) => {
            onValueChange(nextValue);
            setValue(nextValue);
          }}
          value={value}
        />
      );
    }
    render(<ControlledHarness />);
    const textarea = screen.getByRole("textbox", { name: labels.textarea });

    fireEvent.click(screen.getByRole("button", { name: labels.send }));

    await waitFor(() => expect(onSend).toHaveBeenCalledWith("Pregunta"));
    await waitFor(() => expect(onValueChange).toHaveBeenCalledWith(""));
    expect(textarea).toHaveValue("");
    expect(textarea).toHaveFocus();
  });

  it.each([
    ["a newer prompt", "A", "B"],
    ["an exact-draft whitespace edit", " A ", "A"],
  ])(
    "does not clear %s when a controlled send resolves",
    async (_name, submittedValue, newerValue) => {
      let resolveSend!: () => void;
      const onSend = jest.fn(
        () =>
          new Promise<void>((resolve) => {
            resolveSend = resolve;
          }),
      );
      const onValueChange = jest.fn();
      const { rerender } = render(
        <AssistantComposer
          labels={labels}
          maxLength={20}
          onSend={onSend}
          onValueChange={onValueChange}
          value={submittedValue}
        />,
      );
      fireEvent.click(screen.getByRole("button", { name: labels.send }));
      expect(onSend).toHaveBeenCalledWith(submittedValue.trim());

      rerender(
        <AssistantComposer
          labels={labels}
          maxLength={20}
          onSend={onSend}
          onValueChange={onValueChange}
          value={newerValue}
        />,
      );
      await act(async () => {
        resolveSend();
        await Promise.resolve();
      });

      expect(onValueChange).not.toHaveBeenCalledWith("");
      expect(
        screen.getByRole("textbox", { name: labels.textarea }),
      ).toHaveValue(newerValue);
    },
  );

  it("does not ask the parent to clear a failed controlled send", async () => {
    const onValueChange = jest.fn();
    const onSend = jest.fn().mockResolvedValue(false);
    render(
      <AssistantComposer
        labels={labels}
        maxLength={20}
        onSend={onSend}
        onValueChange={onValueChange}
        value="Reintentar"
      />,
    );
    const textarea = screen.getByRole("textbox", { name: labels.textarea });

    fireEvent.click(screen.getByRole("button", { name: labels.send }));

    await waitFor(() => expect(onSend).toHaveBeenCalledWith("Reintentar"));
    expect(onValueChange).not.toHaveBeenCalledWith("");
    expect(textarea).toHaveValue("Reintentar");
  });

  it("requests a bounded controlled value when maxLength decreases", async () => {
    const onValueChange = jest.fn();
    const onSend = jest.fn();
    const { rerender } = render(
      <AssistantComposer
        labels={labels}
        maxLength={10}
        onSend={onSend}
        onValueChange={onValueChange}
        value="12345678"
      />,
    );
    onValueChange.mockClear();

    rerender(
      <AssistantComposer
        labels={labels}
        maxLength={5}
        onSend={onSend}
        onValueChange={onValueChange}
        value="12345678"
      />,
    );

    await waitFor(() => expect(onValueChange).toHaveBeenCalledWith("12345"));
    expect(screen.getByRole("textbox", { name: labels.textarea })).toHaveValue(
      "12345",
    );
  });

  it("never accesses draft storage when the parent controls the value", async () => {
    const getItem = jest.spyOn(Storage.prototype, "getItem");
    const setItem = jest.spyOn(Storage.prototype, "setItem");
    const removeItem = jest.spyOn(Storage.prototype, "removeItem");
    const onValueChange = jest.fn();
    render(
      <AssistantComposer
        draftKey="assistant-draft:ignored"
        labels={labels}
        maxLength={10}
        onSend={jest.fn().mockResolvedValue(undefined)}
        onValueChange={onValueChange}
        value="Controlled"
      />,
    );
    const textarea = screen.getByRole("textbox", { name: labels.textarea });

    fireEvent.change(textarea, { target: { value: "Changed" } });
    fireEvent.click(screen.getByRole("button", { name: labels.send }));

    await waitFor(() => expect(onValueChange).toHaveBeenCalledWith(""));
    expect(getItem).not.toHaveBeenCalled();
    expect(setItem).not.toHaveBeenCalled();
    expect(removeItem).not.toHaveBeenCalled();
  });

  it("persists only the draft associated with the supplied thread key", () => {
    window.localStorage.setItem(
      "assistant-draft:thread-7",
      JSON.stringify("Saved"),
    );
    const { textarea } = renderComposer({
      draftKey: "assistant-draft:thread-7",
    });

    expect(textarea).toHaveValue("Saved");
    fireEvent.change(textarea, { target: { value: "Updated" } });
    expect(window.localStorage.getItem("assistant-draft:thread-7")).toBe(
      JSON.stringify("Updated"),
    );
  });

  it("hydrates each draft key independently and resets an unknown thread", () => {
    window.localStorage.setItem("assistant-draft:one", JSON.stringify("One"));
    window.localStorage.setItem("assistant-draft:two", JSON.stringify("Two"));
    const onSend = jest.fn();
    const { rerender } = render(
      <AssistantComposer
        draftKey="assistant-draft:one"
        labels={labels}
        maxLength={10}
        onSend={onSend}
      />,
    );
    const textarea = screen.getByRole("textbox", { name: labels.textarea });
    expect(textarea).toHaveValue("One");

    rerender(
      <AssistantComposer
        draftKey="assistant-draft:two"
        labels={labels}
        maxLength={10}
        onSend={onSend}
      />,
    );
    expect(textarea).toHaveValue("Two");

    rerender(
      <AssistantComposer
        draftKey="assistant-draft:new"
        labels={labels}
        maxLength={10}
        onSend={onSend}
      />,
    );
    expect(textarea).toHaveValue("");
  });

  it("keeps typing usable when draft storage reads and writes fail", () => {
    jest.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("storage blocked");
    });
    jest.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("storage blocked");
    });
    const onSend = jest.fn();
    render(
      <AssistantComposer
        draftKey="assistant-draft:blocked"
        labels={labels}
        maxLength={10}
        onSend={onSend}
      />,
    );
    const textarea = screen.getByRole("textbox", { name: labels.textarea });

    fireEvent.change(textarea, { target: { value: "Still works" } });
    expect(textarea).toHaveValue("Still work");
  });

  it("does not read or write localStorage without a draft key", () => {
    const getItem = jest.spyOn(Storage.prototype, "getItem");
    const setItem = jest.spyOn(Storage.prototype, "setItem");
    const { textarea } = renderComposer();

    fireEvent.change(textarea, { target: { value: "Ephemeral" } });
    expect(getItem).not.toHaveBeenCalled();
    expect(setItem).not.toHaveBeenCalled();
  });

  it("keeps the explicit mobile send control in a safe-area-aware footer", () => {
    const { sendButton } = renderComposer({
      mobile: true,
      initialValue: "Hola",
    });
    expect(sendButton.closest("form")).toHaveClass("sticky", "bottom-0");
    expect(sendButton.closest("form")?.className).toContain(
      "safe-area-inset-bottom",
    );
  });

  it("moves the mobile controls above the visual viewport keyboard inset", () => {
    const originalViewport = Object.getOwnPropertyDescriptor(
      window,
      "visualViewport",
    );
    let resizeListener: (() => void) | undefined;
    const viewport = {
      height: 500,
      offsetTop: 20,
      addEventListener: jest.fn((_event: string, listener: () => void) => {
        resizeListener = listener;
      }),
      removeEventListener: jest.fn(),
    };
    Object.defineProperty(window, "visualViewport", {
      configurable: true,
      value: viewport,
    });

    try {
      const { sendButton } = renderComposer({
        mobile: true,
        initialValue: "Hola",
      });
      const form = sendButton.closest("form");
      expect(form).toHaveStyle({
        bottom: `${Math.max(0, window.innerHeight - 520)}px`,
      });

      viewport.height = 450;
      act(() => resizeListener?.());
      expect(form).toHaveStyle({
        bottom: `${Math.max(0, window.innerHeight - 470)}px`,
      });
    } finally {
      if (originalViewport) {
        Object.defineProperty(window, "visualViewport", originalViewport);
      } else {
        Reflect.deleteProperty(window, "visualViewport");
      }
    }
  });
});
