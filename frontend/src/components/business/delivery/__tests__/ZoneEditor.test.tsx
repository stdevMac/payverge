/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ZoneEditor } from "../ZoneEditor";
import { asDollars } from "@/types/money";
import type { DeliveryZoneDto } from "@/api/delivery";

const t = (k: string) => k;

const baseZone: DeliveryZoneDto = {
  id: 1,
  name: "Downtown",
  delivery_fee: asDollars(4),
  minimum_order_amount: asDollars(15),
  estimated_time: 30,
  priority: 1,
  cutoff_buffer_minutes: 5,
  is_active: true,
  boundaries: {
    postal_codes: ["10001", "10002"],
    cities: ["New York"],
  },
};

describe("ZoneEditor", () => {
  it("renders form fields with zone values", () => {
    const { container } = render(
      <ZoneEditor zone={baseZone} onChange={jest.fn()} tString={t} />,
    );
    const inputs = container.querySelectorAll("input");
    const values = Array.from(inputs).map((i) => i.value);
    expect(values).toContain("Downtown");
    expect(values).toContain("4");
    expect(values).toContain("15");
    expect(values).toContain("30");
    expect(values).toContain("1");
  });

  it("renders postal codes as comma-joined text", () => {
    const { container } = render(
      <ZoneEditor zone={baseZone} onChange={jest.fn()} tString={t} />,
    );
    const inputs = container.querySelectorAll("input");
    const values = Array.from(inputs).map((i) => i.value);
    expect(values.some((v) => v.includes("10001"))).toBe(true);
  });

  it("fires onChange when name is updated", () => {
    const onChange = jest.fn();
    render(<ZoneEditor zone={baseZone} onChange={onChange} tString={t} />);
    const nameInput = screen.getByDisplayValue("Downtown");
    fireEvent.change(nameInput, { target: { value: "Midtown" } });
    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ name: "Midtown" }),
    );
  });

  it("fires onChange when is_active switch is toggled", async () => {
    const user = userEvent.setup();
    const onChange = jest.fn();
    render(<ZoneEditor zone={baseZone} onChange={onChange} tString={t} />);
    const toggle = screen.getByRole("switch");
    await user.click(toggle);
    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ is_active: false }),
    );
  });

  it("fires onChange when delivery_fee changes", async () => {
    const user = userEvent.setup();
    const onChange = jest.fn();
    render(<ZoneEditor zone={baseZone} onChange={onChange} tString={t} />);
    const feeInput = screen.getByDisplayValue("4") as HTMLInputElement;
    await user.clear(feeInput);
    await user.type(feeInput, "6");
    fireEvent.blur(feeInput);
    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({ delivery_fee: 6 }),
    );
  });

  it("keeps commas while typing and only commits the array on blur", () => {
    const onChange = jest.fn();
    render(<ZoneEditor zone={baseZone} onChange={onChange} tString={t} />);
    const postal = screen.getByDisplayValue("10001, 10002") as HTMLInputElement;

    // Mid-type with a trailing comma — must not normalize yet (old path
    // ate the comma by immediately textToArray → join).
    fireEvent.change(postal, { target: { value: "10001, 10002," } });
    expect(postal.value).toBe("10001, 10002,");
    expect(onChange).not.toHaveBeenCalled();

    fireEvent.blur(postal);
    expect(onChange).toHaveBeenCalledWith(
      expect.objectContaining({
        boundaries: expect.objectContaining({
          postal_codes: ["10001", "10002"],
        }),
      }),
    );
  });
});
