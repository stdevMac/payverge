"use client";

import React, { useEffect, useState } from "react";
import { WifiOff, Wifi } from "lucide-react";
import { useConnectivity } from "@/contexts/ConnectivityContext";
import {
  useSimpleLocale,
  getTranslation,
} from "@/i18n/SimpleTranslationProvider";

export function OfflineBanner() {
  const { isOnline } = useConnectivity();
  const [showReconnected, setShowReconnected] = useState(false);
  const [wasOffline, setWasOffline] = useState(false);
  const { locale } = useSimpleLocale();

  const t = (key: string): string => {
    const fullKey = `businessDashboard.connectivity.${key}`;
    const result = getTranslation(fullKey, locale);
    return typeof result === "string" ? result : key;
  };

  useEffect(() => {
    if (!isOnline) {
      setWasOffline(true);
      setShowReconnected(false);
    } else if (wasOffline) {
      setShowReconnected(true);
      const timer = setTimeout(() => {
        setShowReconnected(false);
        setWasOffline(false);
      }, 3000);
      return () => clearTimeout(timer);
    }
  }, [isOnline, wasOffline]);

  if (isOnline && !showReconnected) return null;

  if (showReconnected) {
    return (
      <div
        role="status"
        aria-live="polite"
        className="bg-emerald-50 border-b border-emerald-200 px-4 py-2 flex items-center justify-center gap-2 text-sm text-emerald-800"
      >
        <Wifi className="w-4 h-4" />
        <span>{t("backOnline")}</span>
      </div>
    );
  }

  return (
    <div
      role="alert"
      aria-live="assertive"
      className="bg-amber-50 border-b border-amber-200 px-4 py-2 flex items-center justify-center gap-2 text-sm text-amber-800"
    >
      <WifiOff className="w-4 h-4" />
      <span>{t("offline")}</span>
      <span className="w-2 h-2 bg-amber-500 rounded-full animate-pulse" />
    </div>
  );
}
