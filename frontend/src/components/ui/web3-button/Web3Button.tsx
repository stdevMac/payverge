"use client";

import { useEffect, useState } from "react";
import { useAccount } from "wagmi";
import { Spinner } from "@nextui-org/react";
import { DynamicWidget } from "@/providers/DynamicProvider";

export const Web3Button = () => {
  const { status } = useAccount();
  const [isLoading, setIsLoading] = useState(true);
  const [hasInitialized, setHasInitialized] = useState(false);

  const markDynamicIntent = () => {
    if (typeof window === "undefined") return;
    localStorage.setItem(
      "dynamic_auth_intent",
      JSON.stringify({ intent: "wallet", timestamp: Date.now() }),
    );
  };

  useEffect(() => {
    // Only set loading to false when we have a definitive wallet state
    if (status !== "reconnecting" && !hasInitialized) {
      const timer = setTimeout(() => {
        setIsLoading(false);
        setHasInitialized(true);
      }, 500);
      return () => clearTimeout(timer);
    }
  }, [status, hasInitialized]);

  if (isLoading || status === "reconnecting") {
    return (
      <div className="h-[40px] flex items-center justify-center px-4">
        <Spinner size="sm" />
      </div>
    );
  }

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={markDynamicIntent}
      onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); markDynamicIntent(); } }}
    >
      <DynamicWidget />
    </div>
  );
};
