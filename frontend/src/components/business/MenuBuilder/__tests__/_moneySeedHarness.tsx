/**
 * Test-only Edit Item stand-in that surfaces the seeded money strings
 * MenuBuilder/index.tsx passes after populateItemForm.
 */
import React from "react";

export function editHarness(props: Record<string, unknown>) {
  if (!props.isOpen) return null;
  return (
    <div>
      <span data-testid="seeded-price">{String(props.itemPrice ?? "")}</span>
      <span data-testid="seeded-cogs">{String(props.itemCogs ?? "")}</span>
    </div>
  );
}
