// Compile-time guards for FE↔BE contract long-tail items that were tightened in
// prior batches (the 2026-05-29 contract long-tail triage). If any type loosens,
// `npm run typecheck` fails.
import type { UpdateBusinessRequest } from "./business";
import type { DeliveryDriver, VehicleType, DriverStatus } from "./delivery";

// #92 — UpdateBusinessRequest must NOT accept delivery fields (they are stripped
// at runtime by toBusinessUpdatePayload, and the type must reject them too).
const updateReq: UpdateBusinessRequest = { name: "Cafe" };
// @ts-expect-error delivery_enabled is not part of UpdateBusinessRequest.
updateReq.delivery_enabled = true;
void updateReq;

// #113/#114 — DeliveryDriver.vehicle_type/status must use the backend enums.
const driver: DeliveryDriver = {
  id: 1,
  business_id: 1,
  name: "Sam",
  phone: "555",
  status: "online",
  is_available: true,
  is_active: true,
};
const vt: VehicleType = "bicycle";
const ds: DriverStatus = "on_break";
// @ts-expect-error "spaceship" is not a VehicleType.
const badVehicle: VehicleType = "spaceship";
// @ts-expect-error "sleeping" is not a DriverStatus.
const badStatus: DriverStatus = "sleeping";
void driver;
void vt;
void ds;
void badVehicle;
void badStatus;
