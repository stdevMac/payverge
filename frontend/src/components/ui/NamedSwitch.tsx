"use client";

import React, { useLayoutEffect, useRef } from "react";
import { Switch, type SwitchProps } from "@nextui-org/react";
import { applyNamedControl } from "./namedControl";

/**
 * NextUI Switch always emits aria-labelledby pointing at an empty generated
 * label node when there are no children. That wins over aria-label, so
 * screen readers hear an unnamed switch (#446). After mount, stamp the
 * visible purpose onto the role=switch input and drop the empty labelledby.
 */
export function NamedSwitch({
  name,
  labelId,
  descriptionId,
  ...props
}: SwitchProps & {
  name: string;
  labelId?: string;
  descriptionId?: string;
}) {
  const rootRef = useRef<HTMLDivElement>(null);

  useLayoutEffect(() => {
    const input = () =>
      rootRef.current?.querySelector<HTMLElement>('[role="switch"]') ?? null;

    const stamp = () => {
      const node = input();
      applyNamedControl(node, name, { labelId, descriptionId });
      if (node) {
        node.setAttribute(
          "aria-checked",
          props.isSelected ? "true" : "false",
        );
      }
    };

    stamp();
    const node = input();
    if (!node) return undefined;

    const observer = new MutationObserver(() => {
      observer.disconnect();
      stamp();
      const next = input();
      if (next) {
        observer.observe(next, {
          attributes: true,
          attributeFilter: ["aria-label", "aria-labelledby", "aria-describedby"],
        });
      }
    });
    observer.observe(node, {
      attributes: true,
      attributeFilter: ["aria-label", "aria-labelledby", "aria-describedby"],
    });
    return () => observer.disconnect();
  }, [name, labelId, descriptionId, props.isSelected]);

  return (
    <div ref={rootRef}>
      <Switch {...props} aria-label={name} />
    </div>
  );
}
