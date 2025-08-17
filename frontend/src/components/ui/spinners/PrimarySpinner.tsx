import React from "react";
import { Spinner } from "@nextui-org/react";

export const PrimarySpinner = () => {
  return (
    <div className="fixed inset-0 flex items-center justify-center bg-slate-200 dark:bg-gray-800 transition-colors duration-200">
      <Spinner size="lg" />
    </div>
  );
};
