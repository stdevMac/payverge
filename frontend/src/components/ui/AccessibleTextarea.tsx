"use client";

import { Textarea } from "@nextui-org/react";
import { forwardRef, useCallback, useLayoutEffect, useRef, type ComponentProps } from "react";
import { stripDuplicateFieldName } from "./AccessibleInput";

type TextAreaProps = ComponentProps<typeof Textarea>;

export const AccessibleTextarea = forwardRef<HTMLTextAreaElement, TextAreaProps>(
  function AccessibleTextarea(props, ref) {
    const innerRef = useRef<HTMLTextAreaElement | null>(null);

    const setRefs = useCallback(
      (node: HTMLTextAreaElement | null) => {
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

    return <Textarea {...props} ref={setRefs} />;
  },
);

AccessibleTextarea.displayName = "AccessibleTextarea";
