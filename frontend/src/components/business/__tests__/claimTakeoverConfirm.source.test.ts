/** @jest-environment node */
import fs from "fs";
import path from "path";

const files = [
  path.join(__dirname, "../ReservationManager.tsx"),
  path.join(__dirname, "../AiWaiter/AiWaiterDashboard.tsx"),
  path.join(__dirname, "../delivery/DispatchConsole.tsx"),
];

describe("operator claim takeover uses ConfirmationModal", () => {
  it.each(files)("%s does not call window.confirm", (file) => {
    const src = fs.readFileSync(file, "utf8");
    expect(src).not.toMatch(/window\.confirm/);
    expect(src).toMatch(/ConfirmationModal/);
  });
});
