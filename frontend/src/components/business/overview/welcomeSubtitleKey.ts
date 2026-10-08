// Issue #795: the owner Overview greeted every venue with "Your business is
// ready to accept payments" — including a demo venue where every payment
// plugin is disabled and Payments & apps itself says "No card processor
// enabled". The readiness claim needs positive backend evidence: the plugin
// flags have loaded AND at least one payment rail is enabled. While flags are
// unknown (loading, fetch error, a locked or role-restricted view that never
// fetches) or all rails are off, the neutral subtitle is used instead.

const STAFF_SUBTITLE_KEYS: Record<string, string> = {
  kitchen: "roleSpecific.kitchen.welcome.subtitle",
  host: "roleSpecific.host.welcome.subtitle",
  server: "roleSpecific.server.welcome.subtitle",
  manager: "roleSpecific.manager.welcome.subtitle",
};

export interface WelcomeSubtitleFlags {
  isStaffUser: boolean;
  staffRole?: string;
  /** True once the plugin flags actually arrived from the backend. */
  paymentRailsKnown: boolean;
  /** True when at least one payment rail (card or crypto) is enabled. */
  anyPaymentRailEnabled: boolean;
}

export function welcomeSubtitleKey({
  isStaffUser,
  staffRole,
  paymentRailsKnown,
  anyPaymentRailEnabled,
}: WelcomeSubtitleFlags): string {
  if (isStaffUser) {
    return STAFF_SUBTITLE_KEYS[staffRole ?? ""] ?? "welcome.subtitle";
  }
  if (paymentRailsKnown && anyPaymentRailEnabled) {
    return "welcome.subtitle";
  }
  return "welcome.subtitleNoPaymentRail";
}
