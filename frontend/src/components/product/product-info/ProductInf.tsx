import React, { ReactNode } from "react";

type ConfigItem = {
  key: string;
  label: string;
  isArray?: boolean;
  subItems?: ConfigItem[];
  isDate?: boolean;
  isInteger?: boolean;
};

type Props<T> = {
  configArray: ConfigItem[];
  data: T;
};

function formatDate(dateString: string): string {
  const date = new Date(dateString);
  const options: Intl.DateTimeFormatOptions = {
    year: "numeric",
    month: "long",
    day: "numeric",
  };
  return date.toLocaleDateString("en-US", options);
}

const renderValue = (value: any, config: ConfigItem): ReactNode => {
  if (config.isDate && typeof value === "string") {
    return formatDate(value);
  }

  // Handle numeric values for Purchase Costs and Expected Returns
  if (typeof value === "number") {
    if (config.key === "percentage") {
      return `${value.toFixed(2)}%`;
    }

    // Check if the value should be displayed as currency
    if (
      [
        "purchase_price",
        "company_charge",
        "security_deposit",
        "insurance_yearly",
        "initial_mods",
        "fixed_expenses",
        "expected_rental_income",
        "annual_operating_expenses",
        "net_annual_rental_income",
        "expected_depreciation",
        "profit",
        "net_profit",
      ].includes(config.key)
    ) {
      return `$${Math.round(value).toLocaleString()}`;
    }

    return value.toLocaleString();
  }

  if (config.isArray && Array.isArray(value)) {
    if (value.length === 0) {
      return (
        <p className="text-gray-500 italic">
          No items available yet. Check back soon!
        </p>
      );
    }
    return (
      <div className="space-y-2">
        {value.map((item, index) => (
          <div key={index} className="bg-gray-100 p-3 rounded-lg">
            {config.subItems?.map((subConfig) => (
              <div
                key={subConfig.key}
                className="flex justify-between items-center mb-1"
              >
                <span className="text-sm font-medium text-gray-600">
                  {subConfig.label}:
                </span>
                <span className="font-semibold text-gray-800">
                  {renderValue(item[subConfig.key], subConfig)}
                </span>
              </div>
            ))}
          </div>
        ))}
      </div>
    );
  }

  if (typeof value === "object" && value !== null) {
    return JSON.stringify(value);
  }

  if (value === undefined || value === null) {
    return <span className="text-gray-500 italic">Not available yet</span>;
  }

  return value;
};

export const ProductInf = <T,>({ configArray, data }: Props<T>) => {
  const singleItems = configArray.filter((item) => !item.isArray);
  const arrayItems = configArray.filter((item) => item.isArray);

  return (
    <div className="mt-4 space-y-4">
      {singleItems.length > 0 && (
        <div className="bg-white p-4 rounded-lg shadow-md">
          {singleItems.map((item) => (
            <div
              key={item.key}
              className="flex justify-between items-center mb-2 last:mb-0"
            >
              <span className="text-sm font-medium text-gray-600">
                {item.label}
              </span>
              <span className="font-semibold text-gray-800">
                {renderValue(data[item.key as keyof T], item)}
              </span>
            </div>
          ))}
        </div>
      )}
      {arrayItems.map((item) => (
        <div key={item.key} className="bg-white p-4 rounded-lg shadow-md">
          <h2 className="text-lg font-semibold text-gray-800 mb-2">
            {item.label}
          </h2>
          {renderValue(data, item)}
        </div>
      ))}
    </div>
  );
};
