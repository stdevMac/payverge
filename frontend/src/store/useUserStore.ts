import { create } from "zustand";
import { UserInterface } from "@/interface";

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

// NOTE: User state is intentionally NOT persisted to localStorage.
// Storing auth/session data client-side exposes it to XSS attacks.
// The backend maintains the canonical session in httpOnly cookies.
export const useUserStore = create<UserState>((set, get) => ({
    ...initialState,
    setUser: (user) => set({ user }),
    updateUser: (update) =>
        set((state) => ({
            user: state.user ? { ...state.user, ...update } : null,
        })),
    clearUser: () => {
      try {
        if (get().user !== null) {
          set(initialState);
        }
      } catch (error) {
        console.error('Error clearing user:', error);
      }
    },
    setHasClosedPortfolioModal: (value) =>
        set({ hasClosedPortfolioModal: value }),
}));
