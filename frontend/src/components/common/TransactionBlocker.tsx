import React from "react";

interface TransactionBlockerProps {
  isVisible: boolean;
  message: string;
}

export const TransactionBlocker: React.FC<TransactionBlockerProps> = ({
  isVisible,
  message,
}) => {
  if (!isVisible) return null;

  return (
    <div className="fixed inset-0 bg-black/50 backdrop-blur-sm z-[9999] flex items-center justify-center">
      <div className="bg-content1 p-6 rounded-lg shadow-lg max-w-md w-full mx-4">
        <div className="flex flex-col items-center gap-4">
          <div className="w-12 h-12 border-4 border-primary border-t-transparent rounded-full animate-spin" />
          <p className="text-lg font-semibold text-center">{message}</p>
          <p className="text-sm text-default-500 text-center">
            Please wait while the transaction is being processed...
          </p>
        </div>
      </div>
    </div>
  );
};
