/**
 * L3-26: layout nodes without live status feed data are not silent inert shapes.
 */
export type LiveMapNodeAffordance = {
  clickable: boolean;
  showNoLiveDataLabel: boolean;
  opacity: number;
};

export function liveMapNodeAffordance(hasLiveStatus: boolean): LiveMapNodeAffordance {
  if (hasLiveStatus) {
    return { clickable: true, showNoLiveDataLabel: false, opacity: 1 };
  }
  return { clickable: false, showNoLiveDataLabel: true, opacity: 0.55 };
}
