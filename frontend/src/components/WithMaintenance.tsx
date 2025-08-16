"use client";

import React from 'react';
import Maintenance from './Maintenance';

const WithMaintenance = ({ children }: { children: React.ReactNode }) => {
  const isMaintenanceMode = process.env.NEXT_PUBLIC_MAINTENANCE_MODE === 'true';

  if (isMaintenanceMode) {
    return <Maintenance />;
  }

  return <>{children}</>;
};

export default WithMaintenance;