"use client";

import React from "react";
import type { InstanceInfo } from "@/lib/instance/instanceInfo";
import { InstanceContext } from "@/lib/instance/instanceContext";

/** Hands the layout's server-fetched instance payload to useInstance(). */
export default function InstanceProvider({
  value,
  children,
}: {
  value: InstanceInfo | null;
  children: React.ReactNode;
}) {
  return (
    <InstanceContext.Provider value={value}>
      {children}
    </InstanceContext.Provider>
  );
}
