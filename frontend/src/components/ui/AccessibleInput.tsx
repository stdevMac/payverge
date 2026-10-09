"use client";

import { Input, type InputProps } from "@nextui-org/react";
import { forwardRef, useCallback, useLayoutEffect, useRef } from "react";

/**
 * NextUI Input copies `label` into aria-label while React Aria also
 * aria-labelledby's the visible label (and the input id). Screen readers
 * then announce "Email Email". Keep the visible label; drop the extra name.
 */
export function stripDuplicateFieldName(field: HTMLElement | null): void {
  if (
    !(field instanceof HTMLInputElement) &&
    !(field instanceof HTMLTextAreaElement)
  ) {
    return;
  }
  const labelledBy = field.getAttribute("aria-labelledby");
  if (!labelledBy || !field.hasAttribute("aria-label")) return;
  field.removeAttribute("aria-label");
  const ids = labelledBy.split(/\s+/).filter((id) => id && id !== field.id);
  if (ids.length > 0) {
    field.setAttribute("aria-labelledby", ids.join(" "));
  } else {
    field.removeAttribute("aria-labelledby");
  }
}

const LTR_INPUT_TYPES = new Set(["email", "tel", "url"]);

export const AccessibleInput = forwardRef<HTMLInputElement, InputProps>(
  function AccessibleInput(props, ref) {
    const innerRef = useRef<HTMLInputElement | null>(null);

    const setRefs = useCallback(
      (node: HTMLInputElement | null) => {
        innerRef.current = node;
        if (typeof ref === "function") ref(node);
        else if (ref) ref.current = node;
        stripDuplicateFieldName(node);
      },
      [ref],
    );

    useLayoutEffect(() => {
      const node = innerRef.current;
      stripDuplicateFieldName(node);
      if (!node) return undefined;
      const observer = new MutationObserver(() => {
        stripDuplicateFieldName(node);
      });
      observer.observe(node, {
        attributes: true,
        attributeFilter: ["aria-label", "aria-labelledby"],
      });
      return () => observer.disconnect();
    });

    // Emails, phone numbers and URLs are left-to-right strings even inside an
    // RTL page; without dir="ltr" the bidi algorithm reorders "+" and "@".
    const dir =
      props.dir ??
      (props.type && LTR_INPUT_TYPES.has(props.type) ? "ltr" : undefined);

    return <Input {...props} dir={dir} ref={setRefs} />;
  },
);

AccessibleInput.displayName = "AccessibleInput";
