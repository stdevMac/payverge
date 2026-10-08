"use client";

import { useEffect } from "react";

export interface KeyboardShortcutHandlers {
  onUndo?: () => void;
  onRedo?: () => void;
  onCopy?: () => void;
  onPaste?: () => void;
  onDuplicate?: () => void;
  onDelete?: () => void;
  onSelectAll?: () => void;
  onNudge?: (dx: number, dy: number) => void;
  onEscape?: () => void;
  onSave?: () => void;
  enabled?: boolean;
}

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false;
  const tag = target.tagName;
  if (tag === "INPUT" || tag === "TEXTAREA" || tag === "SELECT") return true;
  if (target.isContentEditable) return true;
  return Boolean(target.closest("[contenteditable=true]"));
}

export function useKeyboardShortcuts(handlers: KeyboardShortcutHandlers) {
  const {
    onUndo,
    onRedo,
    onCopy,
    onPaste,
    onDuplicate,
    onDelete,
    onSelectAll,
    onNudge,
    onEscape,
    onSave,
    enabled = true,
  } = handlers;

  useEffect(() => {
    if (!enabled) return;

    const onKeyDown = (e: KeyboardEvent) => {
      if (isEditableTarget(e.target)) {
        if (e.key === "Escape") onEscape?.();
        return;
      }

      const mod = e.metaKey || e.ctrlKey;
      const shift = e.shiftKey;
      const step = shift ? 10 * 10 : 10; // mm: 10 or 100 (shift=10x of base 10)

      if (mod && e.key.toLowerCase() === "z" && !shift) {
        e.preventDefault();
        onUndo?.();
        return;
      }
      if (mod && (e.key.toLowerCase() === "y" || (e.key.toLowerCase() === "z" && shift))) {
        e.preventDefault();
        onRedo?.();
        return;
      }
      if (mod && e.key.toLowerCase() === "c") {
        e.preventDefault();
        onCopy?.();
        return;
      }
      if (mod && e.key.toLowerCase() === "v") {
        e.preventDefault();
        onPaste?.();
        return;
      }
      if (mod && e.key.toLowerCase() === "d") {
        e.preventDefault();
        onDuplicate?.();
        return;
      }
      if (mod && e.key.toLowerCase() === "a") {
        e.preventDefault();
        onSelectAll?.();
        return;
      }
      if (mod && e.key.toLowerCase() === "s") {
        e.preventDefault();
        onSave?.();
        return;
      }
      if (e.key === "Delete" || e.key === "Backspace") {
        e.preventDefault();
        onDelete?.();
        return;
      }
      if (e.key === "Escape") {
        onEscape?.();
        return;
      }
      if (e.key === "ArrowLeft") {
        e.preventDefault();
        onNudge?.(-step, 0);
        return;
      }
      if (e.key === "ArrowRight") {
        e.preventDefault();
        onNudge?.(step, 0);
        return;
      }
      if (e.key === "ArrowUp") {
        e.preventDefault();
        onNudge?.(0, -step);
        return;
      }
      if (e.key === "ArrowDown") {
        e.preventDefault();
        onNudge?.(0, step);
        return;
      }
    };

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [
    enabled,
    onUndo,
    onRedo,
    onCopy,
    onPaste,
    onDuplicate,
    onDelete,
    onSelectAll,
    onNudge,
    onEscape,
    onSave,
  ]);
}
