/** @jest-environment jsdom */
import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { ZonesSection } from "../ZonesSection";
import { asDollars } from "@/types/money";
import type { DeliveryZoneDto } from "@/api/delivery";

const t = (k: string) => k;

const zone1: DeliveryZoneDto = {
  id: 1,
  name: "Downtown",
  delivery_fee: asDollars(4),
  minimum_order_amount: asDollars(15),
  estimated_time: 30,
  priority: 1,
  cutoff_buffer_minutes: 0,
  is_active: true,
};

const zone2: DeliveryZoneDto = {
  id: 2,
  name: "Uptown",
  delivery_fee: asDollars(6),
  minimum_order_amount: asDollars(20),
  estimated_time: 45,
  priority: 2,
  cutoff_buffer_minutes: 0,
  is_active: false,
};

describe("ZonesSection", () => {
  it("renders card title", () => {
    render(
      <ZonesSection zones={[]} onChange={jest.fn()} tString={t} />,
    );
    expect(screen.getByText("focused.zones.cardTitle")).toBeInTheDocument();
  });

  it("renders empty state when no zones", () => {
    render(
      <ZonesSection zones={[]} onChange={jest.fn()} tString={t} />,
    );
    expect(screen.getByText("focused.zones.noZones")).toBeInTheDocument();
  });

  it("renders zone names in list", () => {
    render(
      <ZonesSection zones={[zone1, zone2]} onChange={jest.fn()} tString={t} />,
    );
    expect(screen.getByText("Downtown")).toBeInTheDocument();
    expect(screen.getByText("Uptown")).toBeInTheDocument();
  });

  // DEL-OP-3: every new zone used to default to priority 1, so adding a second
  // active zone always tripped the backend "duplicate active zone priority"
  // validation. New zones must default to max(existing priorities) + 1.
  it("defaults a new zone's priority to max(existing) + 1 (DEL-OP-3)", () => {
    const onChange = jest.fn();
    render(
      <ZonesSection zones={[zone1, zone2]} onChange={onChange} tString={t} />,
    );
    fireEvent.click(screen.getByText("focused.zones.addZone"));
    expect(onChange).toHaveBeenCalledTimes(1);
    const next = onChange.mock.calls[0][0] as DeliveryZoneDto[];
    expect(next).toHaveLength(3);
    // zone1 has priority 1, zone2 has priority 2 → the new zone must be 3.
    expect(next[2].priority).toBe(3);
  });

  it("defaults the first zone's priority to 1", () => {
    const onChange = jest.fn();
    render(<ZonesSection zones={[]} onChange={onChange} tString={t} />);
    fireEvent.click(screen.getByText("focused.zones.addZone"));
    const next = onChange.mock.calls[0][0] as DeliveryZoneDto[];
    expect(next[0].priority).toBe(1);
  });

  it("fires onChange when add zone is pressed", () => {
    const onChange = jest.fn();
    render(
      <ZonesSection zones={[]} onChange={onChange} tString={t} />,
    );
    fireEvent.click(screen.getByText("focused.zones.addZone"));
    // L3-40: zones are born inactive — activation is an explicit, validated
    // step. This assertion previously locked in the active-by-default bug.
    expect(onChange).toHaveBeenCalledWith(
      expect.arrayContaining([expect.objectContaining({ is_active: false })]),
    );
  });

  it("confirms before removing a zone (no immediate onChange)", () => {
    const onChange = jest.fn();
    render(
      <ZonesSection zones={[zone1]} onChange={onChange} tString={t} />,
    );
    // The trash button opens a confirm modal — it must NOT drop the zone yet.
    const removeBtn = screen.getByRole("button", {
      name: "focused.zones.removeZone",
    });
    fireEvent.click(removeBtn);
    expect(onChange).not.toHaveBeenCalled();

    // Confirming actually removes the zone.
    fireEvent.click(screen.getByText("focused.zones.confirm.removeConfirm"));
    expect(onChange).toHaveBeenCalledWith([]);
  });

  it("does not remove a zone when the confirm is cancelled", () => {
    const onChange = jest.fn();
    render(
      <ZonesSection zones={[zone1]} onChange={onChange} tString={t} />,
    );
    fireEvent.click(
      screen.getByRole("button", { name: "focused.zones.removeZone" }),
    );
    fireEvent.click(screen.getByText("focused.zones.confirm.removeCancel"));
    expect(onChange).not.toHaveBeenCalled();
  });

  it("expands zone editor when zone row is clicked", () => {
    render(
      <ZonesSection zones={[zone1]} onChange={jest.fn()} tString={t} />,
    );
    const toggleBtn = screen.getByRole("button", { expanded: false });
    fireEvent.click(toggleBtn);
    // After expanding, the ZoneEditor form should appear with the zone name input
    expect(screen.getByDisplayValue("Downtown")).toBeInTheDocument();
  });
});
