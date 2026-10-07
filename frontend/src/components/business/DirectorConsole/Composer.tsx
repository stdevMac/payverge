"use client";

import React from "react";
import { Button, Textarea } from "@nextui-org/react";
import { Square } from "lucide-react";

import SlashPalette from "./SlashPalette";
import {
  filterSlashItems,
  insertSlashSelection,
  isSlashDraft,
  slashQueryFromValue,
} from "./slashPaletteUtils";

interface ComposerProps {
  value: string;
  onChange: (next: string) => void;
  onSend: () => Promise<void> | void;
  onAbort: () => void;
  sending: boolean;
  assistantName: string;
  placeholder: string;
  sendLabel: string;
  sendingLabel: string;
  stopLabel?: string;
  hint: string;
  disabled?: boolean;
  paletteItems?: string[];
  /**
   * Called when a slash item is chosen. Must insert into the composer only —
   * do not auto-send (L4-12c). Default parent behavior inserts text.
   */
  onPaletteSelect?: (item: string) => void;
  /** Localized aria-label for the slash-command palette listbox. */
  slashPaletteAria?: string;
}

/**
 * Composer — textarea + send/stop button + ⌘↵ keybinding.
 *
 * Owns the textarea ref so focus can be returned after every send,
 * including the async case. Plain Enter inserts a newline; ⌘↵ or ⌃↵ sends.
 *
 * Slash palette (L4-12): open on leading "/", sticky Esc dismiss per token,
 * filter by query after "/", insert-not-send on select, keyboard scoped to
 * the composer root (not document).
 */
const Composer = React.forwardRef<HTMLTextAreaElement, ComposerProps>(
  function Composer(props, externalRef) {
    const {
      value,
      onChange,
      onSend,
      onAbort,
      sending,
      placeholder,
      sendLabel,
      sendingLabel,
      stopLabel = "Stop",
      hint,
      disabled,
      paletteItems,
      onPaletteSelect,
      slashPaletteAria,
    } = props;
    const localRef = React.useRef<HTMLTextAreaElement | null>(null);
    const rootRef = React.useRef<HTMLDivElement | null>(null);
    const [paletteOpen, setPaletteOpen] = React.useState(false);
    // Esc dismiss is sticky for the current slash token; cleared when the
    // leading "/" draft ends or the token text changes.
    const [dismissedToken, setDismissedToken] = React.useState<string | null>(
      null,
    );

    const slashQuery = slashQueryFromValue(value);
    const filteredItems = React.useMemo(
      () => filterSlashItems(paletteItems ?? [], slashQuery ?? ""),
      [paletteItems, slashQuery],
    );

    React.useEffect(() => {
      if (!isSlashDraft(value)) {
        setPaletteOpen(false);
        setDismissedToken(null);
        return;
      }
      const token = slashQueryFromValue(value) ?? "";
      if (dismissedToken !== null && dismissedToken === token) {
        setPaletteOpen(false);
        return;
      }
      // Token changed after a dismiss → allow reopen.
      if (dismissedToken !== null && dismissedToken !== token) {
        setDismissedToken(null);
      }
      setPaletteOpen(true);
    }, [value, dismissedToken]);

    React.useImperativeHandle(
      externalRef,
      () => localRef.current as HTMLTextAreaElement,
    );

    const focusTextarea = React.useCallback(() => {
      localRef.current?.focus();
    }, []);

    const handlePaletteClose = React.useCallback(() => {
      const token = slashQueryFromValue(value) ?? "";
      setDismissedToken(token);
      setPaletteOpen(false);
    }, [value]);

    const handlePaletteSelect = React.useCallback(
      (item: string) => {
        setPaletteOpen(false);
        setDismissedToken(null);
        // L4-12c: insert-not-send — put the prompt text into the composer.
        const next = insertSlashSelection(value, item);
        onChange(next);
        // Optional parent hook (defaults should not auto-send).
        setTimeout(() => onPaletteSelect?.(item), 0);
        focusTextarea();
      },
      [value, onChange, onPaletteSelect, focusTextarea],
    );

    const handleSend = React.useCallback(async () => {
      if (disabled || sending || !value.trim()) return;
      try {
        await onSend();
      } finally {
        focusTextarea();
      }
    }, [disabled, sending, value, onSend, focusTextarea]);

    const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      // The slash palette listens natively on the composer root, which runs
      // before this delegated handler. If it (or anything else) already
      // consumed the key, sending here would fire a second action for one
      // keystroke.
      if (e.defaultPrevented) return;
      if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        void handleSend();
      }
    };

    return (
      <div
        ref={rootRef}
        data-testid="dc-composer"
        className="flex-none bg-white border-t border-warm-200 px-2 py-3 md:px-3 flex flex-col gap-2 md:flex-row md:items-end"
      >
        <div className="relative min-w-0 flex-1">
          <SlashPalette
            open={paletteOpen && filteredItems.length > 0}
            items={filteredItems}
            onSelect={handlePaletteSelect}
            onClose={handlePaletteClose}
            ariaLabel={slashPaletteAria}
            containerRef={rootRef}
          />
          <Textarea
            ref={(node) => {
              localRef.current =
                (node as unknown as HTMLTextAreaElement) ?? null;
            }}
            value={value}
            onValueChange={onChange}
            onKeyDown={
              handleKeyDown as unknown as React.KeyboardEventHandler<HTMLInputElement>
            }
            minRows={2}
            maxRows={5}
            placeholder={placeholder}
            variant="bordered"
            classNames={{
              base: "flex-1",
              inputWrapper:
                "border border-warm-200 shadow-none bg-white rounded-xl " +
                "data-[hover=true]:border-warm-300 " +
                "group-data-[focus=true]:border-brand/40 group-data-[focus=true]:ring-2 " +
                "group-data-[focus=true]:ring-brand/15",
              input: "text-body text-ink-900 placeholder:text-ink-400",
            }}
            isDisabled={disabled}
          />
          <div className="mt-1.5 pl-1 text-xs text-ink-500">{hint}</div>
        </div>
        {sending ? (
          <Button
            color="default"
            size="lg"
            variant="flat"
            startContent={<Square className="w-4 h-4" />}
            onPress={onAbort}
            className="md:self-stretch md:w-auto w-full font-semibold"
          >
            {stopLabel}
          </Button>
        ) : (
          <Button
            color="primary"
            size="lg"
            isDisabled={disabled || !value.trim()}
            onPress={() => void handleSend()}
            className="md:self-stretch md:w-auto w-full font-semibold"
          >
            {sending ? sendingLabel : sendLabel}
          </Button>
        )}
      </div>
    );
  },
);

export default Composer;
