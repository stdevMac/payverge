import { useEffect, useState } from "react";

/**
 * L3-13 / S-10: remount key for NextUI (and hand-rolled) modals.
 *
 * Reopening a modal during its exit fade reuses the previous React subtree
 * and resurrects stale form values. Bump a counter each time `isOpen` becomes
 * true and put it on `ModalContent` (or the dialog body) as `key={openKey}`.
 */
export function useModalOpenKey(isOpen: boolean): number {
  const [openKey, setOpenKey] = useState(0);
  useEffect(() => {
    if (isOpen) {
      setOpenKey((k) => k + 1);
    }
  }, [isOpen]);
  return openKey;
}
