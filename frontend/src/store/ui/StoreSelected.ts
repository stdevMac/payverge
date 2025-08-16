import { create } from "zustand";

export type FleetStage =
    | "All"
    | "Posted"
    | "Funded"
    | "OnPurchase"
    | "Active"
    | "Closing"
    | "Closed";

interface State {
    selectMenu: FleetStage;
    setStage: (stage: FleetStage) => void;
}

export const StoreSelected = create<State>((set) => ({
    selectMenu: "Posted",
    setStage: (stage) => set({ selectMenu: stage }),
}));
