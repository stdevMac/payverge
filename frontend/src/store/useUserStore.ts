import { create } from "zustand";
import { persist, createJSONStorage } from "zustand/middleware";
import { UserInterface } from "@/interface";
import { createCustomStorage } from "./middleware";

export interface UserState {
    user: UserInterface | null;
    hasClosedPortfolioModal: boolean;
    setUser: (user: UserInterface | null) => void;
    updateUser: (update: Partial<UserInterface>) => void;
    clearUser: () => void;
    setHasClosedPortfolioModal: (value: boolean) => void;
}

const initialState = {
    user: null,
    hasClosedPortfolioModal: false,
} as const;

type InitialState = typeof initialState;

export const useUserStore = create(
  persist<UserState>(
    (set, get) => ({
    ...initialState,
    setUser: (user) => set({ user }),
    updateUser: (update) =>
        set((state) => ({
            user: state.user ? { ...state.user, ...update } : null,
        })),
    clearUser: () => {
      try {
        // Only clear if we have something to clear
        if (get().user !== null) {
          set(initialState);
        }
      } catch (error) {
        console.error('Error clearing user:', error);
      }
    },
    setHasClosedPortfolioModal: (value) =>
        set({ hasClosedPortfolioModal: value }),
    }),
    {
      name: 'user-storage',
      storage: createJSONStorage(() => createCustomStorage()),
    }
  )
);
