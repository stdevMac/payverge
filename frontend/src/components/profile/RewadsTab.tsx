import React, {useMemo, useState, useEffect, useCallback} from "react";
import {
  waitForTransactionReceipt,
  writeContract,
  readContract,
} from "wagmi/actions";
import { config } from "@/config";
import DealManagerABI from "@/artifacts/DealManager.json";
import DealABI from "@/artifacts/Deal.json";
import PortfolioABI from "@/artifacts/Portfolio.json";
import { UserInterface } from "@/interface";
import {
  IoCashOutline,
  IoWalletOutline,
  IoTimeOutline,
  IoChevronDownOutline,
  IoChevronUpOutline,
  IoTrendingUpOutline,
  IoCalendarOutline,
  IoStatsChartOutline,
  IoListOutline,
} from "react-icons/io5";
import { Spinner, Button } from "@nextui-org/react";
import toast from "react-hot-toast";
import { encodeFunctionData } from "viem";

interface RewardsInterface {
  amount: number;
  date: string;
  claimed: boolean;
  contract_address: string;
}
interface RewardsTabProps {
  user: UserInterface;
  reFetchUser: () => void;
}

const RewardsTab: React.FC<RewardsTabProps> = ({
  user,
  reFetchUser,
}) => {
  const [openDropdown, setOpenDropdown] = useState<{
    [key: string]: boolean;
  }>({});
  const [isLoading, setIsLoading] = useState(true);

  const formatCurrency = (value: number) => {
    return new Intl.NumberFormat("en-US", {
      style: "currency",
      currency: "USD",
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(value);
  };

  const formatDate = (dateString: string) => {
    if (!dateString || dateString === "0001-01-01T00:00:00Z") return "Never";
    return new Date(dateString).toLocaleDateString("en-US", {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  };

  return (
    <div className="space-y-6">
      {/* Performance Overview */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
      </div>
    </div>
  );
};

export default RewardsTab;
