"use client";

import { useContext } from "react";
import { OperationalAlertsContext } from "./OperationalAlertsProvider";

export function useOptionalOperationalAlerts() {
  return useContext(OperationalAlertsContext) ?? null;
}

export function useOperationalAlerts() {
  const context = useContext(OperationalAlertsContext);
  if (!context) {
    throw new Error(
      "useOperationalAlerts must be used within an OperationalAlertsProvider",
    );
  }
  return context;
}
