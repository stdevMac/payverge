"use client";

import React, { Suspense } from "react";
import AcceptInvitation from "../../../../components/staff/AcceptInvitation";
import { RouteLoadingFallback } from "@/components/ui/AsyncState";

export default function AcceptInvitationClient() {
  return (
    <Suspense fallback={<RouteLoadingFallback />}>
      <AcceptInvitation />
    </Suspense>
  );
}
