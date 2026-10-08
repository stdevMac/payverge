/** @jest-environment jsdom */
/**
 * L3-25 residual: whitespace-only rename must not leave the field showing
 * text the server never accepted — resync draft + inline error.
 */
import { act, fireEvent, render, screen } from "@testing-library/react";
import TableDetailModal from "../TableDetailModal";

jest.mock("@/api/onboarding", () => ({
  markQRPreviewed: jest.fn().mockResolvedValue({}),
}));

jest.mock("../../QRCodeWithText", () => ({
  __esModule: true,
  default: () => <canvas aria-label="table qr" />,
}));

const table = {
  id: 1,
  name: "Table 1",
  table_code: "T1",
  is_active: true,
  status: "available" as const,
  capacity: 4,
  server_name: null,
  next_reservation: null,
  active_bill: null,
  last_seen: null,
};

describe("L3-25 TableDetailModal rename whitespace", () => {
  it("resyncs draft and shows error when the name is whitespace-only", async () => {
    const onRename = jest.fn();
    render(
      <TableDetailModal
        table={table}
        open
        onClose={jest.fn()}
        businessId={1}
        onRename={onRename}
      />,
    );
    const input = screen.getByTestId("rename-table-name") as HTMLInputElement;
    await act(async () => {
      fireEvent.change(input, { target: { value: "   " } });
    });
    expect(input.value).toBe("   ");
    await act(async () => {
      fireEvent.blur(input);
    });
    expect(onRename).not.toHaveBeenCalled();
    // Draft must return to server truth — not keep three spaces.
    expect((screen.getByTestId("rename-table-name") as HTMLInputElement).value).toBe(
      "Table 1",
    );
    expect(screen.getByText(/spaces alone|solo espacios|Enter a name/i)).toBeInTheDocument();
  });

  it("resyncs draft without calling onRename when only whitespace around same name", async () => {
    const onRename = jest.fn();
    render(
      <TableDetailModal
        table={table}
        open
        onClose={jest.fn()}
        businessId={1}
        onRename={onRename}
      />,
    );
    const input = screen.getByTestId("rename-table-name") as HTMLInputElement;
    await act(async () => {
      fireEvent.change(input, { target: { value: "  Table 1  " } });
    });
    await act(async () => {
      fireEvent.blur(input);
    });
    expect(onRename).not.toHaveBeenCalled();
    expect((screen.getByTestId("rename-table-name") as HTMLInputElement).value).toBe(
      "Table 1",
    );
  });
});
