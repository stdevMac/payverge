"use client";

import React from "react";
import { Button } from "@nextui-org/react";
import { BellRing, Volume2, VolumeX } from "lucide-react";
import { getTranslation, useSimpleLocale } from "@/i18n/SimpleTranslationProvider";
import { useOptionalOperationalAlerts } from "./useOperationalAlerts";

interface EnableAlertSoundButtonProps {
  compact?: boolean;
  className?: string;
}

export default function EnableAlertSoundButton({
  compact = false,
  className = "",
}: EnableAlertSoundButtonProps) {
  const { locale } = useSimpleLocale();
  const alerts = useOptionalOperationalAlerts();
  const [busy, setBusy] = React.useState(false);

  if (!alerts || alerts.settings?.sound_enabled === false) {
    return null;
  }

  const label = alerts.audioBlocked
    ? String(getTranslation("businessSettings.notifications.enableAlertSound", locale))
    : String(getTranslation("businessSettings.notifications.alertSoundReady", locale));
  const Icon = alerts.audioBlocked ? VolumeX : compact ? BellRing : Volume2;

  const handlePress = async () => {
    setBusy(true);
    try {
      await alerts.enableSound();
    } finally {
      setBusy(false);
    }
  };

  return (
    <Button
      isIconOnly
      size="sm"
      variant={alerts.audioBlocked ? "solid" : "light"}
      color={alerts.audioBlocked ? "warning" : "default"}
      isLoading={busy}
      onPress={handlePress}
      aria-label={label}
      title={label}
      className={className}
    >
      <Icon className="w-4 h-4" aria-hidden="true" />
    </Button>
  );
}
