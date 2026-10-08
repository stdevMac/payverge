/** @jest-environment jsdom */
import { fireEvent, render, screen } from "@testing-library/react";
import { WarningChips } from "./WarningChips";

const labels = {
  overtime: "{name} is at {hours}h",
  postedLate:
    "Published late — the team had less than {days} days' notice. Tap to open schedule settings.",
  minorLate: "A minor is scheduled past {time}",
};

describe("WarningChips", () => {
  it("wraps the posted-late action so long copy cannot expand the page (#459)", () => {
    const onPostedLateAction = jest.fn();
    render(
      <WarningChips
        warnings={[{ code: "posted_late", detail: 7 }]}
        labels={labels}
        onPostedLateAction={onPostedLateAction}
        postedLateActionLabel="Open schedule settings"
      />,
    );

    const action = screen.getByRole("listitem", { name: "Open schedule settings" });
    expect(action.tagName).toBe("BUTTON");
    expect(action.className).toMatch(/\bmax-w-full\b/);
    expect(action.className).toMatch(/\bmin-w-0\b/);

    const chip = action.querySelector("[data-slot], .max-w-full") ?? action.firstElementChild;
    expect(chip).not.toBeNull();
    fireEvent.click(action);
    expect(onPostedLateAction).toHaveBeenCalledTimes(1);
  });

  it("keeps non-action chips inside the list width", () => {
    const { container } = render(
      <WarningChips
        warnings={[{ code: "overtime", detail: 2520, staff_id: 3 }]}
        labels={labels}
        staffNames={new Map([[3, "Dana"]])}
      />,
    );
    const list = container.querySelector("[role='list']");
    expect(list?.className).toMatch(/\bmax-w-full\b/);
    expect(list?.className).toMatch(/\bmin-w-0\b/);
    expect(screen.getByText("Dana is at 42h")).toBeInTheDocument();
  });
});
