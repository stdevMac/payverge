/**
 * Guest Sage panel geometry (#724).
 *
 * NextUI Modal applies these through its real `modal()` slots
 * (`@nextui-org/theme`). Defaults we must not inherit: size=md (max-w-md),
 * placement=auto (sm:items-center), scrollBehavior=inside (max-h − 8rem),
 * and sm:my-16. Those paint a floating phone card over the menu.
 */
export const AI_WAITER_PANEL_MODAL = {
  size: "full" as const,
  placement: "bottom" as const,
  scrollBehavior: "normal" as const,
  backdrop: "opaque" as const,
  hideCloseButton: true,
  classNames: {
    base: "!m-0 !mx-0 !my-0 w-full !max-w-none h-[100dvh] !max-h-[100dvh] rounded-none border-0 overflow-hidden sm:!my-0 sm:!mx-0 sm:h-[100dvh] sm:!max-h-[100dvh] sm:w-[26rem] sm:!max-w-[26rem] lg:w-[30rem] lg:!max-w-[30rem] xl:w-[34rem] xl:!max-w-[34rem] sm:rounded-none sm:rounded-s-[1.5rem] sm:border-s sm:border-warm-200",
    wrapper:
      "items-end justify-center sm:!items-stretch sm:!justify-end",
    backdrop: "bg-ink-900/40",
    body: "p-0 bg-warm-50/30 min-h-0",
  },
};
