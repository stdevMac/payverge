export type PwaPlatformState = "installed" | "manual-install" | "unavailable";

export interface PlatformSnapshot {
  userAgent: string;
  platform: string;
  maxTouchPoints: number;
  standalone: boolean;
  displayModeStandalone: boolean;
}

const IOS_SAFARI_EXCLUSIONS =
  /CriOS|FxiOS|EdgiOS|OPiOS|Ddg|DuckDuckGo|GSA|FBAN|FBAV/i;

export function detectPwaPlatform(
  snapshot: PlatformSnapshot,
): PwaPlatformState {
  if (snapshot.standalone || snapshot.displayModeStandalone) return "installed";

  const iosDevice =
    /iPad|iPhone|iPod/.test(snapshot.userAgent) ||
    (snapshot.platform === "MacIntel" && snapshot.maxTouchPoints > 1);
  const safari =
    /Version\/[\d.]+/.test(snapshot.userAgent) &&
    /Safari\//.test(snapshot.userAgent) &&
    !IOS_SAFARI_EXCLUSIONS.test(snapshot.userAgent);

  return iosDevice && safari ? "manual-install" : "unavailable";
}

export function browserPlatformSnapshot(): PlatformSnapshot {
  const nav = navigator as Navigator & { standalone?: boolean };

  return {
    userAgent: nav.userAgent,
    platform: nav.platform,
    maxTouchPoints: nav.maxTouchPoints,
    standalone: nav.standalone === true,
    displayModeStandalone: window.matchMedia("(display-mode: standalone)")
      .matches,
  };
}
