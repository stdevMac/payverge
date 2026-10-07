"use client";

import React, {
  useCallback,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
} from "react";

export interface AssistantComposerLabels {
  textarea: string;
  send: string;
  busy: string;
  characterCount: (current: number, maximum: number) => string;
}

interface AssistantComposerBaseProps {
  labels: AssistantComposerLabels;
  maxLength: number;
  onSend: (prompt: string) => boolean | void | Promise<boolean | void>;
  placeholder?: string;
  focusOnMount?: boolean;
  busy?: boolean;
  disabled?: boolean;
  mobile?: boolean;
  draftKey?: string;
  initialValue?: string;
  errorMessage?: string | null;
}

export type AssistantComposerProps = AssistantComposerBaseProps &
  (
    | {
        value: string;
        onValueChange: (value: string) => void;
      }
    | {
        value?: never;
        onValueChange?: never;
      }
  );

const MAX_VISIBLE_LINES = 5;

function boundedDraft(value: string, maxLength: number) {
  return value.slice(0, Math.max(0, maxLength));
}

export function AssistantComposer({
  labels,
  maxLength,
  onSend,
  placeholder,
  focusOnMount = false,
  busy = false,
  disabled = false,
  mobile = false,
  draftKey,
  initialValue = "",
  errorMessage,
  value,
  onValueChange,
}: AssistantComposerProps) {
  const [internalDraft, setInternalDraft] = useState(() =>
    boundedDraft(initialValue, maxLength),
  );
  const [sending, setSending] = useState(false);
  const [mobileBottom, setMobileBottom] = useState(0);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const isComposingRef = useRef(false);
  const isSubmittingRef = useRef(false);
  const restoreFocusRef = useRef(false);
  const latestControlledDraftRef = useRef<string>();
  const initialValueRef = useRef(initialValue);
  const maxLengthRef = useRef(maxLength);
  initialValueRef.current = initialValue;
  maxLengthRef.current = maxLength;
  const countId = useId();
  const errorId = useId();
  const isControlled = value !== undefined;
  const currentDraft = boundedDraft(
    isControlled ? value : internalDraft,
    maxLength,
  );
  latestControlledDraftRef.current = isControlled ? currentDraft : undefined;
  const isBusy = busy || sending;
  const trimmedDraft = currentDraft.trim();
  const showCount =
    maxLength > 0 && currentDraft.length >= Math.ceil(maxLength * 0.8);

  useEffect(() => {
    if (isControlled) return;

    const nextInitialValue = initialValueRef.current;
    const nextMaxLength = maxLengthRef.current;
    if (!draftKey) {
      setInternalDraft(boundedDraft(nextInitialValue, nextMaxLength));
      return;
    }

    try {
      const saved = window.localStorage.getItem(draftKey);
      const parsed = saved === null ? nextInitialValue : JSON.parse(saved);
      setInternalDraft(
        boundedDraft(
          typeof parsed === "string" ? parsed : nextInitialValue,
          nextMaxLength,
        ),
      );
    } catch {
      setInternalDraft(boundedDraft(nextInitialValue, nextMaxLength));
    }
  }, [draftKey, isControlled]);

  useEffect(() => {
    if (isControlled) {
      const nextValue = boundedDraft(value, maxLength);
      if (nextValue !== value) onValueChange(nextValue);
      return;
    }

    setInternalDraft((draft) => boundedDraft(draft, maxLength));
  }, [isControlled, maxLength, onValueChange, value]);

  useLayoutEffect(() => {
    const textarea = textareaRef.current;
    if (!textarea) return;

    textarea.style.height = "auto";
    const computed = window.getComputedStyle(textarea);
    const lineHeight = Number.parseFloat(computed.lineHeight) || 24;
    const padding =
      (Number.parseFloat(computed.paddingTop) || 0) +
      (Number.parseFloat(computed.paddingBottom) || 0);
    const maxHeight = lineHeight * MAX_VISIBLE_LINES + padding;
    const nextHeight = Math.min(textarea.scrollHeight, maxHeight);
    textarea.style.height = `${nextHeight}px`;
    textarea.style.overflowY =
      textarea.scrollHeight > maxHeight ? "auto" : "hidden";
  }, [currentDraft]);

  useLayoutEffect(() => {
    if (focusOnMount && !isBusy && !disabled) {
      textareaRef.current?.focus();
    }
  }, [disabled, focusOnMount, isBusy]);

  useEffect(() => {
    if (!focusOnMount || isBusy || disabled) return;
    const focusID = window.setTimeout(() => textareaRef.current?.focus(), 0);
    return () => window.clearTimeout(focusID);
  }, [disabled, focusOnMount, isBusy]);

  useLayoutEffect(() => {
    if (isBusy || disabled || !restoreFocusRef.current) return;
    restoreFocusRef.current = false;
    textareaRef.current?.focus();
  }, [disabled, isBusy]);

  useEffect(() => {
    if (!mobile || !window.visualViewport) {
      setMobileBottom(0);
      return;
    }

    const viewport = window.visualViewport;
    const updateBottom = () => {
      setMobileBottom(
        Math.max(0, window.innerHeight - viewport.height - viewport.offsetTop),
      );
    };

    updateBottom();
    viewport.addEventListener("resize", updateBottom);
    viewport.addEventListener("scroll", updateBottom);
    return () => {
      viewport.removeEventListener("resize", updateBottom);
      viewport.removeEventListener("scroll", updateBottom);
    };
  }, [mobile]);

  const persistDraft = useCallback(
    (value: string) => {
      if (!draftKey) return;
      try {
        window.localStorage.setItem(draftKey, JSON.stringify(value));
      } catch {
        // Draft persistence is optional; storage failures must not block typing.
      }
    },
    [draftKey],
  );

  const clearPersistedDraft = useCallback(() => {
    if (!draftKey) return;
    try {
      window.localStorage.removeItem(draftKey);
    } catch {
      // Draft persistence is optional; storage failures must not block sending.
    }
  }, [draftKey]);

  const submit = useCallback(async () => {
    if (
      disabled ||
      busy ||
      isSubmittingRef.current ||
      trimmedDraft.length === 0
    ) {
      return;
    }

    const submittedDraft = currentDraft;
    isSubmittingRef.current = true;
    setSending(true);
    try {
      const result = await onSend(submittedDraft.trim());
      if (result !== false) {
        if (isControlled) {
          if (latestControlledDraftRef.current === submittedDraft) {
            onValueChange("");
          }
        } else {
          setInternalDraft("");
          clearPersistedDraft();
        }
      }
    } catch {
      // The caller owns localized error messaging; retain the draft for retry.
    } finally {
      isSubmittingRef.current = false;
      restoreFocusRef.current = true;
      setSending(false);
      if (!textareaRef.current?.disabled) {
        textareaRef.current?.focus();
      }
    }
  }, [
    busy,
    clearPersistedDraft,
    currentDraft,
    disabled,
    isControlled,
    onSend,
    onValueChange,
    trimmedDraft,
  ]);

  const describedBy = [
    showCount ? countId : null,
    errorMessage ? errorId : null,
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <form
      className="sticky bottom-0 shrink-0 flex items-end gap-2 border-t border-warm-200 bg-white px-3 pt-3 pb-[max(0.75rem,env(safe-area-inset-bottom))]"
      style={{ bottom: mobile ? `${mobileBottom}px` : undefined }}
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      <div className="min-w-0 flex-1">
        <textarea
          ref={textareaRef}
          aria-describedby={describedBy || undefined}
          aria-invalid={Boolean(errorMessage)}
          aria-label={labels.textarea}
          className="block min-h-11 w-full resize-none rounded-xl border border-warm-300 bg-white px-3 py-2 text-body text-ink-950 outline-none transition placeholder:text-ink-500 focus:border-brand focus:ring-2 focus:ring-brand/20 disabled:cursor-not-allowed disabled:bg-warm-100 disabled:text-ink-500"
          disabled={disabled || isBusy}
          maxLength={maxLength}
          placeholder={placeholder ?? labels.textarea}
          rows={1}
          value={currentDraft}
          onChange={(event) => {
            const nextDraft = boundedDraft(event.target.value, maxLength);
            if (isControlled) {
              onValueChange(nextDraft);
            } else {
              setInternalDraft(nextDraft);
              persistDraft(nextDraft);
            }
          }}
          onCompositionEnd={() => {
            isComposingRef.current = false;
          }}
          onCompositionStart={() => {
            isComposingRef.current = true;
          }}
          onKeyDown={(event) => {
            if (
              event.key !== "Enter" ||
              event.shiftKey ||
              mobile ||
              event.nativeEvent.isComposing ||
              event.nativeEvent.keyCode === 229 ||
              event.nativeEvent.which === 229 ||
              isComposingRef.current
            ) {
              return;
            }
            event.preventDefault();
            void submit();
          }}
        />
        {showCount ? (
          <p
            id={countId}
            aria-live="polite"
            className="mt-1 text-end text-label text-ink-600"
          >
            {labels.characterCount(currentDraft.length, maxLength)}
          </p>
        ) : null}
        {errorMessage ? (
          <p
            id={errorId}
            role="alert"
            className="mt-1 text-label text-rose-700"
          >
            {errorMessage}
          </p>
        ) : null}
      </div>
      <button
        type="submit"
        aria-label={isBusy ? labels.busy : labels.send}
        className="min-h-11 shrink-0 rounded-xl bg-brand px-4 py-2 font-semibold text-white transition hover:bg-brand-dark focus:outline-none focus:ring-2 focus:ring-brand/30 focus:ring-offset-2 disabled:cursor-not-allowed disabled:bg-warm-300 disabled:text-ink-600"
        disabled={disabled || isBusy || trimmedDraft.length === 0}
      >
        {isBusy ? labels.busy : labels.send}
      </button>
    </form>
  );
}
