import type { SetupStep } from "./SetupChecklist";

export interface SetupState {
  hasPositions: boolean;
  /** staff members OR pending invitations — either counts as "team started". */
  hasTeam: boolean;
}

export interface SetupActions {
  addPositions?: () => void;
  invite?: () => void;
  goToSchedule?: () => void;
}

// Builds the three cold-start steps from a translation resolver bound to the
// `dashboardSetup` namespace, so the Schedule and Team tabs render the exact
// same path. The schedule step is never auto-marked done here — it's the
// destination, and the checklist only renders at cold start anyway.
export function buildSetupSteps(
  t: (key: string) => string,
  state: SetupState,
  actions: SetupActions,
): SetupStep[] {
  return [
    {
      key: "positions",
      title: t("positionsTitle"),
      description: t("positionsDescription"),
      done: state.hasPositions,
      actionLabel: t("positionsAction"),
      onAction: actions.addPositions,
    },
    {
      key: "invite",
      title: t("inviteTitle"),
      description: t("inviteDescription"),
      done: state.hasTeam,
      actionLabel: t("inviteAction"),
      onAction: actions.invite,
    },
    {
      key: "schedule",
      title: t("scheduleTitle"),
      description: t("scheduleDescription"),
      done: false,
      actionLabel: t("scheduleAction"),
      onAction: actions.goToSchedule,
    },
  ];
}
