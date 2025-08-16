// Purchase Cost Array
export const purchaseCostArray = [
    { key: "purchase_price", label: "Purchase Price" },
    { key: "company_charge", label: "Company Charge" },
    { key: "security_deposit", label: "Security Deposit" },
    { key: "insurance_yearly", label: "Insurance Yearly" },
    { key: "initial_mods", label: "Initial Mods" },
    { key: "fixed_expenses", label: "Fixed Expenses" },
];

// Funding Array
export const fundingArray = [
    { key: "total_filled", label: "Total Funded" },
    { key: "total", label: "Total to be Funded" },
    { key: "percentage", label: "Percentage(%) Funded" },
];

// Expected Returns Array
export const expectedReturnsArray = [
    { key: "rental_return_frequency", label: "Rental Return Frequency" },
    { key: "expected_rental_income", label: "Estimated Rental Income" },
    { key: "annual_operating_expenses", label: "Annual Operating Expenses" },
    { key: "net_annual_rental_income", label: "Net Annual Rental Income" },
    { key: "expected_depreciation", label: "Expected Depreciation" },
    { key: "profit", label: "Expected Profits" },
    { key: "net_profit", label: "Expected Net Profits" },
];

// Car Documents Array
export const carDocumentsArray = [
    {
        key: "documents",
        label: "Documents",
        isArray: true,
        subItems: [
            { key: "name", label: "Name" },
            { key: "path", label: "Link" },
        ],
    },
];

// Car Income Array
export const carIncomeArray = [
    {
        key: "income",
        label: "Income",
        isArray: true,
        subItems: [
            { key: "date", label: "Date", isDate: true },
            { key: "rent_frequency", label: "Rent Frequency" },
            { key: "time_of_rental", label: "Time of Rental" },
            { key: "rent_price", label: "Rent Price" },
            { key: "deposit", label: "Deposit" },
            { key: "receipts_path", label: "Receipts", isArray: true },
        ],
    },
];

// Car Expenses Array
export const carExpensesArray = [
    {
        key: "expenses",
        label: "Expenses",
        isArray: true,
        subItems: [
            { key: "date", label: "Date", isDate: true },
            { key: "expense_type", label: "Expense Type" },
            { key: "amount", label: "Amount" },
            { key: "receipt", label: "Receipt" },
        ],
    },
];

// Car Deposit To Shareholders Array
export const carDepositToShareholdersArray = [
    {
        key: "deposit_to_shareholders",
        label: "Deposit To Share Holders",
        isArray: true,
        subItems: [
            { key: "date", label: "Date", isDate: true },
            { key: "amount", label: "Amount" },
            { key: "transaction_hash", label: "Transaction Hash" },
        ],
    },
];
