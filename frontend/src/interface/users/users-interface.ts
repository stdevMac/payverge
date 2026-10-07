
export interface UserInterface {
    username: string;
    address: string;
    joined_at: string;
    email: string;
    role: string;
    notifications: Notification[];
    language_selected: string;
}

export interface UpdateUserInterface {
    address: string;
    username: string;
    email: string;
}
