/** @jest-environment jsdom */
/**
 * Wave 4 Task 15: team chat delete uses shared ConfirmationModal, not window.confirm.
 */
import fs from "fs";
import path from "path";

it("does not use window.confirm for message deletion", () => {
  const src = fs.readFileSync(
    path.join(__dirname, "TeamChatPanel.tsx"),
    "utf8",
  );
  expect(src).not.toMatch(/window\.confirm/);
  expect(src).toMatch(/ConfirmationModal/);
});
