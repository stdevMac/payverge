/**
 * A3b: zone money fields accept per-keystroke decimals (incl. Spanish comma)
 * without collapsing "12." / "12,5" via String(number) round-trip.
 *
 * @jest-environment jsdom
 */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { ZoneEditor } from "./ZoneEditor";
import type { DeliveryZoneDto } from "@/api/delivery";
import { asDollars } from "@/types/money";

function typeChars(el: HTMLElement, chars: string) {
  let acc = (el as HTMLInputElement).value || "";
  for (const ch of chars) {
    acc = acc + ch;
    fireEvent.change(el, { target: { value: acc } });
  }
}

const baseZone: DeliveryZoneDto = {
  id: 1,
  business_id: 1,
  name: "Centro",
  priority: 1,
  delivery_fee: asDollars(0),
  minimum_order_amount: asDollars(0),
  estimated_time: 30,
  cutoff_buffer_minutes: 0,
  is_active: false,
  boundaries: { postal_codes: [], cities: [] },
} as DeliveryZoneDto;

describe("A3b ZoneEditor money DecimalInput", () => {
  it("keeps 12. mid-keystroke and accepts 12,50 Spanish comma", () => {
    const onChange = jest.fn();
    render(
      <ZoneEditor
        zone={baseZone}
        onChange={onChange}
        tString={(k) => k}
        currencyPrefix="$"
      />,
    );
    const fee = screen.getByTestId("zone-delivery-fee");
    fireEvent.change(fee, { target: { value: "" } });
    typeChars(fee, "1");
    expect((fee as HTMLInputElement).value).toBe("1");
    typeChars(fee, "2");
    expect((fee as HTMLInputElement).value).toBe("12");
    typeChars(fee, ".");
    // Separator must survive (the type=number + String(n) bug erased it).
    expect((fee as HTMLInputElement).value).toBe("12.");
    typeChars(fee, "5");
    expect((fee as HTMLInputElement).value).toBe("12.5");
    fireEvent.blur(fee);

    fireEvent.change(fee, { target: { value: "" } });
    typeChars(fee, "12,50");
    expect((fee as HTMLInputElement).value).toBe("12,50");
    fireEvent.blur(fee);
    // Parent receives a parsed dollar amount on blur.
    expect(onChange).toHaveBeenCalled();
    const last = onChange.mock.calls.at(-1)?.[0] as DeliveryZoneDto;
    expect(Number(last.delivery_fee)).toBeCloseTo(12.5, 5);
  });
});
