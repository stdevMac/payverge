import { axiosInstance } from "./tools/instance";
import type { Bill } from "./bills";
import type { Reservation } from "./reservations";

type FloorAction =
  | "seat_walk_in"
  | "seat_reservation"
  | "clear"
  | "transfer"
  | "merge";

export interface SeatTableRequest {
  party_size?: number;
  reservation_id?: number;
  notes?: string;
}

export interface SeatTableResponse {
  action: FloorAction;
  table_id?: number | null;
  bill?: Bill;
  reservation?: Reservation;
}

export interface ClearTableResponse {
  action: "clear";
  table_id: number;
  bill: Bill;
}

export interface TransferTableRequest {
  target_table_id: number;
}

export interface TransferTableResponse {
  action: "transfer";
  bill: Bill;
  from_table_id: number;
  to_table_id: number;
}

export interface MergeTableRequest {
  target_table_id: number;
}

export interface MergeTableResponse {
  action: "merge";
  target_bill: Bill;
  source_bill?: Bill;
  from_table_id: number;
  to_table_id: number;
}

/** Seat a walk-in or check in the table's next assigned reservation. */
export async function seatTable(
  businessId: number,
  tableId: number,
  data: SeatTableRequest = {},
): Promise<SeatTableResponse> {
  const response = await axiosInstance.post(
    `/inside/businesses/${businessId}/tables/${tableId}/seat`,
    data,
  );
  return response.data;
}

/** Free a table by closing its empty unpaid open check. */
export async function clearTable(
  businessId: number,
  tableId: number,
): Promise<ClearTableResponse> {
  const response = await axiosInstance.post(
    `/inside/businesses/${businessId}/tables/${tableId}/clear`,
  );
  return response.data;
}

/** Move this table's open check onto another free table. */
export async function transferTable(
  businessId: number,
  tableId: number,
  data: TransferTableRequest,
): Promise<TransferTableResponse> {
  const response = await axiosInstance.post(
    `/inside/businesses/${businessId}/tables/${tableId}/transfer`,
    data,
  );
  return response.data;
}

/** Merge this table's open check into another table (or transfer if free). */
export async function mergeTable(
  businessId: number,
  tableId: number,
  data: MergeTableRequest,
): Promise<MergeTableResponse> {
  const response = await axiosInstance.post(
    `/inside/businesses/${businessId}/tables/${tableId}/merge`,
    data,
  );
  return response.data;
}
