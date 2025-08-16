import { create } from 'zustand';

interface ShareBarState {
  isExpanded: boolean;
  expandedHeight: number;
  setIsExpanded: (expanded: boolean) => void;
  setExpandedHeight: (height: number) => void;
}

export const useShareBarStore = create<ShareBarState>((set) => ({
  isExpanded: false,
  expandedHeight: 0,
  setIsExpanded: (expanded) => set({ isExpanded: expanded }),
  setExpandedHeight: (height) => set({ expandedHeight: height }),
}));
