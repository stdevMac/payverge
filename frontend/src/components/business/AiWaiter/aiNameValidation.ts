/** Guest-visible AI waiter name bound — keep in sync with backend MaxAiNameLen (L4-5). */
export const AI_NAME_MAX_LEN = 40;

export function isAiNameTooLong(name: string): boolean {
  return [...name.trim()].length > AI_NAME_MAX_LEN;
}
