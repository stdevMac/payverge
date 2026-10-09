import { getPublicConfig } from "@/config/publicConfig";
import React from "react";
import Maintenance from "./Maintenance";

// MAINTENANCE_MODE is a runtime setting read through
// getPublicConfig(): toggling it needs a container restart, not a rebuild.
// The legacy NEXT_PUBLIC_* names are still honoured as fallbacks.

// Server component: the flag is resolved on the server per request, so
// the gate has no client-side behavior to justify pulling its subtree into
// the client bundle. Mounting it as a client wrapper on every route
// previously forced React to hydrate the entire children tree below it
// (translation provider + auth providers + page) just to evaluate one env
// constant.
const WithMaintenance = ({ children }: { children: React.ReactNode }) => {
  const isMaintenanceMode = getPublicConfig().maintenanceMode;

  if (isMaintenanceMode) {
    return <Maintenance />;
  }

  return <>{children}</>;
};

export default WithMaintenance;
