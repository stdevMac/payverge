import { useLayoutEffect, useState } from "react";

const EVENT = "payverge:operator-access-error";

function readOperatorAccessError(): boolean {
  return (
    typeof document !== "undefined" &&
    document.documentElement.dataset.operatorAccessError === "1"
  );
}

export function setOperatorAccessError(active: boolean): void {
  if (typeof document === "undefined") return;
  if (active) {
    document.documentElement.dataset.operatorAccessError = "1";
  } else {
    delete document.documentElement.dataset.operatorAccessError;
  }
  document.dispatchEvent(new CustomEvent(EVENT, { detail: active }));
}

export function useOperatorAccessError(): boolean {
  const [active, setActive] = useState(readOperatorAccessError);

  useLayoutEffect(() => {
    setActive(readOperatorAccessError());
    const onChange = (event: Event) => {
      setActive(Boolean((event as CustomEvent<boolean>).detail));
    };
    document.addEventListener(EVENT, onChange);
    return () => document.removeEventListener(EVENT, onChange);
  }, []);

  return active;
}
