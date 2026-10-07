/**
 * Test-only modal harnesses for MenuBuilder.priceParse.test.tsx.
 *
 * Lives in its own module because jest.mock() factory functions are hoisted and
 * may not close over out-of-scope test variables. These stand-ins receive the
 * REAL itemPrice / setItemPrice / submit handlers wired by MenuBuilder/index.tsx
 * so the actual submit path (parseLocaleDecimal -> asDollars) is exercised.
 */
import React from "react";

function Harness({
  props,
  submitProp,
}: {
  props: Record<string, unknown>;
  submitProp: "onAddItem" | "onUpdateItem";
}) {
  if (!props.isOpen) return null;
  const setItemPrice = props.setItemPrice as (v: string) => void;
  const submit = props[submitProp] as () => void;
  return (
    <div>
      <button onClick={() => setItemPrice("5,50")}>{`set-price-${submitProp}`}</button>
      <button onClick={() => submit()}>{`submit-${submitProp}`}</button>
    </div>
  );
}

export function addHarness(props: Record<string, unknown>) {
  return <Harness props={props} submitProp="onAddItem" />;
}

export function editHarness(props: Record<string, unknown>) {
  return <Harness props={props} submitProp="onUpdateItem" />;
}
