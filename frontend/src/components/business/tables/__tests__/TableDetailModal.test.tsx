/** @jest-environment jsdom */
import {
  act,
  render,
  screen,
  fireEvent,
  waitFor,
} from "@testing-library/react";
import TableDetailModal from "../TableDetailModal";

const mockMarkQRPreviewed = jest.fn().mockResolvedValue({
  qr_previewed: true,
  qr_previewed_at: "2026-07-31T00:00:00Z",
});
jest.mock("@/api/onboarding", () => ({
  markQRPreviewed: (...args: unknown[]) => mockMarkQRPreviewed(...args),
}));

const mockQRCodeWithText = jest.fn((_props: { onRendered?: () => void }) => (
  <canvas aria-label="table qr" />
));
jest.mock("../../QRCodeWithText", () => ({
  __esModule: true,
  default: (props: { onRendered?: () => void }) => mockQRCodeWithText(props),
}));

const table = {
  id: 37,
  name: "Main Room 1",
  table_code: "AI-T01",
  is_active: true,
  status: "available" as const,
  capacity: 4,
  server_name: null,
  next_reservation: null,
  active_bill: null,
  last_seen: null,
};

describe("TableDetailModal", () => {
  beforeEach(() => {
    mockMarkQRPreviewed.mockClear();
    mockQRCodeWithText.mockClear();
  });

  it("records the authoritative milestone when a valid table QR is rendered", async () => {
    render(
      <TableDetailModal
        table={table}
        open
        onClose={jest.fn()}
        businessId={9}
      />,
    );

    expect(mockMarkQRPreviewed).not.toHaveBeenCalled();
    const qrProps = mockQRCodeWithText.mock.lastCall?.[0];
    expect(qrProps?.onRendered).toEqual(expect.any(Function));
    act(() => qrProps?.onRendered?.());

    await waitFor(() =>
      expect(mockMarkQRPreviewed).toHaveBeenCalledWith(9, table.id),
    );
  });

  it("does not record first value when QR generation has not succeeded", () => {
    render(
      <TableDetailModal
        table={table}
        open
        onClose={jest.fn()}
        businessId={9}
      />,
    );

    expect(mockQRCodeWithText).toHaveBeenCalled();
    expect(mockMarkQRPreviewed).not.toHaveBeenCalled();
  });

  it("renders QR actions and admin actions inside one modal", async () => {
    const onDelete = jest.fn();
    const onToggleActive = jest.fn();

    render(
      <TableDetailModal
        table={table}
        open
        onClose={jest.fn()}
        businessId={1}
        onDelete={onDelete}
        onToggleActive={onToggleActive}
      />,
    );

    // table_code appears in the header chip and in the URL line, so use
    // getAllByText to confirm it's rendered at least once.
    expect(screen.getAllByText(/AI-T01/i).length).toBeGreaterThan(0);
    expect(screen.getAllByText(/Main Room 1/i).length).toBeGreaterThan(0);
    expect(
      screen.getByRole("button", { name: /Download PNG/i }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /Copy table URL/i }),
    ).toBeInTheDocument();

    await act(async () => {
      fireEvent.click(
        screen.getByRole("button", { name: /Deactivate table/i }),
      );
    });
    // L3-28: deactivate is gated by ConfirmationModal — parent onToggleActive
    // must not fire until the operator confirms.
    expect(onToggleActive).not.toHaveBeenCalled();
    expect(
      await screen.findByText(/hides it from the floor plan/i),
    ).toBeInTheDocument();
    await act(async () => {
      fireEvent.click(
        screen.getByRole("button", { name: /Confirm deactivate/i }),
      );
    });
    await waitFor(() => expect(onToggleActive).toHaveBeenCalledTimes(1));

    fireEvent.click(screen.getByRole("button", { name: /Delete table/i }));
    expect(onDelete).toHaveBeenCalledTimes(1);
  });

  it("exposes a preview-as-guest link that opens /t/{tableCode} in a new tab", () => {
    render(
      <TableDetailModal
        table={table}
        open
        onClose={jest.fn()}
        businessId={1}
      />,
    );
    const link = screen.getByRole("button", { name: /open live preview/i });
    expect(link).toHaveAttribute("href", "/t/AI-T01");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", expect.stringContaining("noopener"));
  });

  it("stays out of the DOM when open is false", () => {
    render(
      <TableDetailModal
        table={table}
        open={false}
        onClose={jest.fn()}
        businessId={1}
      />,
    );
    expect(screen.queryByText(/Main Room 1/i)).not.toBeInTheDocument();
    expect(mockMarkQRPreviewed).not.toHaveBeenCalled();
  });

  it("shows the active bill physical quantity as bill items", () => {
    render(
      <TableDetailModal
        table={{
          ...table,
          active_bill: {
            id: 42,
            total: 42,
            physical_item_quantity: 3,
            created_at: "2026-08-11T18:00:00.000Z",
          },
        }}
        open
        onClose={jest.fn()}
        businessId={1}
      />,
    );

    expect(screen.getByText(/3 items/i)).toBeInTheDocument();
  });
});
