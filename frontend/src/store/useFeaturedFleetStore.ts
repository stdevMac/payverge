import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";
import { createCustomStorage } from "./middleware";

interface FeaturedFleetState {
  featuredFleetId: string | null;
  setFeaturedFleetId: (id: string | null) => void;
}

export const useFeaturedFleetStore = create(
  persist<FeaturedFleetState>(
    (set) => ({
      featuredFleetId: null,
      setFeaturedFleetId: (id) => set({ featuredFleetId: id }),
    }),
    {
      name: 'featured-fleet-storage',
      storage: createJSONStorage(() => createCustomStorage()),
    }
  )
);
