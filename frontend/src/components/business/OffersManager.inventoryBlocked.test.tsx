/** @jest-environment jsdom */
/**
 * #835 — offer 13 "$5 Off the Steak Plate" comes back `is_active: true` AND
 * `inventory_blocked: true` while Premium Beef sits at 0 kg. The backend
 * computes `inventory_blocked` on GetOffers (see
 * backend/internal/server/offers_inventory_flag_test.go); the operator card
 * must not paint that row Active.
 *
 * OffersManager mounts a large menu/plugin tree, so — following the
 * OffersManager.dateRange.s5 pattern — this renders a thin harness over the
 * SHIPPED offerStatusChip decision (not a copy of it), plus a source contract
 * against OffersManager.tsx so the card cannot drift back to two chips.
 */
import React from "react";
import { render, screen } from "@testing-library/react";
import { Chip } from "@nextui-org/react";
import { AlertTriangle } from "lucide-react";
import type { Offer } from "@/api/business";
import { offerStatusChip } from "./offerStatusChip";

const messages: Record<string, string> = {
  "status.active": "Active",
  "status.inactive": "Inactive",
  inventoryBlockedChip: "Paused by inventory",
  inventoryBlockedTooltip:
    "The dish this offer targets can't be made right now, so guests don't see the offer.",
};

/** Mirror of the shipped status-chip cell (OffersManager.tsx, #835). */
function OfferStatusChips({
  offer,
}: {
  offer: Pick<Offer, "is_active" | "inventory_blocked" | "manual_active">;
}) {
  const status = offerStatusChip(offer);
  const chip = (
    <Chip
      size="sm"
      color={status.tone}
      variant="flat"
      startContent={
        status.tone === "warning" ? <AlertTriangle className="h-3 w-3" /> : undefined
      }
    >
      {messages[status.labelKey]}
    </Chip>
  );
  return status.tooltipKey ? (
    <span title={messages[status.tooltipKey]}>{chip}</span>
  ) : (
    chip
  );
}

describe("#835 offer inventory-blocked chip", () => {
  it("shows only the paused state for an Active offer whose dish is 86'd", () => {
    render(<OfferStatusChips offer={{ is_active: true, inventory_blocked: true }} />);
    expect(screen.getByText("Paused by inventory")).toBeInTheDocument();
    // The contradiction: an Active chip sitting next to the paused chip.
    expect(screen.queryByText("Active")).not.toBeInTheDocument();
    expect(
      screen.getByTitle(
        "The dish this offer targets can't be made right now, so guests don't see the offer.",
      ),
    ).toBeInTheDocument();
  });

  it("keeps Active for offers on sellable dishes (flag absent or false)", () => {
    const { rerender } = render(
      <OfferStatusChips offer={{ is_active: true, inventory_blocked: false }} />,
    );
    expect(screen.getByText("Active")).toBeInTheDocument();
    expect(screen.queryByText("Paused by inventory")).not.toBeInTheDocument();
    rerender(<OfferStatusChips offer={{ is_active: true }} />);
    expect(screen.getByText("Active")).toBeInTheDocument();
    expect(screen.queryByText("Paused by inventory")).not.toBeInTheDocument();
  });

  it("shows Inactive when the operator switched the offer off", () => {
    render(<OfferStatusChips offer={{ is_active: false, inventory_blocked: true }} />);
    expect(screen.getByText("Inactive")).toBeInTheDocument();
    expect(screen.queryByText("Paused by inventory")).not.toBeInTheDocument();
  });
});

describe("#835 OffersManager source and i18n contract", () => {
  it("shipped OffersManager routes the status chip through offerStatusChip", async () => {
    const fs = await import("fs");
    const path = require("path") as typeof import("path");
    const src = fs.readFileSync(path.join(__dirname, "OffersManager.tsx"), "utf8");
    expect(src).toMatch(/from "\.\/offerStatusChip"/);
    expect(src).toMatch(/offerStatusChip\(offer\)/);
    // The old unconditional Active chip must not come back.
    expect(src).not.toMatch(/offer\.is_active \? t\("status\.active"\)/);
    expect(src).not.toMatch(/offer\.inventory_blocked && \(/);
  });

  it("operator locales en/es/es-ar all carry the offersManager copy", async () => {
    const fs = await import("fs");
    const path = require("path") as typeof import("path");
    for (const locale of ["en", "es", "es-ar"]) {
      const raw = fs.readFileSync(
        path.join(__dirname, "..", "..", "i18n", "messages", locale, "businessDashboard.json"),
        "utf8",
      );
      const om = JSON.parse(raw).dashboard.menuBuilder.offersManager;
      expect(typeof om.inventoryBlockedChip).toBe("string");
      expect(om.inventoryBlockedChip.length).toBeGreaterThan(0);
      expect(typeof om.inventoryBlockedTooltip).toBe("string");
      expect(om.inventoryBlockedTooltip.length).toBeGreaterThan(0);
    }
  });
});

/**
 * #835 round two — the payload, not just the chip. GetOffers on venue 142 now
 * answers `is_active: false` (the effective answer) with the operator's own
 * switch in `manual_active`, so both the card and the edit modal have to read
 * the right one.
 */
describe("#835 post-fix payload shape", () => {
  it("keeps the paused chip when is_active is the effective false", () => {
    render(
      <OfferStatusChips
        offer={{ is_active: false, manual_active: true, inventory_blocked: true }}
      />,
    );
    // Without offerManualActive this row regresses to a plain "Inactive" chip
    // and the operator loses the reason.
    expect(screen.getByText("Paused by inventory")).toBeInTheDocument();
    expect(screen.queryByText("Inactive")).not.toBeInTheDocument();
    expect(screen.queryByText("Active")).not.toBeInTheDocument();
  });

  it("shows Inactive when the stored switch is off", () => {
    render(
      <OfferStatusChips
        offer={{ is_active: false, manual_active: false, inventory_blocked: true }}
      />,
    );
    expect(screen.getByText("Inactive")).toBeInTheDocument();
    expect(screen.queryByText("Paused by inventory")).not.toBeInTheDocument();
  });

  it("seeds the edit modal from the stored switch, not the effective flag", async () => {
    const fs = await import("fs");
    const path = require("path") as typeof import("path");
    const src = fs.readFileSync(path.join(__dirname, "OffersManager.tsx"), "utf8");
    // Seeding from is_active would save a blocked offer as OFF, and it would
    // stay off after the restock.
    expect(src).toMatch(/is_active: offerManualActive\(offer\)/);
    expect(src).not.toMatch(/is_active: offer\.is_active \?\? true/);
  });
});
