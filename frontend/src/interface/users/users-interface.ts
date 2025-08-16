export interface RewardsInterface {
    amount: number;
    date: string;
    claimed: boolean;
    contract_address: string;
    car_name: string;
    fleet_id: string;
}

export interface InvestmentsPerFleetInterface {
    fleet_id: string;
    amount: number;
    car_name: string;
    profits: number;
    contracts: string[];
    transactions_hash: string[];
    date: string[];
}

export interface InvestmentInterface {
    amount: number;
    date: string;
}

export interface KYCInterface {
    name: string;
    date_of_birth: string;
    country: string;
    nationality: string;
    government_id: string;
    document_name: string;
    issued_date: string;
    valid_until: string;
    proof_of_address: string;
    status: string;
    applicant_id: string;
}

export interface UserInterface {
    username: string;
    address: string;
    joined_at: string;
    email: string;
    role: string;
    notifications: Notification[];
    language_selected: string;
    referral_code: string;
    referrer: string;
    referees: { [key: string]: number };
    telegram_chat_id?: string;
    notification_preference?: 'email' | 'telegram';
}

export interface UpdateUserInterface {
    address: string;
    username: string;
    email: string;
}

export interface PointsExpense {
    amount: number;
    date: string;
    reason: string;
}

export interface FullUserInterface {
    // Basic Information
    username: string;
    joined_at: string;
    email: string;
    address: string;
    role: string;
    language_selected: string;
    telegram_chat_id?: string;

    // Referral System
    referral_code: string;
    referrer: string;
    referees: { [key: string]: number };

    // Communication
    notifications: Notification[];
    notification_preference?: 'email' | 'telegram';
}

export interface FullUsersInterfaceResponse {
    users: FullUserInterface[];
}

export interface TopUserInterface {
    address: string;
    points: number;
    referrals: number;
}
