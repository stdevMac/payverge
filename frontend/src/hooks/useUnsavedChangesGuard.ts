"use client";

import { useEffect } from "react";
import { setDirty } from "./unsavedChangesRegistry";

/**
 * useUnsavedChangesGuard — wires a form's `dirty` state into two guards:
 *
 *  1. A native `beforeunload` handler that prompts the browser's built-in
 *     "leave site?" dialog while there are unsaved edits (tab close, reload,
 *     external navigation). Attached only while dirty; removed the moment the
 *     form goes clean or the component unmounts.
 *
 *  2. The shared {@link unsavedChangesRegistry}, keyed by the given `id`, so the
 *     in-app dashboard tab-navigation handler can synchronously consult
 *     `hasUnsavedChanges()` and show a ConfirmationModal before switching tabs.
 *
 * @param dirty whether the form currently has unsaved changes.
 * @param id    a STABLE, unique identifier for this form (used as the registry
 *              key). Must be constant across renders for a given form instance.
 */
export function useUnsavedChangesGuard(dirty: boolean, id: string): void {
  // Publish dirty state into the shared registry. On unmount, always clear this
  // id so a stale "dirty" flag from an unmounted form can't block later prompts.
  useEffect(() => {
    setDirty(id, dirty);
    return () => {
      setDirty(id, false);
    };
  }, [dirty, id]);

  // Native beforeunload guard — only active while dirty.
  useEffect(() => {
    if (!dirty) return;

    const handleBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      // Legacy browsers require returnValue to be set to trigger the prompt.
      event.returnValue = "";
      return "";
    };

    window.addEventListener("beforeunload", handleBeforeUnload);
    return () => {
      window.removeEventListener("beforeunload", handleBeforeUnload);
    };
  }, [dirty]);
}
