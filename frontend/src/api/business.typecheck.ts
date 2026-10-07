import type { Table, TableWithStatus } from "./business";
import { asDollars } from "@/types/money";

const tableWithoutQRURL: Table = {
  id: 1,
  business_id: 42,
  name: "Patio 1",
  table_code: "PATIO-1",
  capacity: 4,
  qr_code: "",
  is_active: true,
  created_at: "2026-05-23T00:00:00Z",
  updated_at: "2026-05-23T00:00:00Z",
};

const statusPayload: TableWithStatus = {
  table: tableWithoutQRURL,
  status: "occupied",
  active_bills_count: 1,
  active_bill_physical_item_quantity: 2,
  active_bills: [
    {
      id: 10,
      bill_number: "B-10",
      total_amount: asDollars(25),
      paid_amount: asDollars(5),
      status: "partial",
      created_at: "2026-05-23T00:00:00Z",
      updated_at: "2026-05-23T00:00:00Z",
    },
  ],
  reservations_count: 1,
  reservations: [
    {
      id: 20,
      customer_name: "Alex",
      party_size: 4,
      reservation_time: "2026-05-23T20:00:00Z",
      status: "confirmed",
    },
  ],
};

statusPayload.active_bills[0].bill_number.toUpperCase();

// @ts-expect-error Table bill summaries must not be any[].
statusPayload.active_bills[0].made_up_field;

const badReservationStatus: TableWithStatus = {
  ...statusPayload,
  reservations: [
    {
      ...statusPayload.reservations[0],
      // @ts-expect-error Reservation summaries use the backend status union.
      status: "not_a_status",
    },
  ],
};

void badReservationStatus;
