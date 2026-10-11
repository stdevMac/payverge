"use client";

import React, { useLayoutEffect, useRef } from "react";
import { Select, type SelectProps } from "@nextui-org/react";
import { applyNamedControl, formatControlName } from "./namedControl";

/**
 * NextUI Select trigger buttons announce the selected value only. Stamp
 * "purpose, current value" onto the trigger so operators hear the field
 * they are changing (#446).
 */
export function NamedSelect({
  name,
  valueLabel,
  descriptionId,
  children,
  ...props
}: SelectProps & {
  name: string;
  valueLabel: string;
  descriptionId?: string;
}) {
  const rootRef = useRef<HTMLDivElement>(null);
  const composed = formatControlName(name, valueLabel);

  useLayoutEffect(() => {
    const trigger = () =>
      rootRef.current?.querySelector<HTMLElement>('[data-slot="trigger"]') ??
      rootRef.current?.querySelector<HTMLElement>("button") ??
      null;

    const stamp = () =>
      applyNamedControl(trigger(), composed, {
        descriptionId,
        replaceLabelledBy: true,
      });

    stamp();
    const node = trigger();
    if (!node) return undefined;

    const observer = new MutationObserver(() => {
      observer.disconnect();
      stamp();
      const next = trigger();
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
  }, [composed, descriptionId]);

  return (
    <div ref={rootRef}>
      <Select {...props} aria-label={composed}>
        {children}
      </Select>
    </div>
  );
}
