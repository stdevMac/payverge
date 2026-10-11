import type { DirectorThread } from "@/api/directorConsole";

/**
 * One row per real thread id (#730). Never key on title — a second
 * "how much did we sell tonight" is a different conversation and must
 * stay in ListThreads / the sidebar. Title-collapse hid real history.
 */
export function uniqueDirectorThreads(
  threads: DirectorThread[] | null | undefined,
): DirectorThread[] {
  if (!threads || threads.length === 0) {
    return [];
  }
  const out: DirectorThread[] = [];
  const seenId = new Set<number>();
  for (const thread of threads) {
    if (!thread?.id || seenId.has(thread.id)) {
      continue;
    }
    seenId.add(thread.id);
    out.push(thread);
  }
  return out;
}
