/** @jest-environment jsdom */
import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import GuestFiscalReceiptCard from "./GuestFiscalReceiptCard";
import * as billsApi from "@/api/bills";

jest.mock("@/api/bills", () => ({
  ...jest.requireActual("@/api/bills"),
  getGuestFiscalReceipt: jest.fn(),
  guestFiscalReceiptPdfUrl: (token: string) =>
    `http://api.test/guest/bill/${token}/fiscal-receipt/pdf`,
}));

jest.mock("../../i18n/GuestTranslationProvider", () => ({
  useGuestTranslation: () => ({ t: (k: string) => k }),
}));

const mockGet = billsApi.getGuestFiscalReceipt as jest.Mock;

describe("GuestFiscalReceiptCard", () => {
  beforeEach(() => {
    mockGet.mockReset();
  });

  it("renders the factura identity and PDF link once authorized", async () => {
    mockGet.mockResolvedValue({
      status: "authorized",
      receipt_type: "factura_b",
      receipt_number: "00000042",
      auth_code: "71234567890123",
      qr_payload: "https://www.arca.gob.ar/fe/qr/?p=abc",
      pdf_available: true,
    });
    render(<GuestFiscalReceiptCard billToken="tok-1" />);
    await waitFor(() =>
      expect(screen.getByText(/71234567890123/)).toBeInTheDocument(),
    );
    const pdfLink = screen.getByRole("link", {
      name: "menu.fiscalReceipt.downloadPdf",
    });
    expect(pdfLink).toHaveAttribute(
      "href",
      "http://api.test/guest/bill/tok-1/fiscal-receipt/pdf",
    );
    const verifyLink = screen.getByRole("link", {
      name: "menu.fiscalReceipt.verify",
    });
    expect(verifyLink).toHaveAttribute(
      "href",
      "https://www.arca.gob.ar/fe/qr/?p=abc",
    );
  });

  it("renders nothing when there is no fiscal receipt", async () => {
    mockGet.mockResolvedValue({ status: "none" });
    const { container } = render(
      <GuestFiscalReceiptCard billToken="tok-2" />,
    );
    await waitFor(() => expect(mockGet).toHaveBeenCalled());
    expect(container.firstChild).toBeNull();
  });

  it("renders nothing when no bill token is available", () => {
    const { container } = render(
      <GuestFiscalReceiptCard billToken={undefined} />,
    );
    expect(container.firstChild).toBeNull();
    expect(mockGet).not.toHaveBeenCalled();
  });
});
